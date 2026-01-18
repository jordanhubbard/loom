# Arbiter

An agentic based coding orchestrator for both on-prem and off-prem development.

## Architecture

Arbiter is built with the following principles:

- **Go-First Implementation**: All primary functionality is implemented in Go for performance, maintainability, and minimal host footprint
- **Containerized Everything**: Every component runs in containers with no exceptions, ensuring consistency across environments
- **Minimal Language Footprint**: While other languages (Python, shell scripts) can be used when more appropriate, we exercise caution to minimize the number of languages and dependencies on the host system

## Prerequisites

- Docker (20.10+)
- Docker Compose (1.29+)
- Go 1.21+ (for local development only)
- Make (optional, for convenience commands)

## Quick Start

### Running with Docker (Recommended)

```bash
# Build and run using docker-compose
make docker-run

# Or manually
docker-compose up -d

# View logs
docker-compose logs -f arbiter

# Stop the service
make docker-stop
```

### Building the Docker Image

```bash
make docker-build

# Or manually
docker build -t arbiter:latest .
```

### Local Development

For local development without Docker:

```bash
# Build the binary
make build

# Run the application
make run

# Run tests
make test

# Run linters
make lint
```

## Usage

Once running, Arbiter provides an orchestration service for coding tasks:

```bash
# Check version
docker exec arbiter /app/arbiter version

# Get help
docker exec arbiter /app/arbiter help
```

## Project Structure

```
arbiter/
├── cmd/
│   └── arbiter/          # Main application entry point
│       └── main.go
├── Dockerfile            # Multi-stage Docker build
├── docker-compose.yml    # Container orchestration
├── Makefile             # Development convenience commands
├── go.mod               # Go module definition
└── README.md            # This file
```

## Development Guidelines

1. **Primary Language**: Implement all core functionality in Go
2. **Containerization**: All services, tools, and components must run in containers
3. **Additional Languages**: Only use Python or shell scripts when they provide clear advantages, and document the rationale
4. **Security**: Run containers as non-root users, use multi-stage builds to minimize image size
5. **Testing**: All code should be tested; use Go's built-in testing framework

## Contributing

When contributing to this project:

1. Ensure all code follows the architecture principles above
2. All new features must be containerized
3. Update documentation for any new features or changes
4. Run tests and linters before submitting changes
An AI Coding Agent Orchestrator for both on-prem and off-prem development.

Arbiter is a lightweight AI coding agent orchestrator, dispatcher, and automatic decision maker. Instead of being just another frontend to systems like Claude or Cursor, Arbiter intelligently routes requests to multiple AI providers and presents a unified OpenAI-compatible API.

## Features

- 🤖 **Multi-Provider Support**: Configure and use multiple AI providers (Claude, OpenAI, Cursor, Factory, and more)
- 🔒 **Secure Secret Storage**: API keys are encrypted and stored securely, never committed to git
- 🌐 **Dual Interface**: Both OpenAI-compatible REST API and web frontend
- 🔍 **Automatic Provider Discovery**: Looks up API endpoints for known providers or accepts custom URLs
- ⚡ **Lightweight**: Minimal overhead, runs as a background service
- 🎯 **Smart Routing**: Automatically routes requests to appropriate providers

## Installation

### Prerequisites

- Go 1.21 or higher (tested with Go 1.24)

### Build from Source

```bash
git clone https://github.com/jordanhubbard/arbiter.git
cd arbiter
go build
```

This will create an `arbiter` binary in the current directory.

## Quick Start

1. **Run Arbiter**:
   ```bash
   ./arbiter
   ```

2. **First-time Setup**: On first run, Arbiter will interactively guide you through configuring your AI providers:
   - Enter the names of providers you have access to (e.g., `claude, openai, cursor`)
   - For each provider, either:
     - Provide a specific API endpoint URL, or
     - Let Arbiter look up the standard endpoint for known providers
   - Enter your API key for each provider

3. **Access the Interfaces**:
   - **Web UI**: http://localhost:8080
   - **OpenAI-compatible API**: http://localhost:8080/v1/...
   - **Health Check**: http://localhost:8080/health

## Configuration

Arbiter stores configuration in two files in your home directory:

- `~/.arbiter.json`: Provider configurations (endpoints, names)
- `~/.arbiter_secrets`: Encrypted API keys (machine-specific encryption)

**Security Note**: These files are never committed to git. The secrets file uses AES-GCM encryption with a machine-specific key derived from hostname and user directory.

## API Endpoints

Arbiter provides an OpenAI-compatible API:

### Chat Completions
```bash
POST /v1/chat/completions
Content-Type: application/json

{
  "model": "claude-default",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ]
}
```

### Text Completions
```bash
POST /v1/completions
Content-Type: application/json

{
  "model": "openai-default",
  "prompt": "Once upon a time"
}
```

### List Models
```bash
GET /v1/models
```

### Health Check
```bash
GET /health
```

### List Providers
```bash
GET /api/providers
```

## Usage Examples

### Using with curl

```bash
# Chat completion
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-default",
    "messages": [{"role": "user", "content": "Write a haiku about coding"}]
  }'

# Check health
curl http://localhost:8080/health
```

### Using with Python OpenAI Client

```python
from openai import OpenAI

# Point the client to Arbiter
client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="not-needed"  # Arbiter manages keys
)

response = client.chat.completions.create(
    model="claude-default",
    messages=[{"role": "user", "content": "Hello!"}]
)

print(response.choices[0].message.content)
```

## Supported Providers

Arbiter has built-in support for the following providers with automatic endpoint lookup:

- **Claude** (Anthropic): `https://api.anthropic.com/v1`
- **OpenAI**: `https://api.openai.com/v1`
- **Cursor**: `https://api.cursor.sh/v1`
- **Factory**: `https://api.factory.ai/v1`
- **Cohere**: `https://api.cohere.ai/v1`
- **HuggingFace**: `https://api-inference.huggingface.co`
- **Replicate**: `https://api.replicate.com/v1`
- **Together**: `https://api.together.xyz/v1`
- **Mistral**: `https://api.mistral.ai/v1`
- **Perplexity**: `https://api.perplexity.ai`

For any other provider, you can manually specify the API endpoint during setup.

## Architecture

```
┌─────────────────────────────────────────┐
│           User Application              │
│  (CLI, IDE Plugin, Web Client, etc.)    │
└───────────────┬─────────────────────────┘
                │
                │ OpenAI-compatible API
                │
┌───────────────▼─────────────────────────┐
│           Arbiter Server                │
│  ┌─────────────────────────────────┐   │
│  │  Request Router & Dispatcher    │   │
│  └─────────────────────────────────┘   │
│  ┌─────────────────────────────────┐   │
│  │    Encrypted Secret Store       │   │
│  └─────────────────────────────────┘   │
└───────────────┬─────────────────────────┘
                │
        ┌───────┴───────┬─────────┐
        │               │         │
┌───────▼──────┐ ┌──────▼─────┐ ┌▼────────┐
│   Claude     │ │   OpenAI   │ │ Cursor  │
│   Provider   │ │  Provider  │ │ Provider│
└──────────────┘ └────────────┘ └─────────┘
```

## Development

### Building

```bash
go build
```

### Running

```bash
./arbiter
```

### Project Structure

```
arbiter/
├── main.go                    # Application entry point
├── pkg/
│   ├── config/
│   │   └── config.go         # Configuration management
│   ├── secrets/
│   │   └── store.go          # Encrypted secret storage
│   └── server/
│       ├── server.go         # HTTP server implementation
│       └── types.go          # API types
├── go.mod                     # Go module definition
├── README.md                  # This file
└── .gitignore                # Git ignore rules
```

## Security Considerations

- API keys are encrypted using AES-GCM with a 256-bit key
- Encryption key is derived from machine-specific data (hostname + home directory)
- Secrets file has restricted permissions (0600)
- Configuration and secrets are stored in home directory, never in repository
- No secrets are logged or exposed in API responses

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

See LICENSE file for details.

## Roadmap

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

For issues, questions, or contributions, please use the GitHub issue tracker.
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

