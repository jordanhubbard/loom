package loom

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/internal/actions"
	"github.com/jordanhubbard/loom/internal/activity"
	"github.com/jordanhubbard/loom/internal/agent"
	"github.com/jordanhubbard/loom/internal/analytics"
	"github.com/jordanhubbard/loom/internal/beads"
	"github.com/jordanhubbard/loom/internal/collaboration"
	"github.com/jordanhubbard/loom/internal/comments"
	"github.com/jordanhubbard/loom/internal/consensus"
	"github.com/jordanhubbard/loom/internal/containers"
	"github.com/jordanhubbard/loom/internal/database"
	"github.com/jordanhubbard/loom/internal/decision"

	"github.com/jordanhubbard/loom/internal/dispatch"
	"github.com/jordanhubbard/loom/internal/eventbus"
	"github.com/jordanhubbard/loom/internal/executor"
	"github.com/jordanhubbard/loom/internal/files"
	"github.com/jordanhubbard/loom/internal/gitops"
	"github.com/jordanhubbard/loom/internal/keymanager"
	"github.com/jordanhubbard/loom/internal/logging"
	"github.com/jordanhubbard/loom/internal/meetings"
	"github.com/jordanhubbard/loom/internal/memory"
	"github.com/jordanhubbard/loom/internal/messagebus"
	"github.com/jordanhubbard/loom/internal/metrics"
	"github.com/jordanhubbard/loom/internal/modelcatalog"
	internalmodels "github.com/jordanhubbard/loom/internal/models"
	"github.com/jordanhubbard/loom/internal/motivation"
	"github.com/jordanhubbard/loom/internal/notifications"
	"github.com/jordanhubbard/loom/internal/observability"
	"github.com/jordanhubbard/loom/internal/openclaw"
	"github.com/jordanhubbard/loom/internal/orchestrator"
	"github.com/jordanhubbard/loom/internal/orgchart"
	"github.com/jordanhubbard/loom/internal/patterns"
	"github.com/jordanhubbard/loom/internal/persona"
	"github.com/jordanhubbard/loom/internal/project"
	"github.com/jordanhubbard/loom/internal/provider"
	"github.com/jordanhubbard/loom/internal/ralph"
	"github.com/jordanhubbard/loom/internal/statusboard"
	"github.com/jordanhubbard/loom/internal/swarm"
	"github.com/jordanhubbard/loom/internal/taskexecutor"
	"github.com/jordanhubbard/loom/internal/workflow"
	"github.com/jordanhubbard/loom/pkg/config"
	"github.com/jordanhubbard/loom/pkg/connectors"
	"github.com/jordanhubbard/loom/pkg/models"
)

func New(cfg *config.Config) (*Loom, error) {
	personaPath := cfg.Agents.DefaultPersonaPath
	if personaPath == "" {
		personaPath = "./personas"
	}

	providerRegistry := provider.NewRegistry()

	// Initialize NATS message bus if configured
	var messageBus interface{}
	natsURL := os.Getenv("NATS_URL")
	if natsURL != "" {
		mbCfg := messagebus.Config{
			URL:        natsURL,
			StreamName: "LOOM",
			Timeout:    10 * time.Second,
		}
		mb, err := messagebus.NewNatsMessageBus(mbCfg)
		if err != nil {
			log.Printf("Warning: failed to initialize NATS message bus: %v", err)
			// Don't fail startup if NATS is unavailable - allow graceful degradation
		} else {
			messageBus = mb
			log.Printf("Initialized NATS message bus at %s", natsURL)
		}
	}

	// Initialize in-memory event bus.
	eb := eventbus.NewEventBus()

	// Bridge the in-memory EventBus to NATS for cross-container communication.
	var bridge *messagebus.BridgedMessageBus
	if messageBus != nil {
		if mb, ok := messageBus.(*messagebus.NatsMessageBus); ok {
			hostname, _ := os.Hostname()
			bridge = messagebus.NewBridgedMessageBus(mb, eb, "loom-control-"+hostname)
		}
	}

	// Initialize PostgreSQL database.
	// Config DSN takes priority; otherwise fall back to environment variables (POSTGRES_HOST, etc.)
	// An empty database Type means "no database" (skip initialization).
	var db *database.Database
	if cfg.Database.DSN != "" {
		var err error
		db, err = database.NewPostgres(cfg.Database.DSN)
		if err != nil {
			log.Printf("Warning: failed to initialize postgres: %v (running without persistence)", err)
		}
		log.Printf("Initialized postgres database from config DSN")
	} else if cfg.Database.Type != "" {
		var err error
		db, err = database.NewFromEnv()
		if err != nil {
			log.Printf("Warning: failed to initialize database: %v (running without persistence)", err)
		} else {
			log.Printf("Initialized postgres database from environment")
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
		log.Printf("Warning: failed to initialize gitops manager: %v", err)
	}
	gitopsMgr.SetSelfProjectID(cfg.GetSelfProjectID())

	// All projects are cloned consistently - no special workdir handling

	agentMgr := agent.NewWorkerManager(cfg.Agents.MaxConcurrent, providerRegistry, eb)
	if db != nil {
		agentMgr.SetAgentPersister(db)
		// Enable conversation context support for multi-turn conversations
		// Deprecated: WorkerPool is deprecated in favor of taskexecutor workers.
		// agentMgr.GetWorkerPool().SetDatabase(db)
	}

	// Initialize shell executor if database is available
	var shellExec *executor.ShellExecutor
	if db != nil {
		shellExec = executor.NewShellExecutor(db.DB())
	}
	var logMgr *logging.Manager
	if db != nil {
		logMgr = logging.NewManager(db.DB())
		logMgr.InstallLogInterceptor()
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

	// Initialize meetings manager
	var meetingsMgr *meetings.Manager
	if db != nil {
		meetingsMgr = meetings.NewManager()
	}
	// Initialize pattern manager and analytics logger if database is available
	var patternMgr *patterns.Manager
	if db != nil {
		analyticsStorage, err := analytics.NewDatabaseStorage(db.DB())
		if err != nil {
			log.Printf("Warning: failed to initialize analytics storage: %v", err)
		} else if analyticsStorage != nil {
			patternMgr = patterns.NewManager(analyticsStorage, nil)
			// Wire analytics logger to WorkerManager so LLM completions are logged
			agentMgr.SetAnalyticsLogger(analytics.NewLogger(analyticsStorage, analytics.DefaultPrivacyConfig()))
		}
	}

	// Initialize Dolt coordinator for multi-reader/multi-writer bead management
	// DISABLED: Let bd CLI manage Dolt in embedded mode to avoid lock conflicts
	var doltCoord *beads.DoltCoordinator
	// if cfg.Beads.Backend == "dolt" {
	// 	masterProject := cfg.GetSelfProjectID()
	// 	if len(cfg.Projects) > 0 {
	// 		masterProject = cfg.Projects[0].ID
	// 	}
	// 	doltCoord = beads.NewDoltCoordinator(masterProject, cfg.Beads.BDPath, 3307)
	// }

	// Initialize OpenClaw messaging gateway client and bridge (nil when disabled).
	ocClient := openclaw.NewClient(&cfg.OpenClaw)
	ocBridge := openclaw.NewBridge(ocClient, eb, &cfg.OpenClaw)

	// Initialize container orchestrator for per-project containers
	// Control plane URL for project agents to communicate back
	// Use container name "loom" as hostname (Docker network DNS resolution)
	controlPlaneURL := "http://loom:8081" // Port 8081 is the internal port
	if host := os.Getenv("CONTROL_PLANE_HOST"); host != "" {
		controlPlaneURL = fmt.Sprintf("http://%s:8081", host)
	}
	containerOrch, err := containers.NewOrchestrator(projectKeyDir, controlPlaneURL)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize container orchestrator: %w", err)
	}

	// Initialize connector manager for external service integrations
	connectorsConfigPath := filepath.Join("/app/data", "connectors.yaml")
	connectorMgr := connectors.NewManager(connectorsConfigPath)
	if err := connectorMgr.LoadConfig(); err != nil {
		log.Printf("Warning: Failed to load connectors config: %v", err)
	}
	// Start health monitoring for all connectors
	connectorMgr.StartHealthMonitoring(30 * time.Second)

	beadsMgr := beads.NewManager(cfg.Beads.BDPath)
	beadsMgr.SetBackend(cfg.Beads.Backend)

	collaborationStore := collaboration.NewContextStore()
	consensusManager := consensus.NewDecisionManager()
	statusBoard := statusboard.New()

	// Create shutdown context for goroutine lifecycle management
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	// Create motivation engine (will be wired after arb is created)
	var motivationEngine *motivation.Engine

	arb := &Loom{
		config:                cfg,
		startedAt:             time.Now().UTC(),
		agentManager:          agentMgr,
		projectManager:        project.NewManager(),
		personaManager:        persona.NewManager(personaPath),
		beadsManager:          beadsMgr,
		decisionManager:       decision.NewManager(),
		fileLockManager:       NewFileLockManager(cfg.Agents.FileLockTimeout),
		orgChartManager:       orgchart.NewManager(),
		providerRegistry:      providerRegistry,
		database:              db,
		eventBus:              eb,
		modelCatalog:          modelCatalog,
		gitopsManager:         gitopsMgr,
		shellExecutor:         shellExec,
		logManager:            logMgr,
		activityManager:       activityMgr,
		notificationManager:   notificationMgr,
		commentsManager:       commentsMgr,
		collaborationStore:    collaborationStore,
		consensusManager:      consensusManager,
		meetingsManager:       meetingsMgr,
		motivationRegistry:    motivationRegistry,
		idleDetector:          idleDetector,
		motivationEngine:      motivationEngine,
		workflowEngine:        workflowEngine,
		patternManager:        patternMgr,
		metrics:               metrics.NewMetrics(),
		doltCoordinator:       doltCoord,
		openclawClient:        ocClient,
		openclawBridge:        ocBridge,
		containerOrchestrator: containerOrch,
		connectorManager:      connectorMgr,
		messageBus:            messageBus,
		bridge:                bridge,
		statusBoard:           statusBoard,
		shutdownCtx:           shutdownCtx,
		shutdownCancel:        shutdownCancel,
	}

	buildEnv := actions.NewBuildEnvManager(providerRegistry)
	if containerOrch != nil {
		buildEnv.SetOnReady(containerOrch.SnapshotAfterSetup)
	}

	actionRouter := &actions.Router{
		Beads:         arb,
		Closer:        arb,
		Escalator:     arb,
		Commands:      arb,
		Files:         files.NewManager(gitopsMgr),
		Git:           actions.NewProjectGitRouter(gitopsMgr),
		Logger:        arb,
		Workflow:      arb,
		Projects:      arb,
		ContainerOrch: actions.NewContainerOrchAdapter(containerOrch),
		BuildEnv:      buildEnv,
		BeadType:      "task",
		BeadReader:    arb,
		DefaultP0:     true,
		Board:         arb.statusBoard,
		Meetings:      arb.meetingsManager,
		Consulter:     arb,
		Voter:         arb,
		Cfg:           cfg,
	}
	arb.actionRouter = actionRouter
	motivationEngine = motivation.NewEngine(motivationRegistry, NewLoomStateProvider(arb), arb)
	arb.motivationEngine = motivationEngine
	agentMgr.SetActionRouter(actionRouter)

	// Enable multi-turn action loop
	agentMgr.SetActionLoopEnabled(true)
	agentMgr.SetMaxLoopIterations(100) // Increased to 100 to allow full development cycle (explore + plan + edit + build + test + commit)
	if db != nil {
		agentMgr.SetDatabase(db)
		arb.memoryManager = memory.NewMemoryManager(db)
	}

	arb.readinessCache = make(map[string]projectReadinessState)
	arb.readinessFailures = make(map[string]time.Time)

	// Configure container orchestrator with message bus if available
	if messageBus != nil {
		if mb, ok := messageBus.(*messagebus.NatsMessageBus); ok {
			if containerOrch != nil {
				containerOrch.SetMessageBus(mb)
			}
		}
	}

	// Wire container orchestrator for per-project isolation
	if containerOrch != nil {
		if shellExec != nil {
			shellExec.SetContainerOrchestrator(containerOrch, arb.projectManager)
			shellExec.SetEnvReadyHook(func(ctx context.Context, projectID string, agent *containers.ProjectAgentClient) {
				if actionRouter.BuildEnv != nil {
					if err := actionRouter.BuildEnv.EnsureReady(ctx, projectID, agent); err != nil {
						log.Printf("[ShellExecutor] env init for %s failed (non-fatal): %v", projectID, err)
					}
				}
			})
		}
	}

	// Setup provider metrics tracking
	arb.setupProviderMetrics()

	return arb, nil
}
func (a *Loom) Initialize(ctx context.Context) error {
	log.Printf("[Loom] DEBUG: Initialize started")
	// Prefer database-backed configuration when available.
	var projects []*models.Project
	if a.database != nil {
		storedProjects, err := a.database.ListProjects()
		if err != nil {
			return fmt.Errorf("failed to load projects: %w", err)
		}
		if len(storedProjects) > 0 {
			projects = storedProjects
			// Apply config overrides for fields not stored in the DB schema (e.g. UseContainer).
			cfgByID := make(map[string]struct{ UseContainer, UseWorktrees bool })
			for _, cp := range a.config.Projects {
				cfgByID[cp.ID] = struct{ UseContainer, UseWorktrees bool }{UseContainer: cp.UseContainer, UseWorktrees: cp.UseWorktrees}
			}
			for _, sp := range projects {
				if sp == nil {
					continue
				}
				if cfg, ok := cfgByID[sp.ID]; ok {
					sp.UseContainer = cfg.UseContainer
					sp.UseWorktrees = cfg.UseWorktrees
				}
			}
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
					GitHubRepo:      p.GitHubRepo,
					Branch:          p.Branch,
					BeadsPath:       p.BeadsPath,
					GitAuthMethod:   models.GitAuthMethod(p.GitAuthMethod),
					GitStrategy:     normalizeGitStrategy(models.GitStrategy(p.GitStrategy)),
					GitCredentialID: p.GitCredentialID,
					IsPerpetual:     p.IsPerpetual,
					IsSticky:        p.IsSticky,
					UseContainer:    p.UseContainer,
					UseWorktrees:    p.UseWorktrees,
					Context:         p.Context,
					Status:          models.ProjectStatusOpen,
				}
				if err := a.database.UpsertProject(proj); err != nil {
					log.Printf("[Loom] Warning: failed to persist project %s to database: %v", proj.ID, err)
				}
				projects = append(projects, proj)
			}
		} else {
			// Bootstrap from config.yaml into the configuration database.
			for _, p := range a.config.Projects {
				proj := &models.Project{
					ID:              p.ID,
					Name:            p.Name,
					GitRepo:         p.GitRepo,
					GitHubRepo:      p.GitHubRepo,
					Branch:          p.Branch,
					BeadsPath:       p.BeadsPath,
					GitAuthMethod:   models.GitAuthMethod(p.GitAuthMethod),
					GitStrategy:     normalizeGitStrategy(models.GitStrategy(p.GitStrategy)),
					GitCredentialID: p.GitCredentialID,
					IsPerpetual:     p.IsPerpetual,
					IsSticky:        p.IsSticky,
					UseWorktrees:    p.UseWorktrees,
					UseContainer:    p.UseContainer,
					Context:         p.Context,
					Status:          models.ProjectStatusOpen,
				}
				if err := a.database.UpsertProject(proj); err != nil {
					log.Printf("[Loom] Warning: failed to persist project %s to database: %v", proj.ID, err)
				}
				projects = append(projects, proj)
			}
		}
	} else {
		for _, p := range a.config.Projects {
			projects = append(projects, &models.Project{
				ID:              p.ID,
				Name:            p.Name,
				GitRepo:         p.GitRepo,
				GitHubRepo:      p.GitHubRepo,
				Branch:          p.Branch,
				BeadsPath:       p.BeadsPath,
				GitAuthMethod:   models.GitAuthMethod(p.GitAuthMethod),
				GitStrategy:     normalizeGitStrategy(models.GitStrategy(p.GitStrategy)),
				GitCredentialID: p.GitCredentialID,
				IsPerpetual:     p.IsPerpetual,
				UseWorktrees:    p.UseWorktrees,
				IsSticky:        p.IsSticky,
				UseContainer:    p.UseContainer,
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
				GitHubRepo:      p.GitHubRepo,
				Branch:          p.Branch,
				BeadsPath:       normalizeBeadsPath(p.BeadsPath),
				GitAuthMethod:   normalizeGitAuthMethod(p.GitRepo, models.GitAuthMethod(p.GitAuthMethod)),
				GitStrategy:     normalizeGitStrategy(models.GitStrategy(p.GitStrategy)),
				GitCredentialID: p.GitCredentialID,
				UseWorktrees:    p.UseWorktrees,
				IsPerpetual:     p.IsPerpetual,
				IsSticky:        p.IsSticky,
				UseContainer:    p.UseContainer,
				Context:         p.Context,
				Status:          models.ProjectStatusOpen,
			})
		}
	}
	hasLoomProject := false
	for _, p := range projectValues {
		if p.ID == "loom" {
			hasLoomProject = true
			break
		}
	}
	if !hasLoomProject {
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
			if err := a.database.UpsertProject(&p); err != nil {
				log.Printf("[Loom] Warning: failed to persist project %s to database: %v", p.ID, err)
			}
		}
	}

	// Load beads from registered projects.
	log.Printf("[Loom] DEBUG: Starting project loop, %d projects", len(projectValues))
	for i := range projectValues {
		p := &projectValues[i]
		if p.BeadsPath == "" {
			continue
		}

		// All projects are now treated consistently - clone from git
		// No special case for self project

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
			if err := a.database.UpsertProject(p); err != nil {
				log.Printf("[Loom] Warning: failed to persist project %s to database: %v", p.ID, err)
			}
		}

		// Setup git worktrees for project
		// Main branch at /app/data/projects/{id}/main
		// Beads branch at /app/data/projects/{id}/beads
		wtManager := gitops.NewGitWorktreeManager("/app/data/projects")
		beadsBranch := p.BeadsBranch
		if beadsBranch == "" {
			beadsBranch = a.config.Beads.BeadsBranch
		}
		if beadsBranch == "" {
			beadsBranch = "beads-sync" // Default
		}
		if err := wtManager.SetupBeadsWorktree(p.ID, p.Branch, beadsBranch); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to setup beads worktree for %s: %v\n", p.ID, err)
		} else {
			log.Printf("[Loom] Setup beads worktree for project %s", p.ID)
		}

		// Load beads from the beads worktree
		beadsWorktree := wtManager.GetWorktreePath(p.ID, "beads")
		beadsPath := filepath.Join(beadsWorktree, p.BeadsPath)
		a.beadsManager.SetBeadsPath(beadsPath)
		// Track per-project beads path to avoid last-writer-wins across projects
		a.beadsManager.SetProjectBeadsPath(p.ID, beadsPath)
		// Configure git storage for this project
		a.beadsManager.SetGitStorage(p.ID, wtManager, beadsBranch, a.config.Beads.UseGitStorage, string(p.GitAuthMethod), p.GitRepo)
		// Load project prefix from config
		configPath := filepath.Join(beadsWorktree, p.BeadsPath)
		if err := a.beadsManager.LoadProjectPrefixFromConfig(p.ID, configPath); err != nil {
			log.Printf("[Loom] Warning: failed to load project prefix for %s: %v", p.ID, err)
		}
		// Use project's BeadPrefix if set in the model
		if p.BeadPrefix != "" {
			a.beadsManager.SetProjectPrefix(p.ID, p.BeadPrefix)
		}
		// Load historical beads from main worktree first (baseline).
		// These may not yet be on the beads-sync branch.
		mainWorktree := wtManager.GetWorktreePath(p.ID, "main")
		mainBeadsPath := filepath.Join(mainWorktree, p.BeadsPath)
		if mainBeadsPath != beadsPath {
			if err := a.beadsManager.LoadBeadsFromFilesystem(p.ID, mainBeadsPath); err != nil {
				log.Printf("[Loom] Warning: failed to load beads from filesystem for %s: %v", p.ID, err)
			}
		}

		// Load from beads worktree (beads-sync branch) last - overwrites stale
		// main-worktree copies with authoritative beads-sync state.
		if err := a.beadsManager.LoadBeadsFromGit(ctx, p.ID, beadsPath); err != nil {
			log.Printf("[Loom] Warning: failed to load beads from git for %s: %v", p.ID, err)
		}

		// Spawn isolated container for project if configured.
		// Run asynchronously so a slow Docker build/pull does not block startup.
		if p.UseContainer {
			a.goroutineWg.Add(1)
			projCopy := *p
			go func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Fprintf(os.Stderr, "[Loom] PANIC in EnsureProjectContainer for %s: %v\n", projCopy.ID, r)
					}
				}()
				defer a.goroutineWg.Done()
				fmt.Fprintf(os.Stderr, "[Loom] Spawning isolated container for project %s (async)\n", projCopy.ID)
				bgCtx := context.Background()
				if err := a.containerOrchestrator.EnsureProjectContainer(bgCtx, &projCopy); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to start container for project %s: %v\n", projCopy.ID, err)
				} else {
					fmt.Fprintf(os.Stderr, "[Loom] Project %s container started successfully\n", projCopy.ID)
				}
				// Add a mechanism to signal completion or error
				// For example, using a channel to notify when done
			}()
		}

		// Start git-based federation (replaces Dolt)
		if a.config.Beads.Federation.Enabled && a.config.Beads.Federation.SyncMode == "git-native" {
			syncInterval := a.config.Beads.Federation.SyncInterval
			if syncInterval == 0 {
				syncInterval = 30 * time.Second // Default to 30 seconds
			}
			a.goroutineWg.Add(1)
			coordinator := beads.NewGitCoordinator(p.ID, wtManager, syncInterval)
			go func() {
				defer a.goroutineWg.Done()
				coordinator.StartSyncLoop(ctx, a.beadsManager)
			}()
			log.Printf("[Loom] Started GitCoordinator for project %s", p.ID)
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

		// Auto-bootstrap or reconcile provider from LOOM_PROVIDER_URL env var.
		// If no providers exist, seed one. If the "tokenhub" provider exists but
		// its endpoint drifted (e.g. container network changed), update it so
		// workers don't keep hitting an unreachable address.
		if envURL := os.Getenv("LOOM_PROVIDER_URL"); envURL != "" {
			if len(providers) == 0 {
				log.Printf("[Loom] No providers configured — bootstrapping from LOOM_PROVIDER_URL: %s", envURL)
				envAPIKey := os.Getenv("LOOM_PROVIDER_API_KEY")
				seed := &internalmodels.Provider{
					ID:          "tokenhub",
					Name:        "TokenHub",
					Type:        "openai",
					Endpoint:    envURL,
					RequiresKey: envAPIKey != "",
					Status:      "pending",
				}
				if _, regErr := a.RegisterProvider(ctx, seed, envAPIKey); regErr != nil {
					log.Printf("[Loom] Failed to bootstrap provider from env: %v", regErr)
				}
			} else {
				for _, p := range providers {
					if p.ID == "tokenhub" && p.Endpoint != envURL {
						log.Printf("[Loom] Reconciling tokenhub endpoint: %s → %s", p.Endpoint, envURL)
						p.Endpoint = envURL
						if dbErr := a.database.UpsertProvider(p); dbErr != nil {
							log.Printf("[Loom] Failed to reconcile tokenhub endpoint: %v", dbErr)
						}
						break
					}
				}
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
			var apiKey string
			if p.KeyID != "" && a.keyManager != nil && a.keyManager.IsUnlocked() {
				apiKey, _ = a.keyManager.GetKey(p.KeyID)
			}
			if apiKey == "" {
				apiKey = p.APIKey // fall back to key stored directly in provider record
			}
			if err := a.providerRegistry.Upsert(&provider.ProviderConfig{
				ID:                     p.ID,
				Name:                   p.Name,
				Type:                   p.Type,
				Endpoint:               normalizeProviderEndpoint(p.Endpoint),
				APIKey:                 apiKey,
				Model:                  selected,
				ConfiguredModel:        p.ConfiguredModel,
				SelectedModel:          selected,
				Status:                 p.Status,
				LastHeartbeatAt:        p.LastHeartbeatAt,
				LastHeartbeatLatencyMs: p.LastHeartbeatLatencyMs,
			}); err != nil {
				log.Printf("[Loom] Warning: failed to upsert provider %s: %v", p.ID, err)
			}
		}

		// Count providers ready for dispatch; re-probe any that aren't healthy.
		// checkProviderHealthAndActivate is normally called when a provider is first
		// registered. On restart, providers are loaded from DB via Upsert (no probe),
		// so we must re-probe them here to promote unhealthy/pending ones to active.
		healthyCount := 0
		for _, p := range providers {
			if p.Status == "healthy" || p.Status == "active" {
				healthyCount++
			} else {
				pID := p.ID
				go a.checkProviderHealthAndActivate(pID)
			}
		}
		if healthyCount > 0 {
			log.Printf("[Loom] %d providers already healthy, dispatch ready", healthyCount)
		} else {
			log.Printf("[Loom] No providers healthy yet — probing all providers now")
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
			if _, err := a.agentManager.RestoreAgentWorker(ctx, ag); err != nil {
				log.Printf("[Loom] Warning: failed to restore agent worker %s: %v", ag.ID, err)
			}
			if err := a.projectManager.AddAgentToProject(ag.ProjectID, ag.ID); err != nil {
				log.Printf("[Loom] Warning: failed to add agent %s to project %s: %v", ag.ID, ag.ProjectID, err)
			}
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

	// Paranoid model check: validate provider models meet minimum intelligence tier.
	if err := a.runModelCheck(ctx); err != nil {
		return err // only non-nil when on_violation=="block"
	}

	// Ensure default agents are assigned for each project.
	for _, p := range projectValues {
		if p.ID == "" {
			continue
		}
		if err := a.ensureDefaultAgents(ctx, p.ID); err != nil {
			log.Printf("[Loom] Warning: failed to ensure default agents for project %s: %v", p.ID, err)
		}
	}

	// After restoring agents from DB, reset any that were left in "working" state.
	// They have no running goroutine after restart, so clearing their status allows
	// the dispatch loop to re-assign their beads on the first tick.
	if resetCount := a.agentManager.ResetStuckAgents(0); resetCount > 0 {
		log.Printf("[Loom] Reset %d agent(s) left in 'working' state from previous run", resetCount)
	}

	// Reset any beads left in_progress with ephemeral executor IDs from the previous
	// run. Agent status reset above only covers named agents; exec-* goroutine IDs
	// die silently on restart and must be cleaned up here so the task executor can
	// reclaim the work immediately.
	if zombieCount := a.resetZombieBeads(); zombieCount > 0 {
		log.Printf("[Loom] Reset %d zombie bead(s) left in 'in_progress' state from previous run", zombieCount)
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

	// Start the Ralph Loop — a plain goroutine ticker that runs maintenance
	// every 10 seconds (resets stuck agents, auto-blocks looped beads, etc.).
	ralphActs := ralph.New(a.database, a.beadsManager, a.agentManager)
	a.goroutineWg.Add(1)
	go func() {
		defer a.goroutineWg.Done()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		beatCount := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				beatCount++
				if err := ralphActs.Beat(ctx, beatCount); err != nil {
					log.Printf("[Ralph] Beat %d failed: %v", beatCount, err)
				}
			}
		}
	}()

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
			`This is an automated diagnostic task to verify the Loom workflow is operational.

## Your Task

1. Run the project build command to verify the build system works
2. Run the project tests to verify the test system works
3. If both pass, use the 'done' action with reason "Diagnostic complete: build and tests pass"
4. If either fails, use the 'done' action with reason explaining what failed

This is a simple verification task. Do NOT search for bugs or make changes. Just verify build and test, then mark done.`,
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
	}

	// ── Multi-service pub/sub wiring ───────────────────────────────────
	// Start the NATS ↔ EventBus bridge so cross-container events flow.
	if a.bridge != nil {
		if err := a.bridge.Start(ctx); err != nil {
			log.Printf("[Loom] Warning: Failed to start NATS bridge: %v", err)
		}
	}

	// Apply UseNATSDispatch feature flag from config.
	if a.config.Dispatch.UseNATSDispatch && a.messageBus != nil {
		log.Printf("[Loom] NATS dispatch enabled – tasks will be routed to agent containers")
	}

	// Start PDA orchestrator if enabled.
	if a.config.PDA.Enabled && a.messageBus != nil {
		if mb, ok := a.messageBus.(*messagebus.NatsMessageBus); ok {
			var planner orchestrator.Planner
			if a.config.PDA.PlannerEndpoint != "" {
				planner = orchestrator.NewLLMPlanner(
					a.config.PDA.PlannerEndpoint,
					a.config.PDA.PlannerAPIKey,
					a.config.PDA.PlannerModel,
				)
			} else {
				planner = &orchestrator.StaticPlanner{}
			}
			adapter := orchestrator.NewBeadManagerAdapter(a.beadsManager)
			a.pdaOrchestrator = orchestrator.NewPDAOrchestrator(mb, planner, adapter, adapter)
			if err := a.pdaOrchestrator.Start(ctx); err != nil {
				log.Printf("[Loom] Warning: Failed to start PDA orchestrator: %v", err)
			}
		}
	}

	// Start swarm manager if enabled.
	if a.config.Swarm.Enabled && a.messageBus != nil {
		if mb, ok := a.messageBus.(*messagebus.NatsMessageBus); ok {
			hostname, _ := os.Hostname()
			a.swarmManager = swarm.NewManager(mb, "loom-control-plane", "control-plane")
			var projectIDs []string
			for _, p := range a.config.Projects {
				projectIDs = append(projectIDs, p.ID)
			}
			port := a.config.Server.HTTPPort
			endpoint := fmt.Sprintf("http://%s:%d", hostname, port)
			if err := a.swarmManager.Start(ctx, []string{"control-plane"}, projectIDs, endpoint); err != nil {
				log.Printf("[Loom] Warning: Failed to start swarm manager: %v", err)
			}

			// Federation with peer NATS instances
			if len(a.config.Swarm.PeerNATSURLs) > 0 {
				a.swarmFederation = swarm.NewFederation(mb, swarm.FederationConfig{
					PeerNATSURLs: a.config.Swarm.PeerNATSURLs,
					GatewayName:  a.config.Swarm.GatewayName,
				})
				if err := a.swarmFederation.Start(ctx); err != nil {
					log.Printf("[Loom] Warning: Failed to start federation: %v", err)
				}
			}
		}
	}

	// Start motivation engine (non-blocking)
	if a.motivationEngine != nil {
		a.goroutineWg.Add(1)
		go func() {
			defer a.goroutineWg.Done()
			if err := a.motivationEngine.Start(ctx); err != nil && err != ctx.Err() {
				log.Printf("[Loom] Warning: motivation engine exited: %v", err)
			}
		}()
	}

	log.Printf("[Loom] DEBUG: Initialize completed successfully")
	return nil
}

// kickstartOpenBeads starts Temporal workflows for all open beads in registered projects.
func (a *Loom) Shutdown() {
	a.shutdownOnce.Do(func() {
		// Stop all agents first - they may be using other subsystems
		if a.agentManager != nil {
			a.agentManager.StopAll()
		}

		if a.connectorManager != nil {
			if err := a.connectorManager.Close(); err != nil {
				log.Printf("[Loom] Warning: failed to close connector manager: %v", err)
			}
		}
		if a.pdaOrchestrator != nil {
			a.pdaOrchestrator.Close()
		}
		if a.swarmFederation != nil {
			a.swarmFederation.Close()
		}
		if a.swarmManager != nil {
			a.swarmManager.Close()
		}
		if a.bridge != nil {
			a.bridge.Close()
		}
		if a.openclawBridge != nil {
			a.openclawBridge.Close()
		}
		if a.doltCoordinator != nil {
			a.doltCoordinator.Shutdown()
		}
		if a.eventBus != nil {
			a.eventBus.Close()
		}
		if a.messageBus != nil {
			if mb, ok := a.messageBus.(*messagebus.NatsMessageBus); ok {
				if err := mb.Close(); err != nil {
					log.Printf("[Loom] Warning: failed to close message bus: %v", err)
				}
			}
		}
		if a.database != nil {
			if err := a.database.Close(); err != nil {
				log.Printf("[Loom] Warning: failed to close database: %v", err)
			}
		}
	})
}
func (a *Loom) GetEventBus() *eventbus.EventBus {
	return a.eventBus
}

// GetDatabase returns the database instance
func (a *Loom) GetDatabase() *database.Database {
	return a.database
}

// GetMessageBus returns the NATS message bus instance
func (a *Loom) GetMessageBus() interface{} {
	return a.messageBus
}

func (a *Loom) GetConnectorManager() *connectors.Manager {
	return a.connectorManager
}

func (a *Loom) ExecuteShellCommand(ctx context.Context, req executor.ExecuteCommandRequest) (*executor.ExecuteCommandResult, error) {
	if a.shellExecutor == nil {
		return nil, fmt.Errorf("shell executor not available (database not configured)")
	}
	return a.shellExecutor.ExecuteCommand(ctx, req)
}

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

func (a *Loom) GetCommandLogs(filters map[string]interface{}, limit int) ([]*models.CommandLog, error) {
	if a.shellExecutor == nil {
		return nil, fmt.Errorf("shell executor not available (database not configured)")
	}
	return a.shellExecutor.GetCommandLogs(filters, limit)
}

func (a *Loom) GetCommandLog(id string) (*models.CommandLog, error) {
	if a.shellExecutor == nil {
		return nil, fmt.Errorf("shell executor not available (database not configured)")
	}
	return a.shellExecutor.GetCommandLog(id)
}

func (a *Loom) GetActionRouter() *actions.Router {
	return a.actionRouter
}
func (a *Loom) GetGitOpsManager() *gitops.Manager {
	return a.gitopsManager
}

// GetGitopsManager is an alias for GetGitOpsManager (lowercase 'o' variant).
func (a *Loom) GetGitopsManager() *gitops.Manager {
	return a.gitopsManager
}

// GetFileLockManager returns the file lock manager.
func (a *Loom) GetFileLockManager() *FileLockManager {
	return a.fileLockManager
}

// GetWorkGraph returns the dependency graph of beads for a project.
func (a *Loom) GetWorkGraph(projectID string) (*models.WorkGraph, error) {
	return a.beadsManager.GetWorkGraph(projectID)
}

// SetKeyManager sets the key manager for encrypted credential storage.
func (a *Loom) SetKeyManager(km *keymanager.KeyManager) {
	a.keyManager = km
	// Also wire it into gitops manager for SSH key DB persistence
	if a.gitopsManager != nil {
		a.gitopsManager.SetKeyManager(km)
	}
}

func (a *Loom) GetKeyManager() *keymanager.KeyManager {
	return a.keyManager
}

// GetDispatcher is deprecated; the dispatch loop was replaced by TaskExecutor.
func (a *Loom) GetDispatcher() *dispatch.Dispatcher {
	return nil
}

// GetPersonaManager returns the persona manager
func (a *Loom) GetPersonaManager() *persona.Manager {
	return a.personaManager
}

func (a *Loom) GetDoltCoordinator() *beads.DoltCoordinator {
	return a.doltCoordinator
}

// GetOrgChartManager returns the org chart manager
func (a *Loom) GetOrgChartManager() *orgchart.Manager {
	return a.orgChartManager
}

func (a *Loom) GetMotivationRegistry() *motivation.Registry {
	return a.motivationRegistry
}

func (a *Loom) GetMotivationEngine() *motivation.Engine {
	return a.motivationEngine
}

func (a *Loom) GetIdleDetector() *motivation.IdleDetector {
	return a.idleDetector
}

func (a *Loom) GetWorkflowEngine() *workflow.Engine {
	return a.workflowEngine
}

func (a *Loom) GetMeetingsManager() *meetings.Manager {
	return a.meetingsManager
}

func (a *Loom) GetFeedbackManager() interface{} {
	return nil
}

func (a *Loom) GetDepartmentManager() interface{} {
	return nil
}

func (a *Loom) GetStatusManager() interface{} {
	return nil
}

func (a *Loom) GetActivityManager() *activity.Manager {
	return a.activityManager
}

func (a *Loom) GetNotificationManager() *notifications.Manager {
	return a.notificationManager
}

func (a *Loom) GetCommentsManager() *comments.Manager {
	return a.commentsManager
}

func (a *Loom) GetLogManager() *logging.Manager {
	return a.logManager
}

func (a *Loom) GetPatternManager() *patterns.Manager {
	return a.patternManager
}

func (a *Loom) GetModelCatalog() *modelcatalog.Catalog {
	return a.modelCatalog
}

func (a *Loom) GetMetrics() *metrics.Metrics {
	return a.metrics
}

func (a *Loom) GetOpenClawClient() *openclaw.Client {
	return a.openclawClient
}

func (a *Loom) GetOpenClawBridge() *openclaw.Bridge {
	return a.openclawBridge
}

func (a *Loom) GetContainerOrchestrator() *containers.Orchestrator {
	return a.containerOrchestrator
}

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

func (a *Loom) StartDevelopment(ctx context.Context, workflow string, requireReviews bool, projectPath string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("StartDevelopment is handled directly by the router")
}

func (a *Loom) WhatsNext(ctx context.Context, userInput, contextStr, conversationSummary string, recentMessages []map[string]string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("WhatsNext is handled directly by the router")
}

func (a *Loom) ProceedToPhase(ctx context.Context, targetPhase, reviewState, reason string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("ProceedToPhase is handled directly by the router")
}

func (a *Loom) ConductReview(ctx context.Context, targetPhase string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("ConductReview is handled directly by the router")
}

func (a *Loom) ResumeWorkflow(ctx context.Context, includeSystemPrompt bool) (map[string]interface{}, error) {
	return nil, fmt.Errorf("ResumeWorkflow is handled directly by the router")
}

func (a *Loom) GetWorkerManager() *agent.WorkerManager {
	return a.agentManager
}

// Project management helpers
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

	// Backfill any positions from the default template that are missing from
	// the existing project chart. This ensures that new personas added to
	// DefaultOrgChartPositions() are automatically propagated to all existing
	// projects without requiring a fresh project creation.
	defaultPositions := models.DefaultOrgChartPositions()
	existingRoles := make(map[string]struct{})
	for _, p := range chart.Positions {
		existingRoles[p.RoleName] = struct{}{}
	}
	for _, tmplPos := range defaultPositions {
		if _, alreadyExists := existingRoles[tmplPos.RoleName]; !alreadyExists {
			newPos := models.Position{
				ID:           tmplPos.ID,
				RoleName:     tmplPos.RoleName,
				PersonaPath:  tmplPos.PersonaPath,
				Required:     tmplPos.Required,
				MaxInstances: tmplPos.MaxInstances,
				ReportsTo:    tmplPos.ReportsTo,
				AgentIDs:     []string{},
			}
			if addErr := a.orgChartManager.AddPosition(projectID, newPos); addErr == nil {
				log.Printf("[OrgChart] Backfilled missing position %q for project %s", tmplPos.RoleName, projectID)
			}
			// Refresh chart reference after mutation
			if _, err := a.orgChartManager.CreateForProject(projectID, project.Name); err != nil {
				log.Printf("[Loom] Warning: failed to refresh org chart for project %s: %v", projectID, err)
			}
		}
	}

	allowedRoles := a.allowedRoleSet()

	// Map existing agents to their roles (check in-memory first)
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

	// Also check DB agents for this project to avoid creating duplicates when
	// agents couldn't be restored to memory (e.g. persona loading failures on restart).
	// Prefer the most recently active agent per role to avoid accumulating stale duplicates.
	if a.database != nil {
		dbAgents, err := a.database.ListAgents()
		if err == nil {
			// Track the most recently active agent per role.
			type roleCandidate struct {
				id         string
				lastActive time.Time
			}
			bestByRole := map[string]roleCandidate{}
			for _, dbAgent := range dbAgents {
				if dbAgent == nil || dbAgent.ProjectID != project.ID {
					continue
				}
				role := dbAgent.Role
				if role == "" {
					role = roleFromPersonaName(dbAgent.PersonaName)
				}
				if role == "" {
					continue
				}
				current, exists := bestByRole[role]
				if !exists || dbAgent.LastActive.After(current.lastActive) {
					bestByRole[role] = roleCandidate{id: dbAgent.ID, lastActive: dbAgent.LastActive}
				}
			}
			for role, candidate := range bestByRole {
				if existingByRole[role] == "" {
					existingByRole[role] = candidate.id
				}
			}
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
		if err := a.orgChartManager.AssignAgentToRole(projectID, pos.RoleName, agent.ID); err != nil {
			log.Printf("[Loom] Warning: failed to assign agent %s to role %s: %v", agent.ID, pos.RoleName, err)
		}
	}

	return nil
}

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

// findDefaultAssignee returns the ID of the best default triage agent for a project.
// Preference order: CTO > Engineering Manager > any agent assigned to the project.
func (a *Loom) findDefaultAssignee(projectID string) string {
	if a.agentManager == nil {
		return ""
	}
	agents := a.agentManager.ListAgentsByProject(projectID)
	if len(agents) == 0 {
		agents = a.agentManager.ListAgents()
	}
	var fallback string
	for _, ag := range agents {
		role := normalizeRole(ag.Role)
		if role == "cto" || role == "chief-technology-officer" {
			return ag.ID
		}
		if role == "engineering-manager" && fallback == "" {
			fallback = ag.ID
		}
	}
	if fallback != "" {
		return fallback
	}
	// Last resort: first agent for this project
	for _, ag := range agents {
		if ag.ProjectID == projectID || ag.ProjectID == "" {
			return ag.ID
		}
	}
	return ""
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
					if err := a.fileLockManager.ReleaseAgentLocks(agent.ID); err != nil {
						log.Printf("[Loom] Warning: failed to release file locks for agent %s: %v", agent.ID, err)
					}
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

			// Refresh bead cache to pick up beads created externally
			for _, p := range a.projectManager.ListProjects() {
				beadsRoot := a.beadsManager.GetProjectBeadsPath(p.ID)
				if beadsRoot == "" {
					continue
				}
				if err := a.beadsManager.RefreshBeads(p.ID, beadsRoot); err != nil {
					log.Printf("[Maintenance] Bead refresh failed for %s: %v", p.ID, err)
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

			// Self-removal for inactive agents: a persona agent that has had no
			// work for 30 days removes itself from the org chart so the slot can
			// be GC'd. The CEO or the project can always re-spawn the persona
			// later. Required positions (e.g. ceo, engineering-manager) are
			// intentionally excluded from this cleanup.
			a.retireInactiveAgents(30 * 24 * time.Hour)
		}
	}
}

// loomCEOEscalator adapts Loom.EscalateBeadToCEO to the taskexecutor.CEOEscalator
// interface so the recovery sweep can escalate irrecoverable beads.
type loomCEOEscalator struct{ app *Loom }

func (e loomCEOEscalator) EscalateBeadToCEO(beadID, reason, returnedTo string) error {
	_, err := e.app.EscalateBeadToCEO(beadID, reason, returnedTo)
	return err
}

// StartTaskExecutor starts the direct bead execution engine for all registered projects.
// It creates a taskexecutor.Executor and launches worker goroutines per project.
// Call this instead of StartDispatchLoop to bypass Temporal/NATS/WorkerPool overhead.
func (a *Loom) StartTaskExecutor(ctx context.Context) {
	exec := taskexecutor.New(
		a.providerRegistry,
		a.beadsManager,
		a.actionRouter,
		a.projectManager,
		a.database,
	)

	// Wire in the persona manager so workers use rich persona definitions
	// instead of the hardcoded fallback map.
	if a.personaManager != nil {
		exec.SetPersonaManager(a.personaManager)
	}

	// Wire in CEO escalation so irrecoverable beads get human attention.
	exec.SetCEOEscalator(loomCEOEscalator{app: a})

	// Wire in the WorkerManager so the executor uses named agents instead
	// of anonymous goroutines. This makes agent status visible in the UI
	// and enables role-based bead routing.
	if a.agentManager != nil {
		exec.SetAgentManager(a.agentManager)
	}

	// Wire in the org chart for role-based routing and manager escalation.
	if a.orgChartManager != nil {
		exec.SetOrgChart(taskexecutor.NewOrgChartAdapter(a.orgChartManager))
	}

	a.taskExecutor = exec

	// Start watcher + initial workers + oversight loops for all currently registered projects
	for _, proj := range a.projectManager.ListProjects() {
		if proj == nil || proj.ID == "" {
			continue
		}
		exec.Start(ctx, proj.ID)
	}

	// Start the weekly performance review system
	exec.StartReviewSystem(ctx)

	// Watch for newly registered projects
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	started := make(map[string]struct{})
	for _, proj := range a.projectManager.ListProjects() {
		if proj != nil && proj.ID != "" {
			started[proj.ID] = struct{}{}
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, proj := range a.projectManager.ListProjects() {
				if proj == nil || proj.ID == "" {
					continue
				}
				if _, ok := started[proj.ID]; !ok {
					log.Printf("[TaskExecutor] Starting executor for new project %s", proj.ID)
					exec.Start(ctx, proj.ID)
					started[proj.ID] = struct{}{}
				}
			}
		}
	}
}

// GetMemoryManager returns the memory manager instance.
func (a *Loom) GetMemoryManager() *memory.MemoryManager {
	return a.memoryManager
}

// GetSwarmManager returns the swarm manager (nil if NATS is not configured).
func (a *Loom) GetSwarmManager() *swarm.Manager {
	return a.swarmManager
}

// GetCollaborationStore returns the collaboration context store
func (a *Loom) GetCollaborationStore() *collaboration.ContextStore {
	return a.collaborationStore
}

// GetConsensusManager returns the consensus decision manager
func (a *Loom) GetConsensusManager() *consensus.DecisionManager {
	return a.consensusManager
}

// StartedAt returns when this Loom instance was created.
func (a *Loom) StartedAt() time.Time {
	return a.startedAt
}

// CastVote records a vote for a consensus decision.
func (a *Loom) CastVote(ctx context.Context, decisionID, agentID, choice, rationale string) error {
	if a.consensusManager == nil {
		return fmt.Errorf("consensus manager not available")
	}

	return a.consensusManager.CastVote(ctx, decisionID, agentID, consensus.VoteChoice(choice), rationale, 1.0)
}

// StateProvider interface implementations

// GetCurrentTime returns the current time
func (a *Loom) GetCurrentTime() time.Time {
	return time.Now()
}

// PublishMotivationFired publishes a motivation fired event to the event bus.
func (a *Loom) PublishMotivationFired(trigger *motivation.MotivationTrigger) error {
	if a.eventBus == nil {
		return nil
	}
	data := map[string]interface{}{
		"motivation_id": trigger.MotivationID,
		"triggered_at":  trigger.TriggeredAt,
	}
	if trigger.Motivation != nil {
		data["motivation_name"] = trigger.Motivation.Name
		data["agent_role"] = trigger.Motivation.AgentRole
		data["project_id"] = trigger.Motivation.ProjectID
	}
	if err := a.eventBus.Publish(&eventbus.Event{
		Type:   eventbus.EventTypeConfigUpdated,
		Source: "motivation-engine",
		Data:   data,
	}); err != nil {
		log.Printf("[Loom] Warning: failed to publish motivation event: %v", err)
	}
	return nil
}

// StartWorkflow starts a workflow of the given type. input is a map[string]interface{}
// that may contain "project_id" and "bead_id" keys.
func (a *Loom) StartWorkflow(workflowType string, input interface{}) (string, error) {
	if a.workflowEngine == nil {
		return "", fmt.Errorf("workflow engine not available")
	}
	if workflowType == "" {
		return "", fmt.Errorf("workflow type required")
	}

	// Extract project and bead context from input.
	var projectID, beadID string
	if inputMap, ok := input.(map[string]interface{}); ok {
		if pid, ok := inputMap["project_id"].(string); ok {
			projectID = pid
		}
		if bid, ok := inputMap["bead_id"].(string); ok {
			beadID = bid
		}
	}

	// Validate the project exists when specified.
	if projectID != "" {
		if _, err := a.projectManager.GetProject(projectID); err != nil {
			return "", fmt.Errorf("project not found: %w", err)
		}
	}

	// Look up workflow definition by type.
	workflows, err := a.workflowEngine.GetDatabase().ListWorkflows(workflowType, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to list workflows: %w", err)
	}
	if len(workflows) == 0 {
		return "", fmt.Errorf("no workflow found for type %s", workflowType)
	}
	wf := workflows[0]

	// Start the execution (sets state to active and records start timestamp).
	exec, err := a.workflowEngine.StartWorkflow(beadID, wf.ID, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to start workflow: %w", err)
	}

	// Emit workflow.started event.
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeWorkflowStarted,
			Source:    "workflow-engine",
			ProjectID: projectID,
			Data: map[string]interface{}{
				"workflow_id":   wf.ID,
				"workflow_name": wf.Name,
				"execution_id":  exec.ID,
				"bead_id":       beadID,
			},
		})
	}

	return exec.ID, nil
}
