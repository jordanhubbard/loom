package loom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jordanhubbard/loom/internal/actions"
	"github.com/jordanhubbard/loom/internal/activity"
	"github.com/jordanhubbard/loom/internal/agent"
	"github.com/jordanhubbard/loom/internal/analytics"
	"github.com/jordanhubbard/loom/internal/beads"
	"github.com/jordanhubbard/loom/internal/comments"
	"github.com/jordanhubbard/loom/internal/database"
	"github.com/jordanhubbard/loom/internal/decision"
	"github.com/jordanhubbard/loom/internal/dispatch"
	"github.com/jordanhubbard/loom/internal/executor"
	"github.com/jordanhubbard/loom/internal/files"
	"github.com/jordanhubbard/loom/internal/gitops"
	"github.com/jordanhubbard/loom/internal/keymanager"
	"github.com/jordanhubbard/loom/internal/logging"
	"github.com/jordanhubbard/loom/internal/metrics"
	"github.com/jordanhubbard/loom/internal/modelcatalog"
	internalmodels "github.com/jordanhubbard/loom/internal/models"
	"github.com/jordanhubbard/loom/internal/motivation"
	"github.com/jordanhubbard/loom/internal/notifications"
	"github.com/jordanhubbard/loom/internal/observability"
	"github.com/jordanhubbard/loom/internal/orgchart"
	"github.com/jordanhubbard/loom/internal/patterns"
	"github.com/jordanhubbard/loom/internal/persona"
	"github.com/jordanhubbard/loom/internal/project"
	"github.com/jordanhubbard/loom/internal/provider"
	"github.com/jordanhubbard/loom/internal/routing"
	"github.com/jordanhubbard/loom/internal/temporal"
	temporalactivities "github.com/jordanhubbard/loom/internal/temporal/activities"
	"github.com/jordanhubbard/loom/internal/temporal/eventbus"
	"github.com/jordanhubbard/loom/internal/temporal/workflows"
	"github.com/jordanhubbard/loom/internal/workflow"
	"github.com/jordanhubbard/loom/pkg/config"
	"github.com/jordanhubbard/loom/pkg/models"
)

const readinessCacheTTL = 2 * time.Minute

type projectReadinessState struct {
	ready     bool
	issues    []string
	checkedAt time.Time
}

// Loom is the main orchestrator
type Loom struct {
	config              *config.Config
	agentManager        *agent.WorkerManager
	actionRouter        *actions.Router
	projectManager      *project.Manager
	personaManager      *persona.Manager
	beadsManager        *beads.Manager
	decisionManager     *decision.Manager
	fileLockManager     *FileLockManager
	orgChartManager     *orgchart.Manager
	providerRegistry    *provider.Registry
	database            *database.Database
	dispatcher          *dispatch.Dispatcher
	eventBus            *eventbus.EventBus
	temporalManager     *temporal.Manager
	modelCatalog        *modelcatalog.Catalog
	gitopsManager       *gitops.Manager
	shellExecutor       *executor.ShellExecutor
	logManager          *logging.Manager
	activityManager     *activity.Manager
	notificationManager *notifications.Manager
	commentsManager     *comments.Manager
	motivationRegistry  *motivation.Registry
	motivationEngine    *motivation.Engine
	idleDetector        *motivation.IdleDetector
	workflowEngine      *workflow.Engine
	patternManager      *patterns.Manager
	metrics             *metrics.Metrics
	keyManager          *keymanager.KeyManager
	readinessMu         sync.Mutex
	readinessCache      map[string]projectReadinessState
	readinessFailures   map[string]time.Time
}

// New creates a new Loom instance
func New(cfg *config.Config) (*Loom, error) {
	personaPath := cfg.Agents.DefaultPersonaPath
	if personaPath == "" {
		personaPath = "./personas"
	}

	providerRegistry := provider.NewRegistry()

	// Initialize Temporal manager if configured
	var temporalMgr *temporal.Manager
	if cfg.Temporal.Host != "" {
		var err error
		temporalMgr, err = temporal.NewManager(&cfg.Temporal)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize temporal: %w", err)
		}
	}

	// Always have an event bus (Temporal-backed when enabled, otherwise in-memory only).
	var eb *eventbus.EventBus
	if temporalMgr != nil && temporalMgr.GetEventBus() != nil {
		eb = temporalMgr.GetEventBus()
	} else {
		eb = eventbus.NewEventBus(nil, &cfg.Temporal)
	}

	// Initialize database if configured
	var db *database.Database
	if cfg.Database.Type == "sqlite" && cfg.Database.Path != "" {
		var err error
		db, err = database.New(cfg.Database.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
	} else if cfg.Database.Type == "postgres" && cfg.Database.DSN != "" {
		var err error
		db, err = database.NewPostgres(cfg.Database.DSN)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize postgres: %w", err)
		}
	}

	// Initialize model catalog from config or use defaults.
	// Priority: 1) config.yaml preferred_models, 2) database override, 3) hardcoded defaults
	modelCatalog := modelcatalog.DefaultCatalog()
	if len(cfg.Models.PreferredModels) > 0 {
		// Convert config models to ModelSpec
		specs := make([]internalmodels.ModelSpec, 0, len(cfg.Models.PreferredModels))
		for _, pm := range cfg.Models.PreferredModels {
			spec := internalmodels.ModelSpec{
				Name:      pm.Name,
				Rank:      pm.Rank,
				MinVRAMGB: pm.MinVRAMGB,
			}
			// Map tier to interactivity
			switch pm.Tier {
			case "extended":
				spec.Interactivity = "slow"
			case "complex":
				spec.Interactivity = "medium"
			case "medium":
				spec.Interactivity = "medium"
			case "simple":
				spec.Interactivity = "fast"
			default:
				spec.Interactivity = "medium"
			}
			specs = append(specs, spec)
		}
		modelCatalog.Replace(specs)
		log.Printf("[ModelCatalog] Loaded %d preferred models from config.yaml", len(specs))
	}
	// Database can override config (for runtime updates via API)
	if db != nil {
		if raw, ok, err := db.GetConfigValue(modelCatalogKey); err == nil && ok {
			var specs []internalmodels.ModelSpec
			if err := json.Unmarshal([]byte(raw), &specs); err == nil && len(specs) > 0 {
				modelCatalog.Replace(specs)
				log.Printf("[ModelCatalog] Overrode with %d models from database", len(specs))
			}
		}
	}

	// Initialize gitops manager for project repository management.
	// baseWorkDir is where project repos are cloned to.
	// projectKeyDir is where SSH keys are stored (separate from clones to prevent
	// git stash/clean from destroying them).
	projectKeyDir := cfg.Git.ProjectKeyDir
	if projectKeyDir == "" {
		projectKeyDir = "/app/data/projects"
	}
	sshKeyDir := filepath.Join(filepath.Dir(projectKeyDir), "keys")
	gitopsMgr, err := gitops.NewManager(projectKeyDir, sshKeyDir, db, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gitops manager: %w", err)
	}

	// For sticky projects running from their own source tree, override the
	// workdir to point to the actual source (cwd) instead of a separate clone.
	cwd, _ := os.Getwd()
	for _, p := range cfg.Projects {
		if p.IsSticky && cwd != "" {
			if _, err := os.Stat(filepath.Join(cwd, ".git")); err == nil {
				gitopsMgr.SetProjectWorkDir(p.ID, cwd)
				log.Printf("[Loom] Project %s workdir set to source tree: %s", p.ID, cwd)
			}
		}
	}

	agentMgr := agent.NewWorkerManager(cfg.Agents.MaxConcurrent, providerRegistry, eb)
	if db != nil {
		agentMgr.SetAgentPersister(db)
		// Enable conversation context support for multi-turn conversations
		agentMgr.GetWorkerPool().SetDatabase(db)
	}

	// Initialize shell executor if database is available
	var shellExec *executor.ShellExecutor
	if db != nil {
		shellExec = executor.NewShellExecutor(db.DB())
	}
	var logMgr *logging.Manager
	if db != nil {
		logMgr = logging.NewManager(db.DB())
	}

	// Initialize motivation system
	motivationRegistry := motivation.NewRegistry(motivation.DefaultConfig())
	idleDetector := motivation.NewIdleDetector(motivation.DefaultIdleConfig())

	// Initialize workflow engine (if database is available)
	var workflowEngine *workflow.Engine
	if db != nil {
		beadsMgr := beads.NewManager(cfg.Beads.BDPath)
		workflowEngine = workflow.NewEngine(db, beadsMgr)
	}

	// Initialize activity, notification, and comments managers
	var activityMgr *activity.Manager
	var notificationMgr *notifications.Manager
	var commentsMgr *comments.Manager
	if db != nil {
		activityMgr = activity.NewManager(db, eb)
		notificationMgr = notifications.NewManager(db, activityMgr)
		commentsMgr = comments.NewManager(db, notificationMgr, eb)
	}

	// Initialize pattern manager and analytics logger if database is available
	var patternMgr *patterns.Manager
	if db != nil {
		analyticsStorage, err := analytics.NewDatabaseStorage(db.DB())
		if err == nil && analyticsStorage != nil {
			patternMgr = patterns.NewManager(analyticsStorage, nil)
			// Wire analytics logger to WorkerManager so LLM completions are logged
			agentMgr.SetAnalyticsLogger(analytics.NewLogger(analyticsStorage, analytics.DefaultPrivacyConfig()))
		}
	}

	arb := &Loom{
		config:              cfg,
		agentManager:        agentMgr,
		projectManager:      project.NewManager(),
		personaManager:      persona.NewManager(personaPath),
		beadsManager:        beads.NewManager(cfg.Beads.BDPath),
		decisionManager:     decision.NewManager(),
		fileLockManager:     NewFileLockManager(cfg.Agents.FileLockTimeout),
		orgChartManager:     orgchart.NewManager(),
		providerRegistry:    providerRegistry,
		database:            db,
		eventBus:            eb,
		temporalManager:     temporalMgr,
		modelCatalog:        modelCatalog,
		gitopsManager:       gitopsMgr,
		shellExecutor:       shellExec,
		logManager:          logMgr,
		activityManager:     activityMgr,
		notificationManager: notificationMgr,
		commentsManager:     commentsMgr,
		motivationRegistry:  motivationRegistry,
		idleDetector:        idleDetector,
		workflowEngine:      workflowEngine,
		patternManager:      patternMgr,
		metrics:             metrics.NewMetrics(),
	}

	actionRouter := &actions.Router{
		Beads:     arb,
		Closer:    arb,
		Escalator: arb,
		Commands:  arb,
		Files:     files.NewManager(gitopsMgr),
		Git:       actions.NewProjectGitRouter(gitopsMgr),
		Logger:    arb,
		Workflow:  arb,
		BeadType:  "task",
		DefaultP0: true,
	}
	arb.actionRouter = actionRouter
	agentMgr.SetActionRouter(actionRouter)

	// Enable multi-turn action loop
	agentMgr.SetActionLoopEnabled(true)
	agentMgr.SetMaxLoopIterations(15)
	if db != nil {
		agentMgr.SetDatabase(db)
		lessonsProvider := dispatch.NewLessonsProvider(db)
		if lessonsProvider != nil {
			agentMgr.SetLessonsProvider(lessonsProvider)
		}
	}

	arb.dispatcher = dispatch.NewDispatcher(arb.beadsManager, arb.projectManager, arb.agentManager, arb.providerRegistry, eb)
	arb.readinessCache = make(map[string]projectReadinessState)
	arb.readinessFailures = make(map[string]time.Time)
	arb.dispatcher.SetReadinessCheck(arb.CheckProjectReadiness)
	arb.dispatcher.SetReadinessMode(dispatch.ReadinessMode(cfg.Readiness.Mode))
	arb.dispatcher.SetMaxDispatchHops(cfg.Dispatch.MaxHops)
	arb.dispatcher.SetEscalator(arb)
	// Enable conversation context support for multi-turn conversations
	if db != nil {
		arb.dispatcher.SetDatabase(db)
	}

	// Setup provider metrics tracking
	arb.setupProviderMetrics()

	return arb, nil
}

// setupProviderMetrics sets up metrics tracking callback for provider requests
func (a *Loom) setupProviderMetrics() {
	if a.metrics == nil || a.providerRegistry == nil {
		return
	}

	// Set metrics callback to record provider requests
	a.providerRegistry.SetMetricsCallback(func(providerID string, success bool, latencyMs int64, totalTokens int64) {
		// Update provider metrics
		if a.metrics != nil {
			a.metrics.RecordProviderRequest(providerID, "", success, latencyMs, totalTokens)
		}

		// Also update provider model metrics if available
		if a.database == nil {
			return
		}
		provider, err := a.database.GetProvider(providerID)
		if err != nil || provider == nil {
			return
		}
		// Record success/failure on provider model
		if success {
			provider.RecordSuccess(latencyMs, totalTokens)
		} else {
			provider.RecordFailure(latencyMs)
		}
		// Persist updated metrics
		_ = a.database.UpsertProvider(provider)

		// Emit event for real-time updates
		if a.eventBus != nil {
			_ = a.eventBus.Publish(&eventbus.Event{
				Type: eventbus.EventTypeProviderUpdated,
				Data: map[string]interface{}{
					"provider_id":   providerID,
					"success":       success,
					"latency_ms":    latencyMs,
					"total_tokens":  totalTokens,
					"overall_score": provider.GetScore(),
				},
			})
		}
	})
}

// Initialize sets up loom
func (a *Loom) Initialize(ctx context.Context) error {
	// Prefer database-backed configuration when available.
	var projects []*models.Project
	if a.database != nil {
		storedProjects, err := a.database.ListProjects()
		if err != nil {
			return fmt.Errorf("failed to load projects: %w", err)
		}
		if len(storedProjects) > 0 {
			projects = storedProjects
			known := map[string]struct{}{}
			for _, project := range storedProjects {
				if project == nil {
					continue
				}
				known[project.ID] = struct{}{}
			}
			for _, p := range a.config.Projects {
				if !p.IsSticky {
					continue
				}
				if _, ok := known[p.ID]; ok {
					continue
				}
				proj := &models.Project{
					ID:              p.ID,
					Name:            p.Name,
					GitRepo:         p.GitRepo,
					Branch:          p.Branch,
					BeadsPath:       p.BeadsPath,
					GitAuthMethod:   models.GitAuthMethod(p.GitAuthMethod),
					GitStrategy:     normalizeGitStrategy(models.GitStrategy(p.GitStrategy)),
					GitCredentialID: p.GitCredentialID,
					IsPerpetual:     p.IsPerpetual,
					IsSticky:        p.IsSticky,
					Context:         p.Context,
					Status:          models.ProjectStatusOpen,
				}
				_ = a.database.UpsertProject(proj)
				projects = append(projects, proj)
			}
		} else {
			// Bootstrap from config.yaml into the configuration database.
			for _, p := range a.config.Projects {
				proj := &models.Project{
					ID:              p.ID,
					Name:            p.Name,
					GitRepo:         p.GitRepo,
					Branch:          p.Branch,
					BeadsPath:       p.BeadsPath,
					GitAuthMethod:   models.GitAuthMethod(p.GitAuthMethod),
					GitStrategy:     normalizeGitStrategy(models.GitStrategy(p.GitStrategy)),
					GitCredentialID: p.GitCredentialID,
					IsPerpetual:     p.IsPerpetual,
					IsSticky:        p.IsSticky,
					Context:         p.Context,
					Status:          models.ProjectStatusOpen,
				}
				_ = a.database.UpsertProject(proj)
				projects = append(projects, proj)
			}
		}
	} else {
		for _, p := range a.config.Projects {
			projects = append(projects, &models.Project{
				ID:              p.ID,
				Name:            p.Name,
				GitRepo:         p.GitRepo,
				Branch:          p.Branch,
				BeadsPath:       p.BeadsPath,
				GitAuthMethod:   models.GitAuthMethod(p.GitAuthMethod),
				GitStrategy:     normalizeGitStrategy(models.GitStrategy(p.GitStrategy)),
				GitCredentialID: p.GitCredentialID,
				IsPerpetual:     p.IsPerpetual,
				IsSticky:        p.IsSticky,
				Context:         p.Context,
				Status:          models.ProjectStatusOpen,
			})
		}
	}

	// Load projects into the in-memory project manager.
	var projectValues []models.Project
	for _, p := range projects {
		if p == nil {
			continue
		}
		copy := *p
		copy.BeadsPath = normalizeBeadsPath(copy.BeadsPath)
		copy.GitAuthMethod = normalizeGitAuthMethod(copy.GitRepo, copy.GitAuthMethod)
		projectValues = append(projectValues, copy)
	}
	if len(projectValues) == 0 && len(a.config.Projects) > 0 {
		for _, p := range a.config.Projects {
			projectValues = append(projectValues, models.Project{
				ID:              p.ID,
				Name:            p.Name,
				GitRepo:         p.GitRepo,
				Branch:          p.Branch,
				BeadsPath:       normalizeBeadsPath(p.BeadsPath),
				GitAuthMethod:   normalizeGitAuthMethod(p.GitRepo, models.GitAuthMethod(p.GitAuthMethod)),
				GitStrategy:     normalizeGitStrategy(models.GitStrategy(p.GitStrategy)),
				GitCredentialID: p.GitCredentialID,
				IsPerpetual:     p.IsPerpetual,
				IsSticky:        p.IsSticky,
				Context:         p.Context,
				Status:          models.ProjectStatusOpen,
			})
		}
	}
	if len(projectValues) == 0 {
		projectValues = append(projectValues, models.Project{
			ID:            "loom",
			Name:          "Loom",
			GitRepo:       ".",
			Branch:        "main",
			BeadsPath:     normalizeBeadsPath(".beads"),
			GitAuthMethod: normalizeGitAuthMethod(".", ""),
			GitStrategy:   models.GitStrategyDirect,
			IsPerpetual:   true,
			IsSticky:      true,
			Context: map[string]string{
				"build_command": "make build",
				"test_command":  "make test",
				"lint_command":  "make lint",
			},
			Status: models.ProjectStatusOpen,
		})
	}
	if err := a.projectManager.LoadProjects(projectValues); err != nil {
		return fmt.Errorf("failed to load projects: %w", err)
	}
	if a.database != nil {
		for i := range projectValues {
			p := projectValues[i]
			_ = a.database.UpsertProject(&p)
		}
	}

	// Load beads from registered projects.
	for i := range projectValues {
		p := &projectValues[i]
		if p.BeadsPath == "" {
			continue
		}

		// Detect local project: git_repo is "." or empty, OR the beads path
		// already exists in the current working directory (self-hosted project).
		isLocal := p.GitRepo == "" || p.GitRepo == "."
		if !isLocal {
			if _, err := os.Stat(p.BeadsPath); err == nil {
				isLocal = true
			}
		}

		// For projects with git repositories, clone/pull them first
		if !isLocal {
			// Set default auth method if not specified
			if p.GitAuthMethod == "" {
				p.GitAuthMethod = models.GitAuthNone // Default to no auth for public repos
			}

			// For SSH-auth projects, ensure an SSH key exists
			if p.GitAuthMethod == models.GitAuthSSH {
				pubKey, err := a.gitopsManager.EnsureProjectSSHKey(p.ID)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to ensure SSH key for %s: %v\n", p.ID, err)
				} else {
					fmt.Printf("Project %s SSH public key:\n%s\n", p.ID, pubKey)
				}
			}

		// Check if already cloned
		workDir := a.gitopsManager.GetProjectWorkDir(p.ID)
		p.WorkDir = workDir
		// Persist WorkDir so maintenance loop and dispatcher can find project files
		if mgdProject, _ := a.projectManager.GetProject(p.ID); mgdProject != nil {
			mgdProject.WorkDir = workDir
		}

		needsClone := false
		gitDir := filepath.Join(workDir, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			needsClone = true
		} else {
			// .git exists, but check if it's a valid clone (has commits)
			// An empty git-init repo with no commits means clone never succeeded
			checkCmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
			checkCmd.Dir = workDir
			if out, err := checkCmd.CombinedOutput(); err != nil {
				outStr := strings.TrimSpace(string(out))
				if strings.Contains(outStr, "does not have any commits") || strings.Contains(outStr, "unknown revision") {
					fmt.Printf("Project %s has empty repo (prior clone failed), re-cloning...\n", p.ID)
					// Remove the broken repo so CloneProject can start fresh
					os.RemoveAll(workDir)
					needsClone = true
				}
			}
		}

		if needsClone {
			// Clone the repository
			fmt.Printf("Cloning project %s from %s...\n", p.ID, p.GitRepo)
			if err := a.gitopsManager.CloneProject(ctx, p); err != nil {
				errStr := err.Error()
				fmt.Fprintf(os.Stderr, "Warning: Failed to clone project %s: %v\n", p.ID, err)

				// If SSH auth failed, show the deploy key that needs to be registered
				if p.GitAuthMethod == models.GitAuthSSH && strings.Contains(errStr, "Permission denied") {
					if pubKey, keyErr := a.gitopsManager.EnsureProjectSSHKey(p.ID); keyErr == nil {
						fmt.Fprintf(os.Stderr, "\n"+
							"╔══════════════════════════════════════════════════════════════════╗\n"+
							"║  DEPLOY KEY NOT REGISTERED                                      ║\n"+
							"║                                                                  ║\n"+
							"║  Add this deploy key to your git remote:                         ║\n"+
							"║  %s\n"+
							"║                                                                  ║\n"+
							"║  For GitHub: Settings → Deploy Keys → Add deploy key             ║\n"+
							"║  Enable 'Allow write access' if agents need to push.             ║\n"+
							"╚══════════════════════════════════════════════════════════════════╝\n\n",
							pubKey)
					}
				}
				continue
			}
			fmt.Printf("Successfully cloned project %s\n", p.ID)
		} else {
			// Pull latest changes
			fmt.Printf("Pulling latest changes for project %s...\n", p.ID)
			if err := a.gitopsManager.PullProject(ctx, p); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to pull project %s: %v\n", p.ID, err)
				// Continue anyway with existing checkout
			} else {
				fmt.Printf("Successfully pulled project %s\n", p.ID)
			}
		}

			// Initialize beads database if needed.
			// For dolt backend, ensure bd is initialized with the correct prefix
			// so that bead creation doesn't fail with "database not initialized".
			{
				beadsDir := filepath.Join(workDir, p.BeadsPath)
				if _, err := os.Stat(beadsDir); err == nil {
					bdPath := a.config.Beads.BDPath
					if bdPath == "" {
						bdPath = "bd"
					}
					// Determine prefix for this project
					bdPrefix := p.BeadPrefix
					if bdPrefix == "" {
						bdPrefix = p.ID
					}
					initArgs := []string{"init", "--prefix", bdPrefix}
					if a.config.Beads.Backend != "dolt" {
						initArgs = append(initArgs, "--from-jsonl")
					}
					initCmd := exec.Command(bdPath, initArgs...)
					initCmd.Dir = workDir
					if out, err := initCmd.CombinedOutput(); err != nil {
						outStr := strings.TrimSpace(string(out))
						if !strings.Contains(outStr, "already initialized") {
							fmt.Fprintf(os.Stderr, "Warning: bd init failed for %s: %v (%s)\n", p.ID, err, outStr)
						}
					} else {
						fmt.Printf("Initialized beads database for project %s\n", p.ID)
					}
				}
			}

			// Update project in database with git metadata
			if a.database != nil {
				_ = a.database.UpsertProject(p)
			}

			// Load beads from the cloned repository
			beadsPath := filepath.Join(workDir, p.BeadsPath)
			a.beadsManager.SetBeadsPath(beadsPath)
			// Load project prefix from config
			configPath := filepath.Join(workDir, p.BeadsPath)
			_ = a.beadsManager.LoadProjectPrefixFromConfig(p.ID, configPath)
			// Use project's BeadPrefix if set in the model
			if p.BeadPrefix != "" {
				a.beadsManager.SetProjectPrefix(p.ID, p.BeadPrefix)
			}
			_ = a.beadsManager.LoadBeadsFromFilesystem(p.ID, beadsPath)

			// Federation sync after loading beads
			if a.config.Beads.Federation.Enabled && a.config.Beads.Federation.AutoSync {
				fmt.Printf("Syncing federation peers for project %s...\n", p.ID)
				if err := a.beadsManager.SyncFederation(ctx, &a.config.Beads.Federation); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Federation sync failed for %s: %v\n", p.ID, err)
				}
				// Reload beads after sync to pick up remote changes
				_ = a.beadsManager.LoadBeadsFromFilesystem(p.ID, beadsPath)
			}
		} else {
			// Local project (Loom itself), load beads directly
			cwd, _ := os.Getwd()
			p.WorkDir = cwd
			if mgdProject, _ := a.projectManager.GetProject(p.ID); mgdProject != nil {
				mgdProject.WorkDir = cwd
			}
			a.beadsManager.SetBeadsPath(p.BeadsPath)
			// Load project prefix from config
			_ = a.beadsManager.LoadProjectPrefixFromConfig(p.ID, p.BeadsPath)
			// Use project's BeadPrefix if set in the model
			if p.BeadPrefix != "" {
				a.beadsManager.SetProjectPrefix(p.ID, p.BeadPrefix)
			}
			_ = a.beadsManager.LoadBeadsFromFilesystem(p.ID, p.BeadsPath)

			// Federation sync after loading beads
			if a.config.Beads.Federation.Enabled && a.config.Beads.Federation.AutoSync {
				fmt.Printf("Syncing federation peers for project %s...\n", p.ID)
				if err := a.beadsManager.SyncFederation(ctx, &a.config.Beads.Federation); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Federation sync failed for %s: %v\n", p.ID, err)
				}
				// Reload beads after sync to pick up remote changes
				_ = a.beadsManager.LoadBeadsFromFilesystem(p.ID, p.BeadsPath)
			}
		}
	}

	// Load providers from database into the in-memory registry.
	if a.database != nil {
		providers, err := a.database.ListProviders()
		if err != nil {
			return fmt.Errorf("failed to load providers: %w", err)
		}
		if len(providers) == 0 && len(a.config.Providers) > 0 {
			for _, cfgProvider := range a.config.Providers {
				if !cfgProvider.Enabled {
					continue
				}
				providerID := cfgProvider.ID
				if providerID == "" && cfgProvider.Name != "" {
					providerID = strings.ReplaceAll(strings.ToLower(cfgProvider.Name), " ", "-")
				}
				if providerID == "" {
					log.Printf("Skipping provider seed without id or name: endpoint=%s", cfgProvider.Endpoint)
					continue
				}
				seed := &internalmodels.Provider{
					ID:          providerID,
					Name:        cfgProvider.Name,
					Type:        cfgProvider.Type,
					Endpoint:    cfgProvider.Endpoint,
					Model:       cfgProvider.Model,
					RequiresKey: cfgProvider.APIKey != "",
					Status:      "pending",
				}
				if _, regErr := a.RegisterProvider(ctx, seed); regErr != nil {
					log.Printf("Failed to seed provider %s: %v", providerID, regErr)
				}
			}
			providers, err = a.database.ListProviders()
			if err != nil {
				return fmt.Errorf("failed to reload providers: %w", err)
			}
		}
		for _, p := range providers {
			selected := p.SelectedModel
			if selected == "" {
				selected = p.Model
			}
			if selected == "" {
				selected = p.ConfiguredModel
			}
			_ = a.providerRegistry.Upsert(&provider.ProviderConfig{
				ID:                     p.ID,
				Name:                   p.Name,
				Type:                   p.Type,
				Endpoint:               normalizeProviderEndpoint(p.Endpoint),
				APIKey:                 "",
				Model:                  selected,
				ConfiguredModel:        p.ConfiguredModel,
				SelectedModel:          selected,
				SelectedGPU:            p.SelectedGPU,
				Status:                 p.Status,
				LastHeartbeatAt:        p.LastHeartbeatAt,
				LastHeartbeatLatencyMs: p.LastHeartbeatLatencyMs,
			})
		}

		// Count providers ready for dispatch
		healthyCount := 0
		for _, p := range providers {
			if p.Status == "healthy" {
				healthyCount++
			}
		}
		if healthyCount > 0 {
			log.Printf("[Loom] %d providers already healthy, dispatch ready", healthyCount)
		} else {
			log.Printf("[Loom] No providers healthy yet — heartbeat will activate them shortly")
		}

		// Restore agents from database (best-effort).
		storedAgents, err := a.database.ListAgents()
		if err != nil {
			return fmt.Errorf("failed to load agents: %w", err)
		}
		for _, ag := range storedAgents {
			if ag == nil {
				continue
			}
			// Attach persona (required for the system prompt).
			persona, err := a.personaManager.LoadPersona(ag.PersonaName)
			if err != nil {
				continue
			}
			ag.Persona = persona
			// Ensure a provider exists.
			if ag.ProviderID == "" {
				providers := a.providerRegistry.ListActive()
				if len(providers) == 0 {
					continue
				}
				ag.ProviderID = providers[0].Config.ID
			}
			_, _ = a.agentManager.RestoreAgentWorker(ctx, ag)
			_ = a.projectManager.AddAgentToProject(ag.ProjectID, ag.ID)
		}
	}

	// Ensure all projects are persisted to the database before creating agents (to avoid FK constraint failures)
	if a.database != nil {
		log.Printf("Persisting %d project(s) to database before agent creation", len(projectValues))
		for i := range projectValues {
			p := &projectValues[i]
			if err := a.database.UpsertProject((*models.Project)(p)); err != nil {
				log.Printf("Warning: Failed to persist project %s: %v", p.ID, err)
			} else {
				log.Printf("Successfully persisted project %s to database", p.ID)
			}
		}
	}

	// Ensure default agents are assigned for each project.
	for _, p := range projectValues {
		if p.ID == "" {
			continue
		}
		_ = a.ensureDefaultAgents(ctx, p.ID)
	}

	// Attach healthy providers to any paused agents after creating default agents
	// Small delay to ensure agents are persisted to database
	time.Sleep(500 * time.Millisecond)
	healthyProviders := a.providerRegistry.ListActive()
	for _, provider := range healthyProviders {
		if provider != nil && provider.Config != nil {
			log.Printf("Attaching healthy provider %s to paused agents on startup", provider.Config.ID)
			a.attachProviderToPausedAgents(ctx, provider.Config.ID)
		}
	}

	// Register dispatch activities and start the Temporal worker if configured.

	// Start Temporal worker if configured
	if a.temporalManager != nil {
		a.temporalManager.RegisterActivity(temporalactivities.NewDispatchActivities(a.dispatcher))
		a.temporalManager.RegisterActivity(temporalactivities.NewProviderActivities(a.providerRegistry, a.database, a.eventBus, a.modelCatalog, a.keyManager))
		a.temporalManager.RegisterActivity(temporalactivities.NewLoomActivities(a.database, a.dispatcher, a.beadsManager, a.agentManager))

		if err := a.temporalManager.Start(); err != nil {
			return fmt.Errorf("failed to start temporal: %w", err)
		}

		// Start the Ralph Loop (10 second interval) — drains all dispatchable work per beat
		_ = a.temporalManager.StartLoomHeartbeatWorkflow(ctx, 10*time.Second)
		// Start provider heartbeats (monitor provider health)
		_ = a.startProviderHeartbeats(ctx)
	}

	// Kick-start work on all open beads across registered projects.
	a.kickstartOpenBeads(ctx)

	// Register default motivations for all agent roles
	if a.motivationRegistry != nil {
		if err := motivation.RegisterDefaults(a.motivationRegistry); err != nil {
			log.Printf("Warning: Failed to register default motivations: %v", err)
		} else {
			log.Printf("Registered %d default motivations", a.motivationRegistry.Count())
		}
	}

	// FIX #1: Start motivation engine evaluation loop
	// The motivation engine creates beads automatically based on conditions
	// (idle detection, deadline monitoring, budget thresholds, etc.)
	if a.motivationEngine != nil {
		if err := a.motivationEngine.Start(ctx); err != nil {
			log.Printf("[Loom] Warning: Failed to start motivation engine: %v", err)
		} else {
			log.Printf("[Loom] Motivation engine started successfully")
		}
	} else {
		log.Printf("[Loom] Warning: Motivation engine not initialized")
	}

	// FIX #4: Ensure at least one project has beads for work to flow
	// If no beads exist across all projects, create a diagnostic bead
	hasBeads := false
	allProjects := a.projectManager.ListProjects()
	for _, proj := range allProjects {
		if proj == nil {
			continue
		}
		beads, _ := a.beadsManager.ListBeads(map[string]interface{}{"project_id": proj.ID})
		if len(beads) > 0 {
			hasBeads = true
			break
		}
	}

	// If no beads exist and we have at least one project, create a sample diagnostic bead
	if !hasBeads && len(allProjects) > 0 {
		proj := allProjects[0]
		log.Printf("[Loom] No beads found - creating sample diagnostic bead for project %s", proj.ID)

		bead, err := a.beadsManager.CreateBead(
			"System diagnostic check",
			"This bead was auto-created to verify the work flow is operational. The system detected no beads in any project and created this diagnostic task. If you see this bead completed successfully, the work flow is functioning correctly.",
			models.BeadPriorityP2,
			"task",
			proj.ID,
		)
		if err != nil {
			log.Printf("[Loom] Failed to create sample diagnostic bead: %v", err)
		} else {
			log.Printf("[Loom] Created sample diagnostic bead: %s", bead.ID)
		}
	} else if len(allProjects) == 0 {
		log.Printf("[Loom] Warning: No projects configured - no work can be dispatched")
	} else {
		log.Printf("[Loom] Found existing beads across projects - work flow should be operational")
	}

	// Load default workflows
	if a.database != nil && a.workflowEngine != nil {
		workflowsDir := "./workflows/defaults"
		if _, err := os.Stat(workflowsDir); err == nil {
			log.Printf("Loading default workflows from %s", workflowsDir)
			if err := workflow.InstallDefaultWorkflows(a.database, workflowsDir); err != nil {
				log.Printf("Warning: Failed to load default workflows: %v", err)
			} else {
				log.Printf("Successfully loaded default workflows")
			}
		} else {
			log.Printf("Default workflows directory not found: %s", workflowsDir)
		}

		// Set workflow engine in dispatcher for workflow-aware routing
		if a.dispatcher != nil {
			a.dispatcher.SetWorkflowEngine(a.workflowEngine)
			log.Printf("Workflow engine connected to dispatcher")
		}
	}

	return nil
}

// kickstartOpenBeads starts Temporal workflows for all open beads in registered projects.
// This ensures that when Loom starts (or restarts), all pending work is queued for processing.
func (a *Loom) kickstartOpenBeads(ctx context.Context) {
	projects := a.projectManager.ListProjects()
	if len(projects) == 0 {
		return
	}

	var totalKickstarted int
	for _, p := range projects {
		if p == nil || p.ID == "" {
			continue
		}

		beadsList, err := a.beadsManager.GetReadyBeads(p.ID)
		if err != nil {
			continue
		}

		for _, b := range beadsList {
			if b == nil {
				continue
			}
			// Skip decision beads - they require human/CEO input
			if b.Type == "decision" {
				continue
			}
			// Skip beads that are already in progress with an assigned agent
			if b.Status == models.BeadStatusInProgress && b.AssignedTo != "" {
				continue
			}

			// Publish event to signal work is available
			if a.eventBus != nil {
				_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadCreated, b.ID, p.ID, map[string]interface{}{
					"title":       b.Title,
					"type":        b.Type,
					"priority":    b.Priority,
					"kickstarted": true,
				})
			}

			// Start Temporal workflow for the bead if Temporal is enabled
			if a.temporalManager != nil {
				if err := a.temporalManager.StartBeadWorkflow(ctx, b.ID, p.ID, b.Title, b.Description, int(b.Priority), b.Type); err != nil {
					// Log error but continue with other beads
					fmt.Printf("Warning: failed to kickstart bead workflow %s: %v\n", b.ID, err)
					continue
				}
			}

			totalKickstarted++
		}
	}

	if totalKickstarted > 0 {
		fmt.Printf("Kickstarted %d open bead(s) across %d project(s)\n", totalKickstarted, len(projects))
	}
}

// Shutdown gracefully shuts down loom
func (a *Loom) Shutdown() {
	a.agentManager.StopAll()
	if a.temporalManager != nil {
		a.temporalManager.Stop()
	}
	if a.eventBus != nil {
		// Avoid double-closing the Temporal-backed event bus.
		if a.temporalManager == nil || a.temporalManager.GetEventBus() != a.eventBus {
			a.eventBus.Close()
		}
	}
	if a.database != nil {
		_ = a.database.Close()
	}
}

// GetTemporalManager returns the Temporal manager
func (a *Loom) GetTemporalManager() *temporal.Manager {
	return a.temporalManager
}

func (a *Loom) GetEventBus() *eventbus.EventBus {
	return a.eventBus
}

// GetDatabase returns the database instance
func (a *Loom) GetDatabase() *database.Database {
	return a.database
}

// ExecuteShellCommand executes a shell command and logs it
func (a *Loom) ExecuteShellCommand(ctx context.Context, req executor.ExecuteCommandRequest) (*executor.ExecuteCommandResult, error) {
	if a.shellExecutor == nil {
		return nil, fmt.Errorf("shell executor not available (database not configured)")
	}
	return a.shellExecutor.ExecuteCommand(ctx, req)
}

// ExecuteCommand satisfies actions.CommandExecutor.
func (a *Loom) ExecuteCommand(ctx context.Context, req executor.ExecuteCommandRequest) (*executor.ExecuteCommandResult, error) {
	return a.ExecuteShellCommand(ctx, req)
}

func (a *Loom) LogAction(ctx context.Context, actx actions.ActionContext, action actions.Action, result actions.Result) {
	metadata := map[string]interface{}{
		"agent_id":    actx.AgentID,
		"bead_id":     actx.BeadID,
		"project_id":  actx.ProjectID,
		"action_type": action.Type,
		"status":      result.Status,
		"message":     result.Message,
	}
	for k, v := range result.Metadata {
		metadata[k] = v
	}
	if a.logManager != nil {
		a.logManager.Log(logging.LogLevelInfo, "actions", "action executed", metadata)
	}
	observability.Info("agent.action", metadata)
}

// GetCommandLogs retrieves command logs with filters
func (a *Loom) GetCommandLogs(filters map[string]interface{}, limit int) ([]*models.CommandLog, error) {
	if a.shellExecutor == nil {
		return nil, fmt.Errorf("shell executor not available (database not configured)")
	}
	return a.shellExecutor.GetCommandLogs(filters, limit)
}

// GetCommandLog retrieves a single command log by ID
func (a *Loom) GetCommandLog(id string) (*models.CommandLog, error) {
	if a.shellExecutor == nil {
		return nil, fmt.Errorf("shell executor not available (database not configured)")
	}
	return a.shellExecutor.GetCommandLog(id)
}

// GetAgentManager returns the agent manager
func (a *Loom) GetAgentManager() *agent.WorkerManager {
	return a.agentManager
}

func (a *Loom) GetProviderRegistry() *provider.Registry {
	return a.providerRegistry
}

func (a *Loom) GetActionRouter() *actions.Router {
	return a.actionRouter
}

func (a *Loom) GetGitOpsManager() *gitops.Manager {
	return a.gitopsManager
}

// SetKeyManager sets the key manager for encrypted credential storage.
// This must be called after Loom is created (since KeyManager is initialized separately in main).
func (a *Loom) SetKeyManager(km *keymanager.KeyManager) {
	a.keyManager = km
	// Also wire it into gitops manager for SSH key DB persistence
	if a.gitopsManager != nil {
		a.gitopsManager.SetKeyManager(km)
	}
}

// GetKeyManager returns the key manager
func (a *Loom) GetKeyManager() *keymanager.KeyManager {
	return a.keyManager
}

func (a *Loom) GetDispatcher() *dispatch.Dispatcher {
	return a.dispatcher
}

// GetProjectManager returns the project manager
func (a *Loom) GetProjectManager() *project.Manager {
	return a.projectManager
}

// GetPersonaManager returns the persona manager
func (a *Loom) GetPersonaManager() *persona.Manager {
	return a.personaManager
}

// GetBeadsManager returns the beads manager
func (a *Loom) GetBeadsManager() *beads.Manager {
	return a.beadsManager
}

// GetDecisionManager returns the decision manager
func (a *Loom) GetDecisionManager() *decision.Manager {
	return a.decisionManager
}

// GetOrgChartManager returns the org chart manager
func (a *Loom) GetOrgChartManager() *orgchart.Manager {
	return a.orgChartManager
}

// GetMotivationRegistry returns the motivation registry
func (a *Loom) GetMotivationRegistry() *motivation.Registry {
	return a.motivationRegistry
}

// GetMotivationEngine returns the motivation engine
func (a *Loom) GetMotivationEngine() *motivation.Engine {
	return a.motivationEngine
}

// GetIdleDetector returns the idle detector
func (a *Loom) GetIdleDetector() *motivation.IdleDetector {
	return a.idleDetector
}

// GetWorkflowEngine returns the workflow engine
func (a *Loom) GetWorkflowEngine() *workflow.Engine {
	return a.workflowEngine
}

// GetActivityManager returns the activity manager
func (a *Loom) GetActivityManager() *activity.Manager {
	return a.activityManager
}

// GetNotificationManager returns the notification manager
func (a *Loom) GetNotificationManager() *notifications.Manager {
	return a.notificationManager
}

// GetCommentsManager returns the comments manager
func (a *Loom) GetCommentsManager() *comments.Manager {
	return a.commentsManager
}

// GetLogManager returns the log manager
func (a *Loom) GetLogManager() *logging.Manager {
	return a.logManager
}

// GetPatternManager returns the pattern manager
func (a *Loom) GetPatternManager() *patterns.Manager {
	return a.patternManager
}

// AdvanceWorkflowWithCondition advances a bead's workflow with a specific condition
func (a *Loom) AdvanceWorkflowWithCondition(beadID, agentID string, condition string, resultData map[string]string) error {
	if a.workflowEngine == nil {
		return fmt.Errorf("workflow engine not available")
	}

	// Get workflow execution for this bead
	execution, err := a.workflowEngine.GetDatabase().GetWorkflowExecutionByBeadID(beadID)
	if err != nil {
		return fmt.Errorf("failed to get workflow execution: %w", err)
	}
	if execution == nil {
		return fmt.Errorf("no workflow execution found for bead %s", beadID)
	}

	// Convert condition string to EdgeCondition
	var edgeCondition workflow.EdgeCondition
	switch condition {
	case "approved":
		edgeCondition = workflow.EdgeConditionApproved
	case "rejected":
		edgeCondition = workflow.EdgeConditionRejected
	case "success":
		edgeCondition = workflow.EdgeConditionSuccess
	case "failure":
		edgeCondition = workflow.EdgeConditionFailure
	case "timeout":
		edgeCondition = workflow.EdgeConditionTimeout
	case "escalated":
		edgeCondition = workflow.EdgeConditionEscalated
	default:
		return fmt.Errorf("unknown workflow condition: %s", condition)
	}

	// Advance the workflow
	return a.workflowEngine.AdvanceWorkflow(execution.ID, edgeCondition, agentID, resultData)
}

// StartDevelopment is handled directly by the router via MCP tools
func (a *Loom) StartDevelopment(ctx context.Context, workflow string, requireReviews bool, projectPath string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("StartDevelopment is handled directly by the router")
}

// WhatsNext is handled directly by the router via MCP tools
func (a *Loom) WhatsNext(ctx context.Context, userInput, contextStr, conversationSummary string, recentMessages []map[string]string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("WhatsNext is handled directly by the router")
}

// ProceedToPhase is handled directly by the router via MCP tools
func (a *Loom) ProceedToPhase(ctx context.Context, targetPhase, reviewState, reason string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("ProceedToPhase is handled directly by the router")
}

// ConductReview is handled directly by the router via MCP tools
func (a *Loom) ConductReview(ctx context.Context, targetPhase string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("ConductReview is handled directly by the router")
}

// ResumeWorkflow is handled directly by the router via MCP tools
func (a *Loom) ResumeWorkflow(ctx context.Context, includeSystemPrompt bool) (map[string]interface{}, error) {
	return nil, fmt.Errorf("ResumeWorkflow is handled directly by the router")
}

// GetWorkerManager returns the agent worker manager
func (a *Loom) GetWorkerManager() *agent.WorkerManager {
	return a.agentManager
}

// Project management helpers

func (a *Loom) CreateProject(name, gitRepo, branch, beadsPath string, ctxMap map[string]string) (*models.Project, error) {
	p, err := a.projectManager.CreateProject(name, gitRepo, branch, beadsPath, ctxMap)
	if err != nil {
		return nil, err
	}
	p.BeadsPath = normalizeBeadsPath(p.BeadsPath)
	p.GitAuthMethod = normalizeGitAuthMethod(p.GitRepo, p.GitAuthMethod)
	_ = a.ensureDefaultAgents(context.Background(), p.ID)
	if a.database != nil {
		_ = a.database.UpsertProject(p)
	}
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeProjectCreated,
			Source:    "project-manager",
			ProjectID: p.ID,
			Data: map[string]interface{}{
				"project_id": p.ID,
				"name":       p.Name,
			},
		})
	}
	return p, nil
}

func (a *Loom) ensureDefaultAgents(ctx context.Context, projectID string) error {
	return a.ensureOrgChart(ctx, projectID)
}

// ensureOrgChart creates an org chart for a project and fills all positions with agents
func (a *Loom) ensureOrgChart(ctx context.Context, projectID string) error {
	project, err := a.projectManager.GetProject(projectID)
	if err != nil {
		return err
	}

	// Create or get the org chart for this project
	chart, err := a.orgChartManager.CreateForProject(projectID, project.Name)
	if err != nil {
		return err
	}

	allowedRoles := a.allowedRoleSet()

	// Map existing agents to their roles
	existingByRole := map[string]string{} // role -> agentID
	for _, agent := range a.agentManager.ListAgentsByProject(project.ID) {
		role := agent.Role
		if role == "" {
			role = roleFromPersonaName(agent.PersonaName)
		}
		if role != "" {
			existingByRole[role] = agent.ID
		}
	}

	// Fill positions from existing agents first
	for i := range chart.Positions {
		pos := &chart.Positions[i]
		if len(allowedRoles) > 0 {
			if _, ok := allowedRoles[strings.ToLower(pos.RoleName)]; !ok {
				continue
			}
		}
		if agentID, ok := existingByRole[pos.RoleName]; ok {
			if !pos.HasAgent(agentID) && pos.CanAddAgent() {
				pos.AgentIDs = append(pos.AgentIDs, agentID)
			}
		}
	}

	// Create agents for ALL positions that are still vacant (agents start paused without a provider)
	for _, pos := range chart.Positions {
		if len(allowedRoles) > 0 {
			if _, ok := allowedRoles[strings.ToLower(pos.RoleName)]; !ok {
				continue
			}
		}
		if pos.IsFilled() {
			continue
		}

		// Check if persona exists
		_, err := a.personaManager.LoadPersona(pos.PersonaPath)
		if err != nil {
			continue // Skip if persona doesn't exist
		}

		agentName := formatAgentName(pos.RoleName, "Default")
		agent, err := a.CreateAgent(ctx, agentName, pos.PersonaPath, projectID, pos.RoleName)
		if err != nil {
			continue
		}

		// Assign agent to position in org chart
		_ = a.orgChartManager.AssignAgentToRole(projectID, pos.RoleName, agent.ID)
	}

	return nil
}

// CheckProjectReadiness validates git access and bead path availability for dispatch gating.
func (a *Loom) CheckProjectReadiness(ctx context.Context, projectID string) (bool, []string) {
	if projectID == "" {
		return true, nil
	}

	now := time.Now()
	a.readinessMu.Lock()
	if cached, ok := a.readinessCache[projectID]; ok {
		if now.Sub(cached.checkedAt) < readinessCacheTTL {
			issues := append([]string(nil), cached.issues...)
			ready := cached.ready
			a.readinessMu.Unlock()
			return ready, issues
		}
	}
	a.readinessMu.Unlock()

	project, err := a.projectManager.GetProject(projectID)
	if err != nil {
		return false, []string{err.Error()}
	}

	issues := []string{}
	publicKey := ""
	if project.GitRepo != "" && project.GitRepo != "." {
		if project.GitAuthMethod == "" {
			project.GitAuthMethod = normalizeGitAuthMethod(project.GitRepo, project.GitAuthMethod)
		}
		if project.GitAuthMethod == models.GitAuthSSH {
			key, err := a.gitopsManager.EnsureProjectSSHKey(project.ID)
			if err != nil {
				issues = append(issues, fmt.Sprintf("ssh key generation failed: %v", err))
			} else {
				publicKey = key
			}
			if !isSSHRepo(project.GitRepo) {
				issues = append(issues, "git repo is not using SSH (update git_repo to an SSH URL or set git_auth_method)")
			}
		}
		if err := a.gitopsManager.CheckRemoteAccess(ctx, project); err != nil {
			issues = append(issues, fmt.Sprintf("git remote access failed: %v", err))
		}
	}

	beadsPath := project.BeadsPath
	if project.GitRepo != "" && project.GitRepo != "." {
		beadsPath = filepath.Join(a.gitopsManager.GetProjectWorkDir(project.ID), project.BeadsPath)
	}
	if !beadsPathExists(beadsPath) {
		issues = append(issues, fmt.Sprintf("beads path missing: %s", beadsPath))
	}

	ready := len(issues) == 0
	a.readinessMu.Lock()
	a.readinessCache[projectID] = projectReadinessState{ready: ready, issues: issues, checkedAt: now}
	a.readinessMu.Unlock()

	if !ready {
		a.maybeFileReadinessBead(project, issues, publicKey)
	}

	return ready, issues
}

func (a *Loom) maybeFileReadinessBead(project *models.Project, issues []string, publicKey string) {
	if project == nil || len(issues) == 0 {
		return
	}
	issueKey := fmt.Sprintf("%s:%s", project.ID, strings.Join(issues, "|"))
	now := time.Now()
	a.readinessMu.Lock()
	if last, ok := a.readinessFailures[issueKey]; ok && now.Sub(last) < 30*time.Minute {
		a.readinessMu.Unlock()
		return
	}
	a.readinessFailures[issueKey] = now
	a.readinessMu.Unlock()

	description := fmt.Sprintf("Project readiness failed for %s.\n\nIssues:\n- %s", project.ID, strings.Join(issues, "\n- "))
	if publicKey != "" {
		description = fmt.Sprintf("%s\n\nProject SSH public key (register this with your git host):\n%s", description, publicKey)
	}

	bead, err := a.CreateBead(
		fmt.Sprintf("[auto-filed] P3 - Project readiness failed for %s", project.ID),
		description,
		models.BeadPriorityP3, // Readiness failures need human config (SSH keys), not agent work
		"bug",
		project.ID,
	)
	if err != nil {
		log.Printf("failed to auto-file readiness bead for %s: %v", project.ID, err)
		return
	}

	_ = a.beadsManager.UpdateBead(bead.ID, map[string]interface{}{
		"tags": []string{"auto-filed", "readiness", "p3"},
	})
}

func isSSHRepo(repo string) bool {
	repo = strings.TrimSpace(repo)
	return strings.HasPrefix(repo, "git@") || strings.HasPrefix(repo, "ssh://")
}

func (a *Loom) GetProjectGitPublicKey(projectID string) (string, error) {
	project, err := a.projectManager.GetProject(projectID)
	if err != nil {
		return "", err
	}
	if project.GitAuthMethod != models.GitAuthSSH {
		return "", fmt.Errorf("project %s is not configured for ssh auth", projectID)
	}
	return a.gitopsManager.GetProjectPublicKey(projectID)
}

func (a *Loom) RotateProjectGitKey(projectID string) (string, error) {
	project, err := a.projectManager.GetProject(projectID)
	if err != nil {
		return "", err
	}
	if project.GitAuthMethod != models.GitAuthSSH {
		return "", fmt.Errorf("project %s is not configured for ssh auth", projectID)
	}
	return a.gitopsManager.RotateProjectSSHKey(projectID)
}

func roleFromPersonaName(personaName string) string {
	personaName = strings.TrimSpace(personaName)
	if strings.HasPrefix(personaName, "default/") {
		return strings.TrimPrefix(personaName, "default/")
	}
	if strings.HasPrefix(personaName, "projects/") {
		parts := strings.Split(personaName, "/")
		if len(parts) >= 3 {
			return parts[2]
		}
	}
	if strings.Contains(personaName, "/") {
		parts := strings.Split(personaName, "/")
		return parts[len(parts)-1]
	}
	return personaName
}

// formatAgentName formats an agent name as "Role Name (Persona Type)" for better readability
func formatAgentName(roleName, personaType string) string {
	// Convert kebab-case to Title Case
	words := strings.Split(roleName, "-")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	titleRole := strings.Join(words, " ")
	// Capitalize acronyms like CEO, CFO
	titleRole = capitalizeAcronyms(titleRole)
	return fmt.Sprintf("%s (%s)", titleRole, personaType)
}

// capitalizeAcronyms capitalizes known acronyms like CEO, CFO
func capitalizeAcronyms(name string) string {
	// Only replace whole words (space-bounded or at start/end)
	words := strings.Split(name, " ")
	acronyms := map[string]string{
		"Ceo": "CEO",
		"Cfo": "CFO",
		"Qa":  "QA",
	}
	for i, word := range words {
		if replacement, ok := acronyms[word]; ok {
			words[i] = replacement
		}
	}
	return strings.Join(words, " ")
}

func normalizeBeadsPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		trimmed = ".beads"
	}

	// Check paths in order of priority
	candidates := []string{
		trimmed,
		// Relative path with dot prefix
		"." + strings.TrimPrefix(trimmed, "/"),
		// Fallback
		".beads",
	}

	for _, candidate := range candidates {
		if beadsPathExists(candidate) {
			return candidate
		}
	}
	return trimmed
}

func beadsPathExists(path string) bool {
	if path == "" {
		return false
	}
	issuesPath := filepath.Join(path, "issues.jsonl")
	if _, err := os.Stat(issuesPath); err == nil {
		return true
	}
	beadsDir := filepath.Join(path, "beads")
	if _, err := os.Stat(beadsDir); err == nil {
		return true
	}
	return false
}

func normalizeGitAuthMethod(repo string, method models.GitAuthMethod) models.GitAuthMethod {
	if method != "" {
		return method
	}
	if repo == "" || repo == "." {
		return models.GitAuthNone
	}
	return models.GitAuthSSH
}

func normalizeGitStrategy(strategy models.GitStrategy) models.GitStrategy {
	if strategy != "" {
		return strategy
	}
	return models.GitStrategyDirect
}

func (a *Loom) allowedRoleSet() map[string]struct{} {
	roles := a.config.Agents.AllowedRoles
	if len(roles) == 0 {
		roles = rolesForProfile(a.config.Agents.CorpProfile)
	}
	if len(roles) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(strings.ToLower(role))
		if role == "" {
			continue
		}
		set[role] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func rolesForProfile(profile string) []string {
	profile = strings.TrimSpace(strings.ToLower(profile))
	switch profile {
	case "startup":
		return []string{"ceo", "engineering-manager", "web-designer"}
	case "solo":
		return []string{"ceo", "engineering-manager"}
	case "full", "enterprise", "":
		return nil
	default:
		return nil
	}
}

// CloneAgentPersona clones a default persona into a project-specific persona and spawns a new agent.
func (a *Loom) CloneAgentPersona(ctx context.Context, agentID, newPersonaName, newAgentName, sourcePersona string, replace bool) (*models.Agent, error) {
	agent, err := a.agentManager.GetAgent(agentID)
	if err != nil {
		return nil, err
	}
	if newPersonaName == "" {
		return nil, errors.New("new persona name is required")
	}

	roleName := ""
	if strings.HasPrefix(agent.PersonaName, "default/") {
		roleName = strings.TrimPrefix(agent.PersonaName, "default/")
	}
	if roleName == "" {
		roleName = path.Base(agent.PersonaName)
	}

	if sourcePersona == "" {
		sourcePersona = fmt.Sprintf("default/%s", roleName)
	}

	clonedPersona := fmt.Sprintf("projects/%s/%s/%s", agent.ProjectID, roleName, newPersonaName)
	_, err = a.personaManager.ClonePersona(sourcePersona, clonedPersona)
	if err != nil {
		return nil, err
	}

	if newAgentName == "" {
		newAgentName = fmt.Sprintf("%s-%s", roleName, newPersonaName)
	}
	newAgent, err := a.SpawnAgent(ctx, newAgentName, clonedPersona, agent.ProjectID, agent.ProviderID)
	if err != nil {
		return nil, err
	}

	if replace {
		_ = a.StopAgent(ctx, agent.ID)
	}

	return newAgent, nil
}

// AssignAgentToProject assigns an existing agent to a project.
func (a *Loom) AssignAgentToProject(agentID, projectID string) error {
	agent, err := a.agentManager.GetAgent(agentID)
	if err != nil {
		return err
	}
	if _, err := a.projectManager.GetProject(projectID); err != nil {
		return err
	}

	if agent.ProjectID != "" && agent.ProjectID != projectID {
		_ = a.projectManager.RemoveAgentFromProject(agent.ProjectID, agentID)
		a.PersistProject(agent.ProjectID)
	}

	if err := a.agentManager.UpdateAgentProject(agentID, projectID); err != nil {
		return err
	}
	_ = a.projectManager.AddAgentToProject(projectID, agentID)
	a.PersistProject(projectID)

	return nil
}

// UnassignAgentFromProject removes an agent from the project without deleting the agent.
func (a *Loom) UnassignAgentFromProject(agentID, projectID string) error {
	if _, err := a.projectManager.GetProject(projectID); err != nil {
		return err
	}
	if err := a.projectManager.RemoveAgentFromProject(projectID, agentID); err != nil {
		return err
	}
	_ = a.agentManager.UpdateAgentProject(agentID, "")
	a.PersistProject(projectID)
	return nil
}

func (a *Loom) PersistProject(projectID string) {
	if a.database == nil {
		return
	}
	p, err := a.projectManager.GetProject(projectID)
	if err != nil {
		return
	}
	_ = a.database.UpsertProject(p)
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeProjectUpdated,
			Source:    "project-manager",
			ProjectID: p.ID,
			Data: map[string]interface{}{
				"project_id": p.ID,
				"name":       p.Name,
			},
		})
	}
}

func (a *Loom) DeleteProject(projectID string) error {
	if err := a.projectManager.DeleteProject(projectID); err != nil {
		return err
	}
	if a.database != nil {
		_ = a.database.DeleteProject(projectID)
	}
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeProjectDeleted,
			Source:    "project-manager",
			ProjectID: projectID,
			Data: map[string]interface{}{
				"project_id": projectID,
			},
		})
	}
	return nil
}

// SpawnAgent spawns a new agent with a given persona
// CreateAgent creates an agent without requiring a provider (agent will be "paused" until provider available)
func (a *Loom) CreateAgent(ctx context.Context, name, personaName, projectID, role string) (*models.Agent, error) {
	// Load persona
	persona, err := a.personaManager.LoadPersona(personaName)
	if err != nil {
		return nil, fmt.Errorf("failed to load persona: %w", err)
	}

	// Verify project exists
	if _, err := a.projectManager.GetProject(projectID); err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	// Create agent record without a worker
	agent, err := a.agentManager.CreateAgent(ctx, name, personaName, projectID, role, persona)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent: %w", err)
	}

	// Add agent to project
	if err := a.projectManager.AddAgentToProject(projectID, agent.ID); err != nil {
		return nil, fmt.Errorf("failed to add agent to project: %w", err)
	}

	// Persist agent to the configuration database
	if a.database != nil {
		if err := a.database.UpsertAgent(agent); err != nil {
			log.Printf("Warning: Failed to persist agent %s to database: %v", agent.ID, err)
		} else {
			log.Printf("Persisted agent %s (%s) to database with status: %s", agent.ID, agent.Name, agent.Status)
		}
	}

	return agent, nil
}

func (a *Loom) SpawnAgent(ctx context.Context, name, personaName, projectID string, providerID string) (*models.Agent, error) {
	// Load persona
	persona, err := a.personaManager.LoadPersona(personaName)
	if err != nil {
		return nil, fmt.Errorf("failed to load persona: %w", err)
	}

	// Verify project exists
	if _, err := a.projectManager.GetProject(projectID); err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	// If no provider specified, pick the first registered provider.
	if providerID == "" {
		providers := a.providerRegistry.ListActive()
		if len(providers) == 0 {
			return nil, fmt.Errorf("no active providers registered")
		}
		providerID = providers[0].Config.ID
	}

	// Spawn agent + worker
	agent, err := a.agentManager.SpawnAgentWorker(ctx, name, personaName, projectID, providerID, persona)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn agent: %w", err)
	}

	// Add agent to project
	if err := a.projectManager.AddAgentToProject(projectID, agent.ID); err != nil {
		return nil, fmt.Errorf("failed to add agent to project: %w", err)
	}

	// Start Temporal workflow for agent if Temporal is enabled
	if a.temporalManager != nil {
		if err := a.temporalManager.StartAgentWorkflow(ctx, agent.ID, projectID, personaName, name); err != nil {
			// Log error but don't fail agent creation
			fmt.Printf("Warning: failed to start agent workflow: %v\n", err)
		}
	}

	// Persist agent assignment to the configuration database.
	if a.database != nil {
		_ = a.database.UpsertAgent(agent)
	}

	return agent, nil
}

// StopAgent stops an agent and removes it from the configuration database.
func (a *Loom) StopAgent(ctx context.Context, agentID string) error {
	ag, err := a.agentManager.GetAgent(agentID)
	if err != nil {
		return err
	}

	if err := a.agentManager.StopAgent(agentID); err != nil {
		return err
	}
	_ = a.fileLockManager.ReleaseAgentLocks(agentID)
	_ = a.projectManager.RemoveAgentFromProject(ag.ProjectID, ag.ID)
	_ = a.orgChartManager.RemoveAgentFromAll(ag.ProjectID, agentID)
	a.PersistProject(ag.ProjectID)
	if a.database != nil {
		_ = a.database.DeleteAgent(agentID)
	}
	if a.temporalManager != nil {
		_ = a.temporalManager.SignalAgentWorkflow(ctx, agentID, "shutdown", "stopped")
	}
	return nil
}

// Provider management

func (a *Loom) ListProviders() ([]*internalmodels.Provider, error) {
	if a.database == nil {
		return []*internalmodels.Provider{}, nil
	}
	return a.database.ListProviders()
}

func (a *Loom) RegisterProvider(ctx context.Context, p *internalmodels.Provider, apiKeys ...string) (*internalmodels.Provider, error) {
	log.Printf("RegisterProvider called for: %s (type: %s, endpoint: %s)", p.ID, p.Type, p.Endpoint)
	if a.database == nil {
		return nil, fmt.Errorf("database not configured")
	}
	if p.ID == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	if p.Type == "" {
		p.Type = "local"
	}
	if p.Status == "" {
		p.Status = "pending"
	}
	// Endpoint is bootstrapped via heartbeats (port/protocol discovery), but keep the existing
	// OpenAI default normalization for compatibility.
	if p.Type != "ollama" {
		p.Endpoint = normalizeProviderEndpoint(p.Endpoint)
	}
	p.LastHeartbeatError = ""
	if p.ConfiguredModel == "" {
		p.ConfiguredModel = p.Model
	}
	if p.ConfiguredModel == "" {
		p.ConfiguredModel = "nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-FP8"
	}
	if p.SelectedModel == "" {
		p.SelectedModel = p.ConfiguredModel
	}
	p.Model = p.SelectedModel

	if err := a.database.UpsertProvider(p); err != nil {
		return nil, err
	}

	// Pass API key to the registry so the Protocol gets authentication
	regAPIKey := ""
	if len(apiKeys) > 0 {
		regAPIKey = apiKeys[0]
	}
	_ = a.providerRegistry.Upsert(&provider.ProviderConfig{
		ID:                     p.ID,
		Name:                   p.Name,
		Type:                   p.Type,
		Endpoint:               p.Endpoint,
		APIKey:                 regAPIKey,
		Model:                  p.SelectedModel,
		ConfiguredModel:        p.ConfiguredModel,
		SelectedModel:          p.SelectedModel,
		SelectedGPU:            p.SelectedGPU,
		Status:                 p.Status,
		LastHeartbeatAt:        p.LastHeartbeatAt,
		LastHeartbeatLatencyMs: p.LastHeartbeatLatencyMs,
	})
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:   eventbus.EventTypeProviderRegistered,
			Source: "provider-manager",
			Data: map[string]interface{}{
				"provider_id": p.ID,
				"name":        p.Name,
				"endpoint":    p.Endpoint,
				"model":       p.SelectedModel,
				"configured":  p.ConfiguredModel,
			},
		})
	}
	_ = a.ensureProviderHeartbeat(ctx, p.ID)

	// Immediately attempt to get models from the provider to validate and update status
	log.Printf("Launching health check goroutine for provider: %s", p.ID)
	go a.checkProviderHealthAndActivate(p.ID)

	return p, nil
}

func (a *Loom) UpdateProvider(ctx context.Context, p *internalmodels.Provider) (*internalmodels.Provider, error) {
	if a.database == nil {
		return nil, fmt.Errorf("database not configured")
	}
	if p == nil {
		return nil, fmt.Errorf("provider cannot be nil")
	}
	if p.ID == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	if p.Type == "" {
		p.Type = "local"
	}
	if p.Status == "" {
		p.Status = "pending"
	}
	if p.Type != "ollama" {
		p.Endpoint = normalizeProviderEndpoint(p.Endpoint)
	}
	// If the operator edits a provider, we treat it as needing re-validation.
	p.LastHeartbeatError = ""
	if p.ConfiguredModel == "" {
		p.ConfiguredModel = p.Model
	}
	if p.ConfiguredModel == "" {
		p.ConfiguredModel = "nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-FP8"
	}
	if p.SelectedModel == "" {
		p.SelectedModel = p.ConfiguredModel
	}
	p.Model = p.SelectedModel

	if err := a.database.UpsertProvider(p); err != nil {
		return nil, err
	}

	_ = a.providerRegistry.Upsert(&provider.ProviderConfig{
		ID:                     p.ID,
		Name:                   p.Name,
		Type:                   p.Type,
		Endpoint:               p.Endpoint,
		Model:                  p.SelectedModel,
		ConfiguredModel:        p.ConfiguredModel,
		SelectedModel:          p.SelectedModel,
		SelectedGPU:            p.SelectedGPU,
		Status:                 p.Status,
		LastHeartbeatAt:        p.LastHeartbeatAt,
		LastHeartbeatLatencyMs: p.LastHeartbeatLatencyMs,
	})
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:   eventbus.EventTypeProviderUpdated,
			Source: "provider-manager",
			Data: map[string]interface{}{
				"provider_id": p.ID,
				"name":        p.Name,
				"endpoint":    p.Endpoint,
				"model":       p.SelectedModel,
				"configured":  p.ConfiguredModel,
			},
		})
	}
	_ = a.ensureProviderHeartbeat(ctx, p.ID)

	return p, nil
}

func (a *Loom) DeleteProvider(ctx context.Context, providerID string) error {
	if a.database == nil {
		return fmt.Errorf("database not configured")
	}
	_ = a.providerRegistry.Unregister(providerID)
	err := a.database.DeleteProvider(providerID)
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:   eventbus.EventTypeProviderDeleted,
			Source: "provider-manager",
			Data: map[string]interface{}{
				"provider_id": providerID,
			},
		})
	}
	return err
}

func (a *Loom) GetProviderModels(ctx context.Context, providerID string) ([]provider.Model, error) {
	return a.providerRegistry.GetModels(ctx, providerID)
}

// ReplResult represents a CEO REPL response.
type ReplResult struct {
	BeadID       string `json:"bead_id"`
	ProviderID   string `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	Model        string `json:"model"`
	Response     string `json:"response"`
	TokensUsed   int    `json:"tokens_used"`
	LatencyMs    int64  `json:"latency_ms"`
}

// RunReplQuery sends a high-priority query through Temporal using the best provider.
// All CEO queries automatically create P0 beads to preserve state.
func (a *Loom) RunReplQuery(ctx context.Context, message string) (*ReplResult, error) {
	if strings.TrimSpace(message) == "" {
		return nil, fmt.Errorf("message is required")
	}
	if a.temporalManager == nil {
		return nil, fmt.Errorf("temporal manager not configured")
	}
	if a.database == nil {
		return nil, fmt.Errorf("database not configured")
	}

	// Extract persona hint and clean message if "persona: message" format is used
	personaHint, cleanMessage := extractPersonaFromMessage(message)

	// Create a P0 bead for this CEO query
	beadTitle := "CEO Query"
	if personaHint != "" {
		beadTitle = fmt.Sprintf("CEO Query for %s", personaHint)
	}

	// Truncate message for title if it's short
	if len(cleanMessage) < 80 {
		beadTitle = fmt.Sprintf("CEO: %s", cleanMessage)
	}

	bead, err := a.beadsManager.CreateBead(beadTitle, cleanMessage, models.BeadPriorityP0, "task", "loom-self")
	if err != nil {
		// If bead creation fails, continue anyway but log it
		log.Printf("Warning: Failed to create CEO query bead: %v", err)
	}

	var beadID string
	if bead != nil {
		beadID = bead.ID

		// If persona hint was provided, add it to the bead description
		if personaHint != "" {
			updatedDesc := fmt.Sprintf("Persona: %s\n\n%s", personaHint, cleanMessage)
			_ = a.beadsManager.UpdateBead(beadID, map[string]interface{}{
				"description": updatedDesc,
			})
		}

		// Add CEO context
		_ = a.beadsManager.UpdateBead(beadID, map[string]interface{}{
			"context": map[string]string{
				"source":     "ceo-repl",
				"created_by": "ceo",
			},
		})
	}

	providerRecord, err := a.selectBestProviderForRepl()
	if err != nil {
		return nil, err
	}

	systemPrompt := a.buildLoomPersonaPrompt()
	input := workflows.ProviderQueryWorkflowInput{
		ProviderID:   providerRecord.ID,
		SystemPrompt: systemPrompt,
		Message:      cleanMessage,
		Temperature:  0.2,
		MaxTokens:    1200,
	}

	result, err := a.temporalManager.RunProviderQueryWorkflow(ctx, input)
	if err != nil {
		// Update bead with error if it was created
		if beadID != "" {
			_ = a.beadsManager.UpdateBead(beadID, map[string]interface{}{
				"context": map[string]string{
					"source":     "ceo-repl",
					"created_by": "ceo",
					"error":      err.Error(),
				},
			})
		}
		return nil, err
	}

	// Enforce strict JSON action output and execute actions
	var actionResults []actions.Result
	if a.actionRouter != nil {
		actx := actions.ActionContext{
			AgentID:   "ceo",
			BeadID:    beadID,
			ProjectID: "loom-self",
		}
		env, parseErr := actions.DecodeLenient([]byte(result.Response))
		if parseErr != nil {
			actionResult := a.actionRouter.AutoFileParseFailure(ctx, actx, parseErr, result.Response)
			actionResults = []actions.Result{actionResult}
		} else {
			actionResults, _ = a.actionRouter.Execute(ctx, env, actx)
		}
	}

	// Update bead with response
	if beadID != "" {
		actionsJSON, _ := json.Marshal(actionResults)
		_ = a.beadsManager.UpdateBead(beadID, map[string]interface{}{
			"context": map[string]string{
				"source":      "ceo-repl",
				"created_by":  "ceo",
				"response":    result.Response,
				"actions":     string(actionsJSON),
				"provider_id": providerRecord.ID,
				"model":       result.Model,
				"tokens_used": fmt.Sprintf("%d", result.TokensUsed),
			},
			"status": models.BeadStatusClosed,
		})
	}

	model := result.Model
	if model == "" {
		model = providerRecord.SelectedModel
	}
	if model == "" {
		model = providerRecord.Model
	}
	return &ReplResult{
		BeadID:       beadID,
		ProviderID:   providerRecord.ID,
		ProviderName: providerRecord.Name,
		Model:        model,
		Response:     result.Response,
		TokensUsed:   result.TokensUsed,
		LatencyMs:    result.LatencyMs,
	}, nil
}

// extractPersonaFromMessage extracts persona hint from "persona: message" format
// Returns (personaHint, cleanMessage)
func extractPersonaFromMessage(message string) (string, string) {
	message = strings.TrimSpace(message)

	// Check for "persona: rest of message" format
	if idx := strings.Index(message, ":"); idx > 0 && idx < 50 {
		potentialPersona := strings.TrimSpace(message[:idx])
		// Check if it looks like a persona (single word or hyphenated, lowercase)
		if isLikelyPersona(potentialPersona) {
			restOfMessage := strings.TrimSpace(message[idx+1:])
			return potentialPersona, restOfMessage
		}
	}

	return "", message
}

func isLikelyPersona(s string) bool {
	s = strings.ToLower(s)
	// Must be 3-40 characters, contain only letters, hyphens, and spaces
	if len(s) < 3 || len(s) > 40 {
		return false
	}
	for _, ch := range s {
		if !((ch >= 'a' && ch <= 'z') || ch == '-' || ch == ' ') {
			return false
		}
	}
	// Can't start or end with hyphen/space
	if s[0] == '-' || s[0] == ' ' || s[len(s)-1] == '-' || s[len(s)-1] == ' ' {
		return false
	}
	return true
}

func (a *Loom) selectBestProviderForRepl() (*internalmodels.Provider, error) {
	providers, err := a.database.ListProviders()
	if err != nil {
		return nil, err
	}

	// Filter for chat-capable models (Instruct, Chat, claude, gpt, etc.)
	// Exclude completion-only models like StarCoder, CodeGen, etc.
	chatCapable := make([]*internalmodels.Provider, 0)
	for _, p := range providers {
		if p == nil || p.Status != "healthy" {
			continue
		}
		if isChatCapableModel(p.SelectedModel) || isChatCapableModel(p.Model) {
			chatCapable = append(chatCapable, p)
		}
	}

	if len(chatCapable) == 0 {
		return nil, fmt.Errorf("no chat-capable providers available (need Instruct/Chat model)")
	}

	router := routing.NewRouter(routing.PolicyBalanced)
	return router.SelectProvider(context.Background(), chatCapable, nil)
}

// isChatCapableModel checks if a model name indicates chat/instruct capability
func isChatCapableModel(modelName string) bool {
	if modelName == "" {
		return false
	}
	lower := strings.ToLower(modelName)

	// Models that ARE chat-capable
	chatPatterns := []string{
		"instruct",
		"chat",
		"claude",
		"gpt-",
		"gpt4",
		"opus",
		"sonnet",
		"haiku",
		"llama-3",    // Llama 3 has chat templates
		"qwen",       // Qwen models generally have chat templates
		"mistral",    // Mistral instruct models
		"deepseek",   // DeepSeek chat models
		"gemma",      // Gemma instruct
		"phi-",       // Phi models with chat
		"nemotron",   // NVIDIA Nemotron
	}
	for _, pattern := range chatPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	// Models that are NOT chat-capable (completion only)
	completionPatterns := []string{
		"starcoder",
		"codegen",
		"santacoder",
		"codellama-7b",  // Base CodeLlama without instruct
		"codellama-13b", // Base CodeLlama without instruct
		"codellama-34b", // Base CodeLlama without instruct
		"-base",
		"-raw",
	}
	for _, pattern := range completionPatterns {
		if strings.Contains(lower, pattern) {
			return false
		}
	}

	// Default: assume chat-capable if not explicitly a completion model
	return true
}

// SelectProvider chooses the best provider based on policy and requirements
func (a *Loom) SelectProvider(ctx context.Context, requirements *routing.ProviderRequirements, policy string) (*internalmodels.Provider, error) {
	providers, err := a.database.ListProviders()
	if err != nil {
		return nil, err
	}

	// Default policy
	routingPolicy := routing.PolicyBalanced
	if policy != "" {
		routingPolicy = routing.RoutingPolicy(policy)
	}

	router := routing.NewRouter(routingPolicy)
	return router.SelectProvider(ctx, providers, requirements)
}

func (a *Loom) buildLoomPersonaPrompt() string {
	persona, err := a.personaManager.LoadPersona("loom")
	if err != nil {
		return fmt.Sprintf("You are Loom, the orchestration system. Respond to the CEO with clear guidance and actionable next steps.\n\n%s", actions.ActionPrompt)
	}

	focus := strings.Join(persona.FocusAreas, ", ")
	standards := strings.Join(persona.Standards, "; ")

	return fmt.Sprintf(
		"You are Loom, the orchestration system. Treat this as a high-priority CEO request.\n\nMission: %s\nCharacter: %s\nTone: %s\nFocus Areas: %s\nDecision Making: %s\nStandards: %s\n\n%s",
		strings.TrimSpace(persona.Mission),
		strings.TrimSpace(persona.Character),
		strings.TrimSpace(persona.Tone),
		strings.TrimSpace(focus),
		strings.TrimSpace(persona.DecisionMaking),
		strings.TrimSpace(standards),
		actions.ActionPrompt,
	)
}

func (a *Loom) startProviderHeartbeats(ctx context.Context) error {
	if a.temporalManager == nil || a.database == nil {
		return nil
	}
	providers, err := a.database.ListProviders()
	if err != nil {
		return err
	}
	for _, p := range providers {
		if p == nil || p.ID == "" {
			continue
		}
		_ = a.ensureProviderHeartbeat(ctx, p.ID)
	}
	return nil
}

func (a *Loom) ensureProviderHeartbeat(ctx context.Context, providerID string) error {
	if a.temporalManager == nil || providerID == "" {
		return nil
	}
	return a.temporalManager.StartProviderHeartbeatWorkflow(ctx, providerID, 30*time.Second)
}

// NegotiateProviderModel selects the best available model from the catalog for a provider.
func (a *Loom) NegotiateProviderModel(ctx context.Context, providerID string) (*internalmodels.Provider, error) {
	if a.database == nil {
		return nil, fmt.Errorf("database not configured")
	}
	if providerID == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	providerRecord, err := a.database.GetProvider(providerID)
	if err != nil {
		return nil, err
	}

	models, err := a.providerRegistry.GetModels(ctx, providerID)
	if err != nil {
		providerRecord.SelectionReason = fmt.Sprintf("failed to load models: %s", err.Error())
		providerRecord.SelectedModel = providerRecord.ConfiguredModel
		providerRecord.Model = providerRecord.SelectedModel
		_ = a.database.UpsertProvider(providerRecord)
		return providerRecord, err
	}
	available := make([]string, 0, len(models))
	for _, m := range models {
		if m.ID != "" {
			available = append(available, m.ID)
		}
	}

	if providerRecord.ConfiguredModel == "" {
		providerRecord.ConfiguredModel = providerRecord.Model
	}
	if providerRecord.ConfiguredModel == "" {
		providerRecord.ConfiguredModel = "nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-FP8"
	}

	if providerRecord.ConfiguredModel != "" {
		for _, modelName := range available {
			if strings.EqualFold(modelName, providerRecord.ConfiguredModel) {
				providerRecord.SelectedModel = providerRecord.ConfiguredModel
				providerRecord.SelectionReason = "configured model available"
				providerRecord.ModelScore = 0
				break
			}
		}
	}

	if providerRecord.SelectedModel == "" && a.modelCatalog != nil {
		if best, score, ok := a.modelCatalog.SelectBest(available); ok {
			providerRecord.SelectedModel = best.Name
			providerRecord.SelectionReason = "matched recommended catalog"
			providerRecord.ModelScore = score
		}
	}

	if providerRecord.SelectedModel == "" {
		providerRecord.SelectedModel = providerRecord.ConfiguredModel
		providerRecord.SelectionReason = "fallback to configured model"
	}

	providerRecord.Model = providerRecord.SelectedModel

	if err := a.database.UpsertProvider(providerRecord); err != nil {
		return nil, err
	}
	_ = a.providerRegistry.Upsert(&provider.ProviderConfig{
		ID:              providerRecord.ID,
		Name:            providerRecord.Name,
		Type:            providerRecord.Type,
		Endpoint:        providerRecord.Endpoint,
		Model:           providerRecord.SelectedModel,
		ConfiguredModel: providerRecord.ConfiguredModel,
		SelectedModel:   providerRecord.SelectedModel,
		SelectedGPU:     providerRecord.SelectedGPU,
		Status:          "active",
	})
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:   eventbus.EventTypeProviderUpdated,
			Source: "provider-manager",
			Data: map[string]interface{}{
				"provider_id": providerRecord.ID,
				"name":        providerRecord.Name,
				"endpoint":    providerRecord.Endpoint,
				"model":       providerRecord.SelectedModel,
				"configured":  providerRecord.ConfiguredModel,
				"score":       providerRecord.ModelScore,
			},
		})
	}

	return providerRecord, nil
}

// ListModelCatalog returns the recommended model catalog.
func (a *Loom) ListModelCatalog() []internalmodels.ModelSpec {
	if a.modelCatalog == nil {
		return nil
	}
	return a.modelCatalog.List()
}

func normalizeProviderEndpoint(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	// vLLM is typically OpenAI-compatible at /v1.
	if len(endpoint) >= 3 && endpoint[len(endpoint)-3:] == "/v1" {
		return endpoint
	}
	return fmt.Sprintf("%s/v1", strings.TrimSuffix(endpoint, "/"))
}

// RequestFileAccess handles file lock requests from agents
func (a *Loom) RequestFileAccess(projectID, filePath, agentID, beadID string) (*models.FileLock, error) {
	// Verify agent exists
	if _, err := a.agentManager.GetAgent(agentID); err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	// Verify project exists
	if _, err := a.projectManager.GetProject(projectID); err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	// Acquire lock
	lock, err := a.fileLockManager.AcquireLock(projectID, filePath, agentID, beadID)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	return lock, nil
}

// ReleaseFileAccess releases a file lock
func (a *Loom) ReleaseFileAccess(projectID, filePath, agentID string) error {
	return a.fileLockManager.ReleaseLock(projectID, filePath, agentID)
}

// CreateBead creates a new work bead
func (a *Loom) CreateBead(title, description string, priority models.BeadPriority, beadType, projectID string) (*models.Bead, error) {
	// Verify project exists
	if _, err := a.projectManager.GetProject(projectID); err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	bead, err := a.beadsManager.CreateBead(title, description, priority, beadType, projectID)
	if err != nil {
		return nil, err
	}

	if a.eventBus != nil {
		_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadCreated, bead.ID, projectID, map[string]interface{}{
			"title":    title,
			"type":     beadType,
			"priority": priority,
		})
	}

	// Start Temporal workflow for bead if Temporal is enabled
	if a.temporalManager != nil {
		ctx := context.Background()
		if err := a.temporalManager.StartBeadWorkflow(ctx, bead.ID, projectID, title, description, int(priority), beadType); err != nil {
			// Log error but don't fail bead creation
			fmt.Printf("Warning: failed to start bead workflow: %v\n", err)
		}

	}

	return bead, nil
}

// CloseBead closes a bead with an optional reason
func (a *Loom) CloseBead(beadID, reason string) error {
	bead, err := a.beadsManager.GetBead(beadID)
	if err != nil {
		return fmt.Errorf("bead not found: %w", err)
	}

	updates := map[string]interface{}{
		"status": models.BeadStatusClosed,
	}
	if reason != "" {
		ctx := bead.Context
		if ctx == nil {
			ctx = make(map[string]string)
		}
		ctx["close_reason"] = reason
		updates["context"] = ctx
	}

	if err := a.beadsManager.UpdateBead(beadID, updates); err != nil {
		return fmt.Errorf("failed to close bead: %w", err)
	}

	if a.eventBus != nil {
		_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadStatusChange, beadID, bead.ProjectID, map[string]interface{}{
			"status": string(models.BeadStatusClosed),
			"reason": reason,
		})
	}

	// Auto-create apply-fix bead if this was an approved code fix proposal
	if strings.Contains(strings.ToLower(bead.Title), "code fix approval") &&
		bead.Type == "decision" &&
		strings.Contains(strings.ToLower(reason), "approve") {

		if err := a.createApplyFixBead(bead, reason); err != nil {
			log.Printf("[AutoFix] Failed to create apply-fix bead for %s: %v", beadID, err)
			// Don't fail the close operation if apply-fix creation fails
		}
	}

	return nil
}

// createApplyFixBead automatically creates an apply-fix task when a code fix proposal is approved
func (a *Loom) createApplyFixBead(approvalBead *models.Bead, closeReason string) error {
	// Extract original bug ID from approval bead description
	originalBugID := extractOriginalBugID(approvalBead.Description)
	if originalBugID == "" {
		return fmt.Errorf("could not extract original bug ID from approval bead")
	}

	// Get the agent who created the proposal (from context or assigned_to)
	agentID := ""
	if approvalBead.Context != nil {
		agentID = approvalBead.Context["agent_id"]
	}
	if agentID == "" && approvalBead.AssignedTo != "" {
		agentID = approvalBead.AssignedTo
	}

	projectID := approvalBead.ProjectID
	if projectID == "" {
		projectID = "loom-self"
	}

	// Create apply-fix bead
	title := fmt.Sprintf("[apply-fix] Apply approved patch from %s", approvalBead.ID)

	description := fmt.Sprintf(`## Apply Approved Code Fix

**Approval Bead:** %s
**Original Bug:** %s
**Approved By:** CEO
**Approved At:** %s
**Approval Reason:** %s

### Instructions

1. Read the approved fix proposal from bead %s
2. Extract the patch or code changes from the proposal
3. Apply the changes using write_file or apply_patch action
4. Verify the fix (compile/test if applicable)
5. Update cache versions if needed (for frontend changes)
6. Close this bead and the original bug bead %s
7. Add comment to bug bead: "Fixed by applying approved patch from %s"

### Approved Proposal

%s

### Important Notes

- This fix has been reviewed and approved by the CEO
- Apply the changes exactly as specified in the proposal
- Test thoroughly after applying
- Report any issues or unexpected errors immediately
- If hot-reload is enabled, verify the fix works after automatic browser refresh
`,
		approvalBead.ID,
		originalBugID,
		time.Now().Format(time.RFC3339),
		closeReason,
		approvalBead.ID,
		originalBugID,
		approvalBead.ID,
		approvalBead.Description,
	)

	// Create the bead
	bead, err := a.CreateBead(title, description, models.BeadPriority(1), "task", projectID)
	if err != nil {
		return fmt.Errorf("failed to create apply-fix bead: %w", err)
	}

	// Update with tags, assignment, and context
	tags := []string{"apply-fix", "auto-created", "code-fix"}
	ctx := map[string]string{
		"approval_bead_id": approvalBead.ID,
		"original_bug_id":  originalBugID,
		"fix_type":         "code-fix",
		"created_by":       "auto_fix_system",
	}

	updates := map[string]interface{}{
		"tags":    tags,
		"context": ctx,
	}

	// Assign to the agent who created the proposal, if available
	if agentID != "" {
		updates["assigned_to"] = agentID
	}

	if err := a.beadsManager.UpdateBead(bead.ID, updates); err != nil {
		log.Printf("[AutoFix] Failed to update apply-fix bead %s: %v", bead.ID, err)
		// Don't fail - bead is created, just missing some metadata
	}

	log.Printf("[AutoFix] Created apply-fix bead %s for approved proposal %s (original bug: %s)",
		bead.ID, approvalBead.ID, originalBugID)

	return nil
}

// extractOriginalBugID extracts the original bug bead ID from an approval bead description
func extractOriginalBugID(description string) string {
	// Look for patterns like "**Original Bug:** ac-001" or "Original Bug: bd-123"
	patterns := []string{
		"**Original Bug:** ",
		"Original Bug: ",
		"**Original Bug:**",
	}

	for _, pattern := range patterns {
		idx := strings.Index(description, pattern)
		if idx >= 0 {
			// Extract the bead ID after the pattern
			start := idx + len(pattern)
			end := start
			for end < len(description) && ((description[end] >= 'a' && description[end] <= 'z') ||
				(description[end] >= '0' && description[end] <= '9') ||
				description[end] == '-') {
				end++
			}
			if end > start {
				bugID := strings.TrimSpace(description[start:end])
				return bugID
			}
		}
	}

	return ""
}

// CreateDecisionBead creates a decision bead when an agent needs a decision
func (a *Loom) CreateDecisionBead(question, parentBeadID, requesterID string, options []string, recommendation string, priority models.BeadPriority, projectID string) (*models.DecisionBead, error) {
	// Verify requester exists (agent or user/system)
	if requesterID != "system" && !strings.HasPrefix(requesterID, "user-") {
		if _, err := a.agentManager.GetAgent(requesterID); err != nil {
			return nil, fmt.Errorf("requester agent not found: %w", err)
		}
	}

	// Create decision
	decision, err := a.decisionManager.CreateDecision(question, parentBeadID, requesterID, options, recommendation, priority, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create decision: %w", err)
	}

	// Block parent bead on this decision
	if parentBeadID != "" {
		if err := a.beadsManager.AddDependency(parentBeadID, decision.ID, "blocks"); err != nil {
			return nil, fmt.Errorf("failed to add blocking dependency: %w", err)
		}
	}

	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeDecisionCreated,
			Source:    "decision-manager",
			ProjectID: projectID,
			Data: map[string]interface{}{
				"decision_id":  decision.ID,
				"question":     question,
				"requester_id": requesterID,
			},
		})
	}

	// Start Temporal decision workflow if Temporal is enabled
	if a.temporalManager != nil {
		ctx := context.Background()
		if err := a.temporalManager.StartDecisionWorkflow(ctx, decision.ID, projectID, question, requesterID, options); err != nil {
			// Log error but don't fail decision creation
			fmt.Printf("Warning: failed to start decision workflow: %v\n", err)
		}
	}

	return decision, nil
}

// MakeDecision resolves a decision bead
func (a *Loom) MakeDecision(decisionID, deciderID, decisionText, rationale string) error {
	// Verify decider exists (could be agent or user)
	// For users, we'll allow any decider ID starting with "user-"
	if !strings.HasPrefix(deciderID, "user-") {
		if _, err := a.agentManager.GetAgent(deciderID); err != nil {
			return fmt.Errorf("decider not found: %w", err)
		}
	}

	// Make decision
	if err := a.decisionManager.MakeDecision(decisionID, deciderID, decisionText, rationale); err != nil {
		return fmt.Errorf("failed to make decision: %w", err)
	}

	// Unblock dependent beads
	if err := a.UnblockDependents(decisionID); err != nil {
		return fmt.Errorf("failed to unblock dependents: %w", err)
	}

	if a.eventBus != nil {
		if d, err := a.decisionManager.GetDecision(decisionID); err == nil && d != nil {
			_ = a.eventBus.Publish(&eventbus.Event{
				Type:      eventbus.EventTypeDecisionResolved,
				Source:    "decision-manager",
				ProjectID: d.ProjectID,
				Data: map[string]interface{}{
					"decision_id": decisionID,
					"decision":    decisionText,
					"decider_id":  deciderID,
				},
			})
		}
	}

	_ = a.applyCEODecisionToParent(decisionID)

	return nil
}

func (a *Loom) EscalateBeadToCEO(beadID, reason, returnedTo string) (*models.DecisionBead, error) {
	b, err := a.beadsManager.GetBead(beadID)
	if err != nil {
		return nil, fmt.Errorf("bead not found: %w", err)
	}
	if returnedTo == "" {
		returnedTo = b.AssignedTo
	}

	question := fmt.Sprintf("CEO decision required for bead %s (%s).\n\nReason: %s\n\nChoose: approve | deny | needs_more_info", b.ID, b.Title, reason)
	decision, err := a.decisionManager.CreateDecision(question, beadID, "system", []string{"approve", "deny", "needs_more_info"}, "", models.BeadPriorityP0, b.ProjectID)
	if err != nil {
		return nil, err
	}
	if decision.Context == nil {
		decision.Context = make(map[string]string)
	}
	decision.Context["escalated_to"] = "ceo"
	decision.Context["returned_to"] = returnedTo
	decision.Context["escalation_reason"] = reason

	_, _ = a.UpdateBead(beadID, map[string]interface{}{
		"priority": models.BeadPriorityP0,
		"context": map[string]string{
			"escalated_to_ceo_at":          time.Now().UTC().Format(time.RFC3339),
			"escalated_to_ceo_reason":      reason,
			"escalated_to_ceo_decision_id": decision.ID,
		},
	})

	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeDecisionCreated,
			Source:    "ceo-escalation",
			ProjectID: b.ProjectID,
			Data: map[string]interface{}{
				"decision_id": decision.ID,
				"bead_id":     beadID,
				"reason":      reason,
			},
		})
	}

	return decision, nil
}

func (a *Loom) applyCEODecisionToParent(decisionID string) error {
	d, err := a.decisionManager.GetDecision(decisionID)
	if err != nil || d == nil || d.Context == nil {
		return nil
	}
	if d.Context["escalated_to"] != "ceo" {
		return nil
	}
	parentID := d.Parent
	if parentID == "" {
		return nil
	}

	decision := strings.ToLower(strings.TrimSpace(d.Decision))
	switch decision {
	case "approve":
		_, _ = a.UpdateBead(parentID, map[string]interface{}{"status": models.BeadStatusClosed})
	case "deny":
		_, _ = a.UpdateBead(parentID, map[string]interface{}{
			"status":      models.BeadStatusOpen,
			"assigned_to": "",
			"context": map[string]string{
				"ceo_denied_at": time.Now().UTC().Format(time.RFC3339),
				"ceo_comment":   d.Rationale,
			},
		})
	case "needs_more_info":
		returnedTo := d.Context["returned_to"]
		_, _ = a.UpdateBead(parentID, map[string]interface{}{
			"status":      models.BeadStatusOpen,
			"assigned_to": returnedTo,
			"context": map[string]string{
				"redispatch_requested":   "true",
				"ceo_needs_more_info_at": time.Now().UTC().Format(time.RFC3339),
				"ceo_comment":            d.Rationale,
			},
		})
	}

	return nil
}

// UnblockDependents unblocks beads that were waiting on a decision
func (a *Loom) UnblockDependents(decisionID string) error {
	blocked := a.decisionManager.GetBlockedBeads(decisionID)

	for _, beadID := range blocked {
		if err := a.beadsManager.UnblockBead(beadID, decisionID); err != nil {
			return fmt.Errorf("failed to unblock bead %s: %w", beadID, err)
		}
	}

	return nil
}

// ClaimBead assigns a bead to an agent
func (a *Loom) ClaimBead(beadID, agentID string) error {
	// Verify agent exists
	if _, err := a.agentManager.GetAgent(agentID); err != nil {
		observability.Error("bead.claim", map[string]interface{}{
			"agent_id": agentID,
			"bead_id":  beadID,
		}, err)
		return fmt.Errorf("agent not found: %w", err)
	}

	// Claim the bead
	if err := a.beadsManager.ClaimBead(beadID, agentID); err != nil {
		observability.Error("bead.claim", map[string]interface{}{
			"agent_id": agentID,
			"bead_id":  beadID,
		}, err)
		return fmt.Errorf("failed to claim bead: %w", err)
	}

	// Update agent status
	if err := a.agentManager.AssignBead(agentID, beadID); err != nil {
		observability.Error("agent.assign_bead", map[string]interface{}{
			"agent_id": agentID,
			"bead_id":  beadID,
		}, err)
		return fmt.Errorf("failed to assign bead to agent: %w", err)
	}

	if a.eventBus != nil {
		projectID := ""
		if b, err := a.beadsManager.GetBead(beadID); err == nil && b != nil {
			projectID = b.ProjectID
		}
		_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadAssigned, beadID, projectID, map[string]interface{}{
			"assigned_to": agentID,
		})
		_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadStatusChange, beadID, projectID, map[string]interface{}{
			"status": string(models.BeadStatusInProgress),
		})
	}

	projectID := ""
	if b, err := a.beadsManager.GetBead(beadID); err == nil && b != nil {
		projectID = b.ProjectID
	}
	observability.Info("bead.claim", map[string]interface{}{
		"agent_id":   agentID,
		"bead_id":    beadID,
		"project_id": projectID,
		"status":     "claimed",
	})

	return nil
}

// UpdateBead updates a bead and publishes relevant events.
func (a *Loom) UpdateBead(beadID string, updates map[string]interface{}) (*models.Bead, error) {
	if err := a.beadsManager.UpdateBead(beadID, updates); err != nil {
		return nil, err
	}

	bead, err := a.beadsManager.GetBead(beadID)
	if err != nil {
		return nil, err
	}

	if a.eventBus != nil {
		if status, ok := updates["status"].(models.BeadStatus); ok {
			_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadStatusChange, beadID, bead.ProjectID, map[string]interface{}{
				"status": string(status),
			})
			if status == models.BeadStatusClosed {
				_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadCompleted, beadID, bead.ProjectID, map[string]interface{}{})
			}
		}
		if assignedTo, ok := updates["assigned_to"].(string); ok && assignedTo != "" {
			_ = a.eventBus.PublishBeadEvent(eventbus.EventTypeBeadAssigned, beadID, bead.ProjectID, map[string]interface{}{
				"assigned_to": assignedTo,
			})
		}
	}

	return bead, nil
}

// GetReadyBeads returns beads that are ready to work on
func (a *Loom) GetReadyBeads(projectID string) ([]*models.Bead, error) {
	return a.beadsManager.GetReadyBeads(projectID)
}

// GetWorkGraph returns the dependency graph of beads
func (a *Loom) GetWorkGraph(projectID string) (*models.WorkGraph, error) {
	return a.beadsManager.GetWorkGraph(projectID)
}

// GetFileLockManager returns the file lock manager
func (a *Loom) GetFileLockManager() *FileLockManager {
	return a.fileLockManager
}

// GetGitopsManager returns the gitops manager
func (a *Loom) GetGitopsManager() *gitops.Manager {
	return a.gitopsManager
}

// StartMaintenanceLoop starts background maintenance tasks
func (a *Loom) StartMaintenanceLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	var lastFederationSync time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Clean expired file locks
			cleaned := a.fileLockManager.CleanExpiredLocks()
			if cleaned > 0 {
				// Log: cleaned N expired locks
				_ = cleaned
			}

			// Check for stale agents (no heartbeat in 2x interval)
			staleThreshold := time.Now().Add(-2 * a.config.Agents.HeartbeatInterval)
			for _, agent := range a.agentManager.ListAgents() {
				if agent.LastActive.Before(staleThreshold) {
					// Log: agent stale, releasing locks
					_ = a.fileLockManager.ReleaseAgentLocks(agent.ID)
				}
			}

			// FIX #5: Reset agents stuck in working state for > 5 minutes
			resetCount := a.agentManager.ResetStuckAgents(5 * time.Minute)
			if resetCount > 0 {
				log.Printf("[Maintenance] Reset %d stuck agents", resetCount)
			}

			// NOTE: Stuck bead resolution is handled by the Ralph Loop
			// (LoomHeartbeatActivity). CEO escalation is only available via
			// explicit CLI/REPL commands.

			// Refresh bead cache from Dolt to pick up beads created externally
			for _, p := range a.projectManager.ListProjects() {
				if p.BeadsPath != "" {
					beadsRoot := p.BeadsPath
					if p.WorkDir != "" {
						beadsRoot = filepath.Join(p.WorkDir, p.BeadsPath)
					}
					if err := a.beadsManager.RefreshBeads(p.ID, beadsRoot); err != nil {
						log.Printf("[Maintenance] Bead refresh failed for %s: %v", p.ID, err)
					}
				}
			}

			// Periodic federation sync
			if a.config.Beads.Federation.Enabled && a.config.Beads.Federation.SyncInterval > 0 {
				if time.Since(lastFederationSync) >= a.config.Beads.Federation.SyncInterval {
					if err := a.beadsManager.SyncFederation(ctx, &a.config.Beads.Federation); err != nil {
						log.Printf("[Federation] Periodic sync failed: %v", err)
					}
					lastFederationSync = time.Now()
				}
			}
		}
	}
}

// StartDispatchLoop runs a periodic dispatcher that fills all idle agents with work.
func (a *Loom) StartDispatchLoop(ctx context.Context, interval time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[DispatchLoop] PANIC recovered: %v", r)
		}
	}()

	if a == nil || a.dispatcher == nil {
		log.Printf("[DispatchLoop] No dispatcher configured, skipping")
		return
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}

	log.Printf("[DispatchLoop] Starting with %s interval", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for i := 0; i < 50; i++ {
				dr, err := a.dispatcher.DispatchOnce(ctx, "")
				if err != nil || dr == nil || !dr.Dispatched {
					break
				}
			}
		}
	}
}

// checkProviderHealthAndActivate checks if a newly registered provider has models available
// and immediately activates it if so, without waiting for the heartbeat workflow
func (a *Loom) checkProviderHealthAndActivate(providerID string) {
	time.Sleep(300 * time.Millisecond)
	log.Printf("Checking health for provider: %s", providerID)
	models, err := a.GetProviderModels(context.Background(), providerID)
	if err != nil {
		log.Printf("Provider %s health check failed: %v", providerID, err)
		return
	}
	if len(models) == 0 {
		log.Printf("Provider %s returned no models", providerID)
		return
	}

	log.Printf("Provider %s is healthy, activating (models: %d)", providerID, len(models))
	// Update provider status to active in both database and registry
	if dbProvider, err := a.database.GetProvider(providerID); err == nil && dbProvider != nil {
		dbProvider.Status = "active"
		_ = a.database.UpsertProvider(dbProvider)
		// Sync to registry so UI sees the updated status
		_ = a.providerRegistry.Upsert(&provider.ProviderConfig{
			ID:                     dbProvider.ID,
			Name:                   dbProvider.Name,
			Type:                   dbProvider.Type,
			Endpoint:               dbProvider.Endpoint,
			Model:                  dbProvider.SelectedModel,
			ConfiguredModel:        dbProvider.ConfiguredModel,
			SelectedModel:          dbProvider.SelectedModel,
			SelectedGPU:            dbProvider.SelectedGPU,
			Status:                 "active",
			LastHeartbeatAt:        dbProvider.LastHeartbeatAt,
			LastHeartbeatLatencyMs: dbProvider.LastHeartbeatLatencyMs,
		})
		log.Printf("Provider %s activated successfully", providerID)
	}

	// Attach newly active provider to paused agents (best-effort)
	a.attachProviderToPausedAgents(context.Background(), providerID)
}

// Perpetual tasks are implemented via the motivation system.
// See internal/motivation/perpetual.go for role-based perpetual task definitions.
// These tasks run on scheduled intervals (hourly, daily, weekly) to enable proactive
// agent workflows. Examples:
// - CFO: Daily budget reviews, weekly cost optimization reports
// - QA Engineer: Daily automated test runs, weekly integration tests
// - PR Manager: Hourly GitHub activity checks
// - Documentation Manager: Daily documentation audits
// The motivation engine evaluates these on regular intervals and creates beads automatically.

// ResumeAgentsWaitingForProvider resumes agents that were paused waiting for a provider to become healthy
func (a *Loom) ResumeAgentsWaitingForProvider(ctx context.Context, providerID string) error {
	if a.agentManager == nil || a.database == nil {
		return nil
	}

	// Get all agents using this provider
	agents := a.agentManager.ListAgents()
	if agents == nil {
		return nil
	}

	for _, agent := range agents {
		if agent == nil || agent.ProviderID != providerID || agent.Status != "paused" {
			continue
		}
		// Resume the paused agent
		agent.Status = "idle"
		agent.LastActive = time.Now()

		// Write-through: Update database first
		if err := a.database.UpsertAgent(agent); err != nil {
			fmt.Printf("Warning: failed to resume agent %s: %v\n", agent.ID, err)
			continue
		}

		// Write-through: Update in-memory cache
		if err := a.agentManager.UpdateAgentStatus(agent.ID, "idle"); err != nil {
			fmt.Printf("Warning: failed to update agent %s status in memory: %v\n", agent.ID, err)
		}
	}

	// Trigger dispatch to pick up any waiting beads
	if a.dispatcher != nil {
		_, _ = a.dispatcher.DispatchOnce(ctx, "")
	}

	return nil
}

// attachProviderToPausedAgents assigns a newly active provider to any paused agents that lack one.
func (a *Loom) attachProviderToPausedAgents(ctx context.Context, providerID string) {
	if a.agentManager == nil || a.database == nil || providerID == "" {
		return
	}

	if !a.providerRegistry.IsActive(providerID) {
		return
	}

	agents, err := a.database.ListAgents()
	if err != nil {
		log.Printf("Failed to list agents for provider attachment: %v", err)
		return
	}

	log.Printf("Found %d agent(s) to check for provider %s attachment", len(agents), providerID)
	attachedCount := 0
	updatedCount := 0
	skippedCount := 0
	for _, ag := range agents {
		if ag == nil {
			continue
		}

		// If agent already has a provider, check if we should upgrade it
		if ag.ProviderID != "" {
			// Check if current provider is healthy
			if a.providerRegistry.IsActive(ag.ProviderID) {
				// Current provider is healthy - skip this agent
				log.Printf("Skipping agent %s (%s) - already has healthy provider %s (status: %s)", ag.ID, ag.Name, ag.ProviderID, ag.Status)
				skippedCount++
				continue
			}

			// Current provider is unhealthy/failed - upgrade to new healthy provider
			log.Printf("Agent %s (%s) has unhealthy provider %s - upgrading to healthy provider %s", ag.ID, ag.Name, ag.ProviderID, providerID)

			// If agent is paused, also update status to idle
			if ag.Status == "paused" {
				ag.Status = "idle"
			}
			// Don't continue here - fall through to attach the new provider
		}

		// Attach persona for prompt context
		if ag.Persona == nil && ag.PersonaName != "" {
			persona, err := a.personaManager.LoadPersona(ag.PersonaName)
			if err != nil {
				log.Printf("Failed to load persona %s for agent %s: %v", ag.PersonaName, ag.ID, err)
				continue
			}
			ag.Persona = persona
		}

		// Update agent with provider
		ag.ProviderID = providerID
		ag.Status = "idle"
		ag.LastActive = time.Now()

		// Write-through cache: Update database first (source of truth)
		if err := a.database.UpsertAgent(ag); err != nil {
			log.Printf("Failed to upsert agent %s with provider %s: %v", ag.ID, providerID, err)
			continue
		}

		// Write-through cache: Update in-memory cache (RestoreAgentWorker handles both new and existing agents)
		if _, err := a.agentManager.RestoreAgentWorker(ctx, ag); err != nil {
			log.Printf("Failed to restore/update agent worker %s: %v", ag.ID, err)
			continue
		}

		if ag.ProjectID != "" {
			_ = a.projectManager.AddAgentToProject(ag.ProjectID, ag.ID)
		}
		attachedCount++
		log.Printf("Successfully attached provider %s to agent %s (%s)", providerID, ag.ID, ag.Name)
	}
	if attachedCount > 0 || updatedCount > 0 {
		log.Printf("Provider %s: attached to %d agent(s), updated status for %d agent(s), skipped %d agent(s)",
			providerID, attachedCount, updatedCount, skippedCount)
	}
}
