package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/internal/openclaw"
	"github.com/jordanhubbard/loom/internal/temporal/eventbus"
)

// handleOpenClawWebhook receives inbound messages from the OpenClaw gateway.
// POST /api/v1/webhooks/openclaw
func (s *Server) handleOpenClawWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Graceful degradation: return 404 when integration is disabled.
	if s.config == nil || !s.config.OpenClaw.Enabled {
		s.respondError(w, http.StatusNotFound, "OpenClaw integration is not enabled")
		return
	}

	// Read body (needed for HMAC verification before parsing).
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	defer r.Body.Close()

	// Verify HMAC signature when a webhook secret is configured.
	if s.config.OpenClaw.WebhookSecret != "" {
		signature := r.Header.Get("X-OpenClaw-Signature")
		if !verifyOpenClawSignature(body, signature, s.config.OpenClaw.WebhookSecret) {
			s.respondError(w, http.StatusUnauthorized, "Invalid webhook signature")
			return
		}
	}

	// Parse payload.
	var msg openclaw.InboundMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	if msg.Text == "" {
		s.respondError(w, http.StatusBadRequest, "Missing text field")
		return
	}

	// Publish raw inbound event for observability.
	if s.app != nil {
		if eb := s.app.GetEventBus(); eb != nil {
			_ = eb.Publish(&eventbus.Event{
				Type:   eventbus.EventTypeOpenClawMessageReceived,
				Source: "openclaw-webhook",
				Data: map[string]interface{}{
					"session_key": msg.SessionKey,
					"sender":      msg.Sender,
					"channel":     msg.Channel,
					"message_id":  msg.MessageID,
				},
			})
		}
	}

	// Route based on session key.
	if strings.HasPrefix(msg.SessionKey, "loom:decision:") {
		result := s.processDecisionReply(&msg)
		s.respondJSON(w, http.StatusOK, result)
		return
	}

	// Unknown session key — acknowledge receipt.
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "received",
		"session_key": msg.SessionKey,
		"processed":   false,
	})
}

// processDecisionReply maps a CEO text reply to a decision resolution.
// Recognized commands: "approve", "deny", "needs_more_info".
// Anything else is treated as a free-form decision.
func (s *Server) processDecisionReply(msg *openclaw.InboundMessage) map[string]interface{} {
	decisionID := strings.TrimPrefix(msg.SessionKey, "loom:decision:")

	if s.app == nil {
		return map[string]interface{}{"status": "error", "error": "loom not initialized"}
	}

	dm := s.app.GetDecisionManager()
	if dm == nil {
		return map[string]interface{}{"status": "error", "error": "decision manager not available"}
	}

	text := strings.TrimSpace(msg.Text)
	normalized := strings.ToLower(text)

	var decisionText, rationale string
	switch normalized {
	case "approve", "approved", "yes", "lgtm":
		decisionText = "approved"
		rationale = "Approved via OpenClaw by " + msg.Sender
	case "deny", "denied", "no", "reject", "rejected":
		decisionText = "denied"
		rationale = "Denied via OpenClaw by " + msg.Sender
	case "needs_more_info", "more info", "need more info":
		decisionText = "needs_more_info"
		rationale = "More information requested via OpenClaw by " + msg.Sender
	default:
		// Free-form decision text.
		decisionText = text
		rationale = "Free-form decision via OpenClaw by " + msg.Sender
	}

	deciderID := "ceo"
	if msg.Sender != "" {
		deciderID = msg.Sender
	}

	if err := dm.MakeDecision(decisionID, deciderID, decisionText, rationale); err != nil {
		return map[string]interface{}{
			"status":      "error",
			"decision_id": decisionID,
			"error":       err.Error(),
		}
	}

	// Publish reply-processed event.
	if eb := s.app.GetEventBus(); eb != nil {
		_ = eb.Publish(&eventbus.Event{
			Type:   eventbus.EventTypeOpenClawReplyProcessed,
			Source: "openclaw-webhook",
			Data: map[string]interface{}{
				"decision_id": decisionID,
				"decision":    decisionText,
				"decider_id":  deciderID,
				"channel":     msg.Channel,
			},
		})
	}

	return map[string]interface{}{
		"status":      "resolved",
		"decision_id": decisionID,
		"decision":    decisionText,
	}
}

// handleOpenClawStatus reports the health and configuration state of the
// OpenClaw integration.
// GET /api/v1/openclaw/status
func (s *Server) handleOpenClawStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	enabled := s.config != nil && s.config.OpenClaw.Enabled
	status := map[string]interface{}{
		"enabled": enabled,
	}

	if !enabled {
		s.respondJSON(w, http.StatusOK, status)
		return
	}

	status["gateway_url"] = s.config.OpenClaw.GatewayURL
	status["escalations_only"] = s.config.OpenClaw.EscalationsOnly
	status["default_channel"] = s.config.OpenClaw.DefaultChannel

	// Check gateway health if loom has the client.
	if s.app != nil {
		if client := s.app.GetOpenClawClient(); client != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			status["healthy"] = client.Healthy(ctx)
		} else {
			status["healthy"] = false
		}
	}

	s.respondJSON(w, http.StatusOK, status)
}

// verifyOpenClawSignature verifies an HMAC-SHA256 signature from the OpenClaw
// gateway, following the same pattern as verifyGitHubSignature.
func verifyOpenClawSignature(payload []byte, signature, secret string) bool {
	if signature == "" || secret == "" {
		return false
	}

	// Strip optional "sha256=" prefix.
	signature = strings.TrimPrefix(signature, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}
