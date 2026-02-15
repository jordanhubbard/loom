package openclaw

import "time"

// DeliveryStatus represents the delivery state of an outbound message.
type DeliveryStatus string

const (
	DeliveryStatusPending   DeliveryStatus = "pending"
	DeliveryStatusSent      DeliveryStatus = "sent"
	DeliveryStatusDelivered DeliveryStatus = "delivered"
	DeliveryStatusFailed    DeliveryStatus = "failed"
)

// AgentRequest is the payload POSTed to the OpenClaw /hooks/agent endpoint.
type AgentRequest struct {
	AgentID    string `json:"agent_id"`
	Channel    string `json:"channel,omitempty"`
	Recipient  string `json:"recipient,omitempty"`
	SessionKey string `json:"session_key,omitempty"` // correlation key for reply routing
	Message    string `json:"message"`
	Priority   string `json:"priority,omitempty"` // "p0", "p1", etc.
}

// AgentResponse is the response from the OpenClaw /hooks/agent endpoint.
type AgentResponse struct {
	OK        bool   `json:"ok"`
	MessageID string `json:"message_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// OutboundMessage records an outbound message for observability.
type OutboundMessage struct {
	DecisionID string         `json:"decision_id"`
	SessionKey string         `json:"session_key"`
	Message    string         `json:"message"`
	Status     DeliveryStatus `json:"status"`
	SentAt     time.Time      `json:"sent_at"`
	MessageID  string         `json:"message_id,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// InboundMessage is the payload POSTed by OpenClaw to loom's webhook endpoint.
type InboundMessage struct {
	SessionKey string `json:"session_key"`           // correlation key (e.g. "loom:decision:<id>")
	Sender     string `json:"sender,omitempty"`       // who replied
	Channel    string `json:"channel,omitempty"`      // originating channel
	Text       string `json:"text"`                   // reply body
	Timestamp  string `json:"timestamp,omitempty"`    // ISO-8601
	MessageID  string `json:"message_id,omitempty"`   // OpenClaw message ID
}
