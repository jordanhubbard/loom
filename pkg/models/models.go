package models

import "time"

// Persona represents an agent's personality, capabilities, and behavioral guidelines
type Persona struct {
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

// Agent represents a running agent instance with a specific persona
type Agent struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	PersonaName string    `json:"persona_name"`
	Persona     *Persona  `json:"persona,omitempty"`
	Status      string    `json:"status"` // "idle", "working", "deciding", "blocked"
	CurrentBead string    `json:"current_bead,omitempty"`
	ProjectID   string    `json:"project_id"`
	StartedAt   time.Time `json:"started_at"`
	LastActive  time.Time `json:"last_active"`
}

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
	AuthorID  string    `json:"author_id"`  // Agent ID or "user-{id}"
	Comment   string    `json:"comment"`
	Timestamp time.Time `json:"timestamp"`
}

// Project represents a project that agents work on
type Project struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	GitRepo     string            `json:"git_repo"`
	Branch      string            `json:"branch"`
	BeadsPath   string            `json:"beads_path"`   // Path to .beads directory
	Context     map[string]string `json:"context"`      // Additional context for agents
	Status      ProjectStatus     `json:"status"`       // Current project status
	IsPerpetual bool              `json:"is_perpetual"` // If true, project never closes
	Comments    []ProjectComment  `json:"comments"`     // Comments on project state
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ClosedAt    *time.Time        `json:"closed_at,omitempty"`
	Agents      []string          `json:"agents"` // Agent IDs working on this project
}

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
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ClosedAt    *time.Time        `json:"closed_at,omitempty"`
}

// DecisionBead represents a specific decision point that needs resolution
type DecisionBead struct {
	*Bead
	Question      string   `json:"question"`
	Options       []string `json:"options,omitempty"`
	Recommendation string   `json:"recommendation,omitempty"`
	RequesterID   string   `json:"requester_id"` // Agent ID that filed the decision
	DeciderID     string   `json:"decider_id,omitempty"`
	Decision      string   `json:"decision,omitempty"`
	Rationale     string   `json:"rationale,omitempty"`
	DecidedAt     *time.Time `json:"decided_at,omitempty"`
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
	Beads    map[string]*Bead `json:"beads"`
	Edges    []Edge           `json:"edges"`
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
	AutonomyFull       AutonomyLevel = "full"        // Can make all non-P0 decisions
	AutonomySemi       AutonomyLevel = "semi"        // Can make routine decisions
	AutonomySupervised AutonomyLevel = "supervised"  // Requires approval for all decisions
)
