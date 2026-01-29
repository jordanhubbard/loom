package temporal

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jordanhubbard/agenticorp/internal/observability"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/jordanhubbard/agenticorp/internal/temporal/activities"
	temporalclient "github.com/jordanhubbard/agenticorp/internal/temporal/client"
	"github.com/jordanhubbard/agenticorp/internal/temporal/eventbus"
	"github.com/jordanhubbard/agenticorp/internal/temporal/workflows"
	"github.com/jordanhubbard/agenticorp/pkg/config"
)

// Manager manages Temporal integration for the agenticorp
type Manager struct {
	client   *temporalclient.Client
	eventBus *eventbus.EventBus
	worker   worker.Worker
	config   *config.TemporalConfig
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewManager creates a new Temporal manager
func NewManager(cfg *config.TemporalConfig) (*Manager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("temporal config cannot be nil")
	}

	// Create Temporal client
	client, err := temporalclient.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporal client: %w", err)
	}

	// Create event bus
	var eventBus *eventbus.EventBus
	if cfg.EnableEventBus {
		eventBus = eventbus.NewEventBus(client, cfg)
		log.Println("Temporal event bus initialized")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create worker
	w := worker.New(client.GetClient(), cfg.TaskQueue, worker.Options{})

	// Register workflows
	w.RegisterWorkflow(workflows.AgentLifecycleWorkflow)
	w.RegisterWorkflow(workflows.BeadProcessingWorkflow)
	w.RegisterWorkflow(workflows.DecisionWorkflow)
	w.RegisterWorkflow(workflows.DispatcherWorkflow)
	w.RegisterWorkflow(eventbus.EventAggregatorWorkflow)
	w.RegisterWorkflow(workflows.ProviderHeartbeatWorkflow)
	w.RegisterWorkflow(workflows.ProviderQueryWorkflow)
	w.RegisterWorkflow(workflows.AgentiCorpHeartbeatWorkflow) // Master clock

	// Register activities
	if eventBus != nil {
		activities := activities.NewActivities(eventBus)
		w.RegisterActivity(activities)
	}

	log.Printf("Temporal worker registered for task queue: %s", cfg.TaskQueue)

	return &Manager{
		client:   client,
		eventBus: eventBus,
		worker:   w,
		config:   cfg,
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// RegisterActivity registers additional activities before the worker starts.
func (m *Manager) RegisterActivity(a interface{}) {
	m.worker.RegisterActivity(a)
}

// RegisterWorkflow registers additional workflows before the worker starts.
func (m *Manager) RegisterWorkflow(wf interface{}) {
	m.worker.RegisterWorkflow(wf)
}

// Start starts the Temporal worker
func (m *Manager) Start() error {
	log.Println("Starting Temporal worker...")

	// Start worker in a goroutine
	go func() {
		if err := m.worker.Run(worker.InterruptCh()); err != nil {
			log.Printf("Temporal worker error: %v", err)
		}
	}()

	log.Println("Temporal worker started successfully")
	return nil
}

// Stop stops the Temporal manager
func (m *Manager) Stop() {
	log.Println("Stopping Temporal manager...")

	m.cancel()

	if m.worker != nil {
		m.worker.Stop()
	}

	if m.eventBus != nil {
		m.eventBus.Close()
	}

	if m.client != nil {
		m.client.Close()
	}

	log.Println("Temporal manager stopped")
}

// GetClient returns the Temporal client
func (m *Manager) GetClient() *temporalclient.Client {
	return m.client
}

// GetEventBus returns the event bus
func (m *Manager) GetEventBus() *eventbus.EventBus {
	return m.eventBus
}

// StartAgentWorkflow starts an agent lifecycle workflow
func (m *Manager) StartAgentWorkflow(ctx context.Context, agentID, projectID, personaName, name string) error {
	start := time.Now()
	observability.Info("temporal.workflow_start", map[string]interface{}{
		"workflow":   "agent_lifecycle",
		"agent_id":   agentID,
		"project_id": projectID,
	})
	workflowOptions := client.StartWorkflowOptions{
		ID:                  fmt.Sprintf("agent-%s", agentID),
		TaskQueue:           m.config.TaskQueue,
		WorkflowTaskTimeout: m.config.WorkflowTaskTimeout,
		WorkflowRunTimeout:  m.config.WorkflowExecutionTimeout,
	}

	input := workflows.AgentLifecycleWorkflowInput{
		AgentID:     agentID,
		ProjectID:   projectID,
		PersonaName: personaName,
		Name:        name,
	}

	_, err := m.client.ExecuteWorkflow(ctx, workflowOptions, workflows.AgentLifecycleWorkflow, input)
	if err != nil {
		observability.Error("temporal.workflow_start", map[string]interface{}{
			"workflow":    "agent_lifecycle",
			"agent_id":    agentID,
			"project_id":  projectID,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return fmt.Errorf("failed to start agent workflow: %w", err)
	}

	log.Printf("Started agent workflow for agent %s", agentID)
	observability.Info("temporal.workflow_started", map[string]interface{}{
		"workflow":    "agent_lifecycle",
		"agent_id":    agentID,
		"project_id":  projectID,
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return nil
}

// StartBeadWorkflow starts a bead processing workflow
func (m *Manager) StartBeadWorkflow(ctx context.Context, beadID, projectID, title, description string, priority int, beadType string) error {
	start := time.Now()
	observability.Info("temporal.workflow_start", map[string]interface{}{
		"workflow":   "bead_processing",
		"bead_id":    beadID,
		"project_id": projectID,
	})
	workflowOptions := client.StartWorkflowOptions{
		ID:                  fmt.Sprintf("bead-%s", beadID),
		TaskQueue:           m.config.TaskQueue,
		WorkflowTaskTimeout: m.config.WorkflowTaskTimeout,
		WorkflowRunTimeout:  m.config.WorkflowExecutionTimeout,
	}

	input := workflows.BeadProcessingWorkflowInput{
		BeadID:      beadID,
		ProjectID:   projectID,
		Title:       title,
		Description: description,
		Priority:    priority,
		Type:        beadType,
	}

	_, err := m.client.ExecuteWorkflow(ctx, workflowOptions, workflows.BeadProcessingWorkflow, input)
	if err != nil {
		observability.Error("temporal.workflow_start", map[string]interface{}{
			"workflow":    "bead_processing",
			"bead_id":     beadID,
			"project_id":  projectID,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return fmt.Errorf("failed to start bead workflow: %w", err)
	}

	log.Printf("Started bead workflow for bead %s", beadID)
	observability.Info("temporal.workflow_started", map[string]interface{}{
		"workflow":    "bead_processing",
		"bead_id":     beadID,
		"project_id":  projectID,
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return nil
}

// StartDecisionWorkflow starts a decision approval workflow
func (m *Manager) StartDecisionWorkflow(ctx context.Context, decisionID, projectID, question, requesterID string, options []string) error {
	start := time.Now()
	observability.Info("temporal.workflow_start", map[string]interface{}{
		"workflow":     "decision",
		"decision_id":  decisionID,
		"project_id":   projectID,
		"requester_id": requesterID,
	})
	workflowOptions := client.StartWorkflowOptions{
		ID:                  fmt.Sprintf("decision-%s", decisionID),
		TaskQueue:           m.config.TaskQueue,
		WorkflowTaskTimeout: m.config.WorkflowTaskTimeout,
		WorkflowRunTimeout:  m.config.WorkflowExecutionTimeout,
	}

	input := workflows.DecisionWorkflowInput{
		DecisionID:  decisionID,
		ProjectID:   projectID,
		Question:    question,
		RequesterID: requesterID,
		Options:     options,
	}

	_, err := m.client.ExecuteWorkflow(ctx, workflowOptions, workflows.DecisionWorkflow, input)
	if err != nil {
		observability.Error("temporal.workflow_start", map[string]interface{}{
			"workflow":    "decision",
			"decision_id": decisionID,
			"project_id":  projectID,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return fmt.Errorf("failed to start decision workflow: %w", err)
	}

	log.Printf("Started decision workflow for decision %s", decisionID)
	observability.Info("temporal.workflow_started", map[string]interface{}{
		"workflow":    "decision",
		"decision_id": decisionID,
		"project_id":  projectID,
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return nil
}

// StartDispatcherWorkflow starts the periodic dispatch loop workflow.
func (m *Manager) StartDispatcherWorkflow(ctx context.Context, projectID string, interval time.Duration) error {
	start := time.Now()
	observability.Info("temporal.workflow_start", map[string]interface{}{
		"workflow":   "dispatcher",
		"project_id": projectID,
		"interval":   interval.String(),
	})
	workflowID := "dispatcher-global"
	if projectID != "" {
		workflowID = fmt.Sprintf("dispatcher-%s", projectID)
	}
	workflowOptions := client.StartWorkflowOptions{
		ID:                  workflowID,
		TaskQueue:           m.config.TaskQueue,
		WorkflowTaskTimeout: m.config.WorkflowTaskTimeout,
		WorkflowRunTimeout:  0, // run indefinitely
	}

	input := workflows.DispatcherWorkflowInput{
		ProjectID: projectID,
		Interval:  interval,
	}

	_, err := m.client.ExecuteWorkflow(ctx, workflowOptions, workflows.DispatcherWorkflow, input)
	if err != nil {
		observability.Error("temporal.workflow_start", map[string]interface{}{
			"workflow":    "dispatcher",
			"project_id":  projectID,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return fmt.Errorf("failed to start dispatcher workflow: %w", err)
	}

	if projectID == "" {
		log.Printf("Started dispatcher workflow for ALL projects")
	} else {
		log.Printf("Started dispatcher workflow for project %s", projectID)
	}
	observability.Info("temporal.workflow_started", map[string]interface{}{
		"workflow":    "dispatcher",
		"project_id":  projectID,
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return nil
}

// StartProviderHeartbeatWorkflow starts or resumes a provider heartbeat workflow.
func (m *Manager) StartProviderHeartbeatWorkflow(ctx context.Context, providerID string, interval time.Duration) error {
	start := time.Now()
	observability.Info("temporal.workflow_start", map[string]interface{}{
		"workflow":    "provider_heartbeat",
		"provider_id": providerID,
		"interval":    interval.String(),
	})
	workflowID := fmt.Sprintf("provider-heartbeat-%s", providerID)
	workflowOptions := client.StartWorkflowOptions{
		ID:                  workflowID,
		TaskQueue:           m.config.TaskQueue,
		WorkflowTaskTimeout: m.config.WorkflowTaskTimeout,
		WorkflowRunTimeout:  0,
	}

	input := workflows.ProviderHeartbeatWorkflowInput{
		ProviderID: providerID,
		Interval:   interval,
	}

	_, err := m.client.ExecuteWorkflow(ctx, workflowOptions, workflows.ProviderHeartbeatWorkflow, input)
	if err != nil {
		if _, ok := err.(*serviceerror.WorkflowExecutionAlreadyStarted); ok {
			return nil
		}
		observability.Error("temporal.workflow_start", map[string]interface{}{
			"workflow":    "provider_heartbeat",
			"provider_id": providerID,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return fmt.Errorf("failed to start provider heartbeat workflow: %w", err)
	}

	log.Printf("Started provider heartbeat workflow for %s", providerID)
	observability.Info("temporal.workflow_started", map[string]interface{}{
		"workflow":    "provider_heartbeat",
		"provider_id": providerID,
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return nil
}

// StartAgentiCorpHeartbeatWorkflow starts the master clock heartbeat workflow
func (m *Manager) StartAgentiCorpHeartbeatWorkflow(ctx context.Context, interval time.Duration) error {
	if interval == 0 {
		interval = 10 * time.Second
	}
	start := time.Now()
	observability.Info("temporal.workflow_start", map[string]interface{}{
		"workflow": "agenticorp_heartbeat",
		"interval": interval.String(),
	})

	workflowID := "agenticorp-heartbeat-master"
	workflowOptions := client.StartWorkflowOptions{
		ID:                  workflowID,
		TaskQueue:           m.config.TaskQueue,
		WorkflowTaskTimeout: m.config.WorkflowTaskTimeout,
		WorkflowRunTimeout:  0, // Infinite duration for master clock
	}

	input := workflows.AgentiCorpHeartbeatWorkflowInput{
		Interval: interval,
	}

	_, err := m.client.ExecuteWorkflow(ctx, workflowOptions, workflows.AgentiCorpHeartbeatWorkflow, input)
	if err != nil {
		if _, ok := err.(*serviceerror.WorkflowExecutionAlreadyStarted); ok {
			return nil // Already running
		}
		observability.Error("temporal.workflow_start", map[string]interface{}{
			"workflow":    "agenticorp_heartbeat",
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return fmt.Errorf("failed to start agenticorp heartbeat workflow: %w", err)
	}

	log.Printf("Started AgentiCorp master heartbeat workflow with %v interval", interval)
	observability.Info("temporal.workflow_started", map[string]interface{}{
		"workflow":    "agenticorp_heartbeat",
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return nil
}

// RunProviderQueryWorkflow executes a direct provider query workflow and waits for result.
func (m *Manager) RunProviderQueryWorkflow(ctx context.Context, input workflows.ProviderQueryWorkflowInput) (*activities.ProviderQueryResult, error) {
	start := time.Now()
	observability.Info("temporal.workflow_start", map[string]interface{}{
		"workflow":    "provider_query",
		"provider_id": input.ProviderID,
	})
	workflowOptions := client.StartWorkflowOptions{
		ID:                  fmt.Sprintf("repl-%d", time.Now().UTC().UnixNano()),
		TaskQueue:           m.config.TaskQueue,
		WorkflowTaskTimeout: m.config.WorkflowTaskTimeout,
		WorkflowRunTimeout:  5 * time.Minute,
	}

	we, err := m.client.ExecuteWorkflow(ctx, workflowOptions, workflows.ProviderQueryWorkflow, input)
	if err != nil {
		observability.Error("temporal.workflow_start", map[string]interface{}{
			"workflow":    "provider_query",
			"provider_id": input.ProviderID,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return nil, fmt.Errorf("failed to start provider query workflow: %w", err)
	}

	var result activities.ProviderQueryResult
	if err := we.Get(ctx, &result); err != nil {
		observability.Error("temporal.workflow_result", map[string]interface{}{
			"workflow":    "provider_query",
			"provider_id": input.ProviderID,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return nil, fmt.Errorf("failed to get provider query result: %w", err)
	}
	observability.Info("temporal.workflow_result", map[string]interface{}{
		"workflow":    "provider_query",
		"provider_id": input.ProviderID,
		"duration_ms": time.Since(start).Milliseconds(),
	})

	return &result, nil
}

// SignalAgentWorkflow sends a signal to an agent workflow
func (m *Manager) SignalAgentWorkflow(ctx context.Context, agentID, signalName string, arg interface{}) error {
	workflowID := fmt.Sprintf("agent-%s", agentID)
	start := time.Now()
	err := m.client.SignalWorkflow(ctx, workflowID, "", signalName, arg)
	if err != nil {
		observability.Error("temporal.signal", map[string]interface{}{
			"workflow":    "agent_lifecycle",
			"agent_id":    agentID,
			"signal":      signalName,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return err
	}
	observability.Info("temporal.signal", map[string]interface{}{
		"workflow":    "agent_lifecycle",
		"agent_id":    agentID,
		"signal":      signalName,
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return nil
}

// SignalBeadWorkflow sends a signal to a bead workflow
func (m *Manager) SignalBeadWorkflow(ctx context.Context, beadID, signalName string, arg interface{}) error {
	workflowID := fmt.Sprintf("bead-%s", beadID)
	start := time.Now()
	err := m.client.SignalWorkflow(ctx, workflowID, "", signalName, arg)
	if err != nil {
		observability.Error("temporal.signal", map[string]interface{}{
			"workflow":    "bead_processing",
			"bead_id":     beadID,
			"signal":      signalName,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return err
	}
	observability.Info("temporal.signal", map[string]interface{}{
		"workflow":    "bead_processing",
		"bead_id":     beadID,
		"signal":      signalName,
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return nil
}

// QueryAgentWorkflow queries an agent workflow
func (m *Manager) QueryAgentWorkflow(ctx context.Context, agentID, queryType string) (interface{}, error) {
	workflowID := fmt.Sprintf("agent-%s", agentID)
	start := time.Now()
	resp, err := m.client.QueryWorkflow(ctx, workflowID, "", queryType)
	if err != nil {
		observability.Error("temporal.query", map[string]interface{}{
			"workflow":    "agent_lifecycle",
			"agent_id":    agentID,
			"query":       queryType,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return nil, err
	}

	var result interface{}
	if err := resp.Get(&result); err != nil {
		observability.Error("temporal.query", map[string]interface{}{
			"workflow":    "agent_lifecycle",
			"agent_id":    agentID,
			"query":       queryType,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return nil, err
	}
	observability.Info("temporal.query", map[string]interface{}{
		"workflow":    "agent_lifecycle",
		"agent_id":    agentID,
		"query":       queryType,
		"duration_ms": time.Since(start).Milliseconds(),
	})

	return result, nil
}

// DSL Operations

// ExecuteTemporalDSL parses and executes Temporal DSL from agent instructions
// Returns execution results and the cleaned text with DSL removed
func (m *Manager) ExecuteTemporalDSL(ctx context.Context, agentID string, dslText string) (*TemporalDSLExecution, error) {
	if dslText == "" {
		return nil, fmt.Errorf("dsl text cannot be empty")
	}

	// Parse DSL instructions
	instructions, _, err := ParseTemporalDSL(dslText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSL: %w", err)
	}

	if len(instructions) == 0 {
		return nil, fmt.Errorf("no temporal instructions found in DSL")
	}

	// Execute instructions
	executor := NewDSLExecutor(m)
	execution := executor.ExecuteInstructions(ctx, instructions, agentID)

	return execution, nil
}

// ParseTemporalInstructions parses DSL without executing (for validation)
func (m *Manager) ParseTemporalInstructions(text string) ([]TemporalInstruction, string, error) {
	instructions, cleanedText, err := ParseTemporalDSL(text)
	if err != nil {
		return nil, text, err
	}

	// Validate all instructions
	for _, instr := range instructions {
		if err := ValidateInstruction(instr); err != nil {
			log.Printf("Instruction validation failed: %v", err)
		}
	}

	return instructions, cleanedText, nil
}

// StripTemporalDSL removes Temporal DSL blocks from text
func (m *Manager) StripTemporalDSL(text string) (string, error) {
	_, cleanedText, err := ParseTemporalDSL(text)
	return cleanedText, err
}

// ScheduleWorkflow schedules a workflow with options
func (m *Manager) ScheduleWorkflow(ctx context.Context, opts WorkflowOptions) (string, error) {
	workflowOptions := client.StartWorkflowOptions{
		ID:        opts.ID,
		TaskQueue: m.config.TaskQueue,
	}

	if opts.Timeout > 0 {
		workflowOptions.WorkflowRunTimeout = opts.Timeout
	}

	// Retry policy
	if opts.Retry > 0 {
		// Note: RetryPolicy is per-activity, but we can set it for the workflow
		workflowOptions.WorkflowRunTimeout = opts.Timeout
	}

	run, err := m.client.ExecuteWorkflow(ctx, workflowOptions, opts.Name, opts.Input)
	if err != nil {
		return "", fmt.Errorf("failed to schedule workflow: %w", err)
	}

	return run.GetID(), nil
}

// GetWorkflowResult waits for a workflow to complete and returns its result
func (m *Manager) GetWorkflowResult(ctx context.Context, workflowID string) (interface{}, error) {
	run := m.client.GetWorkflow(ctx, workflowID, "")
	if run == nil {
		return nil, fmt.Errorf("workflow not found: %s", workflowID)
	}

	var result interface{}
	err := run.Get(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow result: %w", err)
	}

	return result, nil
}

// ExecuteActivity executes an activity and waits for result
func (m *Manager) ExecuteActivity(ctx context.Context, opts ActivityOptions) (interface{}, error) {
	// For now, return error since direct activity execution from DSL is complex
	// This would require spawning a temporary workflow to execute the activity
	return nil, fmt.Errorf("direct activity execution not yet implemented")
}

// CreateSchedule creates a recurring schedule
func (m *Manager) CreateSchedule(ctx context.Context, opts ScheduleOptions) (string, error) {
	// Schedule creation is complex and requires special Temporal APIs
	// For now, return error - this would be enhanced in future
	return "", fmt.Errorf("schedule creation not yet implemented")
}

// QueryWorkflow queries a running workflow
func (m *Manager) QueryWorkflow(ctx context.Context, opts QueryOptions) (interface{}, error) {
	resp, err := m.client.QueryWorkflow(ctx, opts.WorkflowID, opts.RunID, opts.QueryType, opts.Args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query workflow: %w", err)
	}

	var result interface{}
	if err := resp.Get(&result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal query result: %w", err)
	}

	return result, nil
}

// SignalWorkflow sends a signal to a running workflow
func (m *Manager) SignalWorkflow(ctx context.Context, opts SignalOptions) error {
	return m.client.SignalWorkflow(ctx, opts.WorkflowID, opts.RunID, opts.Name, opts.Data)
}

// CancelWorkflow cancels a running workflow
func (m *Manager) CancelWorkflow(ctx context.Context, opts CancelOptions) error {
	return m.client.CancelWorkflow(ctx, opts.WorkflowID, opts.RunID)
}

// ListWorkflows lists running workflows
func (m *Manager) ListWorkflows(ctx context.Context) ([]map[string]interface{}, error) {
	// This would require Temporal's list API
	// For now, return empty list
	return []map[string]interface{}{}, nil
}
