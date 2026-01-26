package motivation

import (
	"testing"
	"time"
)

// MockIdleDataProvider implements IdleDataProvider for testing
type MockIdleDataProvider struct {
	agentStates   map[string]AgentActivityState
	beadStates    map[string]int
	projectStates map[string]ProjectActivityState
}

func NewMockIdleDataProvider() *MockIdleDataProvider {
	return &MockIdleDataProvider{
		agentStates:   make(map[string]AgentActivityState),
		beadStates:    make(map[string]int),
		projectStates: make(map[string]ProjectActivityState),
	}
}

func (p *MockIdleDataProvider) GetAgentStates() map[string]AgentActivityState {
	return p.agentStates
}

func (p *MockIdleDataProvider) GetBeadStates() map[string]int {
	return p.beadStates
}

func (p *MockIdleDataProvider) GetProjectStates() map[string]ProjectActivityState {
	return p.projectStates
}

func TestDefaultIdleConfig(t *testing.T) {
	config := DefaultIdleConfig()

	if config.SystemIdleThreshold != 30*time.Minute {
		t.Errorf("expected system idle threshold 30m, got %v", config.SystemIdleThreshold)
	}

	if config.ProjectIdleThreshold != 15*time.Minute {
		t.Errorf("expected project idle threshold 15m, got %v", config.ProjectIdleThreshold)
	}

	if config.AgentIdleThreshold != 5*time.Minute {
		t.Errorf("expected agent idle threshold 5m, got %v", config.AgentIdleThreshold)
	}
}

func TestIdleDetectorCreation(t *testing.T) {
	detector := NewIdleDetector(nil)
	if detector == nil {
		t.Fatal("expected non-nil detector")
	}

	config := detector.GetConfig()
	if config == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestIdleDetectorSystemIdle(t *testing.T) {
	config := &IdleConfig{
		SystemIdleThreshold:  100 * time.Millisecond,
		ProjectIdleThreshold: 50 * time.Millisecond,
		AgentIdleThreshold:   25 * time.Millisecond,
		CheckInterval:        10 * time.Millisecond,
	}

	detector := NewIdleDetector(config)
	provider := NewMockIdleDataProvider()

	// Initially record activity
	detector.RecordAgentActivity("agent-1")

	// Check immediately - should not be idle
	isIdle, duration := detector.IsSystemIdle(provider)
	if isIdle {
		t.Error("expected system to not be idle immediately")
	}
	if duration > 0 {
		t.Errorf("expected zero duration, got %v", duration)
	}

	// Wait for idle threshold
	time.Sleep(150 * time.Millisecond)

	// Now should be idle
	isIdle, duration = detector.IsSystemIdle(provider)
	if !isIdle {
		t.Error("expected system to be idle after threshold")
	}
	if duration < 100*time.Millisecond {
		t.Errorf("expected duration >= 100ms, got %v", duration)
	}
}

func TestIdleDetectorWithWorkingAgents(t *testing.T) {
	config := &IdleConfig{
		SystemIdleThreshold:  50 * time.Millisecond,
		ProjectIdleThreshold: 50 * time.Millisecond,
		AgentIdleThreshold:   25 * time.Millisecond,
		CheckInterval:        10 * time.Millisecond,
	}

	detector := NewIdleDetector(config)
	provider := NewMockIdleDataProvider()

	// Add a working agent
	provider.agentStates["agent-1"] = AgentActivityState{
		AgentID:    "agent-1",
		Status:     "working",
		LastActive: time.Now(),
		ProjectID:  "proj-1",
	}

	// Wait past threshold
	time.Sleep(100 * time.Millisecond)

	// Should NOT be idle because an agent is working
	state := detector.CheckIdleState(provider)
	if state.IsSystemIdle {
		t.Error("expected system to not be idle when agents are working")
	}
	if state.WorkingAgents != 1 {
		t.Errorf("expected 1 working agent, got %d", state.WorkingAgents)
	}
}

func TestIdleDetectorProjectIdle(t *testing.T) {
	config := &IdleConfig{
		SystemIdleThreshold:  100 * time.Millisecond,
		ProjectIdleThreshold: 50 * time.Millisecond,
		AgentIdleThreshold:   25 * time.Millisecond,
		CheckInterval:        10 * time.Millisecond,
	}

	detector := NewIdleDetector(config)
	provider := NewMockIdleDataProvider()

	// Add project with old last activity
	provider.projectStates["proj-1"] = ProjectActivityState{
		ProjectID:        "proj-1",
		LastActivity:     time.Now().Add(-100 * time.Millisecond),
		ActiveAgentCount: 2,
		OpenBeadCount:    5,
	}

	// Check project idle
	isIdle, duration := detector.IsProjectIdle("proj-1", provider)
	if !isIdle {
		t.Error("expected project to be idle")
	}
	if duration < 50*time.Millisecond {
		t.Errorf("expected duration >= 50ms, got %v", duration)
	}
}

func TestIdleDetectorRecordActivity(t *testing.T) {
	detector := NewIdleDetector(nil)

	// Record various activities
	detector.RecordAgentActivity("agent-1")
	detector.RecordBeadActivity("bead-1")
	detector.RecordSystemEvent()

	// Check that times were updated (basic smoke test)
	state := detector.CheckIdleState(nil)
	if state.LastAgentActivity.IsZero() {
		t.Error("expected last agent activity to be set")
	}
	if state.LastBeadActivity.IsZero() {
		t.Error("expected last bead activity to be set")
	}
}

func TestIdleDetectorGetIdleAgentIDs(t *testing.T) {
	config := &IdleConfig{
		SystemIdleThreshold:  100 * time.Millisecond,
		ProjectIdleThreshold: 50 * time.Millisecond,
		AgentIdleThreshold:   50 * time.Millisecond,
		CheckInterval:        10 * time.Millisecond,
	}

	detector := NewIdleDetector(config)
	provider := NewMockIdleDataProvider()

	// Add agents with different idle times
	now := time.Now()
	provider.agentStates["agent-idle"] = AgentActivityState{
		AgentID:    "agent-idle",
		Status:     "idle",
		LastActive: now.Add(-100 * time.Millisecond), // Idle for 100ms
		ProjectID:  "proj-1",
	}
	provider.agentStates["agent-recent"] = AgentActivityState{
		AgentID:    "agent-recent",
		Status:     "idle",
		LastActive: now, // Just became idle
		ProjectID:  "proj-1",
	}
	provider.agentStates["agent-working"] = AgentActivityState{
		AgentID:    "agent-working",
		Status:     "working",
		LastActive: now.Add(-100 * time.Millisecond),
		ProjectID:  "proj-1",
	}

	idleAgents := detector.GetIdleAgentIDs(provider)

	// Should only return agent-idle (idle status + past threshold)
	if len(idleAgents) != 1 {
		t.Errorf("expected 1 idle agent, got %d", len(idleAgents))
	}
	if len(idleAgents) > 0 && idleAgents[0] != "agent-idle" {
		t.Errorf("expected agent-idle, got %s", idleAgents[0])
	}
}

func TestIdleDetectorUpdateConfig(t *testing.T) {
	detector := NewIdleDetector(nil)

	newConfig := &IdleConfig{
		SystemIdleThreshold:  1 * time.Hour,
		ProjectIdleThreshold: 30 * time.Minute,
		AgentIdleThreshold:   10 * time.Minute,
		CheckInterval:        1 * time.Minute,
	}

	detector.UpdateConfig(newConfig)

	config := detector.GetConfig()
	if config.SystemIdleThreshold != 1*time.Hour {
		t.Errorf("expected 1h, got %v", config.SystemIdleThreshold)
	}
}

func TestIdleStateAgentCounts(t *testing.T) {
	detector := NewIdleDetector(nil)
	provider := NewMockIdleDataProvider()

	provider.agentStates["a1"] = AgentActivityState{Status: "idle"}
	provider.agentStates["a2"] = AgentActivityState{Status: "idle"}
	provider.agentStates["a3"] = AgentActivityState{Status: "working"}
	provider.agentStates["a4"] = AgentActivityState{Status: "paused"}

	state := detector.CheckIdleState(provider)

	if state.TotalAgents != 4 {
		t.Errorf("expected 4 total agents, got %d", state.TotalAgents)
	}
	if state.IdleAgents != 2 {
		t.Errorf("expected 2 idle agents, got %d", state.IdleAgents)
	}
	if state.WorkingAgents != 1 {
		t.Errorf("expected 1 working agent, got %d", state.WorkingAgents)
	}
	if state.PausedAgents != 1 {
		t.Errorf("expected 1 paused agent, got %d", state.PausedAgents)
	}
}

func TestIdleStateBeadCounts(t *testing.T) {
	detector := NewIdleDetector(nil)
	provider := NewMockIdleDataProvider()

	provider.beadStates["open"] = 10
	provider.beadStates["in_progress"] = 3
	provider.beadStates["closed"] = 50

	state := detector.CheckIdleState(provider)

	if state.TotalBeads != 63 {
		t.Errorf("expected 63 total beads, got %d", state.TotalBeads)
	}
	if state.OpenBeads != 10 {
		t.Errorf("expected 10 open beads, got %d", state.OpenBeads)
	}
	if state.InProgressBeads != 3 {
		t.Errorf("expected 3 in_progress beads, got %d", state.InProgressBeads)
	}
}
