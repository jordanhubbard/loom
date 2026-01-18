package agent

import (
	"context"
	"fmt"

	"github.com/jordanhubbard/arbiter/pkg/types"
)

// BaseAgent implements the basic agent functionality
type BaseAgent struct {
	id           string
	name         string
	agentType    types.AgentType
	capabilities []string
	status       types.AgentStatus
	currentTask  *types.Task
}

// NewBaseAgent creates a new base agent
func NewBaseAgent(id, name string, agentType types.AgentType, capabilities []string) *BaseAgent {
	return &BaseAgent{
		id:           id,
		name:         name,
		agentType:    agentType,
		capabilities: capabilities,
		status:       types.AgentStatusIdle,
	}
}

// Execute executes a task
func (a *BaseAgent) Execute(ctx context.Context, task *types.Task) (*types.TaskResult, error) {
	if a.status == types.AgentStatusBusy {
		return nil, fmt.Errorf("agent %s is busy", a.id)
	}

	a.status = types.AgentStatusBusy
	a.currentTask = task
	defer func() {
		a.status = types.AgentStatusIdle
		a.currentTask = nil
	}()

	// Simulate task execution
	// In a real implementation, this would call actual AI agent APIs
	result := &types.TaskResult{
		Success: true,
		Message: fmt.Sprintf("Task %s completed by agent %s", task.ID, a.name),
		Data:    make(map[string]interface{}),
	}

	return result, nil
}

// GetCapabilities returns the agent's capabilities
func (a *BaseAgent) GetCapabilities() []string {
	return a.capabilities
}

// GetStatus returns the agent's current status
func (a *BaseAgent) GetStatus() types.AgentStatus {
	return a.status
}

// GetID returns the agent's ID
func (a *BaseAgent) GetID() string {
	return a.id
}

// GetName returns the agent's name
func (a *BaseAgent) GetName() string {
	return a.name
}

// GetType returns the agent's type
func (a *BaseAgent) GetType() types.AgentType {
	return a.agentType
}
