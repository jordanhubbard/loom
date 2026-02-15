package openclaw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jordanhubbard/loom/pkg/config"
)

func TestNewClient_Disabled(t *testing.T) {
	c := NewClient(&config.OpenClawConfig{Enabled: false})
	if c != nil {
		t.Fatal("expected nil client when disabled")
	}
}

func TestNewClient_NilConfig(t *testing.T) {
	c := NewClient(nil)
	if c != nil {
		t.Fatal("expected nil client for nil config")
	}
}

func TestNewClient_Enabled(t *testing.T) {
	c := NewClient(&config.OpenClawConfig{
		Enabled:    true,
		GatewayURL: "http://localhost:18789",
		HookToken:  "tok",
		AgentID:    "loom",
	})
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.GatewayURL() != "http://localhost:18789" {
		t.Fatalf("unexpected gateway URL: %s", c.GatewayURL())
	}
}

func TestSendMessage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/hooks/agent" {
			t.Errorf("expected /hooks/agent, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected bearer token, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected json content type, got %s", r.Header.Get("Content-Type"))
		}

		var req AgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Message != "hello CEO" {
			t.Errorf("unexpected message: %s", req.Message)
		}
		if req.AgentID != "loom" {
			t.Errorf("unexpected agent_id: %s", req.AgentID)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(AgentResponse{OK: true, MessageID: "msg-123"})
	}))
	defer srv.Close()

	c := NewClient(&config.OpenClawConfig{
		Enabled:    true,
		GatewayURL: srv.URL,
		HookToken:  "test-token",
		AgentID:    "loom",
	})

	resp, err := c.SendMessage(context.Background(), &AgentRequest{
		Message:    "hello CEO",
		SessionKey: "loom:decision:bd-dec-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MessageID != "msg-123" {
		t.Errorf("unexpected message_id: %s", resp.MessageID)
	}
}

func TestSendMessage_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("gateway down"))
	}))
	defer srv.Close()

	c := NewClient(&config.OpenClawConfig{
		Enabled:    true,
		GatewayURL: srv.URL,
	})

	_, err := c.SendMessage(context.Background(), &AgentRequest{Message: "test"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestSendMessage_GatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(AgentResponse{OK: false, Error: "rate limited"})
	}))
	defer srv.Close()

	c := NewClient(&config.OpenClawConfig{
		Enabled:    true,
		GatewayURL: srv.URL,
	})

	_, err := c.SendMessage(context.Background(), &AgentRequest{Message: "test"})
	if err == nil {
		t.Fatal("expected error for gateway error response")
	}
}

func TestSendMessageWithRetry_EventualSuccess(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("try again"))
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(AgentResponse{OK: true, MessageID: "msg-ok"})
	}))
	defer srv.Close()

	c := NewClient(&config.OpenClawConfig{
		Enabled:       true,
		GatewayURL:    srv.URL,
		RetryAttempts: 3,
		RetryDelay:    1 * time.Millisecond,
	})

	resp, err := c.SendMessageWithRetry(context.Background(), &AgentRequest{Message: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MessageID != "msg-ok" {
		t.Errorf("unexpected message_id: %s", resp.MessageID)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestSendMessageWithRetry_AllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("down"))
	}))
	defer srv.Close()

	c := NewClient(&config.OpenClawConfig{
		Enabled:       true,
		GatewayURL:    srv.URL,
		RetryAttempts: 2,
		RetryDelay:    1 * time.Millisecond,
	})

	_, err := c.SendMessageWithRetry(context.Background(), &AgentRequest{Message: "test"})
	if err == nil {
		t.Fatal("expected error after all retries fail")
	}
}

func TestHealthy_Up(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("expected HEAD, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(&config.OpenClawConfig{
		Enabled:    true,
		GatewayURL: srv.URL,
	})

	if !c.Healthy(context.Background()) {
		t.Error("expected healthy")
	}
}

func TestHealthy_Down(t *testing.T) {
	c := NewClient(&config.OpenClawConfig{
		Enabled:    true,
		GatewayURL: "http://127.0.0.1:1", // unlikely to be listening
		Timeout:    100 * time.Millisecond,
	})

	if c.Healthy(context.Background()) {
		t.Error("expected unhealthy for unreachable gateway")
	}
}

func TestDefaultsFilled(t *testing.T) {
	c := NewClient(&config.OpenClawConfig{
		Enabled:          true,
		GatewayURL:       "http://gw:8080/",
		DefaultChannel:   "whatsapp",
		DefaultRecipient: "ceo@example.com",
		AgentID:          "loom-prod",
	})

	if c.gatewayURL != "http://gw:8080" {
		t.Errorf("trailing slash not stripped: %s", c.gatewayURL)
	}
	if c.channel != "whatsapp" {
		t.Errorf("unexpected channel: %s", c.channel)
	}
	if c.recipient != "ceo@example.com" {
		t.Errorf("unexpected recipient: %s", c.recipient)
	}
}
