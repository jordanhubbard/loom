package models

import "time"

// CommandLog represents a shell command executed by an agent
type CommandLog struct {
	ID          string                 `json:"id"`
	AgentID     string                 `json:"agent_id"`
	BeadID      string                 `json:"bead_id"`
	ProjectID   string                 `json:"project_id"`
	Command     string                 `json:"command"`
	WorkingDir  string                 `json:"working_dir"`
	ExitCode    int                    `json:"exit_code"`
	Stdout      string                 `json:"stdout"`
	Stderr      string                 `json:"stderr"`
	Duration    int64                  `json:"duration_ms"` // milliseconds
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt time.Time              `json:"completed_at"`
	Context     map[string]interface{} `json:"context"`
	CreatedAt   time.Time              `json:"created_at"`
}
