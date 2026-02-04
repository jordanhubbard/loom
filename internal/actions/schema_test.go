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
