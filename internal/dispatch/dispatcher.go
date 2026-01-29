package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jordanhubbard/agenticorp/internal/agent"
	"github.com/jordanhubbard/agenticorp/internal/beads"
	"github.com/jordanhubbard/agenticorp/internal/observability"
	"github.com/jordanhubbard/agenticorp/internal/project"
	"github.com/jordanhubbard/agenticorp/internal/provider"
	"github.com/jordanhubbard/agenticorp/internal/temporal/eventbus"
	"github.com/jordanhubbard/agenticorp/internal/worker"
	"github.com/jordanhubbard/agenticorp/internal/workflow"
	"github.com/jordanhubbard/agenticorp/pkg/models"
)

type StatusState string

const (
	StatusActive StatusState = "active"
	StatusParked StatusState = "parked"
)

type ReadinessMode string

const (
	ReadinessBlock ReadinessMode = "block"
	ReadinessWarn  ReadinessMode = "warn"
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
	beads          *beads.Manager
	projects       *project.Manager
	agents         *agent.WorkerManager
	providers      *provider.Registry
	eventBus       *eventbus.EventBus
	workflowEngine *workflow.Engine
	personaMatcher *PersonaMatcher
	autoBugRouter  *AutoBugRouter
	readinessCheck func(context.Context, string) (bool, []string)
	readinessMode  ReadinessMode

	mu     sync.RWMutex
	status SystemStatus
}

func NewDispatcher(beadsMgr *beads.Manager, projMgr *project.Manager, agentMgr *agent.WorkerManager, registry *provider.Registry, eb *eventbus.EventBus) *Dispatcher {
	d := &Dispatcher{
		beads:          beadsMgr,
		projects:       projMgr,
		agents:         agentMgr,
		providers:      registry,
		eventBus:       eb,
		personaMatcher: NewPersonaMatcher(),
		autoBugRouter:  NewAutoBugRouter(),
		readinessMode:  ReadinessBlock,
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

// SetWorkflowEngine sets the workflow engine for workflow-aware dispatching
func (d *Dispatcher) SetWorkflowEngine(engine *workflow.Engine) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.workflowEngine = engine
}

func (d *Dispatcher) SetReadinessCheck(check func(context.Context, string) (bool, []string)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.readinessCheck = check
}

func (d *Dispatcher) SetReadinessMode(mode ReadinessMode) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if mode != ReadinessWarn {
		mode = ReadinessBlock
	}
	d.readinessMode = mode
}

// DispatchOnce finds at most one ready bead and asks an idle agent to work on it.
func (d *Dispatcher) DispatchOnce(ctx context.Context, projectID string) (*DispatchResult, error) {
	activeProviders := d.providers.ListActive()
	log.Printf("[Dispatcher] DispatchOnce called for project=%s, active_providers=%d", projectID, len(activeProviders))
	if len(activeProviders) == 0 {
		log.Printf("[Dispatcher] Parked - no active providers")
		d.setStatus(StatusParked, "no active providers registered")
		return &DispatchResult{Dispatched: false, ProjectID: projectID}, nil
	}

	ready, err := d.beads.GetReadyBeads(projectID)
	if err != nil {
		d.setStatus(StatusParked, "failed to list ready beads")
		return nil, err
	}

	d.mu.RLock()
	readinessCheck := d.readinessCheck
	readinessMode := d.readinessMode
	d.mu.RUnlock()

	if readinessCheck != nil {
		if projectID != "" {
			readyOK, issues := readinessCheck(ctx, projectID)
			if !readyOK && readinessMode == ReadinessBlock {
				reason := "project readiness failed"
				if len(issues) > 0 {
					reason = fmt.Sprintf("project readiness failed: %s", strings.Join(issues, "; "))
				}
				d.setStatus(StatusParked, reason)
				return &DispatchResult{Dispatched: false, ProjectID: projectID, Error: reason}, nil
			}
		}

		projectReadiness := make(map[string]bool)
		if readinessMode == ReadinessBlock {
			filtered := make([]*models.Bead, 0, len(ready))
			for _, bead := range ready {
				if bead == nil {
					filtered = append(filtered, bead)
					continue
				}
				if _, ok := projectReadiness[bead.ProjectID]; !ok {
					okReady, _ := readinessCheck(ctx, bead.ProjectID)
					projectReadiness[bead.ProjectID] = okReady
				}
				if projectReadiness[bead.ProjectID] {
					filtered = append(filtered, bead)
				}
			}
			ready = filtered
			if len(ready) == 0 {
				d.setStatus(StatusParked, "project readiness failed")
				return &DispatchResult{Dispatched: false, ProjectID: projectID}, nil
			}
		} else {
			for _, bead := range ready {
				if bead == nil {
					continue
				}
				if _, ok := projectReadiness[bead.ProjectID]; ok {
					continue
				}
				okReady, _ := readinessCheck(ctx, bead.ProjectID)
				projectReadiness[bead.ProjectID] = okReady
			}
		}
	}

	log.Printf("[Dispatcher] GetReadyBeads returned %d beads for project %s", len(ready), projectID)

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
	filteredAgents := make([]*models.Agent, 0, len(idleAgents))
	for _, candidateAgent := range idleAgents {
		if candidateAgent == nil {
			continue
		}
		if candidateAgent.ProviderID == "" {
			continue
		}
		if !d.providers.IsActive(candidateAgent.ProviderID) {
			continue
		}
		filteredAgents = append(filteredAgents, candidateAgent)
	}
	idleAgents = filteredAgents
	idleByID := make(map[string]*models.Agent, len(idleAgents))
	for _, a := range idleAgents {
		if a != nil {
			idleByID[a.ID] = a
		}
	}

	var candidate *models.Bead
	var ag *models.Agent
	skippedReasons := make(map[string]int)
	for _, b := range ready {
		if b == nil {
			skippedReasons["nil_bead"]++
			continue
		}

		// Check if this is an auto-filed bug that needs routing
		if routeInfo := d.autoBugRouter.AnalyzeBugForRouting(b); routeInfo.ShouldRoute {
			log.Printf("[Dispatcher] Auto-bug detected: %s - routing to %s (%s)", b.ID, routeInfo.PersonaHint, routeInfo.RoutingReason)

			// Update the bead with persona hint in title
			updates := map[string]interface{}{
				"title": routeInfo.UpdatedTitle,
			}
			if err := d.beads.UpdateBead(b.ID, updates); err != nil {
				log.Printf("[Dispatcher] Failed to update bead %s with persona hint: %v", b.ID, err)
			} else {
				// Refresh the bead to get updated title
				b.Title = routeInfo.UpdatedTitle
			}
		}

		// Skip P0 beads UNLESS they are auto-filed bugs (which we want to dispatch)
		isAutoFiled := strings.Contains(strings.ToLower(b.Title), "[auto-filed]")
		if b.Priority == models.BeadPriorityP0 && !isAutoFiled {
			skippedReasons["p0_priority"]++
			continue
		}

		if b.Type == "decision" {
			skippedReasons["decision_type"]++
			continue
		}
		// Allow redispatch for in_progress beads (multi-step investigations)
		// This is a temporary fix until workflow system is implemented
		if b.Context != nil {
			// Track dispatch count to prevent infinite loops
			dispatchCountStr := b.Context["dispatch_count"]
			dispatchCount := 0
			if dispatchCountStr != "" {
				fmt.Sscanf(dispatchCountStr, "%d", &dispatchCount)
			}

			// Escalate to CEO after 10 dispatches (safety limit)
			if dispatchCount >= 10 {
				log.Printf("[Dispatcher] WARNING: Bead %s has been dispatched %d times, escalating to CEO", b.ID, dispatchCount)
				// TODO: Create CEO escalation bead when workflow system is ready
				skippedReasons["excessive_dispatches"]++
				continue
			}

			// Warn if dispatch count is getting high
			if dispatchCount >= 5 {
				log.Printf("[Dispatcher] WARNING: Bead %s has been dispatched %d times", b.ID, dispatchCount)
			}

			// Skip beads that have already run UNLESS:
			// 1. They explicitly request redispatch, OR
			// 2. They are still in_progress (multi-step work not complete)
			if b.Context["redispatch_requested"] != "true" &&
				b.Status != "in_progress" &&
				b.Context["last_run_at"] != "" {
				skippedReasons["already_run"]++
				continue
			}
		}

		// If bead is assigned to an agent, only dispatch to that agent.
		if b.AssignedTo != "" {
			assigned, ok := idleByID[b.AssignedTo]
			if !ok {
				skippedReasons["assigned_agent_not_idle"]++
				continue
			}
			ag = assigned
			candidate = b
			break
		}

		// Check if bead has a workflow and needs specific role
		var workflowRoleRequired string
		if d.workflowEngine != nil {
			execution, err := d.ensureBeadHasWorkflow(ctx, b)
			if err != nil {
				log.Printf("[Workflow] Error ensuring workflow for bead %s: %v", b.ID, err)
			} else if execution != nil {
				// Check for timeout before processing
				if !d.workflowEngine.IsNodeReady(execution) {
					skippedReasons["workflow_node_not_ready"]++
					log.Printf("[Workflow] Bead %s workflow node not ready (may have timed out)", b.ID)
					continue
				}

				workflowRoleRequired = d.getWorkflowRoleRequirement(execution)
				if workflowRoleRequired != "" {
					requiredRoleKey := normalizeRoleName(workflowRoleRequired)
					// Find agent with matching role
					for _, agent := range idleAgents {
						if agent != nil && normalizeRoleName(agent.Role) == requiredRoleKey {
							ag = agent
							candidate = b
							log.Printf("[Workflow] Matched bead %s to agent %s by workflow role %s", b.ID, agent.Name, workflowRoleRequired)
							break
						}
					}

					if ag != nil {
						break // Found workflow-matched agent
					}

					// No agent with required role available
					skippedReasons["workflow_role_not_available"]++
					continue
				}
			}
		}

		// Try persona-based routing first, but fall back to any idle agent
		personaHint := d.personaMatcher.ExtractPersonaHint(b)
		if personaHint != "" {
			matchedAgent := d.personaMatcher.FindAgentByPersonaHint(personaHint, idleAgents)
			if matchedAgent != nil {
				ag = matchedAgent
				candidate = b
				log.Printf("[Dispatcher] Matched bead %s to agent %s via persona hint '%s'", b.ID, matchedAgent.Name, personaHint)
				break
			}
			// Persona hint found but no match - log it but fall through to assign any idle agent
			log.Printf("[Dispatcher] Bead %s has persona hint '%s' but no exact match - will assign to any idle agent", b.ID, personaHint)
		}

		// Pick any idle agent (either no persona hint, or hint didn't match)
		if len(idleAgents) == 0 {
			skippedReasons["no_idle_agents"]++
			log.Printf("[Dispatcher] Bead %s: no idle agents available", b.ID)
			continue
		}
		log.Printf("[Dispatcher] Assigning bead %s to agent %s (any idle agent)", b.ID, idleAgents[0].Name)
		ag = idleAgents[0]
		candidate = b
		break
	}

	if len(skippedReasons) > 0 {
		log.Printf("[Dispatcher] Skipped beads: %+v", skippedReasons)
	}

	if candidate == nil {
		log.Printf("[Dispatcher] No dispatchable beads found (ready: %d, idle agents: %d)", len(ready), len(idleAgents))
		d.setStatus(StatusParked, "no dispatchable beads")
		return &DispatchResult{Dispatched: false, ProjectID: projectID}, nil
	}

	selectedProjectID := projectID
	if selectedProjectID == "" {
		selectedProjectID = candidate.ProjectID
	}
	if ag == nil {
		d.setStatus(StatusParked, "no idle agents with active providers")
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
			observability.Error("dispatch.claim", map[string]interface{}{
				"agent_id":   ag.ID,
				"bead_id":    candidate.ID,
				"project_id": candidate.ProjectID,
			}, err)
			return &DispatchResult{Dispatched: false, ProjectID: projectID}, nil
		}
		observability.Info("dispatch.claim", map[string]interface{}{
			"agent_id":   ag.ID,
			"bead_id":    candidate.ID,
			"project_id": candidate.ProjectID,
		})
	}

	// Increment dispatch count for tracking multi-step investigations
	dispatchCount := 0
	if candidate.Context != nil {
		if countStr := candidate.Context["dispatch_count"]; countStr != "" {
			fmt.Sscanf(countStr, "%d", &dispatchCount)
		}
	}
	dispatchCount++

	// Update bead context with incremented dispatch count
	countUpdates := map[string]interface{}{
		"context": map[string]string{
			"dispatch_count": fmt.Sprintf("%d", dispatchCount),
		},
	}
	if err := d.beads.UpdateBead(candidate.ID, countUpdates); err != nil {
		log.Printf("[Dispatcher] WARNING: Failed to update dispatch count for bead %s: %v", candidate.ID, err)
		// Don't fail dispatch on this error - just log it
	}
	log.Printf("[Dispatcher] Bead %s dispatch count: %d", candidate.ID, dispatchCount)

	_ = d.agents.AssignBead(ag.ID, candidate.ID)
	observability.Info("dispatch.assign", map[string]interface{}{
		"agent_id":    ag.ID,
		"bead_id":     candidate.ID,
		"project_id":  selectedProjectID,
		"provider_id": ag.ProviderID,
	})
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
		observability.Error("dispatch.execute", map[string]interface{}{
			"agent_id":    ag.ID,
			"bead_id":     candidate.ID,
			"project_id":  selectedProjectID,
			"provider_id": ag.ProviderID,
		}, execErr)

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

		// Handle workflow failure
		if d.workflowEngine != nil {
			execution, err := d.workflowEngine.GetDatabase().GetWorkflowExecutionByBeadID(candidate.ID)
			if err == nil && execution != nil {
				// Report failure to workflow
				if err := d.workflowEngine.FailNode(execution.ID, ag.ID, execErr.Error()); err != nil {
					log.Printf("[Workflow] Failed to report failure to workflow for bead %s: %v", candidate.ID, err)
				} else {
					log.Printf("[Workflow] Reported failure to workflow for bead %s", candidate.ID)
				}
			}
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

	// Advance workflow after successful task execution
	if d.workflowEngine != nil && !loopDetected {
		execution, err := d.workflowEngine.GetDatabase().GetWorkflowExecutionByBeadID(candidate.ID)
		if err == nil && execution != nil {
			// Advance workflow with success condition
			resultData := map[string]string{
				"agent_id":    ag.ID,
				"output":      result.Response,
				"tokens_used": fmt.Sprintf("%d", result.TokensUsed),
			}
			if err := d.workflowEngine.AdvanceWorkflow(execution.ID, workflow.EdgeConditionSuccess, ag.ID, resultData); err != nil {
				log.Printf("[Workflow] Failed to advance workflow for bead %s: %v", candidate.ID, err)
			} else {
				// Get updated execution to check status
				updatedExec, _ := d.workflowEngine.GetDatabase().GetWorkflowExecution(execution.ID)
				if updatedExec != nil {
					log.Printf("[Workflow] Advanced workflow for bead %s: status=%s, node=%s, cycle=%d",
						candidate.ID, updatedExec.Status, updatedExec.CurrentNodeKey, updatedExec.CycleCount)

					// Check if workflow was escalated and needs CEO bead
					if updatedExec.Status == workflow.ExecutionStatusEscalated && candidate.Context["escalation_bead_created"] != "true" {
						log.Printf("[Workflow] Creating CEO escalation bead for workflow %s (bead %s)", updatedExec.ID, candidate.ID)

						// Get escalation info from workflow engine
						title, description, err := d.workflowEngine.GetEscalationInfo(updatedExec)
						if err != nil {
							log.Printf("[Workflow] Failed to get escalation info for workflow %s: %v", updatedExec.ID, err)
						} else {
							// Create CEO escalation bead
							createdBead, err := d.beads.CreateBead(
								title,
								description,
								models.BeadPriorityP0,
								"decision",
								candidate.ProjectID,
							)
							if err != nil {
								log.Printf("[Workflow] Failed to create CEO escalation bead: %v", err)
							} else {
								log.Printf("[Workflow] Created CEO escalation bead %s for workflow %s", createdBead.ID, updatedExec.ID)

								// Update the escalation bead with tags and context
								escalationBeadUpdates := map[string]interface{}{
									"tags": []string{"workflow-escalation", "ceo-review", "urgent"},
									"context": map[string]string{
										"original_bead_id":      candidate.ID,
										"workflow_execution_id": updatedExec.ID,
										"escalation_reason":     candidate.Context["escalation_reason"],
										"escalated_at":          time.Now().UTC().Format(time.RFC3339),
									},
								}
								if err := d.beads.UpdateBead(createdBead.ID, escalationBeadUpdates); err != nil {
									log.Printf("[Workflow] Failed to update escalation bead with tags and context: %v", err)
								}

								// Mark original bead as having escalation bead created
								originalUpdates := map[string]interface{}{
									"context": map[string]string{
										"escalation_bead_created": "true",
										"escalation_bead_id":      createdBead.ID,
									},
								}
								if err := d.beads.UpdateBead(candidate.ID, originalUpdates); err != nil {
									log.Printf("[Workflow] Failed to update original bead with escalation info: %v", err)
								}
							}
						}
					}
				}
			}
		}
	}

	d.setStatus(StatusParked, "idle")
	observability.Info("dispatch.execute", map[string]interface{}{
		"agent_id":    ag.ID,
		"bead_id":     candidate.ID,
		"project_id":  selectedProjectID,
		"provider_id": ag.ProviderID,
		"status":      "success",
	})
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

	// Add specialized instructions for auto-filed bugs
	if isAutoFiledBug(b) {
		ctx += buildBugInvestigationInstructions(b)
	} else {
		ctx += "Output format:\n"
		ctx += "1) Short plan\n2) Key questions/risks\n3) Concrete next actions (commands/files to change)\n4) Proposed patch snippets if applicable\n"
	}
	return ctx
}

// isAutoFiledBug checks if a bead is an auto-filed bug
func isAutoFiledBug(b *models.Bead) bool {
	if b == nil {
		return false
	}
	if strings.Contains(strings.ToLower(b.Title), "[auto-filed]") {
		return true
	}
	for _, tag := range b.Tags {
		if strings.ToLower(tag) == "auto-filed" {
			return true
		}
	}
	return false
}

// buildBugInvestigationInstructions returns specialized instructions for investigating auto-filed bugs
func buildBugInvestigationInstructions(b *models.Bead) string {
	return `
# Bug Investigation Workflow

You have been assigned an auto-filed bug. Follow this investigation workflow:

## Step 1: Extract Error Context
From the bug report above, identify:
- Error message (what went wrong)
- Stack trace location (file, line, function)
- Error type (JavaScript, Go, API, etc.)
- Additional context (URL, user agent, etc.)

## Step 2: Search for Relevant Code
Use search_text to find:
- The exact error location from stack trace
- Function/variable names mentioned in error
- Related API endpoints or handlers

Example:
{"type": "search_text", "query": "<function_name>", "path": "<directory>"}

## Step 3: Read Relevant Files
Use read_file to examine:
- Files identified in search
- Code around the error location
- Related dependencies

Example:
{"type": "read_file", "path": "<file_path>"}

## Step 4: Analyze Root Cause
Determine:
- What specific bug occurred (undefined variable, nil pointer, API mismatch, etc.)
- Why it happened (missing import, duplicate declaration, wrong format, etc.)
- The correct fix approach

## Step 5: Propose Fix
Create a fix using write_file or apply_patch:
- For small targeted changes: Use apply_patch with unified diff
- For larger rewrites: Use write_file with complete new content

Example patch:
{"type": "apply_patch", "path": "<file>", "patch": "--- a/file\n+++ b/file\n@@ -X,Y +A,B @@\n-old line\n+new line"}

## Step 6: Create CEO Approval Request
Use create_bead to request approval:

{"type": "create_bead", "bead": {
  "title": "[CEO] Code Fix Approval: <Brief Description>",
  "description": "## Code Fix Proposal\n\n**Original Bug:** ` + b.ID + `\n\n### Root Cause Analysis\n<Explain what went wrong and why>\n\n### Proposed Fix\n<High-level solution description>\n\n### Changes Required\n<Unified diff or description of changes>\n\n### Risk Assessment\n**Risk Level:** Low/Medium/High\n**Side Effects:** <List any potential issues>\n**Testing:** <How to verify the fix>\n\n### Recommendation\nI recommend approval because <reasoning>.",
  "type": "decision",
  "priority": 0,
  "tags": ["code-fix", "approval-required", "auto-bug-fix"]
}}

## Step 7: Wait for Approval
After creating the approval bead:
- Add comment to this bug bead linking to approval request
- Wait for CEO to review and approve/reject
- If approved, the fix will be applied in a follow-up dispatch

## Important Notes
- Be thorough in root cause analysis
- Consider side effects and edge cases
- Test your understanding by reading related code
- Propose conservative, minimal fixes
- Document your reasoning clearly

## Output Format
Provide your investigation as a series of actions following the workflow above.
Use the "notes" field in your JSON response to explain your reasoning at each step.
`
}

// ensureBeadHasWorkflow checks if a bead has a workflow execution, and if not, starts one
func (d *Dispatcher) ensureBeadHasWorkflow(ctx context.Context, bead *models.Bead) (*workflow.WorkflowExecution, error) {
	if d.workflowEngine == nil {
		return nil, nil // Workflow engine not available
	}

	// Check if bead already has a workflow
	execution, err := d.workflowEngine.GetDatabase().GetWorkflowExecutionByBeadID(bead.ID)
	if err != nil {
		log.Printf("[Workflow] Error checking workflow for bead %s: %v", bead.ID, err)
		return nil, err
	}

	if execution != nil {
		// Bead already has a workflow
		return execution, nil
	}

	// Determine workflow type from bead
	workflowType := "bug" // Default
	title := strings.ToLower(bead.Title)
	if strings.Contains(title, "feature") || strings.Contains(title, "enhancement") {
		workflowType = "feature"
	} else if strings.Contains(title, "ui") || strings.Contains(title, "design") || strings.Contains(title, "css") || strings.Contains(title, "html") {
		workflowType = "ui"
	}

	// Get default workflow for this type
	workflows, err := d.workflowEngine.GetDatabase().ListWorkflows(workflowType, bead.ProjectID)
	if err != nil || len(workflows) == 0 {
		log.Printf("[Workflow] No workflow found for type %s, bead %s", workflowType, bead.ID)
		return nil, nil // No workflow available
	}

	// Start workflow for this bead
	execution, err = d.workflowEngine.StartWorkflow(bead.ID, workflows[0].ID, bead.ProjectID)
	if err != nil {
		log.Printf("[Workflow] Failed to start workflow for bead %s: %v", bead.ID, err)
		return nil, err
	}

	log.Printf("[Workflow] Started workflow %s for bead %s", workflows[0].Name, bead.ID)
	return execution, nil
}

// getWorkflowRoleRequirement returns the role required for the current workflow node, if any
func (d *Dispatcher) getWorkflowRoleRequirement(execution *workflow.WorkflowExecution) string {
	if d.workflowEngine == nil || execution == nil {
		return ""
	}

	// If at workflow start (no current node), get first node
	if execution.CurrentNodeKey == "" {
		// Get first node from workflow
		wf, err := d.workflowEngine.GetDatabase().GetWorkflow(execution.WorkflowID)
		if err != nil {
			return ""
		}

		// Find edges from start (empty FromNodeKey)
		for _, edge := range wf.Edges {
			if edge.FromNodeKey == "" && edge.Condition == workflow.EdgeConditionSuccess {
				// Found start edge, get target node
				for _, node := range wf.Nodes {
					if node.NodeKey == edge.ToNodeKey {
						// Enforce Engineering Manager for commit nodes
						if node.NodeType == workflow.NodeTypeCommit {
							return "Engineering Manager"
						}
						return node.RoleRequired
					}
				}
			}
		}
		return ""
	}

	// Get current node
	node, err := d.workflowEngine.GetCurrentNode(execution.ID)
	if err != nil || node == nil {
		return ""
	}

	// Enforce Engineering Manager for commit nodes
	if node.NodeType == workflow.NodeTypeCommit {
		return "Engineering Manager"
	}

	return node.RoleRequired
}

func normalizeRoleName(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	if role == "" {
		return ""
	}

	if strings.Contains(role, "/") {
		parts := strings.Split(role, "/")
		role = parts[len(parts)-1]
	}

	if idx := strings.Index(role, "("); idx != -1 {
		role = strings.TrimSpace(role[:idx])
	}

	role = strings.ReplaceAll(role, "_", "-")
	role = strings.ReplaceAll(role, " ", "-")
	for strings.Contains(role, "--") {
		role = strings.ReplaceAll(role, "--", "-")
	}
	role = strings.Trim(role, "-")
	return role
}
