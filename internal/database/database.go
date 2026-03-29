package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	internalmodels "github.com/jordanhubbard/loom/internal/models"
	"github.com/jordanhubbard/loom/pkg/models"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// Database represents the loom database (PostgreSQL only)
type Database struct {
	db         *sql.DB
	supportsHA bool
}

// New creates a new database instance and initializes the schema
// NewFromEnv creates a PostgreSQL database instance from environment variables.
func NewFromEnv() (*Database, error) {
	return NewPostgreSQL()
}

// NewPostgreSQL creates a PostgreSQL database instance from environment variables
func NewPostgreSQL() (*Database, error) {
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		host = "localhost"
	}

	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "5432"
	}

	user := os.Getenv("POSTGRES_USER")
	if user == "" {
		user = "loom"
	}

	password := os.Getenv("POSTGRES_PASSWORD")
	if password == "" {
		password = "loom"
	}

	dbname := os.Getenv("POSTGRES_DB")
	if dbname == "" {
		dbname = "loom"
	}

	sslmode := os.Getenv("POSTGRES_SSLMODE")
	if sslmode == "" {
		sslmode = "disable"
	}

	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL database: %w", err)
	}

	defer func() {
		if err != nil {
			db.Close()
		}
	}()

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(20)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	d := &Database{
		db:         db,
		supportsHA: true,
	}

	// Initialize schema
	if err := d.initSchemaPostgres(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to initialize PostgreSQL schema: %w", err)
	}

	// Run migrations
	if err := d.migrateProviderOwnership(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate provider ownership: %w", err)
	}

	if err := d.migrateProviderRouting(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate provider routing: %w", err)
	}

	if err := d.migrateProviderScoring(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate provider scoring: %w", err)
	}

	if err := d.migrateMotivations(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate motivations: %w", err)
	}

	if err := d.migrateWorkflows(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate workflows: %w", err)
	}

	if err := d.migrateActivity(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate activity: %w", err)
	}

	if err := d.migrateComments(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate comments: %w", err)
	}

	if err := d.migrateConversations(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate conversations: %w", err)
	}

	if err := migratePatterns(d.db); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate patterns: %w", err)
	}

	if err := d.migrateCredentials(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate credentials: %w", err)
	}

	if err := d.migrateLessons(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate lessons: %w", err)
	}

	if err := d.migrateRequestLogs(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate request logs: %w", err)
	}

	if err := d.migrateProviderAPIKey(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate provider api key: %w", err)
	}

	if err := d.migrateProjectMemory(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate project memory: %w", err)
	}

	if err := d.migrateEphemeralState(); err != nil {
		defer db.Close()
		return nil, fmt.Errorf("failed to migrate ephemeral state: %w", err)
	}

	return d, nil
}

// migrateRequestLogs adds columns to request_logs that the analytics package expects.
func (d *Database) migrateRequestLogs() error {
	_, err := d.db.Exec(`ALTER TABLE request_logs ADD COLUMN IF NOT EXISTS created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP`)
	if err != nil {
		return fmt.Errorf("migrateRequestLogs: %w", err)
	}
	return nil
}

// Ping tests the database connection
func (d *Database) Ping() error {
	return d.db.Ping()
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
	return "postgres"
}

// SupportsHA returns whether the database supports HA features
func (d *Database) SupportsHA() bool {
	return d.supportsHA
}

// Configuration KV

func (d *Database) SetConfigValue(key string, value string) error {
	query := `
		INSERT INTO config_kv (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`
	_, err := d.db.Exec(rebind(query), key, value, time.Now())
	if err != nil {
		return fmt.Errorf("failed to set config value: %w", err)
	}
	return nil
}

func (d *Database) GetConfigValue(key string) (string, bool, error) {
	query := `SELECT value FROM config_kv WHERE key = ?`
	var value string
	err := d.db.QueryRow(rebind(query), key).Scan(&value)
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

	gitStrategy := string(project.GitStrategy)
	if gitStrategy == "" {
		gitStrategy = "direct"
	}

	gitAuthMethod := string(project.GitAuthMethod)
	if gitAuthMethod == "" {
		gitAuthMethod = "none"
	}

	query := `
		INSERT INTO projects (id, name, git_repo, branch, beads_path, git_strategy, git_auth_method, is_perpetual, is_sticky, status, context_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			git_repo = excluded.git_repo,
			branch = excluded.branch,
			beads_path = excluded.beads_path,
			git_strategy = excluded.git_strategy,
			git_auth_method = excluded.git_auth_method,
			is_perpetual = excluded.is_perpetual,
			is_sticky = excluded.is_sticky,
			status = excluded.status,
			context_json = excluded.context_json,
			updated_at = excluded.updated_at
	`

	_, err := d.db.Exec(rebind(query),
		project.ID,
		project.Name,
		project.GitRepo,
		project.Branch,
		project.BeadsPath,
		gitStrategy,
		gitAuthMethod,
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
		SELECT id, name, git_repo, branch, beads_path, git_strategy, git_auth_method, is_perpetual, is_sticky, status, context_json, created_at, updated_at
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
		var gitStrategy sql.NullString
		var gitAuthMethod sql.NullString
		var contextJSON sql.NullString
		var isSticky sql.NullBool
		err := rows.Scan(
			&p.ID,
			&p.Name,
			&p.GitRepo,
			&p.Branch,
			&p.BeadsPath,
			&gitStrategy,
			&gitAuthMethod,
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
		if gitStrategy.Valid && gitStrategy.String != "" {
			p.GitStrategy = models.GitStrategy(gitStrategy.String)
		} else {
			p.GitStrategy = models.GitStrategyDirect
		}
		if gitAuthMethod.Valid && gitAuthMethod.String != "" {
			p.GitAuthMethod = models.GitAuthMethod(gitAuthMethod.String)
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
	result, err := d.db.Exec(rebind(query), id)
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

	_, err := d.db.Exec(rebind(query),
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
	result, err := d.db.Exec(rebind(query), id)
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

	_, err := d.db.Exec(rebind(query),
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
		INSERT INTO providers (id, name, type, endpoint, model, configured_model, selected_model, description, requires_key, key_id, api_key, owner_id, is_shared, status, last_heartbeat_at, last_heartbeat_latency_ms, last_heartbeat_error, context_window, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			type = excluded.type,
			endpoint = excluded.endpoint,
			model = excluded.model,
			configured_model = excluded.configured_model,
			selected_model = excluded.selected_model,
			description = excluded.description,
			requires_key = excluded.requires_key,
			key_id = excluded.key_id,
			api_key = CASE WHEN excluded.api_key != '' THEN excluded.api_key ELSE providers.api_key END,
			owner_id = excluded.owner_id,
			is_shared = excluded.is_shared,
			status = excluded.status,
			last_heartbeat_at = excluded.last_heartbeat_at,
			last_heartbeat_latency_ms = excluded.last_heartbeat_latency_ms,
			last_heartbeat_error = excluded.last_heartbeat_error,
			context_window = excluded.context_window,
			updated_at = excluded.updated_at
	`

	_, err := d.db.Exec(rebind(query),
		provider.ID,
		provider.Name,
		provider.Type,
		provider.Endpoint,
		provider.Model,
		provider.ConfiguredModel,
		provider.SelectedModel,
		provider.Description,
		provider.RequiresKey,
		provider.KeyID,
		provider.APIKey,
		provider.OwnerID,
		provider.IsShared,
		provider.Status,
		provider.LastHeartbeatAt,
		provider.LastHeartbeatLatencyMs,
		provider.LastHeartbeatError,
		provider.ContextWindow,
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
		SELECT id, name, type, endpoint, model, configured_model, selected_model, description, requires_key, key_id, COALESCE(api_key, '') as api_key, owner_id, is_shared, status, last_heartbeat_at, last_heartbeat_latency_ms, last_heartbeat_error, context_window, created_at, updated_at
		FROM providers
		WHERE id = ?
	`

	provider := &internalmodels.Provider{}
	err := d.db.QueryRow(rebind(query), id).Scan(
		&provider.ID,
		&provider.Name,
		&provider.Type,
		&provider.Endpoint,
		&provider.Model,
		&provider.ConfiguredModel,
		&provider.SelectedModel,
		&provider.Description,
		&provider.RequiresKey,
		&provider.KeyID,
		&provider.APIKey,
		&provider.OwnerID,
		&provider.IsShared,
		&provider.Status,
		&provider.LastHeartbeatAt,
		&provider.LastHeartbeatLatencyMs,
		&provider.LastHeartbeatError,
		&provider.ContextWindow,
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
		SELECT id, name, type, endpoint, model, configured_model, selected_model, description, requires_key, key_id, COALESCE(api_key, '') as api_key, owner_id, is_shared, status, last_heartbeat_at, last_heartbeat_latency_ms, last_heartbeat_error, context_window, created_at, updated_at
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
		var (
			model, configuredModel, selectedModel sql.NullString
			description, keyID, lastHBError       sql.NullString
			apiKey                                sql.NullString
			ownerID                               sql.NullString
			isShared                              sql.NullBool
			contextWindow                         sql.NullInt64
			lastHBAt                              sql.NullTime
			lastHBLatencyMs                       sql.NullInt64
		)
		err := rows.Scan(
			&provider.ID,
			&provider.Name,
			&provider.Type,
			&provider.Endpoint,
			&model,
			&configuredModel,
			&selectedModel,
			&description,
			&provider.RequiresKey,
			&keyID,
			&apiKey,
			&ownerID,
			&isShared,
			&provider.Status,
			&lastHBAt,
			&lastHBLatencyMs,
			&lastHBError,
			&contextWindow,
			&provider.CreatedAt,
			&provider.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan provider: %w", err)
		}
		provider.Model = model.String
		provider.ConfiguredModel = configuredModel.String
		provider.SelectedModel = selectedModel.String
		provider.Description = description.String
		provider.KeyID = keyID.String
		provider.APIKey = apiKey.String
		provider.LastHeartbeatError = lastHBError.String
		if ownerID.Valid {
			provider.OwnerID = ownerID.String
		}
		if isShared.Valid {
			provider.IsShared = isShared.Bool
		} else {
			provider.IsShared = true
		}
		if contextWindow.Valid {
			provider.ContextWindow = int(contextWindow.Int64)
		}
		if lastHBAt.Valid {
			provider.LastHeartbeatAt = lastHBAt.Time
		}
		if lastHBLatencyMs.Valid {
			provider.LastHeartbeatLatencyMs = lastHBLatencyMs.Int64
		}
		providers = append(providers, provider)
	}

	return providers, nil
}

// ListProvidersForUser retrieves providers accessible to a specific user
// Returns providers owned by the user OR shared providers
func (d *Database) ListProvidersForUser(userID string) ([]*internalmodels.Provider, error) {
	query := `
		SELECT id, name, type, endpoint, model, configured_model, selected_model, description, requires_key, key_id, owner_id, is_shared, status, last_heartbeat_at, last_heartbeat_latency_ms, last_heartbeat_error, created_at, updated_at
		FROM providers
		WHERE owner_id = ? OR is_shared = true OR owner_id IS NULL
		ORDER BY created_at DESC
	`

	rows, err := d.db.Query(rebind(query), userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list providers for user: %w", err)
	}
	defer rows.Close()

	var providers []*internalmodels.Provider
	for rows.Next() {
		provider := &internalmodels.Provider{}
		var (
			model, configuredModel, selectedModel sql.NullString
			description, keyID, lastHBError       sql.NullString
			ownerID                               sql.NullString
			isShared                              sql.NullBool
			lastHBAt                              sql.NullTime
			lastHBLatencyMs                       sql.NullInt64
		)
		err := rows.Scan(
			&provider.ID,
			&provider.Name,
			&provider.Type,
			&provider.Endpoint,
			&model,
			&configuredModel,
			&selectedModel,
			&description,
			&provider.RequiresKey,
			&keyID,
			&ownerID,
			&isShared,
			&provider.Status,
			&lastHBAt,
			&lastHBLatencyMs,
			&lastHBError,
			&provider.CreatedAt,
			&provider.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan provider: %w", err)
		}
		provider.Model = model.String
		provider.ConfiguredModel = configuredModel.String
		provider.SelectedModel = selectedModel.String
		provider.Description = description.String
		provider.KeyID = keyID.String
		provider.LastHeartbeatError = lastHBError.String
		if ownerID.Valid {
			provider.OwnerID = ownerID.String
		}
		if isShared.Valid {
			provider.IsShared = isShared.Bool
		} else {
			provider.IsShared = true
		}
		if lastHBAt.Valid {
			provider.LastHeartbeatAt = lastHBAt.Time
		}
		if lastHBLatencyMs.Valid {
			provider.LastHeartbeatLatencyMs = lastHBLatencyMs.Int64
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

	result, err := d.db.Exec(rebind(query),
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

	result, err := d.db.Exec(rebind(query), id)
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
