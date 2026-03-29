package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jordanhubbard/loom/internal/workflow"
)

// UpsertWorkflow inserts or updates a workflow
func (d *Database) UpsertWorkflow(wf *workflow.Workflow) error {
	if wf == nil {
		return fmt.Errorf("workflow cannot be nil")
	}
	if wf.CreatedAt.IsZero() {
		wf.CreatedAt = time.Now()
	}
	wf.UpdatedAt = time.Now()

	query := `
		INSERT INTO workflows (id, name, description, workflow_type, is_default, project_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			workflow_type = excluded.workflow_type,
			is_default = excluded.is_default,
			project_id = excluded.project_id,
			updated_at = excluded.updated_at
	`

	var projectID interface{}
	if wf.ProjectID != "" {
		projectID = wf.ProjectID
	}

	_, err := d.db.Exec(rebind(query),
		wf.ID,
		wf.Name,
		wf.Description,
		wf.WorkflowType,
		wf.IsDefault,
		projectID,
		wf.CreatedAt,
		wf.UpdatedAt,
	)
	return err
}

// GetWorkflow retrieves a workflow by ID
func (d *Database) GetWorkflow(id string) (*workflow.Workflow, error) {
	query := `
		SELECT id, name, description, workflow_type, is_default, project_id, created_at, updated_at
		FROM workflows
		WHERE id = ?
	`

	wf := &workflow.Workflow{}
	var projectID sql.NullString
	err := d.db.QueryRow(rebind(query), id).Scan(
		&wf.ID,
		&wf.Name,
		&wf.Description,
		&wf.WorkflowType,
		&wf.IsDefault,
		&projectID,
		&wf.CreatedAt,
		&wf.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	if projectID.Valid {
		wf.ProjectID = projectID.String
	}

	// Load nodes
	nodes, err := d.ListWorkflowNodes(id)
	if err != nil {
		return nil, err
	}
	wf.Nodes = nodes

	// Load edges
	edges, err := d.ListWorkflowEdges(id)
	if err != nil {
		return nil, err
	}
	wf.Edges = edges

	return wf, nil
}

// ListWorkflows retrieves workflows, optionally filtered by type or project
func (d *Database) ListWorkflows(workflowType, projectID string) ([]*workflow.Workflow, error) {
	query := `
		SELECT id, name, description, workflow_type, is_default, project_id, created_at, updated_at
		FROM workflows
		WHERE 1=1
	`
	args := []interface{}{}

	if workflowType != "" {
		query += " AND workflow_type = ?"
		args = append(args, workflowType)
	}

	if projectID != "" {
		query += " AND (project_id = ? OR project_id IS NULL)"
		args = append(args, projectID)
	}

	query += " ORDER BY is_default DESC, created_at DESC"

	rows, err := d.db.Query(rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workflows []*workflow.Workflow
	for rows.Next() {
		wf := &workflow.Workflow{}
		var projID sql.NullString
		err := rows.Scan(
			&wf.ID,
			&wf.Name,
			&wf.Description,
			&wf.WorkflowType,
			&wf.IsDefault,
			&projID,
			&wf.CreatedAt,
			&wf.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if projID.Valid {
			wf.ProjectID = projID.String
		}

		// Load nodes for this workflow
		nodes, err := d.ListWorkflowNodes(wf.ID)
		if err == nil {
			wf.Nodes = nodes
		}

		// Load edges for this workflow
		edges, err := d.ListWorkflowEdges(wf.ID)
		if err == nil {
			wf.Edges = edges
		}

		workflows = append(workflows, wf)
	}

	return workflows, nil
}

// UpsertWorkflowNode inserts or updates a workflow node
func (d *Database) UpsertWorkflowNode(node *workflow.WorkflowNode) error {
	if node == nil {
		return fmt.Errorf("workflow node cannot be nil")
	}
	if node.CreatedAt.IsZero() {
		node.CreatedAt = time.Now()
	}

	metadataJSON := ""
	if node.Metadata != nil {
		b, err := json.Marshal(node.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal node metadata: %w", err)
		}
		metadataJSON = string(b)
	}

	query := `
		INSERT INTO workflow_nodes (id, workflow_id, node_key, node_type, role_required, persona_hint, max_attempts, timeout_minutes, instructions, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workflow_id, node_key) DO UPDATE SET
			node_type = excluded.node_type,
			role_required = excluded.role_required,
			persona_hint = excluded.persona_hint,
			max_attempts = excluded.max_attempts,
			timeout_minutes = excluded.timeout_minutes,
			instructions = excluded.instructions,
			metadata_json = excluded.metadata_json
	`

	_, err := d.db.Exec(rebind(query),
		node.ID,
		node.WorkflowID,
		node.NodeKey,
		string(node.NodeType),
		node.RoleRequired,
		node.PersonaHint,
		node.MaxAttempts,
		node.TimeoutMinutes,
		node.Instructions,
		metadataJSON,
		node.CreatedAt,
	)
	return err
}

// ListWorkflowNodes retrieves all nodes for a workflow
func (d *Database) ListWorkflowNodes(workflowID string) ([]workflow.WorkflowNode, error) {
	query := `
		SELECT id, workflow_id, node_key, node_type, role_required, persona_hint, max_attempts, timeout_minutes, instructions, metadata_json, created_at
		FROM workflow_nodes
		WHERE workflow_id = ?
		ORDER BY created_at ASC
	`

	rows, err := d.db.Query(rebind(query), workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []workflow.WorkflowNode
	for rows.Next() {
		node := workflow.WorkflowNode{}
		var metadataJSON sql.NullString
		err := rows.Scan(
			&node.ID,
			&node.WorkflowID,
			&node.NodeKey,
			&node.NodeType,
			&node.RoleRequired,
			&node.PersonaHint,
			&node.MaxAttempts,
			&node.TimeoutMinutes,
			&node.Instructions,
			&metadataJSON,
			&node.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if metadataJSON.Valid && metadataJSON.String != "" {
			_ = json.Unmarshal([]byte(metadataJSON.String), &node.Metadata)
		}
		if node.Metadata == nil {
			node.Metadata = map[string]string{}
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// UpsertWorkflowEdge inserts or updates a workflow edge
func (d *Database) UpsertWorkflowEdge(edge *workflow.WorkflowEdge) error {
	if edge == nil {
		return fmt.Errorf("workflow edge cannot be nil")
	}
	if edge.CreatedAt.IsZero() {
		edge.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO workflow_edges (id, workflow_id, from_node_key, to_node_key, condition, priority, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			from_node_key = excluded.from_node_key,
			to_node_key = excluded.to_node_key,
			condition = excluded.condition,
			priority = excluded.priority
	`

	var fromNodeKey, toNodeKey interface{}
	if edge.FromNodeKey != "" {
		fromNodeKey = edge.FromNodeKey
	}
	if edge.ToNodeKey != "" {
		toNodeKey = edge.ToNodeKey
	}

	_, err := d.db.Exec(rebind(query),
		edge.ID,
		edge.WorkflowID,
		fromNodeKey,
		toNodeKey,
		string(edge.Condition),
		edge.Priority,
		edge.CreatedAt,
	)
	return err
}

// ListWorkflowEdges retrieves all edges for a workflow
func (d *Database) ListWorkflowEdges(workflowID string) ([]workflow.WorkflowEdge, error) {
	query := `
		SELECT id, workflow_id, from_node_key, to_node_key, condition, priority, created_at
		FROM workflow_edges
		WHERE workflow_id = ?
		ORDER BY priority DESC, created_at ASC
	`

	rows, err := d.db.Query(rebind(query), workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []workflow.WorkflowEdge
	for rows.Next() {
		edge := workflow.WorkflowEdge{}
		var fromNodeKey, toNodeKey sql.NullString
		err := rows.Scan(
			&edge.ID,
			&edge.WorkflowID,
			&fromNodeKey,
			&toNodeKey,
			&edge.Condition,
			&edge.Priority,
			&edge.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if fromNodeKey.Valid {
			edge.FromNodeKey = fromNodeKey.String
		}
		if toNodeKey.Valid {
			edge.ToNodeKey = toNodeKey.String
		}

		edges = append(edges, edge)
	}

	return edges, nil
}

// UpsertWorkflowExecution inserts or updates a workflow execution
func (d *Database) UpsertWorkflowExecution(exec *workflow.WorkflowExecution) error {
	if exec == nil {
		return fmt.Errorf("workflow execution cannot be nil")
	}
	if exec.StartedAt.IsZero() {
		exec.StartedAt = time.Now()
	}
	if exec.LastNodeAt.IsZero() {
		exec.LastNodeAt = time.Now()
	}

	query := `
		INSERT INTO workflow_executions (id, workflow_id, bead_id, project_id, current_node_key, status, cycle_count, node_attempt_count, started_at, completed_at, escalated_at, last_node_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(bead_id) DO UPDATE SET
			current_node_key = excluded.current_node_key,
			status = excluded.status,
			cycle_count = excluded.cycle_count,
			node_attempt_count = excluded.node_attempt_count,
			completed_at = excluded.completed_at,
			escalated_at = excluded.escalated_at,
			last_node_at = excluded.last_node_at
	`

	var currentNodeKey interface{}
	if exec.CurrentNodeKey != "" {
		currentNodeKey = exec.CurrentNodeKey
	}

	_, err := d.db.Exec(rebind(query),
		exec.ID,
		exec.WorkflowID,
		exec.BeadID,
		exec.ProjectID,
		currentNodeKey,
		string(exec.Status),
		exec.CycleCount,
		exec.NodeAttemptCount,
		exec.StartedAt,
		exec.CompletedAt,
		exec.EscalatedAt,
		exec.LastNodeAt,
	)
	return err
}

// GetWorkflowExecution retrieves a workflow execution by ID
func (d *Database) GetWorkflowExecution(id string) (*workflow.WorkflowExecution, error) {
	query := `
		SELECT id, workflow_id, bead_id, project_id, current_node_key, status, cycle_count, node_attempt_count, started_at, completed_at, escalated_at, last_node_at
		FROM workflow_executions
		WHERE id = ?
	`

	exec := &workflow.WorkflowExecution{}
	var currentNodeKey sql.NullString
	var completedAt, escalatedAt sql.NullTime
	err := d.db.QueryRow(rebind(query), id).Scan(
		&exec.ID,
		&exec.WorkflowID,
		&exec.BeadID,
		&exec.ProjectID,
		&currentNodeKey,
		&exec.Status,
		&exec.CycleCount,
		&exec.NodeAttemptCount,
		&exec.StartedAt,
		&completedAt,
		&escalatedAt,
		&exec.LastNodeAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow execution not found: %s", id)
	}
	if err != nil {
		return nil, err
	}

	if currentNodeKey.Valid {
		exec.CurrentNodeKey = currentNodeKey.String
	}
	if completedAt.Valid {
		exec.CompletedAt = &completedAt.Time
	}
	if escalatedAt.Valid {
		exec.EscalatedAt = &escalatedAt.Time
	}

	return exec, nil
}

// GetWorkflowExecutionByBeadID retrieves a workflow execution by bead ID
func (d *Database) GetWorkflowExecutionByBeadID(beadID string) (*workflow.WorkflowExecution, error) {
	query := `
		SELECT id, workflow_id, bead_id, project_id, current_node_key, status, cycle_count, node_attempt_count, started_at, completed_at, escalated_at, last_node_at
		FROM workflow_executions
		WHERE bead_id = ?
	`

	exec := &workflow.WorkflowExecution{}
	var currentNodeKey sql.NullString
	var completedAt, escalatedAt sql.NullTime
	err := d.db.QueryRow(rebind(query), beadID).Scan(
		&exec.ID,
		&exec.WorkflowID,
		&exec.BeadID,
		&exec.ProjectID,
		&currentNodeKey,
		&exec.Status,
		&exec.CycleCount,
		&exec.NodeAttemptCount,
		&exec.StartedAt,
		&completedAt,
		&escalatedAt,
		&exec.LastNodeAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil // Not an error - just no execution for this bead yet
	}
	if err != nil {
		return nil, err
	}

	if currentNodeKey.Valid {
		exec.CurrentNodeKey = currentNodeKey.String
	}
	if completedAt.Valid {
		exec.CompletedAt = &completedAt.Time
	}
	if escalatedAt.Valid {
		exec.EscalatedAt = &escalatedAt.Time
	}

	return exec, nil
}

// DeleteWorkflowExecutionByBeadID removes workflow executions for a bead,
// allowing a fresh workflow to be started (e.g., on redispatch).
func (d *Database) DeleteWorkflowExecutionByBeadID(beadID string) error {
	// Delete history first (foreign key)
	_, _ = d.db.Exec(rebind("DELETE FROM workflow_execution_history WHERE execution_id IN (SELECT id FROM workflow_executions WHERE bead_id = ?)"), beadID)
	_, err := d.db.Exec(rebind("DELETE FROM workflow_executions WHERE bead_id = ?"), beadID)
	return err
}

// ListActiveWorkflowExecutions returns all executions with status 'active'.
// Used on startup to identify orphaned executions that were interrupted by a restart.
func (d *Database) ListActiveWorkflowExecutions() ([]*workflow.WorkflowExecution, error) {
	rows, err := d.db.Query(rebind(`
		SELECT id, workflow_id, bead_id, project_id,
		       current_node_key, status, cycle_count, node_attempt_count,
		       started_at, completed_at, escalated_at, last_node_at
		FROM workflow_executions
		WHERE status = ?
		ORDER BY last_node_at ASC
	`), string(workflow.ExecutionStatusActive))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*workflow.WorkflowExecution
	for rows.Next() {
		exec := &workflow.WorkflowExecution{}
		if err := rows.Scan(
			&exec.ID, &exec.WorkflowID, &exec.BeadID, &exec.ProjectID,
			&exec.CurrentNodeKey, &exec.Status, &exec.CycleCount, &exec.NodeAttemptCount,
			&exec.StartedAt, &exec.CompletedAt, &exec.EscalatedAt, &exec.LastNodeAt,
		); err != nil {
			return nil, err
		}
		results = append(results, exec)
	}
	return results, rows.Err()
}

// ResetOrphanedWorkflowExecutions finds all active executions and resets them to
// 'blocked' so the dispatcher can re-pick them up after a server restart.
// Returns the number of executions reset.
func (d *Database) ResetOrphanedWorkflowExecutions() (int, error) {
	result, err := d.db.Exec(rebind(`
		UPDATE workflow_executions
		SET status = ?, last_node_at = ?
		WHERE status = ?
	`), string(workflow.ExecutionStatusBlocked), time.Now(), string(workflow.ExecutionStatusActive))
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// InsertWorkflowHistory adds a history entry for a workflow execution
func (d *Database) InsertWorkflowHistory(history *workflow.WorkflowExecutionHistory) error {
	if history == nil {
		return fmt.Errorf("workflow history cannot be nil")
	}
	if history.CreatedAt.IsZero() {
		history.CreatedAt = time.Now()
	}

	query := `
		INSERT INTO workflow_execution_history (id, execution_id, node_key, agent_id, condition, result_data, attempt_number, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := d.db.Exec(rebind(query),
		history.ID,
		history.ExecutionID,
		history.NodeKey,
		history.AgentID,
		string(history.Condition),
		history.ResultData,
		history.AttemptNumber,
		history.CreatedAt,
	)
	return err
}

// ListWorkflowHistory retrieves history entries for a workflow execution
func (d *Database) ListWorkflowHistory(executionID string) ([]*workflow.WorkflowExecutionHistory, error) {
	query := `
		SELECT id, execution_id, node_key, agent_id, condition, result_data, attempt_number, created_at
		FROM workflow_execution_history
		WHERE execution_id = ?
		ORDER BY created_at ASC
	`

	rows, err := d.db.Query(rebind(query), executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []*workflow.WorkflowExecutionHistory
	for rows.Next() {
		h := &workflow.WorkflowExecutionHistory{}
		var resultData sql.NullString
		err := rows.Scan(
			&h.ID,
			&h.ExecutionID,
			&h.NodeKey,
			&h.AgentID,
			&h.Condition,
			&resultData,
			&h.AttemptNumber,
			&h.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if resultData.Valid {
			h.ResultData = resultData.String
		}

		history = append(history, h)
	}

	return history, nil
}
