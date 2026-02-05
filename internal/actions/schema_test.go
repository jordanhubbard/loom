package actions

import (
	"encoding/json"
	"testing"
)

func TestActionRunTests_Validation(t *testing.T) {
	tests := []struct {
		name    string
		action  Action
		wantErr bool
	}{
		{
			name: "Valid with all fields",
			action: Action{
				Type:           ActionRunTests,
				TestPattern:    "TestFoo",
				Framework:      "go",
				TimeoutSeconds: 300,
			},
			wantErr: false,
		},
		{
			name: "Valid with no fields (all optional)",
			action: Action{
				Type: ActionRunTests,
			},
			wantErr: false,
		},
		{
			name: "Valid with only pattern",
			action: Action{
				Type:        ActionRunTests,
				TestPattern: "TestDatabase",
			},
			wantErr: false,
		},
		{
			name: "Valid with only framework",
			action: Action{
				Type:      ActionRunTests,
				Framework: "jest",
			},
			wantErr: false,
		},
		{
			name: "Valid with only timeout",
			action: Action{
				Type:           ActionRunTests,
				TimeoutSeconds: 600,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAction(tt.action)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAction() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestActionRunTests_JSONDecoding(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(*testing.T, *ActionEnvelope)
	}{
		{
			name: "Run tests with all parameters",
			json: `{
				"actions": [{
					"type": "run_tests",
					"test_pattern": "TestFoo",
					"framework": "go",
					"timeout_seconds": 300
				}]
			}`,
			wantErr: false,
			check: func(t *testing.T, env *ActionEnvelope) {
				if len(env.Actions) != 1 {
					t.Fatal("Expected 1 action")
				}
				action := env.Actions[0]
				if action.Type != ActionRunTests {
					t.Errorf("Expected type %s, got %s", ActionRunTests, action.Type)
				}
				if action.TestPattern != "TestFoo" {
					t.Errorf("Expected pattern TestFoo, got %s", action.TestPattern)
				}
				if action.Framework != "go" {
					t.Errorf("Expected framework go, got %s", action.Framework)
				}
				if action.TimeoutSeconds != 300 {
					t.Errorf("Expected timeout 300, got %d", action.TimeoutSeconds)
				}
			},
		},
		{
			name: "Run tests with minimal parameters",
			json: `{
				"actions": [{
					"type": "run_tests"
				}]
			}`,
			wantErr: false,
			check: func(t *testing.T, env *ActionEnvelope) {
				if len(env.Actions) != 1 {
					t.Fatal("Expected 1 action")
				}
				action := env.Actions[0]
				if action.Type != ActionRunTests {
					t.Errorf("Expected type %s, got %s", ActionRunTests, action.Type)
				}
			},
		},
		{
			name: "Run tests with only pattern",
			json: `{
				"actions": [{
					"type": "run_tests",
					"test_pattern": "TestDatabase*"
				}]
			}`,
			wantErr: false,
			check: func(t *testing.T, env *ActionEnvelope) {
				if len(env.Actions) != 1 {
					t.Fatal("Expected 1 action")
				}
				action := env.Actions[0]
				if action.TestPattern != "TestDatabase*" {
					t.Errorf("Expected pattern TestDatabase*, got %s", action.TestPattern)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := DecodeStrict([]byte(tt.json))
			if (err != nil) != tt.wantErr {
			_ = env  // May be unused in some tests
				t.Errorf("DecodeStrict() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, env)
			}
		})
	}
}

func TestActionRunTests_JSONEncoding(t *testing.T) {
	action := Action{
		Type:           ActionRunTests,
		TestPattern:    "TestCalculator",
		Framework:      "go",
		TimeoutSeconds: 120,
	}

	data, err := json.Marshal(action)
	if err != nil {
		t.Fatalf("Failed to marshal action: %v", err)
	}

	var decoded Action
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal action: %v", err)
	}

	if decoded.Type != action.Type {
		t.Errorf("Type mismatch: got %s, want %s", decoded.Type, action.Type)
	}
	if decoded.TestPattern != action.TestPattern {
		t.Errorf("TestPattern mismatch: got %s, want %s", decoded.TestPattern, action.TestPattern)
	}
	if decoded.Framework != action.Framework {
		t.Errorf("Framework mismatch: got %s, want %s", decoded.Framework, action.Framework)
	}
	if decoded.TimeoutSeconds != action.TimeoutSeconds {
		t.Errorf("TimeoutSeconds mismatch: got %d, want %d", decoded.TimeoutSeconds, action.TimeoutSeconds)
	}
}

func TestActionRunTests_MultipleActions(t *testing.T) {
	json := `{
		"actions": [
			{
				"type": "run_tests",
				"test_pattern": "TestUnit"
			},
			{
				"type": "run_tests",
				"test_pattern": "TestIntegration",
				"timeout_seconds": 600
			}
		],
		"notes": "Running unit and integration tests"
	}`

	env, err := DecodeStrict([]byte(json))
	if err != nil {
		t.Fatalf("DecodeStrict() failed: %v", err)
	}

	if len(env.Actions) != 2 {
		t.Fatalf("Expected 2 actions, got %d", len(env.Actions))
	}

	// Check first action
	if env.Actions[0].Type != ActionRunTests {
		t.Errorf("Action 0: expected type %s, got %s", ActionRunTests, env.Actions[0].Type)
	}
	if env.Actions[0].TestPattern != "TestUnit" {
		t.Errorf("Action 0: expected pattern TestUnit, got %s", env.Actions[0].TestPattern)
	}

	// Check second action
	if env.Actions[1].Type != ActionRunTests {
		t.Errorf("Action 1: expected type %s, got %s", ActionRunTests, env.Actions[1].Type)
	}
	if env.Actions[1].TestPattern != "TestIntegration" {
		t.Errorf("Action 1: expected pattern TestIntegration, got %s", env.Actions[1].TestPattern)
	}
	if env.Actions[1].TimeoutSeconds != 600 {
		t.Errorf("Action 1: expected timeout 600, got %d", env.Actions[1].TimeoutSeconds)
	}

	// Check notes
	if env.Notes != "Running unit and integration tests" {
		t.Errorf("Expected notes to match, got: %s", env.Notes)
	}
}

func TestActionRunLinter_Validation(t *testing.T) {
	tests := []struct {
		name    string
		action  Action
		wantErr bool
	}{
		{
			name: "Valid with all fields",
			action: Action{
				Type:           ActionRunLinter,
				Files:          []string{"foo.go", "bar.go"},
				Framework:      "golangci-lint",
				TimeoutSeconds: 300,
			},
			wantErr: false,
		},
		{
			name: "Valid with no fields (all optional)",
			action: Action{
				Type: ActionRunLinter,
			},
			wantErr: false,
		},
		{
			name: "Valid with only files",
			action: Action{
				Type:  ActionRunLinter,
				Files: []string{"src/*.go"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAction(tt.action)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAction() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestActionRunLinter_JSONDecoding(t *testing.T) {
	json := `{
		"actions": [{
			"type": "run_linter",
			"files": ["internal/*.go", "pkg/*.go"],
			"framework": "golangci-lint",
			"timeout_seconds": 300
		}]
	}`

	env, err := DecodeStrict([]byte(json))
	if err != nil {
		t.Fatalf("DecodeStrict() failed: %v", err)
	}

	if len(env.Actions) != 1 {
		t.Fatal("Expected 1 action")
	}

	action := env.Actions[0]
	if action.Type != ActionRunLinter {
		t.Errorf("Expected type %s, got %s", ActionRunLinter, action.Type)
	}
	if len(action.Files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(action.Files))
	}
	if action.Framework != "golangci-lint" {
		t.Errorf("Expected framework golangci-lint, got %s", action.Framework)
	}
	if action.TimeoutSeconds != 300 {
		t.Errorf("Expected timeout 300, got %d", action.TimeoutSeconds)
	}
}

func TestActionBuildProject_Validation(t *testing.T) {
	tests := []struct {
		name    string
		action  Action
		wantErr bool
	}{
		{
			name: "Valid with all fields",
			action: Action{
				Type:           ActionBuildProject,
				BuildTarget:    "myapp",
				BuildCommand:   "go build -o myapp ./cmd/app",
				Framework:      "go",
				TimeoutSeconds: 300,
			},
			wantErr: false,
		},
		{
			name: "Valid with no fields (all optional)",
			action: Action{
				Type: ActionBuildProject,
			},
			wantErr: false,
		},
		{
			name: "Valid with only target",
			action: Action{
				Type:        ActionBuildProject,
				BuildTarget: "output.bin",
			},
			wantErr: false,
		},
		{
			name: "Valid with only framework",
			action: Action{
				Type:      ActionBuildProject,
				Framework: "npm",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAction(tt.action)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAction() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestActionBuildProject_JSONDecoding(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		check   func(*testing.T, *ActionEnvelope)
	}{
		{
			name: "Build with all parameters",
			json: `{
				"actions": [{
					"type": "build_project",
					"build_target": "myapp",
					"build_command": "go build -o myapp ./cmd/app",
					"framework": "go",
					"timeout_seconds": 300
				}]
			}`,
			wantErr: false,
			check: func(t *testing.T, env *ActionEnvelope) {
				if len(env.Actions) != 1 {
					t.Fatal("Expected 1 action")
				}
				action := env.Actions[0]
				if action.Type != ActionBuildProject {
					t.Errorf("Expected type %s, got %s", ActionBuildProject, action.Type)
				}
				if action.BuildTarget != "myapp" {
					t.Errorf("Expected target myapp, got %s", action.BuildTarget)
				}
				if action.BuildCommand != "go build -o myapp ./cmd/app" {
					t.Errorf("Expected custom command, got %s", action.BuildCommand)
				}
				if action.Framework != "go" {
					t.Errorf("Expected framework go, got %s", action.Framework)
				}
				if action.TimeoutSeconds != 300 {
					t.Errorf("Expected timeout 300, got %d", action.TimeoutSeconds)
				}
			},
		},
		{
			name: "Build with minimal parameters",
			json: `{
				"actions": [{
					"type": "build_project"
				}]
			}`,
			wantErr: false,
			check: func(t *testing.T, env *ActionEnvelope) {
				if len(env.Actions) != 1 {
					t.Fatal("Expected 1 action")
				}
				action := env.Actions[0]
				if action.Type != ActionBuildProject {
					t.Errorf("Expected type %s, got %s", ActionBuildProject, action.Type)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := DecodeStrict([]byte(tt.json))
			if (err != nil) != tt.wantErr {
			_ = env  // May be unused in some tests
				t.Errorf("DecodeStrict() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, env)
			}
		})
	}
}
func TestWorkflowActionValidation(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid start_development",
			json: `{"actions": [{"type": "start_development", "workflow": "epcc", "require_reviews": true}]}`,
			wantErr: false,
		},
		{
			name: "start_development missing workflow",
			json: `{"actions": [{"type": "start_development"}]}`,
			wantErr: true,
			errMsg: "requires workflow",
		},
		{
			name: "valid whats_next",
			json: `{"actions": [{"type": "whats_next"}]}`,
			wantErr: false,
		},
		{
			name: "valid proceed_to_phase",
			json: `{"actions": [{"type": "proceed_to_phase", "target_phase": "implementation", "review_state": "performed"}]}`,
			wantErr: false,
		},
		{
			name: "proceed_to_phase missing target_phase",
			json: `{"actions": [{"type": "proceed_to_phase", "review_state": "performed"}]}`,
			wantErr: true,
			errMsg: "requires target_phase",
		},
		{
			name: "proceed_to_phase missing review_state",
			json: `{"actions": [{"type": "proceed_to_phase", "target_phase": "implementation"}]}`,
			wantErr: true,
			errMsg: "requires review_state",
		},
		{
			name: "valid conduct_review",
			json: `{"actions": [{"type": "conduct_review", "target_phase": "design"}]}`,
			wantErr: false,
		},
		{
			name: "conduct_review missing target_phase",
			json: `{"actions": [{"type": "conduct_review"}]}`,
			wantErr: true,
			errMsg: "requires target_phase",
		},
		{
			name: "valid resume_workflow",
			json: `{"actions": [{"type": "resume_workflow"}]}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := DecodeStrict([]byte(tt.json))
			if (err != nil) != tt.wantErr {
			_ = env  // May be unused in some tests
				t.Errorf("DecodeStrict() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !containsStr(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing %q, got %v", tt.errMsg, err)
				}
			}
			if !tt.wantErr && env == nil {
				t.Error("Expected valid envelope, got nil")
			}
		})
	}
}

// Helper function for error message checking
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || findSubstr(s, substr))
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestCreatePRActionValidation(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name: "valid create_pr with all fields",
			json: `{"actions": [{"type": "create_pr", "pr_title": "Feature X", "pr_body": "Description", "pr_base": "main", "branch": "agent/bead-123/feature", "pr_reviewers": ["user1", "user2"]}]}`,
			wantErr: false,
		},
		{
			name: "valid create_pr minimal",
			json: `{"actions": [{"type": "create_pr"}]}`,
			wantErr: false,
		},
		{
			name: "create_pr with empty reviewers",
			json: `{"actions": [{"type": "create_pr", "pr_reviewers": []}]}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := DecodeStrict([]byte(tt.json))
			if (err != nil) != tt.wantErr {
			_ = env  // May be unused in some tests
				t.Errorf("DecodeStrict() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && env == nil {
				t.Error("Expected valid envelope, got nil")
			}
		})
	}
}

func TestCodeNavigationActions(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "find_references with symbol",
			json: `{"actions": [{"type": "find_references", "path": "src/main.go", "symbol": "MyFunction"}]}`,
			wantErr: false,
		},
		{
			name: "find_references with line and column",
			json: `{"actions": [{"type": "find_references", "path": "src/main.go", "line": 10, "column": 5}]}`,
			wantErr: false,
		},
		{
			name: "find_references missing path",
			json: `{"actions": [{"type": "find_references", "symbol": "MyFunction"}]}`,
			wantErr: true,
			errMsg: "requires path",
		},
		{
			name: "find_references missing both symbol and position",
			json: `{"actions": [{"type": "find_references", "path": "src/main.go"}]}`,
			wantErr: true,
			errMsg: "requires either symbol or (line and column)",
		},
		{
			name: "go_to_definition with symbol",
			json: `{"actions": [{"type": "go_to_definition", "path": "src/main.go", "symbol": "MyType"}]}`,
			wantErr: false,
		},
		{
			name: "go_to_definition with position",
			json: `{"actions": [{"type": "go_to_definition", "path": "src/main.go", "line": 20, "column": 10}]}`,
			wantErr: false,
		},
		{
			name: "find_implementations with symbol",
			json: `{"actions": [{"type": "find_implementations", "path": "src/interface.go", "symbol": "MyInterface"}]}`,
			wantErr: false,
		},
		{
			name: "find_implementations with position",
			json: `{"actions": [{"type": "find_implementations", "path": "src/interface.go", "line": 15, "column": 8}]}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := DecodeStrict([]byte(tt.json))
			if (err != nil) != tt.wantErr {
			_ = env  // May be unused in some tests
				t.Errorf("DecodeStrict() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !containsStr(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing %q, got %v", tt.errMsg, err)
				}
			}
			if !tt.wantErr && env == nil {
				t.Error("Expected valid envelope, got nil")
			}
		})
	}
}

func TestRefactoringActions(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "extract_method valid",
			json: `{"actions": [{"type": "extract_method", "path": "src/main.go", "method_name": "processData", "start_line": 10, "end_line": 25}]}`,
			wantErr: false,
		},
		{
			name: "extract_method missing method_name",
			json: `{"actions": [{"type": "extract_method", "path": "src/main.go", "start_line": 10, "end_line": 25}]}`,
			wantErr: true,
			errMsg: "requires method_name",
		},
		{
			name: "rename_symbol valid",
			json: `{"actions": [{"type": "rename_symbol", "path": "src/main.go", "symbol": "oldName", "new_name": "newName"}]}`,
			wantErr: false,
		},
		{
			name: "rename_symbol missing new_name",
			json: `{"actions": [{"type": "rename_symbol", "path": "src/main.go", "symbol": "oldName"}]}`,
			wantErr: true,
			errMsg: "requires new_name",
		},
		{
			name: "inline_variable valid",
			json: `{"actions": [{"type": "inline_variable", "path": "src/main.go", "variable_name": "tempVar"}]}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := DecodeStrict([]byte(tt.json))
			if (err != nil) != tt.wantErr {
			_ = env  // May be unused in some tests
				t.Errorf("DecodeStrict() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !containsStr(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing %q, got %v", tt.errMsg, err)
				}
			}
		})
	}
}

func TestFileManagementActions(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "move_file valid",
			json: `{"actions": [{"type": "move_file", "source_path": "src/old.go", "target_path": "src/new/old.go"}]}`,
			wantErr: false,
		},
		{
			name: "move_file missing target",
			json: `{"actions": [{"type": "move_file", "source_path": "src/old.go"}]}`,
			wantErr: true,
			errMsg: "requires target_path",
		},
		{
			name: "delete_file valid",
			json: `{"actions": [{"type": "delete_file", "path": "src/old.go"}]}`,
			wantErr: false,
		},
		{
			name: "rename_file valid",
			json: `{"actions": [{"type": "rename_file", "source_path": "src/old.go", "new_name": "new.go"}]}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := DecodeStrict([]byte(tt.json))
			if (err != nil) != tt.wantErr {
			_ = env  // May be unused in some tests
				t.Errorf("DecodeStrict() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !containsStr(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing %q, got %v", tt.errMsg, err)
				}
			}
		})
	}
}

func TestDebuggingActions(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "add_log valid",
			json: `{"actions": [{"type": "add_log", "path": "src/main.go", "line": 42, "log_message": "Debug point", "log_level": "info"}]}`,
			wantErr: false,
		},
		{
			name: "add_log missing message",
			json: `{"actions": [{"type": "add_log", "path": "src/main.go", "line": 42}]}`,
			wantErr: true,
			errMsg: "requires log_message",
		},
		{
			name: "add_breakpoint valid",
			json: `{"actions": [{"type": "add_breakpoint", "path": "src/main.go", "line": 42}]}`,
			wantErr: false,
		},
		{
			name: "add_breakpoint with condition",
			json: `{"actions": [{"type": "add_breakpoint", "path": "src/main.go", "line": 42, "condition": "x > 10"}]}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := DecodeStrict([]byte(tt.json))
			if (err != nil) != tt.wantErr {
			_ = env  // May be unused in some tests
				t.Errorf("DecodeStrict() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || !containsStr(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing %q, got %v", tt.errMsg, err)
				}
			}
		})
	}
}

func TestDocumentationActions(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name: "generate_docs valid",
			json: `{"actions": [{"type": "generate_docs", "path": "src/main.go"}]}`,
			wantErr: false,
		},
		{
			name: "generate_docs with format",
			json: `{"actions": [{"type": "generate_docs", "path": "src/main.go", "doc_format": "godoc"}]}`,
			wantErr: false,
		},
		{
			name: "generate_docs missing path",
			json: `{"actions": [{"type": "generate_docs"}]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := DecodeStrict([]byte(tt.json))
			if (err != nil) != tt.wantErr {
			_ = env  // May be unused in some tests
				t.Errorf("DecodeStrict() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
