package dispatcher

import (
	"context"
	"fmt"
	"sync"

	"github.com/jordanhubbard/arbiter/pkg/types"
)

// TaskDispatcher manages task distribution to agents
type TaskDispatcher struct {
	agents        map[string]*types.Agent
	decisionMaker types.DecisionMaker
	mu            sync.RWMutex
}

// NewTaskDispatcher creates a new task dispatcher
func NewTaskDispatcher(decisionMaker types.DecisionMaker) *TaskDispatcher {
	return &TaskDispatcher{
		agents:        make(map[string]*types.Agent),
		decisionMaker: decisionMaker,
	}
}

// RegisterAgent registers a new agent
func (d *TaskDispatcher) RegisterAgent(agent *types.Agent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.agents[agent.ID] = agent
}

// AssignTask assigns a task to an appropriate agent
func (d *TaskDispatcher) AssignTask(ctx context.Context, task *types.Task) (*types.Agent, error) {
	d.mu.RLock()
	availableAgents := d.GetAvailableAgents()
	d.mu.RUnlock()

	if len(availableAgents) == 0 {
		return nil, fmt.Errorf("no available agents")
	}

	// Use decision maker to select the best agent
	selectedAgent, err := d.decisionMaker.DecideAgent(ctx, task, availableAgents)
	if err != nil {
		return nil, fmt.Errorf("failed to decide agent: %w", err)
	}

	// Update task status
	task.Status = types.TaskStatusAssigned

	// Update agent's current task
	d.mu.Lock()
	selectedAgent.CurrentTask = task
	selectedAgent.Status = types.AgentStatusBusy
	d.mu.Unlock()

	return selectedAgent, nil
}

// GetAvailableAgents returns all agents that are idle
func (d *TaskDispatcher) GetAvailableAgents() []*types.Agent {
	var available []*types.Agent
	for _, agent := range d.agents {
		if agent.Status == types.AgentStatusIdle {
			available = append(available, agent)
		}
	}
	return available
}

// GetAgents returns all registered agents
func (d *TaskDispatcher) GetAgents() []*types.Agent {
	d.mu.RLock()
	defer d.mu.RUnlock()

	agents := make([]*types.Agent, 0, len(d.agents))
	for _, agent := range d.agents {
		agents = append(agents, agent)
	}
	return agents
}
