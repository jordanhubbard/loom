package provider

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// ProviderConfig represents the configuration for a provider
type ProviderConfig struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Type                   string    `json:"type"` // openai, anthropic, local, etc.
	Endpoint               string    `json:"endpoint"`
	APIKey                 string    `json:"api_key,omitempty"`
	Model                  string    `json:"model"` // effective model to use
	ConfiguredModel        string    `json:"configured_model,omitempty"`
	SelectedModel          string    `json:"selected_model,omitempty"`
	SelectedGPU            string    `json:"selected_gpu,omitempty"`
	Status                 string    `json:"status,omitempty"`
	LastHeartbeatAt        time.Time `json:"last_heartbeat_at,omitempty"`
	LastHeartbeatLatencyMs int64     `json:"last_heartbeat_latency_ms,omitempty"`
	CapabilityScore        float64   `json:"capability_score,omitempty"` // Composite score: 60% quality + 20% throughput + 20% latency
	ContextWindow          int       `json:"context_window,omitempty"`
}

// MetricsCallback is called after each provider request to record metrics
type MetricsCallback func(providerID string, success bool, latencyMs int64, totalTokens int64)

// Registry manages registered AI providers
type Registry struct {
	mu              sync.RWMutex
	providers       map[string]*RegisteredProvider
	metricsCallback MetricsCallback
	rrCounter       uint64 // Round-robin counter for equal-priority providers
}

// RegisteredProvider wraps a provider with its configuration and protocol
type RegisteredProvider struct {
	Config   *ProviderConfig
	Protocol Protocol
}

// NewRegistry creates a new provider registry
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]*RegisteredProvider),
	}
}

// Clear removes all registered providers.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers = make(map[string]*RegisteredProvider)
}

// Register registers a new provider
func (r *Registry) Register(config *ProviderConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if config.Status == "" {
		config.Status = "pending"
	}

	// Check if provider already exists
	if _, exists := r.providers[config.ID]; exists {
		return fmt.Errorf("provider %s already registered", config.ID)
	}

	// Create protocol based on provider type
	var protocol Protocol
	switch config.Type {
	case "openai", "anthropic", "local", "custom", "vllm":
		// All use OpenAI-compatible protocol
		protocol = NewOpenAIProvider(config.Endpoint, config.APIKey)
	case "ollama":
		protocol = NewOllamaProvider(config.Endpoint)
	case "mock":
		protocol = NewMockProvider()
	default:
		return fmt.Errorf("unsupported provider type: %s", config.Type)
	}

	// Register provider
	r.providers[config.ID] = &RegisteredProvider{
		Config:   config,
		Protocol: protocol,
	}

	return nil
}

// Upsert registers a provider if it doesn't exist, or replaces it if it does.
func (r *Registry) Upsert(config *ProviderConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if config.Status == "" {
		config.Status = "pending"
	}

	var protocol Protocol
	switch config.Type {
	case "openai", "anthropic", "local", "custom", "vllm":
		protocol = NewOpenAIProvider(config.Endpoint, config.APIKey)
	case "ollama":
		protocol = NewOllamaProvider(config.Endpoint)
	case "mock":
		protocol = NewMockProvider()
	default:
		return fmt.Errorf("unsupported provider type: %s", config.Type)
	}

	r.providers[config.ID] = &RegisteredProvider{Config: config, Protocol: protocol}
	return nil
}

// Unregister removes a provider from the registry
func (r *Registry) Unregister(providerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[providerID]; !exists {
		return fmt.Errorf("provider %s not found", providerID)
	}

	delete(r.providers, providerID)
	return nil
}

// Get retrieves a registered provider
func (r *Registry) Get(providerID string) (*RegisteredProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[providerID]
	if !exists {
		return nil, fmt.Errorf("provider %s not found", providerID)
	}

	return provider, nil
}

// List returns all registered providers
func (r *Registry) List() []*RegisteredProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]*RegisteredProvider, 0, len(r.providers))
	for _, provider := range r.providers {
		providers = append(providers, provider)
	}

	return providers
}

// ListActive returns registered providers with active status, sorted by
// capability score (highest first). Providers with equal scores are
// round-robined so all get work.
func (r *Registry) ListActive() []*RegisteredProvider {
	r.mu.RLock()
	counter := r.rrCounter
	r.mu.RUnlock()

	r.mu.RLock()
	providers := make([]*RegisteredProvider, 0, len(r.providers))
	for _, provider := range r.providers {
		if provider != nil && provider.Config != nil && isProviderHealthy(provider.Config.Status) {
			providers = append(providers, provider)
		}
	}
	r.mu.RUnlock()

	if len(providers) <= 1 {
		return providers
	}

	// Sort by capability score descending
	sort.SliceStable(providers, func(i, j int) bool {
		si := providers[i].Config.CapabilityScore
		sj := providers[j].Config.CapabilityScore
		return si > sj
	})

	// Find the group of providers with equal top score and rotate them
	topScore := providers[0].Config.CapabilityScore
	equalCount := 0
	for _, p := range providers {
		if p.Config.CapabilityScore == topScore {
			equalCount++
		} else {
			break
		}
	}

	if equalCount > 1 {
		// Round-robin within the equal-score group
		rotation := int(counter) % equalCount
		rotated := make([]*RegisteredProvider, 0, len(providers))
		rotated = append(rotated, providers[rotation:equalCount]...)
		rotated = append(rotated, providers[:rotation]...)
		rotated = append(rotated, providers[equalCount:]...)
		providers = rotated
	}

	// Increment counter for next call
	r.mu.Lock()
	r.rrCounter++
	r.mu.Unlock()

	return providers
}

// IsActive returns true if the provider is registered and active.
func (r *Registry) IsActive(providerID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[providerID]
	if !exists || provider == nil || provider.Config == nil {
		return false
	}
	return isProviderHealthy(provider.Config.Status)
}

// SetMetricsCallback sets the callback function for recording metrics
func (r *Registry) SetMetricsCallback(callback MetricsCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.metricsCallback = callback
}

// SendChatCompletionStream sends a streaming chat completion request to a provider
func (r *Registry) SendChatCompletionStream(ctx context.Context, providerID string, req *ChatCompletionRequest, handler StreamHandler) error {
	start := time.Now()

	// Get provider
	registered, err := r.Get(providerID)
	if err != nil {
		return err
	}

	// Check if provider supports streaming
	streamProvider, ok := registered.Protocol.(StreamingProtocol)
	if !ok {
		return fmt.Errorf("provider %s does not support streaming", providerID)
	}

	// Send streaming request
	err = streamProvider.CreateChatCompletionStream(ctx, req, handler)

	// Record metrics
	latencyMs := time.Since(start).Milliseconds()
	if r.metricsCallback != nil {
		r.metricsCallback(providerID, err == nil, latencyMs, 0)
	}

	return err
}

// SendChatCompletion sends a chat completion request to a provider
func (r *Registry) SendChatCompletion(ctx context.Context, providerID string, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	startTime := time.Now()

	provider, err := r.Get(providerID)
	if err != nil {
		return nil, err
	}
	if provider.Config != nil && !isProviderHealthy(provider.Config.Status) {
		return nil, fmt.Errorf("provider %s is disabled", providerID)
	}

	// Use default model if not specified
	if req.Model == "" {
		req.Model = provider.Config.Model
	}

	// Make the request
	resp, err := provider.Protocol.CreateChatCompletion(ctx, req)

	// If model not found (404), the vLLM server may have restarted with a
	// different model. Rediscover available models and retry once.
	if err != nil && (strings.Contains(err.Error(), "status code 404") || strings.Contains(err.Error(), "not found")) {
		log.Printf("[Registry] Model %q returned 404 on provider %s — rediscovering models", req.Model, providerID)
		models, modelErr := provider.Protocol.GetModels(ctx)
		if modelErr == nil && len(models) > 0 {
			newModel := models[0].ID
			log.Printf("[Registry] Provider %s model changed: %q → %q", providerID, req.Model, newModel)
			r.mu.Lock()
			if p, ok := r.providers[providerID]; ok && p.Config != nil {
				p.Config.Model = newModel
				p.Config.SelectedModel = newModel
			}
			r.mu.Unlock()
			req.Model = newModel
			resp, err = provider.Protocol.CreateChatCompletion(ctx, req)
		}
	}

	// Record metrics
	latencyMs := time.Since(startTime).Milliseconds()
	success := err == nil
	totalTokens := int64(0)
	if resp != nil {
		totalTokens = int64(resp.Usage.TotalTokens)
	}

	// Call metrics callback if registered
	r.mu.RLock()
	callback := r.metricsCallback
	r.mu.RUnlock()

	if callback != nil {
		callback(providerID, success, latencyMs, totalTokens)
	}

	return resp, err
}

// GetModels retrieves available models from a provider
func (r *Registry) GetModels(ctx context.Context, providerID string) ([]Model, error) {
	provider, err := r.Get(providerID)
	if err != nil {
		return nil, err
	}

	return provider.Protocol.GetModels(ctx)
}

func isProviderHealthy(status string) bool {
	return status == "healthy" || status == "active"
}
