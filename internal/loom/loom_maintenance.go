package loom

import (
	"log"
	"strings"

	"github.com/jordanhubbard/loom/pkg/models"
)

func (a *Loom) resetZombieBeads() int {
	if a.beadsManager == nil {
		return 0
	}

	inProgressBeads, err := a.beadsManager.ListBeads(map[string]interface{}{
		"status": models.BeadStatusInProgress,
	})
	if err != nil {
		log.Printf("[Loom] resetZombieBeads: could not list in-progress beads: %v", err)
		return 0
	}

	// Build a set of known live agent IDs so we can detect beads assigned to
	// named agents that no longer exist or are permanently idle.
	liveAgentIDs := make(map[string]bool)
	if a.agentManager != nil {
		for _, ag := range a.agentManager.ListAgents() {
			if ag != nil {
				liveAgentIDs[ag.ID] = true
			}
		}
	}

	count := 0
	for _, b := range inProgressBeads {
		if b == nil || b.AssignedTo == "" {
			continue
		}
		isZombie := false
		if strings.HasPrefix(b.AssignedTo, "exec-") {
			// Ephemeral goroutine ID — never survives restart
			isZombie = true
		} else if !liveAgentIDs[b.AssignedTo] {
			// Named agent ID that is not registered in the current run
			isZombie = true
		}
		if !isZombie {
			continue
		}
		if err := a.beadsManager.UpdateBead(b.ID, map[string]interface{}{
			"status":      models.BeadStatusOpen,
			"assigned_to": "",
		}); err != nil {
			log.Printf("[Loom] resetZombieBeads: could not reset bead %s: %v", b.ID, err)
			continue
		}
		log.Printf("[Loom] Recovered zombie bead %s [%s] (was held by stale assignee %s)",
			b.ID, b.Title, b.AssignedTo)
		count++
	}
	return count
}
