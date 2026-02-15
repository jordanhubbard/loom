package openclaw

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jordanhubbard/loom/internal/temporal/eventbus"
	"github.com/jordanhubbard/loom/pkg/config"
)

// newTestEventBus creates an in-memory event bus for testing.
func newTestEventBus() *eventbus.EventBus {
	return eventbus.NewEventBus(nil, &config.TemporalConfig{
		EnableEventBus:  true,
		EventBufferSize: 100,
	})
}

func TestNewBridge_NilClient(t *testing.T) {
	eb := newTestEventBus()
	defer eb.Close()

	b := NewBridge(nil, eb, &config.OpenClawConfig{})
	if b != nil {
		t.Fatal("expected nil bridge when client is nil")
	}
}

func TestNewBridge_NilEventBus(t *testing.T) {
	c := NewClient(&config.OpenClawConfig{Enabled: true, GatewayURL: "http://localhost"})
	b := NewBridge(c, nil, &config.OpenClawConfig{})
	if b != nil {
		t.Fatal("expected nil bridge when event bus is nil")
	}
}

func TestBridge_P0DecisionForwarded(t *testing.T) {
	// Set up a test HTTP server that captures the message.
	received := make(chan *AgentRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req AgentRequest
		json.NewDecoder(r.Body).Decode(&req)
		received <- &req
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(AgentResponse{OK: true, MessageID: "msg-1"})
	}))
	defer srv.Close()

	eb := newTestEventBus()
	defer eb.Close()

	client := NewClient(&config.OpenClawConfig{
		Enabled:       true,
		GatewayURL:    srv.URL,
		RetryAttempts: 1,
	})

	b := NewBridge(client, eb, &config.OpenClawConfig{EscalationsOnly: true})
	defer b.Close()

	// Publish a P0 decision event.
	err := eb.Publish(&eventbus.Event{
		Type:      eventbus.EventTypeDecisionCreated,
		Source:    "test",
		ProjectID: "proj-1",
		Data: map[string]interface{}{
			"decision_id":    "bd-dec-1",
			"question":       "Should we deploy to prod?",
			"recommendation": "Yes",
			"requester_id":   "agent-42",
			"priority":       "0",
		},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Wait for the bridge to forward the message.
	select {
	case req := <-received:
		if req.SessionKey != "loom:decision:bd-dec-1" {
			t.Errorf("unexpected session key: %s", req.SessionKey)
		}
		if req.Priority != "p0" {
			t.Errorf("unexpected priority: %s", req.Priority)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for bridge to forward message")
	}
}

func TestBridge_NonP0Skipped(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(AgentResponse{OK: true})
	}))
	defer srv.Close()

	eb := newTestEventBus()
	defer eb.Close()

	client := NewClient(&config.OpenClawConfig{
		Enabled:       true,
		GatewayURL:    srv.URL,
		RetryAttempts: 1,
	})

	b := NewBridge(client, eb, &config.OpenClawConfig{EscalationsOnly: true})
	defer b.Close()

	// Publish a P2 decision event — should be skipped.
	_ = eb.Publish(&eventbus.Event{
		Type:   eventbus.EventTypeDecisionCreated,
		Source: "test",
		Data: map[string]interface{}{
			"decision_id": "bd-dec-2",
			"question":    "Use tabs or spaces?",
			"priority":    "2",
		},
	})

	// Give the bridge time to (not) process it.
	time.Sleep(200 * time.Millisecond)

	if callCount != 0 {
		t.Errorf("expected 0 calls for P2 decision, got %d", callCount)
	}
}

func TestBridge_CloseIdempotent(t *testing.T) {
	eb := newTestEventBus()
	defer eb.Close()

	client := NewClient(&config.OpenClawConfig{
		Enabled:    true,
		GatewayURL: "http://localhost",
	})

	b := NewBridge(client, eb, &config.OpenClawConfig{EscalationsOnly: true})
	b.Close()
	b.Close() // should not panic

	// nil bridge Close should also be safe
	var nb *Bridge
	nb.Close()
}
