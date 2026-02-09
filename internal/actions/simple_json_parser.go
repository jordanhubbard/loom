package actions

import (
	"encoding/json"
	"fmt"
)

// SimpleJSONAction is the minimal JSON structure agents produce in simple mode.
type SimpleJSONAction struct {
	Action  string `json:"action"`
	Path    string `json:"path,omitempty"`
	Query   string `json:"query,omitempty"`
	Old     string `json:"old,omitempty"`
	New     string `json:"new,omitempty"`
	Content string `json:"content,omitempty"`
	Command string `json:"command,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Message string `json:"message,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

// ParseSimpleJSON parses the minimal JSON action format into an ActionEnvelope.
// Accepts both simple format {"action": "read", "path": "..."} and legacy
// format {"actions": [{"type": "read_file", "path": "..."}]} as fallback.
func ParseSimpleJSON(payload []byte) (*ActionEnvelope, error) {
	// Try simple format first
	var simple SimpleJSONAction
	if err := json.Unmarshal(payload, &simple); err != nil {
		return nil, err
	}

	if simple.Action != "" {
		action, err := simpleToAction(simple)
		if err != nil {
			return nil, err
		}
		return &ActionEnvelope{
			Actions: []Action{action},
			Notes:   simple.Notes,
		}, nil
	}

	// Fallback: try legacy {"actions": [...]} format
	var legacy ActionEnvelope
	if err := json.Unmarshal(payload, &legacy); err == nil && len(legacy.Actions) > 0 {
		// Validate the first action has a type
		if legacy.Actions[0].Type != "" {
			if err := Validate(&legacy); err != nil {
				return nil, &ValidationError{Err: err}
			}
			return &legacy, nil
		}
	}

	return nil, &ValidationError{Err: fmt.Errorf("missing 'action' field — respond with {\"action\": \"scope\", \"path\": \".\"} to start")}
}

func simpleToAction(s SimpleJSONAction) (Action, error) {
	switch s.Action {
	case "scope", "tree":
		path := s.Path
		if path == "" {
			path = "."
		}
		depth := 2
		if s.Action == "tree" {
			depth = 3
		}
		return Action{Type: ActionReadTree, Path: path, MaxDepth: depth}, nil

	case "read":
		if s.Path == "" {
			return Action{}, &ValidationError{Err: fmt.Errorf("read requires 'path'")}
		}
		return Action{Type: ActionReadFile, Path: s.Path}, nil

	case "search":
		if s.Query == "" {
			return Action{}, &ValidationError{Err: fmt.Errorf("search requires 'query'")}
		}
		return Action{Type: ActionSearchText, Query: s.Query, Path: s.Path}, nil

	case "edit":
		if s.Path == "" || s.Old == "" {
			return Action{}, &ValidationError{Err: fmt.Errorf("edit requires 'path' and 'old'")}
		}
		return Action{Type: ActionEditCode, Path: s.Path, OldText: s.Old, NewText: s.New}, nil

	case "write":
		if s.Path == "" || s.Content == "" {
			return Action{}, &ValidationError{Err: fmt.Errorf("write requires 'path' and 'content'")}
		}
		return Action{Type: ActionWriteFile, Path: s.Path, Content: s.Content}, nil

	case "build":
		return Action{Type: ActionBuildProject}, nil

	case "test":
		return Action{Type: ActionRunTests, TestPattern: s.Pattern}, nil

	case "bash":
		if s.Command == "" {
			return Action{}, &ValidationError{Err: fmt.Errorf("bash requires 'command'")}
		}
		return Action{Type: ActionRunCommand, Command: s.Command}, nil

	case "git_commit":
		return Action{Type: ActionGitCommit, CommitMessage: s.Message}, nil

	case "git_push":
		return Action{Type: ActionGitPush}, nil

	case "git_status":
		return Action{Type: ActionGitStatus}, nil

	case "done":
		return Action{Type: ActionDone, Reason: s.Reason}, nil

	case "close_bead":
		return Action{Type: ActionCloseBead, Reason: s.Reason}, nil

	case "escalate":
		return Action{Type: ActionEscalateCEO, Reason: s.Reason}, nil

	default:
		return Action{}, &ValidationError{Err: fmt.Errorf("unknown action '%s'. Use: scope, read, search, edit, write, build, test, bash, done, close_bead, git_commit, git_push", s.Action)}
	}
}
