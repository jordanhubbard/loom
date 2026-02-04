package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jordanhubbard/agenticorp/internal/actions"
	"github.com/jordanhubbard/agenticorp/internal/database"
	"github.com/jordanhubbard/agenticorp/internal/provider"
	"github.com/jordanhubbard/agenticorp/pkg/models"
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
		Model:       w.provider.Config.Model,
		Messages:    messages,
		Temperature: 0.7,
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
	TaskID      string
	WorkerID    string
	AgentID     string
	Response    string
	Actions     []actions.Result
	TokensUsed  int
	CompletedAt time.Time
	Success     bool
	Error       string
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
