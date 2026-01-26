package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	internalmodels "github.com/jordanhubbard/agenticorp/internal/models"
	"github.com/jordanhubbard/agenticorp/pkg/models"
	_ "github.com/mattn/go-sqlite3"
)

// Database represents the agenticorp database
type Database struct {
	db         *sql.DB
	dbType     string // "sqlite" or "postgres"
	supportsHA bool   // true if database supports HA features
}

// New creates a new database instance and initializes the schema
func New(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	d := &Database{
		db:         db,
		dbType:     "sqlite",
		supportsHA: false,
	}

	// Initialize schema
	if err := d.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Run migrations
	if err := d.migrateProviderOwnership(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate provider ownership: %w", err)
	}

	if err := d.migrateProviderRouting(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate provider routing: %w", err)
	}

	if err := d.migrateMotivations(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate motivations: %w", err)
	}

	return d, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// DB returns the underlying sql.DB instance
func (d *Database) DB() *sql.DB {
	return d.db
}

// Type returns the database type
func (d *Database) Type() string {
	return d.dbType
}

// SupportsHA returns whether the database supports HA features
func (d *Database) SupportsHA() bool {
	return d.supportsHA
}

// initSchema creates the database tables
func (d *Database) initSchema() error {
	schema := `
	-- Global configuration key-value store
	CREATE TABLE IF NOT EXISTS config_kv (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME NOT NULL
	);

	-- Global providers (shared across all projects)
	CREATE TABLE IF NOT EXISTS providers (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		endpoint TEXT NOT NULL,
		model TEXT,
		configured_model TEXT,
		selected_model TEXT,
		selection_reason TEXT,
		model_score REAL,
		selected_gpu TEXT,
		gpu_constraints_json TEXT,
		description TEXT,
		requires_key BOOLEAN NOT NULL DEFAULT 0,
		key_id TEXT,
		owner_id TEXT,
		is_shared BOOLEAN NOT NULL DEFAULT 1,
		status TEXT NOT NULL DEFAULT 'active',
		last_heartbeat_at DATETIME,
		last_heartbeat_latency_ms INTEGER,
		last_heartbeat_error TEXT,
		metrics_json TEXT,
		schema_version TEXT NOT NULL DEFAULT '1.0',
		attributes_json TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	-- Projects with hierarchy support (parent_id for sub-projects)
	CREATE TABLE IF NOT EXISTS projects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		git_repo TEXT NOT NULL,
		branch TEXT NOT NULL,
		beads_path TEXT NOT NULL,
		parent_id TEXT,
		is_perpetual BOOLEAN NOT NULL DEFAULT 0,
		is_sticky BOOLEAN NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'open',
		context_json TEXT,
		schema_version TEXT NOT NULL DEFAULT '1.0',
		attributes_json TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		closed_at DATETIME,
		FOREIGN KEY (parent_id) REFERENCES projects(id) ON DELETE SET NULL
	);

	-- Org charts define the team structure for each project
	CREATE TABLE IF NOT EXISTS org_charts (
		id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		name TEXT NOT NULL,
		is_template BOOLEAN NOT NULL DEFAULT 0,
		parent_id TEXT,
		schema_version TEXT NOT NULL DEFAULT '1.0',
		attributes_json TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
		FOREIGN KEY (parent_id) REFERENCES org_charts(id) ON DELETE SET NULL
	);

	-- Positions within an org chart (role slots)
	CREATE TABLE IF NOT EXISTS org_chart_positions (
		id TEXT PRIMARY KEY,
		org_chart_id TEXT NOT NULL,
		role_name TEXT NOT NULL,
		persona_path TEXT NOT NULL,
		required BOOLEAN NOT NULL DEFAULT 0,
		max_instances INTEGER NOT NULL DEFAULT 0,
		reports_to TEXT,
		schema_version TEXT NOT NULL DEFAULT '1.0',
		attributes_json TEXT,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (org_chart_id) REFERENCES org_charts(id) ON DELETE CASCADE,
		FOREIGN KEY (reports_to) REFERENCES org_chart_positions(id) ON DELETE SET NULL
	);

	-- Agent instances assigned to positions
	CREATE TABLE IF NOT EXISTS agents (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		role TEXT,
		persona_name TEXT,
		provider_id TEXT,
		status TEXT NOT NULL DEFAULT 'idle',
		current_bead TEXT,
		project_id TEXT,
		position_id TEXT,
		schema_version TEXT NOT NULL DEFAULT '1.0',
		attributes_json TEXT,
		started_at DATETIME NOT NULL,
		last_active DATETIME NOT NULL,
		FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL,
		FOREIGN KEY (position_id) REFERENCES org_chart_positions(id) ON DELETE SET NULL,
		FOREIGN KEY (provider_id) REFERENCES providers(id) ON DELETE SET NULL
	);

	-- Indexes for performance
	CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
	CREATE INDEX IF NOT EXISTS idx_agents_project_id ON agents(project_id);
	CREATE INDEX IF NOT EXISTS idx_agents_position_id ON agents(position_id);
	CREATE INDEX IF NOT EXISTS idx_providers_status ON providers(status);
	CREATE INDEX IF NOT EXISTS idx_projects_parent_id ON projects(parent_id);
	CREATE INDEX IF NOT EXISTS idx_org_charts_project_id ON org_charts(project_id);
	CREATE INDEX IF NOT EXISTS idx_positions_org_chart_id ON org_chart_positions(org_chart_id);

	-- Command Logs for agent shell command execution
	CREATE TABLE IF NOT EXISTS command_logs (
		id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		bead_id TEXT,
		project_id TEXT,
		command TEXT NOT NULL,
		working_dir TEXT NOT NULL,
		exit_code INTEGER NOT NULL,
		stdout TEXT,
		stderr TEXT,
		duration_ms INTEGER NOT NULL,
		started_at DATETIME NOT NULL,
		completed_at DATETIME NOT NULL,
		context TEXT,
		created_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_command_logs_agent_id ON command_logs(agent_id);
	CREATE INDEX IF NOT EXISTS idx_command_logs_bead_id ON command_logs(bead_id);
	CREATE INDEX IF NOT EXISTS idx_command_logs_project_id ON command_logs(project_id);
	CREATE INDEX IF NOT EXISTS idx_command_logs_created_at ON command_logs(created_at);
	`

	if _, err := d.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Best-effort migrations for existing databases.
	// SQLite doesn't support IF NOT EXISTS on ADD COLUMN.

	// Provider migrations
	_, _ = d.db.Exec("ALTER TABLE providers ADD COLUMN model TEXT")
	_, _ = d.db.Exec("ALTER TABLE providers ADD COLUMN configured_model TEXT")
	_, _ = d.db.Exec("ALTER TABLE providers ADD COLUMN selected_model TEXT")
	_, _ = d.db.Exec("ALTER TABLE providers ADD COLUMN selection_reason TEXT")
	_, _ = d.db.Exec("ALTER TABLE providers ADD COLUMN model_score REAL")
	_, _ = d.db.Exec("ALTER TABLE providers ADD COLUMN selected_gpu TEXT")
	_, _ = d.db.Exec("ALTER TABLE providers ADD COLUMN last_heartbeat_at DATETIME")
	_, _ = d.db.Exec("ALTER TABLE providers ADD COLUMN last_heartbeat_latency_ms INTEGER")
	_, _ = d.db.Exec("ALTER TABLE providers ADD COLUMN last_heartbeat_error TEXT")
	_, _ = d.db.Exec("ALTER TABLE providers ADD COLUMN schema_version TEXT DEFAULT '1.0'")
	_, _ = d.db.Exec("ALTER TABLE providers ADD COLUMN attributes_json TEXT")
	_, _ = d.db.Exec("UPDATE providers SET schema_version = '1.0' WHERE schema_version IS NULL")

	// Project migrations
	_, _ = d.db.Exec("ALTER TABLE projects ADD COLUMN is_sticky BOOLEAN")
	_, _ = d.db.Exec("UPDATE projects SET is_sticky = 0 WHERE is_sticky IS NULL")
	_, _ = d.db.Exec("ALTER TABLE projects ADD COLUMN parent_id TEXT")
	_, _ = d.db.Exec("ALTER TABLE projects ADD COLUMN closed_at DATETIME")
	_, _ = d.db.Exec("ALTER TABLE projects ADD COLUMN schema_version TEXT DEFAULT '1.0'")
	_, _ = d.db.Exec("ALTER TABLE projects ADD COLUMN attributes_json TEXT")
	_, _ = d.db.Exec("UPDATE projects SET schema_version = '1.0' WHERE schema_version IS NULL")

	// Agent migrations
	_, _ = d.db.Exec("ALTER TABLE agents ADD COLUMN provider_id TEXT")
	_, _ = d.db.Exec("ALTER TABLE agents ADD COLUMN role TEXT")
	_, _ = d.db.Exec("ALTER TABLE agents ADD COLUMN position_id TEXT")
	_, _ = d.db.Exec("ALTER TABLE agents ADD COLUMN schema_version TEXT DEFAULT '1.0'")
	_, _ = d.db.Exec("ALTER TABLE agents ADD COLUMN attributes_json TEXT")
	_, _ = d.db.Exec("UPDATE agents SET schema_version = '1.0' WHERE schema_version IS NULL")

	// Org chart migrations
	_, _ = d.db.Exec("ALTER TABLE org_charts ADD COLUMN schema_version TEXT DEFAULT '1.0'")
	_, _ = d.db.Exec("ALTER TABLE org_charts ADD COLUMN attributes_json TEXT")
	_, _ = d.db.Exec("UPDATE org_charts SET schema_version = '1.0' WHERE schema_version IS NULL")

	// Position migrations
	_, _ = d.db.Exec("ALTER TABLE org_chart_positions ADD COLUMN schema_version TEXT DEFAULT '1.0'")
	_, _ = d.db.Exec("ALTER TABLE org_chart_positions ADD COLUMN attributes_json TEXT")
	_, _ = d.db.Exec("UPDATE org_chart_positions SET schema_version = '1.0' WHERE schema_version IS NULL")

	return nil
}

// Configuration KV

func (d *Database) SetConfigValue(key string, value string) error {
	query := `
		INSERT INTO config_kv (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`
	_, err := d.db.Exec(query, key, value, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set config value: %w", err)
	}
	return nil
}

func (d *Database) GetConfigValue(key string) (string, bool, error) {
	query := `SELECT value FROM config_kv WHERE key = ?`
	var value string
	err := d.db.QueryRow(query, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to get config value: %w", err)
	}
	return value, true, nil
}

// Projects

func (d *Database) UpsertProject(project *models.Project) error {
	if project == nil {
		return fmt.Errorf("project cannot be nil")
	}

	contextJSON := ""
	if project.Context != nil {
		b, err := json.Marshal(project.Context)
		if err != nil {
			return fmt.Errorf("failed to marshal project context: %w", err)
		}
		contextJSON = string(b)
	}

	if project.CreatedAt.IsZero() {
		project.CreatedAt = time.Now()
	}
	project.UpdatedAt = time.Now()

	query := `
		INSERT INTO projects (id, name, git_repo, branch, beads_path, is_perpetual, is_sticky, status, context_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			git_repo = excluded.git_repo,
			branch = excluded.branch,
			beads_path = excluded.beads_path,
			is_perpetual = excluded.is_perpetual,
			is_sticky = excluded.is_sticky,
			status = excluded.status,
			context_json = excluded.context_json,
			updated_at = excluded.updated_at
	`

	_, err := d.db.Exec(query,
		project.ID,
		project.Name,
		project.GitRepo,
		project.Branch,
		project.BeadsPath,
		project.IsPerpetual,
		project.IsSticky,
		string(project.Status),
		contextJSON,
		project.CreatedAt,
		project.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert project: %w", err)
	}

	return nil
}

func (d *Database) ListProjects() ([]*models.Project, error) {
	query := `
		SELECT id, name, git_repo, branch, beads_path, is_perpetual, is_sticky, status, context_json, created_at, updated_at
		FROM projects
		ORDER BY created_at DESC
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer rows.Close()

	var projects []*models.Project
	for rows.Next() {
		p := &models.Project{}
		var status string
		var contextJSON sql.NullString
		var isSticky sql.NullBool
		err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.GitRepo,
			&p.Branch,
			&p.BeadsPath,
			&p.IsPerpetual,
			&isSticky,
			&status,
			&contextJSON,
			&p.CreatedAt,
			&p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}
		if isSticky.Valid {
			p.IsSticky = isSticky.Bool
		}
		p.Status = models.ProjectStatus(status)
		if contextJSON.Valid && contextJSON.String != "" {
			_ = json.Unmarshal([]byte(contextJSON.String), &p.Context)
		}
		if p.Context == nil {
			p.Context = map[string]string{}
		}
		p.Agents = []string{}
		p.Comments = []models.ProjectComment{}
		projects = append(projects, p)
	}

	return projects, nil
}

func (d *Database) DeleteProject(id string) error {
	query := `DELETE FROM projects WHERE id = ?`
	result, err := d.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("project not found: %s", id)
	}
	return nil
}

// Agents

func (d *Database) UpsertAgent(agent *models.Agent) error {
	if agent == nil {
		return fmt.Errorf("agent cannot be nil")
	}
	if agent.StartedAt.IsZero() {
		agent.StartedAt = time.Now()
	}
	if agent.LastActive.IsZero() {
		agent.LastActive = time.Now()
	}

	query := `
		INSERT INTO agents (id, name, role, persona_name, provider_id, status, current_bead, project_id, started_at, last_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			role = excluded.role,
			persona_name = excluded.persona_name,
			provider_id = excluded.provider_id,
			status = excluded.status,
			current_bead = excluded.current_bead,
			project_id = excluded.project_id,
			last_active = excluded.last_active
	`

	// Convert empty strings to nil for SQL NULL (for FK constraints)
	var providerID, currentBead, projectID interface{}
	if agent.ProviderID != "" {
		providerID = agent.ProviderID
	}
	if agent.CurrentBead != "" {
		currentBead = agent.CurrentBead
	}
	if agent.ProjectID != "" {
		projectID = agent.ProjectID
	}

	_, err := d.db.Exec(query,
		agent.ID,
		agent.Name,
		agent.Role,
		agent.PersonaName,
		providerID,
		agent.Status,
		currentBead,
		projectID,
		agent.StartedAt,
		agent.LastActive,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert agent: %w", err)
	}
	return nil
}

func (d *Database) ListAgents() ([]*models.Agent, error) {
	query := `
		SELECT id, name, role, persona_name, provider_id, status, current_bead, project_id, started_at, last_active
		FROM agents
		ORDER BY started_at DESC
	`
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}
	defer rows.Close()

	var agents []*models.Agent
	for rows.Next() {
		a := &models.Agent{}
		var providerID, currentBead, projectID sql.NullString
		err := rows.Scan(
			&a.ID,
			&a.Name,
			&a.Role,
			&a.PersonaName,
			&providerID,
			&a.Status,
			&currentBead,
			&projectID,
			&a.StartedAt,
			&a.LastActive,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}
		// Convert sql.NullString to regular strings
		if providerID.Valid {
			a.ProviderID = providerID.String
		}
		if currentBead.Valid {
			a.CurrentBead = currentBead.String
		}
		if projectID.Valid {
			a.ProjectID = projectID.String
		}
		agents = append(agents, a)
	}
	return agents, nil
}

func (d *Database) DeleteAgent(id string) error {
	query := `DELETE FROM agents WHERE id = ?`
	result, err := d.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete agent: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("agent not found: %s", id)
	}
	return nil
}

// CreateProvider creates a new provider
func (d *Database) CreateProvider(provider *internalmodels.Provider) error {
	provider.CreatedAt = time.Now()
	provider.UpdatedAt = time.Now()

	query := `
		INSERT INTO providers (id, name, type, endpoint, model, description, requires_key, key_id, status, last_heartbeat_at, last_heartbeat_latency_ms, last_heartbeat_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := d.db.Exec(query,
		provider.ID,
		provider.Name,
		provider.Type,
		provider.Endpoint,
		provider.Model,
		provider.Description,
		provider.RequiresKey,
		provider.KeyID,
		provider.Status,
		provider.LastHeartbeatAt,
		provider.LastHeartbeatLatencyMs,
		provider.LastHeartbeatError,
		provider.CreatedAt,
		provider.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	return nil
}

// UpsertProvider inserts or updates a provider.
func (d *Database) UpsertProvider(provider *internalmodels.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider cannot be nil")
	}
	if provider.CreatedAt.IsZero() {
		provider.CreatedAt = time.Now()
	}
	provider.UpdatedAt = time.Now()

	query := `
		INSERT INTO providers (id, name, type, endpoint, model, configured_model, selected_model, selection_reason, model_score, selected_gpu, description, requires_key, key_id, owner_id, is_shared, status, last_heartbeat_at, last_heartbeat_latency_ms, last_heartbeat_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			type = excluded.type,
			endpoint = excluded.endpoint,
			model = excluded.model,
			configured_model = excluded.configured_model,
			selected_model = excluded.selected_model,
			selection_reason = excluded.selection_reason,
			model_score = excluded.model_score,
			selected_gpu = excluded.selected_gpu,
			description = excluded.description,
			requires_key = excluded.requires_key,
			key_id = excluded.key_id,
			owner_id = excluded.owner_id,
			is_shared = excluded.is_shared,
			status = excluded.status,
			last_heartbeat_at = excluded.last_heartbeat_at,
			last_heartbeat_latency_ms = excluded.last_heartbeat_latency_ms,
			last_heartbeat_error = excluded.last_heartbeat_error,
			updated_at = excluded.updated_at
	`

	_, err := d.db.Exec(query,
		provider.ID,
		provider.Name,
		provider.Type,
		provider.Endpoint,
		provider.Model,
		provider.ConfiguredModel,
		provider.SelectedModel,
		provider.SelectionReason,
		provider.ModelScore,
		provider.SelectedGPU,
		provider.Description,
		provider.RequiresKey,
		provider.KeyID,
		provider.OwnerID,
		provider.IsShared,
		provider.Status,
		provider.LastHeartbeatAt,
		provider.LastHeartbeatLatencyMs,
		provider.LastHeartbeatError,
		provider.CreatedAt,
		provider.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert provider: %w", err)
	}

	return nil
}

func (d *Database) DeleteAllProviders() error {
	_, err := d.db.Exec(`DELETE FROM providers`)
	if err != nil {
		return fmt.Errorf("failed to delete all providers: %w", err)
	}
	return nil
}

func (d *Database) DeleteAllProjects() error {
	_, err := d.db.Exec(`DELETE FROM projects`)
	if err != nil {
		return fmt.Errorf("failed to delete all projects: %w", err)
	}
	return nil
}

func (d *Database) DeleteAllAgents() error {
	_, err := d.db.Exec(`DELETE FROM agents`)
	if err != nil {
		return fmt.Errorf("failed to delete all agents: %w", err)
	}
	return nil
}

// GetProvider retrieves a provider by ID
func (d *Database) GetProvider(id string) (*internalmodels.Provider, error) {
	query := `
		SELECT id, name, type, endpoint, model, configured_model, selected_model, selection_reason, model_score, selected_gpu, description, requires_key, key_id, status, last_heartbeat_at, last_heartbeat_latency_ms, last_heartbeat_error, created_at, updated_at
		FROM providers
		WHERE id = ?
	`

	provider := &internalmodels.Provider{}
	err := d.db.QueryRow(query, id).Scan(
		&provider.ID,
		&provider.Name,
		&provider.Type,
		&provider.Endpoint,
		&provider.Model,
		&provider.ConfiguredModel,
		&provider.SelectedModel,
		&provider.SelectionReason,
		&provider.ModelScore,
		&provider.SelectedGPU,
		&provider.Description,
		&provider.RequiresKey,
		&provider.KeyID,
		&provider.Status,
		&provider.LastHeartbeatAt,
		&provider.LastHeartbeatLatencyMs,
		&provider.LastHeartbeatError,
		&provider.CreatedAt,
		&provider.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("provider not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	return provider, nil
}

// ListProviders retrieves all providers
func (d *Database) ListProviders() ([]*internalmodels.Provider, error) {
	query := `
		SELECT id, name, type, endpoint, model, configured_model, selected_model, selection_reason, model_score, selected_gpu, description, requires_key, key_id, owner_id, is_shared, status, last_heartbeat_at, last_heartbeat_latency_ms, last_heartbeat_error, created_at, updated_at
		FROM providers
		ORDER BY created_at DESC
	`

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}
	defer rows.Close()

	var providers []*internalmodels.Provider
	for rows.Next() {
		provider := &internalmodels.Provider{}
		var ownerID sql.NullString
		var isShared sql.NullBool
		err := rows.Scan(
			&provider.ID,
			&provider.Name,
			&provider.Type,
			&provider.Endpoint,
			&provider.Model,
			&provider.ConfiguredModel,
			&provider.SelectedModel,
			&provider.SelectionReason,
			&provider.ModelScore,
			&provider.SelectedGPU,
			&provider.Description,
			&provider.RequiresKey,
			&provider.KeyID,
			&ownerID,
			&isShared,
			&provider.Status,
			&provider.LastHeartbeatAt,
			&provider.LastHeartbeatLatencyMs,
			&provider.LastHeartbeatError,
			&provider.CreatedAt,
			&provider.UpdatedAt,
		)
		if ownerID.Valid {
			provider.OwnerID = ownerID.String
		}
		if isShared.Valid {
			provider.IsShared = isShared.Bool
		} else {
			provider.IsShared = true // Default to shared for backwards compat
		}
		if err != nil {
			return nil, fmt.Errorf("failed to scan provider: %w", err)
		}
		providers = append(providers, provider)
	}

	return providers, nil
}

// ListProvidersForUser retrieves providers accessible to a specific user
// Returns providers owned by the user OR shared providers
func (d *Database) ListProvidersForUser(userID string) ([]*internalmodels.Provider, error) {
	query := `
		SELECT id, name, type, endpoint, model, configured_model, selected_model, selection_reason, model_score, selected_gpu, description, requires_key, key_id, owner_id, is_shared, status, last_heartbeat_at, last_heartbeat_latency_ms, last_heartbeat_error, created_at, updated_at
		FROM providers
		WHERE owner_id = ? OR is_shared = 1 OR owner_id IS NULL
		ORDER BY created_at DESC
	`

	rows, err := d.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list providers for user: %w", err)
	}
	defer rows.Close()

	var providers []*internalmodels.Provider
	for rows.Next() {
		provider := &internalmodels.Provider{}
		var ownerID sql.NullString
		var isShared sql.NullBool
		err := rows.Scan(
			&provider.ID,
			&provider.Name,
			&provider.Type,
			&provider.Endpoint,
			&provider.Model,
			&provider.ConfiguredModel,
			&provider.SelectedModel,
			&provider.SelectionReason,
			&provider.ModelScore,
			&provider.SelectedGPU,
			&provider.Description,
			&provider.RequiresKey,
			&provider.KeyID,
			&ownerID,
			&isShared,
			&provider.Status,
			&provider.LastHeartbeatAt,
			&provider.LastHeartbeatLatencyMs,
			&provider.LastHeartbeatError,
			&provider.CreatedAt,
			&provider.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan provider: %w", err)
		}

		if ownerID.Valid {
			provider.OwnerID = ownerID.String
		}
		if isShared.Valid {
			provider.IsShared = isShared.Bool
		} else {
			provider.IsShared = true
		}

		providers = append(providers, provider)
	}

	return providers, nil
}

// UpdateProvider updates a provider
func (d *Database) UpdateProvider(provider *internalmodels.Provider) error {
	provider.UpdatedAt = time.Now()

	query := `
		UPDATE providers
		SET name = ?, type = ?, endpoint = ?, model = ?, description = ?, requires_key = ?, key_id = ?, status = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := d.db.Exec(query,
		provider.Name,
		provider.Type,
		provider.Endpoint,
		provider.Model,
		provider.Description,
		provider.RequiresKey,
		provider.KeyID,
		provider.Status,
		provider.UpdatedAt,
		provider.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update provider: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("provider not found: %s", provider.ID)
	}

	return nil
}

// DeleteProvider deletes a provider
func (d *Database) DeleteProvider(id string) error {
	query := `DELETE FROM providers WHERE id = ?`

	result, err := d.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("provider not found: %s", id)
	}

	return nil
}
