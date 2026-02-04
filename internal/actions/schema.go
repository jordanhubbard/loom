package actions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	ActionAskFollowup = "ask_followup"
	ActionReadCode    = "read_code"
	ActionEditCode    = "edit_code"
	ActionWriteFile   = "write_file"
	ActionRunCommand  = "run_command"
	ActionRunTests    = "run_tests"
	ActionCreateBead  = "create_bead"
	ActionCloseBead   = "close_bead"
	ActionEscalateCEO = "escalate_ceo"
	ActionReadFile    = "read_file"
	ActionReadTree    = "read_tree"
	ActionSearchText  = "search_text"
	ActionApplyPatch  = "apply_patch"
	ActionGitStatus   = "git_status"
	ActionGitDiff     = "git_diff"
	ActionApproveBead = "approve_bead"
	ActionRejectBead  = "reject_bead"
)

type ActionEnvelope struct {
	Actions []Action `json:"actions"`
	Notes   string   `json:"notes,omitempty"`
}

type Action struct {
	Type string `json:"type"`

	Question string `json:"question,omitempty"`

	Path     string `json:"path,omitempty"`
	Content  string `json:"content,omitempty"`
	Patch    string `json:"patch,omitempty"`
	Query    string `json:"query,omitempty"`
	MaxDepth int    `json:"max_depth,omitempty"`
	Limit    int    `json:"limit,omitempty"`

	Command    string `json:"command,omitempty"`
	WorkingDir string `json:"working_dir,omitempty"`

	// Test execution fields
	TestPattern    string `json:"test_pattern,omitempty"`
	Framework      string `json:"framework,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`

	Bead *BeadPayload `json:"bead,omitempty"`

	BeadID     string `json:"bead_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
	ReturnedTo string `json:"returned_to,omitempty"`
}

type BeadPayload struct {
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Priority    int               `json:"priority,omitempty"`
	Type        string            `json:"type,omitempty"`
	ProjectID   string            `json:"project_id"`
	Tags        []string          `json:"tags,omitempty"`
	Context     map[string]string `json:"context,omitempty"`
}

func DecodeStrict(payload []byte) (*ActionEnvelope, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()

	var env ActionEnvelope
	if err := decoder.Decode(&env); err != nil {
		return nil, err
	}
	if decoder.More() {
		return nil, errors.New("unexpected trailing JSON tokens")
	}
	if err := Validate(&env); err != nil {
		return nil, err
	}
	return &env, nil
}

// DecodeLenient attempts strict decode first, then tries to recover a JSON object
// from responses that include extra text (e.g., markdown fences, model traces, or <think> blocks).
func DecodeLenient(payload []byte) (*ActionEnvelope, error) {
	env, err := DecodeStrict(payload)
	if err == nil {
		return env, nil
	}
	trimmed := bytes.TrimSpace(payload)
	trimmed = stripCodeFences(trimmed)
	trimmed = stripThinkTags(trimmed)
	extracted, extractErr := extractJSONObject(trimmed)
	if extractErr != nil {
		return nil, err
	}
	return DecodeStrict(extracted)
}

func stripCodeFences(payload []byte) []byte {
	if !bytes.HasPrefix(bytes.TrimSpace(payload), []byte("```")) {
		return payload
	}
	lines := strings.Split(string(payload), "\n")
	if len(lines) < 2 {
		return payload
	}
	start := 0
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		start = 1
	}
	end := len(lines)
	if strings.HasPrefix(strings.TrimSpace(lines[end-1]), "```") {
		end--
	}
	if start >= end {
		return payload
	}
	return []byte(strings.Join(lines[start:end], "\n"))
}

// stripThinkTags removes <think>...</think> blocks and handles cases where
// models output </think> without opening tag (everything before it is thinking)
func stripThinkTags(payload []byte) []byte {
	s := string(payload)

	// First, handle paired <think>...</think> blocks
	for {
		start := strings.Index(s, "<think>")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "</think>")
		if end == -1 {
			// Unclosed tag - remove from <think> to end
			s = s[:start]
			break
		}
		// Remove the entire <think>...</think> block
		s = s[:start] + s[start+end+len("</think>"):]
	}

	// Handle case where model outputs </think> without opening tag
	// (common with some reasoning models - everything before </think> is reasoning)
	if closeIdx := strings.Index(s, "</think>"); closeIdx != -1 {
		s = s[closeIdx+len("</think>"):]
	}

	return []byte(strings.TrimSpace(s))
}

func extractJSONObject(payload []byte) ([]byte, error) {
	inString := false
	escaped := false
	depth := 0
	start := -1
	for i, b := range payload {
		if escaped {
			escaped = false
			continue
		}
		if b == '\\' && inString {
			escaped = true
			continue
		}
		if b == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if b == '{' {
			if depth == 0 {
				start = i
			}
			depth++
			continue
		}
		if b == '}' {
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				return payload[start : i+1], nil
			}
		}
	}
	return nil, errors.New("no JSON object found in response")
}

func Validate(env *ActionEnvelope) error {
	if env == nil {
		return errors.New("action envelope is nil")
	}
	if len(env.Actions) == 0 {
		return errors.New("action envelope must include at least one action")
	}

	for idx, action := range env.Actions {
		if action.Type == "" {
			return fmt.Errorf("action[%d] missing type", idx)
		}
		if err := validateAction(action); err != nil {
			return fmt.Errorf("action[%d] %s", idx, err.Error())
		}
	}
	return nil
}

func validateAction(action Action) error {
	switch action.Type {
	case ActionAskFollowup:
		if action.Question == "" {
			return errors.New("ask_followup requires question")
		}
	case ActionReadCode:
		if action.Path == "" {
			return errors.New("read_code requires path")
		}
	case ActionEditCode:
		if action.Path == "" || action.Patch == "" {
			return errors.New("edit_code requires path and patch")
		}
	case ActionWriteFile:
		if action.Path == "" || action.Content == "" {
			return errors.New("write_file requires path and content")
		}
	case ActionReadFile:
		if action.Path == "" {
			return errors.New("read_file requires path")
		}
	case ActionReadTree:
		if action.Path == "" {
			return errors.New("read_tree requires path")
		}
	case ActionSearchText:
		if action.Query == "" {
			return errors.New("search_text requires query")
		}
	case ActionApplyPatch:
		if action.Patch == "" {
			return errors.New("apply_patch requires patch")
		}
	case ActionGitStatus, ActionGitDiff:
	case ActionRunCommand:
		if action.Command == "" {
			return errors.New("run_command requires command")
		}
	case ActionRunTests:
		// All fields are optional - defaults will be used
		// test_pattern, framework (auto-detect), timeout_seconds (default)
	case ActionCreateBead:
		if action.Bead == nil {
			return errors.New("create_bead requires bead payload")
		}
		if action.Bead.Title == "" || action.Bead.ProjectID == "" {
			return errors.New("create_bead requires bead.title and bead.project_id")
		}
	case ActionCloseBead:
		if action.BeadID == "" {
			return errors.New("close_bead requires bead_id")
		}
	case ActionEscalateCEO:
		if action.BeadID == "" {
			return errors.New("escalate_ceo requires bead_id")
		}
	case ActionApproveBead:
		if action.BeadID == "" {
			return errors.New("approve_bead requires bead_id")
		}
	case ActionRejectBead:
		if action.BeadID == "" {
			return errors.New("reject_bead requires bead_id")
		}
		if action.Reason == "" {
			return errors.New("reject_bead requires reason")
		}
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}

	return nil
}
