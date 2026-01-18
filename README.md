# Arbiter

An agentic-based coding orchestrator for both on-prem and off-prem development.

Arbiter is a lightweight AI coding agent orchestration system that manages workflows, handles agent lifecycle, and provides real-time event streaming for monitoring and coordination.

## Features

- 🤖 **Agent Orchestration**: Spawn and manage AI agents with different personas
- 🔄 **Workflow Management**: Temporal-based workflow orchestration for reliable task execution
- 📊 **Work Graph**: Track dependencies and relationships between work items (beads)
- 🔐 **Decision Framework**: Approval workflows for agent decisions
- 📡 **Real-time Events**: Server-Sent Events (SSE) for live status updates
- 🎯 **Smart Routing**: Intelligent task assignment and agent coordination
- 🔒 **Secure**: Encrypted secret storage and secure credential management

## Architecture

Arbiter is built with the following principles:

- **Go-First Implementation**: All primary functionality is implemented in Go for performance and maintainability
- **Containerized Everything**: Every component runs in containers for consistency across environments
- **Temporal Workflows**: Reliable, durable workflow orchestration using Temporal
- **Event-Driven**: Real-time event bus for agent communication and UI updates

## Prerequisites

- Docker (20.10+)
- Docker Compose (1.29+)
- Go 1.24+ (for local development only)
- Make (optional, for convenience commands)

## Quick Start

### Running with Docker (Recommended)

The Docker setup includes:
- Arbiter application server (port 8080)
- Temporal server (port 7233)
- Temporal UI (port 8088)
- PostgreSQL database for Temporal

```bash
# Build and run all services using docker-compose
docker-compose up -d

# View logs
docker-compose logs -f arbiter

# View Temporal UI
open http://localhost:8088

# Stop all services
docker-compose down
```

### Using Make Commands

```bash
# Build and run
make docker-run

# Build Docker image
make docker-build

# Stop services
make docker-stop

# Clean Docker resources
make docker-clean
```

## Temporal Workflow Engine

Arbiter uses [Temporal](https://temporal.io) for reliable workflow orchestration. Temporal provides:

- **Durable Execution**: Workflows survive crashes and restarts
- **Event History**: Complete audit trail of all workflow executions
- **Signals & Queries**: Real-time workflow interaction
- **Timeout Management**: Automatic handling of long-running operations

### Temporal Components

The system includes:

1. **Temporal Server**: Core workflow engine (port 7233)
2. **Temporal UI**: Web interface for monitoring workflows (port 8088)
3. **PostgreSQL**: Persistence layer for workflow state
4. **Temporal Worker**: Executes workflow and activity code

### Workflows

Arbiter implements several key workflows:

#### Agent Lifecycle Workflow
Manages the complete lifecycle of an agent from spawn to shutdown:
- Tracks agent status (spawned, working, idle, shutdown)
- Handles bead assignments
- Responds to queries for current status
- Gracefully shuts down on signal

#### Bead Processing Workflow
Manages work item (bead) lifecycle:
- Tracks status transitions (open, in_progress, blocked, closed)
- Handles agent assignments
- Manages dependencies and blockers
- Provides status queries

#### Decision Workflow
Handles approval workflows with timeout:
- Creates decision points for agent questions
- Waits for human or agent approval
- 48-hour default timeout
- Unblocks dependent work on resolution

### Event Bus

The Temporal-based event bus provides real-time updates:

```
Event Types:
- agent.spawned        - New agent created
- agent.status_change  - Agent status updated
- agent.completed      - Agent finished work
- bead.created         - New work item created
- bead.assigned        - Work assigned to agent
- bead.status_change   - Work status updated
- bead.completed       - Work item finished
- decision.created     - Decision point created
- decision.resolved    - Decision made
- log.message          - System log message
```

## API Endpoints

### Core Resources

```bash
# Health check
GET /api/v1/health

# Agents
GET    /api/v1/agents
POST   /api/v1/agents
GET    /api/v1/agents/{id}

# Beads (work items)
GET    /api/v1/beads
POST   /api/v1/beads
GET    /api/v1/beads/{id}
PUT    /api/v1/beads/{id}

# Decisions
GET    /api/v1/decisions
POST   /api/v1/decisions
PUT    /api/v1/decisions/{id}

# Projects
GET    /api/v1/projects
GET    /api/v1/projects/{id}

# Work Graph
GET    /api/v1/work-graph?project_id={id}
```

### Event Streaming (NEW)

Real-time event streaming via Server-Sent Events:

```bash
# Stream all events
GET /api/v1/events/stream

# Stream events for specific project
GET /api/v1/events/stream?project_id=example-project

# Stream specific event types
GET /api/v1/events/stream?type=agent.spawned

# Get event statistics
GET /api/v1/events/stats
```

Example: Subscribe to events using curl:
```bash
curl -N http://localhost:8080/api/v1/events/stream
```

Example: Subscribe to events using JavaScript:
```javascript
const eventSource = new EventSource('http://localhost:8080/api/v1/events/stream?project_id=my-project');

eventSource.addEventListener('agent.spawned', (e) => {
  const data = JSON.parse(e.data);
  console.log('Agent spawned:', data);
});

eventSource.addEventListener('bead.created', (e) => {
  const data = JSON.parse(e.data);
  console.log('Bead created:', data);
});
```

## Configuration

Configuration is managed via `config.yaml`:

```yaml
server:
  http_port: 8080
  enable_http: true

temporal:
  host: localhost:7233              # Temporal server address
  namespace: arbiter-default        # Temporal namespace
  task_queue: arbiter-tasks         # Task queue name
  workflow_execution_timeout: 24h   # Max workflow duration
  workflow_task_timeout: 10s        # Workflow task timeout
  enable_event_bus: true            # Enable event bus
  event_buffer_size: 1000           # Event buffer size

agents:
  max_concurrent: 10
  default_persona_path: ./personas
  heartbeat_interval: 30s
  file_lock_timeout: 10m

- [x] Project state management (open, closed, reopened)
- [x] Project comments and closure workflow
- [x] Arbiter persona for self-improvement
- [x] Perpetual projects that never close
- [ ] Implement actual HTTP forwarding to providers
- [ ] Add streaming support for real-time responses
- [ ] Implement request/response logging and analytics
- [ ] Add support for provider-specific features
- [ ] Implement load balancing and failover
- [ ] Add authentication for Arbiter API
- [ ] Support for custom provider plugins
- [ ] Add metrics and monitoring endpoints
- [ ] Implement rate limiting per provider
- [ ] Add caching layer for responses

## Project State Management

Arbiter supports sophisticated project lifecycle management:

### Project States
- **Open**: Active project with ongoing work
- **Closed**: Completed project with no remaining work
- **Reopened**: Previously closed project that has been reopened

### Features
- **Comments**: Add timestamped comments to track project decisions
- **Closure Workflow**: Close projects only when no open work remains
- **Agent Consensus**: If open work exists, requires agent agreement to close
- **Perpetual Projects**: Mark projects (like Arbiter itself) that never close

### API Endpoints

```bash
# Close a project
POST /api/v1/projects/{id}/close
{
  "author_id": "agent-123",
  "comment": "All features complete, tests passing"
}

# Reopen a project
POST /api/v1/projects/{id}/reopen
{
  "author_id": "agent-456",
  "comment": "New requirements discovered"
}

# Add a comment
POST /api/v1/projects/{id}/comments
{
  "author_id": "agent-789",
  "comment": "Architecture review complete"
}

# Get project state
GET /api/v1/projects/{id}/state
```

## The Arbiter Persona

The Arbiter system includes a special **arbiter** persona that works on improving the Arbiter platform itself:

- **Self-Improving**: Continuously enhances the platform
- **Collaborative**: Works with UX, Engineering, PM, and Product personas
- **Perpetual**: The arbiter project never closes
- **Meta-Circular**: An AI orchestrator that orchestrates its own improvement

See `personas/arbiter/` for the complete persona definition.

## Support

## Local Development

### Building Locally

```bash
# Install dependencies
go mod download

# Build the binary
go build -o arbiter ./cmd/arbiter

# Run the application
./arbiter
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific package tests
go test ./internal/temporal/...
```

### Development with Temporal

For local development with Temporal:

1. Start Temporal server:
```bash
docker-compose up -d temporal temporal-postgresql temporal-ui
```

2. Build and run arbiter locally:
```bash
go build -o arbiter ./cmd/arbiter
./arbiter
```

3. Access Temporal UI:
```bash
open http://localhost:8088
```

## Project Structure

```
arbiter/
├── cmd/arbiter/              # Main application entry point
│   └── main.go
├── internal/
│   ├── agent/               # Agent management
│   ├── arbiter/             # Core orchestrator
│   ├── beads/               # Work item management
│   ├── decision/            # Decision framework
│   ├── temporal/            # Temporal integration
│   │   ├── client/          # Temporal client wrapper
│   │   ├── workflows/       # Workflow definitions
│   │   ├── activities/      # Activity implementations
│   │   ├── eventbus/        # Event bus implementation
│   │   └── manager.go       # Temporal manager
│   ├── api/                 # HTTP API handlers
│   └── models/              # Data models
├── pkg/
│   ├── config/              # Configuration management
│   └── models/              # Shared models
├── config/
│   └── temporal/            # Temporal configuration
├── docker-compose.yml       # Container orchestration
├── Dockerfile              # Multi-stage Docker build
├── config.yaml.example     # Example configuration
└── README.md              # This file
```

## Monitoring

### Temporal UI

Access the Temporal UI at http://localhost:8088 to:
- View workflow executions
- Inspect workflow history
- Monitor active workflows
- Debug workflow failures
- Query workflow state

### Event Stream Monitoring

Monitor real-time events:
```bash
# Watch all events
curl -N http://localhost:8080/api/v1/events/stream

# Monitor specific project
curl -N "http://localhost:8080/api/v1/events/stream?project_id=my-project"
```

### Logs

View service logs:
```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f arbiter
docker-compose logs -f temporal
```

## Troubleshooting

### Temporal Connection Issues

If arbiter can't connect to Temporal:

1. Check Temporal is running:
```bash
docker-compose ps temporal
```

2. Check Temporal logs:
```bash
docker-compose logs temporal
```

3. Verify connectivity:
```bash
docker exec arbiter nc -zv temporal 7233
```

### Workflow Not Starting

If workflows aren't starting:

1. Check worker is running:
```bash
docker-compose logs arbiter | grep "Temporal worker"
```

2. Verify task queue in Temporal UI
3. Check workflow registration in logs

### Event Stream Not Working

If event stream endpoint returns errors:

1. Verify Temporal is enabled in config
2. Check event bus initialization:
```bash
docker-compose logs arbiter | grep "event bus"
```

## Development Guidelines

1. **Primary Language**: Implement all core functionality in Go
2. **Containerization**: All services must run in containers
3. **Workflows**: Use Temporal workflows for long-running operations
4. **Events**: Publish events for all state changes
5. **Testing**: Write tests for workflows and activities
6. **Documentation**: Update docs for new features

## Contributing

When contributing to this project:

1. Ensure all code follows the architecture principles
2. All new features must be containerized
3. Write Temporal workflows for async operations
4. Add appropriate event publishing
5. Update documentation
6. Run tests and linters before submitting

## License

See LICENSE file for details.
