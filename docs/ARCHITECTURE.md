# AgentiCorp Architecture Guide

**Last Updated**: January 21, 2026 (Analytics v1.1)

This document describes the architecture of AgentiCorp, the Agent Orchestration System for managing distributed AI workflows.

## System Overview

AgentiCorp is a comprehensive agent orchestration platform that:
- Coordinates multiple AI agents with different roles (personas)
- Manages work items (beads) through a distributed workflow engine
- Integrates with external LLM providers for agent execution
- Uses Temporal for reliable workflow orchestration
- Persists all state to a SQLite database
- Provides a real-time web UI for monitoring and control
- **v1.0**: Multi-user authentication with role-based access control
- **v1.0**: Intelligent provider routing with cost and latency optimization
- **v1.0**: Server-sent events for real-time streaming responses
- **NEW (v1.1)**: Analytics dashboard with real-time usage monitoring
- **NEW (v1.1)**: Per-user and per-provider cost tracking
- **NEW (v1.1)**: Privacy-first logging with GDPR compliance
- **NEW (v1.1)**: Spending alerts and anomaly detection

## Core Components

### 1. Agent System

**Purpose**: Manage autonomous AI agents with role-based personas

**Key Files**: 
- `internal/agenticorp/worker_manager.go`
- `internal/models/agent.go`

**Concepts**:
- **Agent**: An autonomous actor with a defined role and persona
- **Persona**: Instructions, guidelines, and behavioral rules for an agent
- **Status**: `idle`, `working`, `paused`, `complete`
- **Role**: Org chart position (CEO, CFO, Engineer, etc.)

**Workflow**:
1. Agents are created from personas on project assignment
2. Agents remain paused until a provider becomes available
3. When providers are healthy, agents resume and can accept work
4. Agents process beads, make decisions, request escalations

**Database**: `agents` table with status, role, and capability tracking

### 2. Bead System (Work Items)

**Purpose**: Define and track discrete units of work in the system

**Key Files**:
- `internal/models/bead.go`
- `docs/BEADS_WORKFLOW.md`

**Concepts**:
- **Bead**: A YAML-defined work item with type, priority, and dependencies
- **Status**: `open`, `in_progress`, `done`, `blocked`
- **Type**: Describes the work (feature, bugfix, test, decision, etc.)
- **Dependencies**: `blocked_by`, `blocks`, `parent`, `children`

**Workflow**:
1. Beads are YAML files in `.beads/beads/` directories
2. Each bead has a unique ID and metadata
3. Beads are loaded at startup and on project changes
4. Agents process ready beads (no blocking dependencies)
5. Completion updates the work graph in real-time

**Database**: `beads` table with status, priority, and dependency tracking

### 3. Provider System

**Purpose**: Interface with external LLM providers with intelligent routing and cost optimization

**Key Files**:
- `pkg/provider/provider.go`
- `internal/models/provider.go`
- `internal/routing/router.go` (NEW v1.0)
- `internal/agenticorp/agenticorp.go` (provider management)

**Concepts**:
- **Provider**: An external LLM service (local or cloud)
- **Endpoint**: Network address for provider communication
- **Model**: LLM served by the provider (e.g., Nemotron, GPT-4)
- **Status**: `pending`, `active`, `error`
- **Cost Metadata** (NEW v1.0): Cost per million tokens for optimization
- **Capabilities** (NEW v1.0): Context window, function calling, vision support
- **Routing Policy** (NEW v1.0): Selection strategy (cost, latency, quality, balanced)

**Workflow**:
1. Providers are registered via UI with endpoint, model, and cost info
2. Immediate health check validates provider availability
3. Provider heartbeat workflow monitors health (30s interval)
4. On status change, agents resume or pause accordingly
5. **Router selects optimal provider** based on policy and requirements (NEW v1.0)
6. Automatic failover if selected provider fails

**Database**: `providers` table with endpoint, status, cost, capabilities, and heartbeat tracking

### 3.1 Provider Routing System (NEW v1.0)

**Purpose**: Intelligently select providers based on cost, latency, quality, and capabilities

**Key Files**:
- `internal/routing/router.go`
- `internal/routing/router_test.go`
- `internal/api/handlers_routing.go`

**Routing Policies**:
1. **minimize_cost**: Select cheapest provider (30%+ savings)
2. **minimize_latency**: Select fastest provider (<1ms routing)
3. **maximize_quality**: Select provider with best capabilities
4. **balanced** (default): Balance cost (30%), latency (30%), quality (40%)

**Provider Requirements**:
- `MaxCostPerMToken`: Maximum acceptable cost
- `MaxLatencyMs`: Maximum acceptable latency
- `MinContextWindow`: Minimum context window size
- `RequiresFunction`: Must support function calling
- `RequiresVision`: Must support vision/multimodal
- `RequiredTags`: Custom capability tags

**Automatic Failover**:
- Health criteria: status, heartbeat recency, success rate
- Circuit breaker pattern prevents cascading failures
- Transparent failover to backup providers
- Excluded providers list for retry logic

**API Endpoints**:
- `POST /api/v1/routing/select` - Select provider with policy
- `GET /api/v1/routing/policies` - List available policies

### 3.2 Authentication & Authorization System (NEW v1.0)

**Purpose**: Secure multi-user deployments with role-based access control

**Key Files**:
- `internal/auth/manager.go`
- `internal/auth/middleware.go`
- `internal/auth/handlers.go`
- `internal/auth/models.go`

**Authentication Methods**:
1. **JWT Bearer Tokens**: For user sessions (24h expiration)
2. **API Keys**: For service-to-service integrations

**User Roles**:
- `admin`: Full system access (*:* permissions)
- `user`: Read/write access to most resources
- `viewer`: Read-only access
- `service`: Custom permissions per API key

**Permissions**:
Format: `<resource>:<action>` (e.g., `agents:read`, `beads:write`)
- Resources: agents, beads, providers, projects, decisions, repl, system
- Actions: read, write, delete, admin
- Wildcards: `*:*`, `agents:*`

**Per-User Provider Isolation**:
- Providers have `owner_id` and `is_shared` fields
- Users see only their providers + shared providers
- Query: `WHERE owner_id = ? OR is_shared = 1 OR owner_id IS NULL`

**User Management UI**:
- Admin-only "Users" tab for user CRUD
- Role assignment with visual badges
- API key generation with one-time display
- Secure key revocation

**API Endpoints**:
- `POST /api/v1/auth/login` - Get JWT token
- `POST /api/v1/auth/api-keys` - Create API key
- `GET /api/v1/auth/api-keys` - List user's keys
- `DELETE /api/v1/auth/api-keys/{id}` - Revoke key
- `POST /api/v1/auth/users` - Create user (admin only)
- `GET /api/v1/auth/users` - List users (admin only)

**Database**: `users` and `api_keys` managed in-memory by auth.Manager

### 4. Project System

**Purpose**: Organize work into distinct projects with independent workflows

**Key Files**:
- `internal/models/project.go`
- `internal/agenticorp/project_manager.go`

**Concepts**:
- **Project**: A container for beads, agents, and work
- **Git Integration**: Projects map to git repos with branches
- **Beads Path**: Directory where work items are stored
- **Sticky**: Auto-registered on startup
- **Perpetual**: Never completes (continuous operations)

**Workflow**:
1. Projects are defined in `config.yaml`
2. Beads are loaded from git at startup
3. Agents are assigned to projects
4. Work is tracked and dispatched per project
5. Project state persists across restarts

**Database**: `projects` table with git, status, and configuration

### 5. Org Chart System

**Purpose**: Define and manage team structure for projects

**Key Files**:
- `pkg/models/orgchart.go`
- `internal/orgchart/manager.go`

**Concepts**:
- **OrgChart**: Project-specific team structure defining roles and reporting
- **Position**: A role slot in the org chart (CEO, PM, Engineer, etc.)
- **Template**: Default org chart cloned for new projects
- **Hierarchy**: Positions have reporting relationships (ReportsTo field)
- **Capacity**: Positions can limit instances (MaxInstances)

**Workflow**:
1. Default template created on system startup with all standard roles
2. New projects get org chart cloned from template
3. Agents are assigned to fill positions in the org chart
4. UI displays agents sorted by org chart hierarchy
5. Required positions must be filled for project to be active

**API**: `GET /api/v1/org-charts/{projectId}` for retrieving project org structure

**Database**: `org_charts` and `org_chart_positions` tables (schema exists, in-memory storage currently used)

### 6. Model Catalog System

**Purpose**: Manage recommended models and enable intelligent provider negotiation

**Key Files**:
- `internal/modelcatalog/catalog.go`
- `internal/models/model_catalog.go`

**Concepts**:
- **ModelSpec**: Metadata about a model (params, precision, interactivity)
- **Parsing**: Automatic extraction of model attributes from names
- **Scoring**: Heuristic ranking based on size, speed, and quality
- **Negotiation**: SelectBest chooses optimal model from available options

**Features**:
- Parses model names to extract total/active params (MoE support)
- Detects precision (BF16, FP16, INT8, etc.)
- Identifies instruct-tuned models
- Scores models by interactivity (fast/medium/slow) and size
- Case-insensitive matching during negotiation

**Workflow**:
1. Provider registration includes configured model preference
2. Provider bootstrap queries available models
3. Catalog.SelectBest picks highest-scoring available model
4. Selection reason and score persisted to provider metadata
5. Provider negotiation can be manually retriggered via API

**Database**: Provider table stores `selected_model`, `selection_reason`, and `model_score`

### 7. Temporal Orchestration

**Purpose**: Provide reliable, durable workflow execution with temporal primitives

**Key Files**:
- `internal/temporal/manager.go` (orchestration)
- `internal/temporal/workflows/` (workflow definitions)
- `internal/temporal/activities/` (work implementations)
- `internal/temporal/dsl_parser.go` (DSL parsing)
- `internal/temporal/dsl_executor.go` (DSL execution)

**Concepts**:
- **Workflow**: Long-running business process with retry/resume semantics
- **Activity**: Atomic unit of work within a workflow
- **Signal**: Message sent to running workflows
- **Query**: Request for state from running workflows
- **Schedule**: Recurring workflow execution

**Key Workflows**:
- `AgentiCorpHeartbeatWorkflow`: Master clock (10s interval)
- `DispatcherWorkflow`: Work distribution (5s interval)
- `ProviderHeartbeatWorkflow`: Provider health checks (30s interval)
- `BeadProcessingWorkflow`: Agent work execution
- `DecisionWorkflow`: Escalation and approval workflows

**DSL Support**:
- Agents/providers can embed `<temporal>...</temporal>` blocks
- DSL is parsed, executed, and stripped before sending to providers
- See `docs/TEMPORAL_DSL.md` for complete syntax

### 8. Database Layer

**Purpose**: Persist all system state across restarts

**Key Files**:
- `internal/database/database.go`
- `internal/database/models.go`

**Implementation**: SQLite with embedded database

**Key Tables**:
- `agents`: Agent state, role, status
- `beads`: Work items with dependencies
- `providers`: Provider configuration and health
- `projects`: Project metadata and configuration
- `decisions`: Approval/escalation requests
- `personas`: Agent role definitions

**Features**:
- Automatic schema creation on startup
- Transaction support for consistency
- Backup functionality
- Clean reset with `make distclean`

### 9. Event Bus

**Purpose**: Coordinate system-wide events

**Key Files**:
- `internal/temporal/eventbus/event_bus.go`

**Event Types**:
- `EventTypeProviderRegistered`: New provider added
- `EventTypeProviderDeleted`: Provider removed
- `EventTypeBeadStatusChanged`: Work item status change
- `EventTypeAgentStatusChanged`: Agent status change

### 10. Web UI

**Purpose**: Real-time monitoring and control of the system

**Key Files**:
- `web/static/js/app.js`: UI logic
- `web/static/index.html`: UI structure
- `web/static/css/style.css`: UI styling

**Sections**:
- **Project Viewer**: Browse projects, agents, and beads
- **Kanban Board**: Visualize work by status
- **Providers**: Register and manage LLM providers
- **Agents**: View agent assignments and status
- **Decisions**: Approve/deny escalations
- **Personas**: Define agent roles
- **Projects**: Manage project configuration
- **CEO REPL**: Direct query interface
- **System Status**: Overall health overview

### 11. Analytics & Cost Tracking System (NEW v1.1)

**Purpose**: Monitor usage, track costs, and provide insights into system utilization

**Key Files**:
- `internal/analytics/logger.go` - Request/response logging with privacy controls
- `internal/analytics/storage.go` - SQLite storage for logs
- `internal/analytics/alerts.go` - Spending alerts and anomaly detection
- `internal/api/handlers_analytics.go` - Analytics API endpoints
- `web/static/index.html` - Analytics dashboard UI

**Concepts**:
- **Request Log**: Individual API call record with metadata, tokens, cost, latency
- **Privacy Config**: Configurable logging with GDPR-compliant defaults
- **Cost Tracking**: Per-user and per-provider cost aggregation
- **Alert Config**: Budget thresholds and anomaly detection settings
- **Export**: CSV and JSON export for external analysis

**Analytics Components**:

1. **Logger**:
   - Logs all API requests with metadata
   - Privacy-first: request/response bodies NOT logged by default
   - PII redaction (emails, API keys, cards, SSNs)
   - Configurable body logging and max length
   - Generates unique log IDs with timestamps

2. **Storage**:
   - SQLite `request_logs` table with 15+ fields
   - Indexes on timestamp, user_id, provider_id
   - Efficient queries for stats and filtering
   - Aggregate functions for cost/usage totals
   - Per-user and per-provider breakdowns

3. **Cost Tracking**:
   - Token-based cost calculation: `(tokens / 1M) × cost_per_mtoken`
   - Real-time cost aggregation (no batch jobs)
   - Historical cost tracking with time-range filtering
   - Cost per request and cost per 1K tokens metrics
   - Accurate to provider pricing models

4. **Alerting System**:
   - Daily budget alerts (default: $100/day)
   - Monthly budget alerts (default: $2000/month)
   - Anomaly detection using 7-day rolling average (default: 2x threshold)
   - Configurable thresholds per user
   - Multiple severity levels (info, warning, critical)
   - Notification hooks for email/webhook (extensible)

5. **Analytics Dashboard**:
   - Real-time usage monitoring
   - Summary cards: requests, cost, latency, error rate
   - Time-range filters: 1h, 24h, 7d, 30d, custom
   - Bar charts: cost and requests by provider/user
   - Detailed breakdown table
   - Responsive design for mobile/desktop

6. **Data Export**:
   - CSV export (Excel/Sheets compatible)
   - JSON export (programmatic processing)
   - Stats export (aggregate summaries)
   - Logs export (individual requests)
   - One-click UI export buttons
   - API endpoints with time-range filtering

**API Endpoints**:
- `GET /api/v1/analytics/logs` - Retrieve request logs
- `GET /api/v1/analytics/stats` - Get aggregate statistics
- `GET /api/v1/analytics/costs` - Get cost breakdown
- `GET /api/v1/analytics/export` - Export logs (CSV/JSON)
- `GET /api/v1/analytics/export-stats` - Export stats (CSV/JSON)

**Privacy & Security**:
- GDPR-compliant defaults (no body logging)
- Automatic PII redaction with regex patterns
- Users see only their own data
- Admins can filter by user ID
- Configurable data retention
- Purge old logs functionality

**Database**:
- `request_logs` table with indexes
- Schema: id, timestamp, user_id, method, path, provider_id, model_name, prompt_tokens, completion_tokens, total_tokens, latency_ms, status_code, cost_usd, error_message, request_body, response_body, metadata_json, created_at

**Performance**:
- Dashboard loads in <2s
- Export handles 10K+ records efficiently
- Real-time cost calculations (<5ms overhead)
- Indexed queries (<10ms typical)

**Testing**:
- 17 comprehensive tests
- Cost calculation tests
- Per-user/per-provider tracking tests
- Privacy/redaction tests
- Alert detection tests
- Anomaly detection tests

See [docs/ANALYTICS_GUIDE.md](ANALYTICS_GUIDE.md) for usage details.

### 12. Testing Infrastructure

**Purpose**: Ensure system reliability through comprehensive testing

**Key Files**:
- `internal/modelcatalog/catalog_test.go` - Model parsing and negotiation tests
- `internal/orgchart/manager_test.go` - Org chart operations tests
- `tests/postflight/api_test.sh` - Post-deployment API validation

**Test Categories**:

**Unit Tests**:
- Model name parsing (extracts params, precision, vendor)
- Model scoring and ranking logic
- Org chart creation and position management
- Provider registry operations

**Integration Tests**:
- API endpoint validation (14 endpoints tested)
- Health checks and system status
- Event stream connectivity
- Work graph dependency tracking

**Post-Flight Tests**:
- Automated validation after container startup
- Tests all major API endpoints
- Validates JSON response structure
- Configurable via BASE_URL environment variable
- Run with: `make test-api`

**CI/CD**:
- `make test` runs all Go tests
- `make test-api` runs post-flight validation
- Build must pass before deployment
- Test failures block merges

## Data Flow

### Work Distribution Flow

```
Beads Load (Startup)
    ↓
Provider Registration (UI/API)
    ↓
Provider Heartbeat (Temporal, 30s)
    ↓
Provider Health Check (Immediate on registration)
    ↓
Agent Resume (When provider healthy)
    ↓
Dispatcher (Temporal, 5s)
    ↓
Get Ready Beads (No blocking dependencies)
    ↓
Route to Agent (Best agent for bead type)
    ↓
Agent Execution (Via Provider)
    ↓
Bead Complete / Update Dependencies
    ↓
Next Ready Beads Available
```

### Agent Processing Flow

```
Agent Receives Bead
    ↓
Extract Temporal DSL (if any) from persona instructions
    ↓
Execute Temporal DSL (workflows, schedules, queries)
    ↓
Strip DSL from instructions (clean text)
    ↓
Send Clean Instructions + Bead to Provider
    ↓
Provider Executes (may return response with DSL)
    ↓
Parse Provider Response (extract DSL if present)
    ↓
Execute Response DSL (workflows, signals, etc.)
    ↓
Store Clean Response in Database
    ↓
Update Bead Status
```

## Configuration

### config.yaml

Main configuration file with:

```yaml
# API Configuration
api:
  port: 8080
  host: 0.0.0.0

# Database
database:
  path: ./agenticorp.db

# Temporal
temporal:
  host: temporal:7233
  namespace: agenticorp-default
  task_queue: agenticorp-tasks
  enabled: true

# Projects
projects:
  - id: myproject
    name: My Project
    git_repo: https://github.com/user/repo
    branch: main
    beads_path: .beads

# Providers (can also be registered via UI)
providers:
  - id: local-vllm
    name: Local vLLM
    type: local
    endpoint: http://localhost:8000
    model: nvidia/Nemotron
```

## Deployment

### Local Development

```bash
# Start full stack with Docker
docker compose up -d

# Or run locally with external Temporal
make run
```

### Docker Deployment

- AgentiCorp container on port 8080
- Temporal server on port 7233
- Temporal UI on port 8088
- PostgreSQL for Temporal state

## State Persistence

All state is persisted to:
1. **SQLite Database**: Core application state
2. **Temporal Server**: Workflow execution state
3. **Bead YAML Files**: Work item definitions

State survives container restarts. Clean with:

```bash
make distclean  # Wipes database and Temporal state
```

## High-Level Architecture Diagram

**Last Updated**: January 20, 2026

### System Component Diagram

```mermaid
graph TB
    subgraph "Web Layer"
        UI[Web UI :8080<br/>React SPA]
        API[REST API<br/>/api/v1/*]
        SSE[SSE Event Stream<br/>/api/v1/events/stream]
    end

    subgraph "Core Engine"
        subgraph "Managers"
            PM[Project Manager]
            AM[Agent Manager]
            BM[Bead Manager]
            DM[Decision Manager]
            OC[Org Chart Manager]
            PRM[Provider Registry]
            PSM[Persona Manager]
        end
        
        subgraph "Orchestration"
            DISP[Dispatcher<br/>Routes work to agents]
            EB[Event Bus<br/>Real-time notifications]
            WG[Work Graph<br/>Dependency tracking]
        end
    end

    subgraph "Temporal Workflows"
        TM[Temporal Manager]
        AGW[Agent Workflow<br/>Process beads]
        HBW[Heartbeat Workflow<br/>Monitor providers]
        DSLW[DSL Executor<br/>Custom workflows]
    end

    subgraph "External Services"
        TP[Temporal Server<br/>:7233]
        DB[(SQLite Database<br/>State persistence)]
        GIT[Git Repositories<br/>Bead sources]
    end

    subgraph "LLM Providers"
        PROV1[vLLM Provider]
        PROV2[Ollama Provider]
        PROV3[OpenAI API]
    end

    UI --> API
    UI --> SSE
    API --> PM
    API --> AM
    API --> BM
    API --> DM
    API --> PRM
    API --> OC
    
    PM --> DB
    AM --> DB
    BM --> DB
    DM --> DB
    PRM --> DB
    OC --> DB
    
    PM -.loads beads.-> GIT
    PM --> OC
    OC -.defines roles.-> AM
    
    DISP --> AM
    DISP --> BM
    DISP --> WG
    DISP --> TM
    
    TM --> TP
    AGW --> TP
    HBW --> TP
    DSLW --> TP
    
    AM --> AGW
    PRM --> HBW
    
    AGW -.requests completion.-> PRM
    PRM -.routes to.-> PROV1
    PRM -.routes to.-> PROV2
    PRM -.routes to.-> PROV3
    
    EB -.publishes.-> SSE
    AM --> EB
    BM --> EB
    PM --> EB
    PRM --> EB
    
    style UI fill:#e1f5ff
    style API fill:#e1f5ff
    style SSE fill:#e1f5ff
    style DB fill:#fff4e1
    style TP fill:#fff4e1
    style GIT fill:#fff4e1
    style PROV1 fill:#e8f5e9
    style PROV2 fill:#e8f5e9
    style PROV3 fill:#e8f5e9
```

### Data Flow: Bead Processing

```mermaid
sequenceDiagram
    participant User
    participant Git
    participant PM as Project Manager
    participant BM as Bead Manager
    participant WG as Work Graph
    participant DISP as Dispatcher
    participant AM as Agent Manager
    participant PROV as Provider
    participant TP as Temporal
    participant DB as Database

    User->>Git: Push .beads/beads/*.yaml
    PM->>Git: Pull project beads
    PM->>BM: Load beads
    BM->>WG: Update dependency graph
    BM->>DB: Persist beads
    
    DISP->>WG: Get ready beads
    WG-->>DISP: Beads with no blockers
    DISP->>AM: Get available agents
    AM-->>DISP: Agents matching role
    
    DISP->>TP: Start AgentWorkflow
    TP->>AM: Execute bead
    AM->>PROV: Request completion
    PROV-->>AM: LLM response
    AM->>BM: Update bead status
    BM->>DB: Persist update
    BM->>WG: Update dependencies
```

### Org Chart Structure

```mermaid
graph TD
    subgraph "Project: AgentiCorp"
        PC[Project Config]
        OC[Org Chart]
        
        PC --> OC
        
        subgraph "Executive Positions"
            CEO[CEO<br/>Required: 1]
            CFO[CFO<br/>Max: 1]
            CEO --> CFO
        end
        
        subgraph "Product & Engineering"
            PM[Product Manager<br/>Unlimited]
            EM[Engineering Manager<br/>Unlimited]
            CEO --> PM
            CEO --> EM
            
            PROJM[Project Manager<br/>Unlimited]
            QA[QA Engineer<br/>Unlimited]
            DO[DevOps Engineer<br/>Unlimited]
            CR[Code Reviewer<br/>Unlimited]
            
            EM --> PROJM
            EM --> QA
            EM --> DO
            EM --> CR
        end
        
        subgraph "Support Functions"
            DOCS[Doc Manager<br/>Max: 1]
            WD[Web Designer<br/>Unlimited]
            PR[PR Manager<br/>Max: 1]
            DM[Decision Maker<br/>Max: 1]
            HK[Housekeeping<br/>Max: 1]
            
            PM --> DOCS
            PM --> WD
            CEO --> PR
            CEO --> DM
        end
        
        OC --> CEO
    end
    
    style CEO fill:#ffeb3b
    style CFO fill:#ffeb3b
    style OC fill:#e3f2fd
```

### ASCII Diagram (Legacy)

```
┌─────────────────────────────────────────────────────────────┐
│                      Web UI (Port 8080)                     │
│  Projects | Providers | Agents | Beads | Decisions | REPL  │
└────────────────┬──────────────────────────────────────────┘
                 │
┌────────────────▼──────────────────────────────────────────┐
│              AgentiCorp Core Engine                        │
├─────────────────────────────────────────────────────────┤
│                                                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │   Agent     │  │   Project   │  │   Bead      │     │
│  │   Manager   │  │   Manager   │  │   Manager   │     │
│  └────────┬────┘  └─────────────┘  └─────────────┘     │
│           │                                              │
│  ┌────────▼─────────────────────────────────────────┐  │
│  │        Dispatcher (Temporal Workflow)            │  │
│  │         - Routes work to agents                  │  │
│  │         - Manages dependencies                   │  │
│  └────────┬─────────────────────────────────────────┘  │
│           │                                              │
└───────────┼──────────────────────────────────────────┬─┘
            │                                          │
      ┌─────▼──────────┐                    ┌─────────▼─────┐
      │  Temporal      │                    │   Database    │
      │  (Workflows)   │                    │   (SQLite)    │
      │  (Port 7233)   │                    │               │
      └────────────────┘                    └───────────────┘
            │
      ┌─────▼──────────────────────┐
      │  Provider Heartbeat        │
      │  - Check health (30s)      │
      │  - Update status           │
      └─────────┬──────────────────┘
                │
         ┌──────▼──────────┐
         │  LLM Providers  │
         │  (vLLM, Ollama) │
         └─────────────────┘
```

## Extensions

AgentiCorp can be extended via:

1. **Custom Personas**: Add new agent roles in `personas/`
2. **Custom Beads**: Define work items in project `.beads/` directories
3. **Temporal Workflows**: Add workflows in `internal/temporal/workflows/`
4. **Custom Providers**: Register new LLM endpoints
5. **Temporal DSL**: Use DSL in agent instructions for workflows

See individual documentation files for details.
