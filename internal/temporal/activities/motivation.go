package activities

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jordanhubbard/agenticorp/internal/motivation"
	"github.com/jordanhubbard/agenticorp/internal/temporal/eventbus"
)

// MotivationActivities provides Temporal activities for motivation operations
type MotivationActivities struct {
	engine   *motivation.Engine
	eventBus *eventbus.EventBus
}

// NewMotivationActivities creates a new motivation activities instance
func NewMotivationActivities(engine *motivation.Engine, eventBus *eventbus.EventBus) *MotivationActivities {
	return &MotivationActivities{
		engine:   engine,
		eventBus: eventBus,
	}
}

// EvaluateMotivationsActivityInput contains input for the motivation evaluation activity
type EvaluateMotivationsActivityInput struct {
	BeatCount int `json:"beat_count"` // Current heartbeat count
}

// EvaluateMotivationsActivityResult contains the result of motivation evaluation
type EvaluateMotivationsActivityResult struct {
	MotivationsEvaluated int      `json:"motivations_evaluated"`
	MotivationsFired     int      `json:"motivations_fired"`
	FiredMotivationIDs   []string `json:"fired_motivation_ids,omitempty"`
	FiredMotivationNames []string `json:"fired_motivation_names,omitempty"`
	Errors               []string `json:"errors,omitempty"`
}

// EvaluateMotivationsActivity runs one tick of the motivation engine
// This activity should be called from the AgentiCorpHeartbeatWorkflow
func (a *MotivationActivities) EvaluateMotivationsActivity(ctx context.Context, input EvaluateMotivationsActivityInput) (*EvaluateMotivationsActivityResult, error) {
	result := &EvaluateMotivationsActivityResult{
		FiredMotivationIDs:   make([]string, 0),
		FiredMotivationNames: make([]string, 0),
		Errors:               make([]string, 0),
	}

	if a.engine == nil {
		return result, nil
	}

	// Get count of active motivations
	registry := a.engine.GetRegistry()
	if registry == nil {
		return result, nil
	}

	activeMotivations := registry.GetActive()
	result.MotivationsEvaluated = len(activeMotivations)

	// Run the tick
	triggered, err := a.engine.Tick(ctx)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		log.Printf("Motivation evaluation error on beat %d: %v", input.BeatCount, err)
	}

	result.MotivationsFired = triggered

	// Get recently triggered motivation info
	history := registry.GetTriggerHistory(triggered)
	for _, trigger := range history {
		result.FiredMotivationIDs = append(result.FiredMotivationIDs, trigger.MotivationID)
		if trigger.Motivation != nil {
			result.FiredMotivationNames = append(result.FiredMotivationNames, trigger.Motivation.Name)
		}
	}

	if triggered > 0 {
		log.Printf("Motivation activity: beat=%d, evaluated=%d, fired=%d, names=%v",
			input.BeatCount, result.MotivationsEvaluated, triggered, result.FiredMotivationNames)
	}

	return result, nil
}

// TriggerMotivationActivityInput contains input for manually triggering a motivation
type TriggerMotivationActivityInput struct {
	MotivationID string `json:"motivation_id"`
}

// TriggerMotivationActivityResult contains the result of a manual trigger
type TriggerMotivationActivityResult struct {
	Success      bool   `json:"success"`
	TriggerID    string `json:"trigger_id,omitempty"`
	BeadCreated  string `json:"bead_created,omitempty"`
	AgentWoken   string `json:"agent_woken,omitempty"`
	Error        string `json:"error,omitempty"`
}

// TriggerMotivationActivity manually triggers a specific motivation
func (a *MotivationActivities) TriggerMotivationActivity(ctx context.Context, input TriggerMotivationActivityInput) (*TriggerMotivationActivityResult, error) {
	result := &TriggerMotivationActivityResult{}

	if a.engine == nil {
		result.Error = "motivation engine not initialized"
		return result, fmt.Errorf("motivation engine not initialized")
	}

	trigger, err := a.engine.ManualTrigger(ctx, input.MotivationID)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	result.Success = trigger.Result == motivation.TriggerResultSuccess
	result.TriggerID = trigger.ID
	result.BeadCreated = trigger.BeadCreated
	result.AgentWoken = trigger.AgentWoken

	return result, nil
}

// CheckDeadlinesActivityInput contains input for deadline checking
type CheckDeadlinesActivityInput struct {
	ProjectID     string `json:"project_id,omitempty"`      // Empty for all projects
	DaysThreshold int    `json:"days_threshold,omitempty"`  // Default: 7
}

// DeadlineInfo represents deadline information for a bead or milestone
type DeadlineInfo struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Type          string    `json:"type"` // "bead" or "milestone"
	ProjectID     string    `json:"project_id"`
	DueDate       time.Time `json:"due_date"`
	DaysRemaining int       `json:"days_remaining"`
	UrgencyLevel  string    `json:"urgency_level"`
	IsOverdue     bool      `json:"is_overdue"`
}

// CheckDeadlinesActivityResult contains deadline check results
type CheckDeadlinesActivityResult struct {
	UpcomingDeadlines []DeadlineInfo `json:"upcoming_deadlines"`
	OverdueItems      []DeadlineInfo `json:"overdue_items"`
	TotalUpcoming     int            `json:"total_upcoming"`
	TotalOverdue      int            `json:"total_overdue"`
}

// CheckDeadlinesActivity checks for upcoming and overdue deadlines
// This can publish deadline events to wake relevant agents
func (a *MotivationActivities) CheckDeadlinesActivity(ctx context.Context, input CheckDeadlinesActivityInput) (*CheckDeadlinesActivityResult, error) {
	result := &CheckDeadlinesActivityResult{
		UpcomingDeadlines: make([]DeadlineInfo, 0),
		OverdueItems:      make([]DeadlineInfo, 0),
	}

	// Note: This activity would query the database for deadlines
	// For now, return empty result - full implementation would use StateProvider
	
	// Publish deadline events if we have overdue items
	if len(result.OverdueItems) > 0 && a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeDeadlinePassed,
			Source:    "motivation-engine",
			ProjectID: input.ProjectID,
			Data: map[string]interface{}{
				"overdue_count": len(result.OverdueItems),
			},
		})
	}

	// Publish approaching deadline events
	if len(result.UpcomingDeadlines) > 0 && a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:      eventbus.EventTypeDeadlineApproaching,
			Source:    "motivation-engine",
			ProjectID: input.ProjectID,
			Data: map[string]interface{}{
				"upcoming_count":  len(result.UpcomingDeadlines),
				"days_threshold": input.DaysThreshold,
			},
		})
	}

	result.TotalUpcoming = len(result.UpcomingDeadlines)
	result.TotalOverdue = len(result.OverdueItems)

	return result, nil
}

// CheckSystemIdleActivityInput contains input for idle checking
type CheckSystemIdleActivityInput struct {
	IdleThresholdMinutes int `json:"idle_threshold_minutes"` // Default: 30
}

// CheckSystemIdleActivityResult contains idle check results
type CheckSystemIdleActivityResult struct {
	IsSystemIdle     bool     `json:"is_system_idle"`
	IdleAgentCount   int      `json:"idle_agent_count"`
	WorkingAgentCount int     `json:"working_agent_count"`
	OpenBeadCount    int      `json:"open_bead_count"`
	IdleDurationMins int      `json:"idle_duration_mins"`
}

// CheckSystemIdleActivity checks if the system is idle
// If idle, publishes an event to wake the CEO
func (a *MotivationActivities) CheckSystemIdleActivity(ctx context.Context, input CheckSystemIdleActivityInput) (*CheckSystemIdleActivityResult, error) {
	result := &CheckSystemIdleActivityResult{}

	// Note: Full implementation would query agent and bead states
	// For now, return non-idle result
	result.IsSystemIdle = false

	// If system is idle, publish event
	if result.IsSystemIdle && a.eventBus != nil {
		_ = a.eventBus.Publish(&eventbus.Event{
			Type:   eventbus.EventTypeSystemIdle,
			Source: "motivation-engine",
			Data: map[string]interface{}{
				"idle_duration_mins": result.IdleDurationMins,
			},
		})
	}

	return result, nil
}

// PublishMotivationFiredActivity publishes a motivation fired event
func (a *MotivationActivities) PublishMotivationFiredActivity(ctx context.Context, motivationID, motivationName, agentRole, projectID string, triggerData map[string]interface{}) error {
	if a.eventBus == nil {
		return nil
	}

	data := map[string]interface{}{
		"motivation_id":   motivationID,
		"motivation_name": motivationName,
		"agent_role":      agentRole,
	}
	for k, v := range triggerData {
		data[k] = v
	}

	return a.eventBus.Publish(&eventbus.Event{
		Type:      eventbus.EventTypeMotivationFired,
		Source:    "motivation-engine",
		ProjectID: projectID,
		Data:      data,
	})
}
