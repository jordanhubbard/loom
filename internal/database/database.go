package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	internalmodels "github.com/jordanhubbard/arbiter/internal/models"
	"github.com/jordanhubbard/arbiter/pkg/models"
	_ "github.com/mattn/go-sqlite3"
)

// Database represents the arbiter database
type Database struct {
	db *sql.DB
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

	d := &Database{db: db}

	// Initialize schema
	if err := d.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return d, nil
}

// Close closes the database connection
func (d *Database) Close() error {
	return d.db.Close()
}

// initSchema creates the database tables
func (d *Database) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS config_kv (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS providers (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		endpoint TEXT NOT NULL,
		model TEXT,
		description TEXT,
		requires_key BOOLEAN NOT NULL DEFAULT 0,
		key_id TEXT,
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS projects (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		git_repo TEXT NOT NULL,
		branch TEXT NOT NULL,
		beads_path TEXT NOT NULL,
		is_perpetual BOOLEAN NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'open',
		context_json TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS agents (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		persona_name TEXT,
		provider_id TEXT,
		status TEXT NOT NULL DEFAULT 'idle',
		current_bead TEXT,
		project_id TEXT,
		started_at DATETIME NOT NULL,
		last_active DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
	CREATE INDEX IF NOT EXISTS idx_agents_project_id ON agents(project_id);
	CREATE INDEX IF NOT EXISTS idx_providers_status ON providers(status);
	`

	if _, err := d.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	// Best-effort migrations for existing databases.
	// SQLite doesn't support IF NOT EXISTS on ADD COLUMN.
	_, _ = d.db.Exec("ALTER TABLE providers ADD COLUMN model TEXT")
	_, _ = d.db.Exec("ALTER TABLE agents ADD COLUMN provider_id TEXT")

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
		INSERT INTO projects (id, name, git_repo, branch, beads_path, is_perpetual, status, context_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			git_repo = excluded.git_repo,
			branch = excluded.branch,
			beads_path = excluded.beads_path,
			is_perpetual = excluded.is_perpetual,
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
		SELECT id, name, git_repo, branch, beads_path, is_perpetual, status, context_json, created_at, updated_at
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
		err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.GitRepo,
			&p.Branch,
			&p.BeadsPath,
			&p.IsPerpetual,
			&status,
			&contextJSON,
			&p.CreatedAt,
			&p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
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
		INSERT INTO agents (id, name, persona_name, provider_id, status, current_bead, project_id, started_at, last_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			persona_name = excluded.persona_name,
			provider_id = excluded.provider_id,
			status = excluded.status,
			current_bead = excluded.current_bead,
			project_id = excluded.project_id,
			last_active = excluded.last_active
	`

	_, err := d.db.Exec(query,
		agent.ID,
		agent.Name,
		agent.PersonaName,
		agent.ProviderID,
		agent.Status,
		agent.CurrentBead,
		agent.ProjectID,
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
		SELECT id, name, persona_name, provider_id, status, current_bead, project_id, started_at, last_active
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
		err := rows.Scan(
			&a.ID,
			&a.Name,
			&a.PersonaName,
			&a.ProviderID,
			&a.Status,
			&a.CurrentBead,
			&a.ProjectID,
			&a.StartedAt,
			&a.LastActive,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
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
		INSERT INTO providers (id, name, type, endpoint, model, description, requires_key, key_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		INSERT INTO providers (id, name, type, endpoint, model, description, requires_key, key_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			type = excluded.type,
			endpoint = excluded.endpoint,
			model = excluded.model,
			description = excluded.description,
			requires_key = excluded.requires_key,
			key_id = excluded.key_id,
			status = excluded.status,
			updated_at = excluded.updated_at
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
		SELECT id, name, type, endpoint, model, description, requires_key, key_id, status, created_at, updated_at
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
		&provider.Description,
		&provider.RequiresKey,
		&provider.KeyID,
		&provider.Status,
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
		SELECT id, name, type, endpoint, model, description, requires_key, key_id, status, created_at, updated_at
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
		err := rows.Scan(
			&provider.ID,
			&provider.Name,
			&provider.Type,
			&provider.Endpoint,
			&provider.Model,
			&provider.Description,
			&provider.RequiresKey,
			&provider.KeyID,
			&provider.Status,
			&provider.CreatedAt,
			&provider.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan provider: %w", err)
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
