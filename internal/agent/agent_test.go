package agent

import (
	"context"
	"testing"

	"github.com/jordanhubbard/arbiter/pkg/types"
)

func TestNewBaseAgent(t *testing.T) {
	agent := NewBaseAgent("agent-1", "Test Agent", types.AgentTypeGeneral, []string{"coding"})

	if agent.GetID() != "agent-1" {
		t.Errorf("GetID() = %v, want %v", agent.GetID(), "agent-1")
	}

	if agent.GetName() != "Test Agent" {
		t.Errorf("GetName() = %v, want %v", agent.GetName(), "Test Agent")
	}

	if agent.GetType() != types.AgentTypeGeneral {
		t.Errorf("GetType() = %v, want %v", agent.GetType(), types.AgentTypeGeneral)
	}

	if agent.GetStatus() != types.AgentStatusIdle {
		t.Errorf("GetStatus() = %v, want %v", agent.GetStatus(), types.AgentStatusIdle)
	}
}

func TestBaseAgent_Execute(t *testing.T) {
	agent := NewBaseAgent("agent-1", "Test Agent", types.AgentTypeGeneral, []string{"coding"})
	ctx := context.Background()

	task := &types.Task{
		ID:          "task-1",
		Description: "Test task",
		Priority:    5,
		Status:      types.TaskStatusPending,
	}

	// Test successful execution
	result, err := agent.Execute(ctx, task)
	if err != nil {
		t.Errorf("Execute() error = %v, want nil", err)
	}

	if result == nil {
		t.Fatal("Execute() result = nil, want non-nil")
	}

	if !result.Success {
		t.Errorf("result.Success = %v, want true", result.Success)
	}

	// Test agent is busy
	agent.status = types.AgentStatusBusy
	_, err = agent.Execute(ctx, task)
	if err == nil {
		t.Error("Execute() on busy agent should return error")
	}
}

func TestBaseAgent_GetCapabilities(t *testing.T) {
	capabilities := []string{"coding", "testing", "debugging"}
	agent := NewBaseAgent("agent-1", "Test Agent", types.AgentTypeGeneral, capabilities)

	caps := agent.GetCapabilities()
	if len(caps) != len(capabilities) {
		t.Errorf("len(GetCapabilities()) = %v, want %v", len(caps), len(capabilities))
	}

	for i, cap := range caps {
		if cap != capabilities[i] {
			t.Errorf("GetCapabilities()[%d] = %v, want %v", i, cap, capabilities[i])
		}
	}
}
