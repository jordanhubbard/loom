package eventbus

import (
	"testing"
	"time"

	"github.com/jordanhubbard/arbiter/internal/temporal/client"
	"github.com/jordanhubbard/arbiter/pkg/config"
)

// TestEventBusCreation tests that event bus can be created
func TestEventBusCreation(t *testing.T) {
	cfg := &config.TemporalConfig{
		Host:                     "localhost:7233",
		Namespace:                "test-namespace",
		TaskQueue:                "test-queue",
		WorkflowExecutionTimeout: 1 * time.Hour,
		WorkflowTaskTimeout:      10 * time.Second,
		EnableEventBus:           true,
		EventBufferSize:          100,
	}

	temporalClient, err := client.New(cfg)
	if err != nil {
		t.Skipf("Temporal server not available: %v", err)
		return
	}
	defer temporalClient.Close()

	eb := NewEventBus(temporalClient, cfg)
	if eb == nil {
		t.Fatal("Expected event bus to be non-nil")
	}
	defer eb.Close()
}

// TestEventPublishSubscribe tests basic pub/sub functionality
func TestEventPublishSubscribe(t *testing.T) {
	cfg := &config.TemporalConfig{
		Host:                     "localhost:7233",
		Namespace:                "test-namespace",
		TaskQueue:                "test-queue",
		WorkflowExecutionTimeout: 1 * time.Hour,
		WorkflowTaskTimeout:      10 * time.Second,
		EnableEventBus:           true,
		EventBufferSize:          100,
	}

	temporalClient, err := client.New(cfg)
	if err != nil {
		t.Skipf("Temporal server not available: %v", err)
		return
	}
	defer temporalClient.Close()

	eb := NewEventBus(temporalClient, cfg)
	defer eb.Close()

	// Subscribe
	subscriber := eb.Subscribe("test-sub", nil)
	if subscriber == nil {
		t.Fatal("Expected subscriber to be non-nil")
	}

	// Publish event
	event := &Event{
		Type:      EventTypeAgentSpawned,
		Source:    "test",
		ProjectID: "test-project",
		Data: map[string]interface{}{
			"agent_id": "test-agent",
		},
	}

	err = eb.Publish(event)
	if err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	// Receive event
	select {
	case received := <-subscriber.Channel:
		if received.Type != EventTypeAgentSpawned {
			t.Errorf("Expected event type %s, got %s", EventTypeAgentSpawned, received.Type)
		}
		if received.ProjectID != "test-project" {
			t.Errorf("Expected project ID test-project, got %s", received.ProjectID)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for event")
	}

	// Cleanup
	eb.Unsubscribe("test-sub")
}

// TestEventFilter tests event filtering
func TestEventFilter(t *testing.T) {
	cfg := &config.TemporalConfig{
		Host:                     "localhost:7233",
		Namespace:                "test-namespace",
		TaskQueue:                "test-queue",
		WorkflowExecutionTimeout: 1 * time.Hour,
		WorkflowTaskTimeout:      10 * time.Second,
		EnableEventBus:           true,
		EventBufferSize:          100,
	}

	temporalClient, err := client.New(cfg)
	if err != nil {
		t.Skipf("Temporal server not available: %v", err)
		return
	}
	defer temporalClient.Close()

	eb := NewEventBus(temporalClient, cfg)
	defer eb.Close()

	// Subscribe with filter for specific project
	filter := func(event *Event) bool {
		return event.ProjectID == "project-1"
	}
	subscriber := eb.Subscribe("test-sub", filter)

	// Publish event for project-1 (should be received)
	event1 := &Event{
		Type:      EventTypeAgentSpawned,
		Source:    "test",
		ProjectID: "project-1",
		Data:      map[string]interface{}{},
	}
	eb.Publish(event1)

	// Publish event for project-2 (should be filtered out)
	event2 := &Event{
		Type:      EventTypeBeadCreated,
		Source:    "test",
		ProjectID: "project-2",
		Data:      map[string]interface{}{},
	}
	eb.Publish(event2)

	// Should only receive event1
	select {
	case received := <-subscriber.Channel:
		if received.ProjectID != "project-1" {
			t.Errorf("Expected project-1, got %s", received.ProjectID)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timeout waiting for event")
	}

	// Should not receive event2
	select {
	case received := <-subscriber.Channel:
		t.Errorf("Should not have received event for project %s", received.ProjectID)
	case <-time.After(1 * time.Second):
		// Expected - no event received
	}

	eb.Unsubscribe("test-sub")
}

// TestMultipleSubscribers tests multiple subscribers
func TestMultipleSubscribers(t *testing.T) {
	cfg := &config.TemporalConfig{
		Host:                     "localhost:7233",
		Namespace:                "test-namespace",
		TaskQueue:                "test-queue",
		WorkflowExecutionTimeout: 1 * time.Hour,
		WorkflowTaskTimeout:      10 * time.Second,
		EnableEventBus:           true,
		EventBufferSize:          100,
	}

	temporalClient, err := client.New(cfg)
	if err != nil {
		t.Skipf("Temporal server not available: %v", err)
		return
	}
	defer temporalClient.Close()

	eb := NewEventBus(temporalClient, cfg)
	defer eb.Close()

	// Create two subscribers
	sub1 := eb.Subscribe("sub-1", nil)
	sub2 := eb.Subscribe("sub-2", nil)

	// Publish event
	event := &Event{
		Type:      EventTypeAgentSpawned,
		Source:    "test",
		ProjectID: "test-project",
		Data:      map[string]interface{}{},
	}
	eb.Publish(event)

	// Both subscribers should receive the event
	receivedCount := 0

	select {
	case <-sub1.Channel:
		receivedCount++
	case <-time.After(2 * time.Second):
		t.Error("Subscriber 1 timeout")
	}

	select {
	case <-sub2.Channel:
		receivedCount++
	case <-time.After(2 * time.Second):
		t.Error("Subscriber 2 timeout")
	}

	if receivedCount != 2 {
		t.Errorf("Expected 2 subscribers to receive event, got %d", receivedCount)
	}

	eb.Unsubscribe("sub-1")
	eb.Unsubscribe("sub-2")
}

// TestEventTypes tests publishing different event types
func TestEventTypes(t *testing.T) {
	cfg := &config.TemporalConfig{
		Host:                     "localhost:7233",
		Namespace:                "test-namespace",
		TaskQueue:                "test-queue",
		WorkflowExecutionTimeout: 1 * time.Hour,
		WorkflowTaskTimeout:      10 * time.Second,
		EnableEventBus:           true,
		EventBufferSize:          100,
	}

	temporalClient, err := client.New(cfg)
	if err != nil {
		t.Skipf("Temporal server not available: %v", err)
		return
	}
	defer temporalClient.Close()

	eb := NewEventBus(temporalClient, cfg)
	defer eb.Close()

	subscriber := eb.Subscribe("test-sub", nil)

	// Test different event types
	eventTypes := []EventType{
		EventTypeAgentSpawned,
		EventTypeAgentStatusChange,
		EventTypeBeadCreated,
		EventTypeBeadAssigned,
		EventTypeDecisionCreated,
		EventTypeLogMessage,
	}

	for _, eventType := range eventTypes {
		event := &Event{
			Type:      eventType,
			Source:    "test",
			ProjectID: "test-project",
			Data:      map[string]interface{}{},
		}

		err := eb.Publish(event)
		if err != nil {
			t.Errorf("Failed to publish event type %s: %v", eventType, err)
		}

		select {
		case received := <-subscriber.Channel:
			if received.Type != eventType {
				t.Errorf("Expected event type %s, got %s", eventType, received.Type)
			}
		case <-time.After(2 * time.Second):
			t.Errorf("Timeout waiting for event type %s", eventType)
		}
	}

	eb.Unsubscribe("test-sub")
}
