# Arbiter Architecture

## Overview

The Arbiter is a secure orchestration system that manages interactions between AI agents and providers. It maintains its own database and is the sole reader/writer to ensure data integrity.

## Core Concepts

### Agents
An **Agent** is an LLM (Large Language Model) wrapped in glue code that performs tasks. Agents are configured to use a specific provider and can have custom configurations.

### Providers
A **Provider** is an AI engine running on-premise or in the cloud (e.g., OpenAI, Anthropic, local models). Providers may require API credentials (keys) to communicate.

### Key Manager
The **Key Manager** securely stores provider credentials with strong encryption. Keys are encrypted at rest and only accessible when the key store is unlocked with a password.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                        Arbiter                               │
│  ┌─────────────────────────────────────────────────────┐   │
│  │              Main Orchestrator                       │   │
│  │  - Manages Agent/Provider lifecycle                  │   │
│  │  - Coordinates operations                            │   │
│  │  - Ensures security and data integrity               │   │
│  └────────────┬──────────────────────┬──────────────────┘   │
│               │                      │                       │
│  ┌────────────▼──────────┐  ┌───────▼────────────────┐    │
│  │    Database Layer      │  │   Key Manager          │    │
│  │                        │  │                        │    │
│  │  - Agents table        │  │  - Encrypted storage   │    │
│  │  - Providers table     │  │  - AES-256-GCM         │    │
│  │  - SQLite backend      │  │  - PBKDF2 (100k iter)  │    │
│  │  - Foreign keys        │  │  - Per-key salt/nonce  │    │
│  └────────────────────────┘  └────────────────────────┘    │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │             Configuration                             │  │
│  │  - Password from env or secure prompt                │  │
│  │  - Data directory management                         │  │
│  │  - No password storage                               │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Data Flow

### 1. Initialization
```
User starts Arbiter
  ↓
Check ARBITER_PASSWORD env variable
  ↓
If not found, prompt user (hidden input)
  ↓
Initialize database connection
  ↓
Unlock key manager with password
  ↓
Ready to orchestrate
```

### 2. Creating a Provider with Credentials
```
User creates provider with API key
  ↓
Arbiter encrypts API key with key manager
  ↓
Store encrypted key with unique ID
  ↓
Store provider record in database with key_id reference
  ↓
Provider ready for use
```

### 3. Creating an Agent
```
User creates agent with provider reference
  ↓
Arbiter verifies provider exists
  ↓
Store agent record in database
  ↓
Foreign key ensures referential integrity
  ↓
Agent ready to use provider
```

### 4. Using an Agent
```
Retrieve agent by ID
  ↓
Get associated provider
  ↓
If provider requires key, decrypt from key manager
  ↓
Return agent, provider, and decrypted API key
  ↓
Use credentials to communicate with provider
```

## Security Model

### Encryption at Rest
- **Algorithm**: AES-256-GCM (Galois/Counter Mode)
- **Key Derivation**: PBKDF2 with SHA-256
- **Iterations**: 100,000 (protects against brute force)
- **Salt**: 32 bytes per key (unique)
- **Nonce**: 12 bytes per key (unique)

### Password Handling
- **Never stored**: Password exists only in memory
- **Environment variable**: `ARBITER_PASSWORD` (for automation)
- **Interactive prompt**: Hidden input using `golang.org/x/term`
- **Memory clearing**: Password cleared when key manager locks

### File Permissions
- **Key store**: `0600` (owner read/write only)
- **Database**: Default SQLite permissions
- **Data directory**: `0700` (owner access only)

### Key Store Structure
```json
{
  "keys": {
    "key_openai-gpt4": {
      "id": "key_openai-gpt4",
      "name": "OpenAI GPT-4",
      "description": "API Key for OpenAI GPT-4",
      "encrypted_data": "base64-encoded-encrypted-key",
      "created_at": "2026-01-18T17:00:00Z",
      "updated_at": "2026-01-18T17:00:00Z"
    }
  }
}
```

## Database Schema

### Providers Table
```sql
CREATE TABLE providers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,              -- openai, anthropic, local, etc.
    endpoint TEXT NOT NULL,          -- URL or path
    description TEXT,
    requires_key BOOLEAN NOT NULL,   -- Does it need credentials?
    key_id TEXT,                     -- Reference to key manager
    status TEXT NOT NULL,            -- active, inactive, etc.
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
```

### Agents Table
```sql
CREATE TABLE agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    provider_id TEXT NOT NULL,       -- Foreign key to providers
    status TEXT NOT NULL,            -- active, inactive, etc.
    config TEXT,                     -- JSON configuration
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE CASCADE
);
```

## API Usage Examples

### Creating a Provider
```go
provider := &models.Provider{
    ID:          "openai-gpt4",
    Name:        "OpenAI GPT-4",
    Type:        "openai",
    Endpoint:    "https://api.openai.com/v1",
    Description: "OpenAI GPT-4 API",
    RequiresKey: true,
    Status:      "active",
}

apiKey := "sk-..."
err := arbiter.CreateProvider(provider, apiKey)
```

### Creating an Agent
```go
agent := &models.Agent{
    ID:          "coding-agent",
    Name:        "Coding Assistant",
    Description: "AI coding assistant",
    ProviderID:  "openai-gpt4",
    Status:      "active",
    Config:      `{"model": "gpt-4", "temperature": 0.7}`,
}

err := arbiter.CreateAgent(agent)
```

### Using an Agent
```go
agent, provider, apiKey, err := arbiter.GetAgentWithProvider("coding-agent")
if err != nil {
    log.Fatal(err)
}

// Use agent, provider, and apiKey to make AI requests
// The apiKey is decrypted and ready to use
```

## Directory Structure

```
arbiter/
├── cmd/arbiter/              # Main application
│   └── main.go              # Entry point and orchestrator
├── internal/
│   ├── config/              # Configuration management
│   │   └── config.go        # Password handling, data paths
│   ├── database/            # Database layer
│   │   ├── database.go      # SQLite operations
│   │   └── database_test.go # Database tests
│   ├── keymanager/          # Key management
│   │   ├── keymanager.go    # Encryption/decryption
│   │   └── keymanager_test.go # Key manager tests
│   └── models/              # Data models
│       ├── agent.go         # Agent model
│       └── provider.go      # Provider model
├── go.mod                   # Go module definition
├── go.sum                   # Dependency checksums
├── README.md               # User documentation
├── ARCHITECTURE.md         # This file
└── LICENSE                 # License information
```

## Design Decisions

### Why SQLite?
- **Embedded**: No separate database server needed
- **ACID**: Full transaction support
- **Portable**: Single file database
- **Lightweight**: Perfect for local orchestration
- **Mature**: Well-tested and reliable

### Why AES-256-GCM?
- **Authenticated encryption**: Detects tampering
- **NIST approved**: Widely trusted standard
- **Fast**: Hardware acceleration on modern CPUs
- **Secure**: No known practical attacks

### Why PBKDF2?
- **Standard**: NIST and IETF approved
- **Tunable**: Iteration count can increase over time
- **Well-understood**: Extensively analyzed
- **Compatible**: Wide library support

### Why No Password Storage?
- **Security**: Reduces attack surface
- **Best practice**: Password should only exist in user's mind
- **Ephemeral**: Process memory is temporary
- **User control**: User must be present to unlock

## Future Considerations

### Possible Enhancements
- Add agent execution engine
- Implement provider communication layer
- Add task queuing and scheduling
- Support for agent chaining
- Monitoring and logging system
- Web UI for management
- Multi-user support with RBAC
- Backup and restore functionality

### Security Enhancements
- Hardware security module (HSM) support
- Biometric authentication
- Two-factor authentication
- Audit logging
- Key rotation
- Certificate-based authentication for providers

### Scalability
- Distributed orchestration
- Provider pooling
- Load balancing
- High availability
- Horizontal scaling

## Temporal Workflow Architecture

### Overview

Arbiter uses [Temporal](https://temporal.io) as its workflow orchestration engine. Temporal provides durable execution, automatic retries, and a complete audit trail for all workflows.

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                        Arbiter                               │
│  ┌─────────────────────────────────────────────────────┐   │
│  │         Arbiter Orchestrator                         │   │
│  │  - Spawns agents with personas                       │   │
│  │  - Creates beads (work items)                        │   │
│  │  - Manages decisions                                 │   │
│  └────────┬──────────────────────┬─────────────────────┘   │
│           │                      │                           │
│  ┌────────▼──────────┐  ┌───────▼────────────────┐         │
│  │  Temporal Manager  │  │   Event Bus            │         │
│  │  - Workflow starter│  │   - Pub/Sub messaging  │         │
│  │  - Signal sender   │  │   - Event filtering    │         │
│  │  - Query executor  │  │   - SSE streaming      │         │
│  └────────┬───────────┘  └───────┬────────────────┘         │
└───────────┼──────────────────────┼──────────────────────────┘
            │                      │
            │  gRPC (7233)         │  HTTP/SSE (8080)
            │                      │
┌───────────▼──────────────────────▼──────────────────────────┐
│                     Temporal Server                          │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Temporal Worker (in Arbiter process)                 │  │
│  │  - AgentLifecycleWorkflow                            │  │
│  │  - BeadProcessingWorkflow                            │  │
│  │  - DecisionWorkflow                                  │  │
│  │  - EventAggregatorWorkflow                           │  │
│  │  - Activities (event publishing, notifications)      │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Temporal Core Services                               │  │
│  │  - History Service (workflow state)                  │  │
│  │  - Matching Service (task routing)                   │  │
│  │  - Frontend Service (gRPC API)                       │  │
│  └─────────────────────┬────────────────────────────────┘  │
└────────────────────────┼─────────────────────────────────────┘
                         │
                         │  SQL
                         │
              ┌──────────▼──────────┐
              │    PostgreSQL       │
              │  - Workflow history │
              │  - Task queues      │
              │  - Timers           │
              └─────────────────────┘
```

### Workflow Patterns

#### 1. Agent Lifecycle Workflow

Manages the complete lifecycle of an agent from spawn to shutdown.

**Workflow ID**: `agent-{agentID}`

**State Machine**:
```
spawned → idle ⟷ working → shutdown
                    ↓
                 blocked
```

**Signals**:
- `updateStatus`: Change agent status
- `assignBead`: Assign work to agent
- `shutdown`: Gracefully stop agent

**Queries**:
- `getStatus`: Get current agent status
- `getCurrentBead`: Get currently assigned work

**Use Cases**:
- Track agent activity
- Monitor agent health
- Coordinate agent tasks
- Graceful shutdown

**Example**:
```go
// Start agent workflow
err := temporalManager.StartAgentWorkflow(
    ctx, 
    agentID, 
    projectID, 
    personaName, 
    agentName,
)

// Query agent status
status, err := temporalManager.QueryAgentWorkflow(
    ctx,
    agentID,
    "getStatus",
)

// Signal agent to shutdown
err := temporalManager.SignalAgentWorkflow(
    ctx,
    agentID,
    "shutdown",
    "maintenance",
)
```

#### 2. Bead Processing Workflow

Manages the lifecycle of a work item (bead) from creation to completion.

**Workflow ID**: `bead-{beadID}`

**State Machine**:
```
open → in_progress → closed
  ↓         ↓
blocked   blocked
```

**Updates** (Workflow Updates API):
- `assignToAgent`: Assign bead to agent
- `updateStatus`: Change bead status
- `complete`: Mark bead as done

**Queries**:
- `getStatus`: Get current bead status
- `getAssignedAgent`: Get assigned agent ID

**Signals**:
- `statusChange`: External status change

**Use Cases**:
- Track work progress
- Manage dependencies
- Prevent merge conflicts
- Audit work history

**Example**:
```go
// Start bead workflow
err := temporalManager.StartBeadWorkflow(
    ctx,
    beadID,
    projectID,
    title,
    description,
    priority,
    beadType,
)

// Update bead status
err := temporalManager.SignalBeadWorkflow(
    ctx,
    beadID,
    "statusChange",
    "in_progress",
)
```

#### 3. Decision Workflow

Handles approval workflows with timeout for agent decisions.

**Workflow ID**: `decision-{decisionID}`

**State Machine**:
```
pending → resolved
   ↓
timeout (after 48h)
```

**Updates**:
- `resolve`: Resolve decision with choice

**Queries**:
- `getStatus`: Get decision status
- `getDecision`: Get decision result

**Timeout**: 48 hours (configurable)

**Use Cases**:
- Agent approval requests
- Human-in-the-loop decisions
- P0 priority items
- Critical path decisions

**Example**:
```go
// Start decision workflow
err := temporalManager.StartDecisionWorkflow(
    ctx,
    decisionID,
    projectID,
    question,
    requesterID,
    options,
)

// Wait for decision (in separate goroutine)
workflowRun := client.GetWorkflow(ctx, "decision-"+decisionID, "")
var decision string
err := workflowRun.Get(ctx, &decision)
```

#### 4. Event Aggregator Workflow

Long-running workflow that aggregates events for a project.

**Workflow ID**: `events-{projectID}`

**Features**:
- Receives event signals
- Maintains event history
- Supports continue-as-new for long histories
- Provides event queries

**Signals**:
- `event`: Publish event to aggregator

**Use Cases**:
- Event history tracking
- Metrics aggregation
- Audit logging
- Timeline reconstruction

### Event Bus Architecture

The event bus provides real-time pub/sub messaging using Go channels backed by Temporal workflows.

```
┌─────────────────────────────────────────────────────────┐
│                     Event Bus                            │
│                                                          │
│  ┌──────────────┐      ┌──────────────────────────┐   │
│  │   Publisher  │─────▶│  Event Buffer (chan)     │   │
│  │  (Managers)  │      │  Capacity: 1000          │   │
│  └──────────────┘      └──────────┬───────────────┘   │
│                                   │                     │
│                        ┌──────────▼──────────┐         │
│                        │  Event Distributor  │         │
│                        │  - Apply filters    │         │
│                        │  - Non-blocking     │         │
│                        └──────────┬──────────┘         │
│                                   │                     │
│         ┌─────────────────────────┼─────────────┐      │
│         │                         │             │      │
│    ┌────▼─────┐            ┌─────▼────┐  ┌────▼────┐ │
│    │Subscriber│            │Subscriber│  │Subscriber│ │
│    │ (UI/SSE) │            │ (Logger) │  │ (Metrics)│ │
│    └──────────┘            └──────────┘  └──────────┘ │
└─────────────────────────────────────────────────────────┘
```

**Event Types**:
- `agent.spawned`: New agent created
- `agent.status_change`: Agent status updated
- `agent.completed`: Agent finished
- `bead.created`: Work item created
- `bead.assigned`: Work assigned
- `bead.status_change`: Status updated
- `bead.completed`: Work finished
- `decision.created`: Decision requested
- `decision.resolved`: Decision made
- `log.message`: System log

**Subscriber Filters**:
```go
// Filter by project
filter := func(event *Event) bool {
    return event.ProjectID == "my-project"
}

// Filter by event type
filter := func(event *Event) bool {
    return event.Type == "agent.spawned"
}

// Combined filter
filter := func(event *Event) bool {
    return event.ProjectID == "my-project" && 
           strings.HasPrefix(string(event.Type), "bead.")
}
```

### API Event Streaming

Server-Sent Events (SSE) endpoint for real-time updates:

**Endpoint**: `GET /api/v1/events/stream`

**Query Parameters**:
- `project_id`: Filter by project
- `type`: Filter by event type

**Response Format**:
```
event: agent.spawned
data: {"id":"evt-123","type":"agent.spawned","timestamp":"...","data":{...}}

event: bead.created
data: {"id":"evt-124","type":"bead.created","timestamp":"...","data":{...}}
```

**Client Example**:
```javascript
const eventSource = new EventSource(
    '/api/v1/events/stream?project_id=my-project'
);

eventSource.addEventListener('agent.spawned', (e) => {
    const event = JSON.parse(e.data);
    console.log('Agent spawned:', event);
});
```

### Workflow Best Practices

1. **Idempotency**: All activities should be idempotent
2. **Timeouts**: Set appropriate timeouts for all operations
3. **Retries**: Configure retry policies for transient failures
4. **Continue-As-New**: Use for long-running workflows
5. **Signals vs Updates**: Use Updates for synchronous operations
6. **Query State**: Use queries for read-only state access
7. **Event Publishing**: Publish events for all state changes

### Deployment Considerations

1. **Worker Scaling**: Workers can be scaled horizontally
2. **Task Queues**: Use separate queues for different workflow types
3. **Namespaces**: Use different namespaces per environment
4. **Retention**: Configure workflow history retention
5. **Monitoring**: Use Temporal UI and metrics
6. **Backup**: Regular PostgreSQL backups

### Monitoring and Observability

**Temporal UI** (http://localhost:8088):
- View all workflow executions
- Inspect workflow history
- Query workflow state
- Monitor active workflows
- Debug workflow failures

**Metrics**:
- Workflow start/completion rates
- Activity success/failure rates
- Task queue depth
- Worker utilization
- Event bus throughput

**Logging**:
- Workflow execution logs
- Activity execution logs
- Event bus logs
- Integration point logs
