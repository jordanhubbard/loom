package motivation

import (
	"time"
)

// DefaultMotivations returns the built-in motivations for all agent roles
func DefaultMotivations() []*Motivation {
	return []*Motivation{
		// ============================================
		// CEO Motivations
		// ============================================
		{
			Name:        "System Idle - Strategic Review",
			Description: "When the entire system is idle, the CEO wakes to review strategic direction and create new initiatives",
			Type:        MotivationTypeIdle,
			Condition:   ConditionSystemIdle,
			AgentRole:   "ceo",
			WakeAgent:   true,
			CreateBeadOnTrigger: true,
			BeadTemplate: "strategic-review",
			Priority:    90,
			CooldownPeriod: 4 * time.Hour,
			Parameters: map[string]interface{}{
				"idle_duration": "30m",
			},
			IsBuiltIn: true,
		},
		{
			Name:        "Decision Pending - Executive Approval",
			Description: "Wake CEO when decisions require executive approval",
			Type:        MotivationTypeEvent,
			Condition:   ConditionDecisionPending,
			AgentRole:   "ceo",
			WakeAgent:   true,
			Priority:    95,
			CooldownPeriod: 5 * time.Minute,
			IsBuiltIn: true,
		},
		{
			Name:        "Quarterly Business Review",
			Description: "CEO conducts quarterly business review at quarter boundaries",
			Type:        MotivationTypeCalendar,
			Condition:   ConditionQuarterBoundary,
			AgentRole:   "ceo",
			WakeAgent:   true,
			CreateBeadOnTrigger: true,
			BeadTemplate: "quarterly-review",
			Priority:    80,
			CooldownPeriod: 80 * 24 * time.Hour, // ~3 months
			IsBuiltIn: true,
		},

		// ============================================
		// CFO Motivations
		// ============================================
		{
			Name:        "Budget Threshold Exceeded",
			Description: "Alert CFO when spending exceeds budget thresholds",
			Type:        MotivationTypeThreshold,
			Condition:   ConditionCostExceeded,
			AgentRole:   "cfo",
			WakeAgent:   true,
			CreateBeadOnTrigger: true,
			BeadTemplate: "cost-analysis",
			Priority:    85,
			CooldownPeriod: 1 * time.Hour,
			Parameters: map[string]interface{}{
				"period": "daily",
			},
			IsBuiltIn: true,
		},
		{
			Name:        "Monthly Financial Review",
			Description: "CFO conducts monthly financial review at month boundaries",
			Type:        MotivationTypeCalendar,
			Condition:   ConditionMonthBoundary,
			AgentRole:   "cfo",
			WakeAgent:   true,
			CreateBeadOnTrigger: true,
			BeadTemplate: "financial-review",
			Priority:    75,
			CooldownPeriod: 25 * 24 * time.Hour, // ~1 month
			IsBuiltIn: true,
		},
		{
			Name:        "System Idle - Cost Optimization",
			Description: "When system is idle, CFO reviews cost optimization opportunities",
			Type:        MotivationTypeIdle,
			Condition:   ConditionSystemIdle,
			AgentRole:   "cfo",
			WakeAgent:   true,
			Priority:    50,
			CooldownPeriod: 6 * time.Hour,
			Parameters: map[string]interface{}{
				"idle_duration": "45m",
			},
			IsBuiltIn: true,
		},

		// ============================================
		// Project Manager (TPM) Motivations
		// ============================================
		{
			Name:        "Deadline Approaching",
			Description: "Alert PM when beads or milestones are approaching their deadline",
			Type:        MotivationTypeCalendar,
			Condition:   ConditionDeadlineApproach,
			AgentRole:   "project-manager",
			WakeAgent:   true,
			Priority:    80,
			CooldownPeriod: 2 * time.Hour,
			Parameters: map[string]interface{}{
				"days_threshold": 7,
			},
			IsBuiltIn: true,
		},
		{
			Name:        "Deadline Passed",
			Description: "Alert PM when beads or milestones are overdue",
			Type:        MotivationTypeCalendar,
			Condition:   ConditionDeadlinePassed,
			AgentRole:   "project-manager",
			WakeAgent:   true,
			CreateBeadOnTrigger: true,
			BeadTemplate: "overdue-review",
			Priority:    90,
			CooldownPeriod: 4 * time.Hour,
			IsBuiltIn: true,
		},
		{
			Name:        "Velocity Drop Detected",
			Description: "Alert PM when team velocity drops significantly",
			Type:        MotivationTypeThreshold,
			Condition:   ConditionVelocityDrop,
			AgentRole:   "project-manager",
			WakeAgent:   true,
			Priority:    70,
			CooldownPeriod: 24 * time.Hour,
			Parameters: map[string]interface{}{
				"threshold_percent": 20,
			},
			IsBuiltIn: true,
		},

		// ============================================
		// Engineering Manager Motivations
		// ============================================
		{
			Name:        "Deadline Approaching - Technical",
			Description: "Engineering manager reviews approaching technical deadlines",
			Type:        MotivationTypeCalendar,
			Condition:   ConditionDeadlineApproach,
			AgentRole:   "engineering-manager",
			WakeAgent:   true,
			Priority:    75,
			CooldownPeriod: 4 * time.Hour,
			Parameters: map[string]interface{}{
				"days_threshold": 5,
			},
			IsBuiltIn: true,
		},
		{
			Name:        "Test Failure Detected",
			Description: "Alert EM when test failures occur in CI/CD",
			Type:        MotivationTypeThreshold,
			Condition:   ConditionTestFailure,
			AgentRole:   "engineering-manager",
			WakeAgent:   true,
			Priority:    85,
			CooldownPeriod: 30 * time.Minute,
			IsBuiltIn: true,
		},
		{
			Name:        "Coverage Drop Detected",
			Description: "Alert EM when test coverage drops below threshold",
			Type:        MotivationTypeThreshold,
			Condition:   ConditionCoverageDropped,
			AgentRole:   "engineering-manager",
			WakeAgent:   true,
			Priority:    60,
			CooldownPeriod: 24 * time.Hour,
			Parameters: map[string]interface{}{
				"threshold_percent": 80,
			},
			IsBuiltIn: true,
		},

		// ============================================
		// QA Engineer Motivations
		// ============================================
		{
			Name:        "Bead Completed - QA Review",
			Description: "QA reviews completed features for testing",
			Type:        MotivationTypeEvent,
			Condition:   ConditionBeadCompleted,
			AgentRole:   "qa-engineer",
			WakeAgent:   true,
			Priority:    70,
			CooldownPeriod: 10 * time.Minute,
			IsBuiltIn: true,
		},
		{
			Name:        "Release Approaching - QA Sweep",
			Description: "QA conducts comprehensive testing before releases",
			Type:        MotivationTypeCalendar,
			Condition:   ConditionDeadlineApproach,
			AgentRole:   "qa-engineer",
			WakeAgent:   true,
			CreateBeadOnTrigger: true,
			BeadTemplate: "qa-sweep",
			Priority:    80,
			CooldownPeriod: 24 * time.Hour,
			Parameters: map[string]interface{}{
				"days_threshold": 3,
			},
			IsBuiltIn: true,
		},
		{
			Name:        "Test Failure - Investigation",
			Description: "QA investigates test failures",
			Type:        MotivationTypeThreshold,
			Condition:   ConditionTestFailure,
			AgentRole:   "qa-engineer",
			WakeAgent:   true,
			Priority:    85,
			CooldownPeriod: 15 * time.Minute,
			IsBuiltIn: true,
		},

		// ============================================
		// Public Relations Manager Motivations
		// ============================================
		{
			Name:        "Release Published - Announcement",
			Description: "PR prepares announcements for published releases",
			Type:        MotivationTypeEvent,
			Condition:   ConditionReleasePublished,
			AgentRole:   "public-relations-manager",
			WakeAgent:   true,
			CreateBeadOnTrigger: true,
			BeadTemplate: "release-announcement",
			Priority:    80,
			CooldownPeriod: 1 * time.Hour,
			IsBuiltIn: true,
		},
		{
			Name:        "GitHub Issue - Community Response",
			Description: "PR monitors and responds to community issues",
			Type:        MotivationTypeExternal,
			Condition:   ConditionGitHubIssueOpened,
			AgentRole:   "public-relations-manager",
			WakeAgent:   true,
			Priority:    60,
			CooldownPeriod: 30 * time.Minute,
			IsBuiltIn: true,
		},
		{
			Name:        "GitHub Comment - Community Engagement",
			Description: "PR engages with community comments",
			Type:        MotivationTypeExternal,
			Condition:   ConditionGitHubCommentAdded,
			AgentRole:   "public-relations-manager",
			WakeAgent:   true,
			Priority:    50,
			CooldownPeriod: 15 * time.Minute,
			IsBuiltIn: true,
		},

		// ============================================
		// Product Manager Motivations
		// ============================================
		{
			Name:        "Milestone Complete - Feature Review",
			Description: "PM reviews completed milestones and plans next iteration",
			Type:        MotivationTypeEvent,
			Condition:   ConditionBeadCompleted,
			AgentRole:   "product-manager",
			WakeAgent:   true,
			Priority:    70,
			CooldownPeriod: 2 * time.Hour,
			Parameters: map[string]interface{}{
				"milestone_only": true,
			},
			IsBuiltIn: true,
		},
		{
			Name:        "Quarterly Planning",
			Description: "PM conducts quarterly product planning",
			Type:        MotivationTypeCalendar,
			Condition:   ConditionQuarterBoundary,
			AgentRole:   "product-manager",
			WakeAgent:   true,
			CreateBeadOnTrigger: true,
			BeadTemplate: "product-roadmap",
			Priority:    75,
			CooldownPeriod: 80 * 24 * time.Hour,
			IsBuiltIn: true,
		},
		{
			Name:        "GitHub Issue - Feature Request Triage",
			Description: "PM triages incoming feature requests from GitHub",
			Type:        MotivationTypeExternal,
			Condition:   ConditionGitHubIssueOpened,
			AgentRole:   "product-manager",
			WakeAgent:   true,
			Priority:    65,
			CooldownPeriod: 1 * time.Hour,
			IsBuiltIn: true,
		},

		// ============================================
		// DevOps Engineer Motivations
		// ============================================
		{
			Name:        "Release Approaching - Infrastructure Prep",
			Description: "DevOps prepares infrastructure for upcoming releases",
			Type:        MotivationTypeCalendar,
			Condition:   ConditionDeadlineApproach,
			AgentRole:   "devops-engineer",
			WakeAgent:   true,
			Priority:    80,
			CooldownPeriod: 12 * time.Hour,
			Parameters: map[string]interface{}{
				"days_threshold": 2,
			},
			IsBuiltIn: true,
		},
		{
			Name:        "Test Failure - Pipeline Investigation",
			Description: "DevOps investigates CI/CD pipeline failures",
			Type:        MotivationTypeThreshold,
			Condition:   ConditionTestFailure,
			AgentRole:   "devops-engineer",
			WakeAgent:   true,
			Priority:    90,
			CooldownPeriod: 15 * time.Minute,
			IsBuiltIn: true,
		},
		{
			Name:        "System Idle - Infrastructure Maintenance",
			Description: "DevOps performs maintenance during idle periods",
			Type:        MotivationTypeIdle,
			Condition:   ConditionSystemIdle,
			AgentRole:   "devops-engineer",
			WakeAgent:   true,
			Priority:    40,
			CooldownPeriod: 8 * time.Hour,
			Parameters: map[string]interface{}{
				"idle_duration": "1h",
			},
			IsBuiltIn: true,
		},

		// ============================================
		// Documentation Manager Motivations
		// ============================================
		{
			Name:        "Feature Completed - Documentation Update",
			Description: "Docs manager updates documentation for completed features",
			Type:        MotivationTypeEvent,
			Condition:   ConditionBeadCompleted,
			AgentRole:   "documentation-manager",
			WakeAgent:   true,
			Priority:    60,
			CooldownPeriod: 30 * time.Minute,
			IsBuiltIn: true,
		},
		{
			Name:        "Release Approaching - Docs Review",
			Description: "Docs manager reviews documentation before releases",
			Type:        MotivationTypeCalendar,
			Condition:   ConditionDeadlineApproach,
			AgentRole:   "documentation-manager",
			WakeAgent:   true,
			CreateBeadOnTrigger: true,
			BeadTemplate: "docs-review",
			Priority:    70,
			CooldownPeriod: 24 * time.Hour,
			Parameters: map[string]interface{}{
				"days_threshold": 5,
			},
			IsBuiltIn: true,
		},
		{
			Name:        "System Idle - Documentation Improvements",
			Description: "Docs manager improves documentation during idle periods",
			Type:        MotivationTypeIdle,
			Condition:   ConditionSystemIdle,
			AgentRole:   "documentation-manager",
			WakeAgent:   true,
			Priority:    30,
			CooldownPeriod: 4 * time.Hour,
			Parameters: map[string]interface{}{
				"idle_duration": "45m",
			},
			IsBuiltIn: true,
		},

		// ============================================
		// Code Reviewer Motivations
		// ============================================
		{
			Name:        "Pull Request Opened - Code Review",
			Description: "Code reviewer is triggered when new PRs are opened",
			Type:        MotivationTypeExternal,
			Condition:   ConditionGitHubPROpened,
			AgentRole:   "code-reviewer",
			WakeAgent:   true,
			Priority:    85,
			CooldownPeriod: 5 * time.Minute,
			IsBuiltIn: true,
		},
		{
			Name:        "Bead In Progress - Review Check",
			Description: "Code reviewer monitors in-progress work for review opportunities",
			Type:        MotivationTypeEvent,
			Condition:   ConditionBeadStatusChanged,
			AgentRole:   "code-reviewer",
			WakeAgent:   true,
			Priority:    50,
			CooldownPeriod: 30 * time.Minute,
			Parameters: map[string]interface{}{
				"target_status": "in_progress",
			},
			IsBuiltIn: true,
		},

		// ============================================
		// Housekeeping Bot Motivations
		// ============================================
		{
			Name:        "System Idle - Cleanup",
			Description: "Housekeeping bot performs cleanup during idle periods",
			Type:        MotivationTypeIdle,
			Condition:   ConditionSystemIdle,
			AgentRole:   "housekeeping-bot",
			WakeAgent:   true,
			CreateBeadOnTrigger: true,
			BeadTemplate: "system-cleanup",
			Priority:    20,
			CooldownPeriod: 2 * time.Hour,
			Parameters: map[string]interface{}{
				"idle_duration": "20m",
			},
			IsBuiltIn: true,
		},
		{
			Name:        "Daily Maintenance",
			Description: "Housekeeping bot performs daily maintenance tasks",
			Type:        MotivationTypeCalendar,
			Condition:   ConditionScheduledInterval,
			AgentRole:   "housekeeping-bot",
			WakeAgent:   true,
			Priority:    25,
			CooldownPeriod: 24 * time.Hour,
			Parameters: map[string]interface{}{
				"interval": "24h",
			},
			IsBuiltIn: true,
		},

		// ============================================
		// Decision Maker Motivations
		// ============================================
		{
			Name:        "Decision Pending - Resolution",
			Description: "Decision maker is triggered when decisions are pending",
			Type:        MotivationTypeEvent,
			Condition:   ConditionDecisionPending,
			AgentRole:   "decision-maker",
			WakeAgent:   true,
			Priority:    85,
			CooldownPeriod: 10 * time.Minute,
			IsBuiltIn: true,
		},
		{
			Name:        "Project Idle - Decision Review",
			Description: "Decision maker reviews pending decisions during project idle",
			Type:        MotivationTypeIdle,
			Condition:   ConditionProjectIdle,
			AgentRole:   "decision-maker",
			WakeAgent:   true,
			Priority:    60,
			CooldownPeriod: 1 * time.Hour,
			IsBuiltIn: true,
		},
	}
}

// RegisterDefaults registers all default motivations with the registry
func RegisterDefaults(registry *Registry) error {
	defaults := DefaultMotivations()
	for _, m := range defaults {
		if err := registry.Register(m); err != nil {
			// Skip duplicates silently - they may already be registered
			continue
		}
	}
	return nil
}

// GetMotivationsByRole returns default motivations for a specific agent role
func GetMotivationsByRole(role string) []*Motivation {
	defaults := DefaultMotivations()
	result := make([]*Motivation, 0)
	for _, m := range defaults {
		if m.AgentRole == role {
			result = append(result, m)
		}
	}
	return result
}

// ListAllRoles returns all agent roles that have default motivations
func ListAllRoles() []string {
	roleSet := make(map[string]bool)
	for _, m := range DefaultMotivations() {
		if m.AgentRole != "" {
			roleSet[m.AgentRole] = true
		}
	}

	roles := make([]string, 0, len(roleSet))
	for role := range roleSet {
		roles = append(roles, role)
	}
	return roles
}
