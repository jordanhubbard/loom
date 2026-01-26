package motivation

import (
	"time"
)

// MilestoneType represents the category of milestone
type MilestoneType string

const (
	MilestoneTypeRelease        MilestoneType = "release"
	MilestoneTypeSprintEnd      MilestoneType = "sprint_end"
	MilestoneTypeQuarterReview  MilestoneType = "quarterly_review"
	MilestoneTypeAnnualReview   MilestoneType = "annual_review"
	MilestoneTypeCustom         MilestoneType = "custom"
)

// MilestoneStatus represents the current state of a milestone
type MilestoneStatus string

const (
	MilestoneStatusPlanned    MilestoneStatus = "planned"
	MilestoneStatusInProgress MilestoneStatus = "in_progress"
	MilestoneStatusComplete   MilestoneStatus = "complete"
	MilestoneStatusMissed     MilestoneStatus = "missed"
	MilestoneStatusCancelled  MilestoneStatus = "cancelled"
)

// UrgencyLevel represents how urgent a deadline is
type UrgencyLevel string

const (
	UrgencyLevelNone     UrgencyLevel = "none"     // > 30 days
	UrgencyLevelLow      UrgencyLevel = "low"      // 14-30 days
	UrgencyLevelMedium   UrgencyLevel = "medium"   // 7-14 days
	UrgencyLevelHigh     UrgencyLevel = "high"     // 3-7 days
	UrgencyLevelCritical UrgencyLevel = "critical" // < 3 days or overdue
)

// Milestone represents a project milestone with a deadline
type Milestone struct {
	ID          string          `json:"id" db:"id"`
	ProjectID   string          `json:"project_id" db:"project_id"`
	Name        string          `json:"name" db:"name"`
	Description string          `json:"description" db:"description"`
	Type        MilestoneType   `json:"type" db:"type"`
	Status      MilestoneStatus `json:"status" db:"status"`
	
	// Timing
	DueDate     time.Time   `json:"due_date" db:"due_date"`
	StartDate   *time.Time  `json:"start_date,omitempty" db:"start_date"`
	CompletedAt *time.Time  `json:"completed_at,omitempty" db:"completed_at"`

	// Relationships
	ParentID    string   `json:"parent_id,omitempty" db:"parent_id"` // Parent milestone (for hierarchical deadlines)
	BeadIDs     []string `json:"bead_ids,omitempty"`                 // Associated beads
	DependsOn   []string `json:"depends_on,omitempty"`               // Milestone IDs this depends on

	// Metadata
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// DaysRemaining returns the number of days until the milestone is due
// Returns negative if overdue
func (m *Milestone) DaysRemaining() int {
	now := time.Now()
	return int(m.DueDate.Sub(now).Hours() / 24)
}

// IsOverdue returns true if the milestone is past its due date
func (m *Milestone) IsOverdue() bool {
	return m.DueDate.Before(time.Now()) && m.Status != MilestoneStatusComplete
}

// GetUrgencyLevel returns the urgency level based on days remaining
func (m *Milestone) GetUrgencyLevel() UrgencyLevel {
	if m.Status == MilestoneStatusComplete || m.Status == MilestoneStatusCancelled {
		return UrgencyLevelNone
	}

	days := m.DaysRemaining()
	
	if days < 0 {
		return UrgencyLevelCritical // Overdue
	}
	if days <= 3 {
		return UrgencyLevelCritical
	}
	if days <= 7 {
		return UrgencyLevelHigh
	}
	if days <= 14 {
		return UrgencyLevelMedium
	}
	if days <= 30 {
		return UrgencyLevelLow
	}
	return UrgencyLevelNone
}

// DeadlineInfo contains computed deadline information
type DeadlineInfo struct {
	DueDate       time.Time    `json:"due_date"`
	DaysRemaining int          `json:"days_remaining"`
	UrgencyLevel  UrgencyLevel `json:"urgency_level"`
	IsOverdue     bool         `json:"is_overdue"`
	MilestoneID   string       `json:"milestone_id,omitempty"`
	MilestoneName string       `json:"milestone_name,omitempty"`
}

// CalendarEvent represents an event for calendar visualization
type CalendarEvent struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Type        string       `json:"type"` // "milestone", "deadline", "motivation_trigger"
	Date        time.Time    `json:"date"`
	ProjectID   string       `json:"project_id,omitempty"`
	EntityID    string       `json:"entity_id"`    // Milestone ID, Bead ID, or Motivation ID
	UrgencyLevel UrgencyLevel `json:"urgency_level,omitempty"`
	IsRecurring bool         `json:"is_recurring"`
	Interval    string       `json:"interval,omitempty"` // For recurring: "daily", "weekly", "monthly"
}

// MilestoneProgress represents progress toward a milestone
type MilestoneProgress struct {
	MilestoneID   string  `json:"milestone_id"`
	TotalBeads    int     `json:"total_beads"`
	CompletedBeads int    `json:"completed_beads"`
	InProgressBeads int   `json:"in_progress_beads"`
	BlockedBeads  int     `json:"blocked_beads"`
	PercentComplete float64 `json:"percent_complete"`
	OnTrack       bool    `json:"on_track"` // Based on velocity vs. remaining time
	EstimatedCompletion *time.Time `json:"estimated_completion,omitempty"`
}

// CalculatePercentComplete calculates the completion percentage
func (p *MilestoneProgress) CalculatePercentComplete() {
	if p.TotalBeads == 0 {
		p.PercentComplete = 100.0
		return
	}
	p.PercentComplete = float64(p.CompletedBeads) / float64(p.TotalBeads) * 100.0
}

// DefaultDeadlineThresholds returns the default day thresholds for urgency levels
func DefaultDeadlineThresholds() map[UrgencyLevel]int {
	return map[UrgencyLevel]int{
		UrgencyLevelCritical: 3,
		UrgencyLevelHigh:     7,
		UrgencyLevelMedium:   14,
		UrgencyLevelLow:      30,
	}
}
