package motivation

import (
	"time"
)

// MotivationType represents the category of motivation trigger
type MotivationType string

const (
	// MotivationTypeCalendar triggers based on time/dates (deadlines, schedules)
	MotivationTypeCalendar MotivationType = "calendar"

	// MotivationTypeEvent triggers based on system events (bead changes, decisions)
	MotivationTypeEvent MotivationType = "event"

	// MotivationTypeExternal triggers based on external events (GitHub, webhooks)
	MotivationTypeExternal MotivationType = "external"

	// MotivationTypeThreshold triggers when metrics exceed thresholds (cost, coverage)
	MotivationTypeThreshold MotivationType = "threshold"

	// MotivationTypeIdle triggers when system is idle for a period
	MotivationTypeIdle MotivationType = "idle"
)

// TriggerCondition represents when a motivation should fire
type TriggerCondition string

const (
	// Calendar-based conditions
	ConditionTimeReached       TriggerCondition = "time_reached"        // Specific datetime reached
	ConditionDeadlineApproach  TriggerCondition = "deadline_approach"   // Within N days of deadline
	ConditionDeadlinePassed    TriggerCondition = "deadline_passed"     // Deadline has passed
	ConditionScheduledInterval TriggerCondition = "scheduled_interval"  // Every N hours/days
	ConditionQuarterBoundary   TriggerCondition = "quarter_boundary"    // Start of calendar quarter
	ConditionMonthBoundary     TriggerCondition = "month_boundary"      // Start of month

	// Event-based conditions
	ConditionBeadCreated       TriggerCondition = "bead_created"
	ConditionBeadStatusChanged TriggerCondition = "bead_status_changed"
	ConditionBeadCompleted     TriggerCondition = "bead_completed"
	ConditionDecisionPending   TriggerCondition = "decision_pending"
	ConditionDecisionResolved  TriggerCondition = "decision_resolved"
	ConditionReleasePublished  TriggerCondition = "release_published"

	// External conditions
	ConditionGitHubIssueOpened   TriggerCondition = "github_issue_opened"
	ConditionGitHubCommentAdded  TriggerCondition = "github_comment_added"
	ConditionGitHubPROpened      TriggerCondition = "github_pr_opened"
	ConditionWebhookReceived     TriggerCondition = "webhook_received"

	// Threshold conditions
	ConditionCostExceeded       TriggerCondition = "cost_exceeded"
	ConditionCoverageDropped    TriggerCondition = "coverage_dropped"
	ConditionTestFailure        TriggerCondition = "test_failure"
	ConditionVelocityDrop       TriggerCondition = "velocity_drop"

	// Idle conditions
	ConditionSystemIdle         TriggerCondition = "system_idle"
	ConditionAgentIdle          TriggerCondition = "agent_idle"
	ConditionProjectIdle        TriggerCondition = "project_idle"
)

// MotivationStatus represents the current state of a motivation
type MotivationStatus string

const (
	MotivationStatusActive   MotivationStatus = "active"
	MotivationStatusDisabled MotivationStatus = "disabled"
	MotivationStatusCooldown MotivationStatus = "cooldown"
	MotivationStatusFired    MotivationStatus = "fired"
)

// Motivation represents a trigger that can wake an agent or create work
type Motivation struct {
	ID          string           `json:"id" db:"id"`
	Name        string           `json:"name" db:"name"`
	Description string           `json:"description" db:"description"`
	Type        MotivationType   `json:"type" db:"type"`
	Condition   TriggerCondition `json:"condition" db:"condition"`
	Status      MotivationStatus `json:"status" db:"status"`

	// Targeting
	AgentRole  string `json:"agent_role,omitempty" db:"agent_role"`   // Role that should respond (e.g., "ceo", "cfo")
	AgentID    string `json:"agent_id,omitempty" db:"agent_id"`       // Specific agent (if any)
	ProjectID  string `json:"project_id,omitempty" db:"project_id"`   // Project scope (empty = global)

	// Configuration
	Parameters map[string]interface{} `json:"parameters,omitempty"` // Condition-specific params

	// Timing
	CooldownPeriod   time.Duration  `json:"cooldown_period" db:"cooldown_period"`     // Min time between triggers
	LastTriggeredAt  *time.Time     `json:"last_triggered_at,omitempty" db:"last_triggered_at"`
	NextTriggerAt    *time.Time     `json:"next_trigger_at,omitempty" db:"next_trigger_at"` // For scheduled motivations
	TriggerCount     int            `json:"trigger_count" db:"trigger_count"`

	// Priority
	Priority int `json:"priority" db:"priority"` // Higher = more important (0-100)

	// Behavior
	CreateBeadOnTrigger bool   `json:"create_bead_on_trigger" db:"create_bead_on_trigger"` // Create a stimulus bead
	BeadTemplate        string `json:"bead_template,omitempty" db:"bead_template"`         // Template for stimulus bead
	WakeAgent           bool   `json:"wake_agent" db:"wake_agent"`                         // Directly wake the target agent

	// Metadata
	IsBuiltIn  bool       `json:"is_built_in" db:"is_built_in"`   // True for default motivations
	CreatedAt  time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" db:"updated_at"`
	DisabledAt *time.Time `json:"disabled_at,omitempty" db:"disabled_at"`
}

// MotivationTrigger represents a fired motivation event
type MotivationTrigger struct {
	ID           string                 `json:"id"`
	MotivationID string                 `json:"motivation_id"`
	Motivation   *Motivation            `json:"motivation,omitempty"`
	TriggeredAt  time.Time              `json:"triggered_at"`
	TriggerData  map[string]interface{} `json:"trigger_data,omitempty"` // Context that caused trigger
	Result       TriggerResult          `json:"result"`
	Error        string                 `json:"error,omitempty"`

	// What happened
	BeadCreated   string `json:"bead_created,omitempty"`    // ID of stimulus bead if created
	AgentWoken    string `json:"agent_woken,omitempty"`     // ID of agent that was woken
	WorkflowID    string `json:"workflow_id,omitempty"`     // Temporal workflow if started
}

// TriggerResult represents the outcome of a motivation trigger
type TriggerResult string

const (
	TriggerResultSuccess   TriggerResult = "success"
	TriggerResultSkipped   TriggerResult = "skipped"    // Condition not met
	TriggerResultCooldown  TriggerResult = "cooldown"   // In cooldown period
	TriggerResultNoTarget  TriggerResult = "no_target"  // No agent available
	TriggerResultError     TriggerResult = "error"
)

// MotivationConfig holds configuration for the motivation engine
type MotivationConfig struct {
	EvaluationInterval time.Duration `json:"evaluation_interval"` // How often to check motivations
	DefaultCooldown    time.Duration `json:"default_cooldown"`    // Default cooldown period
	MaxTriggersPerTick int           `json:"max_triggers_per_tick"` // Prevent trigger storms
	IdleThreshold      time.Duration `json:"idle_threshold"`      // When system is considered idle
	EnabledByDefault   bool          `json:"enabled_by_default"`  // New motivations enabled by default
}

// DefaultConfig returns sensible defaults for the motivation engine
func DefaultConfig() *MotivationConfig {
	return &MotivationConfig{
		EvaluationInterval: 30 * time.Second,
		DefaultCooldown:    5 * time.Minute,
		MaxTriggersPerTick: 10,
		IdleThreshold:      30 * time.Minute,
		EnabledByDefault:   true,
	}
}
