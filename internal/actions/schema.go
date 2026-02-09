package actions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	ActionAskFollowup   = "ask_followup"
	ActionReadCode      = "read_code"
	ActionEditCode      = "edit_code"
	ActionWriteFile     = "write_file"
	ActionRunCommand    = "run_command"
	ActionRunTests      = "run_tests"
	ActionRunLinter     = "run_linter"
	ActionBuildProject  = "build_project"
	ActionCreateBead    = "create_bead"
	ActionCloseBead     = "close_bead"
	ActionEscalateCEO   = "escalate_ceo"
	ActionReadFile      = "read_file"
	ActionReadTree      = "read_tree"
	ActionSearchText    = "search_text"
	ActionApplyPatch    = "apply_patch"
	ActionGitStatus     = "git_status"
	ActionGitDiff       = "git_diff"
	ActionGitCommit       = "git_commit"
	ActionGitPush         = "git_push"
	ActionCreatePR        = "create_pr"
	ActionStartDev        = "start_development"
	ActionWhatsNext       = "whats_next"
	ActionProceedToPhase  = "proceed_to_phase"
	ActionConductReview   = "conduct_review"
	ActionResumeWorkflow  = "resume_workflow"
	ActionApproveBead     = "approve_bead"
	ActionRejectBead      = "reject_bead"

	// Code navigation actions
	ActionFindReferences      = "find_references"
	ActionGoToDefinition      = "go_to_definition"
	ActionFindImplementations = "find_implementations"

	// Refactoring actions
	ActionExtractMethod  = "extract_method"
	ActionRenameSymbol   = "rename_symbol"
	ActionInlineVariable = "inline_variable"

	// File management actions
	ActionMoveFile   = "move_file"
	ActionDeleteFile = "delete_file"
	ActionRenameFile = "rename_file"

	// Debugging actions
	ActionAddLog        = "add_log"
	ActionAddBreakpoint = "add_breakpoint"

	// Documentation generation actions
	ActionGenerateDocs = "generate_docs"

	// PR review actions
	ActionFetchPR         = "fetch_pr"
	ActionReviewCode      = "review_code"
	ActionAddPRComment    = "add_pr_comment"
	ActionSubmitReview    = "submit_review"
	ActionRequestReview   = "request_review"

	// Extended git operations
	ActionGitMerge        = "git_merge"
	ActionGitRevert       = "git_revert"
	ActionGitBranchDelete = "git_branch_delete"
	ActionGitCheckout     = "git_checkout"
	ActionGitLog          = "git_log"
	ActionGitFetch        = "git_fetch"
	ActionGitListBranches = "git_list_branches"
	ActionGitDiffBranches = "git_diff_branches"
	ActionGitBeadCommits  = "git_bead_commits"

	// Agent signals
	ActionDone = "done"

	// Agent communication actions
	ActionSendAgentMessage = "send_agent_message"
	ActionDelegateTask     = "delegate_task"
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

	// Linter execution fields
	Files []string `json:"files,omitempty"` // Specific files to lint

	// Build execution fields
	BuildTarget  string `json:"build_target,omitempty"`  // Build target (e.g., binary name)
	BuildCommand string `json:"build_command,omitempty"` // Custom build command

	// Git operation fields
	CommitMessage string   `json:"commit_message,omitempty"` // Commit message (auto-generated if empty)
	Branch        string   `json:"branch,omitempty"`         // Branch name
	SetUpstream   bool     `json:"set_upstream,omitempty"`   // Set upstream tracking (-u flag)
	PRTitle       string   `json:"pr_title,omitempty"`       // Pull request title
	PRBody        string   `json:"pr_body,omitempty"`        // Pull request body
	PRBase        string   `json:"pr_base,omitempty"`        // PR base branch (default: main)
	PRReviewers   []string `json:"pr_reviewers,omitempty"`   // PR reviewers

	// Extended git fields
	SourceBranch string   `json:"source_branch,omitempty"` // Branch to merge from or diff against
	TargetBranch string   `json:"target_branch,omitempty"` // Target branch for diff
	CommitSHA    string   `json:"commit_sha,omitempty"`    // Single commit SHA
	CommitSHAs   []string `json:"commit_shas,omitempty"`   // Multiple commit SHAs (for revert)
	MaxCount     int      `json:"max_count,omitempty"`     // Max entries for log
	NoFF         bool     `json:"no_ff,omitempty"`         // No fast-forward merge
	DeleteRemote bool     `json:"delete_remote,omitempty"` // Delete remote branch too

	// Workflow management fields
	Workflow       string `json:"workflow,omitempty"`        // Workflow type (epcc, tdd, waterfall, etc.)
	RequireReviews bool   `json:"require_reviews,omitempty"` // Require reviews before phase transitions
	TargetPhase    string `json:"target_phase,omitempty"`    // Target phase for proceed_to_phase
	ReviewState    string `json:"review_state,omitempty"`    // Review state (not-required, pending, performed)

	// Code navigation fields
	Symbol   string `json:"symbol,omitempty"`    // Symbol name for find_references/go_to_definition
	Line     int    `json:"line,omitempty"`      // Line number for position-based queries
	Column   int    `json:"column,omitempty"`    // Column number for position-based queries
	Language string `json:"language,omitempty"`  // Language hint (go, typescript, python, etc.)

	// Refactoring fields
	NewName       string `json:"new_name,omitempty"`       // New name for rename_symbol/rename_file
	MethodName    string `json:"method_name,omitempty"`    // Method name for extract_method
	StartLine     int    `json:"start_line,omitempty"`     // Start line for extract_method
	EndLine       int    `json:"end_line,omitempty"`       // End line for extract_method
	VariableName  string `json:"variable_name,omitempty"`  // Variable name for inline_variable

	// File management fields
	SourcePath string `json:"source_path,omitempty"` // Source file path for move/rename
	TargetPath string `json:"target_path,omitempty"` // Target file path for move/rename

	// Debugging fields
	LogMessage  string `json:"log_message,omitempty"`  // Log message for add_log
	LogLevel    string `json:"log_level,omitempty"`    // Log level (info, warn, error, debug)
	Condition   string `json:"condition,omitempty"`    // Breakpoint condition

	// Documentation fields
	DocFormat string `json:"doc_format,omitempty"` // Documentation format (godoc, jsdoc, markdown)

	// PR review fields
	PRNumber       int      `json:"pr_number,omitempty"`        // PR number for fetch_pr and review actions
	IncludeFiles   bool     `json:"include_files,omitempty"`    // Include changed files in fetch_pr
	IncludeDiff    bool     `json:"include_diff,omitempty"`     // Include diff in fetch_pr
	ReviewCriteria []string `json:"review_criteria,omitempty"`  // Criteria for review_code (quality, security, testing)
	CommentBody    string   `json:"comment_body,omitempty"`     // Comment text for add_pr_comment
	CommentPath    string   `json:"comment_path,omitempty"`     // File path for inline comment
	CommentLine    int      `json:"comment_line,omitempty"`     // Line number for inline comment
	CommentSide    string   `json:"comment_side,omitempty"`     // Side for inline comment (LEFT, RIGHT)
	ReviewEvent    string   `json:"review_event,omitempty"`     // Review event (APPROVE, REQUEST_CHANGES, COMMENT)
	Reviewer       string   `json:"reviewer,omitempty"`         // Reviewer for request_review

	// Agent communication fields
	ToAgentID      string                 `json:"to_agent_id,omitempty"`      // Target agent ID for send_agent_message
	ToAgentRole    string                 `json:"to_agent_role,omitempty"`    // Target agent role (alternative to ID)
	MessageType    string                 `json:"message_type,omitempty"`     // Message type (question, delegation, notification)
	MessageSubject string                 `json:"message_subject,omitempty"`  // Message subject
	MessageBody    string                 `json:"message_body,omitempty"`     // Message body
	MessagePayload map[string]interface{} `json:"message_payload,omitempty"`  // Optional message payload/context

	// Task delegation fields
	DelegateToRole  string                 `json:"delegate_to_role,omitempty"`  // Role to delegate task to
	TaskTitle       string                 `json:"task_title,omitempty"`        // Title for delegated task
	TaskDescription string                 `json:"task_description,omitempty"`  // Description for delegated task
	TaskPriority    int                    `json:"task_priority,omitempty"`     // Priority for delegated task (0-4)
	ParentBeadID    string                 `json:"parent_bead_id,omitempty"`    // Parent bead that created this delegation

	Bead *BeadPayload `json:"bead,omitempty"`

	BeadID     string `json:"bead_id,omitempty"`
	Reason     string `json:"reason,omitempty"`     // Reason for bead operations or phase transitions
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

// ValidationError wraps action validation failures (JSON parsed OK, but required fields missing).
// This is distinct from JSON parse errors — the model produced valid JSON but incomplete actions.
type ValidationError struct {
	Err error
}

func (e *ValidationError) Error() string { return e.Err.Error() }
func (e *ValidationError) Unwrap() error { return e.Err }

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
		return nil, &ValidationError{Err: err}
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
	case ActionDone:
		// No required fields — agent signals work is complete
	case ActionGitStatus, ActionGitDiff:
		// No required fields
	case ActionGitCommit:
		// commit_message is optional (auto-generated from bead context)
		// All other fields optional
	case ActionGitPush:
		// branch is optional (uses current branch)
		// set_upstream optional
	case ActionCreatePR:
		// pr_title and pr_body optional (auto-generated from bead)
		// pr_base optional (defaults to main)
	case ActionGitMerge:
		if action.SourceBranch == "" {
			return errors.New("git_merge requires source_branch")
		}
	case ActionGitRevert:
		if action.CommitSHA == "" && len(action.CommitSHAs) == 0 {
			return errors.New("git_revert requires commit_sha or commit_shas")
		}
	case ActionGitBranchDelete:
		if action.Branch == "" {
			return errors.New("git_branch_delete requires branch")
		}
	case ActionGitCheckout:
		if action.Branch == "" {
			return errors.New("git_checkout requires branch")
		}
	case ActionGitLog:
		// All fields optional (branch defaults to current, max_count defaults to 20)
	case ActionGitFetch:
		// No required fields
	case ActionGitListBranches:
		// No required fields
	case ActionGitDiffBranches:
		if action.SourceBranch == "" || action.TargetBranch == "" {
			return errors.New("git_diff_branches requires source_branch and target_branch")
		}
	case ActionGitBeadCommits:
		// bead_id comes from action context
	case ActionRunCommand:
		if action.Command == "" {
			return errors.New("run_command requires command")
		}
	case ActionRunTests:
		// All fields are optional - defaults will be used
		// test_pattern, framework (auto-detect), timeout_seconds (default)
	case ActionRunLinter:
		// All fields are optional - defaults will be used
		// files, framework (auto-detect), timeout_seconds (default)
	case ActionBuildProject:
		// All fields are optional - defaults will be used
		// build_target, framework (auto-detect), build_command, timeout_seconds (default)
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
	case ActionStartDev:
		if action.Workflow == "" {
			return errors.New("start_development requires workflow")
		}
	case ActionWhatsNext:
		// All fields optional - context, user_input, conversation_summary, recent_messages
	case ActionProceedToPhase:
		if action.TargetPhase == "" {
			return errors.New("proceed_to_phase requires target_phase")
		}
		if action.ReviewState == "" {
			return errors.New("proceed_to_phase requires review_state")
		}
	case ActionConductReview:
		if action.TargetPhase == "" {
			return errors.New("conduct_review requires target_phase")
		}
	case ActionResumeWorkflow:
		// All fields optional - include_system_prompt
	case ActionFindReferences:
		if action.Path == "" {
			return errors.New("find_references requires path")
		}
		if action.Symbol == "" && (action.Line == 0 || action.Column == 0) {
			return errors.New("find_references requires either symbol or (line and column)")
		}
	case ActionGoToDefinition:
		if action.Path == "" {
			return errors.New("go_to_definition requires path")
		}
		if action.Symbol == "" && (action.Line == 0 || action.Column == 0) {
			return errors.New("go_to_definition requires either symbol or (line and column)")
		}
	case ActionFindImplementations:
		if action.Path == "" {
			return errors.New("find_implementations requires path")
		}
		if action.Symbol == "" && (action.Line == 0 || action.Column == 0) {
			return errors.New("find_implementations requires either symbol or (line and column)")
		}
	case ActionExtractMethod:
		if action.Path == "" {
			return errors.New("extract_method requires path")
		}
		if action.MethodName == "" {
			return errors.New("extract_method requires method_name")
		}
		if action.StartLine == 0 || action.EndLine == 0 {
			return errors.New("extract_method requires start_line and end_line")
		}
	case ActionRenameSymbol:
		if action.Path == "" {
			return errors.New("rename_symbol requires path")
		}
		if action.Symbol == "" {
			return errors.New("rename_symbol requires symbol")
		}
		if action.NewName == "" {
			return errors.New("rename_symbol requires new_name")
		}
	case ActionInlineVariable:
		if action.Path == "" {
			return errors.New("inline_variable requires path")
		}
		if action.VariableName == "" {
			return errors.New("inline_variable requires variable_name")
		}
	case ActionMoveFile:
		if action.SourcePath == "" {
			return errors.New("move_file requires source_path")
		}
		if action.TargetPath == "" {
			return errors.New("move_file requires target_path")
		}
	case ActionDeleteFile:
		if action.Path == "" {
			return errors.New("delete_file requires path")
		}
	case ActionRenameFile:
		if action.SourcePath == "" {
			return errors.New("rename_file requires source_path")
		}
		if action.NewName == "" {
			return errors.New("rename_file requires new_name")
		}
	case ActionAddLog:
		if action.Path == "" {
			return errors.New("add_log requires path")
		}
		if action.Line == 0 {
			return errors.New("add_log requires line")
		}
		if action.LogMessage == "" {
			return errors.New("add_log requires log_message")
		}
	case ActionAddBreakpoint:
		if action.Path == "" {
			return errors.New("add_breakpoint requires path")
		}
		if action.Line == 0 {
			return errors.New("add_breakpoint requires line")
		}
	case ActionGenerateDocs:
		if action.Path == "" {
			return errors.New("generate_docs requires path")
		}
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}

	return nil
}
