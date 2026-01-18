# Worker System Examples

This directory contains examples demonstrating how to use the Arbiter worker system.

## Examples

### worker_demo

A comprehensive demonstration of the worker system workflow.

**What it demonstrates:**
- Setting up a provider registry
- Registering AI providers (OpenAI, Ollama, etc.)
- Creating worker manager
- Defining agent personas
- Spawning agents with workers
- Creating and executing tasks
- Monitoring system status
- Cleanup and shutdown

**Running the demo:**

```bash
# With OpenAI API (real execution)
export OPENAI_API_KEY="sk-your-key-here"
cd examples/worker_demo
go run main.go

# With mock provider (no API calls)
cd examples/worker_demo
go run main.go

# With local Ollama
export USE_OLLAMA=true
cd examples/worker_demo
go run main.go
```

**Expected output:**
```
=== Arbiter Worker System Demo ===

Step 1: Setting up provider registry...
✓ Registered provider: OpenAI GPT-4

Step 2: Creating worker manager...
✓ Worker manager created (max workers: 5)

Step 3: Defining agent personas...
✓ Created persona: Code Reviewer
✓ Created persona: Task Executor

Step 4: Spawning agents with workers...
✓ Spawned agent: code-reviewer-1 (ID: agent-1234...)
✓ Spawned agent: task-executor-1 (ID: agent-5678...)

...
```

## Configuration

The examples can use configuration from:
- `config/providers.example.yaml` - Full provider configuration
- `config/quickstart.yaml` - Minimal quickstart configuration

## Environment Variables

- `OPENAI_API_KEY` - Your OpenAI API key (required for real execution)
- `ANTHROPIC_API_KEY` - Your Anthropic API key (optional)
- `USE_OLLAMA` - Set to "true" to enable local Ollama provider

## Next Steps

After running the examples:

1. **Read the documentation**: `docs/WORKER_SYSTEM.md`
2. **Run integration tests**: `go test ./tests/integration/... -v`
3. **Review configuration**: Check `config/providers.example.yaml`
4. **Customize personas**: See `personas/examples/` directory

## Troubleshooting

**"Provider not found" error:**
- Ensure you've registered the provider in the registry
- Check that the provider ID matches what you're referencing

**"Connection refused" error:**
- For local providers (Ollama, vLLM), ensure the service is running
- Check the endpoint URL in your configuration

**"API key invalid" error:**
- Verify your API key is set correctly
- Check that the environment variable is exported

**Task execution timeout:**
- Increase the timeout in context.WithTimeout()
- Check your network connection
- Verify the provider endpoint is accessible

## Additional Resources

- [Worker System Documentation](../../docs/WORKER_SYSTEM.md)
- [Architecture Overview](../../ARCHITECTURE.md)
- [Provider Configuration](../../config/providers.example.yaml)
- [Integration Tests](../../tests/integration/)
