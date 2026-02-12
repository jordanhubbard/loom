package worker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jordanhubbard/loom/internal/database"
	"github.com/jordanhubbard/loom/internal/provider"
	"github.com/jordanhubbard/loom/pkg/models"
)

// MockProvider implements a simple mock provider for testing
type MockConversationProvider struct {
	responseContent string
	tokenCount      int
}

func (m *MockConversationProvider) CreateChatCompletion(ctx context.Context, req *provider.ChatCompletionRequest) (*provider.ChatCompletionResponse, error) {
	// Return a mock response
	return &provider.ChatCompletionResponse{
		ID:      "test-response",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []struct {
			Index   int                    `json:"index"`
			Message provider.ChatMessage   `json:"message"`
			Finish  string                 `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: provider.ChatMessage{
					Role:    "assistant",
					Content: m.responseContent,
				},
				Finish: "stop",
			},
		},
		Usage: struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     100,
			CompletionTokens: m.tokenCount,
			TotalTokens:      100 + m.tokenCount,
		},
	}, nil
}

func (m *MockConversationProvider) GetModels(ctx context.Context) ([]provider.Model, error) {
	return []provider.Model{}, nil
}

func TestWorker_ExecuteTask_WithConversationContext(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create mock provider
	mockProvider := &MockConversationProvider{
		responseContent: "Hello! How can I help you today?",
		tokenCount:      10,
	}

	registeredProvider := &provider.RegisteredProvider{
		Config: &provider.ProviderConfig{
			ID:    "test-provider",
			Name:  "Test Provider",
			Model: "test-model",
		},
		Protocol: mockProvider,
	}

	// Create test agent with persona
	agent := &models.Agent{
		ID:   "test-agent",
		Name: "Test Agent",
		Persona: &models.Persona{
			Character: "You are a helpful assistant",
			Mission:   "Help users with their tasks",
		},
	}

	// Create worker
	worker := NewWorker("worker-1", agent, registeredProvider)
	worker.SetDatabase(db)

	if err := worker.Start(); err != nil {
		t.Fatalf("Failed to start worker: %v", err)
	}
	defer worker.Stop()

	// Create task with bead and project IDs
	task := &Task{
		ID:          "task-1",
		Description: "What is the weather like?",
		BeadID:      "bead-test-123",
		ProjectID:   "proj-test-456",
	}

	ctx := context.Background()

	// Execute first task (creates new conversation)
	result1, err := worker.ExecuteTask(ctx, task)
	if err != nil {
		t.Fatalf("Failed to execute first task: %v", err)
	}

	if result1.Response != "Hello! How can I help you today?" {
		t.Errorf("Unexpected response: %s", result1.Response)
	}

	// Verify conversation was created in database
	conversation, err := db.GetConversationContextByBeadID(task.BeadID)
	if err != nil {
		t.Fatalf("Failed to get conversation context: %v", err)
	}

	// Should have system message + user message + assistant response
	if len(conversation.Messages) != 3 {
		t.Errorf("Expected 3 messages in conversation, got %d", len(conversation.Messages))
	}

	if conversation.Messages[0].Role != "system" {
		t.Errorf("First message should be system, got %s", conversation.Messages[0].Role)
	}
	if conversation.Messages[1].Role != "user" {
		t.Errorf("Second message should be user, got %s", conversation.Messages[1].Role)
	}
	if conversation.Messages[2].Role != "assistant" {
		t.Errorf("Third message should be assistant, got %s", conversation.Messages[2].Role)
	}

	// Execute second task (continues conversation)
	mockProvider.responseContent = "The weather is sunny today!"
	task2 := &Task{
		ID:          "task-2",
		Description: "Tell me more about the weather",
		BeadID:      "bead-test-123", // Same bead ID
		ProjectID:   "proj-test-456",
	}

	result2, err := worker.ExecuteTask(ctx, task2)
	if err != nil {
		t.Fatalf("Failed to execute second task: %v", err)
	}

	if result2.Response != "The weather is sunny today!" {
		t.Errorf("Unexpected response: %s", result2.Response)
	}

	// Verify conversation was updated
	conversation, err = db.GetConversationContextByBeadID(task.BeadID)
	if err != nil {
		t.Fatalf("Failed to get updated conversation context: %v", err)
	}

	// Should now have 5 messages (system + user1 + assistant1 + user2 + assistant2)
	if len(conversation.Messages) != 5 {
		t.Errorf("Expected 5 messages in conversation after second task, got %d", len(conversation.Messages))
	}

	if conversation.Messages[4].Role != "assistant" {
		t.Errorf("Last message should be assistant, got %s", conversation.Messages[4].Role)
	}
}

func TestWorker_ExecuteTask_SingleShot(t *testing.T) {
	// Test backward compatibility: worker without database should work in single-shot mode
	mockProvider := &MockConversationProvider{
		responseContent: "Single shot response",
		tokenCount:      5,
	}

	registeredProvider := &provider.RegisteredProvider{
		Config: &provider.ProviderConfig{
			ID:    "test-provider",
			Name:  "Test Provider",
			Model: "test-model",
		},
		Protocol: mockProvider,
	}

	agent := &models.Agent{
		ID:   "test-agent",
		Name: "Test Agent",
		Persona: &models.Persona{
			Character: "You are a helpful assistant",
		},
	}

	worker := NewWorker("worker-1", agent, registeredProvider)
	// No database set - should fall back to single-shot mode

	if err := worker.Start(); err != nil {
		t.Fatalf("Failed to start worker: %v", err)
	}
	defer worker.Stop()

	task := &Task{
		ID:          "task-1",
		Description: "Hello",
		BeadID:      "bead-1",
		ProjectID:   "proj-1",
	}

	ctx := context.Background()
	result, err := worker.ExecuteTask(ctx, task)
	if err != nil {
		t.Fatalf("Failed to execute task: %v", err)
	}

	if result.Response != "Single shot response" {
		t.Errorf("Unexpected response: %s", result.Response)
	}
}

func TestWorker_handleTokenLimits(t *testing.T) {
	agent := &models.Agent{
		ID:   "test-agent",
		Name: "Test Agent",
	}

	// Use a model with a 4096 token context window
	registeredProvider := &provider.RegisteredProvider{
		Config: &provider.ProviderConfig{
			ID:            "test-provider",
			Model:         "gpt-3.5-turbo",
			ContextWindow: 4096,
		},
	}

	worker := NewWorker("worker-1", agent, registeredProvider)

	// Create messages that exceed gpt-3.5-turbo's limit (4096 tokens, 80% = 3276 tokens)
	messages := []provider.ChatMessage{
		{Role: "system", Content: "You are a helpful assistant"}, // ~7 tokens
	}

	// Add many messages to exceed 3276 tokens
	// Each message is ~11 tokens, so we need about 300 messages
	for i := 0; i < 200; i++ {
		messages = append(messages, provider.ChatMessage{
			Role:    "user",
			Content: "This is a test message for truncation testing",
		})
		messages = append(messages, provider.ChatMessage{
			Role:    "assistant",
			Content: "This is a response to your test message",
		})
	}

	// Total: 1 system + 400 messages = 401 messages, ~4400 tokens

	// Should truncate to fit within 80% of 4096 = 3276 tokens
	truncated := worker.handleTokenLimits(messages)

	// Should have fewer messages than original
	if len(truncated) >= len(messages) {
		t.Errorf("Expected truncation, but got %d messages (same as input %d)", len(truncated), len(messages))
	}

	// First message should still be system
	if truncated[0].Role != "system" || truncated[0].Content != "You are a helpful assistant" {
		t.Errorf("First message should be original system message, got role=%s", truncated[0].Role)
	}

	// Should have truncation notice as second message
	if len(truncated) < 2 || truncated[1].Role != "system" || len(truncated[1].Content) < 6 || truncated[1].Content[:6] != "[Note:" {
		t.Error("Expected truncation notice as second message")
	}

	// Estimate total tokens (should be reasonably close to limit of 3276)
	// Allow some margin for the truncation notice itself
	totalTokens := 0
	for _, msg := range truncated {
		totalTokens += len(msg.Content) / 4
	}

	// Check that we're within a reasonable range (3276 + overhead for notice)
	if totalTokens > 3400 {
		t.Errorf("Token count should be close to 3276, got %d (too many)", totalTokens)
	}

	// Also verify we actually did truncate significantly
	if totalTokens > 4000 {
		t.Errorf("Truncation didn't work, still have %d tokens (original ~4400)", totalTokens)
	}
}

func TestWorker_buildConversationMessages(t *testing.T) {
	agent := &models.Agent{
		ID:   "test-agent",
		Name: "Test Agent",
		Persona: &models.Persona{
			Character: "You are a helpful assistant",
		},
	}

	registeredProvider := &provider.RegisteredProvider{
		Config: &provider.ProviderConfig{
			ID:    "test-provider",
			Model: "test-model",
		},
	}

	worker := NewWorker("worker-1", agent, registeredProvider)

	// Create conversation context with history
	conversationCtx := models.NewConversationContext(
		"session-1",
		"bead-1",
		"proj-1",
		24*time.Hour,
	)

	conversationCtx.AddMessage("system", "You are a helpful assistant", 7)
	conversationCtx.AddMessage("user", "What is 2+2?", 5)
	conversationCtx.AddMessage("assistant", "2+2 equals 4", 5)

	task := &Task{
		ID:          "task-1",
		Description: "What is 3+3?",
		Context:     "Continue the math questions",
	}

	messages := worker.buildConversationMessages(conversationCtx, task)

	// Should have 4 messages: system + user1 + assistant1 + user2 (new)
	if len(messages) != 4 {
		t.Errorf("Expected 4 messages, got %d", len(messages))
	}

	// Last message should be the new user message
	if messages[3].Role != "user" {
		t.Errorf("Last message should be user, got %s", messages[3].Role)
	}

	// Should include context
	if messages[3].Content != "What is 3+3?\n\nContext:\nContinue the math questions" {
		t.Errorf("User message should include context, got: %s", messages[3].Content)
	}
}

func TestWorker_ExpiredConversation(t *testing.T) {
	// Test that expired conversations create new sessions
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create expired conversation
	expiredConv := models.NewConversationContext(
		"expired-session",
		"bead-1",
		"proj-1",
		-1*time.Hour, // Already expired
	)
	expiredConv.AddMessage("system", "Old system message", 5)

	if err := db.CreateConversationContext(expiredConv); err != nil {
		t.Fatalf("Failed to create expired conversation: %v", err)
	}

	// Create worker
	mockProvider := &MockConversationProvider{
		responseContent: "New session response",
		tokenCount:      5,
	}

	registeredProvider := &provider.RegisteredProvider{
		Config: &provider.ProviderConfig{
			ID:    "test-provider",
			Model: "test-model",
		},
		Protocol: mockProvider,
	}

	agent := &models.Agent{
		ID:      "test-agent",
		Name:    "Test Agent",
		Persona: &models.Persona{Character: "Assistant"},
	}

	worker := NewWorker("worker-1", agent, registeredProvider)
	worker.SetDatabase(db)

	if err := worker.Start(); err != nil {
		t.Fatalf("Failed to start worker: %v", err)
	}
	defer worker.Stop()

	task := &Task{
		ID:          "task-1",
		Description: "Test message",
		BeadID:      "bead-1",
		ProjectID:   "proj-1",
	}

	ctx := context.Background()
	_, err = worker.ExecuteTask(ctx, task)
	if err != nil {
		t.Fatalf("Failed to execute task: %v", err)
	}

	// Should have created a new conversation (not the expired one)
	conversation, err := db.GetConversationContextByBeadID(task.BeadID)
	if err != nil {
		t.Fatalf("Failed to get conversation: %v", err)
	}

	// New session should have a different ID
	if conversation.SessionID == expiredConv.SessionID {
		t.Error("Should have created new session, but got the expired session ID")
	}

	// Should not be expired
	if conversation.IsExpired() {
		t.Error("New conversation should not be expired")
	}
}
