# Changelog

All notable changes to AgentiCorp will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Configuration-based authentication toggle via `security.enable_auth` flag in `config.yaml`
- Authentication can now be completely disabled for development environments
- `CONFIG_PATH` environment variable support for specifying config file location
- Comprehensive authentication configuration documentation (`docs/AUTH_CONFIGURATION.md`)

### Changed
- `main.go` now loads configuration from `config.yaml` instead of using hardcoded defaults
- Docker container now uses volume-mounted config file for live configuration updates
- Default authentication setting changed to `false` for development (set to `true` in production)

### Fixed
- Config file changes now properly apply when container restarts
- Authentication middleware correctly respects `enable_auth` configuration flag

## [Unreleased]

### Added
- Initial release infrastructure
- Automated release script with semantic versioning
- CHANGELOG generation from git commits

## [2.0.0] - 2026-01-21

### Added
- **Developer Experience (Milestone v2.0)**
  - VS Code extension with AI chat panel and inline code suggestions
  - JetBrains plugin for IntelliJ IDEA, PyCharm, etc.
  - Vim/Neovim plugin with terminal-based chat interface
  - Web-based persona editor with visual UI
  - 15+ pre-built persona templates
  - Visual workflow builder documentation
  - Comprehensive persona testing and validation framework

### Added
- **Extensibility & Scale (Milestone v1.2)**
  - Custom provider plugin system
  - HTTP and native plugin support
  - Plugin manifest and discovery
  - Distributed deployment with PostgreSQL
  - Distributed locking and instance registry
  - Health check endpoints (/health, /health/live, /health/ready)
  - Graceful shutdown and startup managers
  - Load balancing support (Nginx, HAProxy, AWS ALB, GCP)
  - Horizontal scaling guide with auto-scaling patterns

### Added
- **Analytics & Intelligence (Milestone v1.1)**
  - Request logging and analytics system
  - Cost tracking per user and provider
  - Spending alerts and budget monitoring
  - Intelligent response caching with LRU eviction
  - Redis cache backend support
  - Cache invalidation by provider, model, age, pattern

### Features
- Agent orchestration with personas
- Temporal DSL for workflow coordination
- Bead-based work item tracking
- Project state management
- Multi-provider AI routing
- API authentication and authorization
- Docker and Kubernetes deployment support
- Comprehensive documentation (20+ guides)

---

*Note: Version numbers follow semantic versioning (MAJOR.MINOR.PATCH)*
