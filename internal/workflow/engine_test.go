package workflow

import (
	"testing"
	"time"
)

// TestShouldRedispatch tests the shouldRedispatch helper function
func TestShouldRedispatch(t *testing.T) {
	tests := []struct {
		name     string
		exec     *WorkflowExecution
		node     *WorkflowNode
		expected string
	}{
		{
			name: "task node with active workflow should redispatch",
			exec: &WorkflowExecution{
				Status:           ExecutionStatusActive,
				NodeAttemptCount: 0,
			},
			node: &WorkflowNode{
				NodeType:    NodeTypeTask,
				MaxAttempts: 3,
			},
			expected: "true",
		},
		{
			name: "approval node should not redispatch",
			exec: &WorkflowExecution{
				Status:           ExecutionStatusActive,
				NodeAttemptCount: 0,
			},
			node: &WorkflowNode{
				NodeType:    NodeTypeApproval,
				MaxAttempts: 3,
			},
			expected: "false",
		},
		{
			name: "commit node with active workflow should redispatch",
			exec: &WorkflowExecution{
				Status:           ExecutionStatusActive,
				NodeAttemptCount: 1,
			},
			node: &WorkflowNode{
				NodeType:    NodeTypeCommit,
				MaxAttempts: 3,
			},
			expected: "true",
		},
		{
			name: "verify node with active workflow should redispatch",
			exec: &WorkflowExecution{
				Status:           ExecutionStatusActive,
				NodeAttemptCount: 0,
			},
			node: &WorkflowNode{
				NodeType:    NodeTypeVerify,
				MaxAttempts: 2,
			},
			expected: "true",
		},
		{
			name: "completed workflow should not redispatch",
			exec: &WorkflowExecution{
				Status:           ExecutionStatusCompleted,
				NodeAttemptCount: 0,
			},
			node: &WorkflowNode{
				NodeType:    NodeTypeTask,
				MaxAttempts: 3,
			},
			expected: "false",
		},
		{
			name: "escalated workflow should not redispatch",
			exec: &WorkflowExecution{
				Status:           ExecutionStatusEscalated,
				NodeAttemptCount: 0,
			},
			node: &WorkflowNode{
				NodeType:    NodeTypeTask,
				MaxAttempts: 3,
			},
			expected: "false",
		},
		{
			name: "failed workflow should not redispatch",
			exec: &WorkflowExecution{
				Status:           ExecutionStatusFailed,
				NodeAttemptCount: 0,
			},
			node: &WorkflowNode{
				NodeType:    NodeTypeTask,
				MaxAttempts: 3,
			},
			expected: "false",
		},
		{
			name: "blocked workflow should not redispatch",
			exec: &WorkflowExecution{
				Status:           ExecutionStatusBlocked,
				NodeAttemptCount: 0,
			},
			node: &WorkflowNode{
				NodeType:    NodeTypeTask,
				MaxAttempts: 3,
			},
			expected: "false",
		},
		{
			name: "node at max attempts should not redispatch",
			exec: &WorkflowExecution{
				Status:           ExecutionStatusActive,
				NodeAttemptCount: 3,
			},
			node: &WorkflowNode{
				NodeType:    NodeTypeTask,
				MaxAttempts: 3,
			},
			expected: "false",
		},
		{
			name: "node exceeding max attempts should not redispatch",
			exec: &WorkflowExecution{
				Status:           ExecutionStatusActive,
				NodeAttemptCount: 5,
			},
			node: &WorkflowNode{
				NodeType:    NodeTypeTask,
				MaxAttempts: 3,
			},
			expected: "false",
		},
		{
			name: "task node at attempt 2 of 3 should redispatch",
			exec: &WorkflowExecution{
				Status:           ExecutionStatusActive,
				NodeAttemptCount: 2,
			},
			node: &WorkflowNode{
				NodeType:    NodeTypeTask,
				MaxAttempts: 3,
			},
			expected: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldRedispatch(tt.exec, tt.node)
			if got != tt.expected {
				t.Errorf("shouldRedispatch() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestShouldRedispatch_EdgeCases tests edge cases
func TestShouldRedispatch_EdgeCases(t *testing.T) {
	t.Run("nil execution", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil execution, got none")
			}
		}()
		shouldRedispatch(nil, &WorkflowNode{NodeType: NodeTypeTask, MaxAttempts: 3})
	})

	t.Run("nil node", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil node, got none")
			}
		}()
		shouldRedispatch(&WorkflowExecution{Status: ExecutionStatusActive}, nil)
	})

	t.Run("node with zero max attempts", func(t *testing.T) {
		exec := &WorkflowExecution{
			Status:           ExecutionStatusActive,
			NodeAttemptCount: 0,
		}
		node := &WorkflowNode{
			NodeType:    NodeTypeTask,
			MaxAttempts: 0,
		}
		// Node at attempt 0 with max 0 means >= max, should not redispatch
		got := shouldRedispatch(exec, node)
		if got != "false" {
			t.Errorf("shouldRedispatch() with max_attempts=0 = %q, want %q", got, "false")
		}
	})
}

// TestShouldRedispatch_AllNodeTypes tests all node types explicitly
func TestShouldRedispatch_AllNodeTypes(t *testing.T) {
	exec := &WorkflowExecution{
		Status:           ExecutionStatusActive,
		NodeAttemptCount: 0,
	}

	nodeTypes := []struct {
		nodeType NodeType
		expected string
	}{
		{NodeTypeTask, "true"},
		{NodeTypeApproval, "false"},
		{NodeTypeCommit, "true"},
		{NodeTypeVerify, "true"},
	}

	for _, nt := range nodeTypes {
		t.Run(string(nt.nodeType), func(t *testing.T) {
			node := &WorkflowNode{
				NodeType:    nt.nodeType,
				MaxAttempts: 3,
			}
			got := shouldRedispatch(exec, node)
			if got != nt.expected {
				t.Errorf("shouldRedispatch() for %s = %q, want %q", nt.nodeType, got, nt.expected)
			}
		})
	}
}

// TestShouldRedispatch_AllExecutionStatuses tests all execution statuses
func TestShouldRedispatch_AllExecutionStatuses(t *testing.T) {
	node := &WorkflowNode{
		NodeType:    NodeTypeTask,
		MaxAttempts: 3,
	}

	statuses := []struct {
		status   ExecutionStatus
		expected string
	}{
		{ExecutionStatusActive, "true"},
		{ExecutionStatusBlocked, "false"},
		{ExecutionStatusCompleted, "false"},
		{ExecutionStatusFailed, "false"},
		{ExecutionStatusEscalated, "false"},
	}

	for _, st := range statuses {
		t.Run(string(st.status), func(t *testing.T) {
			exec := &WorkflowExecution{
				Status:           st.status,
				NodeAttemptCount: 0,
			}
			got := shouldRedispatch(exec, node)
			if got != st.expected {
				t.Errorf("shouldRedispatch() for status %s = %q, want %q", st.status, got, st.expected)
			}
		})
	}
}

// Mock implementations for testing

type mockDatabase struct {
	workflows      map[string]*Workflow
	executions     map[string]*WorkflowExecution
	history        map[string][]*WorkflowExecutionHistory
	beadExecutions map[string]*WorkflowExecution
}

func newMockDatabase() *mockDatabase {
	return &mockDatabase{
		workflows:      make(map[string]*Workflow),
		executions:     make(map[string]*WorkflowExecution),
		history:        make(map[string][]*WorkflowExecutionHistory),
		beadExecutions: make(map[string]*WorkflowExecution),
	}
}

func (m *mockDatabase) GetWorkflow(id string) (*Workflow, error) {
	wf, ok := m.workflows[id]
	if !ok {
		return nil, &workflowError{msg: "workflow not found"}
	}
	return wf, nil
}

func (m *mockDatabase) ListWorkflows(workflowType, projectID string) ([]*Workflow, error) {
	var result []*Workflow
	for _, wf := range m.workflows {
		if workflowType != "" && wf.WorkflowType != workflowType {
			continue
		}
		if projectID != "" && wf.ProjectID != projectID {
			continue
		}
		result = append(result, wf)
	}
	return result, nil
}

func (m *mockDatabase) UpsertWorkflow(wf *Workflow) error {
	m.workflows[wf.ID] = wf
	return nil
}

func (m *mockDatabase) UpsertWorkflowNode(node *WorkflowNode) error {
	return nil
}

func (m *mockDatabase) UpsertWorkflowEdge(edge *WorkflowEdge) error {
	return nil
}

func (m *mockDatabase) UpsertWorkflowExecution(exec *WorkflowExecution) error {
	m.executions[exec.ID] = exec
	m.beadExecutions[exec.BeadID] = exec
	return nil
}

func (m *mockDatabase) GetWorkflowExecution(id string) (*WorkflowExecution, error) {
	exec, ok := m.executions[id]
	if !ok {
		return nil, &workflowError{msg: "execution not found"}
	}
	return exec, nil
}

func (m *mockDatabase) GetWorkflowExecutionByBeadID(beadID string) (*WorkflowExecution, error) {
	exec, ok := m.beadExecutions[beadID]
	if !ok {
		return nil, nil // No execution found, but not an error
	}
	return exec, nil
}

func (m *mockDatabase) InsertWorkflowHistory(history *WorkflowExecutionHistory) error {
	m.history[history.ExecutionID] = append(m.history[history.ExecutionID], history)
	return nil
}

func (m *mockDatabase) ListWorkflowHistory(executionID string) ([]*WorkflowExecutionHistory, error) {
	return m.history[executionID], nil
}

func (m *mockDatabase) DeleteWorkflowExecutionByBeadID(beadID string) error {
	for id, exec := range m.executions {
		if exec.BeadID == beadID {
			delete(m.executions, id)
		}
	}
	return nil
}

func (m *mockDatabase) ListActiveWorkflowExecutions() ([]*WorkflowExecution, error) {
	var result []*WorkflowExecution
	for _, exec := range m.executions {
		if exec.Status == ExecutionStatusActive {
			e := *exec
			result = append(result, &e)
		}
	}
	return result, nil
}

func (m *mockDatabase) ResetOrphanedWorkflowExecutions() (int, error) {
	n := 0
	for _, exec := range m.executions {
		if exec.Status == ExecutionStatusActive {
			exec.Status = ExecutionStatusBlocked
			n++
		}
	}
	return n, nil
}

type mockBeadManager struct {
	beads map[string]map[string]interface{}
}

func newMockBeadManager() *mockBeadManager {
	return &mockBeadManager{
		beads: make(map[string]map[string]interface{}),
	}
}

func (m *mockBeadManager) UpdateBead(beadID string, updates map[string]interface{}) error {
	if m.beads[beadID] == nil {
		m.beads[beadID] = make(map[string]interface{})
	}
	for k, v := range updates {
		m.beads[beadID][k] = v
	}
	return nil
}

func (m *mockBeadManager) CreateBead(bead interface{}) error {
	return nil
}

type workflowError struct {
	msg string
}

func (e *workflowError) Error() string {
	return e.msg
}

// TestAdvanceWorkflow_SetsRedispatchFlag tests that AdvanceWorkflow sets the flag correctly
func TestAdvanceWorkflow_SetsRedispatchFlag(t *testing.T) {
	db := newMockDatabase()
	beads := newMockBeadManager()
	engine := NewEngine(db, beads)

	// Create a simple workflow: start -> task1 -> task2 -> end
	workflow := &Workflow{
		ID:           "wf-test",
		Name:         "Test Workflow",
		WorkflowType: "test",
		Nodes: []WorkflowNode{
			{
				ID:          "node1",
				WorkflowID:  "wf-test",
				NodeKey:     "task1",
				NodeType:    NodeTypeTask,
				MaxAttempts: 3,
			},
			{
				ID:          "node2",
				WorkflowID:  "wf-test",
				NodeKey:     "task2",
				NodeType:    NodeTypeTask,
				MaxAttempts: 3,
			},
		},
		Edges: []WorkflowEdge{
			{
				ID:          "edge1",
				WorkflowID:  "wf-test",
				FromNodeKey: "",
				ToNodeKey:   "task1",
				Condition:   EdgeConditionSuccess,
				Priority:    100,
			},
			{
				ID:          "edge2",
				WorkflowID:  "wf-test",
				FromNodeKey: "task1",
				ToNodeKey:   "task2",
				Condition:   EdgeConditionSuccess,
				Priority:    100,
			},
			{
				ID:          "edge3",
				WorkflowID:  "wf-test",
				FromNodeKey: "task2",
				ToNodeKey:   "",
				Condition:   EdgeConditionSuccess,
				Priority:    100,
			},
		},
	}
	db.workflows["wf-test"] = workflow

	// Create execution
	exec := &WorkflowExecution{
		ID:               "exec-1",
		WorkflowID:       "wf-test",
		BeadID:           "bead-1",
		CurrentNodeKey:   "",
		Status:           ExecutionStatusActive,
		CycleCount:       0,
		NodeAttemptCount: 0,
		StartedAt:        time.Now(),
		LastNodeAt:       time.Now(),
	}
	db.executions["exec-1"] = exec
	db.beadExecutions["bead-1"] = exec

	// Advance to task1 (should set redispatch_requested = "true")
	err := engine.AdvanceWorkflow("exec-1", EdgeConditionSuccess, "agent-1", nil)
	if err != nil {
		t.Fatalf("AdvanceWorkflow() error = %v", err)
	}

	// Check bead context was updated with redispatch flag
	beadUpdates := beads.beads["bead-1"]
	if beadUpdates == nil {
		t.Fatal("Expected bead updates, got nil")
	}

	ctx, ok := beadUpdates["context"].(map[string]string)
	if !ok {
		t.Fatalf("Expected context to be map[string]string, got %T", beadUpdates["context"])
	}

	if ctx["redispatch_requested"] != "true" {
		t.Errorf("Expected redispatch_requested = %q, got %q", "true", ctx["redispatch_requested"])
	}

	if ctx["workflow_node"] != "task1" {
		t.Errorf("Expected workflow_node = %q, got %q", "task1", ctx["workflow_node"])
	}

	// Advance to task2 (should still set redispatch_requested = "true")
	exec.CurrentNodeKey = "task1"
	err = engine.AdvanceWorkflow("exec-1", EdgeConditionSuccess, "agent-1", nil)
	if err != nil {
		t.Fatalf("AdvanceWorkflow() error = %v", err)
	}

	ctx = beads.beads["bead-1"]["context"].(map[string]string)
	if ctx["redispatch_requested"] != "true" {
		t.Errorf("Expected redispatch_requested = %q on task2, got %q", "true", ctx["redispatch_requested"])
	}

	// Advance to end (should set redispatch_requested = "false" and status = completed)
	exec.CurrentNodeKey = "task2"
	err = engine.AdvanceWorkflow("exec-1", EdgeConditionSuccess, "agent-1", nil)
	if err != nil {
		t.Fatalf("AdvanceWorkflow() error = %v", err)
	}

	ctx = beads.beads["bead-1"]["context"].(map[string]string)
	if ctx["redispatch_requested"] != "false" {
		t.Errorf("Expected redispatch_requested = %q on completion, got %q", "false", ctx["redispatch_requested"])
	}

	if ctx["workflow_status"] != string(ExecutionStatusCompleted) {
		t.Errorf("Expected workflow_status = %q, got %q", ExecutionStatusCompleted, ctx["workflow_status"])
	}
}

// TestStartWorkflow_Validation tests input validation for StartWorkflow
func TestStartWorkflow_Validation(t *testing.T) {
	tests := []struct {
		name        string
		beadID      string
		workflowID  string
		projectID   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty beadID",
			beadID:      "",
			workflowID:  "wf-test",
			projectID:   "proj-1",
			expectError: true,
			errorMsg:    "beadID cannot be empty",
		},
		{
			name:        "empty workflowID",
			beadID:      "bead-1",
			workflowID:  "",
			projectID:   "proj-1",
			expectError: true,
			errorMsg:    "workflowID cannot be empty",
		},
		{
			name:        "empty projectID",
			beadID:      "bead-1",
			workflowID:  "wf-test",
			projectID:   "",
			expectError: true,
			errorMsg:    "projectID cannot be empty",
		},
		{
			name:        "all parameters empty",
			beadID:      "",
			workflowID:  "",
			projectID:   "",
			expectError: true,
			errorMsg:    "beadID cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newMockDatabase()
			beads := newMockBeadManager()
			engine := NewEngine(db, beads)

			exec, err := engine.StartWorkflow(tt.beadID, tt.workflowID, tt.projectID)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error %q, got nil", tt.errorMsg)
				} else if err.Error() != tt.errorMsg {
					t.Errorf("Expected error %q, got %q", tt.errorMsg, err.Error())
				}
				if exec != nil {
					t.Errorf("Expected nil execution, got %v", exec)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got %v", err)
				}
				if exec == nil {
					t.Error("Expected execution, got nil")
				}
			}
		})
	}
}

// TestStartWorkflow_Success tests successful workflow start
func TestStartWorkflow_Success(t *testing.T) {
	db := newMockDatabase()
	beads := newMockBeadManager()
	engine := NewEngine(db, beads)

	// Create test workflow
	workflow := &Workflow{
		ID:           "wf-test",
		Name:         "Test Workflow",
		WorkflowType: "test",
		ProjectID:    "proj-1",
	}
	db.workflows["wf-test"] = workflow

	// Start workflow
	exec, err := engine.StartWorkflow("bead-1", "wf-test", "proj-1")
	if err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}

	if exec == nil {
		t.Fatal("Expected execution, got nil")
	}

	if exec.BeadID != "bead-1" {
		t.Errorf("Expected BeadID = %q, got %q", "bead-1", exec.BeadID)
	}

	if exec.WorkflowID != "wf-test" {
		t.Errorf("Expected WorkflowID = %q, got %q", "wf-test", exec.WorkflowID)
	}

	if exec.ProjectID != "proj-1" {
		t.Errorf("Expected ProjectID = %q, got %q", "proj-1", exec.ProjectID)
	}

	if exec.Status != ExecutionStatusActive {
		t.Errorf("Expected Status = %q, got %q", ExecutionStatusActive, exec.Status)
	}

	// Check bead was updated
	beadUpdates := beads.beads["bead-1"]
	if beadUpdates == nil {
		t.Fatal("Expected bead updates, got nil")
	}
}

// TestStartWorkflow_AlreadyExists tests that existing workflow is returned
func TestStartWorkflow_AlreadyExists(t *testing.T) {
	db := newMockDatabase()
	beads := newMockBeadManager()
	engine := NewEngine(db, beads)

	// Create existing execution
	existing := &WorkflowExecution{
		ID:         "exec-existing",
		WorkflowID: "wf-test",
		BeadID:     "bead-1",
		ProjectID:  "proj-1",
		Status:     ExecutionStatusActive,
	}
	db.beadExecutions["bead-1"] = existing

	// Try to start workflow again
	exec, err := engine.StartWorkflow("bead-1", "wf-test", "proj-1")
	if err != nil {
		t.Fatalf("StartWorkflow() error = %v", err)
	}

	if exec.ID != "exec-existing" {
		t.Errorf("Expected existing execution ID = %q, got %q", "exec-existing", exec.ID)
	}
}

// TestAdvanceWorkflow_ApprovalNode tests that approval nodes don't get redispatch flag
func TestAdvanceWorkflow_ApprovalNode(t *testing.T) {
	db := newMockDatabase()
	beads := newMockBeadManager()
	engine := NewEngine(db, beads)

	// Create workflow with approval node
	workflow := &Workflow{
		ID:           "wf-test",
		Name:         "Test Workflow",
		WorkflowType: "test",
		Nodes: []WorkflowNode{
			{
				ID:          "node1",
				WorkflowID:  "wf-test",
				NodeKey:     "approve",
				NodeType:    NodeTypeApproval,
				MaxAttempts: 1,
			},
		},
		Edges: []WorkflowEdge{
			{
				ID:          "edge1",
				WorkflowID:  "wf-test",
				FromNodeKey: "",
				ToNodeKey:   "approve",
				Condition:   EdgeConditionSuccess,
				Priority:    100,
			},
		},
	}
	db.workflows["wf-test"] = workflow

	exec := &WorkflowExecution{
		ID:               "exec-1",
		WorkflowID:       "wf-test",
		BeadID:           "bead-1",
		CurrentNodeKey:   "",
		Status:           ExecutionStatusActive,
		CycleCount:       0,
		NodeAttemptCount: 0,
		StartedAt:        time.Now(),
		LastNodeAt:       time.Now(),
	}
	db.executions["exec-1"] = exec
	db.beadExecutions["bead-1"] = exec

	// Advance to approval node
	err := engine.AdvanceWorkflow("exec-1", EdgeConditionSuccess, "agent-1", nil)
	if err != nil {
		t.Fatalf("AdvanceWorkflow() error = %v", err)
	}

	// Check that redispatch_requested is "false" for approval node
	ctx := beads.beads["bead-1"]["context"].(map[string]string)
	if ctx["redispatch_requested"] != "false" {
		t.Errorf("Expected redispatch_requested = %q for approval node, got %q", "false", ctx["redispatch_requested"])
	}
}
