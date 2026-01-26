package motivation

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Engine evaluates and fires motivations based on system state
type Engine struct {
	registry      *Registry
	config        *MotivationConfig
	evaluators    map[MotivationType]Evaluator
	stateProvider StateProvider
	actionHandler ActionHandler
	mu            sync.RWMutex
	running       bool
	stopCh        chan struct{}
}

// StateProvider interface for querying system state
type StateProvider interface {
	// Time-based state
	GetCurrentTime() time.Time

	// Bead state
	GetBeadsWithUpcomingDeadlines(withinDays int) ([]BeadDeadlineInfo, error)
	GetOverdueBeads() ([]BeadDeadlineInfo, error)
	GetBeadsByStatus(status string) ([]string, error)

	// Milestone state
	GetMilestones(projectID string) ([]*Milestone, error)
	GetUpcomingMilestones(withinDays int) ([]*Milestone, error)

	// Agent state
	GetIdleAgents() ([]string, error)
	GetAgentsByRole(role string) ([]string, error)

	// Project state
	GetProjectIdle(projectID string, duration time.Duration) (bool, error)
	GetSystemIdle(duration time.Duration) (bool, error)

	// Analytics state (for CFO motivations)
	GetCurrentSpending(period string) (float64, error)
	GetBudgetThreshold(projectID string) (float64, error)

	// Decision state
	GetPendingDecisions() ([]string, error)

	// External event state (for GitHub integration)
	GetUnprocessedExternalEvents(eventType string) ([]ExternalEvent, error)
}

// ActionHandler interface for executing motivation actions
type ActionHandler interface {
	// Create a stimulus bead to drive work
	CreateStimulusBead(motivation *Motivation, triggerData map[string]interface{}) (string, error)

	// Wake a specific agent
	WakeAgent(agentID string, motivation *Motivation) error

	// Wake agents by role
	WakeAgentsByRole(role string, motivation *Motivation) error

	// Publish motivation fired event
	PublishMotivationFired(trigger *MotivationTrigger) error

	// Start a Temporal workflow
	StartWorkflow(workflowType string, input interface{}) (string, error)
}

// BeadDeadlineInfo contains deadline info for a bead
type BeadDeadlineInfo struct {
	BeadID        string
	Title         string
	ProjectID     string
	DueDate       time.Time
	DaysRemaining int
	UrgencyLevel  UrgencyLevel
}

// ExternalEvent represents an event from external systems (GitHub, webhooks)
type ExternalEvent struct {
	ID        string
	Type      string // "github_issue", "github_pr", "github_comment", "webhook"
	Source    string
	Data      map[string]interface{}
	Timestamp time.Time
	Processed bool
}

// Evaluator interface for evaluating specific motivation types
type Evaluator interface {
	// Evaluate checks if a motivation should fire
	Evaluate(ctx context.Context, m *Motivation, state StateProvider) (bool, map[string]interface{}, error)
}

// NewEngine creates a new motivation engine
func NewEngine(registry *Registry, stateProvider StateProvider, actionHandler ActionHandler) *Engine {
	config := registry.Config()
	if config == nil {
		config = DefaultConfig()
	}

	e := &Engine{
		registry:      registry,
		config:        config,
		evaluators:    make(map[MotivationType]Evaluator),
		stateProvider: stateProvider,
		actionHandler: actionHandler,
		stopCh:        make(chan struct{}),
	}

	// Register default evaluators
	e.evaluators[MotivationTypeCalendar] = &CalendarEvaluator{}
	e.evaluators[MotivationTypeEvent] = &EventEvaluator{}
	e.evaluators[MotivationTypeThreshold] = &ThresholdEvaluator{}
	e.evaluators[MotivationTypeIdle] = &IdleEvaluator{idleThreshold: config.IdleThreshold}
	e.evaluators[MotivationTypeExternal] = &ExternalEvaluator{}

	return e
}

// RegisterEvaluator registers a custom evaluator for a motivation type
func (e *Engine) RegisterEvaluator(motivationType MotivationType, evaluator Evaluator) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.evaluators[motivationType] = evaluator
}

// Start begins the motivation evaluation loop
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("engine already running")
	}
	e.running = true
	e.stopCh = make(chan struct{})
	e.mu.Unlock()

	log.Printf("Motivation engine started with interval %v", e.config.EvaluationInterval)

	ticker := time.NewTicker(e.config.EvaluationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.mu.Lock()
			e.running = false
			e.mu.Unlock()
			return ctx.Err()
		case <-e.stopCh:
			e.mu.Lock()
			e.running = false
			e.mu.Unlock()
			return nil
		case <-ticker.C:
			e.tick(ctx)
		}
	}
}

// Stop stops the motivation evaluation loop
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		close(e.stopCh)
	}
}

// IsRunning returns whether the engine is running
func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// tick performs one evaluation cycle
func (e *Engine) tick(ctx context.Context) {
	// Update cooldowns first
	e.registry.CheckCooldowns()

	// Get all active motivations
	motivations := e.registry.GetActive()
	if len(motivations) == 0 {
		return
	}

	triggered := 0
	for _, m := range motivations {
		if triggered >= e.config.MaxTriggersPerTick {
			log.Printf("Max triggers per tick (%d) reached, deferring remaining", e.config.MaxTriggersPerTick)
			break
		}

		shouldFire, triggerData, err := e.evaluate(ctx, m)
		if err != nil {
			log.Printf("Error evaluating motivation %s: %v", m.ID, err)
			continue
		}

		if shouldFire {
			if err := e.fire(ctx, m, triggerData); err != nil {
				log.Printf("Error firing motivation %s: %v", m.ID, err)
			} else {
				triggered++
			}
		}
	}
}

// Tick performs a single evaluation cycle (for external callers like Temporal activities)
func (e *Engine) Tick(ctx context.Context) (int, error) {
	// Update cooldowns first
	e.registry.CheckCooldowns()

	// Get all active motivations
	motivations := e.registry.GetActive()
	if len(motivations) == 0 {
		return 0, nil
	}

	triggered := 0
	var lastErr error

	for _, m := range motivations {
		if triggered >= e.config.MaxTriggersPerTick {
			break
		}

		shouldFire, triggerData, err := e.evaluate(ctx, m)
		if err != nil {
			lastErr = err
			continue
		}

		if shouldFire {
			if err := e.fire(ctx, m, triggerData); err != nil {
				lastErr = err
			} else {
				triggered++
			}
		}
	}

	return triggered, lastErr
}

// evaluate checks if a motivation should fire
func (e *Engine) evaluate(ctx context.Context, m *Motivation) (bool, map[string]interface{}, error) {
	evaluator, ok := e.evaluators[m.Type]
	if !ok {
		return false, nil, fmt.Errorf("no evaluator for motivation type: %s", m.Type)
	}

	return evaluator.Evaluate(ctx, m, e.stateProvider)
}

// fire triggers a motivation
func (e *Engine) fire(ctx context.Context, m *Motivation, triggerData map[string]interface{}) error {
	now := time.Now()

	trigger := &MotivationTrigger{
		ID:           fmt.Sprintf("trig-%d", now.UnixNano()),
		MotivationID: m.ID,
		Motivation:   m,
		TriggeredAt:  now,
		TriggerData:  triggerData,
		Result:       TriggerResultSuccess,
	}

	// Execute actions based on motivation configuration
	if m.CreateBeadOnTrigger {
		beadID, err := e.actionHandler.CreateStimulusBead(m, triggerData)
		if err != nil {
			trigger.Result = TriggerResultError
			trigger.Error = err.Error()
		} else {
			trigger.BeadCreated = beadID
		}
	}

	if m.WakeAgent && trigger.Result == TriggerResultSuccess {
		if m.AgentID != "" {
			// Wake specific agent
			if err := e.actionHandler.WakeAgent(m.AgentID, m); err != nil {
				log.Printf("Failed to wake agent %s: %v", m.AgentID, err)
			} else {
				trigger.AgentWoken = m.AgentID
			}
		} else if m.AgentRole != "" {
			// Wake agents by role
			if err := e.actionHandler.WakeAgentsByRole(m.AgentRole, m); err != nil {
				log.Printf("Failed to wake agents by role %s: %v", m.AgentRole, err)
			}
		}
	}

	// Publish the trigger event
	if e.actionHandler != nil {
		if err := e.actionHandler.PublishMotivationFired(trigger); err != nil {
			log.Printf("Failed to publish motivation fired event: %v", err)
		}
	}

	// Record in registry
	e.registry.RecordTrigger(trigger)

	log.Printf("Motivation fired: %s (%s) -> agent_role=%s", m.Name, m.ID, m.AgentRole)
	return nil
}

// ManualTrigger allows manually triggering a motivation (for testing/admin)
func (e *Engine) ManualTrigger(ctx context.Context, motivationID string) (*MotivationTrigger, error) {
	m, err := e.registry.Get(motivationID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	trigger := &MotivationTrigger{
		ID:           fmt.Sprintf("trig-manual-%d", now.UnixNano()),
		MotivationID: m.ID,
		Motivation:   m,
		TriggeredAt:  now,
		TriggerData:  map[string]interface{}{"manual": true},
		Result:       TriggerResultSuccess,
	}

	if err := e.fire(ctx, m, trigger.TriggerData); err != nil {
		trigger.Result = TriggerResultError
		trigger.Error = err.Error()
		return trigger, err
	}

	return trigger, nil
}

// GetRegistry returns the motivation registry
func (e *Engine) GetRegistry() *Registry {
	return e.registry
}
