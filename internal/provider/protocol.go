package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Protocol defines the interface for communicating with AI providers
// using OpenAI-compatible APIs
type Protocol interface {
	// CreateChatCompletion sends a chat completion request
	CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error)
	
	// GetModels lists available models
	GetModels(ctx context.Context) ([]Model, error)
}

// ChatMessage represents a message in the chat
type ChatMessage struct {
	Role    string `json:"role"`    // system, user, assistant
	Content string `json:"content"` // message content
}

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
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
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
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
			Timeout: 60 * time.Second,
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
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}
	
	// Unmarshal response
	var completionResp ChatCompletionResponse
	if err := json.Unmarshal(respBody, &completionResp); err != nil {
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
	
	// Unmarshal response
	var modelsResp struct {
		Data []Model `json:"data"`
	}
	if err := json.Unmarshal(respBody, &modelsResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	return modelsResp.Data, nil
}
