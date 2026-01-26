package motivation

import (
	"testing"
)

func TestDefaultMotivationsCount(t *testing.T) {
	defaults := DefaultMotivations()

	// We should have a good number of default motivations
	if len(defaults) < 30 {
		t.Errorf("expected at least 30 default motivations, got %d", len(defaults))
	}
}

func TestDefaultMotivationsHaveRequiredFields(t *testing.T) {
	defaults := DefaultMotivations()

	for _, m := range defaults {
		if m.Name == "" {
			t.Error("motivation missing name")
		}
		if m.Type == "" {
			t.Errorf("motivation %s missing type", m.Name)
		}
		if m.Condition == "" {
			t.Errorf("motivation %s missing condition", m.Name)
		}
		if m.AgentRole == "" {
			t.Errorf("motivation %s missing agent role", m.Name)
		}
		if !m.IsBuiltIn {
			t.Errorf("motivation %s should be marked as built-in", m.Name)
		}
	}
}

func TestDefaultMotivationsRoles(t *testing.T) {
	roles := ListAllRoles()

	expectedRoles := []string{
		"ceo",
		"cfo",
		"project-manager",
		"engineering-manager",
		"qa-engineer",
		"public-relations-manager",
		"product-manager",
		"devops-engineer",
		"documentation-manager",
		"code-reviewer",
		"housekeeping-bot",
		"decision-maker",
	}

	roleSet := make(map[string]bool)
	for _, r := range roles {
		roleSet[r] = true
	}

	for _, expected := range expectedRoles {
		if !roleSet[expected] {
			t.Errorf("expected role %s not found in default motivations", expected)
		}
	}
}

func TestGetMotivationsByRole(t *testing.T) {
	testCases := []struct {
		role         string
		minExpected  int
	}{
		{"ceo", 3},           // System idle, decision pending, quarterly
		{"cfo", 3},           // Budget, monthly, idle
		{"project-manager", 3}, // Deadline approach, deadline passed, velocity
		{"qa-engineer", 3},   // Bead completed, release approaching, test failure
		{"housekeeping-bot", 2}, // System idle, daily
	}

	for _, tc := range testCases {
		t.Run(tc.role, func(t *testing.T) {
			motivations := GetMotivationsByRole(tc.role)
			if len(motivations) < tc.minExpected {
				t.Errorf("expected at least %d motivations for %s, got %d",
					tc.minExpected, tc.role, len(motivations))
			}

			// Verify all returned motivations are for this role
			for _, m := range motivations {
				if m.AgentRole != tc.role {
					t.Errorf("motivation %s has wrong role: expected %s, got %s",
						m.Name, tc.role, m.AgentRole)
				}
			}
		})
	}
}

func TestRegisterDefaults(t *testing.T) {
	registry := NewRegistry(nil)

	err := RegisterDefaults(registry)
	if err != nil {
		t.Fatalf("RegisterDefaults failed: %v", err)
	}

	// Check that motivations were registered
	count := registry.Count()
	if count < 30 {
		t.Errorf("expected at least 30 registered motivations, got %d", count)
	}

	// Check that we can retrieve by role
	ceoMotivations := registry.ListByRole("ceo")
	if len(ceoMotivations) < 3 {
		t.Errorf("expected at least 3 CEO motivations, got %d", len(ceoMotivations))
	}
}

func TestDefaultMotivationsCooldowns(t *testing.T) {
	defaults := DefaultMotivations()

	for _, m := range defaults {
		if m.CooldownPeriod <= 0 {
			t.Errorf("motivation %s has invalid cooldown: %v", m.Name, m.CooldownPeriod)
		}
	}
}

func TestDefaultMotivationsPriorities(t *testing.T) {
	defaults := DefaultMotivations()

	for _, m := range defaults {
		if m.Priority < 0 || m.Priority > 100 {
			t.Errorf("motivation %s has invalid priority: %d (should be 0-100)",
				m.Name, m.Priority)
		}
	}
}

func TestCEOHighPriorityOnDecisions(t *testing.T) {
	motivations := GetMotivationsByRole("ceo")

	for _, m := range motivations {
		if m.Condition == ConditionDecisionPending {
			if m.Priority < 90 {
				t.Errorf("CEO decision motivation should have high priority (>=90), got %d",
					m.Priority)
			}
			return
		}
	}

	t.Error("CEO should have a decision pending motivation")
}

func TestIdleMotivationsHaveIdleDuration(t *testing.T) {
	defaults := DefaultMotivations()

	for _, m := range defaults {
		if m.Type == MotivationTypeIdle && m.Condition == ConditionSystemIdle {
			if m.Parameters == nil {
				t.Errorf("idle motivation %s should have parameters", m.Name)
				continue
			}
			if _, ok := m.Parameters["idle_duration"]; !ok {
				t.Errorf("idle motivation %s should have idle_duration parameter", m.Name)
			}
		}
	}
}

func TestDeadlineMotivationsHaveDaysThreshold(t *testing.T) {
	defaults := DefaultMotivations()

	for _, m := range defaults {
		if m.Condition == ConditionDeadlineApproach {
			if m.Parameters == nil {
				t.Errorf("deadline motivation %s should have parameters", m.Name)
				continue
			}
			if _, ok := m.Parameters["days_threshold"]; !ok {
				t.Errorf("deadline motivation %s should have days_threshold parameter", m.Name)
			}
		}
	}
}
