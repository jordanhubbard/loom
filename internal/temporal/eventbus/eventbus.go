package eventbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.temporal.io/sdk/workflow"
	
	"github.com/jordanhubbard/arbiter/pkg/config"
	temporalclient "github.com/jordanhubbard/arbiter/internal/temporal/client"
)

// EventType represents the type of event
type EventType string

const (
	EventTypeAgentSpawned      EventType = "agent.spawned"
	EventTypeAgentStatusChange EventType = "agent.status_change"
	EventTypeAgentCompleted    EventType = "agent.completed"
	EventTypeBeadCreated       EventType = "bead.created"
	EventTypeBeadAssigned      EventType = "bead.assigned"
	EventTypeBeadStatusChange  EventType = "bead.status_change"
	EventTypeBeadCompleted     EventType = "bead.completed"
	EventTypeDecisionCreated   EventType = "decision.created"
	EventTypeDecisionResolved  EventType = "decision.resolved"
	EventTypeLogMessage        EventType = "log.message"
	EventTypeWorkflowStarted   EventType = "workflow.started"
	EventTypeWorkflowCompleted EventType = "workflow.completed"
)

// Event represents a system event
type Event struct {
	ID        string                 `json:"id"`
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source"`     // Component that generated the event
	Data      map[string]interface{} `json:"data"`       // Event payload
	ProjectID string                 `json:"project_id,omitempty"`
}

// Subscriber represents an event subscriber
type Subscriber struct {
	ID      string
	Channel chan *Event
	Filter  func(*Event) bool // Optional filter function
}

// EventBus provides pub/sub event messaging using Temporal
type EventBus struct {
	client      *temporalclient.Client
	subscribers map[string]*Subscriber
	mu          sync.RWMutex
	config      *config.TemporalConfig
	ctx         context.Context
	cancel      context.CancelFunc
	buffer      chan *Event
}

// NewEventBus creates a new event bus
func NewEventBus(client *temporalclient.Client, cfg *config.TemporalConfig) *EventBus {
	ctx, cancel := context.WithCancel(context.Background())
	
	bufferSize := cfg.EventBufferSize
	if bufferSize <= 0 {
		bufferSize = 1000
	}

	eb := &EventBus{
		client:      client,
		subscribers: make(map[string]*Subscriber),
		config:      cfg,
		ctx:         ctx,
		cancel:      cancel,
		buffer:      make(chan *Event, bufferSize),
	}

	// Start event processing goroutine
	go eb.processEvents()

	return eb
}

// Publish publishes an event to all subscribers
func (eb *EventBus) Publish(event *Event) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// Set timestamp if not already set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Generate ID if not set
	if event.ID == "" {
		event.ID = fmt.Sprintf("%s-%d", event.Type, time.Now().UnixNano())
	}

	// Add to buffer for async processing
	select {
	case eb.buffer <- event:
		return nil
	default:
		return fmt.Errorf("event buffer is full")
	}
}

// Subscribe creates a new subscription to events
func (eb *EventBus) Subscribe(subscriberID string, filter func(*Event) bool) *Subscriber {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Check if subscriber already exists
	if sub, exists := eb.subscribers[subscriberID]; exists {
		return sub
	}

	// Create new subscriber
	sub := &Subscriber{
		ID:      subscriberID,
		Channel: make(chan *Event, 100), // Buffered channel for subscriber
		Filter:  filter,
	}

	eb.subscribers[subscriberID] = sub
	return sub
}

// Unsubscribe removes a subscriber
func (eb *EventBus) Unsubscribe(subscriberID string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if sub, exists := eb.subscribers[subscriberID]; exists {
		close(sub.Channel)
		delete(eb.subscribers, subscriberID)
	}
}

// processEvents processes events from the buffer and distributes to subscribers
func (eb *EventBus) processEvents() {
	for {
		select {
		case <-eb.ctx.Done():
			return
		case event := <-eb.buffer:
			eb.distributeEvent(event)
		}
	}
}

// distributeEvent sends event to all matching subscribers
func (eb *EventBus) distributeEvent(event *Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for _, sub := range eb.subscribers {
		// Apply filter if present
		if sub.Filter != nil && !sub.Filter(event) {
			continue
		}

		// Non-blocking send to subscriber
		select {
		case sub.Channel <- event:
		default:
			// Subscriber channel is full, skip
		}
	}
}

// Close shuts down the event bus
func (eb *EventBus) Close() {
	eb.cancel()
	close(eb.buffer)

	eb.mu.Lock()
	defer eb.mu.Unlock()

	for _, sub := range eb.subscribers {
		close(sub.Channel)
	}
	eb.subscribers = make(map[string]*Subscriber)
}

// PublishAgentEvent publishes an agent-related event
func (eb *EventBus) PublishAgentEvent(eventType EventType, agentID, projectID string, data map[string]interface{}) error {
	if data == nil {
		data = make(map[string]interface{})
	}
	data["agent_id"] = agentID

	return eb.Publish(&Event{
		Type:      eventType,
		Source:    "agent-manager",
		ProjectID: projectID,
		Data:      data,
	})
}

// PublishBeadEvent publishes a bead-related event
func (eb *EventBus) PublishBeadEvent(eventType EventType, beadID, projectID string, data map[string]interface{}) error {
	if data == nil {
		data = make(map[string]interface{})
	}
	data["bead_id"] = beadID

	return eb.Publish(&Event{
		Type:      eventType,
		Source:    "beads-manager",
		ProjectID: projectID,
		Data:      data,
	})
}

// PublishLogMessage publishes a log message event
func (eb *EventBus) PublishLogMessage(level, message, source, projectID string) error {
	return eb.Publish(&Event{
		Type:      EventTypeLogMessage,
		Source:    source,
		ProjectID: projectID,
		Data: map[string]interface{}{
			"level":   level,
			"message": message,
		},
	})
}

// EventAggregatorWorkflow is a long-running workflow that aggregates events
// This can be used to maintain event history in Temporal
func EventAggregatorWorkflow(ctx workflow.Context, projectID string) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Event aggregator workflow started", "projectID", projectID)

	// This workflow runs indefinitely and processes signals
	selector := workflow.NewSelector(ctx)
	
	// Handle event signals
	var eventChannel workflow.ReceiveChannel = workflow.GetSignalChannel(ctx, "event")
	selector.AddReceive(eventChannel, func(c workflow.ReceiveChannel, more bool) {
		var event Event
		c.Receive(ctx, &event)
		logger.Info("Received event", "type", event.Type, "id", event.ID)
		
		// In a real implementation, you might want to:
		// - Store events in a workflow variable
		// - Aggregate metrics
		// - Trigger other workflows based on events
	})

	// Keep workflow running
	for {
		selector.Select(ctx)
		
		// Check if workflow should continue
		if workflow.GetInfo(ctx).GetCurrentHistoryLength() > 10000 {
			// Start new workflow to avoid history growth
			logger.Warn("Event aggregator history too large, should continue as new")
			return workflow.NewContinueAsNewError(ctx, EventAggregatorWorkflow, projectID)
		}
	}
}
