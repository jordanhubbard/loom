package patterns

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ProviderCostEstimate contains cost estimates for different providers/models
type ProviderCostEstimate struct {
	ProviderID string
	ModelName  string
	CostPer1KTokensUSD float64
	QualityScore       float64 // 0.0-1.0, higher is better
	Tier               string  // "budget", "mid-tier", "premium"
}

// GetProviderCostEstimates returns estimated costs for common providers
// In a production system, this would query a pricing database
func GetProviderCostEstimates() []ProviderCostEstimate {
	return []ProviderCostEstimate{
		// OpenAI Models
		{ProviderID: "openai", ModelName: "gpt-4", CostPer1KTokensUSD: 0.03, QualityScore: 0.95, Tier: "premium"},
		{ProviderID: "openai", ModelName: "gpt-4-turbo", CostPer1KTokensUSD: 0.01, QualityScore: 0.93, Tier: "premium"},
		{ProviderID: "openai", ModelName: "gpt-3.5-turbo", CostPer1KTokensUSD: 0.002, QualityScore: 0.80, Tier: "mid-tier"},

		// Anthropic Models
		{ProviderID: "anthropic", ModelName: "claude-opus-4-5", CostPer1KTokensUSD: 0.015, QualityScore: 0.96, Tier: "premium"},
		{ProviderID: "anthropic", ModelName: "claude-sonnet-4-5", CostPer1KTokensUSD: 0.003, QualityScore: 0.90, Tier: "premium"},
		{ProviderID: "anthropic", ModelName: "claude-haiku-3-5", CostPer1KTokensUSD: 0.0008, QualityScore: 0.75, Tier: "budget"},

		// Other providers (local/self-hosted typically cheaper)
		{ProviderID: "local", ModelName: "llama-3", CostPer1KTokensUSD: 0.0, QualityScore: 0.70, Tier: "budget"},
		{ProviderID: "ollama", ModelName: "mistral", CostPer1KTokensUSD: 0.0, QualityScore: 0.68, Tier: "budget"},
	}
}

// FindProviderCostEstimate looks up cost estimate for a provider/model
func FindProviderCostEstimate(providerID, modelName string) *ProviderCostEstimate {
	estimates := GetProviderCostEstimates()

	// Try exact match first
	for _, est := range estimates {
		if strings.EqualFold(est.ProviderID, providerID) && strings.EqualFold(est.ModelName, modelName) {
			return &est
		}
	}

	// Try fuzzy match on model name
	for _, est := range estimates {
		if strings.EqualFold(est.ProviderID, providerID) && strings.Contains(strings.ToLower(modelName), strings.ToLower(est.ModelName)) {
			return &est
		}
	}

	// Return default estimate
	return &ProviderCostEstimate{
		ProviderID:          providerID,
		ModelName:           modelName,
		CostPer1KTokensUSD:  0.005, // Default mid-tier pricing
		QualityScore:        0.75,
		Tier:                "mid-tier",
	}
}

// FindCheaperAlternatives finds alternative providers with better cost/quality ratio
func FindCheaperAlternatives(currentProvider, currentModel string, currentCostPer1K float64, minQualityScore float64) []SubstitutionRecommendation {
	estimates := GetProviderCostEstimates()
	var recommendations []SubstitutionRecommendation

	// Find current model's quality score
	currentEstimate := FindProviderCostEstimate(currentProvider, currentModel)
	targetQualityScore := minQualityScore
	if currentEstimate.QualityScore > minQualityScore {
		targetQualityScore = currentEstimate.QualityScore * 0.9 // Accept 10% quality degradation
	}

	for _, est := range estimates {
		// Skip same provider/model
		if strings.EqualFold(est.ProviderID, currentProvider) && strings.EqualFold(est.ModelName, currentModel) {
			continue
		}

		// Only recommend if cheaper AND meets quality threshold
		if est.CostPer1KTokensUSD < currentCostPer1K && est.QualityScore >= targetQualityScore {
			savingsPercent := (currentCostPer1K - est.CostPer1KTokensUSD) / currentCostPer1K
			qualityDelta := est.QualityScore - currentEstimate.QualityScore

			qualityImpact := "minimal"
			if qualityDelta < -0.10 {
				qualityImpact = "moderate"
			} else if qualityDelta < -0.05 {
				qualityImpact = "low"
			}

			recommendations = append(recommendations, SubstitutionRecommendation{
				FromProvider:     currentProvider,
				FromModel:        currentModel,
				ToProvider:       est.ProviderID,
				ToModel:          est.ModelName,
				CurrentCostPer1K: currentCostPer1K,
				NewCostPer1K:     est.CostPer1KTokensUSD,
				SavingsPercent:   savingsPercent,
				CurrentQuality:   currentEstimate.QualityScore,
				NewQuality:       est.QualityScore,
				QualityImpact:    qualityImpact,
			})
		}
	}

	return recommendations
}

// SubstitutionRecommendation represents a provider/model substitution suggestion
type SubstitutionRecommendation struct {
	FromProvider     string  `json:"from_provider"`
	FromModel        string  `json:"from_model"`
	ToProvider       string  `json:"to_provider"`
	ToModel          string  `json:"to_model"`
	CurrentCostPer1K float64 `json:"current_cost_per_1k"`
	NewCostPer1K     float64 `json:"new_cost_per_1k"`
	SavingsPercent   float64 `json:"savings_percent"`
	CurrentQuality   float64 `json:"current_quality"`
	NewQuality       float64 `json:"new_quality"`
	QualityImpact    string  `json:"quality_impact"`
}

// createSubstitutionOptimization creates provider/model substitution recommendations (improved version)
func (o *Optimizer) createEnhancedSubstitutionOptimization(pattern *UsagePattern) *Optimization {
	// Only analyze provider-model patterns
	if pattern.Type != "provider-model" {
		return nil
	}

	// Skip if already cheap
	if pattern.AvgCost < 0.001 {
		return nil
	}

	// Get current provider cost estimate
	currentEstimate := FindProviderCostEstimate(pattern.ProviderID, pattern.ModelName)
	currentCostPer1K := currentEstimate.CostPer1KTokensUSD

	// Find cheaper alternatives
	alternatives := FindCheaperAlternatives(pattern.ProviderID, pattern.ModelName, currentCostPer1K, 0.70)

	if len(alternatives) == 0 {
		return nil
	}

	// Use best alternative (highest savings)
	bestAlt := alternatives[0]
	for _, alt := range alternatives {
		if alt.SavingsPercent > bestAlt.SavingsPercent {
			bestAlt = alt
		}
	}

	// Calculate actual savings based on usage
	avgTokensPer1K := float64(pattern.AvgTokens) / 1000.0
	savingsPerRequest := (currentCostPer1K - bestAlt.NewCostPer1K) * avgTokensPer1K
	totalSavings := savingsPerRequest * float64(pattern.RequestCount)
	monthlySavings := totalSavings * 30 / 7 // Extrapolate weekly to monthly

	// Check if savings meet minimum threshold
	if totalSavings < 0.10 {
		return nil
	}

	recommendation := fmt.Sprintf(
		"Switch from %s/%s to %s/%s: %.0f%% cost reduction (from $%.4f to $%.4f per 1K tokens). Quality: %.1f%% → %.1f%%",
		pattern.ProviderID,
		pattern.ModelName,
		bestAlt.ToProvider,
		bestAlt.ToModel,
		bestAlt.SavingsPercent*100,
		currentCostPer1K,
		bestAlt.NewCostPer1K,
		currentEstimate.QualityScore*100,
		bestAlt.NewQuality*100,
	)

	return &Optimization{
		ID:                  uuid.New().String(),
		Type:                "provider-substitution",
		Pattern:             pattern,
		Recommendation:      recommendation,
		CurrentCost:         pattern.TotalCost,
		ProjectedCost:       pattern.TotalCost - totalSavings,
		ProjectedSavingsUSD: totalSavings,
		MonthlySavingsUSD:   monthlySavings,
		ImpactRating:        getImpactRating(monthlySavings),
		QualityImpact:       bestAlt.QualityImpact,
		AutoApplicable:      false, // Requires validation
		Confidence:          0.75,  // Higher confidence with real pricing data
	}
}
