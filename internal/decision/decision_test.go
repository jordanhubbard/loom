package decision

import (
	"context"
	"testing"

	"github.com/jordanhubbard/arbiter/pkg/types"
)

func TestNewSimpleMaker(t *testing.T) {
	maker := NewSimpleMaker()
	if maker == nil {
		t.Error("NewSimpleMaker() returned nil")
	}
}

func TestSimpleMaker_DecideAgent(t *testing.T) {
	maker := NewSimpleMaker()
	ctx := context.Background()

	task := &types.Task{
		ID:          "task-1",
		Description: "Fix Python bug",
		Priority:    5,
	}

	agents := []*types.Agent{
		{
			ID:           "agent-1",
			Name:         "General Agent",
			Type:         types.AgentTypeGeneral,
			Capabilities: []string{"coding"},
			Status:       types.AgentStatusIdle,
		},
		{
			ID:           "agent-2",
			Name:         "Python Specialist",
			Type:         types.AgentTypeSpecialist,
			Capabilities: []string{"python", "coding"},
			Status:       types.AgentStatusIdle,
		},
	}

	selectedAgent, err := maker.DecideAgent(ctx, task, agents)
	if err != nil {
		t.Errorf("DecideAgent() error = %v, want nil", err)
	}

	if selectedAgent == nil {
		t.Fatal("DecideAgent() returned nil agent")
	}

	// Should select the Python specialist due to matching capability
	if selectedAgent.ID != "agent-2" {
		t.Errorf("DecideAgent() selected agent ID = %v, want agent-2", selectedAgent.ID)
	}
}

func TestSimpleMaker_DecideAgent_NoAgents(t *testing.T) {
	maker := NewSimpleMaker()
	ctx := context.Background()

	task := &types.Task{
		ID:          "task-1",
		Description: "Test task",
		Priority:    5,
	}

	_, err := maker.DecideAgent(ctx, task, []*types.Agent{})
	if err == nil {
		t.Error("DecideAgent() with no agents should return error")
	}
}

func TestSimpleMaker_EvaluatePriority(t *testing.T) {
	maker := NewSimpleMaker()

	tests := []struct {
		name        string
		task        *types.Task
		wantMinimum int
	}{
		{
			name: "Urgent task",
			task: &types.Task{
				ID:          "task-1",
				Description: "Urgent: Fix critical bug",
				Priority:    0,
			},
			wantMinimum: 10, // 5 (default) + 5 (urgent)
		},
		{
			name: "Bug fix task",
			task: &types.Task{
				ID:          "task-2",
				Description: "Fix bug in code",
				Priority:    0,
			},
			wantMinimum: 8, // 5 (default) + 3 (bug)
		},
		{
			name: "Normal task",
			task: &types.Task{
				ID:          "task-3",
				Description: "Write documentation",
				Priority:    0,
			},
			wantMinimum: 5, // 5 (default)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priority := maker.EvaluatePriority(tt.task)
			if priority < tt.wantMinimum {
				t.Errorf("EvaluatePriority() = %v, want at least %v", priority, tt.wantMinimum)
			}
		})
	}
}

func TestSimpleMaker_calculateScore(t *testing.T) {
	maker := NewSimpleMaker()

	task := &types.Task{
		ID:          "task-1",
		Description: "Fix Python bug",
		Priority:    5,
	}

	specialist := &types.Agent{
		ID:           "agent-1",
		Type:         types.AgentTypeSpecialist,
		Capabilities: []string{"python"},
		Status:       types.AgentStatusIdle,
	}

	general := &types.Agent{
		ID:           "agent-2",
		Type:         types.AgentTypeGeneral,
		Capabilities: []string{"coding"},
		Status:       types.AgentStatusIdle,
	}

	specialistScore := maker.calculateScore(task, specialist)
	generalScore := maker.calculateScore(task, general)

	if specialistScore <= generalScore {
		t.Errorf("Specialist score (%d) should be higher than general score (%d)", specialistScore, generalScore)
	}
}
