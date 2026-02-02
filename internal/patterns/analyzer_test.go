package patterns

import (
	"context"
	"testing"
	"time"

	"github.com/jordanhubbard/agenticorp/internal/analytics"
)

// MockStorage is a mock implementation of analytics.Storage for testing
type MockStorage struct {
	logs  []*analytics.RequestLog
	stats *analytics.LogStats
}

func (m *MockStorage) SaveLog(ctx context.Context, log *analytics.RequestLog) error {
	m.logs = append(m.logs, log)
	return nil
}

func (m *MockStorage) GetLogs(ctx context.Context, filter *analytics.LogFilter) ([]*analytics.RequestLog, error) {
	return m.logs, nil
}

func (m *MockStorage) GetLogStats(ctx context.Context, filter *analytics.LogFilter) (*analytics.LogStats, error) {
	if m.stats != nil {
		return m.stats, nil
	}
	return &analytics.LogStats{
		TotalRequests: int64(len(m.logs)),
		TotalCostUSD:  0.0,
	}, nil
}

func (m *MockStorage) DeleteOldLogs(ctx context.Context, before time.Time) (int64, error) {
	return 0, nil
}

func TestAnalyzerBasic(t *testing.T) {
	storage := &MockStorage{
		logs: []*analytics.RequestLog{
			{
				ID:               "1",
				Timestamp:        time.Now(),
				UserID:           "user1",
				ProviderID:       "anthropic",
				ModelName:        "claude-sonnet-4-5",
				TotalTokens:      1000,
				LatencyMs:        500,
				CostUSD:          0.01,
				ErrorMessage:     "",
			},
			{
				ID:               "2",
				Timestamp:        time.Now(),
				UserID:           "user1",
				ProviderID:       "anthropic",
				ModelName:        "claude-sonnet-4-5",
				TotalTokens:      2000,
				LatencyMs:        600,
				CostUSD:          0.02,
				ErrorMessage:     "",
			},
			{
				ID:               "3",
				Timestamp:        time.Now(),
				UserID:           "user2",
				ProviderID:       "openai",
				ModelName:        "gpt-4",
				TotalTokens:      1500,
				LatencyMs:        800,
				CostUSD:          0.05,
				ErrorMessage:     "",
			},
		},
		stats: &analytics.LogStats{
			TotalRequests: 3,
			TotalCostUSD:  0.08,
		},
	}

	config := DefaultAnalysisConfig()
	config.MinRequests = 1
	config.MinCostUSD = 0.0

	analyzer := NewAnalyzer(storage, config)

	report, err := analyzer.AnalyzePatterns(context.Background(), config)
	if err != nil {
		t.Fatalf("AnalyzePatterns failed: %v", err)
	}

	if report == nil {
		t.Fatal("Expected non-nil report")
	}

	if report.TotalRequests != 3 {
		t.Errorf("Expected 3 total requests, got %d", report.TotalRequests)
	}

	if report.TotalCost != 0.08 {
		t.Errorf("Expected total cost 0.08, got %f", report.TotalCost)
	}

	if len(report.Patterns) == 0 {
		t.Error("Expected some patterns to be detected")
	}

	// Debug: print all patterns
	t.Logf("Found %d patterns:", len(report.Patterns))
	for _, p := range report.Patterns {
		t.Logf("  Pattern: type=%s, groupKey=%s, provider=%s, model=%s, count=%d",
			p.Type, p.GroupKey, p.ProviderID, p.ModelName, p.RequestCount)
	}

	// Verify provider-model clustering found our patterns
	foundAnthropicPattern := false
	foundOpenAIPattern := false
	for _, pattern := range report.Patterns {
		if pattern.Type == "provider-model" {
			if pattern.ProviderID == "anthropic" && pattern.ModelName == "claude-sonnet-4-5" {
				foundAnthropicPattern = true
				if pattern.RequestCount != 2 {
					t.Errorf("Expected Anthropic pattern to have 2 requests, got %d", pattern.RequestCount)
				}
			}
			if pattern.ProviderID == "openai" && pattern.ModelName == "gpt-4" {
				foundOpenAIPattern = true
				if pattern.RequestCount != 1 {
					t.Errorf("Expected OpenAI pattern to have 1 request, got %d", pattern.RequestCount)
				}
			}
		}
	}

	if !foundAnthropicPattern {
		t.Error("Expected to find Anthropic provider-model pattern")
	}
	if !foundOpenAIPattern {
		t.Error("Expected to find OpenAI provider-model pattern")
	}
}

func TestOptimizerRecommendations(t *testing.T) {
	config := DefaultAnalysisConfig()
	optimizer := NewOptimizer(config)

	patterns := []*UsagePattern{
		{
			ID:               "1",
			Type:             "provider-model",
			GroupKey:         "openai:gpt-4",
			ProviderID:       "openai",
			ModelName:        "gpt-4",
			RequestCount:     100,
			TotalCost:        50.0,
			AvgCost:          0.50,
			AvgTokens:        1000, // 1K tokens per request
			RequestFrequency: 100,
		},
		{
			ID:               "2",
			Type:             "provider-model",
			GroupKey:         "anthropic:claude-sonnet-4-5",
			ProviderID:       "anthropic",
			ModelName:        "claude-sonnet-4-5",
			RequestCount:     200,
			TotalCost:        10.0,
			AvgCost:          0.05,
			AvgTokens:        1000,
			RequestFrequency: 200,
		},
	}

	recommendations := optimizer.GenerateRecommendations(patterns)

	if len(recommendations) == 0 {
		t.Fatal("Expected some optimization recommendations")
	}

	// Check that expensive pattern got a recommendation
	foundExpensiveOptimization := false
	for _, opt := range recommendations {
		if opt.Pattern.GroupKey == "openai:gpt-4" {
			foundExpensiveOptimization = true
			if opt.Type != "provider-substitution" {
				t.Errorf("Expected provider-substitution optimization, got %s", opt.Type)
			}
			if opt.ProjectedSavingsUSD <= 0 {
				t.Error("Expected positive projected savings")
			}
		}
	}

	if !foundExpensiveOptimization {
		t.Error("Expected optimization recommendation for expensive GPT-4 pattern")
	}
}
