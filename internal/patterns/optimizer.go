package patterns

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// Optimizer generates optimization recommendations for usage patterns
type Optimizer struct {
	config *AnalysisConfig
}

// NewOptimizer creates a new optimizer
func NewOptimizer(config *AnalysisConfig) *Optimizer {
	if config == nil {
		config = DefaultAnalysisConfig()
	}
	return &Optimizer{
		config: config,
	}
}

// GenerateRecommendations creates optimization recommendations for patterns
func (o *Optimizer) GenerateRecommendations(patterns []*UsagePattern) []*Optimization {
	var optimizations []*Optimization

	for _, pattern := range patterns {
		// Rate limiting recommendations
		if o.config.EnableRateLimiting && pattern.RequestFrequency > o.config.RateLimitThreshold {
			opt := o.createRateLimitOptimization(pattern)
			if opt != nil {
				optimizations = append(optimizations, opt)
			}
		}

		// Provider substitution recommendations (enhanced version)
		if o.config.EnableSubstitutions && pattern.Type == "provider-model" {
			opt := o.createEnhancedSubstitutionOptimization(pattern)
			if opt != nil {
				optimizations = append(optimizations, opt)
			}
		}
	}

	// Sort by projected savings
	sort.Slice(optimizations, func(i, j int) bool {
		return optimizations[i].ProjectedSavingsUSD > optimizations[j].ProjectedSavingsUSD
	})

	return optimizations
}

// createRateLimitOptimization creates a rate limiting recommendation
func (o *Optimizer) createRateLimitOptimization(pattern *UsagePattern) *Optimization {
	// Calculate potential savings from reduced request volume
	excessRequests := pattern.RequestFrequency - o.config.RateLimitThreshold
	if excessRequests <= 0 {
		return nil
	}

	potentialSavings := excessRequests * pattern.AvgCost
	monthlySavings := potentialSavings * 30 // Extrapolate to monthly

	return &Optimization{
		ID:                  uuid.New().String(),
		Type:                "rate-limit",
		Pattern:             pattern,
		Recommendation:      fmt.Sprintf("Implement rate limiting for %s (%.0f req/day exceeds threshold of %.0f)", pattern.GroupKey, pattern.RequestFrequency, o.config.RateLimitThreshold),
		CurrentCost:         pattern.TotalCost,
		ProjectedCost:       pattern.TotalCost - potentialSavings,
		ProjectedSavingsUSD: potentialSavings,
		MonthlySavingsUSD:   monthlySavings,
		ImpactRating:        getImpactRating(monthlySavings),
		QualityImpact:       "minimal", // Rate limiting has minimal quality impact
		AutoApplicable:      false,     // Requires configuration
		Confidence:          0.8,
	}
}

// createSubstitutionOptimization creates a provider/model substitution recommendation
func (o *Optimizer) createSubstitutionOptimization(pattern *UsagePattern) *Optimization {
	// This is a placeholder - actual implementation would query provider catalog
	// and use routing system to find cheaper alternatives

	// For now, just identify expensive patterns as candidates for substitution
	if pattern.AvgCost < 0.01 {
		return nil // Already cheap, no need to optimize
	}

	// Estimate potential savings (placeholder logic)
	estimatedSavings := pattern.TotalCost * 0.3 // Assume 30% savings from substitution
	monthlySavings := estimatedSavings * 30 / 7 // Extrapolate to monthly

	return &Optimization{
		ID:                  uuid.New().String(),
		Type:                "provider-substitution",
		Pattern:             pattern,
		Recommendation:      fmt.Sprintf("Consider cheaper alternatives for %s (avg cost: $%.4f per request)", pattern.GroupKey, pattern.AvgCost),
		CurrentCost:         pattern.TotalCost,
		ProjectedCost:       pattern.TotalCost - estimatedSavings,
		ProjectedSavingsUSD: estimatedSavings,
		MonthlySavingsUSD:   monthlySavings,
		ImpactRating:        getImpactRating(monthlySavings),
		QualityImpact:       "moderate", // Substitution may affect quality
		AutoApplicable:      false,      // Requires validation
		Confidence:          0.6,        // Lower confidence without actual alternatives
	}
}

// Helper functions

func getImpactRating(monthlySavings float64) string {
	switch {
	case monthlySavings >= 1000:
		return "high"
	case monthlySavings >= 100:
		return "medium"
	default:
		return "low"
	}
}
