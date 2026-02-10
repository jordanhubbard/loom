package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ContextLengthError is returned when the provider rejects a request because
// the input exceeds the model's context window. Callers can check for this
// with errors.As and retry with fewer/shorter messages.
type ContextLengthError struct {
	StatusCode int
	Body       string
}

func (e *ContextLengthError) Error() string {
	return fmt.Sprintf("context length exceeded (HTTP %d): %s", e.StatusCode, e.Body)
}

// isContextLengthError checks whether a provider error body indicates the
// prompt exceeded the model's context window.
func isContextLengthError(body string) bool {
	lower := strings.ToLower(body)
	patterns := []string{
		"context length",
		"context_length",
		"prompt is too long",
		"input is too long",
		"maximum context",
		"token limit",
		"too many tokens",
		"exceed",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// Protocol defines the interface for communicating with AI providers
// using OpenAI-compatible APIs
type Protocol interface {
	// CreateChatCompletion sends a chat completion request
	CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error)

	// GetModels lists available models
	GetModels(ctx context.Context) ([]Model, error)
}

// StreamingProtocol extends Protocol with streaming support
type StreamingProtocol interface {
	Protocol
	// CreateChatCompletionStream sends a streaming chat completion request
	CreateChatCompletionStream(ctx context.Context, req *ChatCompletionRequest, handler StreamHandler) error
}

// ChatMessage represents a message in the chat
type ChatMessage struct {
	Role    string `json:"role"`    // system, user, assistant
	Content string `json:"content"` // message content
}

// ResponseFormat specifies the output format for the LLM response.
// Setting Type to "json_object" enables constrained JSON decoding in
// vLLM and OpenAI-compatible APIs, guaranteeing valid JSON output.
type ResponseFormat struct {
	Type string `json:"type"` // "text" (default) or "json_object"
}

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Model          string          `json:"model"`
	Messages       []ChatMessage   `json:"messages"`
	Temperature    float64         `json:"temperature,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Stream         bool            `json:"stream,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

// ChatCompletionResponse represents a chat completion response
type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int         `json:"index"`
		Message ChatMessage `json:"message"`
		Finish  string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Model represents an AI model
type Model struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	Created     int64  `json:"created"`
	OwnedBy     string `json:"owned_by"`
	MaxModelLen int    `json:"max_model_len,omitempty"` // vLLM: maximum context length in tokens
}

// OpenAIProvider implements the Protocol interface for OpenAI-compatible APIs
type OpenAIProvider struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

// NewOpenAIProvider creates a new OpenAI-compatible provider
func NewOpenAIProvider(endpoint, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		endpoint: strings.TrimSuffix(endpoint, "/"),
		apiKey:   apiKey,
		client: &http.Client{
			Timeout: 5 * time.Minute, // Local models can be slow for long text generation
		},
	}
}

// CreateChatCompletion sends a chat completion request
func (p *OpenAIProvider) CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	url := fmt.Sprintf("%s/chat/completions", p.endpoint)

	// Marshal request body
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))
	}

	// Send request
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		bodyStr := string(respBody)
		if resp.StatusCode == http.StatusBadRequest && isContextLengthError(bodyStr) {
			return nil, &ContextLengthError{StatusCode: resp.StatusCode, Body: bodyStr}
		}
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, bodyStr)
	}

	// Extract and unmarshal JSON response (handling extraneous text)
	var completionResp ChatCompletionResponse
	if err := unmarshalJSON(respBody, &completionResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &completionResp, nil
}

// GetModels lists available models
func (p *OpenAIProvider) GetModels(ctx context.Context) ([]Model, error) {
	url := fmt.Sprintf("%s/models", p.endpoint)

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))
	}

	// Send request
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	// Extract and unmarshal JSON response (handling extraneous text)
	var modelsResp struct {
		Data []Model `json:"data"`
	}
	if err := unmarshalJSON(respBody, &modelsResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return modelsResp.Data, nil
}

// unmarshalJSON extracts and unmarshals JSON from a response body that may contain
// extraneous text before or after the JSON. This ensures we follow the OpenAI API
// specification while being robust to extra content from providers.
func unmarshalJSON(data []byte, v interface{}) error {
	// First, try direct unmarshal (most common case - pure JSON)
	if err := json.Unmarshal(data, v); err == nil {
		return nil
	}

	// If that fails, extract JSON from the response
	jsonData := extractJSON(data)
	if jsonData == nil {
		return fmt.Errorf("no valid JSON found in response: %s", string(data))
	}

	// Try to unmarshal the extracted JSON
	if err := json.Unmarshal(jsonData, v); err != nil {
		return fmt.Errorf("failed to unmarshal extracted JSON: %w (data: %s)", err, string(jsonData))
	}

	return nil
}

// extractJSON finds and extracts valid JSON from a byte slice that may contain
// extraneous text. It looks for JSON objects ({...}) or arrays ([...]).
func extractJSON(data []byte) []byte {
	// Trim whitespace
	data = bytes.TrimSpace(data)

	// Look for JSON object start
	if idx := bytes.IndexByte(data, '{'); idx != -1 {
		// Find the matching closing brace
		if closing := findClosingBrace(data[idx:]); closing != -1 {
			return data[idx : idx+closing+1]
		}
	}

	// Look for JSON array start
	if idx := bytes.IndexByte(data, '['); idx != -1 {
		// Find the matching closing bracket
		if closing := findClosingBracket(data[idx:]); closing != -1 {
			return data[idx : idx+closing+1]
		}
	}

	return nil
}

// findClosingBrace finds the index of the closing brace that matches the opening brace
// at position 0, accounting for nested braces and strings.
func findClosingBrace(data []byte) int {
	depth := 0
	inString := false
	escaped := false

	for i := 0; i < len(data); i++ {
		if escaped {
			escaped = false
			continue
		}

		switch data[i] {
		case '\\':
			if inString {
				escaped = true
			}
		case '"':
			inString = !inString
		case '{':
			if !inString {
				depth++
			}
		case '}':
			if !inString {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}

	return -1
}

// findClosingBracket finds the index of the closing bracket that matches the opening bracket
// at position 0, accounting for nested brackets and strings.
func findClosingBracket(data []byte) int {
	depth := 0
	inString := false
	escaped := false

	for i := 0; i < len(data); i++ {
		if escaped {
			escaped = false
			continue
		}

		switch data[i] {
		case '\\':
			if inString {
				escaped = true
			}
		case '"':
			inString = !inString
		case '[':
			if !inString {
				depth++
			}
		case ']':
			if !inString {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}

	return -1
}
