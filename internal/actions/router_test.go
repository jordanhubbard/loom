package actions

import (
	"context"
	"testing"
)

// mockTestRunner implements the TestRunner interface for testing
type mockTestRunner struct {
	runFunc func(ctx context.Context, projectPath, testPattern, framework string, timeoutSeconds int) (map[string]interface{}, error)
}

func (m *mockTestRunner) Run(ctx context.Context, projectPath, testPattern, framework string, timeoutSeconds int) (map[string]interface{}, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, projectPath, testPattern, framework, timeoutSeconds)
	}
	// Default successful test result
	return map[string]interface{}{
		"framework": "go",
		"success":   true,
		"exit_code": 0,
		"summary": map[string]interface{}{
			"total":  10,
			"passed": 10,
			"failed": 0,
		},
	}, nil
}

func TestRouter_ExecuteAction_RunTests_Success(t *testing.T) {
	mock := &mockTestRunner{
		runFunc: func(ctx context.Context, projectPath, testPattern, framework string, timeoutSeconds int) (map[string]interface{}, error) {
			return map[string]interface{}{
				"framework": "go",
				"success":   true,
				"exit_code": 0,
				"summary": map[string]interface{}{
					"total":  5,
					"passed": 5,
					"failed": 0,
				},
				"raw_output": "ok\tgithub.com/test/pkg\t0.123s",
			}, nil
		},
	}

	router := &Router{
		Tests: mock,
	}

	action := Action{
		Type:           ActionRunTests,
		TestPattern:    "TestFoo",
		Framework:      "go",
		TimeoutSeconds: 300,
	}

	actx := ActionContext{
		AgentID:   "agent-123",
		BeadID:    "bead-456",
		ProjectID: "proj-789",
	}

	result := router.executeAction(context.Background(), action, actx)

	if result.ActionType != ActionRunTests {
		t.Errorf("Expected action type %s, got %s", ActionRunTests, result.ActionType)
	}

	if result.Status != "executed" {
		t.Errorf("Expected status 'executed', got %s", result.Status)
	}

	if result.Message != "tests executed" {
		t.Errorf("Expected message 'tests executed', got %s", result.Message)
	}

	if result.Metadata == nil {
		t.Fatal("Expected metadata to be present")
	}

	// Check metadata fields
	if success, ok := result.Metadata["success"].(bool); !ok || !success {
		t.Error("Expected success to be true")
	}

	if exitCode, ok := result.Metadata["exit_code"].(int); !ok || exitCode != 0 {
		t.Errorf("Expected exit_code 0, got %v", exitCode)
	}

	if framework, ok := result.Metadata["framework"].(string); !ok || framework != "go" {
		t.Errorf("Expected framework 'go', got %v", framework)
	}
}

func TestRouter_ExecuteAction_RunTests_Failure(t *testing.T) {
	mock := &mockTestRunner{
		runFunc: func(ctx context.Context, projectPath, testPattern, framework string, timeoutSeconds int) (map[string]interface{}, error) {
			return map[string]interface{}{
				"framework": "go",
				"success":   false,
				"exit_code": 1,
				"summary": map[string]interface{}{
					"total":  5,
					"passed": 3,
					"failed": 2,
				},
				"tests": []map[string]interface{}{
					{
						"name":   "TestCalculate",
						"status": "fail",
						"error":  "Expected 100, got 99",
					},
					{
						"name":   "TestValidate",
						"status": "fail",
						"error":  "nil pointer dereference",
					},
				},
			}, nil
		},
	}

	router := &Router{
		Tests: mock,
	}

	action := Action{
		Type: ActionRunTests,
	}

	actx := ActionContext{
		AgentID:   "agent-123",
		BeadID:    "bead-456",
		ProjectID: "proj-789",
	}

	result := router.executeAction(context.Background(), action, actx)

	if result.Status != "executed" {
		t.Errorf("Expected status 'executed', got %s", result.Status)
	}

	// Check that failure information is present
	if success, ok := result.Metadata["success"].(bool); !ok || success {
		t.Error("Expected success to be false")
	}

	if exitCode, ok := result.Metadata["exit_code"].(int); !ok || exitCode != 1 {
		t.Errorf("Expected exit_code 1, got %v", exitCode)
	}

	// Check summary
	summary, ok := result.Metadata["summary"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected summary to be present")
	}

	if failed, ok := summary["failed"].(int); !ok || failed != 2 {
		t.Errorf("Expected 2 failed tests, got %v", failed)
	}

	// Check test cases
	tests, ok := result.Metadata["tests"].([]map[string]interface{})
	if !ok {
		t.Fatal("Expected tests array to be present")
	}

	if len(tests) != 2 {
		t.Errorf("Expected 2 test cases, got %d", len(tests))
	}
}

func TestRouter_ExecuteAction_RunTests_NoRunner(t *testing.T) {
	router := &Router{
		Tests: nil, // No test runner configured
	}

	action := Action{
		Type: ActionRunTests,
	}

	actx := ActionContext{
		AgentID:   "agent-123",
		BeadID:    "bead-456",
		ProjectID: "proj-789",
	}

	result := router.executeAction(context.Background(), action, actx)

	if result.Status != "error" {
		t.Errorf("Expected status 'error', got %s", result.Status)
	}

	if result.Message != "test runner not configured" {
		t.Errorf("Expected error message about test runner, got: %s", result.Message)
	}
}

func TestRouter_ExecuteAction_RunTests_MinimalParams(t *testing.T) {
	callCount := 0
	mock := &mockTestRunner{
		runFunc: func(ctx context.Context, projectPath, testPattern, framework string, timeoutSeconds int) (map[string]interface{}, error) {
			callCount++
			// Verify parameters
			if projectPath == "" {
				t.Error("Expected projectPath to be set")
			}
			if testPattern != "" {
				t.Errorf("Expected empty testPattern, got %s", testPattern)
			}
			if framework != "" {
				t.Errorf("Expected empty framework, got %s", framework)
			}
			if timeoutSeconds != 0 {
				t.Errorf("Expected 0 timeoutSeconds, got %d", timeoutSeconds)
			}

			return map[string]interface{}{
				"framework": "go",
				"success":   true,
			}, nil
		},
	}

	router := &Router{
		Tests: mock,
	}

	action := Action{
		Type: ActionRunTests,
		// No optional parameters specified
	}

	actx := ActionContext{
		AgentID:   "agent-123",
		BeadID:    "bead-456",
		ProjectID: "proj-789",
	}

	result := router.executeAction(context.Background(), action, actx)

	if result.Status != "executed" {
		t.Errorf("Expected status 'executed', got %s: %s", result.Status, result.Message)
	}

	if callCount != 1 {
		t.Errorf("Expected Run to be called once, got %d calls", callCount)
	}
}

func TestRouter_Execute_MultipleRunTests(t *testing.T) {
	runCount := 0
	mock := &mockTestRunner{
		runFunc: func(ctx context.Context, projectPath, testPattern, framework string, timeoutSeconds int) (map[string]interface{}, error) {
			runCount++
			return map[string]interface{}{
				"framework":   "go",
				"success":     true,
				"testPattern": testPattern,
			}, nil
		},
	}

	router := &Router{
		Tests: mock,
	}

	env := &ActionEnvelope{
		Actions: []Action{
			{
				Type:        ActionRunTests,
				TestPattern: "TestUnit",
			},
			{
				Type:        ActionRunTests,
				TestPattern: "TestIntegration",
			},
		},
	}

	actx := ActionContext{
		AgentID:   "agent-123",
		BeadID:    "bead-456",
		ProjectID: "proj-789",
	}

	results, err := router.Execute(context.Background(), env, actx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	if runCount != 2 {
		t.Errorf("Expected Run to be called twice, got %d calls", runCount)
	}

	for i, result := range results {
		if result.Status != "executed" {
			t.Errorf("Result %d: expected status 'executed', got %s", i, result.Status)
		}
	}
}
