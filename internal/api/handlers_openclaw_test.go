package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jordanhubbard/loom/internal/openclaw"
	"github.com/jordanhubbard/loom/pkg/config"
)

func TestHandleOpenClawWebhook_Disabled(t *testing.T) {
	s := &Server{config: &config.Config{}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/openclaw", nil)
	w := httptest.NewRecorder()
	s.handleOpenClawWebhook(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when disabled, got %d", w.Code)
	}
}

func TestHandleOpenClawWebhook_MethodNotAllowed(t *testing.T) {
	s := &Server{config: &config.Config{OpenClaw: config.OpenClawConfig{Enabled: true}}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/openclaw", nil)
	w := httptest.NewRecorder()
	s.handleOpenClawWebhook(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleOpenClawWebhook_InvalidSignature(t *testing.T) {
	s := &Server{config: &config.Config{
		OpenClaw: config.OpenClawConfig{
			Enabled:       true,
			WebhookSecret: "my-secret",
		},
	}}

	body := []byte(`{"text":"hello","session_key":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/openclaw", bytes.NewReader(body))
	req.Header.Set("X-OpenClaw-Signature", "sha256=wrong")
	w := httptest.NewRecorder()
	s.handleOpenClawWebhook(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for bad signature, got %d", w.Code)
	}
}

func TestHandleOpenClawWebhook_ValidSignature(t *testing.T) {
	secret := "test-webhook-secret"
	s := &Server{config: &config.Config{
		OpenClaw: config.OpenClawConfig{
			Enabled:       true,
			WebhookSecret: secret,
		},
	}}

	body := []byte(`{"text":"hello","session_key":"unknown:key"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/openclaw", bytes.NewReader(body))
	req.Header.Set("X-OpenClaw-Signature", sig)
	w := httptest.NewRecorder()
	s.handleOpenClawWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["processed"] != false {
		t.Errorf("expected processed=false for unknown session key")
	}
}

func TestHandleOpenClawWebhook_MissingText(t *testing.T) {
	s := &Server{config: &config.Config{
		OpenClaw: config.OpenClawConfig{Enabled: true},
	}}

	body := []byte(`{"session_key":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/openclaw", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleOpenClawWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing text, got %d", w.Code)
	}
}

func TestHandleOpenClawWebhook_InvalidJSON(t *testing.T) {
	s := &Server{config: &config.Config{
		OpenClaw: config.OpenClawConfig{Enabled: true},
	}}

	body := []byte(`not json`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/openclaw", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleOpenClawWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad JSON, got %d", w.Code)
	}
}

func TestHandleOpenClawStatus_Disabled(t *testing.T) {
	s := &Server{config: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openclaw/status", nil)
	w := httptest.NewRecorder()
	s.handleOpenClawStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["enabled"] != false {
		t.Errorf("expected enabled=false")
	}
}

func TestHandleOpenClawStatus_Enabled(t *testing.T) {
	s := &Server{config: &config.Config{
		OpenClaw: config.OpenClawConfig{
			Enabled:         true,
			GatewayURL:      "http://localhost:18789",
			EscalationsOnly: true,
		},
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openclaw/status", nil)
	w := httptest.NewRecorder()
	s.handleOpenClawStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["enabled"] != true {
		t.Errorf("expected enabled=true")
	}
	if resp["gateway_url"] != "http://localhost:18789" {
		t.Errorf("unexpected gateway_url: %v", resp["gateway_url"])
	}
}

func TestVerifyOpenClawSignature(t *testing.T) {
	secret := "my-secret"
	payload := []byte("test payload")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name      string
		payload   []byte
		signature string
		secret    string
		want      bool
	}{
		{"valid", payload, validSig, secret, true},
		{"no prefix", payload, hex.EncodeToString(mac.Sum(nil)), secret, true},
		{"wrong sig", payload, "sha256=deadbeef", secret, false},
		{"empty sig", payload, "", secret, false},
		{"empty secret", payload, validSig, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := verifyOpenClawSignature(tt.payload, tt.signature, tt.secret)
			if got != tt.want {
				t.Errorf("verifyOpenClawSignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Verify the openclaw types are importable and usable in this package.
func TestInboundMessageFields(t *testing.T) {
	msg := openclaw.InboundMessage{
		SessionKey: "loom:decision:123",
		Text:       "approve",
		Sender:     "ceo",
	}
	if msg.SessionKey != "loom:decision:123" {
		t.Error("unexpected session key")
	}
}
