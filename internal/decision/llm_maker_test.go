package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jordanhubbard/loom/pkg/types"
)

// helpers

func makeAgents(names ...string) []*types.Agent {
	agents := make([]*types.Agent, len(names))
	for i, name := range names {
		agents[i] = &types.Agent{
			ID:           name,
			Name:         name,
			Type:         types.AgentTypeGeneral,
			Status:       types.AgentStatusIdle,
			Capabilities: []string{"code", "planning"},
		}
	}
	return agents
}

func makeTask(desc string) *types.Task {
	return &types.Task{
		Description: desc,
		Priority:    5,
	}
}

// TestLLMMaker_FallbackWhenUnconfigured verifies that LLMMaker falls back to
// SimpleMaker when no brain URL is configured.
func TestLLMMaker_FallbackWhenUnconfigured(t *testing.T) {
	m := NewLLMMaker("", "")
	if m.IsAvailable() {
		t.Error("expected IsAvailable=false when no URL configured")
	}
	agents := makeAgents("alice", "bob")
	chosen, err := m.DecideAgent(context.Background(), makeTask("do something"), agents)
	if err != nil {
		t.Fatalf("expected no error on fallback, got %v", err)
	}
	if chosen == nil {
		t.Error("expected a chosen agent even on fallback")
	}
}

// TestLLMMaker_UsesLLMResponse verifies that when the brain returns a valid
// agent name, LLMMaker returns that agent.
func TestLLMMaker_UsesLLMResponse(t *testing.T) {
	agents := makeAgents("alice", "bob", "charlie")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		resp := map[string]interface{}{
			"status": "completed",
			"result": "bob",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	m := NewLLMMaker(srv.URL, "test-token")
	if !m.IsAvailable() {
		t.Fatal("expected IsAvailable=true with configured URL")
	}

	chosen, err := m.DecideAgent(context.Background(), makeTask("review code for PR #42"), agents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chosen == nil {
		t.Fatal("expected agent to be chosen")
	}
	if chosen.Name != "bob" {
		t.Errorf("expected 'bob', got %q", chosen.Name)
	}
}

// TestLLMMaker_FallbackOnBrainError verifies graceful degradation when the
// brain API returns an error status.
func TestLLMMaker_FallbackOnBrainError(t *testing.T) {
	agents := makeAgents("alice", "bob")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	m := NewLLMMaker(srv.URL, "test-token")
	chosen, err := m.DecideAgent(context.Background(), makeTask("urgent task"), agents)
	if err != nil {
		t.Fatalf("expected no error even on brain 503, got %v", err)
	}
	if chosen == nil {
		t.Error("expected fallback to choose an agent")
	}
}

// TestLLMMaker_FallbackOnUnknownAgentName verifies that when the LLM responds
// with an unknown agent name, LLMMaker falls back to SimpleMaker.
func TestLLMMaker_FallbackOnUnknownAgentName(t *testing.T) {
	agents := makeAgents("alice", "bob")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{"status": "completed", "result": "dana"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	m := NewLLMMaker(srv.URL, "token")
	chosen, err := m.DecideAgent(context.Background(), makeTask("task"), agents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chosen == nil {
		t.Error("expected fallback to choose an agent when LLM returned unknown name")
	}
}

// TestLLMMaker_AnyResponse verifies that "any" response triggers fallback.
func TestLLMMaker_AnyResponse(t *testing.T) {
	agents := makeAgents("alice", "bob")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{"status": "completed", "result": "any"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	m := NewLLMMaker(srv.URL, "token")
	chosen, err := m.DecideAgent(context.Background(), makeTask("task"), agents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chosen == nil {
		t.Error("expected fallback to choose an agent on 'any' response")
	}
}

// TestLLMMaker_TimeoutFallback verifies that a slow brain triggers fallback
// rather than blocking indefinitely.
func TestLLMMaker_TimeoutFallback(t *testing.T) {
	agents := makeAgents("alice")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer srv.Close()

	m := NewLLMMaker(srv.URL, "token")
	m.timeout = 100 * time.Millisecond
	m.httpClient = &http.Client{Timeout: 100 * time.Millisecond}

	start := time.Now()
	chosen, err := m.DecideAgent(context.Background(), makeTask("urgent"), agents)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected no error on timeout (should fallback), got %v", err)
	}
	if chosen == nil {
		t.Error("expected fallback to choose an agent")
	}
	if elapsed > 2*time.Second {
		t.Errorf("timeout took too long: %v (should be < 1s)", elapsed)
	}
}
