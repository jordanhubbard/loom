package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jordanhubbard/arbiter/internal/agent"
	"github.com/jordanhubbard/arbiter/internal/provider"
	"github.com/jordanhubbard/arbiter/internal/worker"
	"github.com/jordanhubbard/arbiter/pkg/models"
)

func main() {
	fmt.Println("=== Arbiter Worker System Demo ===")
	fmt.Println()

	// Check if API key is set
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Println("WARNING: OPENAI_API_KEY not set. Using mock endpoint.")
		fmt.Println("To use real OpenAI API: export OPENAI_API_KEY='your-key-here'")
		fmt.Println()
		apiKey = "mock-key"
	}

	ctx := context.Background()

	// Step 1: Create and configure provider registry
	fmt.Println("Step 1: Setting up provider registry...")
	registry := provider.NewRegistry()

	// Register OpenAI provider
	openaiConfig := &provider.ProviderConfig{
		ID:       "openai-gpt4",
		Name:     "OpenAI GPT-4",
		Type:     "openai",
		Endpoint: "https://api.openai.com/v1",
		APIKey:   apiKey,
		Model:    "gpt-4",
	}

	err := registry.Register(openaiConfig)
	if err != nil {
		log.Fatalf("Failed to register OpenAI provider: %v", err)
	}
	fmt.Printf("✓ Registered provider: %s\n", openaiConfig.Name)

	// Optionally register a local provider (if available)
	if os.Getenv("USE_OLLAMA") == "true" {
		ollamaConfig := &provider.ProviderConfig{
			ID:       "ollama-local",
			Name:     "Ollama Local",
			Type:     "local",
			Endpoint: "http://localhost:11434/v1",
			APIKey:   "",
			Model:    "llama2",
		}
		if err := registry.Register(ollamaConfig); err == nil {
			fmt.Printf("✓ Registered provider: %s\n", ollamaConfig.Name)
		}
	}

	fmt.Println()

	// Step 2: Create worker manager
	fmt.Println("Step 2: Creating worker manager...")
	maxWorkers := 5
	workerManager := agent.NewWorkerManager(maxWorkers, registry)
	fmt.Printf("✓ Worker manager created (max workers: %d)\n", maxWorkers)
	fmt.Println()

	// Step 3: Define agent personas
	fmt.Println("Step 3: Defining agent personas...")
	
	codeReviewerPersona := &models.Persona{
		Name:      "Code Reviewer",
		Character: "A thorough, security-conscious code reviewer",
		Mission:   "Find bugs and security vulnerabilities in code",
		Capabilities: []string{
			"Security analysis",
			"Code quality review",
			"Best practices checking",
		},
		Personality: "Direct and educational, provides constructive feedback",
	}
	fmt.Printf("✓ Created persona: %s\n", codeReviewerPersona.Name)

	taskExecutorPersona := &models.Persona{
		Name:      "Task Executor",
		Character: "An efficient task execution specialist",
		Mission:   "Execute tasks accurately and report results",
		Capabilities: []string{
			"Task analysis",
			"Solution implementation",
			"Result verification",
		},
		Personality: "Methodical and precise",
	}
	fmt.Printf("✓ Created persona: %s\n", taskExecutorPersona.Name)
	fmt.Println()

	// Step 4: Spawn agents
	fmt.Println("Step 4: Spawning agents with workers...")

	reviewer, err := workerManager.SpawnAgentWorker(
		ctx,
		"code-reviewer-1",
		"code-reviewer",
		"demo-project",
		"openai-gpt4",
		codeReviewerPersona,
	)
	if err != nil {
		log.Fatalf("Failed to spawn code reviewer: %v", err)
	}
	fmt.Printf("✓ Spawned agent: %s (ID: %s)\n", reviewer.Name, reviewer.ID)

	executor, err := workerManager.SpawnAgentWorker(
		ctx,
		"task-executor-1",
		"task-executor",
		"demo-project",
		"openai-gpt4",
		taskExecutorPersona,
	)
	if err != nil {
		log.Fatalf("Failed to spawn task executor: %v", err)
	}
	fmt.Printf("✓ Spawned agent: %s (ID: %s)\n", executor.Name, executor.ID)
	fmt.Println()

	// Step 5: Display system status
	fmt.Println("Step 5: System status...")
	stats := workerManager.GetPoolStats()
	fmt.Printf("  Total workers: %d\n", stats.TotalWorkers)
	fmt.Printf("  Idle workers: %d\n", stats.IdleWorkers)
	fmt.Printf("  Working workers: %d\n", stats.WorkingWorkers)
	fmt.Println()

	// Step 6: Create and assign tasks
	fmt.Println("Step 6: Creating tasks...")
	
	reviewTask := &worker.Task{
		ID:          "task-review-001",
		Description: "Review the authentication module for security vulnerabilities",
		Context:     "Focus on SQL injection, XSS, and authentication bypass risks",
		BeadID:      "bead-001",
		ProjectID:   "demo-project",
	}
	fmt.Printf("✓ Created task: %s\n", reviewTask.ID)

	executeTask := &worker.Task{
		ID:          "task-execute-001",
		Description: "Implement input validation for user registration form",
		Context:     "Use standard validation patterns, sanitize all inputs",
		BeadID:      "bead-002",
		ProjectID:   "demo-project",
	}
	fmt.Printf("✓ Created task: %s\n", executeTask.ID)
	fmt.Println()

	// Step 7: Execute tasks (note: will fail without real API key)
	fmt.Println("Step 7: Task execution demonstration...")
	if apiKey != "mock-key" {
		fmt.Println("Executing tasks (this will make API calls)...")
		
		// Execute review task
		fmt.Printf("  Executing: %s\n", reviewTask.Description)
		taskCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		result, err := workerManager.ExecuteTask(taskCtx, reviewer.ID, reviewTask)
		if err != nil {
			fmt.Printf("  ✗ Task failed: %v\n", err)
		} else {
			fmt.Printf("  ✓ Task completed by agent: %s\n", result.AgentID)
			fmt.Printf("  ✓ Tokens used: %d\n", result.TokensUsed)
			fmt.Printf("  ✓ Response preview: %.100s...\n", result.Response)
		}
	} else {
		fmt.Println("  Skipping actual task execution (no API key)")
		fmt.Println("  To execute real tasks, set OPENAI_API_KEY environment variable")
	}
	fmt.Println()

	// Step 8: List all agents
	fmt.Println("Step 8: Listing all agents...")
	agents := workerManager.ListAgents()
	for i, a := range agents {
		fmt.Printf("  %d. %s (Status: %s, Persona: %s)\n", i+1, a.Name, a.Status, a.PersonaName)
	}
	fmt.Println()

	// Step 9: Cleanup
	fmt.Println("Step 9: Cleaning up...")
	workerManager.StopAgent(reviewer.ID)
	fmt.Printf("✓ Stopped agent: %s\n", reviewer.Name)
	
	workerManager.StopAgent(executor.ID)
	fmt.Printf("✓ Stopped agent: %s\n", executor.Name)
	fmt.Println()

	// Final stats
	finalStats := workerManager.GetPoolStats()
	fmt.Println("Final status:")
	fmt.Printf("  Total workers: %d\n", finalStats.TotalWorkers)
	fmt.Println()

	fmt.Println("=== Demo Complete ===")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("1. Set OPENAI_API_KEY to execute real tasks")
	fmt.Println("2. Review docs/WORKER_SYSTEM.md for detailed documentation")
	fmt.Println("3. Check config/providers.example.yaml for configuration options")
	fmt.Println("4. Run integration tests: go test ./tests/integration/...")
}
