# Contributing to Loom

Thank you for your interest in contributing to Loom!

## Getting Started

### Prerequisites

- Docker and Docker Compose
- Go 1.25+
- Git
- [bd (beads)](https://github.com/steveyegge/beads) -- for issue tracking

### Development Setup

```bash
git clone https://github.com/jordanhubbard/loom.git
cd loom
make dev-setup    # Download deps, create config.yaml
make start        # Build and start in Docker
```

Loom UI at http://localhost:8080, Temporal UI at http://localhost:8088.

## Development Workflow

### Making Changes

1. **Create a branch**
```bash
git checkout -b feature/your-feature-name
```

2. **File a bead for your work**
```bash
bd create --title="Your feature" --type=feature --priority=2
```

3. **Make your changes**, then format and test:
```bash
make lint         # fmt + vet + yaml + docs
make test         # Run tests locally
make test-docker  # Run tests with Temporal
```

4. **Rebuild and test in Docker**
```bash
make restart      # Rebuild container with your changes
```

5. **Commit and push**
```bash
git add <files>
git commit -m "Brief description of changes"
git push origin feature/your-feature-name
```

6. **Close your bead**
```bash
bd close <bead-id>
bd sync
```

## Project Structure

```
loom/
├── cmd/loom/              # Application entry point
├── internal/              # Internal packages
│   ├── actions/           # Agent action types and execution
│   ├── agent/             # Agent management and worker pool
│   ├── api/               # HTTP API handlers
│   ├── beads/             # Beads integration
│   ├── database/          # SQLite persistence
│   ├── dispatch/          # Bead dispatcher
│   ├── gitops/            # Git operations and SSH keys
│   ├── keymanager/        # Encrypted credential storage
│   ├── loom/              # Core orchestration
│   ├── provider/          # LLM provider protocol
│   ├── temporal/          # Temporal workflows and activities
│   └── worker/            # Worker execution engine
├── pkg/                   # Public packages (config, models)
├── personas/              # Agent persona definitions
├── web/static/            # Frontend (vanilla JS, no framework)
├── workflows/             # Workflow definitions
├── scripts/               # Build and deployment scripts
└── .beads/                # Issue tracking (beads)
```

## Coding Standards

### Go Code

- Follow standard Go conventions
- Run `make fmt` and `make vet` before committing
- Write tests for new functionality
- Keep functions focused and single-purpose

### Frontend

- Vanilla JavaScript (no frameworks)
- All UI in `web/static/js/app.js`
- CSS uses custom properties (app CSS vars)
- Bump `?v=N` in `web/static/index.html` after JS/CSS changes

### API Changes

- Maintain backward compatibility when possible
- Document breaking changes clearly

## Testing

```bash
make test          # Local unit tests
make test-docker   # Integration tests with Temporal
make coverage      # Coverage report
```

## Pull Request Process

1. Ensure tests pass (`make test`)
2. Code is formatted (`make lint`)
3. Include a clear description referencing the bead ID
4. Wait for maintainer review

## Questions?

- Open an issue on GitHub
- Check existing issues and documentation
- Review [AGENTS.md](AGENTS.md) for the full developer guide
