package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jordanhubbard/loom/internal/actions"
	"github.com/jordanhubbard/loom/internal/analytics"
	"github.com/jordanhubbard/loom/internal/database"
	"github.com/jordanhubbard/loom/internal/observability"
	"github.com/jordanhubbard/loom/internal/provider"
	"github.com/jordanhubbard/loom/internal/temporal/eventbus"
	"github.com/jordanhubbard/loom/internal/worker"
	"github.com/jordanhubbard/loom/pkg/models"
)

// WorkerManager manages agents with worker pool integration
type WorkerManager struct {
	agents             map[string]*models.Agent
	workerPool         *worker.Pool
	providerRegistry   *provider.Registry
	eventBus           *eventbus.EventBus
	agentPersister     interface{ UpsertAgent(*models.Agent) error }
	actionRouter       *actions.Router
	analyticsLogger    *analytics.Logger
	actionLoopEnabled  bool
	maxLoopIterations  int
	lessonsProvider    worker.LessonsProvider
	db                 *database.Database
	mu                 sync.RWMutex
	maxAgents          int
}

// NewWorkerManager creates a new agent manager with worker pool
func NewWorkerManager(maxAgents int, providerRegistry *provider.Registry, eventBus *eventbus.EventBus) *WorkerManager {
	return &WorkerManager{
		agents:           make(map[string]*models.Agent),
		workerPool:       worker.NewPool(providerRegistry, maxAgents),
		providerRegistry: providerRegistry,
		eventBus:         eventBus,
		maxAgents:        maxAgents,
	}
}

func (m *WorkerManager) SetAgentPersister(p interface{ UpsertAgent(*models.Agent) error }) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agentPersister = p
}

func (m *WorkerManager) SetActionRouter(r *actions.Router) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actionRouter = r
}

func (m *WorkerManager) SetAnalyticsLogger(l *analytics.Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.analyticsLogger = l
}

func (m *WorkerManager) SetActionLoopEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.actionLoopEnabled = enabled
}

func (m *WorkerManager) SetMaxLoopIterations(max int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxLoopIterations = max
}

func (m *WorkerManager) SetLessonsProvider(lp worker.LessonsProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lessonsProvider = lp
}

func (m *WorkerManager) SetDatabase(db *database.Database) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.db = db
}

func (m *WorkerManager) persistAgent(agent *models.Agent) {
	if agent == nil {
		return
	}

	// agentPersister is set once during startup; avoid locking here to prevent deadlocks.
	p := m.agentPersister
	if p == nil {
		return
	}

	copy := *agent
	copy.Persona = nil
	_ = p.UpsertAgent(&copy)
}

// CreateAgent creates an agent without a worker (paused state until provider available)
func (m *WorkerManager) CreateAgent(ctx context.Context, name, personaName, projectID, role string, persona *models.Persona) (*models.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we've reached max agents
	if len(m.agents) >= m.maxAgents {
		return nil, fmt.Errorf("maximum number of agents (%d) reached", m.maxAgents)
	}

	// Generate agent ID
	agentID := fmt.Sprintf("agent-%d-%s", time.Now().Unix(), name)

	// Use persona name as agent name if custom name not provided
	if name == "" {
		name = personaName
	}

	// Derive role if not provided
	if role == "" {
		role = deriveRoleFromPersonaName(personaName)
	}

	agent := &models.Agent{
		ID:          agentID,
		Name:        name,
		Role:        role,
		PersonaName: personaName,
		Persona:     persona,
		ProviderID:  "", // No provider yet - agent is paused
		Status:      "paused",
		ProjectID:   projectID,
		StartedAt:   time.Now(),
		LastActive:  time.Now(),
	}

	m.agents[agentID] = agent
	m.persistAgent(agent)

	log.Printf("Created paused agent %s (role: %s) - waiting for provider", agent.Name, role)
	if m.eventBus != nil {
		_ = m.eventBus.PublishAgentEvent(eventbus.EventTypeAgentSpawned, agent.ID, projectID, map[string]interface{}{
			"name":         agent.Name,
			"role":         role,
			"persona_name": personaName,
			"status":       "paused",
		})
	}

	return agent, nil
}

// SpawnAgentWorker creates and starts a new agent with a worker
func (m *WorkerManager) SpawnAgentWorker(ctx context.Context, name, personaName, projectID, providerID string, persona *models.Persona) (*models.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we've reached max agents
	if len(m.agents) >= m.maxAgents {
		return nil, fmt.Errorf("maximum number of agents (%d) reached", m.maxAgents)
	}

	// Generate agent ID
	agentID := fmt.Sprintf("agent-%d-%s", time.Now().Unix(), name)

	// Use persona name as agent name if custom name not provided
	if name == "" {
		name = personaName
	}

	agent := &models.Agent{
		ID:          agentID,
		Name:        name,
		Role:        deriveRoleFromPersonaName(personaName),
		PersonaName: personaName,
		Persona:     persona,
		ProviderID:  providerID,
		Status:      "idle",
		ProjectID:   projectID,
		StartedAt:   time.Now(),
		LastActive:  time.Now(),
	}

	m.agents[agentID] = agent

	// Spawn a worker for this agent
	if _, err := m.workerPool.SpawnWorker(agent, providerID); err != nil {
		delete(m.agents, agentID)
		return nil, fmt.Errorf("failed to spawn worker: %w", err)
	}

	m.persistAgent(agent)

	log.Printf("Spawned agent %s with worker using provider %s", agent.Name, providerID)
	if m.eventBus != nil {
		_ = m.eventBus.PublishAgentEvent(eventbus.EventTypeAgentSpawned, agent.ID, projectID, map[string]interface{}{
			"name":         agent.Name,
			"persona_name": personaName,
			"provider_id":  providerID,
		})
	}

	return agent, nil
}

func (m *WorkerManager) UpdateAgentProject(id, projectID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, ok := m.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}

	agent.ProjectID = projectID
	agent.LastActive = time.Now()
	m.persistAgent(agent)

	return nil
}

func deriveRoleFromPersonaName(personaName string) string {
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

// RestoreAgentWorker restores an already-persisted agent and ensures it has a worker attached.
func (m *WorkerManager) RestoreAgentWorker(ctx context.Context, agent *models.Agent) (*models.Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if agent == nil {
		return nil, fmt.Errorf("agent cannot be nil")
	}
	if agent.Role == "" {
		agent.Role = deriveRoleFromPersonaName(agent.PersonaName)
	}

	// If agent already exists, update it and ensure worker exists
	if existing, exists := m.agents[agent.ID]; exists {
		// Update existing agent's provider and status
		existing.ProviderID = agent.ProviderID
		existing.Status = agent.Status
		existing.LastActive = time.Now()
		if existing.Persona == nil && agent.Persona != nil {
			existing.Persona = agent.Persona
		}

		// Ensure worker exists for this agent with the correct provider
		if agent.ProviderID != "" {
			if _, err := m.workerPool.SpawnWorker(existing, existing.ProviderID); err != nil {
				log.Printf("Warning: Failed to spawn/update worker for agent %s: %v", existing.ID, err)
			}
		}

		m.persistAgent(existing)
		log.Printf("Updated existing agent %s with provider %s, status %s", existing.Name, existing.ProviderID, existing.Status)
		return existing, nil
	}

	// Agent doesn't exist - create new one
	if len(m.agents) >= m.maxAgents {
		return nil, fmt.Errorf("maximum number of agents (%d) reached", m.maxAgents)
	}

	// Ensure required fields.
	if agent.Status == "" {
		agent.Status = "idle"
	}
	if agent.StartedAt.IsZero() {
		agent.StartedAt = time.Now()
	}
	if agent.LastActive.IsZero() {
		agent.LastActive = time.Now()
	}

	m.agents[agent.ID] = agent

	if _, err := m.workerPool.SpawnWorker(agent, agent.ProviderID); err != nil {
		delete(m.agents, agent.ID)
		return nil, fmt.Errorf("failed to spawn worker: %w", err)
	}

	m.persistAgent(agent)

	log.Printf("Restored agent %s with worker using provider %s", agent.Name, agent.ProviderID)
	return agent, nil
}

// GetIdleAgentsByProject returns idle agents for a given project.
func (m *WorkerManager) GetIdleAgentsByProject(projectID string) []*models.Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*models.Agent, 0)
	for _, a := range m.agents {
		// Include both "idle" and "paused" agents — paused agents are idle
		// but waiting for a provider, which the dispatcher can auto-assign.
		if a.Status != "idle" && a.Status != "paused" {
			continue
		}
		if projectID != "" && a.ProjectID != projectID {
			continue
		}
		agents = append(agents, a)
	}

	return agents
}

// ExecuteTask assigns a task to an agent's worker
func (m *WorkerManager) ExecuteTask(ctx context.Context, agentID string, task *worker.Task) (*worker.TaskResult, error) {
	m.mu.RLock()
	agent, exists := m.agents[agentID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	startTime := time.Now()
	projectID := agent.ProjectID
	taskID := ""
	beadID := ""
	if task != nil {
		if task.ProjectID != "" {
			projectID = task.ProjectID
		}
		taskID = task.ID
		beadID = task.BeadID
		observability.Info("agent.task_start", map[string]interface{}{
			"agent_id":    agent.ID,
			"project_id":  projectID,
			"provider_id": agent.ProviderID,
			"task_id":     taskID,
			"bead_id":     beadID,
		})
	}

	// Update agent status
	_ = m.UpdateAgentStatus(agentID, "working")
	if task != nil && task.BeadID != "" {
		m.mu.Lock()
		if a, ok := m.agents[agentID]; ok {
			a.CurrentBead = task.BeadID
			a.LastActive = time.Now()
			m.persistAgent(a)
		}
		m.mu.Unlock()
	}
	defer func() {
		if task != nil && task.BeadID != "" {
			m.mu.Lock()
			if a, ok := m.agents[agentID]; ok {
				a.CurrentBead = ""
				m.persistAgent(a)
			}
			m.mu.Unlock()
		}
		_ = m.UpdateAgentStatus(agentID, "idle")
	}()

	// Ensure a worker exists for this agent; auto-spawn if the agent has a
	// provider but no worker yet (e.g. agents created without a provider that
	// were later auto-assigned one by the dispatcher).
	if _, workerErr := m.workerPool.GetWorker(agentID); workerErr != nil && agent.ProviderID != "" {
		if _, spawnErr := m.workerPool.SpawnWorker(agent, agent.ProviderID); spawnErr != nil {
			log.Printf("[WorkerManager] Auto-spawn worker for %s failed: %v", agentID, spawnErr)
		}
	}

	// Action loop mode: delegate full loop to the worker
	router := m.actionRouter
	if m.actionLoopEnabled && router != nil {
		workerInstance, workerErr := m.workerPool.GetWorker(agentID)
		if workerErr != nil {
			return nil, fmt.Errorf("failed to get worker for loop: %w", workerErr)
		}

		// Set database on worker if available
		if m.db != nil {
			workerInstance.SetDatabase(m.db)
		}

		maxIter := m.maxLoopIterations
		if maxIter <= 0 {
			maxIter = 15
		}

		loopConfig := &worker.LoopConfig{
			MaxIterations: maxIter,
			Router:        router,
			ActionContext: actions.ActionContext{
				AgentID:   agentID,
				BeadID:    task.BeadID,
				ProjectID: task.ProjectID,
			},
			LessonsProvider: m.lessonsProvider,
			DB:              m.db,
		}

		loopResult, loopErr := workerInstance.ExecuteTaskWithLoop(ctx, task, loopConfig)
		if loopErr != nil {
			elapsed := time.Since(startTime)
			observability.Error("agent.task_complete", map[string]interface{}{
				"agent_id":    agent.ID,
				"project_id":  projectID,
				"provider_id": agent.ProviderID,
				"task_id":     taskID,
				"bead_id":     beadID,
				"duration_ms": elapsed.Milliseconds(),
				"success":     false,
				"loop_mode":   true,
			}, loopErr)
			return nil, fmt.Errorf("action loop failed: %w", loopErr)
		}

		result := loopResult.TaskResult
		if result == nil {
			result = &worker.TaskResult{
				TaskID:   task.ID,
				WorkerID: agentID,
				AgentID:  agentID,
			}
		}

		// Store loop metadata
		result.LoopIterations = loopResult.Iterations
		result.LoopTerminalReason = loopResult.TerminalReason

		_ = m.UpdateHeartbeat(agentID)

		elapsed := time.Since(startTime)
		if task != nil {
			observability.Info("agent.task_complete", map[string]interface{}{
				"agent_id":        agent.ID,
				"project_id":      projectID,
				"provider_id":     agent.ProviderID,
				"task_id":         taskID,
				"bead_id":         beadID,
				"duration_ms":     elapsed.Milliseconds(),
				"success":         result.Success,
				"error":           result.Error,
				"loop_iterations": loopResult.Iterations,
				"terminal_reason": loopResult.TerminalReason,
				"loop_mode":       true,
			})
		}
		log.Printf("Agent %s completed task %s via action loop (%d iterations, reason: %s)",
			agent.Name, task.ID, loopResult.Iterations, loopResult.TerminalReason)

		if al := m.analyticsLogger; al != nil && result != nil {
			statusCode := 200
			if !result.Success {
				statusCode = 500
			}
			_ = al.LogRequest(ctx, &analytics.RequestLog{
				UserID:      "agent:" + agent.Name,
				Method:      "POST",
				Path:        "/internal/worker/execute-loop",
				ProviderID:  agent.ProviderID,
				TotalTokens: int64(result.TokensUsed),
				LatencyMs:   elapsed.Milliseconds(),
				StatusCode:  statusCode,
				ErrorMessage: result.Error,
				Metadata: map[string]string{
					"agent_id":        agent.ID,
					"bead_id":         beadID,
					"task_id":         taskID,
					"loop_iterations": fmt.Sprintf("%d", loopResult.Iterations),
					"terminal_reason": loopResult.TerminalReason,
				},
			})
		}

		return result, nil
	}

	// Execute task through worker pool (legacy single-shot mode)
	result, err := m.workerPool.ExecuteTask(ctx, task, agentID)
	if err != nil {
		elapsed := time.Since(startTime)
		observability.Error("agent.task_complete", map[string]interface{}{
			"agent_id":    agent.ID,
			"project_id":  projectID,
			"provider_id": agent.ProviderID,
			"task_id":     taskID,
			"bead_id":     beadID,
			"duration_ms": elapsed.Milliseconds(),
			"success":     false,
		}, err)
		if al := m.analyticsLogger; al != nil {
			_ = al.LogRequest(ctx, &analytics.RequestLog{
				UserID:     "agent:" + agent.Name,
				Method:     "POST",
				Path:       "/internal/worker/execute",
				ProviderID: agent.ProviderID,
				LatencyMs:  elapsed.Milliseconds(),
				StatusCode: 500,
				ErrorMessage: err.Error(),
				Metadata: map[string]string{
					"agent_id": agent.ID,
					"bead_id":  beadID,
					"task_id":  taskID,
				},
			})
		}
		return nil, fmt.Errorf("task execution failed: %w", err)
	}

	// Enforce strict JSON action output and route actions
	if result != nil && task != nil {
		router := m.actionRouter
		if router != nil {
			actx := actions.ActionContext{
				AgentID:   agentID,
				BeadID:    task.BeadID,
				ProjectID: task.ProjectID,
			}
			env, parseErr := actions.DecodeLenient([]byte(result.Response))
			if parseErr != nil {
				actionResult := router.AutoFileParseFailure(ctx, actx, parseErr, result.Response)
				result.Actions = []actions.Result{actionResult}
				result.Success = false
				result.Error = fmt.Sprintf("action parse failed: %v", parseErr)
			} else {
				actionsResult, execErr := router.Execute(ctx, env, actx)
				result.Actions = actionsResult
				if execErr != nil {
					result.Success = false
					result.Error = execErr.Error()
				} else {
					for _, ar := range actionsResult {
						if ar.Status == "error" {
							result.Success = false
							result.Error = ar.Message
							break
						}
					}
				}
			}
		}
	}

	// Update last active time
	_ = m.UpdateHeartbeat(agentID)

	elapsed := time.Since(startTime)
	if task != nil {
		observability.Info("agent.task_complete", map[string]interface{}{
			"agent_id":    agent.ID,
			"project_id":  projectID,
			"provider_id": agent.ProviderID,
			"task_id":     taskID,
			"bead_id":     beadID,
			"duration_ms": elapsed.Milliseconds(),
			"success":     result.Success,
			"error":       result.Error,
		})
	}
	log.Printf("Agent %s completed task %s", agent.Name, task.ID)

	// Log to analytics for the observability dashboard
	if al := m.analyticsLogger; al != nil && result != nil {
		statusCode := 200
		if !result.Success {
			statusCode = 500
		}
		// Get model name from worker if available
		modelName := ""
		if w, wErr := m.workerPool.GetWorker(agentID); wErr == nil {
			info := w.GetInfo()
			modelName = info.ProviderID // Best available; provider config has the model
		}
		_ = al.LogRequest(ctx, &analytics.RequestLog{
			UserID:           "agent:" + agent.Name,
			Method:           "POST",
			Path:             "/internal/worker/execute",
			ProviderID:       agent.ProviderID,
			ModelName:        modelName,
			TotalTokens:      int64(result.TokensUsed),
			LatencyMs:        elapsed.Milliseconds(),
			StatusCode:       statusCode,
			ErrorMessage:     result.Error,
			Metadata: map[string]string{
				"agent_id": agent.ID,
				"bead_id":  beadID,
				"task_id":  taskID,
			},
		})
	}

	return result, nil
}

// StopAgent stops and removes an agent and its worker
func (m *WorkerManager) StopAgent(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, ok := m.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}

	// Stop the worker
	if err := m.workerPool.StopWorker(id); err != nil {
		log.Printf("Warning: failed to stop worker for agent %s: %v", id, err)
	}

	// Remove agent
	delete(m.agents, id)

	log.Printf("Stopped agent %s", agent.Name)
	if m.eventBus != nil {
		_ = m.eventBus.PublishAgentEvent(eventbus.EventTypeAgentCompleted, id, agent.ProjectID, map[string]interface{}{"reason": "stopped"})
	}

	return nil
}

// GetAgent retrieves an agent by ID
func (m *WorkerManager) GetAgent(id string) (*models.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, ok := m.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", id)
	}

	return agent, nil
}

// ListAgents returns all agents
func (m *WorkerManager) ListAgents() []*models.Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*models.Agent, 0, len(m.agents))
	for _, agent := range m.agents {
		agents = append(agents, agent)
	}

	return agents
}

// ListAgentsByProject returns agents for a specific project
func (m *WorkerManager) ListAgentsByProject(projectID string) []*models.Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*models.Agent, 0)
	for _, agent := range m.agents {
		if agent.ProjectID == projectID {
			agents = append(agents, agent)
		}
	}

	return agents
}

// UpdateAgentStatus updates an agent's status
func (m *WorkerManager) UpdateAgentStatus(id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, ok := m.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}

	oldStatus := agent.Status
	agent.Status = status
	agent.LastActive = time.Now()
	m.persistAgent(agent)
	if m.eventBus != nil && oldStatus != status {
		_ = m.eventBus.PublishAgentEvent(eventbus.EventTypeAgentStatusChange, agent.ID, agent.ProjectID, map[string]interface{}{
			"old_status":   oldStatus,
			"new_status":   status,
			"current_bead": agent.CurrentBead,
			"provider_id":  agent.ProviderID,
		})
	}
	observability.Info("agent.status_update", map[string]interface{}{
		"agent_id":        agent.ID,
		"project_id":      agent.ProjectID,
		"provider_id":     agent.ProviderID,
		"previous_status": oldStatus,
		"status":          status,
		"bead_id":         agent.CurrentBead,
	})

	return nil
}

// AssignBead assigns a bead to an agent
func (m *WorkerManager) AssignBead(agentID, beadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, ok := m.agents[agentID]
	if !ok {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	oldBead := agent.CurrentBead
	agent.CurrentBead = beadID
	agent.Status = "working"
	agent.LastActive = time.Now()
	m.persistAgent(agent)
	if m.eventBus != nil {
		_ = m.eventBus.PublishAgentEvent(eventbus.EventTypeAgentStatusChange, agent.ID, agent.ProjectID, map[string]interface{}{
			"old_status":  "idle",
			"new_status":  "working",
			"old_bead":    oldBead,
			"bead_id":     beadID,
			"provider_id": agent.ProviderID,
		})
	}
	observability.Info("agent.assign_bead", map[string]interface{}{
		"agent_id":    agent.ID,
		"project_id":  agent.ProjectID,
		"provider_id": agent.ProviderID,
		"bead_id":     beadID,
		"old_bead":    oldBead,
	})

	return nil
}

// UpdateHeartbeat updates an agent's last active time
func (m *WorkerManager) UpdateHeartbeat(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, ok := m.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}

	agent.LastActive = time.Now()
	m.persistAgent(agent)
	if m.eventBus != nil {
		_ = m.eventBus.PublishAgentEvent(eventbus.EventTypeAgentHeartbeat, agent.ID, agent.ProjectID, map[string]interface{}{
			"provider_id": agent.ProviderID,
		})
	}

	return nil
}

// GetIdleAgents returns agents that are idle
func (m *WorkerManager) GetIdleAgents() []*models.Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agents := make([]*models.Agent, 0)
	for _, agent := range m.agents {
		if agent.Status == "idle" {
			agents = append(agents, agent)
		}
	}

	return agents
}

// GetWorkerPool returns the worker pool
func (m *WorkerManager) GetWorkerPool() *worker.Pool {
	return m.workerPool
}

// GetPoolStats returns worker pool statistics
func (m *WorkerManager) GetPoolStats() worker.PoolStats {
	return m.workerPool.GetPoolStats()
}

// ResetStuckAgents resets agents that have been in "working" state too long
// or restores paused agents that have providers assigned.
// Returns the number of agents that were reset.
func (m *WorkerManager) ResetStuckAgents(maxWorkingDuration time.Duration) int {
	// Collect agents to restore outside the lock to avoid deadlock
	// (RestoreAgentWorker also acquires m.mu).
	var toRestore []*models.Agent

	m.mu.Lock()
	count := 0
	now := time.Now()

	for _, agent := range m.agents {
		if agent.Status == "working" {
			elapsed := now.Sub(agent.LastActive)
			if elapsed > maxWorkingDuration {
				log.Printf("[WorkerManager] Resetting stuck agent %s (working for %v)", agent.ID, elapsed)
				agent.Status = "idle"
				agent.CurrentBead = ""

				// Persist the change if we have a persister
				if m.agentPersister != nil {
					_ = m.agentPersister.UpsertAgent(agent)
				}

				// Publish event
				if m.eventBus != nil {
					eventData := map[string]interface{}{
						"agent_id":   agent.ID,
						"project_id": agent.ProjectID,
						"reason":     "stuck_timeout",
					}
					_ = m.eventBus.PublishAgentEvent("agent.reset", agent.ID, agent.ProjectID, eventData)
				}

				count++
			}
		} else if agent.Status == "paused" && agent.ProviderID != "" {
			toRestore = append(toRestore, agent)
		}
	}
	m.mu.Unlock()

	// Restore paused agents outside the lock
	for _, agent := range toRestore {
		log.Printf("[WorkerManager] Attempting to restore paused agent %s", agent.ID)
		if _, err := m.RestoreAgentWorker(context.Background(), agent); err != nil {
			log.Printf("[WorkerManager] Failed to restore agent %s: %v", agent.ID, err)
		} else {
			log.Printf("[WorkerManager] Successfully restored paused agent %s to idle", agent.ID)
			count++
		}
	}

	return count
}

// StopAll stops all agents and workers
func (m *WorkerManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop all workers
	m.workerPool.StopAll()

	// Clear agents
	m.agents = make(map[string]*models.Agent)

	log.Println("Stopped all agents and workers")
}
