package decision

import (
	"context"
	"fmt"
	"strings"

	"github.com/jordanhubbard/arbiter/pkg/types"
)

// SimpleMaker implements a simple decision making algorithm
type SimpleMaker struct{}

// NewSimpleMaker creates a new simple decision maker
func NewSimpleMaker() *SimpleMaker {
	return &SimpleMaker{}
}

// DecideAgent selects the most appropriate agent for a task
func (m *SimpleMaker) DecideAgent(ctx context.Context, task *types.Task, agents []*types.Agent) (*types.Agent, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents available")
	}

	// Simple decision algorithm:
	// 1. Look for agents with matching capabilities
	// 2. Prefer specialist agents over general agents
	// 3. Fall back to any available agent

	var bestAgent *types.Agent
	bestScore := -1

	for _, agent := range agents {
		if agent.Status != types.AgentStatusIdle {
			continue
		}

		score := m.calculateScore(task, agent)
		if score > bestScore {
			bestScore = score
			bestAgent = agent
		}
	}

	if bestAgent == nil {
		return nil, fmt.Errorf("no suitable agent found")
	}

	return bestAgent, nil
}

// calculateScore calculates a score for how well an agent matches a task
func (m *SimpleMaker) calculateScore(task *types.Task, agent *types.Agent) int {
	score := 0

	// Bonus for matching capabilities
	taskLower := strings.ToLower(task.Description)
	for _, capability := range agent.Capabilities {
		if strings.Contains(taskLower, strings.ToLower(capability)) {
			score += 10
		}
	}

	// Bonus for specialist agents
	if agent.Type == types.AgentTypeSpecialist {
		score += 5
	}

	// Base score for any available agent
	score += 1

	return score
}

// EvaluatePriority evaluates and assigns a priority to a task
func (m *SimpleMaker) EvaluatePriority(task *types.Task) int {
	// Simple priority evaluation based on description keywords
	priority := task.Priority
	if priority == 0 {
		priority = 5 // Default priority
	}

	description := strings.ToLower(task.Description)

	// Increase priority for critical keywords
	if strings.Contains(description, "urgent") || strings.Contains(description, "critical") {
		priority += 5
	}

	if strings.Contains(description, "bug") || strings.Contains(description, "fix") {
		priority += 3
	}

	return priority
}
