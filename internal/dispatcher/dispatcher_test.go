package dispatcher

import (
	"context"
	"testing"

	"github.com/jordanhubbard/arbiter/internal/decision"
	"github.com/jordanhubbard/arbiter/pkg/types"
)

func TestNewTaskDispatcher(t *testing.T) {
	decisionMaker := decision.NewSimpleMaker()
	dispatcher := NewTaskDispatcher(decisionMaker)

	if dispatcher == nil {
		t.Error("NewTaskDispatcher() returned nil")
	}

	if len(dispatcher.GetAgents()) != 0 {
		t.Errorf("New dispatcher should have 0 agents, got %d", len(dispatcher.GetAgents()))
	}
}

func TestTaskDispatcher_RegisterAgent(t *testing.T) {
	decisionMaker := decision.NewSimpleMaker()
	dispatcher := NewTaskDispatcher(decisionMaker)

	agent := &types.Agent{
		ID:           "agent-1",
		Name:         "Test Agent",
		Type:         types.AgentTypeGeneral,
		Capabilities: []string{"coding"},
		Status:       types.AgentStatusIdle,
	}

	dispatcher.RegisterAgent(agent)

	agents := dispatcher.GetAgents()
	if len(agents) != 1 {
		t.Errorf("Expected 1 agent, got %d", len(agents))
	}

	if agents[0].ID != "agent-1" {
		t.Errorf("Expected agent ID 'agent-1', got '%s'", agents[0].ID)
	}
}

func TestTaskDispatcher_GetAvailableAgents(t *testing.T) {
	decisionMaker := decision.NewSimpleMaker()
	dispatcher := NewTaskDispatcher(decisionMaker)

	// Register idle agent
	idleAgent := &types.Agent{
		ID:     "agent-1",
		Name:   "Idle Agent",
		Type:   types.AgentTypeGeneral,
		Status: types.AgentStatusIdle,
	}
	dispatcher.RegisterAgent(idleAgent)

	// Register busy agent
	busyAgent := &types.Agent{
		ID:     "agent-2",
		Name:   "Busy Agent",
		Type:   types.AgentTypeGeneral,
		Status: types.AgentStatusBusy,
	}
	dispatcher.RegisterAgent(busyAgent)

	available := dispatcher.GetAvailableAgents()
	if len(available) != 1 {
		t.Errorf("Expected 1 available agent, got %d", len(available))
	}

	if available[0].ID != "agent-1" {
		t.Errorf("Expected available agent ID 'agent-1', got '%s'", available[0].ID)
	}
}

func TestTaskDispatcher_AssignTask(t *testing.T) {
	decisionMaker := decision.NewSimpleMaker()
	dispatcher := NewTaskDispatcher(decisionMaker)
	ctx := context.Background()

	agent := &types.Agent{
		ID:           "agent-1",
		Name:         "Test Agent",
		Type:         types.AgentTypeGeneral,
		Capabilities: []string{"coding"},
		Status:       types.AgentStatusIdle,
	}
	dispatcher.RegisterAgent(agent)

	task := &types.Task{
		ID:          "task-1",
		Description: "Test task",
		Priority:    5,
		Status:      types.TaskStatusPending,
	}

	assignedAgent, err := dispatcher.AssignTask(ctx, task)
	if err != nil {
		t.Errorf("AssignTask() error = %v, want nil", err)
	}

	if assignedAgent == nil {
		t.Fatal("AssignTask() returned nil agent")
	}

	if assignedAgent.ID != "agent-1" {
		t.Errorf("Assigned agent ID = %v, want agent-1", assignedAgent.ID)
	}

	if task.Status != types.TaskStatusAssigned {
		t.Errorf("Task status = %v, want %v", task.Status, types.TaskStatusAssigned)
	}
}

func TestTaskDispatcher_AssignTask_NoAgents(t *testing.T) {
	decisionMaker := decision.NewSimpleMaker()
	dispatcher := NewTaskDispatcher(decisionMaker)
	ctx := context.Background()

	task := &types.Task{
		ID:          "task-1",
		Description: "Test task",
		Priority:    5,
		Status:      types.TaskStatusPending,
	}

	_, err := dispatcher.AssignTask(ctx, task)
	if err == nil {
		t.Error("AssignTask() with no agents should return error")
	}
}
