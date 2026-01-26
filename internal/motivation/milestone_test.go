package motivation

import (
	"testing"
	"time"
)

func TestMilestoneDaysRemaining(t *testing.T) {
	// Milestone due in 10 days
	m := &Milestone{
		DueDate: time.Now().Add(10 * 24 * time.Hour),
	}

	days := m.DaysRemaining()
	if days < 9 || days > 10 {
		t.Errorf("expected ~10 days remaining, got %d", days)
	}

	// Overdue milestone
	m.DueDate = time.Now().Add(-5 * 24 * time.Hour)
	days = m.DaysRemaining()
	if days > -4 || days < -6 {
		t.Errorf("expected ~-5 days remaining (overdue), got %d", days)
	}
}

func TestMilestoneIsOverdue(t *testing.T) {
	// Future milestone
	m := &Milestone{
		DueDate: time.Now().Add(10 * 24 * time.Hour),
		Status:  MilestoneStatusPlanned,
	}
	if m.IsOverdue() {
		t.Error("expected future milestone to not be overdue")
	}

	// Past milestone, not complete
	m.DueDate = time.Now().Add(-5 * 24 * time.Hour)
	if !m.IsOverdue() {
		t.Error("expected past incomplete milestone to be overdue")
	}

	// Past milestone, but complete
	m.Status = MilestoneStatusComplete
	if m.IsOverdue() {
		t.Error("expected completed milestone to not be overdue")
	}
}

func TestMilestoneUrgencyLevel(t *testing.T) {
	tests := []struct {
		name     string
		daysOut  int
		status   MilestoneStatus
		expected UrgencyLevel
	}{
		{"Complete milestone", 1, MilestoneStatusComplete, UrgencyLevelNone},
		{"Cancelled milestone", 1, MilestoneStatusCancelled, UrgencyLevelNone},
		{"Overdue", -5, MilestoneStatusPlanned, UrgencyLevelCritical},
		{"Due today", 0, MilestoneStatusPlanned, UrgencyLevelCritical},
		{"Due in 2 days", 2, MilestoneStatusPlanned, UrgencyLevelCritical},
		{"Due in 5 days", 5, MilestoneStatusPlanned, UrgencyLevelHigh},
		{"Due in 10 days", 10, MilestoneStatusPlanned, UrgencyLevelMedium},
		{"Due in 20 days", 20, MilestoneStatusPlanned, UrgencyLevelLow},
		{"Due in 60 days", 60, MilestoneStatusPlanned, UrgencyLevelNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Milestone{
				DueDate: time.Now().Add(time.Duration(tt.daysOut) * 24 * time.Hour),
				Status:  tt.status,
			}
			got := m.GetUrgencyLevel()
			if got != tt.expected {
				t.Errorf("expected urgency %s, got %s", tt.expected, got)
			}
		})
	}
}

func TestMilestoneProgress(t *testing.T) {
	p := &MilestoneProgress{
		TotalBeads:      10,
		CompletedBeads:  7,
		InProgressBeads: 2,
		BlockedBeads:    1,
	}

	p.CalculatePercentComplete()
	if p.PercentComplete != 70.0 {
		t.Errorf("expected 70%% complete, got %.1f%%", p.PercentComplete)
	}

	// Edge case: no beads
	p2 := &MilestoneProgress{TotalBeads: 0}
	p2.CalculatePercentComplete()
	if p2.PercentComplete != 100.0 {
		t.Errorf("expected 100%% for empty milestone, got %.1f%%", p2.PercentComplete)
	}
}

func TestDefaultDeadlineThresholds(t *testing.T) {
	thresholds := DefaultDeadlineThresholds()
	
	if thresholds[UrgencyLevelCritical] != 3 {
		t.Errorf("expected critical threshold of 3, got %d", thresholds[UrgencyLevelCritical])
	}
	if thresholds[UrgencyLevelHigh] != 7 {
		t.Errorf("expected high threshold of 7, got %d", thresholds[UrgencyLevelHigh])
	}
	if thresholds[UrgencyLevelMedium] != 14 {
		t.Errorf("expected medium threshold of 14, got %d", thresholds[UrgencyLevelMedium])
	}
	if thresholds[UrgencyLevelLow] != 30 {
		t.Errorf("expected low threshold of 30, got %d", thresholds[UrgencyLevelLow])
	}
}
