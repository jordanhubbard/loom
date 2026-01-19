package arbiter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jordanhubbard/arbiter/internal/agent"
	"github.com/jordanhubbard/arbiter/internal/beads"
	"github.com/jordanhubbard/arbiter/internal/database"
	"github.com/jordanhubbard/arbiter/internal/decision"
	"github.com/jordanhubbard/arbiter/internal/dispatch"
	internalmodels "github.com/jordanhubbard/arbiter/internal/models"
	"github.com/jordanhubbard/arbiter/internal/persona"
	"github.com/jordanhubbard/arbiter/internal/project"
	"github.com/jordanhubbard/arbiter/internal/provider"
	"github.com/jordanhubbard/arbiter/internal/temporal"
	temporalactivities "github.com/jordanhubbard/arbiter/internal/temporal/activities"
	"github.com/jordanhubbard/arbiter/internal/temporal/eventbus"
	"github.com/jordanhubbard/arbiter/pkg/config"
	"github.com/jordanhubbard/arbiter/pkg/models"
)

// Arbiter is the main orchestrator
type Arbiter struct {
	config           *config.Config
	agentManager     *agent.WorkerManager
	projectManager   *project.Manager
	personaManager   *persona.Manager
	beadsManager     *beads.Manager
	decisionManager  *decision.Manager
	fileLockManager  *FileLockManager
	providerRegistry *provider.Registry
	database         *database.Database
	dispatcher       *dispatch.Dispatcher
	eventBus         *eventbus.EventBus
	temporalManager  *temporal.Manager
}

// New creates a new Arbiter instance
func New(cfg *config.Config) (*Arbiter, error) {
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
	}

	agentMgr := agent.NewWorkerManager(cfg.Agents.MaxConcurrent, providerRegistry, eb)
	if db != nil {
		agentMgr.SetAgentPersister(db)
	}

	arb := &Arbiter{
		config:           cfg,
		agentManager:     agentMgr,
		projectManager:   project.NewManager(),
		personaManager:   persona.NewManager(personaPath),
		beadsManager:     beads.NewManager(cfg.Beads.BDPath),
		decisionManager:  decision.NewManager(),
		fileLockManager:  NewFileLockManager(cfg.Agents.FileLockTimeout),
		providerRegistry: providerRegistry,
		database:         db,
		eventBus:         eb,
		temporalManager:  temporalMgr,
	}

	arb.dispatcher = dispatch.NewDispatcher(arb.beadsManager, arb.projectManager, arb.agentManager, arb.providerRegistry, eb)
	return arb, nil
}

// Initialize sets up the arbiter
func (a *Arbiter) Initialize(ctx context.Context) error {
	// Prefer database-backed configuration when available.
	var projects []*models.Project
	if a.database != nil {
		storedProjects, err := a.database.ListProjects()
		if err != nil {
			return fmt.Errorf("failed to load projects: %w", err)
		}
		if len(storedProjects) > 0 {
			projects = storedProjects
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
		projectValues = append(projectValues, *p)
	}
	if err := a.projectManager.LoadProjects(projectValues); err != nil {
		return fmt.Errorf("failed to load projects: %w", err)
	}

	// Load beads from registered projects.
	for _, p := range projectValues {
		if p.BeadsPath == "" {
			continue
		}
		a.beadsManager.SetBeadsPath(p.BeadsPath)
		_ = a.beadsManager.LoadBeadsFromFilesystem(p.BeadsPath)
	}

	// Load providers from database into the in-memory registry.
	if a.database != nil {
		providers, err := a.database.ListProviders()
		if err != nil {
			return fmt.Errorf("failed to load providers: %w", err)
		}
		for _, p := range providers {
			_ = a.providerRegistry.Upsert(&provider.ProviderConfig{
				ID:       p.ID,
				Name:     p.Name,
				Type:     p.Type,
				Endpoint: normalizeProviderEndpoint(p.Endpoint),
				APIKey:   "",
				Model:    p.Model,
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
				providers := a.providerRegistry.List()
				if len(providers) == 0 {
					continue
				}
				ag.ProviderID = providers[0].Config.ID
			}
			_, _ = a.agentManager.RestoreAgentWorker(ctx, ag)
			_ = a.projectManager.AddAgentToProject(ag.ProjectID, ag.ID)
		}
	}

	// Register dispatch activities and start the Temporal worker if configured.

	// Start Temporal worker if configured
	if a.temporalManager != nil {
		a.temporalManager.RegisterActivity(temporalactivities.NewDispatchActivities(a.dispatcher))
		if err := a.temporalManager.Start(); err != nil {
			return fmt.Errorf("failed to start temporal: %w", err)
		}

		// Start the Temporal-controlled dispatch loop for all projects.
		_ = a.temporalManager.StartDispatcherWorkflow(ctx, "", 10*time.Second)
	}

	return nil
}

// Shutdown gracefully shuts down the arbiter
func (a *Arbiter) Shutdown() {
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
func (a *Arbiter) GetTemporalManager() *temporal.Manager {
	return a.temporalManager
}

func (a *Arbiter) GetEventBus() *eventbus.EventBus {
	return a.eventBus
}

// GetAgentManager returns the agent manager
func (a *Arbiter) GetAgentManager() *agent.WorkerManager {
	return a.agentManager
}

func (a *Arbiter) GetProviderRegistry() *provider.Registry {
	return a.providerRegistry
}

func (a *Arbiter) GetDispatcher() *dispatch.Dispatcher {
	return a.dispatcher
}

// GetProjectManager returns the project manager
func (a *Arbiter) GetProjectManager() *project.Manager {
	return a.projectManager
}

// GetPersonaManager returns the persona manager
func (a *Arbiter) GetPersonaManager() *persona.Manager {
	return a.personaManager
}

// GetBeadsManager returns the beads manager
func (a *Arbiter) GetBeadsManager() *beads.Manager {
	return a.beadsManager
}

// GetDecisionManager returns the decision manager
func (a *Arbiter) GetDecisionManager() *decision.Manager {
	return a.decisionManager
}

// Project management helpers

func (a *Arbiter) CreateProject(name, gitRepo, branch, beadsPath string, context map[string]string) (*models.Project, error) {
	p, err := a.projectManager.CreateProject(name, gitRepo, branch, beadsPath, context)
	if err != nil {
		return nil, err
	}
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

func (a *Arbiter) PersistProject(projectID string) {
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

func (a *Arbiter) DeleteProject(projectID string) error {
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
func (a *Arbiter) SpawnAgent(ctx context.Context, name, personaName, projectID string, providerID string) (*models.Agent, error) {
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
		providers := a.providerRegistry.List()
		if len(providers) == 0 {
			return nil, fmt.Errorf("no providers registered")
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
func (a *Arbiter) StopAgent(ctx context.Context, agentID string) error {
	ag, err := a.agentManager.GetAgent(agentID)
	if err != nil {
		return err
	}

	if err := a.agentManager.StopAgent(agentID); err != nil {
		return err
	}
	_ = a.fileLockManager.ReleaseAgentLocks(agentID)
	_ = a.projectManager.RemoveAgentFromProject(ag.ProjectID, ag.ID)
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

func (a *Arbiter) ListProviders() ([]*internalmodels.Provider, error) {
	if a.database == nil {
		return []*internalmodels.Provider{}, nil
	}
	return a.database.ListProviders()
}

func (a *Arbiter) RegisterProvider(ctx context.Context, p *internalmodels.Provider) (*internalmodels.Provider, error) {
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
		p.Status = "active"
	}
	p.Endpoint = normalizeProviderEndpoint(p.Endpoint)
	if p.Model == "" {
		p.Model = "NVIDIA-Nemotron-3-Nano-30B-A3B-BF16"
	}

	if err := a.database.UpsertProvider(p); err != nil {
		return nil, err
	}

	_ = a.providerRegistry.Upsert(&provider.ProviderConfig{
		ID:       p.ID,
		Name:     p.Name,
		Type:     p.Type,
		Endpoint: p.Endpoint,
		Model:    p.Model,
	})
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:   eventbus.EventTypeProviderRegistered,
			Source: "provider-manager",
			Data: map[string]interface{}{
				"provider_id": p.ID,
				"name":        p.Name,
				"endpoint":    p.Endpoint,
				"model":       p.Model,
			},
		})
	}

	return p, nil
}

func (a *Arbiter) UpdateProvider(ctx context.Context, p *internalmodels.Provider) (*internalmodels.Provider, error) {
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
		p.Status = "active"
	}
	p.Endpoint = normalizeProviderEndpoint(p.Endpoint)
	if p.Model == "" {
		p.Model = "NVIDIA-Nemotron-3-Nano-30B-A3B-BF16"
	}

	if err := a.database.UpsertProvider(p); err != nil {
		return nil, err
	}

	_ = a.providerRegistry.Upsert(&provider.ProviderConfig{
		ID:       p.ID,
		Name:     p.Name,
		Type:     p.Type,
		Endpoint: p.Endpoint,
		Model:    p.Model,
	})
	if a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:   eventbus.EventTypeProviderUpdated,
			Source: "provider-manager",
			Data: map[string]interface{}{
				"provider_id": p.ID,
				"name":        p.Name,
				"endpoint":    p.Endpoint,
				"model":       p.Model,
			},
		})
	}

	return p, nil
}

func (a *Arbiter) DeleteProvider(ctx context.Context, providerID string) error {
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

func (a *Arbiter) GetProviderModels(ctx context.Context, providerID string) ([]provider.Model, error) {
	return a.providerRegistry.GetModels(ctx, providerID)
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
func (a *Arbiter) RequestFileAccess(projectID, filePath, agentID, beadID string) (*models.FileLock, error) {
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
func (a *Arbiter) ReleaseFileAccess(projectID, filePath, agentID string) error {
	return a.fileLockManager.ReleaseLock(projectID, filePath, agentID)
}

// CreateBead creates a new work bead
func (a *Arbiter) CreateBead(title, description string, priority models.BeadPriority, beadType, projectID string) (*models.Bead, error) {
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
func (a *Arbiter) CreateDecisionBead(question, parentBeadID, requesterID string, options []string, recommendation string, priority models.BeadPriority, projectID string) (*models.DecisionBead, error) {
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
func (a *Arbiter) MakeDecision(decisionID, deciderID, decisionText, rationale string) error {
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

func (a *Arbiter) EscalateBeadToCEO(beadID, reason, returnedTo string) (*models.DecisionBead, error) {
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

func (a *Arbiter) applyCEODecisionToParent(decisionID string) error {
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
func (a *Arbiter) UnblockDependents(decisionID string) error {
	blocked := a.decisionManager.GetBlockedBeads(decisionID)

	for _, beadID := range blocked {
		if err := a.beadsManager.UnblockBead(beadID, decisionID); err != nil {
			return fmt.Errorf("failed to unblock bead %s: %w", beadID, err)
		}
	}

	return nil
}

// ClaimBead assigns a bead to an agent
func (a *Arbiter) ClaimBead(beadID, agentID string) error {
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
func (a *Arbiter) UpdateBead(beadID string, updates map[string]interface{}) (*models.Bead, error) {
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
func (a *Arbiter) GetReadyBeads(projectID string) ([]*models.Bead, error) {
	return a.beadsManager.GetReadyBeads(projectID)
}

// GetWorkGraph returns the dependency graph of beads
func (a *Arbiter) GetWorkGraph(projectID string) (*models.WorkGraph, error) {
	return a.beadsManager.GetWorkGraph(projectID)
}

// GetFileLockManager returns the file lock manager
func (a *Arbiter) GetFileLockManager() *FileLockManager {
	return a.fileLockManager
}

// StartMaintenanceLoop starts background maintenance tasks
func (a *Arbiter) StartMaintenanceLoop(ctx context.Context) {
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
func (a *Arbiter) StartDispatchLoop(ctx context.Context, interval time.Duration) {
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
