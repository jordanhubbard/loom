package motivation

import (
	"testing"
	"time"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry(nil)
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if r.Count() != 0 {
		t.Errorf("expected empty registry, got %d", r.Count())
	}
}

func TestRegisterMotivation(t *testing.T) {
	r := NewRegistry(nil)

	m := &Motivation{
		Name:      "Test Motivation",
		Type:      MotivationTypeCalendar,
		Condition: ConditionDeadlineApproach,
		AgentRole: "project-manager",
		Priority:  50,
	}

	err := r.Register(m)
	if err != nil {
		t.Fatalf("failed to register motivation: %v", err)
	}

	if m.ID == "" {
		t.Error("expected motivation to have generated ID")
	}
	if m.Status != MotivationStatusActive {
		t.Errorf("expected active status, got %s", m.Status)
	}
	if r.Count() != 1 {
		t.Errorf("expected 1 motivation, got %d", r.Count())
	}
}

func TestRegisterDuplicate(t *testing.T) {
	r := NewRegistry(nil)

	m := &Motivation{
		ID:   "test-1",
		Name: "Test Motivation",
	}

	if err := r.Register(m); err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	// Try to register again with same ID
	m2 := &Motivation{
		ID:   "test-1",
		Name: "Another Motivation",
	}
	err := r.Register(m2)
	if err == nil {
		t.Error("expected error for duplicate ID")
	}
}

func TestGetMotivation(t *testing.T) {
	r := NewRegistry(nil)

	m := &Motivation{
		ID:   "test-get",
		Name: "Test Motivation",
	}
	_ = r.Register(m)

	// Get existing
	got, err := r.Get("test-get")
	if err != nil {
		t.Fatalf("failed to get motivation: %v", err)
	}
	if got.Name != "Test Motivation" {
		t.Errorf("expected 'Test Motivation', got '%s'", got.Name)
	}

	// Get non-existing
	_, err = r.Get("non-existent")
	if err == nil {
		t.Error("expected error for non-existent motivation")
	}
}

func TestListByRole(t *testing.T) {
	r := NewRegistry(nil)

	// Register motivations for different roles
	_ = r.Register(&Motivation{Name: "CEO 1", AgentRole: "ceo"})
	_ = r.Register(&Motivation{Name: "CEO 2", AgentRole: "ceo"})
	_ = r.Register(&Motivation{Name: "CFO 1", AgentRole: "cfo"})
	_ = r.Register(&Motivation{Name: "Global", AgentRole: ""}) // Global

	ceoMotivations := r.ListByRole("ceo")
	if len(ceoMotivations) != 3 { // 2 CEO + 1 Global
		t.Errorf("expected 3 motivations for ceo, got %d", len(ceoMotivations))
	}

	cfoMotivations := r.ListByRole("cfo")
	if len(cfoMotivations) != 2 { // 1 CFO + 1 Global
		t.Errorf("expected 2 motivations for cfo, got %d", len(cfoMotivations))
	}
}

func TestEnableDisable(t *testing.T) {
	r := NewRegistry(nil)

	m := &Motivation{
		ID:   "test-enable",
		Name: "Test",
	}
	_ = r.Register(m)

	// Disable
	if err := r.Disable("test-enable"); err != nil {
		t.Fatalf("failed to disable: %v", err)
	}

	got, _ := r.Get("test-enable")
	if got.Status != MotivationStatusDisabled {
		t.Errorf("expected disabled status, got %s", got.Status)
	}
	if got.DisabledAt == nil {
		t.Error("expected DisabledAt to be set")
	}

	// Enable
	if err := r.Enable("test-enable"); err != nil {
		t.Fatalf("failed to enable: %v", err)
	}

	got, _ = r.Get("test-enable")
	if got.Status != MotivationStatusActive {
		t.Errorf("expected active status, got %s", got.Status)
	}
	if got.DisabledAt != nil {
		t.Error("expected DisabledAt to be nil")
	}
}

func TestUnregister(t *testing.T) {
	r := NewRegistry(nil)

	m := &Motivation{
		ID:        "test-unreg",
		Name:      "Test",
		AgentRole: "ceo",
	}
	_ = r.Register(m)

	if err := r.Unregister("test-unreg"); err != nil {
		t.Fatalf("failed to unregister: %v", err)
	}

	if r.Count() != 0 {
		t.Errorf("expected 0 motivations, got %d", r.Count())
	}

	// Verify removed from role index
	ceoMotivations := r.ListByRole("ceo")
	for _, mot := range ceoMotivations {
		if mot.ID == "test-unreg" {
			t.Error("motivation still in role index after unregister")
		}
	}
}

func TestCooldownTracking(t *testing.T) {
	config := &MotivationConfig{
		DefaultCooldown:  100 * time.Millisecond,
		EnabledByDefault: true,
	}
	r := NewRegistry(config)

	m := &Motivation{
		ID:   "test-cooldown",
		Name: "Test",
	}
	_ = r.Register(m)

	// Record a trigger
	now := time.Now()
	trigger := &MotivationTrigger{
		ID:           "trigger-1",
		MotivationID: "test-cooldown",
		TriggeredAt:  now,
		Result:       TriggerResultSuccess,
	}
	r.RecordTrigger(trigger)

	// Check it's in cooldown
	got, _ := r.Get("test-cooldown")
	if got.Status != MotivationStatusCooldown {
		t.Errorf("expected cooldown status, got %s", got.Status)
	}

	// Wait for cooldown to expire
	time.Sleep(150 * time.Millisecond)
	r.CheckCooldowns()

	got, _ = r.Get("test-cooldown")
	if got.Status != MotivationStatusActive {
		t.Errorf("expected active status after cooldown, got %s", got.Status)
	}
}

func TestTriggerHistory(t *testing.T) {
	r := NewRegistry(nil)

	m := &Motivation{ID: "test-hist", Name: "Test"}
	_ = r.Register(m)

	// Record multiple triggers
	for i := 0; i < 5; i++ {
		trigger := &MotivationTrigger{
			ID:           "trigger-" + string(rune('a'+i)),
			MotivationID: "test-hist",
			TriggeredAt:  time.Now(),
			Result:       TriggerResultSuccess,
		}
		r.RecordTrigger(trigger)
	}

	// Get history
	history := r.GetTriggerHistory(3)
	if len(history) != 3 {
		t.Errorf("expected 3 triggers in history, got %d", len(history))
	}
}

func TestFilters(t *testing.T) {
	r := NewRegistry(nil)

	_ = r.Register(&Motivation{Name: "A", Type: MotivationTypeCalendar, AgentRole: "ceo"})
	_ = r.Register(&Motivation{Name: "B", Type: MotivationTypeEvent, AgentRole: "ceo"})
	_ = r.Register(&Motivation{Name: "C", Type: MotivationTypeCalendar, AgentRole: "cfo"})

	// Filter by type
	filters := &MotivationFilters{Type: MotivationTypeCalendar}
	result := r.List(filters)
	if len(result) != 2 {
		t.Errorf("expected 2 calendar motivations, got %d", len(result))
	}

	// Filter by role
	filters = &MotivationFilters{AgentRole: "ceo"}
	result = r.List(filters)
	if len(result) != 2 {
		t.Errorf("expected 2 ceo motivations, got %d", len(result))
	}

	// Filter by both
	filters = &MotivationFilters{Type: MotivationTypeCalendar, AgentRole: "ceo"}
	result = r.List(filters)
	if len(result) != 1 {
		t.Errorf("expected 1 motivation, got %d", len(result))
	}
}
