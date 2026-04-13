package decision

// LLMMaker is an AI-backed decision maker that calls the RCC brain API
// for reasoning about which agent should handle a task.
//
// It satisfies the types.DecisionMaker interface and is used in place of (or chained
// after) SimpleMaker when LOOM_RCC_BRAIN_URL is set in the environment.
// When the brain API is unavailable or times out, it falls back to
// SimpleMaker so loom is never blocked on RCC availability.
//
// RCC Brain API: POST /api/brain/request
//   Body: { messages: [{role:"user", content: <prompt>}], maxTokens: 512, priority: "normal" }
//   Response: { status: "completed", result: "<model output>" }
//
// The prompt is constructed from the task description and the list of
// available agent capabilities.  The LLM is asked to respond with a
// single agent name or "any" if it has no preference.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/pkg/types"
)

// LLMMaker implements the types.DecisionMaker interface using the RCC brain API.
type LLMMaker struct {
	brainURL   string
	brainToken string
	timeout    time.Duration
	fallback   *SimpleMaker
	httpClient *http.Client
}

// Ensure LLMMaker satisfies the types.DecisionMaker interface at compile time.
var _ types.DecisionMaker = (*LLMMaker)(nil)

// NewLLMMaker creates a new LLM-backed decision maker.
// brainURL is the RCC brain endpoint (e.g. "http://rocky:8789").
// brainToken is the RCC agent token for authentication.
// If either is empty, reads from LOOM_RCC_BRAIN_URL / LOOM_RCC_AGENT_TOKEN env vars.
func NewLLMMaker(brainURL, brainToken string) *LLMMaker {
	if brainURL == "" {
		brainURL = os.Getenv("LOOM_RCC_BRAIN_URL")
	}
	if brainToken == "" {
		brainToken = os.Getenv("LOOM_RCC_AGENT_TOKEN")
	}
	timeout := 10 * time.Second
	return &LLMMaker{
		brainURL:   brainURL,
		brainToken: brainToken,
		timeout:    timeout,
		fallback:   NewSimpleMaker(),
		httpClient: &http.Client{Timeout: timeout},
	}
}

// IsAvailable returns true if the brain URL is configured.
func (m *LLMMaker) IsAvailable() bool {
	return m.brainURL != ""
}

// EvaluatePriority delegates priority evaluation to the SimpleMaker fallback.
// LLM-based priority scoring is not currently implemented.
func (m *LLMMaker) EvaluatePriority(task *types.Task) int {
	return m.fallback.EvaluatePriority(task)
}

// DecideAgent selects the most appropriate agent for a task using LLM reasoning.
// Falls back to SimpleMaker if the brain API is unreachable or returns an
// unusable response.
func (m *LLMMaker) DecideAgent(ctx context.Context, task *types.Task, agents []*types.Agent) (*types.Agent, error) {
	if !m.IsAvailable() || len(agents) == 0 {
		return m.fallback.DecideAgent(ctx, task, agents)
	}

	prompt := m.buildPrompt(task, agents)
	result, err := m.callBrain(ctx, prompt)
	if err != nil {
		// Brain unreachable — fall back silently
		return m.fallback.DecideAgent(ctx, task, agents)
	}

	// Parse LLM response: look for an agent name in the output
	chosen := m.parseAgentChoice(result, agents)
	if chosen != nil {
		return chosen, nil
	}
	// LLM gave an unusable response — fall back
	return m.fallback.DecideAgent(ctx, task, agents)
}

// buildPrompt constructs the LLM prompt for agent selection.
func (m *LLMMaker) buildPrompt(task *types.Task, agents []*types.Agent) string {
	var sb strings.Builder
	sb.WriteString("You are a task router for an autonomous agent system called Loom.\n\n")
	sb.WriteString("Task:\n")
	if task.Description != "" {
		sb.WriteString(fmt.Sprintf("  Description: %s\n", task.Description))
	}
	sb.WriteString(fmt.Sprintf("  Priority:    %d\n", task.Priority))
	sb.WriteString("\nAvailable agents:\n")
	for _, a := range agents {
		if a.Status != types.AgentStatusIdle {
			continue
		}
		sb.WriteString(fmt.Sprintf("  - %s (type: %s, capabilities: %s)\n",
			a.Name, string(a.Type), strings.Join(a.Capabilities, ", ")))
	}
	sb.WriteString("\nRespond with ONLY the name of the most appropriate agent ")
	sb.WriteString("(exactly as listed above), or 'any' if you have no preference. ")
	sb.WriteString("Do not add explanation.")
	return sb.String()
}

// callBrain sends the prompt to the RCC brain API and returns the text result.
func (m *LLMMaker) callBrain(ctx context.Context, prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"messages":  []map[string]string{{"role": "user", "content": prompt}},
		"maxTokens": 64,
		"priority":  "normal",
		"metadata":  map[string]string{"source": "loom-decision"},
	}
	body, _ := json.Marshal(reqBody)

	url := strings.TrimRight(m.brainURL, "/") + "/api/brain/request"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.brainToken != "" {
		req.Header.Set("Authorization", "Bearer "+m.brainToken)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("brain API unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("brain API returned HTTP %d", resp.StatusCode)
	}

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}

	var respData struct {
		Status string `json:"status"`
		Result string `json:"result"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &respData); err != nil {
		return "", fmt.Errorf("brain API response parse error: %w", err)
	}
	if respData.Status != "completed" {
		return "", fmt.Errorf("brain API status %q (error: %s)", respData.Status, respData.Error)
	}
	return strings.TrimSpace(respData.Result), nil
}

// parseAgentChoice finds the agent whose name matches the LLM response.
// Returns nil if no match found (caller should fall back to SimpleMaker).
func (m *LLMMaker) parseAgentChoice(result string, agents []*types.Agent) *types.Agent {
	result = strings.ToLower(strings.TrimSpace(result))
	if result == "any" {
		return nil // let fallback choose
	}
	for _, a := range agents {
		if strings.ToLower(a.Name) == result {
			return a
		}
	}
	// Partial match — first agent whose name is a prefix of the result
	for _, a := range agents {
		if strings.HasPrefix(result, strings.ToLower(a.Name)) {
			return a
		}
	}
	return nil
}
