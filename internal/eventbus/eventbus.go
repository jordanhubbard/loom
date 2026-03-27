package eventbus

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type EventType string

const (
	EventTypeAgentSpawned            EventType = "agent.spawned"
	EventTypeAgentStatusChange       EventType = "agent.status_change"
	EventTypeAgentHeartbeat          EventType = "agent.heartbeat"
	EventTypeAgentCompleted          EventType = "agent.completed"
	EventTypeAgentIteration          EventType = "agent.iteration"
	EventTypeBeadCreated             EventType = "bead.created"
	EventTypeBeadAssigned            EventType = "bead.assigned"
	EventTypeBeadStatusChange        EventType = "bead.status_change"
	EventTypeBeadCompleted           EventType = "bead.completed"
	EventTypeDecisionCreated         EventType = "decision.created"
	EventTypeDecisionResolved        EventType = "decision.resolved"
	EventTypeProviderRegistered      EventType = "provider.registered"
	EventTypeProviderDeleted         EventType = "provider.deleted"
	EventTypeProviderUpdated         EventType = "provider.updated"
	EventTypeProjectCreated          EventType = "project.created"
	EventTypeProjectUpdated          EventType = "project.updated"
	EventTypeProjectDeleted          EventType = "project.deleted"
	EventTypeConfigUpdated           EventType = "config.updated"
	EventTypeLogMessage              EventType = "log.message"
	EventTypeWorkflowStarted         EventType = "workflow.started"
	EventTypeWorkflowCompleted       EventType = "workflow.completed"
	EventTypeMotivationFired         EventType = "motivation.fired"
	EventTypeMotivationEnabled       EventType = "motivation.enabled"
	EventTypeMotivationDisabled      EventType = "motivation.disabled"
	EventTypeDeadlineApproaching     EventType = "deadline.approaching"
	EventTypeDeadlinePassed          EventType = "deadline.passed"
	EventTypeSystemIdle              EventType = "system.idle"
	EventTypeOpenClawMessageSent     EventType = "openclaw.message_sent"
	EventTypeOpenClawMessageFailed   EventType = "openclaw.message_failed"
	EventTypeOpenClawMessageReceived EventType = "openclaw.message_received"
	EventTypeOpenClawReplyProcessed  EventType = "openclaw.reply_processed"
)

type Event struct {
	ID        string                 `json:"id"`
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source"`
	Data      map[string]interface{} `json:"data"`
	ProjectID string                 `json:"project_id,omitempty"`
}

type Subscriber struct {
	ID      string
	Channel chan *Event
	Filter  func(*Event) bool
}

type EventBus struct {
	subscribers  map[string]*Subscriber
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	buffer       chan *Event
	closed       bool
	closedMu     sync.Mutex
	wg           sync.WaitGroup
	recentEvents []*Event
	recentIdx    int
	recentCount  int
}

func NewEventBus() *EventBus {
	ctx, cancel := context.WithCancel(context.Background())

	eb := &EventBus{
		subscribers:  make(map[string]*Subscriber),
		ctx:          ctx,
		cancel:       cancel,
		buffer:       make(chan *Event, 1000),
		recentEvents: make([]*Event, 1000),
	}

	eb.wg.Add(1)
	go eb.processEvents()

	return eb
}

func (eb *EventBus) Publish(event *Event) error {
	eb.closedMu.Lock()
	if eb.closed {
		eb.closedMu.Unlock()
		return fmt.Errorf("event bus is closed")
	}
	eb.closedMu.Unlock()

	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	if event.ID == "" {
		event.ID = fmt.Sprintf("%s-%d", event.Type, time.Now().UnixNano())
	}

	select {
	case eb.buffer <- event:
		return nil
	default:
		return fmt.Errorf("event buffer is full")
	}
}

func (eb *EventBus) Subscribe(subscriberID string, filter func(*Event) bool) *Subscriber {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if sub, exists := eb.subscribers[subscriberID]; exists {
		return sub
	}

	sub := &Subscriber{
		ID:      subscriberID,
		Channel: make(chan *Event, 100),
		Filter:  filter,
	}

	eb.subscribers[subscriberID] = sub
	return sub
}

func (eb *EventBus) Unsubscribe(subscriberID string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	delete(eb.subscribers, subscriberID)
}

func (eb *EventBus) processEvents() {
	defer eb.wg.Done()
	for {
		select {
		case <-eb.ctx.Done():
			return
		case event, ok := <-eb.buffer:
			if !ok || event == nil {
				return
			}
			eb.distributeEvent(event)
		}
	}
}

func (eb *EventBus) distributeEvent(event *Event) {
	eb.mu.Lock()
	eb.recentEvents[eb.recentIdx] = event
	eb.recentIdx = (eb.recentIdx + 1) % len(eb.recentEvents)
	if eb.recentCount < len(eb.recentEvents) {
		eb.recentCount++
	}
	eb.mu.Unlock()

	eb.mu.RLock()
	subs := make([]*Subscriber, 0, len(eb.subscribers))
	for _, sub := range eb.subscribers {
		subs = append(subs, sub)
	}
	eb.mu.RUnlock()

	for _, sub := range subs {
		if sub.Filter != nil && !sub.Filter(event) {
			continue
		}

		select {
		case sub.Channel <- event:
		default:
		}
	}
}

func (eb *EventBus) SubscriberCount() int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.subscribers)
}

func (eb *EventBus) GetRecentEvents(limit int, projectID, eventType string) []*Event {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	if limit <= 0 || limit > eb.recentCount {
		limit = eb.recentCount
	}

	result := make([]*Event, 0, limit)
	for i := 0; i < eb.recentCount && len(result) < limit; i++ {
		idx := (eb.recentIdx - 1 - i + len(eb.recentEvents)) % len(eb.recentEvents)
		ev := eb.recentEvents[idx]
		if ev == nil {
			continue
		}
		if projectID != "" && ev.ProjectID != projectID {
			continue
		}
		if eventType != "" && string(ev.Type) != eventType {
			continue
		}
		result = append(result, ev)
	}
	return result
}

func (eb *EventBus) Close() {
	eb.closedMu.Lock()
	if eb.closed {
		eb.closedMu.Unlock()
		return
	}
	eb.closed = true
	eb.closedMu.Unlock()

	eb.cancel()
	close(eb.buffer)
	eb.wg.Wait()

	eb.mu.Lock()
	defer eb.mu.Unlock()

	for _, sub := range eb.subscribers {
		close(sub.Channel)
	}
	eb.subscribers = make(map[string]*Subscriber)
}

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
