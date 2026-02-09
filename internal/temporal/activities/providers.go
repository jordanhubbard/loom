package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/internal/database"
	"github.com/jordanhubbard/loom/internal/modelcatalog"
	internalmodels "github.com/jordanhubbard/loom/internal/models"
	"github.com/jordanhubbard/loom/internal/provider"
	"github.com/jordanhubbard/loom/internal/temporal/eventbus"
)

// ProviderHeartbeatInput represents heartbeat activity input.
type ProviderHeartbeatInput struct {
	ProviderID string
}

// ProviderHeartbeatResult captures heartbeat measurements.
type ProviderHeartbeatResult struct {
	ProviderID string    `json:"provider_id"`
	Status     string    `json:"status"`
	LatencyMs  int64     `json:"latency_ms"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

// ProviderQueryInput represents a direct provider query.
type ProviderQueryInput struct {
	ProviderID   string  `json:"provider_id"`
	SystemPrompt string  `json:"system_prompt"`
	Message      string  `json:"message"`
	Temperature  float64 `json:"temperature"`
	MaxTokens    int     `json:"max_tokens"`
}

// ProviderQueryResult represents the response from a provider query.
type ProviderQueryResult struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
	Response   string `json:"response"`
	TokensUsed int    `json:"tokens_used"`
	LatencyMs  int64  `json:"latency_ms"`
}

// ProviderActivities supplies heartbeat and query activities.
type ProviderActivities struct {
	registry *provider.Registry
	database *database.Database
	eventBus *eventbus.EventBus
	catalog  *modelcatalog.Catalog
}

func NewProviderActivities(registry *provider.Registry, db *database.Database, eb *eventbus.EventBus, catalog *modelcatalog.Catalog) *ProviderActivities {
	return &ProviderActivities{registry: registry, database: db, eventBus: eb, catalog: catalog}
}

// ProviderHeartbeatActivity checks provider responsiveness and updates status.
func (a *ProviderActivities) ProviderHeartbeatActivity(ctx context.Context, input ProviderHeartbeatInput) (*ProviderHeartbeatResult, error) {
	if input.ProviderID == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	start := time.Now()
	result := &ProviderHeartbeatResult{ProviderID: input.ProviderID, CheckedAt: time.Now().UTC()}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Always attempt to discover a working endpoint/protocol and load models.
	record, models, discoveredType, discoveredEndpoint, err := a.discoverAndListModels(ctx, input.ProviderID)
	result.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		a.persistHeartbeat(result)
		return result, nil
	}

	available := make([]string, 0, len(models))
	for _, m := range models {
		if m.ID != "" {
			available = append(available, m.ID)
		}
	}
	if len(available) == 0 {
		result.Status = "failed"
		result.Error = "no models discovered"
		a.persistHeartbeat(result)
		return result, nil
	}
	selected, reason, score := a.selectModel(record, available)
	if selected != "" {
		record.SelectedModel = selected
		record.Model = selected
		record.SelectionReason = reason
		record.ModelScore = score
	}
	if discoveredType != "" {
		record.Type = discoveredType
	}
	if discoveredEndpoint != "" {
		record.Endpoint = discoveredEndpoint
	}
	record.Status = "healthy"
	record.LastHeartbeatError = ""
	_ = a.database.UpsertProvider(record)
	a.syncRegistry(record)

	result.Status = record.Status
	result.Error = ""
	a.persistHeartbeat(result)
	return result, nil
}

func (a *ProviderActivities) ProviderQueryActivity(ctx context.Context, input ProviderQueryInput) (*ProviderQueryResult, error) {
	if input.ProviderID == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	regProvider, err := a.registry.Get(input.ProviderID)
	if err != nil {
		return nil, err
	}
	if regProvider.Config == nil {
		return nil, fmt.Errorf("provider %s is missing config", input.ProviderID)
	}
	if !providerIsHealthy(regProvider.Config.Status) {
		return nil, fmt.Errorf("provider %s is disabled", input.ProviderID)
	}
	if regProvider.Config.Model == "" {
		return nil, fmt.Errorf("provider %s has no model configured", input.ProviderID)
	}

	req := &provider.ChatCompletionRequest{
		Model: regProvider.Config.Model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: input.SystemPrompt},
			{Role: "user", Content: input.Message},
		},
		Temperature: input.Temperature,
		MaxTokens:   input.MaxTokens,
	}

	start := time.Now()
	resp, err := regProvider.Protocol.CreateChatCompletion(ctx, req)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		return nil, err
	}

	responseText := ""
	if len(resp.Choices) > 0 {
		responseText = resp.Choices[0].Message.Content
	}

	result := &ProviderQueryResult{
		ProviderID: input.ProviderID,
		Model:      resp.Model,
		Response:   responseText,
		TokensUsed: resp.Usage.TotalTokens,
		LatencyMs:  latencyMs,
	}
	return result, nil
}

func (a *ProviderActivities) persistHeartbeat(result *ProviderHeartbeatResult) {
	if result == nil || a.database == nil {
		return
	}

	record, err := a.database.GetProvider(result.ProviderID)
	if err != nil {
		return
	}
	record.Status = result.Status
	record.LastHeartbeatAt = result.CheckedAt
	record.LastHeartbeatLatencyMs = result.LatencyMs
	record.LastHeartbeatError = result.Error
	_ = a.database.UpsertProvider(record)

	a.syncRegistry(record)

	a.publishProviderUpdate(record)
}

func (a *ProviderActivities) syncRegistry(record *internalmodels.Provider) {
	if record == nil || a.registry == nil {
		return
	}
	selected := record.SelectedModel
	if selected == "" {
		selected = record.Model
	}
	if selected == "" {
		selected = record.ConfiguredModel
	}
	_ = a.registry.Upsert(&provider.ProviderConfig{
		ID:                     record.ID,
		Name:                   record.Name,
		Type:                   record.Type,
		Endpoint:               record.Endpoint,
		Model:                  selected,
		ConfiguredModel:        record.ConfiguredModel,
		SelectedModel:          selected,
		SelectedGPU:            record.SelectedGPU,
		Status:                 record.Status,
		LastHeartbeatAt:        record.LastHeartbeatAt,
		LastHeartbeatLatencyMs: record.LastHeartbeatLatencyMs,
		CapabilityScore:        record.Metrics.OverallScore,
		ContextWindow:          record.ContextWindow,
	})
}

func (a *ProviderActivities) publishProviderUpdate(record *internalmodels.Provider) {
	if record == nil || a.eventBus == nil {
		return
	}
	_ = a.eventBus.Publish(&eventbus.Event{
		Type:   eventbus.EventTypeProviderUpdated,
		Source: "provider-heartbeat",
		Data: map[string]interface{}{
			"provider_id": record.ID,
			"status":      record.Status,
			"latency_ms":  record.LastHeartbeatLatencyMs,
			"error":       record.LastHeartbeatError,
			"model":       record.SelectedModel,
			"configured":  record.ConfiguredModel,
			"score":       record.ModelScore,
		},
	})
}

func providerIsHealthy(status string) bool {
	switch status {
	case "healthy", "active":
		return true
	default:
		return false
	}
}

func (a *ProviderActivities) selectModel(record *internalmodels.Provider, available []string) (selected string, reason string, score float64) {
	if record == nil {
		return "", "", 0
	}
	configured := strings.TrimSpace(record.ConfiguredModel)
	if configured == "" {
		configured = strings.TrimSpace(record.Model)
	}
	if configured == "" {
		configured = "nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B-FP8"
	}
	record.ConfiguredModel = configured

	for _, name := range available {
		if strings.EqualFold(name, configured) {
			return configured, "configured model available", 0
		}
	}

	if a.catalog != nil {
		if best, bestScore, ok := a.catalog.SelectBest(available); ok {
			return best.Name, "matched recommended catalog", bestScore
		}
	}

	if len(available) > 0 {
		return available[0], "fallback to first discovered model", 0
	}
	return configured, "fallback to configured model", 0
}

func (a *ProviderActivities) discoverAndListModels(ctx context.Context, providerID string) (*internalmodels.Provider, []provider.Model, string, string, error) {
	if a.database == nil {
		return nil, nil, "", "", fmt.Errorf("database not configured")
	}
	record, err := a.database.GetProvider(providerID)
	if err != nil {
		return nil, nil, "", "", err
	}
	if strings.TrimSpace(record.Endpoint) == "" {
		return record, nil, "", "", fmt.Errorf("provider %s has no endpoint", providerID)
	}

	candidates, err := buildProviderCandidates(record.Endpoint, record.Type)
	if err != nil {
		return record, nil, "", "", err
	}

	// Try the registered Protocol first (has auth credentials baked in)
	if a.registry != nil {
		if reg, regErr := a.registry.Get(providerID); regErr == nil && reg.Protocol != nil {
			models, getErr := reg.Protocol.GetModels(ctx)
			if getErr == nil && len(models) > 0 {
				return record, models, record.Type, record.Endpoint, nil
			}
		}
	}

	var lastErr error
	for _, c := range candidates {
		models, probeErr := probeModels(ctx, c)
		if probeErr != nil {
			lastErr = probeErr
			continue
		}
		// Sync record + registry to discovered type/endpoint before returning.
		if c.ProviderType != "" {
			record.Type = c.ProviderType
		}
		record.Endpoint = c.Endpoint
		_ = a.database.UpsertProvider(record)
		a.syncRegistry(record)
		return record, models, c.ProviderType, c.Endpoint, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no candidate endpoints succeeded")
	}
	return record, nil, "", "", lastErr
}

type providerCandidate struct {
	ProviderType string
	Endpoint     string
	APIKey       string // For authenticated endpoints (cloud providers)
}

func buildProviderCandidates(raw string, preferredOpenAIType string) ([]providerCandidate, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse endpoint: %w", err)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return nil, fmt.Errorf("invalid endpoint host")
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "http"
	}
	openAIType := "openai"
	switch preferredOpenAIType {
	case "local", "custom", "anthropic", "openai":
		openAIType = preferredOpenAIType
	}

	var candidates []providerCandidate

	// If the URL has a path (e.g. /v1), use it as-is first — the user told us
	// exactly where the API is. Don't probe random ports on cloud endpoints.
	path := strings.TrimSuffix(u.Path, "/")
	if path != "" {
		full := fmt.Sprintf("%s://%s%s", scheme, u.Host, path)
		candidates = append(candidates, providerCandidate{ProviderType: openAIType, Endpoint: full})
	}

	// For explicit ports or bare hostnames, also try OpenAI + Ollama combos.
	port := u.Port()
	var ports []string
	if port != "" {
		ports = []string{port}
	} else if path == "" {
		// Only scan default ports when no path was provided (bare hostname).
		ports = []string{"8000", "11434"}
	}

	for _, p := range ports {
		base := fmt.Sprintf("%s://%s:%s", scheme, hostname, p)
		candidates = append(candidates,
			providerCandidate{ProviderType: openAIType, Endpoint: base + "/v1"},
			providerCandidate{ProviderType: "ollama", Endpoint: base},
		)
	}

	return uniqueCandidates(candidates), nil
}

func uniqueCandidates(in []providerCandidate) []providerCandidate {
	seen := map[string]struct{}{}
	out := make([]providerCandidate, 0, len(in))
	for _, c := range in {
		key := c.ProviderType + "|" + c.Endpoint
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, c)
	}
	return out
}

func probeModels(ctx context.Context, c providerCandidate) ([]provider.Model, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	switch c.ProviderType {
	case "openai", "local", "custom", "anthropic":
		url := strings.TrimSuffix(c.Endpoint, "/") + "/models"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("openai /models unexpected status %d: %s", resp.StatusCode, string(b))
		}
		var modelsResp struct {
			Data []provider.Model `json:"data"`
		}
		if err := json.Unmarshal(b, &modelsResp); err != nil {
			return nil, err
		}
		return modelsResp.Data, nil
	case "ollama":
		url := strings.TrimSuffix(c.Endpoint, "/") + "/api/tags"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("ollama /api/tags unexpected status %d: %s", resp.StatusCode, string(b))
		}
		var tagsResp struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.Unmarshal(b, &tagsResp); err != nil {
			return nil, err
		}
		models := make([]provider.Model, 0, len(tagsResp.Models))
		for _, m := range tagsResp.Models {
			name := strings.TrimSpace(m.Name)
			if name == "" {
				continue
			}
			models = append(models, provider.Model{ID: name, Object: "model"})
		}
		return models, nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", c.ProviderType)
	}
}
