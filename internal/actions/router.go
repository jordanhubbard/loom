package actions

import (
	"context"
	"fmt"

	"github.com/jordanhubbard/agenticorp/internal/executor"
	"github.com/jordanhubbard/agenticorp/internal/files"
	"github.com/jordanhubbard/agenticorp/pkg/models"
)

type BeadCreator interface {
	CreateBead(title, description string, priority models.BeadPriority, beadType, projectID string) (*models.Bead, error)
}

type BeadCloser interface {
	CloseBead(beadID, reason string) error
}

type BeadEscalator interface {
	EscalateBeadToCEO(beadID, reason, returnedTo string) (*models.DecisionBead, error)
}

type CommandExecutor interface {
	ExecuteCommand(ctx context.Context, req executor.ExecuteCommandRequest) (*executor.ExecuteCommandResult, error)
}

type TestRunner interface {
	Run(ctx context.Context, projectPath string, testPattern, framework string, timeoutSeconds int) (map[string]interface{}, error)
}

type FileManager interface {
	ReadFile(ctx context.Context, projectID, path string) (*files.FileResult, error)
	WriteFile(ctx context.Context, projectID, path, content string) (*files.WriteResult, error)
	ReadTree(ctx context.Context, projectID, path string, maxDepth, limit int) ([]files.TreeEntry, error)
	SearchText(ctx context.Context, projectID, path, query string, limit int) ([]files.SearchMatch, error)
	ApplyPatch(ctx context.Context, projectID, patch string) (*files.PatchResult, error)
}

type GitOperator interface {
	Status(ctx context.Context, projectID string) (string, error)
	Diff(ctx context.Context, projectID string) (string, error)
}

type ActionLogger interface {
	LogAction(ctx context.Context, actx ActionContext, action Action, result Result)
}

type WorkflowOperator interface {
	AdvanceWorkflowWithCondition(beadID, agentID string, condition string, resultData map[string]string) error
}

type ActionContext struct {
	AgentID   string
	BeadID    string
	ProjectID string
}

type Result struct {
	ActionType string                 `json:"action_type"`
	Status     string                 `json:"status"`
	Message    string                 `json:"message"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

type Router struct {
	Beads     BeadCreator
	Closer    BeadCloser
	Escalator BeadEscalator
	Commands  CommandExecutor
	Tests     TestRunner
	Files     FileManager
	Git       GitOperator
	Logger    ActionLogger
	Workflow  WorkflowOperator
	BeadType  string
	BeadTags  []string
	DefaultP0 bool
}

func (r *Router) Execute(ctx context.Context, env *ActionEnvelope, actx ActionContext) ([]Result, error) {
	if env == nil {
		return nil, fmt.Errorf("action envelope is nil")
	}

	results := make([]Result, 0, len(env.Actions))
	for _, action := range env.Actions {
		result := r.executeAction(ctx, action, actx)
		if r.Logger != nil {
			r.Logger.LogAction(ctx, actx, action, result)
		}
		results = append(results, result)
	}

	return results, nil
}

func (r *Router) AutoFileParseFailure(ctx context.Context, actx ActionContext, err error, raw string) Result {
	if r.Beads == nil {
		return Result{ActionType: ActionCreateBead, Status: "error", Message: "bead creator not configured"}
	}
	priority := models.BeadPriority(0)
	if !r.DefaultP0 {
		priority = models.BeadPriority(2)
	}
	description := fmt.Sprintf("Failed to parse strict JSON actions.\n\nError:\n%s\n\nRaw response:\n%s", err.Error(), raw)
	bead, beadErr := r.Beads.CreateBead("Action parse failed", description, priority, "bug", actx.ProjectID)
	if beadErr != nil {
		return Result{ActionType: ActionCreateBead, Status: "error", Message: beadErr.Error()}
	}
	result := Result{
		ActionType: ActionCreateBead,
		Status:     "executed",
		Message:    "auto-filed action parse failure",
		Metadata:   map[string]interface{}{"bead_id": bead.ID},
	}
	if r.Logger != nil {
		r.Logger.LogAction(ctx, actx, Action{Type: ActionCreateBead}, result)
	}
	return result
}

func (r *Router) executeAction(ctx context.Context, action Action, actx ActionContext) Result {
	switch action.Type {
	case ActionAskFollowup:
		return r.createBeadFromAction("Follow-up question", action.Question, actx)
	case ActionReadCode:
		if r.Files == nil {
			return r.createBeadFromAction("Read code", action.Path, actx)
		}
		res, err := r.Files.ReadFile(ctx, actx.ProjectID, action.Path)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "file read",
			Metadata: map[string]interface{}{
				"path":    res.Path,
				"content": res.Content,
				"size":    res.Size,
			},
		}
	case ActionEditCode:
		if r.Files == nil {
			return r.createBeadFromAction("Edit code", fmt.Sprintf("%s\n\nPatch:\n%s", action.Path, action.Patch), actx)
		}
		res, err := r.Files.ApplyPatch(ctx, actx.ProjectID, action.Patch)
		if err != nil {
			message := err.Error()
			if res != nil && res.Output != "" {
				message = fmt.Sprintf("%s: %s", message, res.Output)
			}
			return Result{ActionType: action.Type, Status: "error", Message: message}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "patch applied",
			Metadata:   map[string]interface{}{"output": res.Output},
		}
	case ActionWriteFile:
		if r.Files == nil {
			return r.createBeadFromAction("Write file", fmt.Sprintf("%s\n\nContent:\n%s", action.Path, truncateContent(action.Content, 500)), actx)
		}
		res, err := r.Files.WriteFile(ctx, actx.ProjectID, action.Path, action.Content)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "file written",
			Metadata: map[string]interface{}{
				"path":          res.Path,
				"bytes_written": res.BytesWritten,
			},
		}
	case ActionReadFile:
		if r.Files == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "file manager not configured"}
		}
		res, err := r.Files.ReadFile(ctx, actx.ProjectID, action.Path)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "file read",
			Metadata: map[string]interface{}{
				"path":    res.Path,
				"content": res.Content,
				"size":    res.Size,
			},
		}
	case ActionReadTree:
		if r.Files == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "file manager not configured"}
		}
		path := action.Path
		if path == "" {
			path = "."
		}
		res, err := r.Files.ReadTree(ctx, actx.ProjectID, path, action.MaxDepth, action.Limit)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "tree read",
			Metadata:   map[string]interface{}{"entries": res},
		}
	case ActionSearchText:
		if r.Files == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "file manager not configured"}
		}
		path := action.Path
		if path == "" {
			path = "."
		}
		res, err := r.Files.SearchText(ctx, actx.ProjectID, path, action.Query, action.Limit)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "search completed",
			Metadata:   map[string]interface{}{"matches": res},
		}
	case ActionApplyPatch:
		if r.Files == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "file manager not configured"}
		}
		res, err := r.Files.ApplyPatch(ctx, actx.ProjectID, action.Patch)
		if err != nil {
			message := err.Error()
			if res != nil && res.Output != "" {
				message = fmt.Sprintf("%s: %s", message, res.Output)
			}
			return Result{ActionType: action.Type, Status: "error", Message: message}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "patch applied",
			Metadata:   map[string]interface{}{"output": res.Output},
		}
	case ActionGitStatus:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		out, err := r.Git.Status(ctx, actx.ProjectID)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "git status",
			Metadata:   map[string]interface{}{"output": out},
		}
	case ActionGitDiff:
		if r.Git == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "git operator not configured"}
		}
		out, err := r.Git.Diff(ctx, actx.ProjectID)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "git diff",
			Metadata:   map[string]interface{}{"output": out},
		}
	case ActionRunCommand:
		if r.Commands == nil {
			return r.createBeadFromAction("Run command", action.Command, actx)
		}
		req := executor.ExecuteCommandRequest{
			AgentID:    actx.AgentID,
			BeadID:     actx.BeadID,
			ProjectID:  actx.ProjectID,
			Command:    action.Command,
			WorkingDir: action.WorkingDir,
			Context: map[string]interface{}{
				"action_type": action.Type,
				"reason":      action.Reason,
			},
		}
		res, err := r.Commands.ExecuteCommand(ctx, req)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "command executed",
			Metadata: map[string]interface{}{
				"command_id": res.ID,
				"exit_code":  res.ExitCode,
			},
		}
	case ActionRunTests:
		if r.Tests == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "test runner not configured"}
		}
		// Get project path from Files manager or use default
		projectPath := "."
		// TODO: Get actual project path from context or Files manager

		result, err := r.Tests.Run(ctx, projectPath, action.TestPattern, action.Framework, action.TimeoutSeconds)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "tests executed",
			Metadata:   result,
		}
	case ActionCreateBead:
		if action.Bead == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "missing bead payload"}
		}
		if r.Beads == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "bead creator not configured"}
		}
		beadType := action.Bead.Type
		if beadType == "" {
			beadType = r.BeadType
		}
		if beadType == "" {
			beadType = "task"
		}
		priority := models.BeadPriority(action.Bead.Priority)
		bead, err := r.Beads.CreateBead(action.Bead.Title, action.Bead.Description, priority, beadType, action.Bead.ProjectID)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "bead created",
			Metadata:   map[string]interface{}{"bead_id": bead.ID},
		}
	case ActionCloseBead:
		if r.Closer == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "bead closer not configured"}
		}
		err := r.Closer.CloseBead(action.BeadID, action.Reason)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "bead closed",
			Metadata:   map[string]interface{}{"bead_id": action.BeadID},
		}
	case ActionEscalateCEO:
		if r.Escalator == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "escalator not configured"}
		}
		decision, err := r.Escalator.EscalateBeadToCEO(action.BeadID, action.Reason, action.ReturnedTo)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "escalated to CEO",
			Metadata:   map[string]interface{}{"decision_id": decision.ID},
		}
	case ActionApproveBead:
		if r.Workflow == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "workflow operator not configured"}
		}
		// Advance workflow with approved condition
		resultData := map[string]string{
			"approved_by":     actx.AgentID,
			"approval_reason": action.Reason,
		}
		err := r.Workflow.AdvanceWorkflowWithCondition(action.BeadID, actx.AgentID, "approved", resultData)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "bead approved, workflow advanced",
			Metadata:   map[string]interface{}{"bead_id": action.BeadID},
		}
	case ActionRejectBead:
		if r.Workflow == nil {
			return Result{ActionType: action.Type, Status: "error", Message: "workflow operator not configured"}
		}
		// Advance workflow with rejected condition
		resultData := map[string]string{
			"rejected_by":      actx.AgentID,
			"rejection_reason": action.Reason,
		}
		err := r.Workflow.AdvanceWorkflowWithCondition(action.BeadID, actx.AgentID, "rejected", resultData)
		if err != nil {
			return Result{ActionType: action.Type, Status: "error", Message: err.Error()}
		}
		return Result{
			ActionType: action.Type,
			Status:     "executed",
			Message:    "bead rejected, workflow advanced",
			Metadata:   map[string]interface{}{"bead_id": action.BeadID, "reason": action.Reason},
		}
	default:
		return Result{ActionType: action.Type, Status: "error", Message: "unsupported action"}
	}
}

func (r *Router) createBeadFromAction(title, detail string, actx ActionContext) Result {
	if r.Beads == nil {
		return Result{ActionType: ActionCreateBead, Status: "error", Message: "bead creator not configured"}
	}
	beadType := r.BeadType
	if beadType == "" {
		beadType = "task"
	}
	priority := models.BeadPriority(2)
	if r.DefaultP0 {
		priority = models.BeadPriority(0)
	}
	bead, err := r.Beads.CreateBead(title, detail, priority, beadType, actx.ProjectID)
	if err != nil {
		return Result{ActionType: ActionCreateBead, Status: "error", Message: err.Error()}
	}
	return Result{
		ActionType: ActionCreateBead,
		Status:     "executed",
		Message:    "bead created",
		Metadata:   map[string]interface{}{"bead_id": bead.ID},
	}
}

func truncateContent(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
