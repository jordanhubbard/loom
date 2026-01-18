package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jordanhubbard/arbiter/internal/provider"
	"github.com/jordanhubbard/arbiter/pkg/models"
)

// Worker represents an agent worker that processes tasks
type Worker struct {
	id          string
	agent       *models.Agent
	provider    *provider.RegisteredProvider
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

// ExecuteTask executes a task using the agent's persona and provider
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
	
	// Build the system prompt with persona information
	systemPrompt := w.buildSystemPrompt()
	
	// Build the user prompt with task information
	userPrompt := task.Description
	if task.Context != "" {
		userPrompt = fmt.Sprintf("%s\n\nContext:\n%s", userPrompt, task.Context)
	}
	
	// Create chat completion request
	req := &provider.ChatCompletionRequest{
		Model: w.provider.Config.Model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
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
	ID          string
	Description string
	Context     string
	BeadID      string
	ProjectID   string
}

// TaskResult represents the result of task execution
type TaskResult struct {
	TaskID      string
	WorkerID    string
	AgentID     string
	Response    string
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
