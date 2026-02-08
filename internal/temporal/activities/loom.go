package activities

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jordanhubbard/loom/internal/agent"
	"github.com/jordanhubbard/loom/internal/beads"
	"github.com/jordanhubbard/loom/internal/database"
	"github.com/jordanhubbard/loom/internal/dispatch"
	"github.com/jordanhubbard/loom/pkg/models"
)

const maxDispatchesPerBeat = 50

// LoomActivities supplies activities for the Ralph Loop heartbeat.
type LoomActivities struct {
	database   *database.Database
	dispatcher *dispatch.Dispatcher
	beadsMgr   *beads.Manager
	agentMgr   *agent.WorkerManager
}

func NewLoomActivities(db *database.Database, d *dispatch.Dispatcher, b *beads.Manager, a *agent.WorkerManager) *LoomActivities {
	return &LoomActivities{
		database:   db,
		dispatcher: d,
		beadsMgr:   b,
		agentMgr:   a,
	}
}

// LoomHeartbeatActivity is the Ralph Loop — the relentless work-draining engine.
// Each beat: resets stuck agents, resolves stuck beads, then drains all
// dispatchable work by calling DispatchOnce in a tight loop.
func (a *LoomActivities) LoomHeartbeatActivity(ctx context.Context, beatCount int) error {
	start := time.Now()

	// Phase 1: Reset agents stuck in "working" state for too long
	agentsReset := 0
	if a.agentMgr != nil {
		agentsReset = a.agentMgr.ResetStuckAgents(5 * time.Minute)
	}

	// Phase 2: Auto-block beads stuck in dispatch loops
	stuckResolved := a.resolveStuckBeads()

	// Phase 3: Drain all dispatchable work
	dispatched := 0
	if a.dispatcher != nil {
		for i := 0; i < maxDispatchesPerBeat; i++ {
			result, err := a.dispatcher.DispatchOnce(ctx, "")
			if err != nil {
				log.Printf("[Ralph] Beat %d: dispatch error on iteration %d: %v", beatCount, i+1, err)
				break
			}
			if result == nil || !result.Dispatched {
				break
			}
			dispatched++
		}
	}

	elapsed := time.Since(start)
	if dispatched > 0 || stuckResolved > 0 || agentsReset > 0 || beatCount%30 == 0 {
		log.Printf("[Ralph] Beat %d: dispatched=%d stuck_resolved=%d agents_reset=%d elapsed=%v",
			beatCount, dispatched, stuckResolved, agentsReset, elapsed.Round(time.Millisecond))
	}

	return nil
}

// resolveStuckBeads finds beads with loop_detected=true that haven't been
// resolved by Ralph yet, and auto-blocks them.
func (a *LoomActivities) resolveStuckBeads() int {
	if a.beadsMgr == nil {
		return 0
	}

	allBeads, err := a.beadsMgr.ListBeads(nil)
	if err != nil {
		return 0
	}

	resolved := 0
	for _, b := range allBeads {
		if b == nil || b.Context == nil {
			continue
		}
		if b.Context["loop_detected"] != "true" {
			continue
		}
		// Skip if already resolved by Ralph or escalated to CEO
		if b.Context["ralph_blocked_at"] != "" || b.Context["escalated_to_ceo_decision_id"] != "" {
			continue
		}
		// Skip already-closed or already-blocked beads
		if b.Status == models.BeadStatusClosed || b.Status == models.BeadStatusBlocked {
			continue
		}

		reason := b.Context["loop_detected_reason"]
		if reason == "" {
			reason = "loop detected"
		}

		updates := map[string]interface{}{
			"status":      models.BeadStatusBlocked,
			"assigned_to": "",
			"context": map[string]string{
				"ralph_blocked_at":     time.Now().UTC().Format(time.RFC3339),
				"ralph_blocked_reason": fmt.Sprintf("auto-blocked by Ralph: %s", reason),
				"redispatch_requested": "false",
			},
		}
		if err := a.beadsMgr.UpdateBead(b.ID, updates); err != nil {
			log.Printf("[Ralph] Failed to auto-block stuck bead %s: %v", b.ID, err)
			continue
		}
		log.Printf("[Ralph] Auto-blocked stuck bead %s: %s", b.ID, reason)
		resolved++
	}
	return resolved
}
