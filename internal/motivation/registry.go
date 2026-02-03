package motivation

import (
	"fmt"
	"sync"
	"time"
)

// Registry manages all motivations in the system
type Registry struct {
	motivations map[string]*Motivation
	byRole      map[string][]*Motivation // Index by agent role
	byProject   map[string][]*Motivation // Index by project
	triggers    []*MotivationTrigger     // Recent trigger history
	mu          sync.RWMutex
	config      *MotivationConfig
	nextID      int
}

// NewRegistry creates a new motivation registry
func NewRegistry(config *MotivationConfig) *Registry {
	if config == nil {
		config = DefaultConfig()
	}
	return &Registry{
		motivations: make(map[string]*Motivation),
		byRole:      make(map[string][]*Motivation),
		byProject:   make(map[string][]*Motivation),
		triggers:    make([]*MotivationTrigger, 0),
		config:      config,
		nextID:      1,
	}
}

// Register adds a new motivation to the registry
func (r *Registry) Register(m *Motivation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if m == nil {
		return fmt.Errorf("motivation cannot be nil")
	}

	// Generate ID if not provided
	if m.ID == "" {
		m.ID = fmt.Sprintf("mot-%d", r.nextID)
		r.nextID++
	}

	// Check for duplicates
	if _, exists := r.motivations[m.ID]; exists {
		return fmt.Errorf("motivation with ID %s already exists", m.ID)
	}

	// Set defaults
	if m.Status == "" {
		if r.config.EnabledByDefault {
			m.Status = MotivationStatusActive
		} else {
			m.Status = MotivationStatusDisabled
		}
	}
	if m.CooldownPeriod == 0 {
		m.CooldownPeriod = r.config.DefaultCooldown
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	m.UpdatedAt = time.Now()

	// Store motivation
	r.motivations[m.ID] = m

	// Index by role
	if m.AgentRole != "" {
		r.byRole[m.AgentRole] = append(r.byRole[m.AgentRole], m)
	}

	// Index by project
	if m.ProjectID != "" {
		r.byProject[m.ProjectID] = append(r.byProject[m.ProjectID], m)
	} else {
		// Global motivations indexed under empty string
		r.byProject[""] = append(r.byProject[""], m)
	}

	return nil
}

// Unregister removes a motivation from the registry
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, exists := r.motivations[id]
	if !exists {
		return fmt.Errorf("motivation not found: %s", id)
	}

	// Remove from indexes
	if m.AgentRole != "" {
		r.byRole[m.AgentRole] = r.removeFromSlice(r.byRole[m.AgentRole], m)
	}
	if m.ProjectID != "" {
		r.byProject[m.ProjectID] = r.removeFromSlice(r.byProject[m.ProjectID], m)
	} else {
		r.byProject[""] = r.removeFromSlice(r.byProject[""], m)
	}

	delete(r.motivations, id)
	return nil
}

// Get retrieves a motivation by ID
func (r *Registry) Get(id string) (*Motivation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, exists := r.motivations[id]
	if !exists {
		return nil, fmt.Errorf("motivation not found: %s", id)
	}
	return m, nil
}

// List returns all motivations, optionally filtered
func (r *Registry) List(filters *MotivationFilters) []*Motivation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Motivation, 0)

	for _, m := range r.motivations {
		if filters == nil || filters.Matches(m) {
			result = append(result, m)
		}
	}

	return result
}

// ListByRole returns all motivations for a specific agent role
func (r *Registry) ListByRole(role string) []*Motivation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Include global motivations (no specific role) and role-specific ones
	result := make([]*Motivation, 0)

	for _, m := range r.motivations {
		if m.AgentRole == "" || m.AgentRole == role {
			result = append(result, m)
		}
	}

	return result
}

// ListByProject returns all motivations for a specific project
func (r *Registry) ListByProject(projectID string) []*Motivation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Motivation, 0)

	// Include global motivations and project-specific ones
	result = append(result, r.byProject[""]...)
	if projectID != "" {
		result = append(result, r.byProject[projectID]...)
	}

	return result
}

// GetActive returns all active motivations
func (r *Registry) GetActive() []*Motivation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Motivation, 0)
	for _, m := range r.motivations {
		if m.Status == MotivationStatusActive {
			result = append(result, m)
		}
	}
	return result
}

// Enable enables a motivation
func (r *Registry) Enable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, exists := r.motivations[id]
	if !exists {
		return fmt.Errorf("motivation not found: %s", id)
	}

	m.Status = MotivationStatusActive
	m.DisabledAt = nil
	m.UpdatedAt = time.Now()
	return nil
}

// Disable disables a motivation
func (r *Registry) Disable(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, exists := r.motivations[id]
	if !exists {
		return fmt.Errorf("motivation not found: %s", id)
	}

	now := time.Now()
	m.Status = MotivationStatusDisabled
	m.DisabledAt = &now
	m.UpdatedAt = now
	return nil
}

// Update updates a motivation's configuration
func (r *Registry) Update(id string, updates map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, exists := r.motivations[id]
	if !exists {
		return fmt.Errorf("motivation not found: %s", id)
	}

	// Apply updates
	if name, ok := updates["name"].(string); ok {
		m.Name = name
	}
	if desc, ok := updates["description"].(string); ok {
		m.Description = desc
	}
	if params, ok := updates["parameters"].(map[string]interface{}); ok {
		m.Parameters = params
	}
	if cooldown, ok := updates["cooldown_period"].(time.Duration); ok {
		m.CooldownPeriod = cooldown
	}
	if priority, ok := updates["priority"].(int); ok {
		m.Priority = priority
	}

	m.UpdatedAt = time.Now()
	return nil
}

// RecordTrigger records that a motivation was triggered
func (r *Registry) RecordTrigger(trigger *MotivationTrigger) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Update motivation state
	if m, exists := r.motivations[trigger.MotivationID]; exists {
		m.LastTriggeredAt = &trigger.TriggeredAt
		m.TriggerCount++
		m.UpdatedAt = time.Now()

		// Put in cooldown if successful
		if trigger.Result == TriggerResultSuccess {
			m.Status = MotivationStatusCooldown
		}
	}

	// Add to history (keep last 1000)
	r.triggers = append(r.triggers, trigger)
	if len(r.triggers) > 1000 {
		r.triggers = r.triggers[len(r.triggers)-1000:]
	}
}

// GetTriggerHistory returns recent trigger history
func (r *Registry) GetTriggerHistory(limit int) []*MotivationTrigger {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 || limit > len(r.triggers) {
		limit = len(r.triggers)
	}

	// Return most recent
	start := len(r.triggers) - limit
	if start < 0 {
		start = 0
	}

	result := make([]*MotivationTrigger, limit)
	copy(result, r.triggers[start:])
	return result
}

// CheckCooldowns updates motivation statuses based on cooldown expiration
func (r *Registry) CheckCooldowns() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for _, m := range r.motivations {
		if m.Status == MotivationStatusCooldown && m.LastTriggeredAt != nil {
			if now.Sub(*m.LastTriggeredAt) >= m.CooldownPeriod {
				m.Status = MotivationStatusActive
			}
		}
	}
}

// Count returns the total number of motivations
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.motivations)
}

// Config returns the registry configuration
func (r *Registry) Config() *MotivationConfig {
	return r.config
}

// Helper to remove a motivation from a slice
func (r *Registry) removeFromSlice(slice []*Motivation, m *Motivation) []*Motivation {
	for i, item := range slice {
		if item.ID == m.ID {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// MotivationFilters for querying motivations
type MotivationFilters struct {
	Type      MotivationType
	Status    MotivationStatus
	AgentRole string
	ProjectID string
	IsBuiltIn *bool
}

// Matches checks if a motivation matches the filters
func (f *MotivationFilters) Matches(m *Motivation) bool {
	if f.Type != "" && m.Type != f.Type {
		return false
	}
	if f.Status != "" && m.Status != f.Status {
		return false
	}
	if f.AgentRole != "" && m.AgentRole != f.AgentRole {
		return false
	}
	if f.ProjectID != "" && m.ProjectID != f.ProjectID {
		return false
	}
	if f.IsBuiltIn != nil && m.IsBuiltIn != *f.IsBuiltIn {
		return false
	}
	return true
}
