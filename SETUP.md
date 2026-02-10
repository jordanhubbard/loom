# Loom Setup Guide

Everything you need to get Loom running and create your first project.

## Prerequisites

- Docker (20.10+)
- Docker Compose (1.29+)
- Go 1.25+ (for local development only)

## Running Loom

Loom always runs in Docker. The stack includes:
- Loom application server (port 8080)
- Temporal server (port 7233)
- Temporal UI (port 8088)
- PostgreSQL database for Temporal

```bash
# Start everything
make start

# Verify
docker compose ps

# View loom logs
make logs

# Stop
make stop

# Rebuild and restart (after code changes)
make restart
```

## Connecting to the UI

- **Loom Web UI**: http://localhost:8080
- **Temporal UI**: http://localhost:8088 -- view workflow executions, inspect history, monitor active workflows

## Configuration

Structural configuration is managed via `config.yaml` (server, temporal, agents, projects). Secrets like API keys are **never** stored in `config.yaml` -- see [Registering Providers](#registering-providers) below.

```yaml
server:
  http_port: 8081    # Internal container port (Docker maps 8080 -> 8081)
  enable_http: true

temporal:
  host: temporal:7233  # Temporal service name within Docker network
  namespace: default
  task_queue: loom-tasks

agents:
  max_concurrent: 6
  default_persona_path: ./personas
  heartbeat_interval: 30s
```

Environment variables in `config.yaml` are expanded automatically -- `${MY_VAR}` is replaced with the value of `MY_VAR` from the environment.

## Registering Providers

Providers (LLM backends) are registered via the Loom API, **not** in `config.yaml`. This keeps API keys out of version control.

### Register a Provider

```bash
# Local vLLM server (no API key)
curl -X POST http://localhost:8080/api/v1/providers \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "my-gpu",
    "name": "My GPU",
    "type": "openai",
    "endpoint": "http://gpu-host:8000/v1",
    "model": "Qwen/Qwen2.5-Coder-32B-Instruct"
  }'

# Cloud provider (with API key)
curl -X POST http://localhost:8080/api/v1/providers \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "cloud-llm",
    "name": "Cloud LLM",
    "type": "openai",
    "endpoint": "https://api.example.com/v1",
    "model": "model-name",
    "api_key": "your-key-here"
  }'
```

API keys are encrypted and stored in Loom's key manager. They persist across restarts.

### Using bootstrap.local

For repeatable setup, create a `bootstrap.local` script (gitignored):

```bash
cp bootstrap.local.example bootstrap.local
chmod +x bootstrap.local
# Edit with your providers and API keys
LOOM_URL=http://localhost:8080 ./bootstrap.local
```

Run `bootstrap.local` once after a fresh deployment or database wipe. Providers persist in the database.

## Bootstrapping Your First Project

Projects can be registered via `config.yaml` or the UI.

### Via config.yaml

```yaml
projects:
  - id: myapp
    name: My App
    git_repo: git@github.com:user/myapp.git
    branch: main
    beads_path: .beads
    is_perpetual: true
    context:
      build_command: "make build"
      test_command: "make test"
```

### Via the UI

Navigate to **Projects** > **Add Project** and fill in the repository details.

### Deploy Key Setup

Loom generates a unique SSH keypair for each project. After adding a project, retrieve its public key:

```bash
curl -s http://localhost:8080/api/v1/projects/<project-id>/git-key | jq -r '.public_key'
```

Add this as a **deploy key with write access** in your git hosting service (GitHub: Settings > Deploy keys). Loom will clone the repository on the next dispatch cycle.

## Development

### Building Locally

```bash
make build           # Build Go binary
make test            # Run tests
make test-docker     # Run tests in Docker with Temporal
make lint            # Run all linters
make coverage        # Tests with coverage report
```

### Full Command Reference

```bash
make help            # Show all available commands
```

## Monitoring

### Event Stream

```bash
curl -N http://localhost:8080/api/v1/events/stream
```

### Logs

```bash
make logs                          # Follow loom logs
docker compose logs -f temporal    # Temporal logs
```

## Troubleshooting

### Temporal Connection Issues

```bash
docker compose ps temporal         # Is it running?
docker compose logs temporal       # Check logs
docker compose restart temporal    # Restart it
```

### Providers Show "failed"

```bash
# Check heartbeat error
curl -s http://localhost:8080/api/v1/providers | jq '.[].last_heartbeat_error'
```

Common causes: unreachable endpoint, wrong type (use `"openai"` for vLLM), missing API key.

### Agents Stay Idle

- Check providers are `"healthy"`: `curl -s http://localhost:8080/api/v1/providers | jq '.[].status'`
- Check beads exist: dispatcher needs open beads to assign work
- Check logs: `make logs`
