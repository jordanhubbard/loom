package types

import (
	"context"
	"time"
)

// Task represents a coding task to be executed by an agent
type Task struct {
	ID          string
	Description string
	Priority    int
	Status      TaskStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Result      *TaskResult
}

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusAssigned   TaskStatus = "assigned"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
)

// TaskResult represents the result of a completed task
type TaskResult struct {
	Success bool
	Message string
	Data    map[string]interface{}
}

// Agent represents an AI coding agent
type Agent struct {
	ID           string
	Name         string
	Type         AgentType
	Capabilities []string
	Status       AgentStatus
	CurrentTask  *Task
}

// AgentType represents different types of agents
type AgentType string

const (
	AgentTypeGeneral    AgentType = "general"
	AgentTypeSpecialist AgentType = "specialist"
	AgentTypeReviewer   AgentType = "reviewer"
)

// AgentStatus represents the current state of an agent
type AgentStatus string

const (
	AgentStatusIdle     AgentStatus = "idle"
	AgentStatusBusy     AgentStatus = "busy"
	AgentStatusOffline  AgentStatus = "offline"
)

// AgentInterface defines the contract for all agents
type AgentInterface interface {
	Execute(ctx context.Context, task *Task) (*TaskResult, error)
	GetCapabilities() []string
	GetStatus() AgentStatus
}

// Dispatcher manages task distribution to agents
type Dispatcher interface {
	AssignTask(ctx context.Context, task *Task) (*Agent, error)
	GetAvailableAgents() []*Agent
}

// DecisionMaker makes automatic decisions about task routing
type DecisionMaker interface {
	DecideAgent(ctx context.Context, task *Task, agents []*Agent) (*Agent, error)
	EvaluatePriority(task *Task) int
}
