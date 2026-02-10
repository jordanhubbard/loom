# Loom System Manual

Complete reference for Loom -- the Agent Orchestration System.

## Quick Links

- **[QUICKSTART.md](QUICKSTART.md)** -- Get running in 10 minutes
- **[SETUP.md](SETUP.md)** -- Installation and configuration
- **[AGENTS.md](AGENTS.md)** -- Developer guide, API reference, troubleshooting
- **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** -- System design
- **[docs/ENTITIES_REFERENCE.md](docs/ENTITIES_REFERENCE.md)** -- Data structures

## What is Loom?

Loom is an **agent orchestration system** that:

1. **Coordinates multiple AI agents** with different roles (personas)
2. **Manages distributed work** through beads (work items) with dependencies
3. **Integrates with LLM providers** (local vLLM, cloud APIs)
4. **Uses Temporal** for reliable, durable workflow execution
5. **Provides real-time monitoring** via web UI and CEO dashboard

## Core Concepts

### Agents
- Autonomous AI entities with specific roles (CEO, Engineering Manager, Code Reviewer, etc.)
- Created from **personas** (behavior definitions in `personas/default/`)
- Auto-assigned to healthy providers from the shared pool
- Execute work through a **multi-turn action loop**: LLM call -> parse actions -> execute -> feedback -> repeat

### Beads
- Discrete units of work (features, bugs, tasks)
- Managed via the `bd` CLI with Dolt database backend
- Have **status**: open, in_progress, blocked, closed
- Support **dependencies**: can block other beads, be blocked by others
- Support **priority**: P0 (critical) through P3 (backlog)
- Dispatched to agents automatically based on priority and persona matching

### Providers
- LLM backends (vLLM, OpenAI-compatible APIs)
- Registered via the API with optional API keys (encrypted storage)
- Health-checked every 30 seconds via heartbeat workflow
- Report `context_window` from model metadata for proactive message truncation
- Status: pending -> healthy -> failed

### Projects
- Containers for related work, mapped to **git repositories**
- Loom generates a unique SSH keypair per project for git operations
- Beads loaded from project's `.beads/issues.jsonl` via Dolt
- Can be **perpetual** (never close, like self-improvement projects)
- Agents are assigned per-project

### Temporal Workflows
- Reliable, durable long-running processes
- Power heartbeats, bead processing, and dispatch coordination
- Survive container restarts

## Getting Started

```bash
# 1. Start Loom
make start

# 2. Register a provider
curl -X POST http://localhost:8080/api/v1/providers \
  -H 'Content-Type: application/json' \
  -d '{"id":"my-gpu","name":"My GPU","type":"openai","endpoint":"http://gpu-host:8000/v1","model":"Qwen/Qwen2.5-Coder-32B-Instruct"}'

# 3. Wait for heartbeat (30s), then verify
curl -s http://localhost:8080/api/v1/providers | jq '.[].status'

# 4. Open CEO dashboard
open http://localhost:8080
```

See [QUICKSTART.md](QUICKSTART.md) for the full walkthrough including project setup and filing beads.

## Agent Action Loop

When a bead is dispatched to an agent, the worker executes a multi-turn loop:

1. Build messages: system prompt (persona + lessons) + conversation history + task
2. Proactively truncate if messages exceed provider's context window (80% threshold)
3. Call LLM via provider's chat completions endpoint
4. On context-length 400 error: retry with progressively smaller history (50%, 25%, system+user only)
5. Parse response as JSON actions
6. Execute actions (read files, search code, write files, run commands, create beads, etc.)
7. Format action results as feedback message
8. Append to conversation and repeat from step 2
9. Terminate on: `done`, `close_bead`, `escalate_ceo`, max iterations (15), parse failures, or inner-loop detection

### Terminal Conditions

| Condition | Meaning |
|-----------|---------|
| `completed` | Agent signaled done or closed the bead |
| `escalated` | Agent escalated to CEO for decision |
| `max_iterations` | Hit 15-iteration limit |
| `parse_failures` | 2 consecutive JSON parse errors |
| `validation_failures` | 4 consecutive action validation errors |
| `inner_loop` | Same actions repeated 10 times |
| `error` | LLM call or action execution error |

## Key Features

### Multi-Turn Action Loop
Agents don't just respond once -- they iterate through read-analyze-act cycles, building understanding of the codebase and making incremental progress. Conversation history persists across dispatches.

### Context-Length Negotiation
When prompts exceed the model's context window, the system automatically retries with truncated history. The provider's `max_model_len` is discovered during heartbeat and used for proactive truncation.

### Beads Federation
Host and container share the same Dolt database via port 3307. The `bd` CLI on the host connects to the container's Dolt SQL server, enabling unified issue tracking across environments.

### Per-Project SSH Keys
Each project gets its own ed25519 keypair. Private keys are encrypted via AES-256-GCM in the key manager. Public keys are retrievable via API for adding as deploy keys.

### Provider Auto-Discovery
Heartbeat probes multiple endpoint patterns (OpenAI `/v1/models`, Ollama `/api/tags`) and auto-configures the provider type. API keys flow through to both heartbeat probes and chat completions.

## Make Commands

```bash
make start        # Build and start in Docker
make stop         # Stop all containers
make restart      # Rebuild and restart
make logs         # Follow loom container logs
make build        # Build Go binary (local)
make test         # Run tests locally
make test-docker  # Run tests with Temporal
make lint         # fmt + vet + yaml + docs
make distclean    # Full reset (docker + build cache)
make help         # All commands
```

## API Endpoints

```
# Projects
GET    /api/v1/projects
POST   /api/v1/projects
GET    /api/v1/projects/:id
GET    /api/v1/projects/:id/git-key     # Get SSH deploy key
POST   /api/v1/projects/:id/git-key     # Rotate SSH key

# Providers
GET    /api/v1/providers
POST   /api/v1/providers
DELETE /api/v1/providers/:id

# Agents
GET    /api/v1/agents

# Beads
GET    /api/v1/beads

# Events
GET    /api/v1/events/stream             # SSE real-time events

# Health
GET    /health
GET    /health/live
GET    /health/ready
```

## Troubleshooting

### Providers show "failed"
- Check `last_heartbeat_error` for details
- Use `type: "openai"` for vLLM servers (not `"local"`)
- Verify endpoint is reachable from inside the container

### Agents stay idle
- Need at least one healthy provider
- Need open beads in the project
- Check `make logs` for dispatch skip reasons

### Provider returns 401
- API key must be registered with the provider via the API
- Keys are encrypted in the key manager and flow through to both heartbeat and completions

### Context-length errors (400)
- System auto-retries with truncated history
- Provider's `context_window` is discovered from `max_model_len` in `/models` response
- Proactive truncation at 80% of context window

### Git clone fails (Permission denied)
- Retrieve the project's deploy key: `GET /api/v1/projects/:id/git-key`
- Add it as a write-enabled deploy key in your git host

## Glossary

| Term | Definition |
|------|-----------|
| **Agent** | Autonomous AI actor with a specific persona |
| **Bead** | Unit of work (task, feature, bug) tracked via `bd` CLI |
| **Persona** | Behavioral definition for an agent role |
| **Provider** | LLM backend (vLLM, OpenAI-compatible API) |
| **Project** | Git repository with associated beads and agents |
| **Connector** | Pluggable integration (MCP, OpenClaw, Webhook) -- planned |
| **Dispatch** | Process of assigning beads to idle agents |
| **Action Loop** | Multi-turn LLM -> parse -> execute -> feedback cycle |
| **Heartbeat** | Periodic provider health check (30s) |
