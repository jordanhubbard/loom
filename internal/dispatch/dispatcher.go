package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jordanhubbard/arbiter/internal/agent"
	"github.com/jordanhubbard/arbiter/internal/beads"
	"github.com/jordanhubbard/arbiter/internal/project"
	"github.com/jordanhubbard/arbiter/internal/provider"
	"github.com/jordanhubbard/arbiter/internal/temporal/eventbus"
	"github.com/jordanhubbard/arbiter/internal/worker"
	"github.com/jordanhubbard/arbiter/pkg/models"
)

type StatusState string

const (
	StatusActive StatusState = "active"
	StatusParked StatusState = "parked"
)

type SystemStatus struct {
	State     StatusState `json:"state"`
	Reason    string      `json:"reason"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type DispatchResult struct {
	Dispatched bool   `json:"dispatched"`
	ProjectID  string `json:"project_id,omitempty"`
	BeadID     string `json:"bead_id,omitempty"`
	AgentID    string `json:"agent_id,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Dispatcher is responsible for selecting ready work and executing it using agents/providers.
// For now it focuses on turning beads into LLM tasks and storing the output back into bead context.
type Dispatcher struct {
	beads     *beads.Manager
	projects  *project.Manager
	agents    *agent.WorkerManager
	providers *provider.Registry
	eventBus  *eventbus.EventBus

	mu     sync.RWMutex
	status SystemStatus
}

func NewDispatcher(beadsMgr *beads.Manager, projMgr *project.Manager, agentMgr *agent.WorkerManager, registry *provider.Registry, eb *eventbus.EventBus) *Dispatcher {
	d := &Dispatcher{
		beads:     beadsMgr,
		projects:  projMgr,
		agents:    agentMgr,
		providers: registry,
		eventBus:  eb,
		status: SystemStatus{
			State:     StatusParked,
			Reason:    "not started",
			UpdatedAt: time.Now(),
		},
	}
	return d
}

func (d *Dispatcher) GetSystemStatus() SystemStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.status
}

// DispatchOnce finds at most one ready bead and asks an idle agent to work on it.
func (d *Dispatcher) DispatchOnce(ctx context.Context, projectID string) (*DispatchResult, error) {
	if len(d.providers.List()) == 0 {
		d.setStatus(StatusParked, "no providers registered")
		return &DispatchResult{Dispatched: false, ProjectID: projectID}, nil
	}

	ready, err := d.beads.GetReadyBeads(projectID)
	if err != nil {
		d.setStatus(StatusParked, "failed to list ready beads")
		return nil, err
	}

	sort.SliceStable(ready, func(i, j int) bool {
		if ready[i] == nil {
			return false
		}
		if ready[j] == nil {
			return true
		}
		if ready[i].Priority != ready[j].Priority {
			return ready[i].Priority < ready[j].Priority
		}
		return ready[i].UpdatedAt.After(ready[j].UpdatedAt)
	})

	// Only auto-dispatch non-P0 task/epic beads.
	idleAgents := d.agents.GetIdleAgentsByProject(projectID)
	idleByID := make(map[string]*models.Agent, len(idleAgents))
	for _, a := range idleAgents {
		if a != nil {
			idleByID[a.ID] = a
		}
	}

	var candidate *models.Bead
	var ag *models.Agent
	for _, b := range ready {
		if b == nil {
			continue
		}
		if b.Priority == models.BeadPriorityP0 {
			continue
		}
		if b.Type == "decision" {
			continue
		}
		if b.Context != nil {
			if b.Context["redispatch_requested"] != "true" && b.Context["last_run_at"] != "" {
				continue
			}
		}

		// If bead is assigned to an agent, only dispatch to that agent.
		if b.AssignedTo != "" {
			assigned, ok := idleByID[b.AssignedTo]
			if !ok {
				continue
			}
			ag = assigned
			candidate = b
			break
		}

		// Otherwise, pick any idle agent.
		if len(idleAgents) == 0 {
			continue
		}
		ag = idleAgents[0]
		candidate = b
		break
	}

	if candidate == nil {
		d.setStatus(StatusParked, "no dispatchable beads")
		return &DispatchResult{Dispatched: false, ProjectID: projectID}, nil
	}

	selectedProjectID := projectID
	if selectedProjectID == "" {
		selectedProjectID = candidate.ProjectID
	}
	if ag == nil {
		d.setStatus(StatusParked, "no idle agents")
		return &DispatchResult{Dispatched: false, ProjectID: selectedProjectID}, nil
	}
	if ag.ProviderID == "" {
		d.setStatus(StatusParked, "agent has no provider")
		return &DispatchResult{Dispatched: false, ProjectID: selectedProjectID, AgentID: ag.ID}, nil
	}

	// Ensure bead is claimed/assigned.
	if candidate.AssignedTo == "" {
		if err := d.beads.ClaimBead(candidate.ID, ag.ID); err != nil {
			d.setStatus(StatusParked, "failed to claim bead")
			return &DispatchResult{Dispatched: false, ProjectID: projectID}, nil
		}
	}
	_ = d.agents.AssignBead(ag.ID, candidate.ID)
	if d.eventBus != nil {
		_ = d.eventBus.PublishBeadEvent(eventbus.EventTypeBeadAssigned, candidate.ID, selectedProjectID, map[string]interface{}{"assigned_to": ag.ID})
		_ = d.eventBus.PublishBeadEvent(eventbus.EventTypeBeadStatusChange, candidate.ID, selectedProjectID, map[string]interface{}{"status": string(models.BeadStatusInProgress)})
	}

	proj, _ := d.projects.GetProject(selectedProjectID)

	task := &worker.Task{
		ID:          fmt.Sprintf("task-%s-%d", candidate.ID, time.Now().UnixNano()),
		Description: buildBeadDescription(candidate),
		Context:     buildBeadContext(candidate, proj),
		BeadID:      candidate.ID,
		ProjectID:   selectedProjectID,
	}

	d.setStatus(StatusActive, fmt.Sprintf("dispatching %s", candidate.ID))

	result, execErr := d.agents.ExecuteTask(ctx, ag.ID, task)
	if execErr != nil {
		d.setStatus(StatusParked, "execution failed")

		historyJSON, loopDetected, loopReason := buildDispatchHistory(candidate, ag.ID)
		ctxUpdates := map[string]string{
			"last_run_at":          time.Now().UTC().Format(time.RFC3339),
			"last_run_error":       execErr.Error(),
			"agent_id":             ag.ID,
			"provider_id":          ag.ProviderID,
			"redispatch_requested": "false",
			"dispatch_history":     historyJSON,
			"loop_detected":        fmt.Sprintf("%t", loopDetected),
		}
		if loopDetected {
			ctxUpdates["loop_detected_reason"] = loopReason
			ctxUpdates["loop_detected_at"] = time.Now().UTC().Format(time.RFC3339)
		}
		updates := map[string]interface{}{"context": ctxUpdates}
		if loopDetected {
			updates["priority"] = models.BeadPriorityP0
			updates["status"] = models.BeadStatusOpen
			updates["assigned_to"] = ""
		}
		_ = d.beads.UpdateBead(candidate.ID, updates)
		if d.eventBus != nil {
			status := string(models.BeadStatusInProgress)
			if loopDetected {
				status = string(models.BeadStatusOpen)
			}
			_ = d.eventBus.PublishBeadEvent(eventbus.EventTypeBeadStatusChange, candidate.ID, selectedProjectID, map[string]interface{}{"status": status})
		}
		return &DispatchResult{Dispatched: true, ProjectID: selectedProjectID, BeadID: candidate.ID, AgentID: ag.ID, ProviderID: ag.ProviderID, Error: execErr.Error()}, nil
	}

	ctxUpdates := map[string]string{
		"last_run_at":          time.Now().UTC().Format(time.RFC3339),
		"agent_id":             ag.ID,
		"provider_id":          ag.ProviderID,
		"provider_model":       d.providersModel(ag.ProviderID),
		"agent_output":         result.Response,
		"agent_tokens":         fmt.Sprintf("%d", result.TokensUsed),
		"agent_task_id":        result.TaskID,
		"agent_worker_id":      result.WorkerID,
		"redispatch_requested": "false",
	}
	historyJSON, loopDetected, loopReason := buildDispatchHistory(candidate, ag.ID)
	ctxUpdates["dispatch_history"] = historyJSON
	ctxUpdates["loop_detected"] = fmt.Sprintf("%t", loopDetected)
	if loopDetected {
		ctxUpdates["loop_detected_reason"] = loopReason
		ctxUpdates["loop_detected_at"] = time.Now().UTC().Format(time.RFC3339)
	}

	updates := map[string]interface{}{"context": ctxUpdates}
	if loopDetected {
		updates["priority"] = models.BeadPriorityP0
		updates["status"] = models.BeadStatusOpen
		updates["assigned_to"] = ""
	}
	_ = d.beads.UpdateBead(candidate.ID, updates)
	if d.eventBus != nil {
		status := string(models.BeadStatusInProgress)
		if loopDetected {
			status = string(models.BeadStatusOpen)
		}
		_ = d.eventBus.PublishBeadEvent(eventbus.EventTypeBeadStatusChange, candidate.ID, selectedProjectID, map[string]interface{}{"status": status})
	}

	d.setStatus(StatusParked, "idle")
	return &DispatchResult{Dispatched: true, ProjectID: selectedProjectID, BeadID: candidate.ID, AgentID: ag.ID, ProviderID: ag.ProviderID}, nil
}

func buildDispatchHistory(bead *models.Bead, agentID string) (historyJSON string, loopDetected bool, loopReason string) {
	history := make([]string, 0)
	if bead != nil && bead.Context != nil {
		if raw := bead.Context["dispatch_history"]; raw != "" {
			_ = json.Unmarshal([]byte(raw), &history)
		}
	}
	history = append(history, agentID)
	if len(history) > 20 {
		history = history[len(history)-20:]
	}
	b, _ := json.Marshal(history)
	historyJSON = string(b)

	if len(history) < 6 {
		return historyJSON, false, ""
	}
	last := history[len(history)-6:]
	unique := map[string]struct{}{}
	for _, id := range last {
		unique[id] = struct{}{}
	}
	if len(unique) != 2 {
		return historyJSON, false, ""
	}
	if last[0] == last[1] {
		return historyJSON, false, ""
	}
	for i := 2; i < len(last); i++ {
		if last[i] != last[i%2] {
			return historyJSON, false, ""
		}
	}
	return historyJSON, true, "dispatch alternated between two agents for 6 runs"
}

func (d *Dispatcher) setStatus(state StatusState, reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.status = SystemStatus{State: state, Reason: reason, UpdatedAt: time.Now()}
}

func (d *Dispatcher) providersModel(providerID string) string {
	p, err := d.providers.Get(providerID)
	if err != nil || p == nil || p.Config == nil {
		return ""
	}
	return p.Config.Model
}

func buildBeadDescription(b *models.Bead) string {
	return fmt.Sprintf("Work on bead %s: %s", b.ID, b.Title)
}

func buildBeadContext(b *models.Bead, p *models.Project) string {
	ctx := ""
	if p != nil {
		ctx += fmt.Sprintf("Project: %s (%s)\nBranch: %s\n\n", p.Name, p.ID, p.Branch)
		if len(p.Context) > 0 {
			ctx += "Project Context:\n"
			for k, v := range p.Context {
				ctx += fmt.Sprintf("- %s: %s\n", k, v)
			}
			ctx += "\n"
		}
	}

	ctx += fmt.Sprintf("Bead ID: %s\nType: %s\nPriority: P%d\nStatus: %s\n\n", b.ID, b.Type, b.Priority, b.Status)
	ctx += "Bead Description:\n"
	ctx += b.Description + "\n\n"

	if len(b.Context) > 0 {
		ctx += "Bead Context:\n"
		for k, v := range b.Context {
			ctx += fmt.Sprintf("- %s: %s\n", k, v)
		}
		ctx += "\n"
	}

	ctx += "Output format:\n"
	ctx += "1) Short plan\n2) Key questions/risks\n3) Concrete next actions (commands/files to change)\n4) Proposed patch snippets if applicable\n"
	return ctx
}
