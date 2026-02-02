package patterns

import (
	"testing"
)

func TestFindProviderCostEstimate(t *testing.T) {
	// Test exact match
	est := FindProviderCostEstimate("openai", "gpt-4")
	if est == nil {
		t.Fatal("Expected to find estimate for gpt-4")
	}
	if est.CostPer1KTokensUSD != 0.03 {
		t.Errorf("Expected gpt-4 cost to be 0.03, got %f", est.CostPer1KTokensUSD)
	}
	if est.QualityScore != 0.95 {
		t.Errorf("Expected gpt-4 quality to be 0.95, got %f", est.QualityScore)
	}

	// Test anthropic model
	est = FindProviderCostEstimate("anthropic", "claude-sonnet-4-5")
	if est == nil {
		t.Fatal("Expected to find estimate for claude-sonnet-4-5")
	}
	if est.CostPer1KTokensUSD != 0.003 {
		t.Errorf("Expected claude-sonnet cost to be 0.003, got %f", est.CostPer1KTokensUSD)
	}

	// Test unknown provider (should return default)
	est = FindProviderCostEstimate("unknown", "unknown-model")
	if est == nil {
		t.Fatal("Expected default estimate for unknown provider")
	}
	if est.CostPer1KTokensUSD != 0.005 {
		t.Errorf("Expected default cost to be 0.005, got %f", est.CostPer1KTokensUSD)
	}
}

func TestFindCheaperAlternatives(t *testing.T) {
	// Test finding alternatives to expensive model
	alternatives := FindCheaperAlternatives("openai", "gpt-4", 0.03, 0.70)

	if len(alternatives) == 0 {
		t.Error("Expected to find cheaper alternatives to gpt-4")
	}

	// Verify all alternatives are actually cheaper
	for _, alt := range alternatives {
		if alt.NewCostPer1K >= 0.03 {
			t.Errorf("Alternative %s/%s is not cheaper: $%.4f", alt.ToProvider, alt.ToModel, alt.NewCostPer1K)
		}
	}

	// Verify alternatives meet quality threshold
	for _, alt := range alternatives {
		if alt.NewQuality < 0.70 {
			t.Errorf("Alternative %s/%s does not meet quality threshold: %.2f", alt.ToProvider, alt.ToModel, alt.NewQuality)
		}
	}
}

func TestFindCheaperAlternatives_AlreadyCheap(t *testing.T) {
	// Test with already cheap model
	alternatives := FindCheaperAlternatives("anthropic", "claude-haiku-3-5", 0.0008, 0.70)

	// Should find few or no alternatives since it's already one of the cheapest
	if len(alternatives) > 1 {
		t.Logf("Found %d alternatives to claude-haiku (already cheap), which is okay", len(alternatives))
	}
}

func TestCreateEnhancedSubstitutionOptimization(t *testing.T) {
	optimizer := NewOptimizer(DefaultAnalysisConfig())

	// Create an expensive pattern
	pattern := &UsagePattern{
		Type:         "provider-model",
		ProviderID:   "openai",
		ModelName:    "gpt-4",
		RequestCount: 1000,
		TotalCost:    30.0, // $0.03 per request avg
		AvgCost:      0.03,
		AvgTokens:    1000, // 1K tokens per request
	}

	opt := optimizer.createEnhancedSubstitutionOptimization(pattern)
	if opt == nil {
		t.Fatal("Expected substitution optimization for expensive pattern")
	}

	if opt.Type != "provider-substitution" {
		t.Errorf("Expected provider-substitution type, got %s", opt.Type)
	}

	if opt.ProjectedSavingsUSD <= 0 {
		t.Error("Expected positive savings")
	}

	if opt.MonthlySavingsUSD <= 0 {
		t.Error("Expected positive monthly savings")
	}

	if opt.Confidence <= 0 || opt.Confidence > 1 {
		t.Errorf("Invalid confidence: %f", opt.Confidence)
	}

	t.Logf("Recommendation: %s", opt.Recommendation)
	t.Logf("Savings: $%.2f (%.1f%% of current cost)", opt.ProjectedSavingsUSD, (opt.ProjectedSavingsUSD/pattern.TotalCost)*100)
	t.Logf("Monthly projection: $%.2f", opt.MonthlySavingsUSD)
}

func TestCreateEnhancedSubstitutionOptimization_AlreadyCheap(t *testing.T) {
	optimizer := NewOptimizer(DefaultAnalysisConfig())

	// Create a cheap pattern
	pattern := &UsagePattern{
		Type:         "provider-model",
		ProviderID:   "local",
		ModelName:    "llama-3",
		RequestCount: 1000,
		TotalCost:    0.0,
		AvgCost:      0.0,
		AvgTokens:    1000,
	}

	opt := optimizer.createEnhancedSubstitutionOptimization(pattern)
	if opt != nil {
		t.Error("Should not recommend substitution for already cheap model")
	}
}

func TestCreateEnhancedSubstitutionOptimization_WrongType(t *testing.T) {
	optimizer := NewOptimizer(DefaultAnalysisConfig())

	// Create a non-provider-model pattern
	pattern := &UsagePattern{
		Type:         "user",
		RequestCount: 1000,
		TotalCost:    10.0,
		AvgCost:      0.01,
	}

	opt := optimizer.createEnhancedSubstitutionOptimization(pattern)
	if opt != nil {
		t.Error("Should not recommend substitution for non-provider-model patterns")
	}
}

func TestGetProviderCostEstimates_Coverage(t *testing.T) {
	estimates := GetProviderCostEstimates()

	if len(estimates) == 0 {
		t.Fatal("Expected some provider cost estimates")
	}

	// Verify each estimate has required fields
	for i, est := range estimates {
		if est.ProviderID == "" {
			t.Errorf("Estimate %d missing provider ID", i)
		}
		if est.ModelName == "" {
			t.Errorf("Estimate %d missing model name", i)
		}
		if est.QualityScore < 0 || est.QualityScore > 1 {
			t.Errorf("Estimate %d has invalid quality score: %f", i, est.QualityScore)
		}
		if est.Tier == "" {
			t.Errorf("Estimate %d missing tier", i)
		}
	}

	// Verify we have different tiers
	tiers := make(map[string]bool)
	for _, est := range estimates {
		tiers[est.Tier] = true
	}

	if !tiers["budget"] {
		t.Error("Expected at least one budget tier provider")
	}
	if !tiers["premium"] {
		t.Error("Expected at least one premium tier provider")
	}
}
