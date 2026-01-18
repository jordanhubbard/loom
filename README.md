# arbiter
An agentic based coding orchestrator for both on-prem and off-prem development

Arbiter is a web-based service that helps orchestrate and monitor AI agents working on coding tasks. It provides:
- Work queue management for tracking tasks
- Agent communication monitoring
- Service endpoint tracking with cost analysis
- Priority-based routing that favors fixed-cost (self-hosted) services

## Features

### Work Management
- Submit new work items via REST API
- Track work in progress
- Monitor work status and assignments

### Agent Monitoring
- View active agents and their status
- Monitor inter-agent communications
- Track which service endpoints agents are using

### Service Endpoints
- Track multiple LLM service endpoints (OpenAI, Anthropic, Ollama, vLLM, etc.)
- Monitor token usage and costs
- Prioritize fixed-cost self-hosted services (Ollama, vLLM)
- Interactive cost management UI
- Real-time traffic monitoring

### Cost Tracking
- **Fixed-cost services**: Mark self-hosted services (Ollama, vLLM) as fixed-cost
- **Variable-cost services**: Track per-token costs for paid APIs
- **Automatic prioritization**: System prioritizes fixed-cost services to minimize expenses
- **Interactive cost editing**: Click on any service in the UI to update its cost model

## Getting Started

### Prerequisites
- Go 1.21 or higher

### Installation

1. Clone the repository:
```bash
git clone https://github.com/jordanhubbard/arbiter.git
cd arbiter
```

2. Install dependencies:
```bash
go mod download
```

3. Build the application:
```bash
go build -o arbiter ./cmd/arbiter
```

4. Run the server:
```bash
./arbiter
```

The server will start on port 8080 by default. You can change this by setting the `PORT` environment variable:
```bash
PORT=3000 ./arbiter
```

### Web UI

Once the server is running, open your browser to:
- **Dashboard**: http://localhost:8080
- **API**: http://localhost:8080/api

## API Reference

### Work Endpoints

#### Create Work
```http
POST /api/work/create
Content-Type: application/json

{
  "description": "Implement new feature X"
}
```

#### List All Work
```http
GET /api/work
```

#### List Work In Progress
```http
GET /api/work?status=in_progress
```

### Agent Endpoints

#### List Agents and Communications
```http
GET /api/agents
```

Returns:
```json
{
  "agents": [...],
  "communications": [...]
}
```

### Service Endpoints

#### List All Services
```http
GET /api/services
```

#### List Active Services Only
```http
GET /api/services?active=true
```

#### Get Preferred Services (Fixed-cost first)
```http
GET /api/services/preferred
```

#### Get Service Costs
```http
GET /api/services/:id/costs
```

#### Update Service Costs
```http
PUT /api/services/:id/costs
Content-Type: application/json

{
  "cost_type": "fixed",
  "fixed_cost": 0
}
```

Or for variable cost:
```json
{
  "cost_type": "variable",
  "cost_per_token": 0.00003
}
```

#### Record Service Usage (for simulation/testing)
```http
POST /api/services/:id/usage
Content-Type: application/json

{
  "tokens_used": 1000
}
```

## Development

### Running Tests
```bash
go test ./...
```

### Running with Verbose Test Output
```bash
go test ./... -v
```

### Project Structure
```
arbiter/
├── cmd/arbiter/          # Main application
│   ├── main.go          # Entry point
│   └── web/             # Web UI files
│       ├── index.html   # Dashboard UI
│       ├── style.css    # Styles
│       └── app.js       # Frontend JavaScript
├── internal/
│   ├── api/             # HTTP handlers
│   ├── models/          # Data models
│   └── storage/         # In-memory storage
├── go.mod
└── README.md
```

## Service Priority System

The arbiter automatically prioritizes services based on their cost model:

1. **Fixed-cost services** (e.g., self-hosted Ollama/vLLM): Highest priority
   - Zero or fixed monthly cost
   - Marked as "fixed" cost type
   - Always preferred when available

2. **Variable-cost services** (e.g., OpenAI, Anthropic): Lower priority
   - Pay-per-token pricing
   - Marked as "variable" cost type
   - Used when fixed-cost services are unavailable or overloaded

The `/api/services/preferred` endpoint returns services in priority order, allowing agents to select the most cost-effective service first.

## Web UI Features

### Dashboard
- Create new work items
- View work in progress
- Monitor active agents
- See agent communications
- Manage service endpoints

### Service Management
- View all services or filter by active status
- See preferred service order (fixed-cost first)
- Click any service to edit its cost model
- Real-time token usage and cost tracking
- Visual indicators for service status and cost type

### Cost Editing
Click on any service in the dashboard to:
- Switch between fixed and variable cost models
- Set fixed cost amounts
- Configure per-token pricing
- Update cost models on the fly

## License

See LICENSE file for details.

