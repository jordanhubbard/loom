package arbiter

import (
	"context"
	"fmt"
	"time"

	"github.com/jordanhubbard/arbiter/internal/agent"
	"github.com/jordanhubbard/arbiter/internal/beads"
	"github.com/jordanhubbard/arbiter/internal/decision"
	"github.com/jordanhubbard/arbiter/internal/persona"
	"github.com/jordanhubbard/arbiter/internal/project"
	"github.com/jordanhubbard/arbiter/pkg/config"
	"github.com/jordanhubbard/arbiter/pkg/models"
)

// Arbiter is the main orchestrator
type Arbiter struct {
	config          *config.Config
	agentManager    *agent.Manager
	projectManager  *project.Manager
	personaManager  *persona.Manager
	beadsManager    *beads.Manager
	decisionManager *decision.Manager
	fileLockManager *FileLockManager
}

// New creates a new Arbiter instance
func New(cfg *config.Config) *Arbiter {
	personaPath := cfg.Agents.DefaultPersonaPath
	if personaPath == "" {
		personaPath = "./personas"
	}
	
	return &Arbiter{
		config:          cfg,
		agentManager:    agent.NewManager(cfg.Agents.MaxConcurrent),
		projectManager:  project.NewManager(),
		personaManager:  persona.NewManager(personaPath),
		beadsManager:    beads.NewManager(cfg.Beads.BDPath),
		decisionManager: decision.NewManager(),
		fileLockManager: NewFileLockManager(cfg.Agents.FileLockTimeout),
	}
}

// Initialize sets up the arbiter
func (a *Arbiter) Initialize(ctx context.Context) error {
	// Load projects from config
	var projects []models.Project
	for _, p := range a.config.Projects {
		projects = append(projects, models.Project{
			ID:          p.ID,
			Name:        p.Name,
			GitRepo:     p.GitRepo,
			Branch:      p.Branch,
			BeadsPath:   p.BeadsPath,
			IsPerpetual: p.IsPerpetual,
			Context:     p.Context,
		})
	}
	if err := a.projectManager.LoadProjects(projects); err != nil {
		return fmt.Errorf("failed to load projects: %w", err)
	}

	return nil
}

// SpawnAgent spawns a new agent with a given persona
func (a *Arbiter) SpawnAgent(ctx context.Context, name, personaName, projectID string) (*models.Agent, error) {
	// Load persona
	persona, err := a.personaManager.LoadPersona(personaName)
	if err != nil {
		return nil, fmt.Errorf("failed to load persona: %w", err)
	}

	// Verify project exists
	if _, err := a.projectManager.GetProject(projectID); err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	// Spawn agent
	agent, err := a.agentManager.SpawnAgent(ctx, name, personaName, projectID, persona)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn agent: %w", err)
	}

	// Add agent to project
	if err := a.projectManager.AddAgentToProject(projectID, agent.ID); err != nil {
		return nil, fmt.Errorf("failed to add agent to project: %w", err)
	}

	return agent, nil
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

	return a.beadsManager.CreateBead(title, description, priority, beadType, projectID)
}

// CreateDecisionBead creates a decision bead when an agent needs a decision
func (a *Arbiter) CreateDecisionBead(question, parentBeadID, requesterID string, options []string, recommendation string, priority models.BeadPriority, projectID string) (*models.DecisionBead, error) {
	// Verify agent exists
	if _, err := a.agentManager.GetAgent(requesterID); err != nil {
		return nil, fmt.Errorf("requester agent not found: %w", err)
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

	return decision, nil
}

// MakeDecision resolves a decision bead
func (a *Arbiter) MakeDecision(decisionID, deciderID, decisionText, rationale string) error {
	// Verify decider exists (could be agent or user)
	// For users, we'll allow any decider ID starting with "user-"
	if deciderID[:5] != "user-" {
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

	return nil
}

// GetReadyBeads returns beads that are ready to work on
func (a *Arbiter) GetReadyBeads(projectID string) ([]*models.Bead, error) {
	return a.beadsManager.GetReadyBeads(projectID)
}

// GetWorkGraph returns the dependency graph of beads
func (a *Arbiter) GetWorkGraph(projectID string) (*models.WorkGraph, error) {
	return a.beadsManager.GetWorkGraph(projectID)
}

// GetAgentManager returns the agent manager
func (a *Arbiter) GetAgentManager() *agent.Manager {
	return a.agentManager
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
		}
	}
}
