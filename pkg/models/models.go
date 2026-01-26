package models

import "time"

// Persona represents an agent's personality, capabilities, and behavioral guidelines
type Persona struct {
	EntityMetadata `json:",inline" yaml:",inline"`

	Name                 string   `json:"name" yaml:"name"`
	Character            string   `json:"character" yaml:"character"`
	Tone                 string   `json:"tone" yaml:"tone"`
	FocusAreas           []string `json:"focus_areas" yaml:"focus_areas"`
	AutonomyLevel        string   `json:"autonomy_level" yaml:"autonomy_level"` // "full", "semi", "supervised"
	Capabilities         []string `json:"capabilities" yaml:"capabilities"`
	DecisionMaking       string   `json:"decision_making" yaml:"decision_making"`
	Housekeeping         string   `json:"housekeeping" yaml:"housekeeping"`
	Collaboration        string   `json:"collaboration" yaml:"collaboration"`
	Standards            []string `json:"standards" yaml:"standards"`
	Mission              string   `json:"mission" yaml:"mission"`
	Personality          string   `json:"personality" yaml:"personality"`
	AutonomyInstructions string   `json:"autonomy_instructions" yaml:"autonomy_instructions"`
	DecisionInstructions string   `json:"decision_instructions" yaml:"decision_instructions"`
	PersistentTasks      string   `json:"persistent_tasks" yaml:"persistent_tasks"`

	// File paths
	PersonaFile      string `json:"persona_file" yaml:"persona_file"`
	InstructionsFile string `json:"instructions_file" yaml:"instructions_file"`

	// Metadata
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
}

// VersionedEntity interface implementation for Persona
func (p *Persona) GetEntityType() EntityType          { return EntityTypePersona }
func (p *Persona) GetSchemaVersion() SchemaVersion    { return p.EntityMetadata.SchemaVersion }
func (p *Persona) SetSchemaVersion(v SchemaVersion)   { p.EntityMetadata.SchemaVersion = v }
func (p *Persona) GetEntityMetadata() *EntityMetadata { return &p.EntityMetadata }
func (p *Persona) GetID() string                      { return p.Name }

// Agent represents a running agent instance with a specific persona
type Agent struct {
	EntityMetadata `json:",inline"`

	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Role        string    `json:"role,omitempty"`
	PersonaName string    `json:"persona_name"`
	Persona     *Persona  `json:"persona,omitempty"`
	ProviderID  string    `json:"provider_id,omitempty"`
	Status      string    `json:"status"` // "paused", "idle", "working", "deciding", "blocked"
	CurrentBead string    `json:"current_bead,omitempty"`
	ProjectID   string    `json:"project_id"`
	PositionID  string    `json:"position_id,omitempty"` // Link to org chart position
	StartedAt   time.Time `json:"started_at"`
	LastActive  time.Time `json:"last_active"`
}

// VersionedEntity interface implementation for Agent
func (a *Agent) GetEntityType() EntityType          { return EntityTypeAgent }
func (a *Agent) GetSchemaVersion() SchemaVersion    { return a.EntityMetadata.SchemaVersion }
func (a *Agent) SetSchemaVersion(v SchemaVersion)   { a.EntityMetadata.SchemaVersion = v }
func (a *Agent) GetEntityMetadata() *EntityMetadata { return &a.EntityMetadata }
func (a *Agent) GetID() string                      { return a.ID }

// ProjectStatus represents the current state of a project
type ProjectStatus string

const (
	ProjectStatusOpen     ProjectStatus = "open"
	ProjectStatusClosed   ProjectStatus = "closed"
	ProjectStatusReopened ProjectStatus = "reopened"
)

// ProjectComment represents a comment on a project's state
type ProjectComment struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	AuthorID  string    `json:"author_id"` // Agent ID or "user-{id}"
	Comment   string    `json:"comment"`
	Timestamp time.Time `json:"timestamp"`
}

// GitAuthMethod represents the authentication method for git operations
type GitAuthMethod string

const (
	GitAuthNone      GitAuthMethod = "none"       // Public repos, no auth
	GitAuthSSH       GitAuthMethod = "ssh"        // SSH key authentication
	GitAuthToken     GitAuthMethod = "token"      // HTTPS with token (GitHub PAT, etc.)
	GitAuthBasic     GitAuthMethod = "basic"      // HTTPS with username/password
	GitAuthGitHelper GitAuthMethod = "git-helper" // Use git credential helper
)

// ProjectMilestone represents a milestone within a project (embedded for simplicity)
type ProjectMilestone struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Type        string    `json:"type"`   // "release", "sprint_end", "quarterly_review", "annual_review", "custom"
	Status      string    `json:"status"` // "planned", "in_progress", "complete", "missed", "cancelled"
	DueDate     time.Time `json:"due_date"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// Project represents a project that agents work on
type Project struct {
	EntityMetadata `json:",inline"`

	ID          string            `json:"id"`
	Name        string            `json:"name"`
	GitRepo     string            `json:"git_repo"`
	Branch      string            `json:"branch"`
	BeadsPath   string            `json:"beads_path"`          // Path to .beads directory
	ParentID    string            `json:"parent_id,omitempty"` // For sub-projects
	Context     map[string]string `json:"context"`             // Additional context for agents
	Status      ProjectStatus     `json:"status"`              // Current project status
	IsPerpetual bool              `json:"is_perpetual"`        // If true, project never closes
	IsSticky    bool              `json:"is_sticky"`           // If true, project auto-added on startup
	Comments    []ProjectComment  `json:"comments"`            // Comments on project state
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ClosedAt    *time.Time        `json:"closed_at,omitempty"`
	Agents      []string          `json:"agents"` // Agent IDs working on this project

	// Deadline tracking (motivation system)
	DueDate    *time.Time          `json:"due_date,omitempty"` // Overall project deadline
	Milestones []ProjectMilestone  `json:"milestones,omitempty"` // Project milestones

	// Git management fields
	GitAuthMethod    GitAuthMethod     `json:"git_auth_method"`              // Authentication method
	GitCredentialID  string            `json:"git_credential_id,omitempty"`  // Reference to stored credential
	WorkDir          string            `json:"work_dir,omitempty"`           // Local path where repo is cloned
	LastSyncAt       *time.Time        `json:"last_sync_at,omitempty"`       // Last git pull/fetch
	LastCommitHash   string            `json:"last_commit_hash,omitempty"`   // Last known commit SHA
	GitConfigOptions map[string]string `json:"git_config_options,omitempty"` // Custom git config for this project
}

// VersionedEntity interface implementation for Project
func (p *Project) GetEntityType() EntityType          { return EntityTypeProject }
func (p *Project) GetSchemaVersion() SchemaVersion    { return p.EntityMetadata.SchemaVersion }
func (p *Project) SetSchemaVersion(v SchemaVersion)   { p.EntityMetadata.SchemaVersion = v }
func (p *Project) GetEntityMetadata() *EntityMetadata { return &p.EntityMetadata }
func (p *Project) GetID() string                      { return p.ID }

// BeadStatus represents the status of a bead
type BeadStatus string

const (
	BeadStatusOpen       BeadStatus = "open"
	BeadStatusInProgress BeadStatus = "in_progress"
	BeadStatusBlocked    BeadStatus = "blocked"
	BeadStatusClosed     BeadStatus = "closed"
)

// BeadPriority represents the priority of a bead
type BeadPriority int

const (
	BeadPriorityP0 BeadPriority = 0 // Critical - needs human
	BeadPriorityP1 BeadPriority = 1 // High
	BeadPriorityP2 BeadPriority = 2 // Medium
	BeadPriorityP3 BeadPriority = 3 // Low
)

// Bead represents a work item or decision point
type Bead struct {
	EntityMetadata `json:",inline"`

	ID          string            `json:"id"`
	Type        string            `json:"type"` // "task", "decision", "epic"
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      BeadStatus        `json:"status"`
	Priority    BeadPriority      `json:"priority"`
	ProjectID   string            `json:"project_id"`
	AssignedTo  string            `json:"assigned_to,omitempty"` // Agent ID
	BlockedBy   []string          `json:"blocked_by,omitempty"`  // Bead IDs
	Blocks      []string          `json:"blocks,omitempty"`      // Bead IDs
	RelatedTo   []string          `json:"related_to,omitempty"`  // Bead IDs
	Parent      string            `json:"parent,omitempty"`      // Parent bead ID
	Children    []string          `json:"children,omitempty"`    // Child bead IDs
	Tags        []string          `json:"tags,omitempty"`
	Context     map[string]string `json:"context,omitempty"`

	// Deadline tracking (motivation system)
	DueDate       *time.Time `json:"due_date,omitempty"`       // When this bead should be completed
	MilestoneID   string     `json:"milestone_id,omitempty"`   // Associated milestone
	EstimatedTime int        `json:"estimated_time,omitempty"` // Estimated minutes to complete

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
}

// VersionedEntity interface implementation for Bead
func (b *Bead) GetEntityType() EntityType          { return EntityTypeBead }
func (b *Bead) GetSchemaVersion() SchemaVersion    { return b.EntityMetadata.SchemaVersion }
func (b *Bead) SetSchemaVersion(v SchemaVersion)   { b.EntityMetadata.SchemaVersion = v }
func (b *Bead) GetEntityMetadata() *EntityMetadata { return &b.EntityMetadata }
func (b *Bead) GetID() string                      { return b.ID }

// DecisionBead represents a specific decision point that needs resolution
type DecisionBead struct {
	*Bead
	Question       string     `json:"question"`
	Options        []string   `json:"options,omitempty"`
	Recommendation string     `json:"recommendation,omitempty"`
	RequesterID    string     `json:"requester_id"` // Agent ID that filed the decision
	DeciderID      string     `json:"decider_id,omitempty"`
	Decision       string     `json:"decision,omitempty"`
	Rationale      string     `json:"rationale,omitempty"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
}

// FileLock represents a lock on a file to prevent merge conflicts
type FileLock struct {
	FilePath  string    `json:"file_path"`
	ProjectID string    `json:"project_id"`
	AgentID   string    `json:"agent_id"`
	BeadID    string    `json:"bead_id"`
	LockedAt  time.Time `json:"locked_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// WorkGraph represents the dependency graph of beads
type WorkGraph struct {
	Beads     map[string]*Bead `json:"beads"`
	Edges     []Edge           `json:"edges"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// Edge represents a directed edge in the work graph
type Edge struct {
	From         string `json:"from"`
	To           string `json:"to"`
	Relationship string `json:"relationship"` // "blocks", "parent", "related"
}

// AutonomyLevel defines agent decision-making authority
type AutonomyLevel string

const (
	AutonomyFull       AutonomyLevel = "full"       // Can make all non-P0 decisions
	AutonomySemi       AutonomyLevel = "semi"       // Can make routine decisions
	AutonomySupervised AutonomyLevel = "supervised" // Requires approval for all decisions
)
