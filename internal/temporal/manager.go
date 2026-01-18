package temporal

import (
	"context"
	"fmt"
	"log"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/jordanhubbard/arbiter/internal/temporal/activities"
	temporalclient "github.com/jordanhubbard/arbiter/internal/temporal/client"
	"github.com/jordanhubbard/arbiter/internal/temporal/eventbus"
	"github.com/jordanhubbard/arbiter/internal/temporal/workflows"
	"github.com/jordanhubbard/arbiter/pkg/config"
)

// Manager manages Temporal integration for the arbiter
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
	w.RegisterWorkflow(eventbus.EventAggregatorWorkflow)

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
	workflowOptions := client.StartWorkflowOptions{
		ID:                    fmt.Sprintf("agent-%s", agentID),
		TaskQueue:             m.config.TaskQueue,
		WorkflowTaskTimeout:   m.config.WorkflowTaskTimeout,
		WorkflowRunTimeout:    m.config.WorkflowExecutionTimeout,
	}

	input := workflows.AgentLifecycleWorkflowInput{
		AgentID:     agentID,
		ProjectID:   projectID,
		PersonaName: personaName,
		Name:        name,
	}

	_, err := m.client.ExecuteWorkflow(ctx, workflowOptions, workflows.AgentLifecycleWorkflow, input)
	if err != nil {
		return fmt.Errorf("failed to start agent workflow: %w", err)
	}

	log.Printf("Started agent workflow for agent %s", agentID)
	return nil
}

// StartBeadWorkflow starts a bead processing workflow
func (m *Manager) StartBeadWorkflow(ctx context.Context, beadID, projectID, title, description string, priority int, beadType string) error {
	workflowOptions := client.StartWorkflowOptions{
		ID:                    fmt.Sprintf("bead-%s", beadID),
		TaskQueue:             m.config.TaskQueue,
		WorkflowTaskTimeout:   m.config.WorkflowTaskTimeout,
		WorkflowRunTimeout:    m.config.WorkflowExecutionTimeout,
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
		return fmt.Errorf("failed to start bead workflow: %w", err)
	}

	log.Printf("Started bead workflow for bead %s", beadID)
	return nil
}

// StartDecisionWorkflow starts a decision approval workflow
func (m *Manager) StartDecisionWorkflow(ctx context.Context, decisionID, projectID, question, requesterID string, options []string) error {
	workflowOptions := client.StartWorkflowOptions{
		ID:                    fmt.Sprintf("decision-%s", decisionID),
		TaskQueue:             m.config.TaskQueue,
		WorkflowTaskTimeout:   m.config.WorkflowTaskTimeout,
		WorkflowRunTimeout:    m.config.WorkflowExecutionTimeout,
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
		return fmt.Errorf("failed to start decision workflow: %w", err)
	}

	log.Printf("Started decision workflow for decision %s", decisionID)
	return nil
}

// SignalAgentWorkflow sends a signal to an agent workflow
func (m *Manager) SignalAgentWorkflow(ctx context.Context, agentID, signalName string, arg interface{}) error {
	workflowID := fmt.Sprintf("agent-%s", agentID)
	return m.client.SignalWorkflow(ctx, workflowID, "", signalName, arg)
}

// SignalBeadWorkflow sends a signal to a bead workflow
func (m *Manager) SignalBeadWorkflow(ctx context.Context, beadID, signalName string, arg interface{}) error {
	workflowID := fmt.Sprintf("bead-%s", beadID)
	return m.client.SignalWorkflow(ctx, workflowID, "", signalName, arg)
}

// QueryAgentWorkflow queries an agent workflow
func (m *Manager) QueryAgentWorkflow(ctx context.Context, agentID, queryType string) (interface{}, error) {
	workflowID := fmt.Sprintf("agent-%s", agentID)
	resp, err := m.client.QueryWorkflow(ctx, workflowID, "", queryType)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := resp.Get(&result); err != nil {
		return nil, err
	}

	return result, nil
}
