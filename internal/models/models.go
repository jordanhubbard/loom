package models

import "time"

// WorkStatus represents the current state of a work item
type WorkStatus string

const (
	WorkStatusPending    WorkStatus = "pending"
	WorkStatusInProgress WorkStatus = "in_progress"
	WorkStatusCompleted  WorkStatus = "completed"
	WorkStatusFailed     WorkStatus = "failed"
)

// Work represents a task or job in the system
type Work struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Status      WorkStatus `json:"status"`
	AssignedTo  string     `json:"assigned_to,omitempty"` // Agent ID
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Result      string     `json:"result,omitempty"`
}

// Agent represents an AI agent in the system
type Agent struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Status      string    `json:"status"` // active, idle, busy
	CurrentWork string    `json:"current_work,omitempty"`
	ServiceID   string    `json:"service_id"` // Which service endpoint this agent uses
	CreatedAt   time.Time `json:"created_at"`
}

// AgentCommunication represents communication between two agents
type AgentCommunication struct {
	ID        string    `json:"id"`
	FromAgent string    `json:"from_agent"`
	ToAgent   string    `json:"to_agent"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// CostType represents whether a service has fixed or variable costs
type CostType string

const (
	CostTypeFixed    CostType = "fixed"
	CostTypeVariable CostType = "variable"
)

// ServiceEndpoint represents an LLM service endpoint
type ServiceEndpoint struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	URL           string    `json:"url"`
	Type          string    `json:"type"` // ollama, vllm, openai, anthropic, etc.
	IsActive      bool      `json:"is_active"`
	CostType      CostType  `json:"cost_type"`
	CostPerToken  float64   `json:"cost_per_token"`  // For variable cost services
	FixedCost     float64   `json:"fixed_cost"`      // For fixed cost services (monthly, etc.)
	TokensUsed    int64     `json:"tokens_used"`
	TotalCost     float64   `json:"total_cost"`
	RequestCount  int64     `json:"request_count"`
	LastActive    time.Time `json:"last_active"`
	CreatedAt     time.Time `json:"created_at"`
}

// Traffic represents network traffic to/from a service
type Traffic struct {
	ServiceID     string    `json:"service_id"`
	BytesSent     int64     `json:"bytes_sent"`
	BytesReceived int64     `json:"bytes_received"`
	RequestCount  int64     `json:"request_count"`
	Timestamp     time.Time `json:"timestamp"`
}

// CreateWorkRequest represents a request to create new work
type CreateWorkRequest struct {
	Description string `json:"description"`
}

// UpdateServiceCostRequest represents a request to update service costs
type UpdateServiceCostRequest struct {
	CostType     CostType `json:"cost_type"`
	CostPerToken *float64 `json:"cost_per_token,omitempty"`
	FixedCost    *float64 `json:"fixed_cost,omitempty"`
}
