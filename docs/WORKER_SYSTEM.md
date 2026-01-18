# Agent Worker System

This document describes the new agent worker system that allows agents to be spun up as workers that communicate with AI providers through OpenAI-compatible APIs.

## Architecture

The system consists of several key components:

### 1. Provider Protocol (`internal/provider/protocol.go`)

Defines the interface for communicating with AI providers using OpenAI-compatible APIs:

```go
type Protocol interface {
    CreateChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error)
    GetModels(ctx context.Context) ([]Model, error)
}
```

### 2. Provider Registry (`internal/provider/registry.go`)

Manages registered AI providers:

```go
registry := provider.NewRegistry()

// Register a provider
config := &provider.ProviderConfig{
    ID:       "openai-gpt4",
    Name:     "OpenAI GPT-4",
    Type:     "openai",
    Endpoint: "https://api.openai.com/v1",
    APIKey:   "sk-...",
    Model:    "gpt-4",
}
registry.Register(config)
```

### 3. Worker (`internal/worker/worker.go`)

Executes tasks for agents using their assigned provider:

- Builds system prompts from agent personas
- Sends requests to AI providers
- Returns task results with token usage

### 4. Worker Pool (`internal/worker/pool.go`)

Manages multiple workers:

```go
pool := worker.NewPool(registry, maxWorkers)
```

### 5. WorkerManager (`internal/agent/worker_manager.go`)

Integrates agents with the worker system:

```go
manager := agent.NewWorkerManager(maxAgents, providerRegistry)

// Spawn an agent with a worker
agent, err := manager.SpawnAgentWorker(ctx, 
    "my-agent",           // agent name
    "code-reviewer",      // persona name
    "project-1",          // project ID
    "openai-gpt4",        // provider ID
    persona,              // loaded persona
)

// Execute a task
task := &worker.Task{
    ID:          "task-1",
    Description: "Review the authentication code",
    Context:     "Focus on security vulnerabilities",
}

result, err := manager.ExecuteTask(ctx, agent.ID, task)
```

## Usage Example

### Step 1: Set up the provider registry

```go
// Create provider registry
registry := provider.NewRegistry()

// Register OpenAI
registry.Register(&provider.ProviderConfig{
    ID:       "openai-gpt4",
    Name:     "OpenAI GPT-4",
    Type:     "openai",
    Endpoint: "https://api.openai.com/v1",
    APIKey:   os.Getenv("OPENAI_API_KEY"),
    Model:    "gpt-4",
})

// Register Anthropic
registry.Register(&provider.ProviderConfig{
    ID:       "anthropic-claude",
    Name:     "Anthropic Claude",
    Type:     "anthropic",
    Endpoint: "https://api.anthropic.com/v1",
    APIKey:   os.Getenv("ANTHROPIC_API_KEY"),
    Model:    "claude-3-opus",
})

// Register local model
registry.Register(&provider.ProviderConfig{
    ID:       "local-llama",
    Name:     "Local Llama",
    Type:     "local",
    Endpoint: "http://localhost:8000/v1",
    Model:    "llama-2-70b",
})
```

### Step 2: Create the worker manager

```go
maxAgents := 10
manager := agent.NewWorkerManager(maxAgents, registry)
```

### Step 3: Load a persona

```go
personaManager := persona.NewManager("./personas")
persona, err := personaManager.LoadPersona("examples/code-reviewer")
if err != nil {
    log.Fatal(err)
}
```

### Step 4: Spawn an agent worker

```go
agent, err := manager.SpawnAgentWorker(
    context.Background(),
    "reviewer-1",              // agent name
    "examples/code-reviewer",  // persona name
    "my-project",              // project ID
    "openai-gpt4",             // provider to use
    persona,                   // loaded persona
)
if err != nil {
    log.Fatal(err)
}

log.Printf("Spawned agent: %s", agent.ID)
```

### Step 5: Execute tasks

```go
// Create a task
task := &worker.Task{
    ID:          "task-001",
    Description: "Review the authentication module for security vulnerabilities",
    Context:     "Pay special attention to SQL injection and XSS vulnerabilities",
    ProjectID:   "my-project",
}

// Execute the task
result, err := manager.ExecuteTask(context.Background(), agent.ID, task)
if err != nil {
    log.Fatal(err)
}

// Process the result
log.Printf("Task completed by agent %s", result.AgentID)
log.Printf("Response: %s", result.Response)
log.Printf("Tokens used: %d", result.TokensUsed)
```

### Step 6: Monitor workers

```go
// Get pool statistics
stats := manager.GetPoolStats()
log.Printf("Total workers: %d", stats.TotalWorkers)
log.Printf("Idle workers: %d", stats.IdleWorkers)
log.Printf("Working workers: %d", stats.WorkingWorkers)

// List all agents
agents := manager.ListAgents()
for _, agent := range agents {
    log.Printf("Agent %s: status=%s, last_active=%v", 
        agent.Name, agent.Status, agent.LastActive)
}
```

### Step 7: Stop agents

```go
// Stop a specific agent
err := manager.StopAgent(agent.ID)

// Or stop all agents
manager.StopAll()
```

## Provider Types

The system supports any OpenAI-compatible API provider:

- **OpenAI**: GPT-4, GPT-3.5, etc.
- **Anthropic**: Claude models (via OpenAI-compatible endpoints)
- **Local Models**: Ollama, vLLM, LM Studio, etc.
- **Custom**: Any service implementing the OpenAI chat completion API

## Persona Integration

Workers automatically build system prompts from agent personas, including:

- Character description
- Mission statement
- Personality traits
- Capabilities
- Autonomy guidelines
- Decision-making instructions

This allows each agent to have a distinct "personality" and approach to tasks based on their persona definition.

## Token Usage Tracking

Each task execution returns token usage information:

```go
type TaskResult struct {
    TaskID      string
    WorkerID    string
    AgentID     string
    Response    string
    TokensUsed  int        // Total tokens used
    CompletedAt time.Time
    Success     bool
    Error       string
}
```

This can be used for cost tracking and performance monitoring.

## Future Enhancements

Potential improvements to the system:

1. **Streaming responses**: Support for streaming chat completions
2. **Rate limiting**: Per-provider rate limiting
3. **Failover**: Automatic failover to backup providers
4. **Load balancing**: Distribute load across multiple instances
5. **Caching**: Cache responses for identical requests
6. **Metrics**: Detailed performance metrics and logging
7. **Retry logic**: Automatic retry with exponential backoff
8. **Cost tracking**: Per-agent and per-provider cost tracking
