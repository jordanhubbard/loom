package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jordanhubbard/arbiter/internal/agent"
	"github.com/jordanhubbard/arbiter/internal/persona"
	"github.com/jordanhubbard/arbiter/internal/provider"
	"github.com/jordanhubbard/arbiter/internal/worker"
	"github.com/jordanhubbard/arbiter/pkg/models"
)

// TestWorkerSystemIntegration tests the complete workflow:
// 1. Register providers
// 2. Load personas
// 3. Spawn agents with workers
// 4. Execute tasks
// 5. Verify results
func TestWorkerSystemIntegration(t *testing.T) {
	// Skip in short mode as this is an integration test
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Step 1: Create provider registry
	registry := provider.NewRegistry()

	// Register a mock provider (in real usage, this would be OpenAI, etc.)
	mockProviderConfig := &provider.ProviderConfig{
		ID:       "mock-provider",
		Name:     "Mock Provider",
		Type:     "custom",
		Endpoint: "http://localhost:8888/v1",  // Mock endpoint
		APIKey:   "mock-key",
		Model:    "mock-model",
	}

	err := registry.Register(mockProviderConfig)
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Step 2: Create worker manager
	maxAgents := 5
	workerManager := agent.NewWorkerManager(maxAgents, registry)

	// Step 3: Load a test persona
	_ = persona.NewManager("../../personas") // persona manager available if needed
	testPersona := &models.Persona{
		Name:         "test-agent",
		Character:    "A helpful test agent",
		Mission:      "Execute test tasks",
		Capabilities: []string{"testing", "verification"},
	}

	// Step 4: Spawn an agent with worker
	// Note: This will fail to connect to the mock provider, but tests the workflow
	spawnedAgent, err := workerManager.SpawnAgentWorker(
		ctx,
		"test-agent-1",
		"test-agent",
		"test-project",
		"mock-provider",
		testPersona,
	)

	if err != nil {
		t.Fatalf("Failed to spawn agent worker: %v", err)
	}

	if spawnedAgent == nil {
		t.Fatal("Spawned agent is nil")
	}

	// Verify agent was created
	if spawnedAgent.Name != "test-agent-1" {
		t.Errorf("Expected agent name 'test-agent-1', got '%s'", spawnedAgent.Name)
	}

	if spawnedAgent.PersonaName != "test-agent" {
		t.Errorf("Expected persona name 'test-agent', got '%s'", spawnedAgent.PersonaName)
	}

	if spawnedAgent.Status != "idle" {
		t.Errorf("Expected status 'idle', got '%s'", spawnedAgent.Status)
	}

	// Step 5: Verify worker was created in pool
	workerPool := workerManager.GetWorkerPool()
	workers := workerPool.ListWorkers()

	if len(workers) != 1 {
		t.Errorf("Expected 1 worker, got %d", len(workers))
	}

	// Step 6: Get pool stats
	stats := workerManager.GetPoolStats()
	if stats.TotalWorkers != 1 {
		t.Errorf("Expected 1 total worker, got %d", stats.TotalWorkers)
	}

	if stats.IdleWorkers != 1 {
		t.Errorf("Expected 1 idle worker, got %d", stats.IdleWorkers)
	}

	// Step 7: List agents
	agents := workerManager.ListAgents()
	if len(agents) != 1 {
		t.Errorf("Expected 1 agent, got %d", len(agents))
	}

	// Step 8: Clean up - stop the agent
	err = workerManager.StopAgent(spawnedAgent.ID)
	if err != nil {
		t.Errorf("Failed to stop agent: %v", err)
	}

	// Verify agent was removed
	agents = workerManager.ListAgents()
	if len(agents) != 0 {
		t.Errorf("Expected 0 agents after stop, got %d", len(agents))
	}
}

// TestMultipleAgentsWorkflow tests spawning multiple agents
func TestMultipleAgentsWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create registry and register provider
	registry := provider.NewRegistry()
	err := registry.Register(&provider.ProviderConfig{
		ID:       "test-provider",
		Name:     "Test Provider",
		Type:     "openai",
		Endpoint: "http://localhost:8888/v1",
		APIKey:   "test-key",
		Model:    "test-model",
	})
	if err != nil {
		t.Fatalf("Failed to register provider: %v", err)
	}

	// Create worker manager
	workerManager := agent.NewWorkerManager(10, registry)

	// Create test personas
	persona1 := &models.Persona{Name: "agent-1", Mission: "Task 1"}
	persona2 := &models.Persona{Name: "agent-2", Mission: "Task 2"}
	persona3 := &models.Persona{Name: "agent-3", Mission: "Task 3"}

	// Spawn multiple agents
	agent1, err := workerManager.SpawnAgentWorker(ctx, "agent-1", "persona-1", "project-1", "test-provider", persona1)
	if err != nil {
		t.Fatalf("Failed to spawn agent 1: %v", err)
	}

	agent2, err := workerManager.SpawnAgentWorker(ctx, "agent-2", "persona-2", "project-1", "test-provider", persona2)
	if err != nil {
		t.Fatalf("Failed to spawn agent 2: %v", err)
	}

	agent3, err := workerManager.SpawnAgentWorker(ctx, "agent-3", "persona-3", "project-2", "test-provider", persona3)
	if err != nil {
		t.Fatalf("Failed to spawn agent 3: %v", err)
	}

	// Verify all agents were created
	agents := workerManager.ListAgents()
	if len(agents) != 3 {
		t.Errorf("Expected 3 agents, got %d", len(agents))
	}

	// Verify pool stats
	stats := workerManager.GetPoolStats()
	if stats.TotalWorkers != 3 {
		t.Errorf("Expected 3 total workers, got %d", stats.TotalWorkers)
	}

	// Test filtering by project
	project1Agents := workerManager.ListAgentsByProject("project-1")
	if len(project1Agents) != 2 {
		t.Errorf("Expected 2 agents for project-1, got %d", len(project1Agents))
	}

	project2Agents := workerManager.ListAgentsByProject("project-2")
	if len(project2Agents) != 1 {
		t.Errorf("Expected 1 agent for project-2, got %d", len(project2Agents))
	}

	// Clean up all agents
	workerManager.StopAgent(agent1.ID)
	workerManager.StopAgent(agent2.ID)
	workerManager.StopAgent(agent3.ID)

	// Verify cleanup
	agents = workerManager.ListAgents()
	if len(agents) != 0 {
		t.Errorf("Expected 0 agents after cleanup, got %d", len(agents))
	}
}

// TestProviderRegistry tests the provider registry functionality
func TestProviderRegistry(t *testing.T) {
	registry := provider.NewRegistry()

	// Register multiple providers
	providers := []*provider.ProviderConfig{
		{
			ID:       "openai",
			Name:     "OpenAI",
			Type:     "openai",
			Endpoint: "https://api.openai.com/v1",
			APIKey:   "test-key-1",
			Model:    "gpt-4",
		},
		{
			ID:       "anthropic",
			Name:     "Anthropic",
			Type:     "anthropic",
			Endpoint: "https://api.anthropic.com/v1",
			APIKey:   "test-key-2",
			Model:    "claude-3",
		},
		{
			ID:       "ollama",
			Name:     "Ollama Local",
			Type:     "local",
			Endpoint: "http://localhost:11434/v1",
			APIKey:   "",
			Model:    "llama2",
		},
	}

	// Register all providers
	for _, config := range providers {
		err := registry.Register(config)
		if err != nil {
			t.Fatalf("Failed to register provider %s: %v", config.ID, err)
		}
	}

	// Verify all were registered
	registered := registry.List()
	if len(registered) != 3 {
		t.Errorf("Expected 3 providers, got %d", len(registered))
	}

	// Test retrieval
	openai, err := registry.Get("openai")
	if err != nil {
		t.Fatalf("Failed to get OpenAI provider: %v", err)
	}

	if openai.Config.Model != "gpt-4" {
		t.Errorf("Expected model 'gpt-4', got '%s'", openai.Config.Model)
	}

	// Test unregistering
	err = registry.Unregister("anthropic")
	if err != nil {
		t.Fatalf("Failed to unregister anthropic: %v", err)
	}

	registered = registry.List()
	if len(registered) != 2 {
		t.Errorf("Expected 2 providers after unregister, got %d", len(registered))
	}
}

// TestWorkerTaskExecution tests task execution (without actual API calls)
func TestWorkerTaskExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Setup
	registry := provider.NewRegistry()
	registry.Register(&provider.ProviderConfig{
		ID:       "test",
		Name:     "Test",
		Type:     "custom",
		Endpoint: "http://localhost:8888/v1",
		APIKey:   "test",
		Model:    "test",
	})

	workerManager := agent.NewWorkerManager(5, registry)
	persona := &models.Persona{
		Name:    "test",
		Mission: "test mission",
	}

	// Spawn agent
	testAgent, err := workerManager.SpawnAgentWorker(ctx, "test", "test", "test-project", "test", persona)
	if err != nil {
		t.Fatalf("Failed to spawn agent: %v", err)
	}

	// Create a task
	task := &worker.Task{
		ID:          "task-001",
		Description: "Test task",
		Context:     "Test context",
		ProjectID:   "test-project",
	}

	// Note: This will fail because there's no real API endpoint
	// But it tests that the task structure is correct
	result, err := workerManager.ExecuteTask(ctx, testAgent.ID, task)

	// We expect an error since the endpoint doesn't exist
	if err == nil {
		t.Error("Expected error when executing task with mock endpoint")
	}

	// Result should be nil on error
	if result != nil {
		t.Error("Expected nil result on error")
	}

	// Clean up
	workerManager.StopAgent(testAgent.ID)
}

// TestWorkerPoolLimits tests the worker pool limits
func TestWorkerPoolLimits(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(&provider.ProviderConfig{
		ID:       "test",
		Name:     "Test",
		Type:     "custom",
		Endpoint: "http://localhost:8888/v1",
		APIKey:   "test",
		Model:    "test",
	})

	// Create manager with max 2 workers
	maxWorkers := 2
	workerManager := agent.NewWorkerManager(maxWorkers, registry)

	ctx := context.Background()
	persona := &models.Persona{Name: "test"}

	// Spawn maximum number of workers
	_, err := workerManager.SpawnAgentWorker(ctx, "agent-1", "test", "project", "test", persona)
	if err != nil {
		t.Fatalf("Failed to spawn agent 1: %v", err)
	}

	_, err = workerManager.SpawnAgentWorker(ctx, "agent-2", "test", "project", "test", persona)
	if err != nil {
		t.Fatalf("Failed to spawn agent 2: %v", err)
	}

	// Try to spawn one more (should fail)
	_, err = workerManager.SpawnAgentWorker(ctx, "agent-3", "test", "project", "test", persona)
	if err == nil {
		t.Error("Expected error when exceeding max workers")
	}

	// Verify we have exactly max workers
	stats := workerManager.GetPoolStats()
	if stats.TotalWorkers != maxWorkers {
		t.Errorf("Expected %d workers, got %d", maxWorkers, stats.TotalWorkers)
	}

	// Clean up
	workerManager.StopAll()
}
