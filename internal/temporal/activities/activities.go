package activities

import (
	"context"
	"fmt"

	"github.com/jordanhubbard/arbiter/internal/temporal/eventbus"
)

// Activities provides Temporal activities for arbiter operations
type Activities struct {
	eventBus *eventbus.EventBus
}

// NewActivities creates a new activities instance
func NewActivities(eventBus *eventbus.EventBus) *Activities {
	return &Activities{
		eventBus: eventBus,
	}
}

// PublishEventActivity publishes an event to the event bus
func (a *Activities) PublishEventActivity(ctx context.Context, event *eventbus.Event) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	return a.eventBus.Publish(event)
}

// PublishAgentEventActivity publishes an agent-related event
func (a *Activities) PublishAgentEventActivity(ctx context.Context, eventType eventbus.EventType, agentID, projectID string, data map[string]interface{}) error {
	return a.eventBus.PublishAgentEvent(eventType, agentID, projectID, data)
}

// PublishBeadEventActivity publishes a bead-related event
func (a *Activities) PublishBeadEventActivity(ctx context.Context, eventType eventbus.EventType, beadID, projectID string, data map[string]interface{}) error {
	return a.eventBus.PublishBeadEvent(eventType, beadID, projectID, data)
}

// PublishLogMessageActivity publishes a log message event
func (a *Activities) PublishLogMessageActivity(ctx context.Context, level, message, source, projectID string) error {
	return a.eventBus.PublishLogMessage(level, message, source, projectID)
}

// NotifyAgentSpawnedActivity notifies that an agent has been spawned
func (a *Activities) NotifyAgentSpawnedActivity(ctx context.Context, agentID, name, personaName, projectID string) error {
	return a.eventBus.PublishAgentEvent(
		eventbus.EventTypeAgentSpawned,
		agentID,
		projectID,
		map[string]interface{}{
			"name":         name,
			"persona_name": personaName,
		},
	)
}

// NotifyAgentStatusChangeActivity notifies of agent status change
func (a *Activities) NotifyAgentStatusChangeActivity(ctx context.Context, agentID, oldStatus, newStatus, projectID string) error {
	return a.eventBus.PublishAgentEvent(
		eventbus.EventTypeAgentStatusChange,
		agentID,
		projectID,
		map[string]interface{}{
			"old_status": oldStatus,
			"new_status": newStatus,
		},
	)
}

// NotifyBeadCreatedActivity notifies that a bead has been created
func (a *Activities) NotifyBeadCreatedActivity(ctx context.Context, beadID, title, beadType, projectID string, priority int) error {
	return a.eventBus.PublishBeadEvent(
		eventbus.EventTypeBeadCreated,
		beadID,
		projectID,
		map[string]interface{}{
			"title":    title,
			"type":     beadType,
			"priority": priority,
		},
	)
}

// NotifyBeadAssignedActivity notifies that a bead has been assigned
func (a *Activities) NotifyBeadAssignedActivity(ctx context.Context, beadID, agentID, projectID string) error {
	return a.eventBus.PublishBeadEvent(
		eventbus.EventTypeBeadAssigned,
		beadID,
		projectID,
		map[string]interface{}{
			"assigned_to": agentID,
		},
	)
}

// NotifyBeadStatusChangeActivity notifies of bead status change
func (a *Activities) NotifyBeadStatusChangeActivity(ctx context.Context, beadID, oldStatus, newStatus, projectID string) error {
	return a.eventBus.PublishBeadEvent(
		eventbus.EventTypeBeadStatusChange,
		beadID,
		projectID,
		map[string]interface{}{
			"old_status": oldStatus,
			"new_status": newStatus,
		},
	)
}

// NotifyDecisionCreatedActivity notifies that a decision has been created
func (a *Activities) NotifyDecisionCreatedActivity(ctx context.Context, decisionID, question, requesterID, projectID string) error {
	return a.eventBus.Publish(&eventbus.Event{
		Type:      eventbus.EventTypeDecisionCreated,
		Source:    "decision-manager",
		ProjectID: projectID,
		Data: map[string]interface{}{
			"decision_id":  decisionID,
			"question":     question,
			"requester_id": requesterID,
		},
	})
}

// NotifyDecisionResolvedActivity notifies that a decision has been resolved
func (a *Activities) NotifyDecisionResolvedActivity(ctx context.Context, decisionID, decision, deciderID, projectID string) error {
	return a.eventBus.Publish(&eventbus.Event{
		Type:      eventbus.EventTypeDecisionResolved,
		Source:    "decision-manager",
		ProjectID: projectID,
		Data: map[string]interface{}{
			"decision_id": decisionID,
			"decision":    decision,
			"decider_id":  deciderID,
		},
	})
}
