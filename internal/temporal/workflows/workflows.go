package workflows

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	
	"github.com/jordanhubbard/arbiter/internal/temporal/eventbus"
)

// AgentLifecycleWorkflowInput contains input for agent lifecycle workflow
type AgentLifecycleWorkflowInput struct {
	AgentID     string
	ProjectID   string
	PersonaName string
	Name        string
}

// AgentLifecycleWorkflow manages the complete lifecycle of an agent
func AgentLifecycleWorkflow(ctx workflow.Context, input AgentLifecycleWorkflowInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Agent lifecycle workflow started", "agentID", input.AgentID, "projectID", input.ProjectID)

	// Set up workflow options for activities
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	// Agent state
	agentState := struct {
		Status      string
		CurrentBead string
		StartTime   time.Time
	}{
		Status:    "spawned",
		StartTime: time.Now(),
	}

	// Signal handlers for agent control
	workflow.SetUpdateHandler(ctx, "updateStatus", func(ctx workflow.Context, status string) error {
		agentState.Status = status
		logger.Info("Agent status updated", "status", status)
		return nil
	})

	workflow.SetUpdateHandler(ctx, "assignBead", func(ctx workflow.Context, beadID string) error {
		agentState.CurrentBead = beadID
		agentState.Status = "working"
		logger.Info("Bead assigned to agent", "beadID", beadID)
		return nil
	})

	// Query handlers for agent state
	workflow.SetQueryHandler(ctx, "getStatus", func() (string, error) {
		return agentState.Status, nil
	})

	workflow.SetQueryHandler(ctx, "getCurrentBead", func() (string, error) {
		return agentState.CurrentBead, nil
	})

	// Keep workflow running to handle signals
	selector := workflow.NewSelector(ctx)
	
	// Handle shutdown signal
	shutdownChannel := workflow.GetSignalChannel(ctx, "shutdown")
	selector.AddReceive(shutdownChannel, func(c workflow.ReceiveChannel, more bool) {
		var reason string
		c.Receive(ctx, &reason)
		logger.Info("Agent shutdown requested", "reason", reason)
		agentState.Status = "shutdown"
	})

	// Wait for shutdown or timeout
	for agentState.Status != "shutdown" {
		selector.Select(ctx)
	}

	logger.Info("Agent lifecycle workflow completed", "agentID", input.AgentID)
	return nil
}

// BeadProcessingWorkflowInput contains input for bead processing workflow
type BeadProcessingWorkflowInput struct {
	BeadID      string
	ProjectID   string
	Title       string
	Description string
	Priority    int
	Type        string
}

// BeadProcessingWorkflow manages the lifecycle of a bead (work item)
func BeadProcessingWorkflow(ctx workflow.Context, input BeadProcessingWorkflowInput) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Bead processing workflow started", "beadID", input.BeadID, "projectID", input.ProjectID)

	// Set up workflow options
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	// Bead state
	beadState := struct {
		Status     string
		AssignedTo string
		CreatedAt  time.Time
		UpdatedAt  time.Time
	}{
		Status:    "open",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Signal handlers for bead updates
	workflow.SetUpdateHandler(ctx, "assignToAgent", func(ctx workflow.Context, agentID string) error {
		beadState.AssignedTo = agentID
		beadState.Status = "in_progress"
		beadState.UpdatedAt = time.Now()
		logger.Info("Bead assigned", "agentID", agentID)
		return nil
	})

	workflow.SetUpdateHandler(ctx, "updateStatus", func(ctx workflow.Context, status string) error {
		beadState.Status = status
		beadState.UpdatedAt = time.Now()
		logger.Info("Bead status updated", "status", status)
		return nil
	})

	workflow.SetUpdateHandler(ctx, "complete", func(ctx workflow.Context, result string) error {
		beadState.Status = "closed"
		beadState.UpdatedAt = time.Now()
		logger.Info("Bead completed", "result", result)
		return nil
	})

	// Query handlers for bead state
	workflow.SetQueryHandler(ctx, "getStatus", func() (string, error) {
		return beadState.Status, nil
	})

	workflow.SetQueryHandler(ctx, "getAssignedAgent", func() (string, error) {
		return beadState.AssignedTo, nil
	})

	// Keep workflow running until bead is closed
	selector := workflow.NewSelector(ctx)
	
	// Handle status change signals
	statusChannel := workflow.GetSignalChannel(ctx, "statusChange")
	selector.AddReceive(statusChannel, func(c workflow.ReceiveChannel, more bool) {
		var newStatus string
		c.Receive(ctx, &newStatus)
		beadState.Status = newStatus
		beadState.UpdatedAt = time.Now()
		logger.Info("Bead status changed", "newStatus", newStatus)
	})

	// Wait for completion
	for beadState.Status != "closed" {
		selector.Select(ctx)
	}

	logger.Info("Bead processing workflow completed", "beadID", input.BeadID)
	return nil
}

// DecisionWorkflowInput contains input for decision approval workflow
type DecisionWorkflowInput struct {
	DecisionID  string
	ProjectID   string
	Question    string
	RequesterID string
	Options     []string
}

// DecisionWorkflow manages the decision approval process
func DecisionWorkflow(ctx workflow.Context, input DecisionWorkflowInput) (string, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Decision workflow started", "decisionID", input.DecisionID, "projectID", input.ProjectID)

	// Set up activity options
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 48 * time.Hour, // Decisions can take longer
		HeartbeatTimeout:    10 * time.Minute,
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	// Decision state
	decisionState := struct {
		Status     string
		Decision   string
		DeciderID  string
		DecidedAt  time.Time
	}{
		Status: "pending",
	}

	// Signal handler for decision resolution
	workflow.SetUpdateHandler(ctx, "resolve", func(ctx workflow.Context, decision string, deciderID string) error {
		decisionState.Decision = decision
		decisionState.DeciderID = deciderID
		decisionState.Status = "resolved"
		decisionState.DecidedAt = time.Now()
		logger.Info("Decision resolved", "decision", decision, "deciderID", deciderID)
		return nil
	})

	// Query handlers
	workflow.SetQueryHandler(ctx, "getStatus", func() (string, error) {
		return decisionState.Status, nil
	})

	workflow.SetQueryHandler(ctx, "getDecision", func() (string, error) {
		return decisionState.Decision, nil
	})

	// Wait for decision with timeout
	selector := workflow.NewSelector(ctx)
	
	// Handle decision signal
	decisionChannel := workflow.GetSignalChannel(ctx, "resolve")
	selector.AddReceive(decisionChannel, func(c workflow.ReceiveChannel, more bool) {
		var resolution struct {
			Decision  string
			DeciderID string
		}
		c.Receive(ctx, &resolution)
		decisionState.Decision = resolution.Decision
		decisionState.DeciderID = resolution.DeciderID
		decisionState.Status = "resolved"
		decisionState.DecidedAt = time.Now()
	})

	// Add timeout
	timer := workflow.NewTimer(ctx, 48*time.Hour)
	selector.AddFuture(timer, func(f workflow.Future) {
		decisionState.Status = "timeout"
		logger.Warn("Decision timed out")
	})

	// Wait for decision or timeout
	for decisionState.Status == "pending" {
		selector.Select(ctx)
	}

	logger.Info("Decision workflow completed", "decisionID", input.DecisionID, "status", decisionState.Status)
	return decisionState.Decision, nil
}

// SendEventActivity sends an event through the event bus
func SendEventActivity(ctx workflow.Context, event *eventbus.Event) error {
	// This would be implemented as an activity that publishes to the event bus
	logger := workflow.GetLogger(ctx)
	logger.Info("Sending event", "type", event.Type, "id", event.ID)
	return nil
}
