package openclaw

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jordanhubbard/loom/internal/temporal/eventbus"
	"github.com/jordanhubbard/loom/pkg/config"
)

// Bridge subscribes to the EventBus and forwards relevant events to the
// OpenClaw messaging gateway so that humans (CEO, on-call) receive
// notifications about P0 decisions and escalations.
type Bridge struct {
	client          *Client
	eventBus        *eventbus.EventBus
	subscriber      *eventbus.Subscriber
	escalationsOnly bool
	cancel          context.CancelFunc
	done            chan struct{}
}

// NewBridge creates a new OpenClaw bridge. Returns nil if the client is nil
// (integration disabled) or the event bus is nil.
func NewBridge(client *Client, eb *eventbus.EventBus, cfg *config.OpenClawConfig) *Bridge {
	if client == nil || eb == nil {
		return nil
	}

	escalationsOnly := true
	if cfg != nil {
		escalationsOnly = cfg.EscalationsOnly
	}

	ctx, cancel := context.WithCancel(context.Background())

	b := &Bridge{
		client:          client,
		eventBus:        eb,
		escalationsOnly: escalationsOnly,
		cancel:          cancel,
		done:            make(chan struct{}),
	}

	// Subscribe to decision and motivation events.
	b.subscriber = eb.Subscribe("openclaw-bridge", func(e *eventbus.Event) bool {
		switch e.Type {
		case eventbus.EventTypeDecisionCreated,
			eventbus.EventTypeDecisionResolved,
			eventbus.EventTypeMotivationFired:
			return true
		}
		return false
	})

	go func() {
		defer close(b.done)
		b.run(ctx)
	}()
	return b
}

// Close unsubscribes from the event bus and stops the bridge goroutine.
// Blocks until the goroutine has exited. Safe to call multiple times.
func (b *Bridge) Close() {
	if b == nil {
		return
	}
	b.cancel()
	if b.eventBus != nil {
		b.eventBus.Unsubscribe("openclaw-bridge")
	}
	// Wait for goroutine to exit. The channel is closed exactly once by the
	// goroutine, so subsequent reads return immediately.
	<-b.done
}

// run processes events from the subscription channel.
func (b *Bridge) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-b.subscriber.Channel:
			if !ok {
				return
			}
			b.handleEvent(ctx, event)
		}
	}
}

// handleEvent formats a human-readable message from the event and sends it
// through the OpenClaw client.
func (b *Bridge) handleEvent(ctx context.Context, event *eventbus.Event) {
	if event == nil {
		return
	}

	msg, sessionKey, priority := b.formatMessage(event)
	if msg == "" {
		return
	}

	req := &AgentRequest{
		SessionKey: sessionKey,
		Message:    msg,
		Priority:   priority,
	}

	resp, err := b.client.SendMessageWithRetry(ctx, req)
	if err != nil {
		log.Printf("[OpenClaw] Failed to send message for %s: %v", event.Type, err)
		b.publishStatus(eventbus.EventTypeOpenClawMessageFailed, event, err.Error())
		return
	}

	log.Printf("[OpenClaw] Message sent for %s (message_id=%s)", event.Type, resp.MessageID)
	b.publishStatus(eventbus.EventTypeOpenClawMessageSent, event, resp.MessageID)
}

// formatMessage converts an EventBus event into a human-readable message,
// session key for reply correlation, and priority string. Returns empty
// message if the event should be skipped.
func (b *Bridge) formatMessage(event *eventbus.Event) (message, sessionKey, priority string) {
	data := event.Data
	if data == nil {
		data = make(map[string]interface{})
	}

	switch event.Type {
	case eventbus.EventTypeDecisionCreated:
		// Filter: if escalations-only, skip non-P0 decisions.
		if b.escalationsOnly {
			p, _ := data["priority"]
			pStr := fmt.Sprintf("%v", p)
			if pStr != "0" && !strings.EqualFold(pStr, "p0") {
				return "", "", ""
			}
		}

		decisionID, _ := data["decision_id"].(string)
		question, _ := data["question"].(string)
		recommendation, _ := data["recommendation"].(string)
		requester, _ := data["requester_id"].(string)
		projectID := event.ProjectID

		var sb strings.Builder
		sb.WriteString("P0 Decision Required\n\n")
		if projectID != "" {
			fmt.Fprintf(&sb, "Project: %s\n", projectID)
		}
		fmt.Fprintf(&sb, "Question: %s\n", question)
		if recommendation != "" {
			fmt.Fprintf(&sb, "Recommendation: %s\n", recommendation)
		}
		if requester != "" {
			fmt.Fprintf(&sb, "Requested by: %s\n", requester)
		}
		sb.WriteString("\nReply with: approve / deny / needs_more_info / <your decision>")

		sessionKey = "loom:decision:" + decisionID
		return sb.String(), sessionKey, "p0"

	case eventbus.EventTypeDecisionResolved:
		decisionID, _ := data["decision_id"].(string)
		decision, _ := data["decision"].(string)
		decider, _ := data["decider_id"].(string)

		msg := fmt.Sprintf("Decision resolved: %s\nDecided: %s\nBy: %s", decisionID, decision, decider)
		sessionKey = "loom:decision:" + decisionID
		return msg, sessionKey, ""

	case eventbus.EventTypeMotivationFired:
		if b.escalationsOnly {
			return "", "", ""
		}
		name, _ := data["motivation_name"].(string)
		reason, _ := data["reason"].(string)
		msg := fmt.Sprintf("Motivation fired: %s\nReason: %s", name, reason)
		return msg, "", ""
	}

	return "", "", ""
}

// publishStatus publishes an observability event on the event bus.
// Tolerates closed event bus channels during shutdown.
func (b *Bridge) publishStatus(eventType eventbus.EventType, source *eventbus.Event, detail string) {
	if b.eventBus == nil {
		return
	}

	// Recover from panic if the event bus buffer channel is already closed
	// during shutdown teardown.
	defer func() { recover() }()

	_ = b.eventBus.Publish(&eventbus.Event{
		Type:      eventType,
		Source:    "openclaw-bridge",
		ProjectID: source.ProjectID,
		Data: map[string]interface{}{
			"source_event_type": string(source.Type),
			"source_event_id":   source.ID,
			"detail":            detail,
		},
	})
}
