package agenticorp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/jordanhubbard/agenticorp/internal/agent"
	"github.com/jordanhubbard/agenticorp/internal/beads"
	"github.com/jordanhubbard/agenticorp/internal/database"
	"github.com/jordanhubbard/agenticorp/internal/decision"
	"github.com/jordanhubbard/agenticorp/internal/dispatch"
	"github.com/jordanhubbard/agenticorp/internal/executor"
	"github.com/jordanhubbard/agenticorp/internal/gitops"
	"github.com/jordanhubbard/agenticorp/internal/modelcatalog"
	internalmodels "github.com/jordanhubbard/agenticorp/internal/models"
	"github.com/jordanhubbard/agenticorp/internal/orgchart"
	"github.com/jordanhubbard/agenticorp/internal/persona"
	"github.com/jordanhubbard/agenticorp/internal/project"
	"github.com/jordanhubbard/agenticorp/internal/provider"
	"github.com/jordanhubbard/agenticorp/internal/routing"
	"github.com/jordanhubbard/agenticorp/internal/temporal"
	temporalactivities "github.com/jordanhubbard/agenticorp/internal/temporal/activities"
	"github.com/jordanhubbard/agenticorp/internal/temporal/eventbus"
	"github.com/jordanhubbard/agenticorp/internal/temporal/workflows"
	"github.com/jordanhubbard/agenticorp/pkg/config"
	"github.com/jordanhubbard/agenticorp/pkg/models"
)

// AgentiCorp is the main orchestrator
type AgentiCorp struct {
	config           *config.Config
	agentManager     *agent.WorkerManager
	projectManager   *project.Manager
	personaManager   *persona.Manager
	beadsManager     *beads.Manager
	decisionManager  *decision.Manager
	fileLockManager  *FileLockManager
	orgChartManager  *orgchart.Manager
	providerRegistry *provider.Registry
	database         *database.Database
	dispatcher       *dispatch.Dispatcher
	eventBus         *eventbus.EventBus
	temporalManager  *temporal.Manager
	modelCatalog     *modelcatalog.Catalog
	gitopsManager    *gitops.Manager
	shellExecutor    *executor.ShellExecutor
}

// New creates a new AgentiCorp instance
func New(cfg *config.Config) (*AgentiCorp, error) {
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

	modelCatalog := modelcatalog.DefaultCatalog()
	if db != nil {
		if raw, ok, err := db.GetConfigValue(modelCatalogKey); err == nil && ok {
			var specs []internalmodels.ModelSpec
			if err := json.Unmarshal([]byte(raw), &specs); err == nil && len(specs) > 0 {
				modelCatalog.Replace(specs)
			}
		}
	}

	// Initialize gitops manager for project repository management
	gitWorkDir := cfg.Projects[0].BeadsPath
	if gitWorkDir == "" {
		gitWorkDir = "/app/src"
	} else {
		// Use parent directory of beads path as work directory base
		gitWorkDir = filepath.Join(filepath.Dir(gitWorkDir), "src")
	}
	gitopsMgr, err := gitops.NewManager(gitWorkDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gitops manager: %w", err)
	}

	agentMgr := agent.NewWorkerManager(cfg.Agents.MaxConcurrent, providerRegistry, eb)
	if db != nil {
		agentMgr.SetAgentPersister(db)
	}

	// Initialize shell executor if database is available
	var shellExec *executor.ShellExecutor
	if db != nil {
		shellExec = executor.NewShellExecutor(db.DB())
	}

	arb := &AgentiCorp{
		config:           cfg,
		agentManager:     agentMgr,
		projectManager:   project.NewManager(),
		personaManager:   persona.NewManager(personaPath),
		beadsManager:     beads.NewManager(cfg.Beads.BDPath),
		decisionManager:  decision.NewManager(),
		fileLockManager:  NewFileLockManager(cfg.Agents.FileLockTimeout),
		orgChartManager:  orgchart.NewManager(),
		providerRegistry: providerRegistry,
		database:         db,
		eventBus:         eb,
		temporalManager:  temporalMgr,
		modelCatalog:     modelCatalog,
		gitopsManager:    gitopsMgr,
		shellExecutor:    shellExec,
	}

	arb.dispatcher = dispatch.NewDispatcher(arb.beadsManager, arb.projectManager, arb.agentManager, arb.providerRegistry, eb)

	// Setup provider metrics tracking
	arb.setupProviderMetrics()

	return arb, nil
}

// Initialize sets up the agenticorp
func (a *AgentiCorp) Initialize(ctx context.Context) error {
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
					ID:          p.ID,
					Name:        p.Name,
					GitRepo:     p.GitRepo,
					Branch:      p.Branch,
					BeadsPath:   p.BeadsPath,
					IsPerpetual: p.IsPerpetual,
					IsSticky:    p.IsSticky,
					Context:     p.Context,
					Status:      models.ProjectStatusOpen,
				}
				_ = a.database.UpsertProject(proj)
				projects = append(projects, proj)
			}
		} else {
			// Bootstrap from config.yaml into the configuration database.
			for _, p := range a.config.Projects {
				proj := &models.Project{
					ID:          p.ID,
					Name:        p.Name,
					GitRepo:     p.GitRepo,
					Branch:      p.Branch,
					BeadsPath:   p.BeadsPath,
					IsPerpetual: p.IsPerpetual,
					IsSticky:    p.IsSticky,
					Context:     p.Context,
					Status:      models.ProjectStatusOpen,
				}
				_ = a.database.UpsertProject(proj)
				projects = append(projects, proj)
			}
		}
	} else {
		for _, p := range a.config.Projects {
			projects = append(projects, &models.Project{
				ID:          p.ID,
				Name:        p.Name,
				GitRepo:     p.GitRepo,
				Branch:      p.Branch,
				BeadsPath:   p.BeadsPath,
				IsPerpetual: p.IsPerpetual,
				IsSticky:    p.IsSticky,
				Context:     p.Context,
				Status:      models.ProjectStatusOpen,
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
		projectValues = append(projectValues, copy)
	}
	if len(projectValues) == 0 && len(a.config.Projects) > 0 {
		for _, p := range a.config.Projects {
			projectValues = append(projectValues, models.Project{
				ID:          p.ID,
				Name:        p.Name,
				GitRepo:     p.GitRepo,
				Branch:      p.Branch,
				BeadsPath:   normalizeBeadsPath(p.BeadsPath),
				IsPerpetual: p.IsPerpetual,
				IsSticky:    p.IsSticky,
				Context:     p.Context,
				Status:      models.ProjectStatusOpen,
			})
		}
	}
	if len(projectValues) == 0 {
		projectValues = append(projectValues, models.Project{
			ID:          "agenticorp",
			Name:        "AgentiCorp",
			GitRepo:     ".",
			Branch:      "main",
			BeadsPath:   normalizeBeadsPath(".beads"),
			IsPerpetual: true,
			IsSticky:    true,
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

		// For projects with git repositories, clone/pull them first
		if p.GitRepo != "" && p.GitRepo != "." {
			// Set default auth method if not specified
			if p.GitAuthMethod == "" {
				p.GitAuthMethod = models.GitAuthNone // Default to no auth for public repos
			}

			// Check if already cloned
			workDir := a.gitopsManager.GetProjectWorkDir(p.ID)
			if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
				// Clone the repository
				fmt.Printf("Cloning project %s from %s...\n", p.ID, p.GitRepo)
				if err := a.gitopsManager.CloneProject(ctx, p); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to clone project %s: %v\n", p.ID, err)
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

			// Update project in database with git metadata
			if a.database != nil {
				_ = a.database.UpsertProject(p)
			}

			// Load beads from the cloned repository
			beadsPath := filepath.Join(workDir, p.BeadsPath, "beads")
			a.beadsManager.SetBeadsPath(beadsPath)
			_ = a.beadsManager.LoadBeadsFromFilesystem(beadsPath)
		} else {
			// Local project (AgentiCorp itself), load beads directly
			a.beadsManager.SetBeadsPath(p.BeadsPath)
			_ = a.beadsManager.LoadBeadsFromFilesystem(p.BeadsPath)
		}
	}

	// Load providers from database into the in-memory registry.
	if a.database != nil {
		providers, err := a.database.ListProviders()
		if err != nil {
			return fmt.Errorf("failed to load providers: %w", err)
		}
		// Bootstrap a mock provider when none are configured to keep the system runnable.
		if len(providers) == 0 {
			mock := &internalmodels.Provider{
				ID:              "mock-local",
				Name:            "Local Mock Provider",
				Type:            "mock",
				Endpoint:        "mock://local",
				Status:          "active",
				ConfiguredModel: "mock-model",
				SelectedModel:   "mock-model",
				Model:           "mock-model",
				LastHeartbeatAt: time.Now(),
			}
			_ = a.database.UpsertProvider(mock)
			providers = append(providers, mock)
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

	// Ensure default agents are assigned for each project.
	for _, p := range projectValues {
		if p.ID == "" {
			continue
		}
		_ = a.ensureDefaultAgents(ctx, p.ID)
	}

	// Register dispatch activities and start the Temporal worker if configured.

	// Start Temporal worker if configured
	if a.temporalManager != nil {
		a.temporalManager.RegisterActivity(temporalactivities.NewDispatchActivities(a.dispatcher))
		a.temporalManager.RegisterActivity(temporalactivities.NewProviderActivities(a.providerRegistry, a.database, a.eventBus, a.modelCatalog))
		a.temporalManager.RegisterActivity(temporalactivities.NewAgentiCorpActivities(a.database))

		if err := a.temporalManager.Start(); err != nil {
			return fmt.Errorf("failed to start temporal: %w", err)
		}

		// Start the master heartbeat (10 second interval) - timing/coordination
		_ = a.temporalManager.StartAgentiCorpHeartbeatWorkflow(ctx, 10*time.Second)
		// Start the dispatcher (triggers work distribution)
		_ = a.temporalManager.StartDispatcherWorkflow(ctx, "", 5*time.Second)
		// Start provider heartbeats (monitor provider health)
		_ = a.startProviderHeartbeats(ctx)
	}

	// Kick-start work on all open beads across registered projects.
	a.kickstartOpenBeads(ctx)

	return nil
}

// kickstartOpenBeads starts Temporal workflows for all open beads in registered projects.
// This ensures that when AgentiCorp starts (or restarts), all pending work is queued for processing.
func (a *AgentiCorp) kickstartOpenBeads(ctx context.Context) {
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

// Shutdown gracefully shuts down the agenticorp
func (a *AgentiCorp) Shutdown() {
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
func (a *AgentiCorp) GetTemporalManager() *temporal.Manager {
	return a.temporalManager
}

func (a *AgentiCorp) GetEventBus() *eventbus.EventBus {
	return a.eventBus
}

// GetDatabase returns the database instance
func (a *AgentiCorp) GetDatabase() *database.Database {
	return a.database
}

// ExecuteShellCommand executes a shell command and logs it
func (a *AgentiCorp) ExecuteShellCommand(ctx context.Context, req executor.ExecuteCommandRequest) (*executor.ExecuteCommandResult, error) {
	if a.shellExecutor == nil {
		return nil, fmt.Errorf("shell executor not available (database not configured)")
	}
	return a.shellExecutor.ExecuteCommand(ctx, req)
}

// GetCommandLogs retrieves command logs with filters
func (a *AgentiCorp) GetCommandLogs(filters map[string]interface{}, limit int) ([]*models.CommandLog, error) {
	if a.shellExecutor == nil {
		return nil, fmt.Errorf("shell executor not available (database not configured)")
	}
	return a.shellExecutor.GetCommandLogs(filters, limit)
}

// GetCommandLog retrieves a single command log by ID
func (a *AgentiCorp) GetCommandLog(id string) (*models.CommandLog, error) {
	if a.shellExecutor == nil {
		return nil, fmt.Errorf("shell executor not available (database not configured)")
	}
	return a.shellExecutor.GetCommandLog(id)
}

// GetAgentManager returns the agent manager
func (a *AgentiCorp) GetAgentManager() *agent.WorkerManager {
	return a.agentManager
}

func (a *AgentiCorp) GetProviderRegistry() *provider.Registry {
	return a.providerRegistry
}

func (a *AgentiCorp) GetDispatcher() *dispatch.Dispatcher {
	return a.dispatcher
}

// GetProjectManager returns the project manager
func (a *AgentiCorp) GetProjectManager() *project.Manager {
	return a.projectManager
}

// GetPersonaManager returns the persona manager
func (a *AgentiCorp) GetPersonaManager() *persona.Manager {
	return a.personaManager
}

// GetBeadsManager returns the beads manager
func (a *AgentiCorp) GetBeadsManager() *beads.Manager {
	return a.beadsManager
}

// GetDecisionManager returns the decision manager
func (a *AgentiCorp) GetDecisionManager() *decision.Manager {
	return a.decisionManager
}

// GetOrgChartManager returns the org chart manager
func (a *AgentiCorp) GetOrgChartManager() *orgchart.Manager {
	return a.orgChartManager
}

// Project management helpers

func (a *AgentiCorp) CreateProject(name, gitRepo, branch, beadsPath string, ctxMap map[string]string) (*models.Project, error) {
	p, err := a.projectManager.CreateProject(name, gitRepo, branch, beadsPath, ctxMap)
	if err != nil {
		return nil, err
	}
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

func (a *AgentiCorp) ensureDefaultAgents(ctx context.Context, projectID string) error {
	return a.ensureOrgChart(ctx, projectID)
}

// ensureOrgChart creates an org chart for a project and fills all positions with agents
func (a *AgentiCorp) ensureOrgChart(ctx context.Context, projectID string) error {
	project, err := a.projectManager.GetProject(projectID)
	if err != nil {
		return err
	}

	// Create or get the org chart for this project
	chart, err := a.orgChartManager.CreateForProject(projectID, project.Name)
	if err != nil {
		return err
	}

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
		if agentID, ok := existingByRole[pos.RoleName]; ok {
			if !pos.HasAgent(agentID) && pos.CanAddAgent() {
				pos.AgentIDs = append(pos.AgentIDs, agentID)
			}
		}
	}

	// Create agents for ALL positions that are still vacant (agents start paused without a provider)
	for _, pos := range chart.Positions {
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
		// Container mount path (source mounted at /app/src)
		filepath.Join("/app/src", trimmed),
		// Relative path with dot prefix
		"." + strings.TrimPrefix(trimmed, "/"),
		filepath.Join("/app/src", "."+strings.TrimPrefix(trimmed, "/")),
		// Fallbacks
		".beads",
		"/app/src/.beads",
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
	beadsDir := filepath.Join(path, "beads")
	if _, err := os.Stat(beadsDir); err == nil {
		return true
	}
	return false
}

// CloneAgentPersona clones a default persona into a project-specific persona and spawns a new agent.
func (a *AgentiCorp) CloneAgentPersona(ctx context.Context, agentID, newPersonaName, newAgentName, sourcePersona string, replace bool) (*models.Agent, error) {
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
func (a *AgentiCorp) AssignAgentToProject(agentID, projectID string) error {
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
func (a *AgentiCorp) UnassignAgentFromProject(agentID, projectID string) error {
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

func (a *AgentiCorp) PersistProject(projectID string) {
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

func (a *AgentiCorp) DeleteProject(projectID string) error {
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
func (a *AgentiCorp) CreateAgent(ctx context.Context, name, personaName, projectID, role string) (*models.Agent, error) {
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
		_ = a.database.UpsertAgent(agent)
	}

	return agent, nil
}

func (a *AgentiCorp) SpawnAgent(ctx context.Context, name, personaName, projectID string, providerID string) (*models.Agent, error) {
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
func (a *AgentiCorp) StopAgent(ctx context.Context, agentID string) error {
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

func (a *AgentiCorp) ListProviders() ([]*internalmodels.Provider, error) {
	if a.database == nil {
		return []*internalmodels.Provider{}, nil
	}
	return a.database.ListProviders()
}

func (a *AgentiCorp) RegisterProvider(ctx context.Context, p *internalmodels.Provider) (*internalmodels.Provider, error) {
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

func (a *AgentiCorp) UpdateProvider(ctx context.Context, p *internalmodels.Provider) (*internalmodels.Provider, error) {
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

func (a *AgentiCorp) DeleteProvider(ctx context.Context, providerID string) error {
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

func (a *AgentiCorp) GetProviderModels(ctx context.Context, providerID string) ([]provider.Model, error) {
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
func (a *AgentiCorp) RunReplQuery(ctx context.Context, message string) (*ReplResult, error) {
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
	
	bead, err := a.beadsManager.CreateBead(beadTitle, cleanMessage, models.BeadPriorityP0, "task", "agenticorp-self")
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
				"source": "ceo-repl",
				"created_by": "ceo",
			},
		})
	}

	providerRecord, err := a.selectBestProviderForRepl()
	if err != nil {
		return nil, err
	}

	systemPrompt := a.buildAgentiCorpPersonaPrompt()
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
					"source": "ceo-repl",
					"created_by": "ceo",
					"error": err.Error(),
				},
			})
		}
		return nil, err
	}

	// Update bead with response
	if beadID != "" {
		_ = a.beadsManager.UpdateBead(beadID, map[string]interface{}{
			"context": map[string]string{
				"source": "ceo-repl",
				"created_by": "ceo",
				"response": result.Response,
				"provider_id": providerRecord.ID,
				"model": result.Model,
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

func (a *AgentiCorp) selectBestProviderForRepl() (*internalmodels.Provider, error) {
	return a.SelectProvider(context.Background(), nil, "balanced")
}

// SelectProvider chooses the best provider based on policy and requirements
func (a *AgentiCorp) SelectProvider(ctx context.Context, requirements *routing.ProviderRequirements, policy string) (*internalmodels.Provider, error) {
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

func (a *AgentiCorp) scoreProviderForRepl(p *internalmodels.Provider) float64 {
	quality := p.ModelScore
	modelName := p.SelectedModel
	if modelName == "" {
		modelName = p.Model
	}
	if modelName == "" {
		modelName = p.ConfiguredModel
	}
	if quality == 0 && a.modelCatalog != nil && modelName != "" {
		spec := modelcatalog.ParseModelName(modelName)
		spec.Name = modelName
		quality = a.modelCatalog.Score(spec)
	}
	latency := p.LastHeartbeatLatencyMs
	if latency <= 0 {
		latency = 120000
	}
	return (quality * 1000) - float64(latency)
}

func providerIsHealthy(status string) bool {
	switch status {
	case "healthy", "active":
		return true
	default:
		return false
	}
}

func (a *AgentiCorp) buildAgentiCorpPersonaPrompt() string {
	persona, err := a.personaManager.LoadPersona("agenticorp")
	if err != nil {
		return "You are AgentiCorp, the orchestration system. Respond to the CEO with clear guidance and actionable next steps."
	}

	focus := strings.Join(persona.FocusAreas, ", ")
	standards := strings.Join(persona.Standards, "; ")

	return fmt.Sprintf(
		"You are AgentiCorp, the orchestration system. Treat this as a high-priority CEO request.\n\nMission: %s\nCharacter: %s\nTone: %s\nFocus Areas: %s\nDecision Making: %s\nStandards: %s",
		strings.TrimSpace(persona.Mission),
		strings.TrimSpace(persona.Character),
		strings.TrimSpace(persona.Tone),
		strings.TrimSpace(focus),
		strings.TrimSpace(persona.DecisionMaking),
		strings.TrimSpace(standards),
	)
}

func (a *AgentiCorp) startProviderHeartbeats(ctx context.Context) error {
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

func (a *AgentiCorp) ensureProviderHeartbeat(ctx context.Context, providerID string) error {
	if a.temporalManager == nil || providerID == "" {
		return nil
	}
	return a.temporalManager.StartProviderHeartbeatWorkflow(ctx, providerID, 30*time.Second)
}

// NegotiateProviderModel selects the best available model from the catalog for a provider.
func (a *AgentiCorp) NegotiateProviderModel(ctx context.Context, providerID string) (*internalmodels.Provider, error) {
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
func (a *AgentiCorp) ListModelCatalog() []internalmodels.ModelSpec {
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
func (a *AgentiCorp) RequestFileAccess(projectID, filePath, agentID, beadID string) (*models.FileLock, error) {
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
func (a *AgentiCorp) ReleaseFileAccess(projectID, filePath, agentID string) error {
	return a.fileLockManager.ReleaseLock(projectID, filePath, agentID)
}

// CreateBead creates a new work bead
func (a *AgentiCorp) CreateBead(title, description string, priority models.BeadPriority, beadType, projectID string) (*models.Bead, error) {
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

// CreateDecisionBead creates a decision bead when an agent needs a decision
func (a *AgentiCorp) CreateDecisionBead(question, parentBeadID, requesterID string, options []string, recommendation string, priority models.BeadPriority, projectID string) (*models.DecisionBead, error) {
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
func (a *AgentiCorp) MakeDecision(decisionID, deciderID, decisionText, rationale string) error {
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

func (a *AgentiCorp) EscalateBeadToCEO(beadID, reason, returnedTo string) (*models.DecisionBead, error) {
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

func (a *AgentiCorp) applyCEODecisionToParent(decisionID string) error {
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
func (a *AgentiCorp) UnblockDependents(decisionID string) error {
	blocked := a.decisionManager.GetBlockedBeads(decisionID)

	for _, beadID := range blocked {
		if err := a.beadsManager.UnblockBead(beadID, decisionID); err != nil {
			return fmt.Errorf("failed to unblock bead %s: %w", beadID, err)
		}
	}

	return nil
}

// ClaimBead assigns a bead to an agent
func (a *AgentiCorp) ClaimBead(beadID, agentID string) error {
	// Verify agent exists
	if _, err := a.agentManager.GetAgent(agentID); err != nil {
		return fmt.Errorf("agent not found: %w", err)
	}

	// Claim the bead
	if err := a.beadsManager.ClaimBead(beadID, agentID); err != nil {
		return fmt.Errorf("failed to claim bead: %w", err)
	}

	// Update agent status
	if err := a.agentManager.AssignBead(agentID, beadID); err != nil {
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

	return nil
}

// UpdateBead updates a bead and publishes relevant events.
func (a *AgentiCorp) UpdateBead(beadID string, updates map[string]interface{}) (*models.Bead, error) {
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
func (a *AgentiCorp) GetReadyBeads(projectID string) ([]*models.Bead, error) {
	return a.beadsManager.GetReadyBeads(projectID)
}

// GetWorkGraph returns the dependency graph of beads
func (a *AgentiCorp) GetWorkGraph(projectID string) (*models.WorkGraph, error) {
	return a.beadsManager.GetWorkGraph(projectID)
}

// GetFileLockManager returns the file lock manager
func (a *AgentiCorp) GetFileLockManager() *FileLockManager {
	return a.fileLockManager
}

// GetGitopsManager returns the gitops manager
func (a *AgentiCorp) GetGitopsManager() *gitops.Manager {
	return a.gitopsManager
}

// StartMaintenanceLoop starts background maintenance tasks
func (a *AgentiCorp) StartMaintenanceLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

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

			// Auto-escalate loop-detected beads to CEO (best-effort).
			beads, _ := a.beadsManager.ListBeads(nil)
			for _, b := range beads {
				if b == nil || b.Context == nil {
					continue
				}
				if b.Context["loop_detected"] != "true" {
					continue
				}
				if b.Context["escalated_to_ceo_decision_id"] != "" {
					continue
				}
				reason := b.Context["loop_detected_reason"]
				returnedTo := b.Context["agent_id"]
				_, _ = a.EscalateBeadToCEO(b.ID, reason, returnedTo)
			}
		}
	}
}

// StartDispatchLoop runs a best-effort periodic dispatcher loop when Temporal is not configured.
func (a *AgentiCorp) StartDispatchLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = a.dispatcher.DispatchOnce(ctx, "")
		}
	}
}

// checkProviderHealthAndActivate checks if a newly registered provider has models available
// and immediately activates it if so, without waiting for the heartbeat workflow
func (a *AgentiCorp) checkProviderHealthAndActivate(providerID string) {
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
		a.providerRegistry.Upsert(&provider.ProviderConfig{
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

// TODO: Implement perpetual tasks for each org chart role:
// - CFO: Monthly budget reviews, financial reporting
// - PR Manager: Poll GitHub for new issues/PRs
// - Documentation Manager: Automated documentation updates
// - QA Engineer: Run automated test suites
// These will be created as beads by the dispatcher when in idle mode

// ResumeAgentsWaitingForProvider resumes agents that were paused waiting for a provider to become healthy
func (a *AgentiCorp) ResumeAgentsWaitingForProvider(ctx context.Context, providerID string) error {
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
		if err := a.database.UpsertAgent(agent); err != nil {
			fmt.Printf("Warning: failed to resume agent %s: %v\n", agent.ID, err)
			continue
		}
	}

	// Trigger dispatch to pick up any waiting beads
	if a.dispatcher != nil {
		_, _ = a.dispatcher.DispatchOnce(ctx, "")
	}

	return nil
}

// attachProviderToPausedAgents assigns a newly active provider to any paused agents that lack one.
func (a *AgentiCorp) attachProviderToPausedAgents(ctx context.Context, providerID string) {
	if a.agentManager == nil || a.database == nil || providerID == "" {
		return
	}

	if !a.providerRegistry.IsActive(providerID) {
		return
	}

	agents, err := a.database.ListAgents()
	if err != nil {
		return
	}

	for _, ag := range agents {
		if ag == nil || ag.ProviderID != "" {
			continue
		}
		// Attach persona for prompt context
		if ag.Persona == nil && ag.PersonaName != "" {
			ag.Persona, _ = a.personaManager.LoadPersona(ag.PersonaName)
		}
		ag.ProviderID = providerID
		ag.Status = "idle"
		_ = a.database.UpsertAgent(ag)
		if _, err := a.agentManager.RestoreAgentWorker(ctx, ag); err != nil {
			continue
		}
		if ag.ProjectID != "" {
			_ = a.projectManager.AddAgentToProject(ag.ProjectID, ag.ID)
		}
	}
}
