package motivation

import (
	"log"
	"sync"
	"time"
)

// IdleDetector monitors system activity and detects idle states
type IdleDetector struct {
	config           *IdleConfig
	lastAgentWork    time.Time
	lastBeadActivity time.Time
	lastSystemEvent  time.Time
	listeners        []IdleListener
	mu               sync.RWMutex
}

// IdleConfig contains configuration for idle detection
type IdleConfig struct {
	// SystemIdleThreshold is how long the entire system must be idle to trigger CEO
	SystemIdleThreshold time.Duration `json:"system_idle_threshold"`

	// ProjectIdleThreshold is how long a project must be idle to trigger review
	ProjectIdleThreshold time.Duration `json:"project_idle_threshold"`

	// AgentIdleThreshold is how long an agent must be idle to be considered available
	AgentIdleThreshold time.Duration `json:"agent_idle_threshold"`

	// CheckInterval is how often to check for idle state
	CheckInterval time.Duration `json:"check_interval"`
}

// DefaultIdleConfig returns sensible defaults
func DefaultIdleConfig() *IdleConfig {
	return &IdleConfig{
		SystemIdleThreshold:  30 * time.Minute,
		ProjectIdleThreshold: 15 * time.Minute,
		AgentIdleThreshold:   5 * time.Minute,
		CheckInterval:        30 * time.Second,
	}
}

// IdleListener receives notifications about idle state changes
type IdleListener interface {
	OnSystemIdle(duration time.Duration)
	OnProjectIdle(projectID string, duration time.Duration)
	OnAgentIdle(agentID string, duration time.Duration)
}

// IdleState represents the current idle state of the system
type IdleState struct {
	// System-level idle state
	IsSystemIdle     bool          `json:"is_system_idle"`
	SystemIdleSince  *time.Time    `json:"system_idle_since,omitempty"`
	SystemIdlePeriod time.Duration `json:"system_idle_period"`

	// Agent counts
	TotalAgents   int `json:"total_agents"`
	WorkingAgents int `json:"working_agents"`
	IdleAgents    int `json:"idle_agents"`
	PausedAgents  int `json:"paused_agents"`

	// Bead counts
	TotalBeads    int `json:"total_beads"`
	OpenBeads     int `json:"open_beads"`
	InProgressBeads int `json:"in_progress_beads"`

	// Project idle states
	IdleProjects []ProjectIdleState `json:"idle_projects,omitempty"`

	// Last activity timestamps
	LastAgentActivity time.Time `json:"last_agent_activity"`
	LastBeadActivity  time.Time `json:"last_bead_activity"`
	CheckedAt         time.Time `json:"checked_at"`
}

// ProjectIdleState represents idle state for a single project
type ProjectIdleState struct {
	ProjectID   string        `json:"project_id"`
	IsIdle      bool          `json:"is_idle"`
	IdleSince   *time.Time    `json:"idle_since,omitempty"`
	IdlePeriod  time.Duration `json:"idle_period"`
	AgentCount  int           `json:"agent_count"`
	OpenBeads   int           `json:"open_beads"`
}

// IdleDataProvider provides data needed for idle detection
type IdleDataProvider interface {
	// GetAgentStates returns agent IDs mapped to their status and last activity
	GetAgentStates() map[string]AgentActivityState

	// GetBeadStates returns bead counts by status
	GetBeadStates() map[string]int

	// GetProjectStates returns project IDs mapped to their activity state
	GetProjectStates() map[string]ProjectActivityState
}

// AgentActivityState represents an agent's activity for idle detection
type AgentActivityState struct {
	AgentID    string
	Status     string // "idle", "working", "paused"
	LastActive time.Time
	ProjectID  string
}

// ProjectActivityState represents a project's activity for idle detection
type ProjectActivityState struct {
	ProjectID        string
	LastActivity     time.Time
	ActiveAgentCount int
	OpenBeadCount    int
}

// NewIdleDetector creates a new idle detector
func NewIdleDetector(config *IdleConfig) *IdleDetector {
	if config == nil {
		config = DefaultIdleConfig()
	}

	return &IdleDetector{
		config:           config,
		lastAgentWork:    time.Now(),
		lastBeadActivity: time.Now(),
		lastSystemEvent:  time.Now(),
		listeners:        make([]IdleListener, 0),
	}
}

// AddListener adds a listener for idle state changes
func (d *IdleDetector) AddListener(listener IdleListener) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.listeners = append(d.listeners, listener)
}

// RecordAgentActivity records that an agent did something
func (d *IdleDetector) RecordAgentActivity(agentID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastAgentWork = time.Now()
	d.lastSystemEvent = time.Now()
}

// RecordBeadActivity records that a bead changed
func (d *IdleDetector) RecordBeadActivity(beadID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastBeadActivity = time.Now()
	d.lastSystemEvent = time.Now()
}

// RecordSystemEvent records any system event
func (d *IdleDetector) RecordSystemEvent() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastSystemEvent = time.Now()
}

// CheckIdleState evaluates the current idle state
func (d *IdleDetector) CheckIdleState(provider IdleDataProvider) *IdleState {
	d.mu.RLock()
	config := d.config
	lastAgentWork := d.lastAgentWork
	lastBeadActivity := d.lastBeadActivity
	d.mu.RUnlock()

	now := time.Now()
	state := &IdleState{
		CheckedAt:         now,
		LastAgentActivity: lastAgentWork,
		LastBeadActivity:  lastBeadActivity,
		IdleProjects:      make([]ProjectIdleState, 0),
	}

	// Get agent states
	if provider != nil {
		agentStates := provider.GetAgentStates()
		for _, a := range agentStates {
			state.TotalAgents++
			switch a.Status {
			case "working":
				state.WorkingAgents++
			case "paused":
				state.PausedAgents++
			default:
				state.IdleAgents++
			}
		}

		// Get bead states
		beadStates := provider.GetBeadStates()
		for status, count := range beadStates {
			state.TotalBeads += count
			switch status {
			case "open":
				state.OpenBeads += count
			case "in_progress":
				state.InProgressBeads += count
			}
		}

		// Get project states
		projectStates := provider.GetProjectStates()
		for projectID, ps := range projectStates {
			projectIdle := ProjectIdleState{
				ProjectID:  projectID,
				AgentCount: ps.ActiveAgentCount,
				OpenBeads:  ps.OpenBeadCount,
			}

			// Check if project is idle
			idleDuration := now.Sub(ps.LastActivity)
			if idleDuration >= config.ProjectIdleThreshold {
				projectIdle.IsIdle = true
				idleSince := ps.LastActivity
				projectIdle.IdleSince = &idleSince
				projectIdle.IdlePeriod = idleDuration
			}

			state.IdleProjects = append(state.IdleProjects, projectIdle)
		}
	}

	// Determine system idle state
	// System is idle when:
	// 1. No agents are working
	// 2. No recent agent activity
	// 3. Enough time has passed since last system event
	systemIdleDuration := now.Sub(lastAgentWork)
	if state.WorkingAgents == 0 && systemIdleDuration >= config.SystemIdleThreshold {
		state.IsSystemIdle = true
		idleSince := lastAgentWork
		state.SystemIdleSince = &idleSince
		state.SystemIdlePeriod = systemIdleDuration
	}

	return state
}

// IsSystemIdle returns whether the system is currently idle
func (d *IdleDetector) IsSystemIdle(provider IdleDataProvider) (bool, time.Duration) {
	state := d.CheckIdleState(provider)
	return state.IsSystemIdle, state.SystemIdlePeriod
}

// IsProjectIdle returns whether a specific project is idle
func (d *IdleDetector) IsProjectIdle(projectID string, provider IdleDataProvider) (bool, time.Duration) {
	state := d.CheckIdleState(provider)
	for _, p := range state.IdleProjects {
		if p.ProjectID == projectID {
			return p.IsIdle, p.IdlePeriod
		}
	}
	return false, 0
}

// GetIdleAgentIDs returns IDs of agents that have been idle longer than threshold
func (d *IdleDetector) GetIdleAgentIDs(provider IdleDataProvider) []string {
	d.mu.RLock()
	threshold := d.config.AgentIdleThreshold
	d.mu.RUnlock()

	if provider == nil {
		return nil
	}

	now := time.Now()
	agentStates := provider.GetAgentStates()
	idleAgents := make([]string, 0)

	for agentID, state := range agentStates {
		if state.Status == "idle" && now.Sub(state.LastActive) >= threshold {
			idleAgents = append(idleAgents, agentID)
		}
	}

	return idleAgents
}

// NotifyListeners sends idle state notifications to all listeners
func (d *IdleDetector) NotifyListeners(state *IdleState) {
	d.mu.RLock()
	listeners := make([]IdleListener, len(d.listeners))
	copy(listeners, d.listeners)
	d.mu.RUnlock()

	for _, listener := range listeners {
		if state.IsSystemIdle {
			listener.OnSystemIdle(state.SystemIdlePeriod)
		}

		for _, project := range state.IdleProjects {
			if project.IsIdle {
				listener.OnProjectIdle(project.ProjectID, project.IdlePeriod)
			}
		}
	}
}

// GetConfig returns the idle configuration
func (d *IdleDetector) GetConfig() *IdleConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.config
}

// UpdateConfig updates the idle configuration
func (d *IdleDetector) UpdateConfig(config *IdleConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.config = config
	log.Printf("Idle detector config updated: system=%v, project=%v, agent=%v",
		config.SystemIdleThreshold, config.ProjectIdleThreshold, config.AgentIdleThreshold)
}
