package workflows

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type DispatcherWorkflowInput struct {
	ProjectID string        `json:"project_id"`
	Interval  time.Duration `json:"interval"`
}

// DispatcherWorkflow runs a periodic dispatch loop (Temporal-controlled clock).
func DispatcherWorkflow(ctx workflow.Context, input DispatcherWorkflowInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Dispatcher workflow started", "projectID", input.ProjectID, "interval", input.Interval)

	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	if input.Interval <= 0 {
		input.Interval = 10 * time.Second
	}

	triggerCh := workflow.GetSignalChannel(ctx, "dispatcher.trigger")
	iteration := 0

	for {
		_ = workflow.ExecuteActivity(ctx, "DispatchOnceActivity", input.ProjectID).Get(ctx, nil)
		iteration++
		if iteration%100 == 0 && workflow.GetInfo(ctx).GetCurrentHistoryLength() > 10000 {
			logger.Warn("Dispatcher history too large, continuing as new")
			return workflow.NewContinueAsNewError(ctx, DispatcherWorkflow, input)
		}

		timer := workflow.NewTimer(ctx, input.Interval)
		selector := workflow.NewSelector(ctx)
		selector.AddFuture(timer, func(workflow.Future) {})
		selector.AddReceive(triggerCh, func(c workflow.ReceiveChannel, more bool) {
			var payload map[string]interface{}
			c.Receive(ctx, &payload)
		})
		selector.Select(ctx)
	}
}
