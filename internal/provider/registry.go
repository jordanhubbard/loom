package provider

import (
	"context"
	"fmt"
	"sync"
)

// ProviderConfig represents the configuration for a provider
type ProviderConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // openai, anthropic, local, etc.
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"api_key,omitempty"`
	Model    string `json:"model"` // default model to use
}

// Registry manages registered AI providers
type Registry struct {
	mu        sync.RWMutex
	providers map[string]*RegisteredProvider
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

// Register registers a new provider
func (r *Registry) Register(config *ProviderConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Check if provider already exists
	if _, exists := r.providers[config.ID]; exists {
		return fmt.Errorf("provider %s already registered", config.ID)
	}
	
	// Create protocol based on provider type
	var protocol Protocol
	switch config.Type {
	case "openai", "anthropic", "local", "custom":
		// All use OpenAI-compatible protocol
		protocol = NewOpenAIProvider(config.Endpoint, config.APIKey)
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

// SendChatCompletion sends a chat completion request to a provider
func (r *Registry) SendChatCompletion(ctx context.Context, providerID string, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	provider, err := r.Get(providerID)
	if err != nil {
		return nil, err
	}
	
	// Use default model if not specified
	if req.Model == "" {
		req.Model = provider.Config.Model
	}
	
	return provider.Protocol.CreateChatCompletion(ctx, req)
}

// GetModels retrieves available models from a provider
func (r *Registry) GetModels(ctx context.Context, providerID string) ([]Model, error) {
	provider, err := r.Get(providerID)
	if err != nil {
		return nil, err
	}
	
	return provider.Protocol.GetModels(ctx)
}
