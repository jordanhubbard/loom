package workflow

import (
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// Database interface for workflow operations
type Database interface {
	GetWorkflow(id string) (*Workflow, error)
	ListWorkflows(workflowType, projectID string) ([]*Workflow, error)
	UpsertWorkflow(wf *Workflow) error
	UpsertWorkflowNode(node *WorkflowNode) error
	UpsertWorkflowEdge(edge *WorkflowEdge) error
	UpsertWorkflowExecution(exec *WorkflowExecution) error
	GetWorkflowExecution(id string) (*WorkflowExecution, error)
	GetWorkflowExecutionByBeadID(beadID string) (*WorkflowExecution, error)
	InsertWorkflowHistory(history *WorkflowExecutionHistory) error
	ListWorkflowHistory(executionID string) ([]*WorkflowExecutionHistory, error)
}

// BeadManager interface for bead operations
type BeadManager interface {
	UpdateBead(id string, updates map[string]interface{}) error
}

// BeadCreator interface for creating escalation beads
type BeadCreator interface {
	GetBead(id string) (interface{}, error)
}

// Engine manages workflow execution
type Engine struct {
	db    Database
	beads BeadManager
}

// NewEngine creates a new workflow engine
func NewEngine(db Database, beads BeadManager) *Engine {
	return &Engine{
		db:    db,
		beads: beads,
	}
}

// GetDatabase returns the underlying database interface
func (e *Engine) GetDatabase() Database {
	return e.db
}

// StartWorkflow initiates a workflow for a bead
func (e *Engine) StartWorkflow(beadID, workflowID, projectID string) (*WorkflowExecution, error) {
	// Check if execution already exists
	existing, err := e.db.GetWorkflowExecutionByBeadID(beadID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing execution: %w", err)
	}
	if existing != nil {
		return existing, nil // Already has a workflow
	}

	// Verify workflow exists
	wf, err := e.db.GetWorkflow(workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	// Create new execution
	exec := &WorkflowExecution{
		ID:               fmt.Sprintf("wfex-%s", uuid.New().String()[:8]),
		WorkflowID:       workflowID,
		BeadID:           beadID,
		ProjectID:        projectID,
		CurrentNodeKey:   "", // Empty = workflow start
		Status:           ExecutionStatusActive,
		CycleCount:       0,
		NodeAttemptCount: 0,
		StartedAt:        time.Now(),
		LastNodeAt:       time.Now(),
	}

	if err := e.db.UpsertWorkflowExecution(exec); err != nil {
		return nil, fmt.Errorf("failed to create workflow execution: %w", err)
	}

	// Update bead context to track workflow
	updates := map[string]interface{}{
		"context": map[string]string{
			"workflow_id":      workflowID,
			"workflow_exec_id": exec.ID,
			"workflow_node":    "",
			"workflow_status":  string(ExecutionStatusActive),
		},
	}
	if err := e.beads.UpdateBead(beadID, updates); err != nil {
		log.Printf("[Workflow] Warning: failed to update bead context: %v", err)
	}

	log.Printf("[Workflow] Started workflow %s for bead %s (exec: %s)", wf.Name, beadID, exec.ID)
	return exec, nil
}

// GetNextNode determines the next node to execute based on the current node and condition
func (e *Engine) GetNextNode(execution *WorkflowExecution, condition EdgeCondition) (*WorkflowNode, error) {
	wf, err := e.db.GetWorkflow(execution.WorkflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	// Find matching edge from current node
	var targetNodeKey string
	highestPriority := -1

	for _, edge := range wf.Edges {
		// Match edges from current node (or from start if currentNodeKey is empty)
		if edge.FromNodeKey == execution.CurrentNodeKey && edge.Condition == condition {
			if edge.Priority > highestPriority {
				highestPriority = edge.Priority
				targetNodeKey = edge.ToNodeKey
			}
		}
	}

	if targetNodeKey == "" {
		// No matching edge - check if this is workflow end
		if condition == EdgeConditionSuccess && execution.CurrentNodeKey != "" {
			// Look for workflow end transition (ToNodeKey empty)
			for _, edge := range wf.Edges {
				if edge.FromNodeKey == execution.CurrentNodeKey && edge.ToNodeKey == "" {
					return nil, nil // Workflow complete
				}
			}
		}
		return nil, fmt.Errorf("no edge found for condition %s from node %s", condition, execution.CurrentNodeKey)
	}

	// Empty target means workflow end
	if targetNodeKey == "" {
		return nil, nil
	}

	// Find target node
	for _, node := range wf.Nodes {
		if node.NodeKey == targetNodeKey {
			return &node, nil
		}
	}

	return nil, fmt.Errorf("target node not found: %s", targetNodeKey)
}

// AdvanceWorkflow moves the workflow to the next node based on condition
func (e *Engine) AdvanceWorkflow(executionID string, condition EdgeCondition, agentID string, resultData map[string]string) error {
	exec, err := e.db.GetWorkflowExecution(executionID)
	if err != nil {
		return fmt.Errorf("failed to get execution: %w", err)
	}

	// Check if already completed or escalated
	if exec.Status == ExecutionStatusCompleted || exec.Status == ExecutionStatusEscalated {
		return fmt.Errorf("workflow execution already %s", exec.Status)
	}

	// Record history
	resultJSON := ""
	if resultData != nil {
		// Simple JSON encoding - in production use encoding/json
		resultJSON = fmt.Sprintf("%v", resultData)
	}

	history := &WorkflowExecutionHistory{
		ID:            fmt.Sprintf("wfhist-%s", uuid.New().String()[:8]),
		ExecutionID:   executionID,
		NodeKey:       exec.CurrentNodeKey,
		AgentID:       agentID,
		Condition:     condition,
		ResultData:    resultJSON,
		AttemptNumber: exec.NodeAttemptCount,
		CreatedAt:     time.Now(),
	}
	if err := e.db.InsertWorkflowHistory(history); err != nil {
		log.Printf("[Workflow] Warning: failed to insert history: %v", err)
	}

	// Get next node
	nextNode, err := e.GetNextNode(exec, condition)
	if err != nil {
		return fmt.Errorf("failed to get next node: %w", err)
	}

	// If no next node, workflow is complete
	if nextNode == nil {
		exec.Status = ExecutionStatusCompleted
		now := time.Now()
		exec.CompletedAt = &now
		exec.LastNodeAt = now

		if err := e.db.UpsertWorkflowExecution(exec); err != nil {
			return fmt.Errorf("failed to complete workflow: %w", err)
		}

		// Update bead context
		updates := map[string]interface{}{
			"context": map[string]string{
				"workflow_status": string(ExecutionStatusCompleted),
			},
		}
		if err := e.beads.UpdateBead(exec.BeadID, updates); err != nil {
			log.Printf("[Workflow] Warning: failed to update bead context: %v", err)
		}

		log.Printf("[Workflow] Completed workflow execution %s for bead %s", executionID, exec.BeadID)
		return nil
	}

	// Check if transitioning to a node we've already visited (cycle detection)
	historyList, err := e.db.ListWorkflowHistory(executionID)
	if err == nil {
		for _, h := range historyList {
			if h.NodeKey == nextNode.NodeKey {
				exec.CycleCount++
				log.Printf("[Workflow] Cycle detected for bead %s: cycle_count=%d", exec.BeadID, exec.CycleCount)
				break
			}
		}
	}

	// Check for escalation conditions
	if exec.CycleCount >= 3 {
		return e.escalateWorkflow(exec, fmt.Sprintf("Exceeded max cycles (3): workflow has cycled %d times", exec.CycleCount))
	}

	// Move to next node
	exec.CurrentNodeKey = nextNode.NodeKey
	exec.NodeAttemptCount = 0 // Reset attempt count for new node
	exec.LastNodeAt = time.Now()

	if err := e.db.UpsertWorkflowExecution(exec); err != nil {
		return fmt.Errorf("failed to update workflow execution: %w", err)
	}

	// Update bead context with current node
	updates := map[string]interface{}{
		"context": map[string]string{
			"workflow_node":   nextNode.NodeKey,
			"workflow_status": string(exec.Status),
			"cycle_count":     fmt.Sprintf("%d", exec.CycleCount),
		},
	}

	// Set role assignment hint if specified
	if nextNode.RoleRequired != "" {
		roleUpdates := updates["context"].(map[string]string)
		roleUpdates["required_role"] = nextNode.RoleRequired
	}

	if err := e.beads.UpdateBead(exec.BeadID, updates); err != nil {
		log.Printf("[Workflow] Warning: failed to update bead context: %v", err)
	}

	log.Printf("[Workflow] Advanced bead %s to node %s (attempt %d, cycle %d)",
		exec.BeadID, nextNode.NodeKey, exec.NodeAttemptCount, exec.CycleCount)

	return nil
}

// CompleteNode marks a node as completed and advances the workflow
func (e *Engine) CompleteNode(executionID, agentID string, result map[string]string) error {
	exec, err := e.db.GetWorkflowExecution(executionID)
	if err != nil {
		return err
	}

	// Increment attempt count
	exec.NodeAttemptCount++

	// Check max attempts for current node
	wf, err := e.db.GetWorkflow(exec.WorkflowID)
	if err != nil {
		return err
	}

	var currentNode *WorkflowNode
	for _, node := range wf.Nodes {
		if node.NodeKey == exec.CurrentNodeKey {
			currentNode = &node
			break
		}
	}

	if currentNode != nil && currentNode.MaxAttempts > 0 {
		if exec.NodeAttemptCount >= currentNode.MaxAttempts {
			return e.escalateWorkflow(exec, fmt.Sprintf("Exceeded max attempts (%d) for node %s", currentNode.MaxAttempts, currentNode.NodeKey))
		}
	}

	// Update execution with incremented attempt
	if err := e.db.UpsertWorkflowExecution(exec); err != nil {
		return err
	}

	// Advance workflow with success condition
	return e.AdvanceWorkflow(executionID, EdgeConditionSuccess, agentID, result)
}

// FailNode marks a node as failed and transitions based on failure edge
func (e *Engine) FailNode(executionID, agentID, reason string) error {
	exec, err := e.db.GetWorkflowExecution(executionID)
	if err != nil {
		return err
	}

	exec.NodeAttemptCount++

	// Check max attempts
	wf, err := e.db.GetWorkflow(exec.WorkflowID)
	if err != nil {
		return err
	}

	var currentNode *WorkflowNode
	for _, node := range wf.Nodes {
		if node.NodeKey == exec.CurrentNodeKey {
			currentNode = &node
			break
		}
	}

	if currentNode != nil && currentNode.MaxAttempts > 0 {
		if exec.NodeAttemptCount >= currentNode.MaxAttempts {
			return e.escalateWorkflow(exec, fmt.Sprintf("Exceeded max attempts (%d) for node %s: %s", currentNode.MaxAttempts, currentNode.NodeKey, reason))
		}
	}

	if err := e.db.UpsertWorkflowExecution(exec); err != nil {
		return err
	}

	// Advance with failure condition
	resultData := map[string]string{"failure_reason": reason}
	return e.AdvanceWorkflow(executionID, EdgeConditionFailure, agentID, resultData)
}

// escalateWorkflow escalates the workflow to CEO
func (e *Engine) escalateWorkflow(exec *WorkflowExecution, reason string) error {
	log.Printf("[Workflow] Escalating workflow execution %s for bead %s: %s", exec.ID, exec.BeadID, reason)

	exec.Status = ExecutionStatusEscalated
	now := time.Now()
	exec.EscalatedAt = &now
	exec.LastNodeAt = now

	if err := e.db.UpsertWorkflowExecution(exec); err != nil {
		return fmt.Errorf("failed to escalate workflow: %w", err)
	}

	// Update bead context
	updates := map[string]interface{}{
		"context": map[string]string{
			"workflow_status":   string(ExecutionStatusEscalated),
			"escalation_reason": reason,
			"escalated_at":      now.Format(time.RFC3339),
			"needs_ceo_review":  "true",
		},
	}
	if err := e.beads.UpdateBead(exec.BeadID, updates); err != nil {
		log.Printf("[Workflow] Warning: failed to update bead context: %v", err)
	}

	log.Printf("[Workflow] Workflow escalated for bead %s - CEO escalation bead should be created", exec.BeadID)

	return nil
}

// GetEscalationInfo returns information needed to create an escalation bead
func (e *Engine) GetEscalationInfo(exec *WorkflowExecution) (string, string, error) {
	// Get workflow details
	wf, err := e.db.GetWorkflow(exec.WorkflowID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get workflow: %w", err)
	}

	// Get workflow history for context
	history, err := e.db.ListWorkflowHistory(exec.ID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get workflow history: %w", err)
	}

	// Build escalation title and description
	title := fmt.Sprintf("[CEO-Escalation] Workflow stuck: %s", exec.BeadID)

	description := fmt.Sprintf(`# Workflow Escalation

**Original Bead:** %s
**Workflow:** %s (%s)
**Escalation Reason:** Exceeded max cycles or attempts

## Workflow Progress

- **Cycles Completed:** %d
- **Current Node:** %s
- **Node Attempts:** %d
- **Escalated At:** %s

## History Summary

`, exec.BeadID, wf.Name, wf.WorkflowType, exec.CycleCount, exec.CurrentNodeKey, exec.NodeAttemptCount, time.Now().Format(time.RFC3339))

	if len(history) > 0 {
		description += fmt.Sprintf("Total workflow steps: %d\n\n", len(history))
		// Show last 5 steps
		startIdx := 0
		if len(history) > 5 {
			startIdx = len(history) - 5
		}
		for i := startIdx; i < len(history); i++ {
			h := history[i]
			description += fmt.Sprintf("- **%s** (attempt %d): %s\n", h.NodeKey, h.AttemptNumber, h.Condition)
		}
	}

	description += fmt.Sprintf(`

## Required Action

This workflow has exceeded the maximum number of cycles (%d) or attempts. Please review:

1. The original bead and its requirements
2. The workflow execution history above
3. Whether the workflow needs to be adjusted
4. Whether the bead should be reassigned or closed

## Options

- **Approve with Instructions:** Provide specific guidance for how to proceed
- **Reject and Reassign:** Assign to a different agent or role
- **Close Bead:** Mark as won't fix or duplicate
- **Modify Workflow:** Update the workflow definition if the process is flawed

`, exec.CycleCount)

	return title, description, nil
}

// GetCurrentNode returns the current node for an execution
func (e *Engine) GetCurrentNode(executionID string) (*WorkflowNode, error) {
	exec, err := e.db.GetWorkflowExecution(executionID)
	if err != nil {
		return nil, err
	}

	if exec.CurrentNodeKey == "" {
		return nil, nil // At workflow start
	}

	wf, err := e.db.GetWorkflow(exec.WorkflowID)
	if err != nil {
		return nil, err
	}

	for _, node := range wf.Nodes {
		if node.NodeKey == exec.CurrentNodeKey {
			return &node, nil
		}
	}

	return nil, fmt.Errorf("current node not found: %s", exec.CurrentNodeKey)
}

// IsNodeReady checks if a node is ready to be executed (no blocking conditions)
func (e *Engine) IsNodeReady(execution *WorkflowExecution) bool {
	if execution.Status != ExecutionStatusActive {
		return false
	}

	// Check for timeout
	if err := e.CheckNodeTimeout(execution); err != nil {
		log.Printf("[Workflow] Node timeout detected for bead %s: %v", execution.BeadID, err)
		return false
	}

	return true
}

// CheckNodeTimeout checks if the current node has exceeded its timeout
func (e *Engine) CheckNodeTimeout(execution *WorkflowExecution) error {
	if execution.CurrentNodeKey == "" {
		return nil // At workflow start, no timeout
	}

	// Get current node
	node, err := e.GetCurrentNode(execution.ID)
	if err != nil || node == nil {
		return nil // No node, no timeout
	}

	// Check if node has timeout configured
	if node.TimeoutMinutes <= 0 {
		return nil // No timeout configured
	}

	// Calculate time since node started
	timeSinceNode := time.Since(execution.LastNodeAt)
	timeoutDuration := time.Duration(node.TimeoutMinutes) * time.Minute

	if timeSinceNode > timeoutDuration {
		// Node has timed out - advance workflow with timeout condition
		log.Printf("[Workflow] Node %s timed out for bead %s (elapsed: %v, timeout: %v)",
			node.NodeKey, execution.BeadID, timeSinceNode, timeoutDuration)

		resultData := map[string]string{
			"timeout_reason": fmt.Sprintf("Node exceeded timeout of %d minutes", node.TimeoutMinutes),
			"elapsed_time":   timeSinceNode.String(),
		}

		// Advance with timeout condition
		if err := e.AdvanceWorkflow(execution.ID, EdgeConditionTimeout, "system", resultData); err != nil {
			return fmt.Errorf("node timed out but failed to advance workflow: %w", err)
		}

		return fmt.Errorf("node %s timed out after %v", node.NodeKey, timeSinceNode)
	}

	return nil
}

// GetWorkflowForBead determines which workflow to use for a bead
func (e *Engine) GetWorkflowForBead(beadType, beadTitle, projectID string) (*Workflow, error) {
	// Determine workflow type from bead type/title
	workflowType := "bug" // Default

	// TODO: Add heuristics to detect workflow type from bead
	// For now, use simple mapping:
	// - Beads with "UI" or "design" → "ui"
	// - Beads with "feature" or "enhancement" → "feature"
	// - Everything else → "bug"

	workflows, err := e.db.ListWorkflows(workflowType, projectID)
	if err != nil {
		return nil, err
	}

	if len(workflows) == 0 {
		return nil, fmt.Errorf("no workflow found for type %s", workflowType)
	}

	// Return first matching workflow (prioritizes project-specific, then defaults)
	return workflows[0], nil
}
