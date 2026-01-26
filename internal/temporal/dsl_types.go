package temporal

import (
	"time"
)

// TemporalInstructionType represents the type of Temporal instruction
type TemporalInstructionType string

const (
	InstructionTypeWorkflow   TemporalInstructionType = "WORKFLOW"
	InstructionTypeSchedule   TemporalInstructionType = "SCHEDULE"
	InstructionTypeQuery      TemporalInstructionType = "QUERY"
	InstructionTypeSignal     TemporalInstructionType = "SIGNAL"
	InstructionTypeActivity   TemporalInstructionType = "ACTIVITY"
	InstructionTypeCancelWF   TemporalInstructionType = "CANCEL"
	InstructionTypeListWF     TemporalInstructionType = "LIST"
	InstructionTypeMotivation TemporalInstructionType = "MOTIVATION" // Register/trigger motivations
)

// TemporalInstruction represents a parsed Temporal DSL instruction
type TemporalInstruction struct {
	Type           TemporalInstructionType `json:"type"`
	Name           string                  `json:"name"`                      // Workflow/Activity name
	WorkflowID     string                  `json:"workflow_id,omitempty"`     // For QUERY, SIGNAL, CANCEL
	Input          map[string]interface{}  `json:"input,omitempty"`           // Workflow/Activity input
	Timeout        time.Duration           `json:"timeout,omitempty"`         // Execution timeout
	Retry          int                     `json:"retry,omitempty"`           // Number of retries
	Wait           bool                    `json:"wait,omitempty"`            // Wait for completion
	Interval       time.Duration           `json:"interval,omitempty"`        // For SCHEDULE
	QueryType      string                  `json:"query_type,omitempty"`      // For QUERY
	SignalName     string                  `json:"signal_name,omitempty"`     // For SIGNAL
	SignalData     map[string]interface{}  `json:"signal_data,omitempty"`     // For SIGNAL
	RunID          string                  `json:"run_id,omitempty"`          // Optional run ID
	Priority       int                     `json:"priority,omitempty"`        // Workflow priority
	IdempotencyKey string                  `json:"idempotency_key,omitempty"` // Idempotency
	Description    string                  `json:"description,omitempty"`     // Human readable description

	// Motivation-specific fields
	AgentRole       string `json:"agent_role,omitempty"`       // For MOTIVATION: target agent role
	Condition       string `json:"condition,omitempty"`        // For MOTIVATION: trigger condition
	Enabled         bool   `json:"enabled,omitempty"`          // For MOTIVATION: is enabled
	CooldownMinutes int    `json:"cooldown_minutes,omitempty"` // For MOTIVATION: cooldown period
}

// TemporalInstructionResult represents the result of executing a Temporal instruction
type TemporalInstructionResult struct {
	Instruction TemporalInstruction `json:"instruction"`
	Success     bool                `json:"success"`
	Result      interface{}         `json:"result,omitempty"`
	Error       string              `json:"error,omitempty"`
	ExecutedAt  time.Time           `json:"executed_at"`
	Duration    time.Duration       `json:"duration"`
}

// TemporalDSLExecution represents the full execution of a DSL block
type TemporalDSLExecution struct {
	AgentID        string                      `json:"agent_id"`
	Instructions   []TemporalInstruction       `json:"instructions"`
	Results        []TemporalInstructionResult `json:"results"`
	CleanedText    string                      `json:"cleaned_text"`
	ExecutionError string                      `json:"execution_error,omitempty"`
	TotalDuration  time.Duration               `json:"total_duration"`
	ExecutedAt     time.Time                   `json:"executed_at"`
}

// WorkflowOptions represents options for scheduling a workflow
type WorkflowOptions struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Input          interface{}   `json:"input"`
	Timeout        time.Duration `json:"timeout"`
	Retry          int           `json:"retry"`
	Wait           bool          `json:"wait"`
	Priority       int           `json:"priority"`
	IdempotencyKey string        `json:"idempotency_key"`
}

// ActivityOptions represents options for executing an activity
type ActivityOptions struct {
	Name    string        `json:"name"`
	Input   interface{}   `json:"input"`
	Timeout time.Duration `json:"timeout"`
	Retry   int           `json:"retry"`
	Wait    bool          `json:"wait"`
}

// ScheduleOptions represents options for creating a scheduled workflow
type ScheduleOptions struct {
	Name     string        `json:"name"`
	Workflow string        `json:"workflow"`
	Input    interface{}   `json:"input"`
	Interval time.Duration `json:"interval"`
	Timeout  time.Duration `json:"timeout"`
	Retry    int           `json:"retry"`
}

// QueryOptions represents options for querying a workflow
type QueryOptions struct {
	WorkflowID string        `json:"workflow_id"`
	RunID      string        `json:"run_id"`
	QueryType  string        `json:"query_type"`
	Args       []interface{} `json:"args"`
}

// SignalOptions represents options for signaling a workflow
type SignalOptions struct {
	WorkflowID string      `json:"workflow_id"`
	RunID      string      `json:"run_id"`
	Name       string      `json:"name"`
	Data       interface{} `json:"data"`
}

// CancelOptions represents options for canceling a workflow
type CancelOptions struct {
	WorkflowID string `json:"workflow_id"`
	RunID      string `json:"run_id"`
}

// MotivationOptions represents options for registering/triggering a motivation
type MotivationOptions struct {
	Name            string                 `json:"name"`
	Type            string                 `json:"type"`              // "calendar", "event", "threshold", "idle", "external"
	Condition       string                 `json:"condition"`         // Trigger condition
	AgentRole       string                 `json:"agent_role"`        // Target agent role
	Enabled         bool                   `json:"enabled"`           // Is motivation enabled
	CooldownMinutes int                    `json:"cooldown_minutes"`  // Cooldown between triggers
	Parameters      map[string]interface{} `json:"parameters"`        // Condition parameters
	CreateBead      bool                   `json:"create_bead"`       // Create stimulus bead on trigger
	WakeAgent       bool                   `json:"wake_agent"`        // Wake agent on trigger
}
