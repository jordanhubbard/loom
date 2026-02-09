package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jordanhubbard/loom/internal/actions"
	"github.com/jordanhubbard/loom/internal/database"
	"github.com/jordanhubbard/loom/internal/provider"
	"github.com/jordanhubbard/loom/pkg/models"
)

// Worker represents an agent worker that processes tasks
type Worker struct {
	id          string
	agent       *models.Agent
	provider    *provider.RegisteredProvider
	db          *database.Database
	status      WorkerStatus
	currentTask string
	startedAt   time.Time
	lastActive  time.Time
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
}

// WorkerStatus represents the status of a worker
type WorkerStatus string

const (
	WorkerStatusIdle    WorkerStatus = "idle"
	WorkerStatusWorking WorkerStatus = "working"
	WorkerStatusStopped WorkerStatus = "stopped"
	WorkerStatusError   WorkerStatus = "error"
)

// NewWorker creates a new agent worker
func NewWorker(id string, agent *models.Agent, provider *provider.RegisteredProvider) *Worker {
	ctx, cancel := context.WithCancel(context.Background())

	return &Worker{
		id:         id,
		agent:      agent,
		provider:   provider,
		status:     WorkerStatusIdle,
		startedAt:  time.Now(),
		lastActive: time.Now(),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start starts the worker
func (w *Worker) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.status == WorkerStatusWorking {
		return fmt.Errorf("worker %s is already running", w.id)
	}

	w.status = WorkerStatusIdle
	w.lastActive = time.Now()

	log.Printf("Worker %s started for agent %s using provider %s", w.id, w.agent.Name, w.provider.Config.Name)

	// Worker is now ready to receive tasks
	// The actual task processing will be handled by the pool

	return nil
}

// Stop stops the worker
func (w *Worker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.cancel()
	w.status = WorkerStatusStopped

	log.Printf("Worker %s stopped", w.id)
}

// SetDatabase sets the database for conversation context management
func (w *Worker) SetDatabase(db *database.Database) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.db = db
}

// ExecuteTask executes a task using the agent's persona and provider
// Supports multi-turn conversations when ConversationSession is provided or database is available
func (w *Worker) ExecuteTask(ctx context.Context, task *Task) (*TaskResult, error) {
	w.mu.Lock()
	if w.status != WorkerStatusIdle {
		w.mu.Unlock()
		return nil, fmt.Errorf("worker %s is not idle", w.id)
	}
	w.status = WorkerStatusWorking
	w.currentTask = task.ID
	w.lastActive = time.Now()
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.status = WorkerStatusIdle
		w.currentTask = ""
		w.lastActive = time.Now()
		w.mu.Unlock()
	}()

	// Try to load or create conversation context
	var messages []provider.ChatMessage
	var conversationCtx *models.ConversationContext
	var err error

	if task.ConversationSession != nil {
		// Use provided conversation session
		conversationCtx = task.ConversationSession
	} else if w.db != nil && task.BeadID != "" && task.ProjectID != "" {
		// Try to load existing conversation from database
		conversationCtx, err = w.db.GetConversationContextByBeadID(task.BeadID)
		if err != nil {
			// No existing conversation, create new one
			log.Printf("No existing conversation for bead %s, creating new session", task.BeadID)
			conversationCtx = models.NewConversationContext(
				uuid.New().String(),
				task.BeadID,
				task.ProjectID,
				24*time.Hour, // Default 24h expiration
			)

			// Save new session to database
			if err := w.db.CreateConversationContext(conversationCtx); err != nil {
				log.Printf("Warning: Failed to create conversation context: %v", err)
				conversationCtx = nil // Fall back to single-shot
			}
		} else if conversationCtx.IsExpired() {
			// Session expired, create new one
			log.Printf("Conversation session %s expired, creating new session", conversationCtx.SessionID)
			conversationCtx = models.NewConversationContext(
				uuid.New().String(),
				task.BeadID,
				task.ProjectID,
				24*time.Hour,
			)

			if err := w.db.CreateConversationContext(conversationCtx); err != nil {
				log.Printf("Warning: Failed to create conversation context: %v", err)
				conversationCtx = nil
			}
		}
	}

	// Build message history
	if conversationCtx != nil {
		// Multi-turn conversation mode
		messages = w.buildConversationMessages(conversationCtx, task)

		// Handle token limits
		messages = w.handleTokenLimits(messages)
	} else {
		// Single-shot mode (backward compatibility)
		messages = w.buildSingleShotMessages(task)
	}

	// Create chat completion request
	req := &provider.ChatCompletionRequest{
		Model:          w.provider.Config.Model,
		Messages:       messages,
		Temperature:    0.7,
		ResponseFormat: &provider.ResponseFormat{Type: "json_object"},
	}

	// Send request to provider
	resp, err := w.provider.Protocol.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get completion: %w", err)
	}

	// Extract result from response
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from provider")
	}

	// Store assistant response in conversation context
	if conversationCtx != nil && w.db != nil {
		// Convert provider messages back to conversation messages
		for _, msg := range messages {
			// Only add new messages (not already in history)
			if len(conversationCtx.Messages) == 0 ||
			   !w.messageExists(conversationCtx.Messages, msg.Content) {
				conversationCtx.AddMessage(msg.Role, msg.Content, len(msg.Content)/4)
			}
		}

		// Add assistant response
		conversationCtx.AddMessage(
			"assistant",
			resp.Choices[0].Message.Content,
			resp.Usage.CompletionTokens,
		)

		// Update conversation context in database
		if err := w.db.UpdateConversationContext(conversationCtx); err != nil {
			log.Printf("Warning: Failed to update conversation context: %v", err)
		}
	}

	result := &TaskResult{
		TaskID:      task.ID,
		WorkerID:    w.id,
		AgentID:     w.agent.ID,
		Response:    resp.Choices[0].Message.Content,
		TokensUsed:  resp.Usage.TotalTokens,
		CompletedAt: time.Now(),
		Success:     true,
	}

	return result, nil
}

// buildConversationMessages builds messages from conversation history + new task
func (w *Worker) buildConversationMessages(conversationCtx *models.ConversationContext, task *Task) []provider.ChatMessage {
	var messages []provider.ChatMessage

	// If no messages in history, add system prompt
	if len(conversationCtx.Messages) == 0 {
		systemPrompt := w.buildSystemPrompt()
		conversationCtx.AddMessage("system", systemPrompt, len(systemPrompt)/4)
	}

	// Convert conversation messages to provider messages
	for _, msg := range conversationCtx.Messages {
		messages = append(messages, provider.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	// Append new user message
	userPrompt := task.Description
	if task.Context != "" {
		userPrompt = fmt.Sprintf("%s\n\nContext:\n%s", userPrompt, task.Context)
	}

	messages = append(messages, provider.ChatMessage{
		Role:    "user",
		Content: userPrompt,
	})

	return messages
}

// buildSingleShotMessages builds messages for single-shot execution (no conversation history)
func (w *Worker) buildSingleShotMessages(task *Task) []provider.ChatMessage {
	systemPrompt := w.buildSystemPrompt()
	userPrompt := task.Description
	if task.Context != "" {
		userPrompt = fmt.Sprintf("%s\n\nContext:\n%s", userPrompt, task.Context)
	}

	return []provider.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
}

// handleTokenLimits truncates messages if they exceed model token limits
func (w *Worker) handleTokenLimits(messages []provider.ChatMessage) []provider.ChatMessage {
	// Get model token limit (default to 100K if not specified)
	modelLimit := w.getModelTokenLimit()
	maxTokens := int(float64(modelLimit) * 0.8) // Use 80% of limit

	// Calculate current tokens (rough estimate: 1 token ~= 4 characters)
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += len(msg.Content) / 4
	}

	if totalTokens <= maxTokens {
		return messages // No truncation needed
	}

	// Strategy: Sliding window - keep system message + recent messages
	if len(messages) == 0 {
		return messages
	}

	systemMsg := messages[0] // Assume first message is system
	systemTokens := len(systemMsg.Content) / 4

	// Find how many recent messages we can keep
	recentTokens := 0
	startIndex := len(messages) // Start from end

	// Work backwards to find where to truncate
	for i := len(messages) - 1; i > 0; i-- {
		msgTokens := len(messages[i].Content) / 4
		if systemTokens+recentTokens+msgTokens > maxTokens {
			// Can't fit this message
			startIndex = i + 1
			break
		}
		recentTokens += msgTokens
	}

	// If we truncated messages, add notice
	if startIndex > 1 {
		truncatedCount := startIndex - 1 // Don't count system message
		noticeMsg := provider.ChatMessage{
			Role:    "system",
			Content: fmt.Sprintf("[Note: %d older messages truncated to stay within token limit]", truncatedCount),
		}

		// Build result: system message + notice + recent messages
		result := []provider.ChatMessage{systemMsg, noticeMsg}
		result = append(result, messages[startIndex:]...)
		return result
	}

	// No truncation needed (edge case)
	return messages
}

// getModelTokenLimit returns the token limit for the current model
func (w *Worker) getModelTokenLimit() int {
	// Default limits for common models
	// TODO: Make this configurable via provider config
	modelLimits := map[string]int{
		"gpt-4":             8192,
		"gpt-4-32k":         32768,
		"gpt-4-turbo":       128000,
		"gpt-3.5-turbo":     4096,
		"gpt-3.5-turbo-16k": 16384,
		"claude-3-opus":     200000,
		"claude-3-sonnet":   200000,
		"claude-3-haiku":    200000,
	}

	if limit, ok := modelLimits[w.provider.Config.Model]; ok {
		return limit
	}

	// Default to 100K for unknown models
	return 100000
}

// messageExists checks if a message with the same content already exists in history
func (w *Worker) messageExists(messages []models.ChatMessage, content string) bool {
	for _, msg := range messages {
		if msg.Content == content {
			return true
		}
	}
	return false
}

// buildSystemPrompt builds the system prompt from the agent's persona
func (w *Worker) buildSystemPrompt() string {
	if w.agent.Persona == nil {
		return fmt.Sprintf("You are %s, an AI agent.", w.agent.Name)
	}

	persona := w.agent.Persona
	prompt := ""

	// Add identity
	if persona.Character != "" {
		prompt += fmt.Sprintf("# Your Character\n%s\n\n", persona.Character)
	}

	// Add mission
	if persona.Mission != "" {
		prompt += fmt.Sprintf("# Your Mission\n%s\n\n", persona.Mission)
	}

	// Add personality
	if persona.Personality != "" {
		prompt += fmt.Sprintf("# Your Personality\n%s\n\n", persona.Personality)
	}

	// Add capabilities
	if len(persona.Capabilities) > 0 {
		prompt += "# Your Capabilities\n"
		for _, cap := range persona.Capabilities {
			prompt += fmt.Sprintf("- %s\n", cap)
		}
		prompt += "\n"
	}

	// Add autonomy instructions
	if persona.AutonomyInstructions != "" {
		prompt += fmt.Sprintf("# Autonomy Guidelines\n%s\n\n", persona.AutonomyInstructions)
	}

	// Add decision instructions
	if persona.DecisionInstructions != "" {
		prompt += fmt.Sprintf("# Decision Making\n%s\n\n", persona.DecisionInstructions)
	}

	prompt += fmt.Sprintf("# Required Output Format\n%s\n\n", actions.ActionPrompt)

	return prompt
}

// GetStatus returns the current worker status
func (w *Worker) GetStatus() WorkerStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.status
}

// GetInfo returns worker information
func (w *Worker) GetInfo() WorkerInfo {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return WorkerInfo{
		ID:          w.id,
		AgentName:   w.agent.Name,
		PersonaName: w.agent.PersonaName,
		ProviderID:  w.provider.Config.ID,
		Status:      w.status,
		CurrentTask: w.currentTask,
		StartedAt:   w.startedAt,
		LastActive:  w.lastActive,
	}
}

// Task represents a task for a worker to execute
type Task struct {
	ID                  string
	Description         string
	Context             string
	BeadID              string
	ProjectID           string
	ConversationSession *models.ConversationContext // Optional: enables multi-turn conversation
}

// TaskResult represents the result of task execution
type TaskResult struct {
	TaskID             string
	WorkerID           string
	AgentID            string
	Response           string
	Actions            []actions.Result
	TokensUsed         int
	CompletedAt        time.Time
	Success            bool
	Error              string
	LoopIterations     int    // Set when action loop is used
	LoopTerminalReason string // Set when action loop is used
}

// WorkerInfo contains information about a worker
type WorkerInfo struct {
	ID          string
	AgentName   string
	PersonaName string
	ProviderID  string
	Status      WorkerStatus
	CurrentTask string
	StartedAt   time.Time
	LastActive  time.Time
}

// --- Multi-turn action loop ---

// LessonsProvider supplies and records project-specific lessons.
type LessonsProvider interface {
	GetLessonsForPrompt(projectID string) string
	RecordLesson(projectID, category, title, detail, beadID, agentID string) error
}

// LoopConfig configures the multi-turn action loop.
type LoopConfig struct {
	MaxIterations   int
	Router          *actions.Router
	ActionContext   actions.ActionContext
	LessonsProvider LessonsProvider
	DB              *database.Database
}

// LoopResult contains the result of a multi-turn action loop.
type LoopResult struct {
	*TaskResult
	Iterations     int              `json:"iterations"`
	TerminalReason string           `json:"terminal_reason"` // "completed", "max_iterations", "escalated", "error", "no_actions", "parse_failures"
	ActionLog      []ActionLogEntry `json:"action_log"`
}

// ActionLogEntry records a single iteration of the action loop.
type ActionLogEntry struct {
	Iteration int              `json:"iteration"`
	Actions   []actions.Action `json:"actions"`
	Results   []actions.Result `json:"results"`
	Timestamp time.Time        `json:"timestamp"`
}

// ExecuteTaskWithLoop runs the task in a multi-turn action loop:
// call LLM → parse actions → execute → format results → feed back → repeat.
func (w *Worker) ExecuteTaskWithLoop(ctx context.Context, task *Task, config *LoopConfig) (*LoopResult, error) {
	w.mu.Lock()
	if w.status != WorkerStatusIdle {
		w.mu.Unlock()
		return nil, fmt.Errorf("worker %s is not idle", w.id)
	}
	w.status = WorkerStatusWorking
	w.currentTask = task.ID
	w.lastActive = time.Now()
	w.mu.Unlock()

	defer func() {
		w.mu.Lock()
		w.status = WorkerStatusIdle
		w.currentTask = ""
		w.lastActive = time.Now()
		w.mu.Unlock()
	}()

	maxIter := config.MaxIterations
	if maxIter <= 0 {
		maxIter = 15
	}

	// Build initial messages
	var messages []provider.ChatMessage
	var conversationCtx *models.ConversationContext

	if task.ConversationSession != nil {
		conversationCtx = task.ConversationSession
	} else if config.DB != nil && task.BeadID != "" && task.ProjectID != "" {
		var err error
		conversationCtx, err = config.DB.GetConversationContextByBeadID(task.BeadID)
		if err != nil {
			conversationCtx = models.NewConversationContext(
				uuid.New().String(), task.BeadID, task.ProjectID, 24*time.Hour,
			)
			if createErr := config.DB.CreateConversationContext(conversationCtx); createErr != nil {
				log.Printf("[ActionLoop] Warning: Failed to create conversation context: %v", createErr)
				conversationCtx = nil
			}
		} else if conversationCtx != nil && conversationCtx.IsExpired() {
			conversationCtx = models.NewConversationContext(
				uuid.New().String(), task.BeadID, task.ProjectID, 24*time.Hour,
			)
			if createErr := config.DB.CreateConversationContext(conversationCtx); createErr != nil {
				conversationCtx = nil
			}
		}
	}

	// Build system prompt with lessons
	systemPrompt := w.buildEnhancedSystemPrompt(config.LessonsProvider, task.ProjectID, task.Context)

	if conversationCtx != nil {
		if len(conversationCtx.Messages) == 0 {
			conversationCtx.AddMessage("system", systemPrompt, len(systemPrompt)/4)
		}
		for _, msg := range conversationCtx.Messages {
			messages = append(messages, provider.ChatMessage{Role: msg.Role, Content: msg.Content})
		}
		userPrompt := task.Description
		if task.Context != "" {
			userPrompt = fmt.Sprintf("%s\n\nContext:\n%s", userPrompt, task.Context)
		}
		messages = append(messages, provider.ChatMessage{Role: "user", Content: userPrompt})
	} else {
		userPrompt := task.Description
		if task.Context != "" {
			userPrompt = fmt.Sprintf("%s\n\nContext:\n%s", userPrompt, task.Context)
		}
		messages = []provider.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}
	}

	loopResult := &LoopResult{
		TaskResult: &TaskResult{
			TaskID:   task.ID,
			WorkerID: w.id,
			AgentID:  w.agent.ID,
			Success:  true,
		},
	}

	var allActions []actions.Result
	consecutiveParseFailures := 0
	consecutiveValidationFailures := 0
	actionHashes := make(map[string]int) // for inner loop detection

	for iteration := 0; iteration < maxIter; iteration++ {
		select {
		case <-ctx.Done():
			loopResult.TerminalReason = "context_canceled"
			loopResult.Iterations = iteration
			loopResult.Actions = allActions
			loopResult.CompletedAt = time.Now()
			return loopResult, ctx.Err()
		default:
		}

		// Handle token limits
		trimmedMessages := w.handleTokenLimits(messages)

		req := &provider.ChatCompletionRequest{
			Model:          w.provider.Config.Model,
			Messages:       trimmedMessages,
			Temperature:    0.7,
			ResponseFormat: &provider.ResponseFormat{Type: "json_object"},
		}

		log.Printf("[ActionLoop] Iteration %d/%d for task %s (messages: %d)", iteration+1, maxIter, task.ID, len(trimmedMessages))

		resp, err := w.provider.Protocol.CreateChatCompletion(ctx, req)
		if err != nil {
			loopResult.TerminalReason = "error"
			loopResult.Iterations = iteration + 1
			loopResult.Actions = allActions
			loopResult.Success = false
			loopResult.Error = err.Error()
			loopResult.CompletedAt = time.Now()
			return loopResult, fmt.Errorf("LLM call failed on iteration %d: %w", iteration+1, err)
		}

		if len(resp.Choices) == 0 {
			loopResult.TerminalReason = "error"
			loopResult.Iterations = iteration + 1
			loopResult.Actions = allActions
			loopResult.Success = false
			loopResult.Error = "no response from provider"
			loopResult.CompletedAt = time.Now()
			return loopResult, fmt.Errorf("no response from provider on iteration %d", iteration+1)
		}

		llmResponse := resp.Choices[0].Message.Content
		loopResult.Response = llmResponse
		loopResult.TokensUsed += resp.Usage.TotalTokens

		// Add assistant message to conversation
		messages = append(messages, provider.ChatMessage{Role: "assistant", Content: llmResponse})
		if conversationCtx != nil {
			conversationCtx.AddMessage("assistant", llmResponse, resp.Usage.CompletionTokens)
		}

		// Parse actions
		env, parseErr := actions.DecodeLenient([]byte(llmResponse))
		if parseErr != nil {
			var validationErr *actions.ValidationError
			if errors.As(parseErr, &validationErr) {
				// JSON parsed fine but action fields are incomplete.
				// Give specific feedback and let the model retry — don't count
				// this as a hard parse failure.
				consecutiveValidationFailures++
				if consecutiveValidationFailures >= 4 {
					loopResult.TerminalReason = "validation_failures"
					loopResult.Iterations = iteration + 1
					loopResult.Actions = allActions
					loopResult.Success = false
					loopResult.Error = fmt.Sprintf("repeated validation failures: %v", validationErr)
					loopResult.CompletedAt = time.Now()
					return loopResult, nil
				}
				feedback := fmt.Sprintf("## Action Validation Error\n\nYour JSON was valid but the action is incomplete: %v\n\nPlease include all required fields. For write_file you need both \"path\" and \"content\". For read_code you need \"path\". Check the action schema and try again.", validationErr)
				messages = append(messages, provider.ChatMessage{Role: "user", Content: feedback})
				if conversationCtx != nil {
					conversationCtx.AddMessage("user", feedback, len(feedback)/4)
				}
				log.Printf("[ActionLoop] Validation error on iteration %d: %v", iteration+1, validationErr)
				continue
			}

			// Actual JSON parse failure
			consecutiveParseFailures++
			if consecutiveParseFailures >= 2 {
				loopResult.TerminalReason = "parse_failures"
				loopResult.Iterations = iteration + 1
				loopResult.Actions = allActions
				loopResult.Success = false
				loopResult.Error = fmt.Sprintf("two consecutive parse failures: %v", parseErr)
				loopResult.CompletedAt = time.Now()
				return loopResult, nil
			}

			feedback := fmt.Sprintf("## Parse Error\n\nFailed to parse your response as valid JSON actions: %v\n\nPlease respond with a valid JSON object containing an \"actions\" array. Do not include any text outside the JSON.", parseErr)
			messages = append(messages, provider.ChatMessage{Role: "user", Content: feedback})
			if conversationCtx != nil {
				conversationCtx.AddMessage("user", feedback, len(feedback)/4)
			}
			log.Printf("[ActionLoop] Parse error on iteration %d: %v", iteration+1, parseErr)
			continue
		}
		consecutiveParseFailures = 0
		consecutiveValidationFailures = 0

		// Check for empty actions (agent just provided analysis)
		if len(env.Actions) == 0 {
			loopResult.TerminalReason = "no_actions"
			loopResult.Iterations = iteration + 1
			loopResult.Actions = allActions
			loopResult.CompletedAt = time.Now()
			return loopResult, nil
		}

		// Execute actions
		results, execErr := config.Router.Execute(ctx, env, config.ActionContext)
		if execErr != nil {
			loopResult.TerminalReason = "error"
			loopResult.Iterations = iteration + 1
			loopResult.Actions = allActions
			loopResult.Success = false
			loopResult.Error = execErr.Error()
			loopResult.CompletedAt = time.Now()
			return loopResult, nil
		}

		allActions = append(allActions, results...)

		// Log the iteration
		loopResult.ActionLog = append(loopResult.ActionLog, ActionLogEntry{
			Iteration: iteration + 1,
			Actions:   env.Actions,
			Results:   results,
			Timestamp: time.Now(),
		})

		// Check for terminal actions
		termReason := checkTerminalCondition(env, results)
		if termReason != "" {
			loopResult.TerminalReason = termReason
			loopResult.Iterations = iteration + 1
			loopResult.Actions = allActions
			loopResult.CompletedAt = time.Now()

			// Record lessons from build failures
			w.recordBuildLessons(config, env, results)

			break
		}

		// Record lessons from build failures even on non-terminal iterations
		w.recordBuildLessons(config, env, results)

		// Inner loop detection: hash the actions and check for repeats
		hash := hashActions(env.Actions)
		actionHashes[hash]++
		if actionHashes[hash] >= 10 {
			loopResult.TerminalReason = "inner_loop"
			loopResult.Iterations = iteration + 1
			loopResult.Actions = allActions
			loopResult.Success = false
			loopResult.Error = "detected stuck inner loop (same actions repeated 10 times)"
			loopResult.CompletedAt = time.Now()

			if config.LessonsProvider != nil {
				_ = config.LessonsProvider.RecordLesson(
					task.ProjectID, "loop_pattern",
					"Agent stuck in action loop",
					fmt.Sprintf("Agent repeated the same actions 10 times. Actions hash: %s", hash),
					task.BeadID, w.agent.ID,
				)
			}
			return loopResult, nil
		}
		if actionHashes[hash] >= 5 {
			log.Printf("[ActionLoop] Warning: same actions repeated %d times (hash %s)", actionHashes[hash], hash[:8])
		}

		// Format results as user message and continue
		feedback := actions.FormatResultsAsUserMessage(results)
		messages = append(messages, provider.ChatMessage{Role: "user", Content: feedback})
		if conversationCtx != nil {
			conversationCtx.AddMessage("user", feedback, len(feedback)/4)
		}

		// Persist conversation context periodically
		if conversationCtx != nil && config.DB != nil && (iteration%3 == 2 || iteration == maxIter-1) {
			if err := config.DB.UpdateConversationContext(conversationCtx); err != nil {
				log.Printf("[ActionLoop] Warning: Failed to persist conversation: %v", err)
			}
		}
	}

	// If we exhausted iterations without terminal condition
	if loopResult.TerminalReason == "" {
		loopResult.TerminalReason = "max_iterations"
		loopResult.Iterations = maxIter
		loopResult.Actions = allActions
		loopResult.CompletedAt = time.Now()
	}

	// Final persist
	if conversationCtx != nil && config.DB != nil {
		if err := config.DB.UpdateConversationContext(conversationCtx); err != nil {
			log.Printf("[ActionLoop] Warning: Failed to persist final conversation: %v", err)
		}
	}

	return loopResult, nil
}

// buildEnhancedSystemPrompt builds the system prompt with lessons and progress context.
func (w *Worker) buildEnhancedSystemPrompt(lp LessonsProvider, projectID, progressCtx string) string {
	persona := w.agent.Persona
	prompt := ""

	if persona == nil {
		prompt = fmt.Sprintf("You are %s, an AI agent.\n\n", w.agent.Name)
	} else {
		if persona.Character != "" {
			prompt += fmt.Sprintf("# Your Character\n%s\n\n", persona.Character)
		}
		if persona.Mission != "" {
			prompt += fmt.Sprintf("# Your Mission\n%s\n\n", persona.Mission)
		}
		if persona.Personality != "" {
			prompt += fmt.Sprintf("# Your Personality\n%s\n\n", persona.Personality)
		}
		if len(persona.Capabilities) > 0 {
			prompt += "# Your Capabilities\n"
			for _, cap := range persona.Capabilities {
				prompt += fmt.Sprintf("- %s\n", cap)
			}
			prompt += "\n"
		}
		if persona.AutonomyInstructions != "" {
			prompt += fmt.Sprintf("# Autonomy Guidelines\n%s\n\n", persona.AutonomyInstructions)
		}
		if persona.DecisionInstructions != "" {
			prompt += fmt.Sprintf("# Decision Making\n%s\n\n", persona.DecisionInstructions)
		}
	}

	// Get lessons
	var lessons string
	if lp != nil && projectID != "" {
		lessons = lp.GetLessonsForPrompt(projectID)
	}

	prompt += fmt.Sprintf("# Required Output Format\n%s\n\n", actions.BuildEnhancedPrompt(lessons, progressCtx))

	return prompt
}

// checkTerminalCondition checks if any action in the envelope signals termination.
func checkTerminalCondition(env *actions.ActionEnvelope, results []actions.Result) string {
	for _, a := range env.Actions {
		switch a.Type {
		case actions.ActionCloseBead:
			return "completed"
		case actions.ActionDone:
			return "completed"
		case actions.ActionEscalateCEO:
			return "escalated"
		}
	}
	return ""
}

// recordBuildLessons checks action results for build/test failures and records lessons.
func (w *Worker) recordBuildLessons(config *LoopConfig, env *actions.ActionEnvelope, results []actions.Result) {
	if config.LessonsProvider == nil {
		return
	}

	for i, r := range results {
		if r.Status != "error" && r.Status != "executed" {
			continue
		}

		var category, title, detail string

		switch r.ActionType {
		case actions.ActionBuildProject:
			if r.Status == "error" || (r.Metadata != nil && r.Metadata["success"] == false) {
				category = "compiler_error"
				title = "Build failure"
				output, _ := r.Metadata["output"].(string)
				if output == "" {
					output = r.Message
				}
				detail = truncateForLesson(output)
			}
		case actions.ActionRunTests:
			if r.Status == "error" || (r.Metadata != nil && r.Metadata["success"] == false) {
				category = "test_failure"
				title = "Test failure"
				output, _ := r.Metadata["output"].(string)
				if output == "" {
					output = r.Message
				}
				detail = truncateForLesson(output)
			}
		case actions.ActionApplyPatch, actions.ActionEditCode:
			if r.Status == "error" {
				category = "edit_failure"
				title = "Patch/edit failure"
				detail = truncateForLesson(r.Message)
			}
		}

		if category != "" {
			_ = i // suppress unused warning
			_ = config.LessonsProvider.RecordLesson(
				config.ActionContext.ProjectID,
				category, title, detail,
				config.ActionContext.BeadID,
				w.agent.ID,
			)
		}
	}
}

func truncateForLesson(s string) string {
	if len(s) <= 500 {
		return s
	}
	return s[:500]
}

// hashActions computes a deterministic hash of action types and key fields.
func hashActions(acts []actions.Action) string {
	var sb strings.Builder
	for _, a := range acts {
		sb.WriteString(a.Type)
		sb.WriteString("|")
		sb.WriteString(a.Path)
		sb.WriteString("|")
		sb.WriteString(a.Command)
		sb.WriteString("|")
	}
	h := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(h[:8])
}
