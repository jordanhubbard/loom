package motivation

import (
	"context"
	"log"
)

// MotivationActivityInput contains input for the motivation evaluation activity
type MotivationActivityInput struct {
	BeatCount int // Current heartbeat count (for logging)
}

// MotivationActivityResult contains the result of motivation evaluation
type MotivationActivityResult struct {
	MotivationsEvaluated int      `json:"motivations_evaluated"`
	MotivationsFired     int      `json:"motivations_fired"`
	FiredMotivationIDs   []string `json:"fired_motivation_ids,omitempty"`
	Errors               []string `json:"errors,omitempty"`
}

// MotivationActivity is a Temporal activity that evaluates motivations
// This should be called by the AgentiCorpHeartbeatWorkflow
type MotivationActivity struct {
	engine *Engine
}

// NewMotivationActivity creates a new motivation activity
func NewMotivationActivity(engine *Engine) *MotivationActivity {
	return &MotivationActivity{engine: engine}
}

// EvaluateMotivations runs one tick of the motivation engine
func (a *MotivationActivity) EvaluateMotivations(ctx context.Context, input MotivationActivityInput) (*MotivationActivityResult, error) {
	result := &MotivationActivityResult{
		FiredMotivationIDs: make([]string, 0),
		Errors:             make([]string, 0),
	}

	if a.engine == nil {
		return result, nil
	}

	// Get count of active motivations
	activeMotivations := a.engine.GetRegistry().GetActive()
	result.MotivationsEvaluated = len(activeMotivations)

	// Run the tick
	triggered, err := a.engine.Tick(ctx)
	if err != nil {
		result.Errors = append(result.Errors, err.Error())
		log.Printf("Motivation evaluation error on beat %d: %v", input.BeatCount, err)
	}

	result.MotivationsFired = triggered

	// Get recently triggered motivation IDs
	history := a.engine.GetRegistry().GetTriggerHistory(triggered)
	for _, trigger := range history {
		result.FiredMotivationIDs = append(result.FiredMotivationIDs, trigger.MotivationID)
	}

	if triggered > 0 {
		log.Printf("Motivation activity: beat=%d, evaluated=%d, fired=%d",
			input.BeatCount, result.MotivationsEvaluated, triggered)
	}

	return result, nil
}

// GetActivityName returns the name used to register this activity
func (a *MotivationActivity) GetActivityName() string {
	return "EvaluateMotivationsActivity"
}

// MotivationWorkflowInput contains input for the motivation workflow
type MotivationWorkflowInput struct {
	// Configuration overrides (optional)
	EvaluationIntervalSeconds int `json:"evaluation_interval_seconds,omitempty"`
}

// Integration point: Register the activity with Temporal worker
// This should be called during AgentiCorp initialization:
//
// Example usage in agenticorp.go or temporal/manager.go:
//
//   motivationEngine := motivation.NewEngine(registry, stateProvider, actionHandler)
//   motivationActivity := motivation.NewMotivationActivity(motivationEngine)
//   worker.RegisterActivity(motivationActivity.EvaluateMotivations)
//
// Then in AgentiCorpHeartbeatWorkflow, call the activity:
//
//   var result motivation.MotivationActivityResult
//   err := workflow.ExecuteActivity(ctx, "EvaluateMotivationsActivity",
//       motivation.MotivationActivityInput{BeatCount: beatCount}).Get(ctx, &result)
