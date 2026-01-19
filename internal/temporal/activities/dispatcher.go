package activities

import (
	"context"

	"github.com/jordanhubbard/arbiter/internal/dispatch"
)

// DispatchActivities provides activities for the Temporal-controlled dispatch loop.
type DispatchActivities struct {
	Dispatcher *dispatch.Dispatcher
}

func NewDispatchActivities(d *dispatch.Dispatcher) *DispatchActivities {
	return &DispatchActivities{Dispatcher: d}
}

func (a *DispatchActivities) DispatchOnceActivity(ctx context.Context, projectID string) (*dispatch.DispatchResult, error) {
	return a.Dispatcher.DispatchOnce(ctx, projectID)
}
