package activities

import (
	"context"
	"fmt"

	"github.com/jordanhubbard/agenticorp/internal/executor"
)

// ShellActivity handles shell command execution for agents
type ShellActivity struct {
	executor *executor.ShellExecutor
}

// NewShellActivity creates a new shell activity
func NewShellActivity(exec *executor.ShellExecutor) *ShellActivity {
	return &ShellActivity{
		executor: exec,
	}
}

// ExecuteCommand is a Temporal activity that executes a shell command
func (a *ShellActivity) ExecuteCommand(ctx context.Context, req executor.ExecuteCommandRequest) (*executor.ExecuteCommandResult, error) {
	if a.executor == nil {
		return nil, fmt.Errorf("shell executor not initialized")
	}
	
	return a.executor.ExecuteCommand(ctx, req)
}
