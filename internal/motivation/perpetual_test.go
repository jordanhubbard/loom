package motivation

import (
	"testing"
	"time"
)

func TestPerpetualTaskMotivations(t *testing.T) {
	motivations := PerpetualTaskMotivations()

	if len(motivations) == 0 {
		t.Fatal("Expected perpetual task motivations, got none")
	}

	// Check that all perpetual tasks have required fields
	for _, m := range motivations {
		if m.Name == "" {
			t.Error("Perpetual task missing name")
		}
		if m.Type != MotivationTypeCalendar {
			t.Errorf("Perpetual task %s has wrong type: %s (expected calendar)", m.Name, m.Type)
		}
		if m.Condition != ConditionScheduledInterval {
			t.Errorf("Perpetual task %s has wrong condition: %s (expected scheduled_interval)", m.Name, m.Condition)
		}
		if m.AgentRole == "" {
			t.Errorf("Perpetual task %s missing agent role", m.Name)
		}
		if !m.WakeAgent {
			t.Errorf("Perpetual task %s should wake agent", m.Name)
		}
		if !m.CreateBeadOnTrigger {
			t.Errorf("Perpetual task %s should create bead on trigger", m.Name)
		}
		if m.BeadTemplate == "" {
			t.Errorf("Perpetual task %s missing bead template", m.Name)
		}
		if m.CooldownPeriod == 0 {
			t.Errorf("Perpetual task %s missing cooldown period", m.Name)
		}
		if !m.IsBuiltIn {
			t.Errorf("Perpetual task %s should be marked as built-in", m.Name)
		}
		if taskType, ok := m.Parameters["task_type"].(string); !ok || taskType != "perpetual" {
			t.Errorf("Perpetual task %s missing task_type=perpetual parameter", m.Name)
		}
		if _, ok := m.Parameters["interval"]; !ok {
			t.Errorf("Perpetual task %s missing interval parameter", m.Name)
		}
	}

	t.Logf("Successfully validated %d perpetual task motivations", len(motivations))
}

func TestGetPerpetualTasksByRole(t *testing.T) {
	testCases := []struct {
		role          string
		expectedCount int
	}{
		{"cfo", 2},
		{"qa-engineer", 3},
		{"public-relations-manager", 2},
		{"documentation-manager", 2},
		{"devops-engineer", 2},
		{"project-manager", 2},
		{"housekeeping-bot", 2},
		{"nonexistent-role", 0},
	}

	for _, tc := range testCases {
		t.Run(tc.role, func(t *testing.T) {
			tasks := GetPerpetualTasksByRole(tc.role)
			if len(tasks) != tc.expectedCount {
				t.Errorf("Expected %d perpetual tasks for role %s, got %d",
					tc.expectedCount, tc.role, len(tasks))
			}

			// Verify all returned tasks have the correct role
			for _, task := range tasks {
				if task.AgentRole != tc.role {
					t.Errorf("Task %s has wrong role: %s (expected %s)",
						task.Name, task.AgentRole, tc.role)
				}
			}
		})
	}
}

func TestPerpetualTaskIntervals(t *testing.T) {
	motivations := PerpetualTaskMotivations()

	hourlyTasks := 0
	dailyTasks := 0
	weeklyTasks := 0

	for _, m := range motivations {
		interval, ok := m.Parameters["interval"].(string)
		if !ok {
			t.Errorf("Task %s has invalid interval parameter", m.Name)
			continue
		}

		duration, err := time.ParseDuration(interval)
		if err != nil {
			t.Errorf("Task %s has unparseable interval: %s", m.Name, interval)
			continue
		}

		switch {
		case duration < 2*time.Hour:
			hourlyTasks++
		case duration < 48*time.Hour:
			dailyTasks++
		default:
			weeklyTasks++
		}
	}

	t.Logf("Perpetual tasks by frequency: %d hourly, %d daily, %d weekly",
		hourlyTasks, dailyTasks, weeklyTasks)

	if hourlyTasks == 0 {
		t.Error("Expected some hourly perpetual tasks")
	}
	if dailyTasks == 0 {
		t.Error("Expected some daily perpetual tasks")
	}
	if weeklyTasks == 0 {
		t.Error("Expected some weekly perpetual tasks")
	}
}

func TestPerpetualTaskPriorities(t *testing.T) {
	motivations := PerpetualTaskMotivations()

	for _, m := range motivations {
		if m.Priority < 0 || m.Priority > 100 {
			t.Errorf("Task %s has invalid priority: %d (should be 0-100)", m.Name, m.Priority)
		}
	}
}

func TestRegisterPerpetualTasks(t *testing.T) {
	registry := NewRegistry(nil)

	err := RegisterPerpetualTasks(registry)
	if err != nil {
		t.Fatalf("Failed to register perpetual tasks: %v", err)
	}

	// Check that tasks were registered
	allMotivations := registry.List(nil)
	if len(allMotivations) == 0 {
		t.Fatal("No motivations registered")
	}

	// Count perpetual tasks (those with task_type=perpetual)
	perpetualCount := 0
	for _, m := range allMotivations {
		if taskType, ok := m.Parameters["task_type"].(string); ok && taskType == "perpetual" {
			perpetualCount++
		}
	}

	expected := len(PerpetualTaskMotivations())
	if perpetualCount != expected {
		t.Errorf("Expected %d perpetual tasks registered, got %d", expected, perpetualCount)
	}

	t.Logf("Successfully registered %d perpetual task motivations", perpetualCount)
}

func TestPerpetualTaskCooldowns(t *testing.T) {
	motivations := PerpetualTaskMotivations()

	for _, m := range motivations {
		interval, ok := m.Parameters["interval"].(string)
		if !ok {
			t.Errorf("Task %s missing interval", m.Name)
			continue
		}

		intervalDuration, err := time.ParseDuration(interval)
		if err != nil {
			t.Errorf("Task %s has invalid interval: %s", m.Name, interval)
			continue
		}

		if m.CooldownPeriod > intervalDuration {
			t.Errorf("Task %s has cooldown (%v) greater than interval (%v)",
				m.Name, m.CooldownPeriod, intervalDuration)
		}

		// Cooldown should be at least 90% of interval
		minCooldown := time.Duration(float64(intervalDuration) * 0.9)
		if m.CooldownPeriod < minCooldown {
			t.Errorf("Task %s has cooldown (%v) too short for interval (%v)",
				m.Name, m.CooldownPeriod, intervalDuration)
		}
	}
}

func TestPerpetualTaskRolesCoverage(t *testing.T) {
	// Verify that all key roles have perpetual tasks
	requiredRoles := []string{
		"cfo",
		"qa-engineer",
		"public-relations-manager",
		"documentation-manager",
	}

	for _, role := range requiredRoles {
		tasks := GetPerpetualTasksByRole(role)
		if len(tasks) == 0 {
			t.Errorf("Required role %s has no perpetual tasks", role)
		}
	}
}
