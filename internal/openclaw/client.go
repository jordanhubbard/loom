package openclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/pkg/config"
)

// Client communicates with the OpenClaw messaging gateway.
type Client struct {
	gatewayURL    string
	hookToken     string
	agentID       string
	channel       string
	recipient     string
	retryAttempts int
	retryDelay    time.Duration
	httpClient    *http.Client
}

// NewClient creates a new OpenClaw client. Returns nil if the integration is
// not enabled, allowing callers to treat a nil *Client as "disabled".
func NewClient(cfg *config.OpenClawConfig) *Client {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	retryAttempts := cfg.RetryAttempts
	if retryAttempts <= 0 {
		retryAttempts = 3
	}

	retryDelay := cfg.RetryDelay
	if retryDelay <= 0 {
		retryDelay = 2 * time.Second
	}

	return &Client{
		gatewayURL:    strings.TrimSuffix(cfg.GatewayURL, "/"),
		hookToken:     cfg.HookToken,
		agentID:       cfg.AgentID,
		channel:       cfg.DefaultChannel,
		recipient:     cfg.DefaultRecipient,
		retryAttempts: retryAttempts,
		retryDelay:    retryDelay,
		httpClient:    &http.Client{Timeout: timeout},
	}
}

// SendMessage sends an agent request to the OpenClaw /hooks/agent endpoint.
func (c *Client) SendMessage(ctx context.Context, req *AgentRequest) (*AgentResponse, error) {
	if req.AgentID == "" {
		req.AgentID = c.agentID
	}
	if req.Channel == "" {
		req.Channel = c.channel
	}
	if req.Recipient == "" {
		req.Recipient = c.recipient
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("openclaw: marshal request: %w", err)
	}

	url := c.gatewayURL + "/hooks/agent"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openclaw: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.hookToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.hookToken)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openclaw: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openclaw: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openclaw: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var agentResp AgentResponse
	if err := json.Unmarshal(respBody, &agentResp); err != nil {
		return nil, fmt.Errorf("openclaw: decode response: %w", err)
	}

	if !agentResp.OK {
		return &agentResp, fmt.Errorf("openclaw: gateway error: %s", agentResp.Error)
	}

	return &agentResp, nil
}

// SendMessageWithRetry sends a message with bounded retries.
func (c *Client) SendMessageWithRetry(ctx context.Context, req *AgentRequest) (*AgentResponse, error) {
	var lastErr error
	for attempt := 0; attempt < c.retryAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.retryDelay):
			}
		}

		resp, err := c.SendMessage(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("openclaw: exhausted %d retries: %w", c.retryAttempts, lastErr)
}

// Healthy checks gateway reachability with a HEAD request to the gateway root.
func (c *Client) Healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.gatewayURL+"/", nil)
	if err != nil {
		return false
	}
	if c.hookToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.hookToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// GatewayURL returns the configured gateway URL (useful for status reporting).
func (c *Client) GatewayURL() string {
	return c.gatewayURL
}
