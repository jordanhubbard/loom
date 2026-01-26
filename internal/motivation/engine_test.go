package motivation

import (
	"context"
	"testing"
	"time"
)

// MockStateProvider implements StateProvider for testing
type MockStateProvider struct {
	currentTime           time.Time
	upcomingDeadlines     []BeadDeadlineInfo
	overdueBeads          []BeadDeadlineInfo
	beadsByStatus         map[string][]string
	milestones            []*Milestone
	idleAgents            []string
	agentsByRole          map[string][]string
	systemIdle            bool
	projectIdle           map[string]bool
	currentSpending       float64
	budgetThreshold       float64
	pendingDecisions      []string
	externalEvents        map[string][]ExternalEvent
}

func NewMockStateProvider() *MockStateProvider {
	return &MockStateProvider{
		currentTime:    time.Now(),
		beadsByStatus:  make(map[string][]string),
		agentsByRole:   make(map[string][]string),
		projectIdle:    make(map[string]bool),
		externalEvents: make(map[string][]ExternalEvent),
	}
}

func (m *MockStateProvider) GetCurrentTime() time.Time { return m.currentTime }

func (m *MockStateProvider) GetBeadsWithUpcomingDeadlines(withinDays int) ([]BeadDeadlineInfo, error) {
	return m.upcomingDeadlines, nil
}

func (m *MockStateProvider) GetOverdueBeads() ([]BeadDeadlineInfo, error) {
	return m.overdueBeads, nil
}

func (m *MockStateProvider) GetBeadsByStatus(status string) ([]string, error) {
	return m.beadsByStatus[status], nil
}

func (m *MockStateProvider) GetMilestones(projectID string) ([]*Milestone, error) {
	return m.milestones, nil
}

func (m *MockStateProvider) GetUpcomingMilestones(withinDays int) ([]*Milestone, error) {
	result := make([]*Milestone, 0)
	for _, ms := range m.milestones {
		if ms.DaysRemaining() <= withinDays {
			result = append(result, ms)
		}
	}
	return result, nil
}

func (m *MockStateProvider) GetIdleAgents() ([]string, error) {
	return m.idleAgents, nil
}

func (m *MockStateProvider) GetAgentsByRole(role string) ([]string, error) {
	return m.agentsByRole[role], nil
}

func (m *MockStateProvider) GetProjectIdle(projectID string, duration time.Duration) (bool, error) {
	return m.projectIdle[projectID], nil
}

func (m *MockStateProvider) GetSystemIdle(duration time.Duration) (bool, error) {
	return m.systemIdle, nil
}

func (m *MockStateProvider) GetCurrentSpending(period string) (float64, error) {
	return m.currentSpending, nil
}

func (m *MockStateProvider) GetBudgetThreshold(projectID string) (float64, error) {
	return m.budgetThreshold, nil
}

func (m *MockStateProvider) GetPendingDecisions() ([]string, error) {
	return m.pendingDecisions, nil
}

func (m *MockStateProvider) GetUnprocessedExternalEvents(eventType string) ([]ExternalEvent, error) {
	return m.externalEvents[eventType], nil
}

// MockActionHandler implements ActionHandler for testing
type MockActionHandler struct {
	beadsCreated     []string
	agentsWoken      []string
	rolesWoken       []string
	triggersPublished []*MotivationTrigger
	workflowsStarted []string
}

func NewMockActionHandler() *MockActionHandler {
	return &MockActionHandler{
		beadsCreated:     make([]string, 0),
		agentsWoken:      make([]string, 0),
		rolesWoken:       make([]string, 0),
		triggersPublished: make([]*MotivationTrigger, 0),
		workflowsStarted: make([]string, 0),
	}
}

func (h *MockActionHandler) CreateStimulusBead(motivation *Motivation, triggerData map[string]interface{}) (string, error) {
	beadID := "bead-stimulus-" + motivation.ID
	h.beadsCreated = append(h.beadsCreated, beadID)
	return beadID, nil
}

func (h *MockActionHandler) WakeAgent(agentID string, motivation *Motivation) error {
	h.agentsWoken = append(h.agentsWoken, agentID)
	return nil
}

func (h *MockActionHandler) WakeAgentsByRole(role string, motivation *Motivation) error {
	h.rolesWoken = append(h.rolesWoken, role)
	return nil
}

func (h *MockActionHandler) PublishMotivationFired(trigger *MotivationTrigger) error {
	h.triggersPublished = append(h.triggersPublished, trigger)
	return nil
}

func (h *MockActionHandler) StartWorkflow(workflowType string, input interface{}) (string, error) {
	workflowID := "wf-" + workflowType
	h.workflowsStarted = append(h.workflowsStarted, workflowID)
	return workflowID, nil
}

func TestEngineCreation(t *testing.T) {
	registry := NewRegistry(nil)
	stateProvider := NewMockStateProvider()
	actionHandler := NewMockActionHandler()

	engine := NewEngine(registry, stateProvider, actionHandler)
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}

	if engine.GetRegistry() != registry {
		t.Error("expected engine to reference the registry")
	}
}

func TestEngineDeadlineApproachMotivation(t *testing.T) {
	registry := NewRegistry(&MotivationConfig{
		EvaluationInterval: 100 * time.Millisecond,
		DefaultCooldown:    50 * time.Millisecond,
		MaxTriggersPerTick: 10,
		EnabledByDefault:   true,
	})

	stateProvider := NewMockStateProvider()
	stateProvider.upcomingDeadlines = []BeadDeadlineInfo{
		{BeadID: "bd-1", Title: "Important Task", DaysRemaining: 3, UrgencyLevel: UrgencyLevelCritical},
	}

	actionHandler := NewMockActionHandler()

	// Register motivation
	m := &Motivation{
		Name:      "Deadline Approaching",
		Type:      MotivationTypeCalendar,
		Condition: ConditionDeadlineApproach,
		AgentRole: "project-manager",
		WakeAgent: true,
		Parameters: map[string]interface{}{
			"days_threshold": 7,
		},
	}
	_ = registry.Register(m)

	engine := NewEngine(registry, stateProvider, actionHandler)

	// Run one tick
	ctx := context.Background()
	triggered, err := engine.Tick(ctx)
	if err != nil {
		t.Fatalf("tick failed: %v", err)
	}

	if triggered != 1 {
		t.Errorf("expected 1 trigger, got %d", triggered)
	}

	if len(actionHandler.rolesWoken) != 1 || actionHandler.rolesWoken[0] != "project-manager" {
		t.Errorf("expected project-manager to be woken, got %v", actionHandler.rolesWoken)
	}

	if len(actionHandler.triggersPublished) != 1 {
		t.Errorf("expected 1 trigger published, got %d", len(actionHandler.triggersPublished))
	}
}

func TestEngineSystemIdleMotivation(t *testing.T) {
	registry := NewRegistry(&MotivationConfig{
		EvaluationInterval: 100 * time.Millisecond,
		DefaultCooldown:    50 * time.Millisecond,
		MaxTriggersPerTick: 10,
		IdleThreshold:      30 * time.Minute,
		EnabledByDefault:   true,
	})

	stateProvider := NewMockStateProvider()
	stateProvider.systemIdle = true

	actionHandler := NewMockActionHandler()

	// Register CEO idle motivation
	m := &Motivation{
		Name:      "System Idle",
		Type:      MotivationTypeIdle,
		Condition: ConditionSystemIdle,
		AgentRole: "ceo",
		WakeAgent: true,
	}
	_ = registry.Register(m)

	engine := NewEngine(registry, stateProvider, actionHandler)

	ctx := context.Background()
	triggered, _ := engine.Tick(ctx)

	if triggered != 1 {
		t.Errorf("expected 1 trigger, got %d", triggered)
	}

	if len(actionHandler.rolesWoken) != 1 || actionHandler.rolesWoken[0] != "ceo" {
		t.Errorf("expected ceo to be woken, got %v", actionHandler.rolesWoken)
	}
}

func TestEngineCostExceededMotivation(t *testing.T) {
	registry := NewRegistry(&MotivationConfig{
		EvaluationInterval: 100 * time.Millisecond,
		DefaultCooldown:    50 * time.Millisecond,
		MaxTriggersPerTick: 10,
		EnabledByDefault:   true,
	})

	stateProvider := NewMockStateProvider()
	stateProvider.currentSpending = 150.0
	stateProvider.budgetThreshold = 100.0

	actionHandler := NewMockActionHandler()

	// Register CFO cost motivation
	m := &Motivation{
		Name:      "Cost Exceeded",
		Type:      MotivationTypeThreshold,
		Condition: ConditionCostExceeded,
		AgentRole: "cfo",
		WakeAgent: true,
		CreateBeadOnTrigger: true,
		Parameters: map[string]interface{}{
			"period": "daily",
		},
	}
	_ = registry.Register(m)

	engine := NewEngine(registry, stateProvider, actionHandler)

	ctx := context.Background()
	triggered, _ := engine.Tick(ctx)

	if triggered != 1 {
		t.Errorf("expected 1 trigger, got %d", triggered)
	}

	if len(actionHandler.beadsCreated) != 1 {
		t.Errorf("expected 1 bead created, got %d", len(actionHandler.beadsCreated))
	}
}

func TestEnginePendingDecisionsMotivation(t *testing.T) {
	registry := NewRegistry(&MotivationConfig{
		EvaluationInterval: 100 * time.Millisecond,
		DefaultCooldown:    50 * time.Millisecond,
		MaxTriggersPerTick: 10,
		EnabledByDefault:   true,
	})

	stateProvider := NewMockStateProvider()
	stateProvider.pendingDecisions = []string{"decision-1", "decision-2"}

	actionHandler := NewMockActionHandler()

	// Register CEO decision motivation
	m := &Motivation{
		Name:      "Decision Pending",
		Type:      MotivationTypeEvent,
		Condition: ConditionDecisionPending,
		AgentRole: "ceo",
		WakeAgent: true,
	}
	_ = registry.Register(m)

	engine := NewEngine(registry, stateProvider, actionHandler)

	ctx := context.Background()
	triggered, _ := engine.Tick(ctx)

	if triggered != 1 {
		t.Errorf("expected 1 trigger, got %d", triggered)
	}
}

func TestEngineCooldownPreventsRetrigger(t *testing.T) {
	registry := NewRegistry(&MotivationConfig{
		EvaluationInterval: 50 * time.Millisecond,
		DefaultCooldown:    200 * time.Millisecond,
		MaxTriggersPerTick: 10,
		EnabledByDefault:   true,
	})

	stateProvider := NewMockStateProvider()
	stateProvider.systemIdle = true

	actionHandler := NewMockActionHandler()

	m := &Motivation{
		Name:      "System Idle",
		Type:      MotivationTypeIdle,
		Condition: ConditionSystemIdle,
		AgentRole: "ceo",
		WakeAgent: true,
	}
	_ = registry.Register(m)

	engine := NewEngine(registry, stateProvider, actionHandler)
	ctx := context.Background()

	// First tick should trigger
	triggered1, _ := engine.Tick(ctx)
	if triggered1 != 1 {
		t.Errorf("expected 1 trigger on first tick, got %d", triggered1)
	}

	// Second tick immediately after should NOT trigger (cooldown)
	triggered2, _ := engine.Tick(ctx)
	if triggered2 != 0 {
		t.Errorf("expected 0 triggers during cooldown, got %d", triggered2)
	}

	// Wait for cooldown to expire
	time.Sleep(250 * time.Millisecond)

	// Third tick should trigger again
	triggered3, _ := engine.Tick(ctx)
	if triggered3 != 1 {
		t.Errorf("expected 1 trigger after cooldown, got %d", triggered3)
	}
}

func TestEngineMaxTriggersPerTick(t *testing.T) {
	registry := NewRegistry(&MotivationConfig{
		EvaluationInterval: 100 * time.Millisecond,
		DefaultCooldown:    50 * time.Millisecond,
		MaxTriggersPerTick: 2, // Limit to 2
		EnabledByDefault:   true,
	})

	stateProvider := NewMockStateProvider()
	stateProvider.systemIdle = true
	stateProvider.pendingDecisions = []string{"d1"}
	stateProvider.upcomingDeadlines = []BeadDeadlineInfo{{BeadID: "b1"}}

	actionHandler := NewMockActionHandler()

	// Register 3 motivations that would all fire
	_ = registry.Register(&Motivation{Name: "M1", Type: MotivationTypeIdle, Condition: ConditionSystemIdle, AgentRole: "ceo", WakeAgent: true})
	_ = registry.Register(&Motivation{Name: "M2", Type: MotivationTypeEvent, Condition: ConditionDecisionPending, AgentRole: "ceo", WakeAgent: true})
	_ = registry.Register(&Motivation{Name: "M3", Type: MotivationTypeCalendar, Condition: ConditionDeadlineApproach, AgentRole: "pm", WakeAgent: true, Parameters: map[string]interface{}{"days_threshold": 30}})

	engine := NewEngine(registry, stateProvider, actionHandler)

	ctx := context.Background()
	triggered, _ := engine.Tick(ctx)

	if triggered > 2 {
		t.Errorf("expected at most 2 triggers (max per tick), got %d", triggered)
	}
}

func TestEngineManualTrigger(t *testing.T) {
	registry := NewRegistry(nil)
	stateProvider := NewMockStateProvider()
	actionHandler := NewMockActionHandler()

	m := &Motivation{
		ID:        "test-manual",
		Name:      "Test Motivation",
		Type:      MotivationTypeCalendar,
		Condition: ConditionScheduledInterval,
		AgentRole: "ceo",
		WakeAgent: true,
	}
	_ = registry.Register(m)

	engine := NewEngine(registry, stateProvider, actionHandler)

	ctx := context.Background()
	trigger, err := engine.ManualTrigger(ctx, "test-manual")
	if err != nil {
		t.Fatalf("manual trigger failed: %v", err)
	}

	if trigger.Result != TriggerResultSuccess {
		t.Errorf("expected success result, got %s", trigger.Result)
	}

	if trigger.TriggerData["manual"] != true {
		t.Error("expected manual flag in trigger data")
	}
}
