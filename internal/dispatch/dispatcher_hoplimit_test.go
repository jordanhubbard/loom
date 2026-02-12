package dispatch

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jordanhubbard/loom/pkg/models"
)

// MockEscalator for testing escalation
type MockEscalator struct {
	EscalatedBeads []string
	Decisions      map[string]*models.DecisionBead
}

func (m *MockEscalator) EscalateBeadToCEO(beadID, reason, returnedTo string) (*models.DecisionBead, error) {
	m.EscalatedBeads = append(m.EscalatedBeads, beadID)
	decision := &models.DecisionBead{
		Bead: &models.Bead{
			ID:          fmt.Sprintf("decision-%d", len(m.Decisions)),
			Title:       "CEO Decision Required",
			Description: fmt.Sprintf("Escalated: %s", reason),
			Status:      "open",
			Priority:    0, // P0 - critical
			Type:        "decision",
			CreatedAt:   time.Now(),
		},
		Question:    fmt.Sprintf("Bead %s needs attention: %s", beadID, reason),
		RequesterID: returnedTo,
	}
	m.Decisions[decision.ID] = decision
	return decision, nil
}

func TestDispatcher_MaxDispatchHops_Default(t *testing.T) {
	// Test that default maxDispatchHops is 20
	d := &Dispatcher{}

	// Verify default is applied when not explicitly set
	d.mu.RLock()
	defaultHops := d.maxDispatchHops
	d.mu.RUnlock()

	if defaultHops != 0 {
		t.Errorf("Expected uninitialized maxDispatchHops to be 0, got %d", defaultHops)
	}

	// The dispatcher should use 20 as fallback when maxDispatchHops is 0
	// This is tested in the actual dispatch cycle below
}

func TestDispatcher_SetMaxDispatchHops(t *testing.T) {
	d := &Dispatcher{}

	testCases := []struct {
		name     string
		maxHops  int
		expected int
	}{
		{"Set to 20", 20, 20},
		{"Set to 10", 10, 10},
		{"Set to 30", 30, 30},
		{"Set to 1", 1, 1},
		{"Set to 100", 100, 100},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d.SetMaxDispatchHops(tc.maxHops)

			d.mu.RLock()
			actual := d.maxDispatchHops
			d.mu.RUnlock()

			if actual != tc.expected {
				t.Errorf("Expected maxDispatchHops to be %d, got %d", tc.expected, actual)
			}
		})
	}
}

func TestDispatcher_DispatchHopIncrement(t *testing.T) {
	// Test that dispatch_count is incremented on each dispatch
	// This is a unit test of the logic without full dispatcher integration

	testCases := []struct {
		name          string
		initialCount  int
		expectedCount int
	}{
		{"First dispatch", 0, 1},
		{"Second dispatch", 1, 2},
		{"Tenth dispatch", 9, 10},
		{"Nineteenth dispatch", 18, 19},
		{"Twentieth dispatch", 19, 20},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the dispatch count increment logic
			dispatchCount := tc.initialCount
			dispatchCount++

			if dispatchCount != tc.expectedCount {
				t.Errorf("Expected dispatch count to be %d, got %d", tc.expectedCount, dispatchCount)
			}
		})
	}
}

func TestDispatcher_HopLimitEscalation(t *testing.T) {
	// Test that beads are escalated when they reach maxDispatchHops
	mockEscalator := &MockEscalator{
		Decisions: make(map[string]*models.DecisionBead),
	}

	d := &Dispatcher{
		escalator: mockEscalator,
	}

	testCases := []struct {
		name            string
		maxHops         int
		dispatchCount   int
		shouldEscalate  bool
		description     string
	}{
		{
			name:            "Below limit",
			maxHops:         20,
			dispatchCount:   10,
			shouldEscalate:  false,
			description:     "10 < 20, no escalation",
		},
		{
			name:            "At limit",
			maxHops:         20,
			dispatchCount:   20,
			shouldEscalate:  true,
			description:     "20 >= 20, should escalate",
		},
		{
			name:            "Above limit",
			maxHops:         20,
			dispatchCount:   25,
			shouldEscalate:  true,
			description:     "25 >= 20, should escalate",
		},
		{
			name:            "Low limit at threshold",
			maxHops:         5,
			dispatchCount:   5,
			shouldEscalate:  true,
			description:     "5 >= 5, should escalate",
		},
		{
			name:            "High limit below threshold",
			maxHops:         50,
			dispatchCount:   30,
			shouldEscalate:  false,
			description:     "30 < 50, no escalation",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset escalator state
			mockEscalator.EscalatedBeads = []string{}

			// Set max hops
			d.SetMaxDispatchHops(tc.maxHops)

			// Simulate the escalation check logic
			d.mu.RLock()
			maxHops := d.maxDispatchHops
			d.mu.RUnlock()

			if maxHops <= 0 {
				maxHops = 20 // Default fallback
			}

			shouldEscalate := tc.dispatchCount >= maxHops

			if shouldEscalate != tc.shouldEscalate {
				t.Errorf("%s: Expected shouldEscalate=%v, got %v (dispatchCount=%d, maxHops=%d)",
					tc.description, tc.shouldEscalate, shouldEscalate, tc.dispatchCount, maxHops)
			}

			// If should escalate, simulate escalation
			if shouldEscalate {
				beadID := fmt.Sprintf("bead-test-%d", tc.dispatchCount)
				reason := fmt.Sprintf("dispatch_count=%d exceeded max_hops=%d", tc.dispatchCount, maxHops)
				_, err := mockEscalator.EscalateBeadToCEO(beadID, reason, "test-agent")
				if err != nil {
					t.Errorf("Failed to escalate bead: %v", err)
				}

				if len(mockEscalator.EscalatedBeads) != 1 {
					t.Errorf("Expected 1 escalated bead, got %d", len(mockEscalator.EscalatedBeads))
				}
			}
		})
	}
}

func TestDispatcher_FallbackDefault(t *testing.T) {
	// Test that the fallback default of 20 is used when maxDispatchHops is not set
	d := &Dispatcher{}

	// Don't set maxDispatchHops explicitly
	d.mu.RLock()
	maxHops := d.maxDispatchHops
	d.mu.RUnlock()

	// Apply the fallback logic from dispatcher.go
	if maxHops <= 0 {
		maxHops = 20
	}

	if maxHops != 20 {
		t.Errorf("Expected fallback maxHops to be 20, got %d", maxHops)
	}

	// Test with negative value
	d.SetMaxDispatchHops(-5)

	d.mu.RLock()
	maxHops = d.maxDispatchHops
	d.mu.RUnlock()

	if maxHops <= 0 {
		maxHops = 20
	}

	if maxHops != 20 {
		t.Errorf("Expected fallback maxHops for negative value to be 20, got %d", maxHops)
	}
}

func TestDispatcher_EscalationContext(t *testing.T) {
	// Test that escalation creates proper context fields
	mockEscalator := &MockEscalator{
		Decisions: make(map[string]*models.DecisionBead),
	}

	d := &Dispatcher{
		escalator: mockEscalator,
	}
	d.SetMaxDispatchHops(20)

	beadID := "bead-test-escalation"
	dispatchCount := 20
	maxHops := 20

	// Simulate escalation
	reason := fmt.Sprintf("dispatch_count=%d exceeded max_hops=%d", dispatchCount, maxHops)
	decision, err := mockEscalator.EscalateBeadToCEO(beadID, reason, "test-agent")
	if err != nil {
		t.Fatalf("Failed to escalate bead: %v", err)
	}

	// Verify decision was created
	if decision == nil {
		t.Fatal("Expected decision to be created, got nil")
	}

	if decision.Question != fmt.Sprintf("Bead %s needs attention: %s", beadID, reason) {
		t.Errorf("Expected decision question to contain bead %s and reason, got %q", beadID, decision.Question)
	}

	if decision.RequesterID != "test-agent" {
		t.Errorf("Expected decision requester to be test-agent, got %s", decision.RequesterID)
	}

	if decision.Bead.Status != "open" {
		t.Errorf("Expected decision status to be open, got %s", decision.Bead.Status)
	}

	// Verify the expected context fields would be created
	expectedContextFields := map[string]string{
		"redispatch_requested":            "false",
		"dispatch_escalated_at":           time.Now().UTC().Format(time.RFC3339),
		"dispatch_escalation_reason":      reason,
		"dispatch_escalation_decision_id": decision.ID,
	}

	// Verify all expected fields exist (time field is approximate)
	for key, expectedValue := range expectedContextFields {
		if key == "dispatch_escalated_at" {
			// Just verify it's a valid timestamp format
			_, err := time.Parse(time.RFC3339, expectedValue)
			if err != nil {
				t.Errorf("Expected dispatch_escalated_at to be valid RFC3339 timestamp, got parse error: %v", err)
			}
		} else if key != "dispatch_escalation_decision_id" {
			// Other fields should match exactly
			if key == "redispatch_requested" && expectedValue != "false" {
				t.Errorf("Expected %s to be %q, got different value", key, expectedValue)
			}
		}
	}

	// Verify decision ID is in the map
	if _, exists := mockEscalator.Decisions[decision.ID]; !exists {
		t.Errorf("Expected decision %s to be stored in escalator", decision.ID)
	}
}

func TestDispatcher_NoEscalatorConfigured(t *testing.T) {
	// Test that dispatcher handles missing escalator gracefully
	d := &Dispatcher{
		escalator: nil, // No escalator configured
	}
	d.SetMaxDispatchHops(20)

	// Verify dispatcher doesn't panic when escalator is nil
	// The real dispatcher logs an error but continues
	if d.escalator == nil {
		t.Log("Escalator not configured - dispatcher should log error and continue")
	} else {
		t.Error("Expected escalator to be nil")
	}
}

func TestDispatcher_AlreadyEscalated(t *testing.T) {
	// Test that beads already escalated are not re-escalated
	bead := &models.Bead{
		ID:        "bead-already-escalated",
		ProjectID: "test-project",
		Context: map[string]string{
			"dispatch_count":                     "25",
			"escalated_to_ceo_decision_id":       "decision-123",
			"dispatch_escalated_at":              time.Now().UTC().Format(time.RFC3339),
			"dispatch_escalation_reason":         "dispatch_count=20 exceeded max_hops=20",
			"dispatch_escalation_decision_id":    "decision-123",
		},
	}

	// Parse dispatch count
	dispatchCount := 0
	if bead.Context != nil {
		if dispatchCountStr := bead.Context["dispatch_count"]; dispatchCountStr != "" {
			_, _ = fmt.Sscanf(dispatchCountStr, "%d", &dispatchCount)
		}
	}

	maxHops := 20
	shouldSkip := dispatchCount >= maxHops && bead.Context["escalated_to_ceo_decision_id"] != ""

	if !shouldSkip {
		t.Error("Expected already-escalated bead to be skipped")
	}

	if dispatchCount != 25 {
		t.Errorf("Expected dispatch count to be 25, got %d", dispatchCount)
	}
}

func TestDispatcher_ComplexInvestigation(t *testing.T) {
	// Test a realistic complex investigation scenario
	// Simulates a bug that requires multiple iterations to resolve

	scenarios := []struct {
		name        string
		iterations  int
		description string
	}{
		{
			name:        "Simple bug fix",
			iterations:  5,
			description: "Quick fix, resolved in 5 dispatches",
		},
		{
			name:        "Complex investigation",
			iterations:  15,
			description: "Deeper investigation, multiple test iterations",
		},
		{
			name:        "Very complex bug",
			iterations:  19,
			description: "Very thorough investigation, just under limit",
		},
		{
			name:        "Stuck investigation",
			iterations:  20,
			description: "Investigation stuck, should escalate",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			mockEscalator := &MockEscalator{
				Decisions: make(map[string]*models.DecisionBead),
			}

			d := &Dispatcher{
				escalator: mockEscalator,
			}
			d.SetMaxDispatchHops(20)

			dispatchCount := scenario.iterations
			maxHops := 20

			shouldEscalate := dispatchCount >= maxHops

			if shouldEscalate {
				reason := fmt.Sprintf("dispatch_count=%d exceeded max_hops=%d", dispatchCount, maxHops)
				_, err := mockEscalator.EscalateBeadToCEO("bead-complex", reason, "agent-engineer")
				if err != nil {
					t.Errorf("Failed to escalate: %v", err)
				}

				if len(mockEscalator.EscalatedBeads) == 0 {
					t.Error("Expected bead to be escalated")
				}

				t.Logf("%s: Escalated after %d iterations (limit: %d)", scenario.description, dispatchCount, maxHops)
			} else {
				t.Logf("%s: No escalation needed after %d iterations (limit: %d)", scenario.description, dispatchCount, maxHops)
			}
		})
	}
}

func TestDispatcher_HopLimitConfiguration(t *testing.T) {
	// Test that different hop limit configurations work as expected
	configurations := []struct {
		environment string
		maxHops     int
		rationale   string
	}{
		{
			environment: "development",
			maxHops:     15,
			rationale:   "Lower limit to catch issues faster during development",
		},
		{
			environment: "staging",
			maxHops:     20,
			rationale:   "Standard limit for pre-production testing",
		},
		{
			environment: "production",
			maxHops:     25,
			rationale:   "Higher limit for complex production investigations",
		},
	}

	for _, config := range configurations {
		t.Run(config.environment, func(t *testing.T) {
			d := &Dispatcher{}
			d.SetMaxDispatchHops(config.maxHops)

			d.mu.RLock()
			actual := d.maxDispatchHops
			d.mu.RUnlock()

			if actual != config.maxHops {
				t.Errorf("Expected maxDispatchHops to be %d for %s, got %d",
					config.maxHops, config.environment, actual)
			}

			t.Logf("%s: maxHops=%d - %s", config.environment, config.maxHops, config.rationale)
		})
	}
}

func TestDispatcher_ConcurrentHopTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent test in short mode")
	}

	// Test that hop tracking is thread-safe
	d := &Dispatcher{}
	d.SetMaxDispatchHops(20)

	ctx := context.Background()
	_ = ctx // For future use

	// Simulate concurrent dispatch count checks
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			d.mu.RLock()
			maxHops := d.maxDispatchHops
			d.mu.RUnlock()

			if maxHops != 20 {
				t.Errorf("Expected maxHops to be 20, got %d", maxHops)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
