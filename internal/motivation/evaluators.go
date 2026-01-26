package motivation

import (
	"context"
	"time"
)

// CalendarEvaluator evaluates time-based motivations
type CalendarEvaluator struct{}

func (e *CalendarEvaluator) Evaluate(ctx context.Context, m *Motivation, state StateProvider) (bool, map[string]interface{}, error) {
	now := state.GetCurrentTime()
	data := make(map[string]interface{})

	switch m.Condition {
	case ConditionTimeReached:
		// Check if a specific time has been reached
		if m.NextTriggerAt != nil && now.After(*m.NextTriggerAt) {
			data["scheduled_time"] = m.NextTriggerAt
			return true, data, nil
		}

	case ConditionDeadlineApproach:
		// Check for beads/milestones approaching deadline
		daysThreshold := 7 // default
		if v, ok := m.Parameters["days_threshold"].(int); ok {
			daysThreshold = v
		}
		if v, ok := m.Parameters["days_threshold"].(float64); ok {
			daysThreshold = int(v)
		}

		beads, err := state.GetBeadsWithUpcomingDeadlines(daysThreshold)
		if err != nil {
			return false, nil, err
		}

		if len(beads) > 0 {
			data["approaching_deadlines"] = beads
			data["count"] = len(beads)
			data["days_threshold"] = daysThreshold
			return true, data, nil
		}

	case ConditionDeadlinePassed:
		// Check for overdue beads
		beads, err := state.GetOverdueBeads()
		if err != nil {
			return false, nil, err
		}

		if len(beads) > 0 {
			data["overdue_beads"] = beads
			data["count"] = len(beads)
			return true, data, nil
		}

	case ConditionScheduledInterval:
		// Check if enough time has passed since last trigger
		if m.LastTriggeredAt == nil {
			// Never triggered, fire now
			return true, data, nil
		}

		interval := m.CooldownPeriod // Use cooldown as the interval
		if v, ok := m.Parameters["interval"].(string); ok {
			if parsed, err := time.ParseDuration(v); err == nil {
				interval = parsed
			}
		}

		if now.Sub(*m.LastTriggeredAt) >= interval {
			data["interval"] = interval.String()
			data["last_triggered"] = m.LastTriggeredAt
			return true, data, nil
		}

	case ConditionQuarterBoundary:
		// Fire at the start of each calendar quarter
		isQuarterStart := (now.Month() == time.January || now.Month() == time.April ||
			now.Month() == time.July || now.Month() == time.October) && now.Day() <= 7

		if isQuarterStart {
			// Only fire once per quarter boundary
			if m.LastTriggeredAt == nil || now.Sub(*m.LastTriggeredAt) > 80*24*time.Hour {
				data["quarter"] = (int(now.Month())-1)/3 + 1
				data["year"] = now.Year()
				return true, data, nil
			}
		}

	case ConditionMonthBoundary:
		// Fire at the start of each month
		isMonthStart := now.Day() <= 3

		if isMonthStart {
			// Only fire once per month boundary
			if m.LastTriggeredAt == nil || now.Sub(*m.LastTriggeredAt) > 25*24*time.Hour {
				data["month"] = int(now.Month())
				data["year"] = now.Year()
				return true, data, nil
			}
		}
	}

	return false, nil, nil
}

// EventEvaluator evaluates system event-based motivations
type EventEvaluator struct{}

func (e *EventEvaluator) Evaluate(ctx context.Context, m *Motivation, state StateProvider) (bool, map[string]interface{}, error) {
	data := make(map[string]interface{})

	switch m.Condition {
	case ConditionDecisionPending:
		// Check for pending decisions
		decisions, err := state.GetPendingDecisions()
		if err != nil {
			return false, nil, err
		}

		if len(decisions) > 0 {
			data["pending_decisions"] = decisions
			data["count"] = len(decisions)
			return true, data, nil
		}

	case ConditionBeadCreated, ConditionBeadStatusChanged, ConditionBeadCompleted:
		// These are typically triggered by event bus, not polling
		// The engine will receive these via signals/events
		// For now, return false (event-driven, not poll-driven)
		return false, nil, nil

	case ConditionReleasePublished:
		// Check for recent releases (would need release tracking)
		// For now, this would be triggered by external event
		return false, nil, nil
	}

	return false, nil, nil
}

// ThresholdEvaluator evaluates metric threshold-based motivations
type ThresholdEvaluator struct{}

func (e *ThresholdEvaluator) Evaluate(ctx context.Context, m *Motivation, state StateProvider) (bool, map[string]interface{}, error) {
	data := make(map[string]interface{})

	switch m.Condition {
	case ConditionCostExceeded:
		// Check if spending exceeds budget
		period := "daily"
		if v, ok := m.Parameters["period"].(string); ok {
			period = v
		}

		currentSpending, err := state.GetCurrentSpending(period)
		if err != nil {
			return false, nil, err
		}

		threshold, err := state.GetBudgetThreshold(m.ProjectID)
		if err != nil {
			return false, nil, err
		}

		if currentSpending > threshold {
			data["current_spending"] = currentSpending
			data["threshold"] = threshold
			data["period"] = period
			data["overage"] = currentSpending - threshold
			return true, data, nil
		}

	case ConditionCoverageDropped:
		// Would need test coverage metrics integration
		return false, nil, nil

	case ConditionTestFailure:
		// Would need CI/CD integration
		return false, nil, nil

	case ConditionVelocityDrop:
		// Would need velocity tracking
		return false, nil, nil
	}

	return false, nil, nil
}

// IdleEvaluator evaluates idle-state motivations
type IdleEvaluator struct {
	idleThreshold time.Duration
}

func (e *IdleEvaluator) Evaluate(ctx context.Context, m *Motivation, state StateProvider) (bool, map[string]interface{}, error) {
	data := make(map[string]interface{})

	threshold := e.idleThreshold
	if v, ok := m.Parameters["idle_duration"].(string); ok {
		if parsed, err := time.ParseDuration(v); err == nil {
			threshold = parsed
		}
	}

	switch m.Condition {
	case ConditionSystemIdle:
		// Check if entire system is idle
		isIdle, err := state.GetSystemIdle(threshold)
		if err != nil {
			return false, nil, err
		}

		if isIdle {
			data["idle_duration"] = threshold.String()
			data["scope"] = "system"
			return true, data, nil
		}

	case ConditionAgentIdle:
		// Check for idle agents of a specific role
		if m.AgentRole != "" {
			agents, err := state.GetAgentsByRole(m.AgentRole)
			if err != nil {
				return false, nil, err
			}

			idleAgents, err := state.GetIdleAgents()
			if err != nil {
				return false, nil, err
			}

			// Find intersection
			idleRoleAgents := make([]string, 0)
			idleSet := make(map[string]bool)
			for _, a := range idleAgents {
				idleSet[a] = true
			}
			for _, a := range agents {
				if idleSet[a] {
					idleRoleAgents = append(idleRoleAgents, a)
				}
			}

			if len(idleRoleAgents) > 0 {
				data["idle_agents"] = idleRoleAgents
				data["role"] = m.AgentRole
				return true, data, nil
			}
		}

	case ConditionProjectIdle:
		// Check if a specific project is idle
		if m.ProjectID != "" {
			isIdle, err := state.GetProjectIdle(m.ProjectID, threshold)
			if err != nil {
				return false, nil, err
			}

			if isIdle {
				data["project_id"] = m.ProjectID
				data["idle_duration"] = threshold.String()
				return true, data, nil
			}
		}
	}

	return false, nil, nil
}

// ExternalEvaluator evaluates external event-based motivations (GitHub, webhooks)
type ExternalEvaluator struct{}

func (e *ExternalEvaluator) Evaluate(ctx context.Context, m *Motivation, state StateProvider) (bool, map[string]interface{}, error) {
	data := make(map[string]interface{})

	var eventType string
	switch m.Condition {
	case ConditionGitHubIssueOpened:
		eventType = "github_issue_opened"
	case ConditionGitHubCommentAdded:
		eventType = "github_comment"
	case ConditionGitHubPROpened:
		eventType = "github_pr_opened"
	case ConditionWebhookReceived:
		eventType = "webhook"
		if v, ok := m.Parameters["webhook_type"].(string); ok {
			eventType = v
		}
	default:
		return false, nil, nil
	}

	events, err := state.GetUnprocessedExternalEvents(eventType)
	if err != nil {
		return false, nil, err
	}

	if len(events) > 0 {
		data["events"] = events
		data["event_type"] = eventType
		data["count"] = len(events)
		return true, data, nil
	}

	return false, nil, nil
}
