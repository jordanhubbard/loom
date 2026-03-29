package models

import "time"

// OrgChartSnapshot represents a snapshot of the org chart at a point in time.
type OrgChartSnapshot struct {
	ID          string                 `json:"id"`
	Timestamp   time.Time              `json:"timestamp"`
	Structure   map[string]interface{} `json:"structure"`    // The org chart structure
	ReportLines map[string]string      `json:"report_lines"` // Manager -> direct reports
	Metadata    map[string]interface{} `json:"metadata"`
}

// AgentGrade represents a performance grade for an agent.
type AgentGrade struct {
	ID                  string                 `json:"id"`
	AgentID             string                 `json:"agent_id"`
	AgentRole           string                 `json:"agent_role"`
	Grade               string                 `json:"grade"` // A-F
	BeadCompletionRate  float64                `json:"bead_completion_rate"`
	BlockRate           float64                `json:"block_rate"`
	IterationEfficiency float64                `json:"iteration_efficiency"`
	ReviewPeriod        string                 `json:"review_period"` // e.g., "2026-W10"
	ReviewedAt          time.Time              `json:"reviewed_at"`
	Feedback            string                 `json:"feedback"`
	Metadata            map[string]interface{} `json:"metadata"`
}

// StatusBoardEntry represents an entry on the status board.
type StatusBoardEntry struct {
	ID         string                 `json:"id"`
	AuthorID   string                 `json:"author_id"`
	AuthorRole string                 `json:"author_role"`
	Title      string                 `json:"title"`
	Content    string                 `json:"content"`
	Category   string                 `json:"category"` // e.g., "shipped", "blocked", "feedback", "priority"
	Priority   string                 `json:"priority"` // P0, P1, P2, P3
	PostedAt   time.Time              `json:"posted_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// MeetingActionItem represents an action item from a meeting.
type MeetingActionItem struct {
	Description string    `json:"description"`
	Owner       string    `json:"owner"`
	DueDate     time.Time `json:"due_date"`
	Status      string    `json:"status"` // open, in_progress, completed
}

// MeetingSummary represents a summary of a meeting.
type MeetingSummary struct {
	ID          string                 `json:"id"`
	MeetingID   string                 `json:"meeting_id"`
	Title       string                 `json:"title"`
	Attendees   []string               `json:"attendees"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time"`
	Agenda      string                 `json:"agenda"`
	Summary     string                 `json:"summary"`
	Decisions   []string               `json:"decisions"`
	ActionItems []MeetingActionItem    `json:"action_items"`
	NextMeeting *time.Time             `json:"next_meeting,omitempty"`
	RecordedAt  time.Time              `json:"recorded_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}
