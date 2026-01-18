package types

import (
	"testing"
	"time"
)

func TestTaskStatus(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		want   TaskStatus
	}{
		{"Pending status", TaskStatusPending, "pending"},
		{"Assigned status", TaskStatusAssigned, "assigned"},
		{"InProgress status", TaskStatusInProgress, "in_progress"},
		{"Completed status", TaskStatusCompleted, "completed"},
		{"Failed status", TaskStatusFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status != tt.want {
				t.Errorf("TaskStatus = %v, want %v", tt.status, tt.want)
			}
		})
	}
}

func TestAgentType(t *testing.T) {
	tests := []struct {
		name      string
		agentType AgentType
		want      AgentType
	}{
		{"General type", AgentTypeGeneral, "general"},
		{"Specialist type", AgentTypeSpecialist, "specialist"},
		{"Reviewer type", AgentTypeReviewer, "reviewer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.agentType != tt.want {
				t.Errorf("AgentType = %v, want %v", tt.agentType, tt.want)
			}
		})
	}
}

func TestAgentStatus(t *testing.T) {
	tests := []struct {
		name   string
		status AgentStatus
		want   AgentStatus
	}{
		{"Idle status", AgentStatusIdle, "idle"},
		{"Busy status", AgentStatusBusy, "busy"},
		{"Offline status", AgentStatusOffline, "offline"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status != tt.want {
				t.Errorf("AgentStatus = %v, want %v", tt.status, tt.want)
			}
		})
	}
}

func TestTask(t *testing.T) {
	now := time.Now()
	task := &Task{
		ID:          "test-1",
		Description: "Test task",
		Priority:    5,
		Status:      TaskStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if task.ID != "test-1" {
		t.Errorf("Task.ID = %v, want %v", task.ID, "test-1")
	}

	if task.Status != TaskStatusPending {
		t.Errorf("Task.Status = %v, want %v", task.Status, TaskStatusPending)
	}

	if task.Priority != 5 {
		t.Errorf("Task.Priority = %v, want %v", task.Priority, 5)
	}
}

func TestAgent(t *testing.T) {
	agent := &Agent{
		ID:           "agent-1",
		Name:         "Test Agent",
		Type:         AgentTypeGeneral,
		Capabilities: []string{"coding", "testing"},
		Status:       AgentStatusIdle,
	}

	if agent.ID != "agent-1" {
		t.Errorf("Agent.ID = %v, want %v", agent.ID, "agent-1")
	}

	if agent.Type != AgentTypeGeneral {
		t.Errorf("Agent.Type = %v, want %v", agent.Type, AgentTypeGeneral)
	}

	if len(agent.Capabilities) != 2 {
		t.Errorf("len(Agent.Capabilities) = %v, want %v", len(agent.Capabilities), 2)
	}
}
