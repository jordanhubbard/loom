package agent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jordanhubbard/arbiter/internal/provider"
	"github.com/jordanhubbard/arbiter/internal/worker"
	"github.com/jordanhubbard/arbiter/pkg/models"
)

// WorkerManager manages agents with worker pool integration
type WorkerManager struct {
	agents         map[string]*models.Agent
	workerPool     *worker.Pool
	providerRegistry *provider.Registry
	mu             sync.RWMutex
	maxAgents      int
}

// NewWorkerManager creates a new agent manager with worker pool
func NewWorkerManager(maxAgents int, providerRegistry *provider.Registry) *WorkerManager {
	return &WorkerManager{
		agents:           make(map[string]*models.Agent),
		workerPool:       worker.NewPool(providerRegistry, maxAgents),
		providerRegistry: providerRegistry,
		maxAgents:        maxAgents,
	}
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
		PersonaName: personaName,
		Persona:     persona,
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
	
	log.Printf("Spawned agent %s with worker using provider %s", agent.Name, providerID)
	
	return agent, nil
}

// ExecuteTask assigns a task to an agent's worker
func (m *WorkerManager) ExecuteTask(ctx context.Context, agentID string, task *worker.Task) (*worker.TaskResult, error) {
	m.mu.RLock()
	agent, exists := m.agents[agentID]
	m.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	
	// Update agent status
	m.UpdateAgentStatus(agentID, "working")
	defer m.UpdateAgentStatus(agentID, "idle")
	
	// Execute task through worker pool
	result, err := m.workerPool.ExecuteTask(ctx, task, agentID)
	if err != nil {
		return nil, fmt.Errorf("task execution failed: %w", err)
	}
	
	// Update last active time
	m.UpdateHeartbeat(agentID)
	
	log.Printf("Agent %s completed task %s", agent.Name, task.ID)
	
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
	
	agent.Status = status
	agent.LastActive = time.Now()
	
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
	
	agent.CurrentBead = beadID
	agent.Status = "working"
	agent.LastActive = time.Now()
	
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
