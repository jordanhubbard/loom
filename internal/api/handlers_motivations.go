package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jordanhubbard/agenticorp/internal/motivation"
)

// MotivationResponse represents a motivation in API responses
type MotivationResponse struct {
	ID              string                 `json:"id"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Type            string                 `json:"type"`
	Condition       string                 `json:"condition"`
	Status          string                 `json:"status"`
	AgentRole       string                 `json:"agent_role,omitempty"`
	AgentID         string                 `json:"agent_id,omitempty"`
	ProjectID       string                 `json:"project_id,omitempty"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
	CooldownMinutes int                    `json:"cooldown_minutes"`
	LastTriggeredAt *time.Time             `json:"last_triggered_at,omitempty"`
	NextTriggerAt   *time.Time             `json:"next_trigger_at,omitempty"`
	TriggerCount    int                    `json:"trigger_count"`
	Priority        int                    `json:"priority"`
	CreateBead      bool                   `json:"create_bead"`
	WakeAgent       bool                   `json:"wake_agent"`
	IsBuiltIn       bool                   `json:"is_built_in"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// CreateMotivationRequest represents a request to create a motivation
type CreateMotivationRequest struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Type            string                 `json:"type"`
	Condition       string                 `json:"condition"`
	AgentRole       string                 `json:"agent_role,omitempty"`
	AgentID         string                 `json:"agent_id,omitempty"`
	ProjectID       string                 `json:"project_id,omitempty"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
	CooldownMinutes int                    `json:"cooldown_minutes"`
	Priority        int                    `json:"priority"`
	CreateBead      bool                   `json:"create_bead"`
	BeadTemplate    string                 `json:"bead_template,omitempty"`
	WakeAgent       bool                   `json:"wake_agent"`
}

// UpdateMotivationRequest represents a request to update a motivation
type UpdateMotivationRequest struct {
	Name            *string                `json:"name,omitempty"`
	Description     *string                `json:"description,omitempty"`
	Parameters      map[string]interface{} `json:"parameters,omitempty"`
	CooldownMinutes *int                   `json:"cooldown_minutes,omitempty"`
	Priority        *int                   `json:"priority,omitempty"`
	CreateBead      *bool                  `json:"create_bead,omitempty"`
	WakeAgent       *bool                  `json:"wake_agent,omitempty"`
	Enabled         *bool                  `json:"enabled,omitempty"`
}

// TriggerHistoryResponse represents a trigger event in API responses
type TriggerHistoryResponse struct {
	ID             string                 `json:"id"`
	MotivationID   string                 `json:"motivation_id"`
	MotivationName string                 `json:"motivation_name,omitempty"`
	TriggeredAt    time.Time              `json:"triggered_at"`
	TriggerData    map[string]interface{} `json:"trigger_data,omitempty"`
	Result         string                 `json:"result"`
	Error          string                 `json:"error,omitempty"`
	BeadCreated    string                 `json:"bead_created,omitempty"`
	AgentWoken     string                 `json:"agent_woken,omitempty"`
}

// IdleStateResponse represents the system idle state
type IdleStateResponse struct {
	IsSystemIdle      bool                  `json:"is_system_idle"`
	SystemIdlePeriod  string                `json:"system_idle_period,omitempty"`
	TotalAgents       int                   `json:"total_agents"`
	WorkingAgents     int                   `json:"working_agents"`
	IdleAgents        int                   `json:"idle_agents"`
	PausedAgents      int                   `json:"paused_agents"`
	TotalBeads        int                   `json:"total_beads"`
	OpenBeads         int                   `json:"open_beads"`
	InProgressBeads   int                   `json:"in_progress_beads"`
	IdleProjects      []ProjectIdleResponse `json:"idle_projects,omitempty"`
	LastAgentActivity time.Time             `json:"last_agent_activity"`
	CheckedAt         time.Time             `json:"checked_at"`
}

// ProjectIdleResponse represents a project's idle state
type ProjectIdleResponse struct {
	ProjectID  string `json:"project_id"`
	IsIdle     bool   `json:"is_idle"`
	IdlePeriod string `json:"idle_period,omitempty"`
	AgentCount int    `json:"agent_count"`
	OpenBeads  int    `json:"open_beads"`
}

// handleMotivations handles GET /api/v1/motivations and POST /api/v1/motivations
func (s *Server) handleMotivations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListMotivations(w, r)
	case http.MethodPost:
		s.handleCreateMotivation(w, r)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleMotivation handles operations on a single motivation
func (s *Server) handleMotivation(w http.ResponseWriter, r *http.Request) {
	id := s.extractID(r.URL.Path, "/api/v1/motivations/")

	// Check for sub-paths
	if strings.HasSuffix(r.URL.Path, "/enable") {
		s.handleEnableMotivation(w, r, id)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/disable") {
		s.handleDisableMotivation(w, r, id)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/trigger") {
		s.handleTriggerMotivation(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetMotivation(w, r, id)
	case http.MethodPut, http.MethodPatch:
		s.handleUpdateMotivation(w, r, id)
	case http.MethodDelete:
		s.handleDeleteMotivation(w, r, id)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleListMotivations lists all motivations with optional filters
func (s *Server) handleListMotivations(w http.ResponseWriter, r *http.Request) {
	registry := s.getMotivationRegistry()
	if registry == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Motivation system not available")
		return
	}

	// Parse query filters
	query := r.URL.Query()
	filters := &motivation.MotivationFilters{
		Type:      motivation.MotivationType(query.Get("type")),
		AgentRole: query.Get("agent_role"),
		ProjectID: query.Get("project_id"),
	}

	// Handle status filter (active = only active motivations)
	if query.Get("active") == "true" {
		filters.Status = motivation.MotivationStatusActive
	}

	// Handle built_in filter
	if query.Get("built_in") == "true" {
		isBuiltIn := true
		filters.IsBuiltIn = &isBuiltIn
	} else if query.Get("built_in") == "false" {
		isBuiltIn := false
		filters.IsBuiltIn = &isBuiltIn
	}

	motivations := registry.List(filters)

	responses := make([]MotivationResponse, 0, len(motivations))
	for _, m := range motivations {
		responses = append(responses, motivationToResponse(m))
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"motivations": responses,
		"count":       len(responses),
	})
}

// handleGetMotivation gets a single motivation by ID
func (s *Server) handleGetMotivation(w http.ResponseWriter, r *http.Request, id string) {
	registry := s.getMotivationRegistry()
	if registry == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Motivation system not available")
		return
	}

	m, err := registry.Get(id)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "Motivation not found")
		return
	}

	s.respondJSON(w, http.StatusOK, motivationToResponse(m))
}

// handleCreateMotivation creates a new motivation
func (s *Server) handleCreateMotivation(w http.ResponseWriter, r *http.Request) {
	registry := s.getMotivationRegistry()
	if registry == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Motivation system not available")
		return
	}

	var req CreateMotivationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.Name == "" {
		s.respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Type == "" {
		s.respondError(w, http.StatusBadRequest, "type is required")
		return
	}
	if req.Condition == "" {
		s.respondError(w, http.StatusBadRequest, "condition is required")
		return
	}

	cooldown := time.Duration(req.CooldownMinutes) * time.Minute
	if cooldown == 0 {
		cooldown = 5 * time.Minute // Default 5 minute cooldown
	}

	m := &motivation.Motivation{
		Name:                req.Name,
		Description:         req.Description,
		Type:                motivation.MotivationType(req.Type),
		Condition:           motivation.TriggerCondition(req.Condition),
		AgentRole:           req.AgentRole,
		AgentID:             req.AgentID,
		ProjectID:           req.ProjectID,
		Parameters:          req.Parameters,
		CooldownPeriod:      cooldown,
		Priority:            req.Priority,
		CreateBeadOnTrigger: req.CreateBead,
		BeadTemplate:        req.BeadTemplate,
		WakeAgent:           req.WakeAgent,
		IsBuiltIn:           false, // User-created motivations are never built-in
	}

	if err := registry.Register(m); err != nil {
		s.respondError(w, http.StatusConflict, err.Error())
		return
	}

	s.respondJSON(w, http.StatusCreated, motivationToResponse(m))
}

// handleUpdateMotivation updates an existing motivation
func (s *Server) handleUpdateMotivation(w http.ResponseWriter, r *http.Request, id string) {
	registry := s.getMotivationRegistry()
	if registry == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Motivation system not available")
		return
	}

	var req UpdateMotivationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Build updates map
	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Parameters != nil {
		updates["parameters"] = req.Parameters
	}
	if req.CooldownMinutes != nil {
		updates["cooldown_period"] = time.Duration(*req.CooldownMinutes) * time.Minute
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.CreateBead != nil {
		updates["create_bead_on_trigger"] = *req.CreateBead
	}
	if req.WakeAgent != nil {
		updates["wake_agent"] = *req.WakeAgent
	}

	if err := registry.Update(id, updates); err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// Handle enable/disable
	if req.Enabled != nil {
		if *req.Enabled {
			_ = registry.Enable(id)
		} else {
			_ = registry.Disable(id)
		}
	}

	// Return updated motivation
	m, _ := registry.Get(id)
	s.respondJSON(w, http.StatusOK, motivationToResponse(m))
}

// handleDeleteMotivation deletes a motivation
func (s *Server) handleDeleteMotivation(w http.ResponseWriter, r *http.Request, id string) {
	registry := s.getMotivationRegistry()
	if registry == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Motivation system not available")
		return
	}

	// Check if motivation exists and is not built-in
	m, err := registry.Get(id)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "Motivation not found")
		return
	}

	if m.IsBuiltIn {
		s.respondError(w, http.StatusForbidden, "Cannot delete built-in motivations")
		return
	}

	if err := registry.Unregister(id); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleEnableMotivation enables a motivation
func (s *Server) handleEnableMotivation(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	registry := s.getMotivationRegistry()
	if registry == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Motivation system not available")
		return
	}

	if err := registry.Enable(id); err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	m, _ := registry.Get(id)
	s.respondJSON(w, http.StatusOK, motivationToResponse(m))
}

// handleDisableMotivation disables a motivation
func (s *Server) handleDisableMotivation(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	registry := s.getMotivationRegistry()
	if registry == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Motivation system not available")
		return
	}

	if err := registry.Disable(id); err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}

	m, _ := registry.Get(id)
	s.respondJSON(w, http.StatusOK, motivationToResponse(m))
}

// handleTriggerMotivation manually triggers a motivation
func (s *Server) handleTriggerMotivation(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	engine := s.getMotivationEngine()
	if engine == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Motivation engine not available")
		return
	}

	trigger, err := engine.ManualTrigger(r.Context(), id)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := TriggerHistoryResponse{
		ID:           trigger.ID,
		MotivationID: trigger.MotivationID,
		TriggeredAt:  trigger.TriggeredAt,
		TriggerData:  trigger.TriggerData,
		Result:       string(trigger.Result),
		Error:        trigger.Error,
		BeadCreated:  trigger.BeadCreated,
		AgentWoken:   trigger.AgentWoken,
	}
	if trigger.Motivation != nil {
		resp.MotivationName = trigger.Motivation.Name
	}

	s.respondJSON(w, http.StatusOK, resp)
}

// handleMotivationHistory handles GET /api/v1/motivations/history
func (s *Server) handleMotivationHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	registry := s.getMotivationRegistry()
	if registry == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Motivation system not available")
		return
	}

	// Get limit from query (default 50)
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsedLimit, err := json.Marshal(l); err == nil {
			// Parse limit
			_ = parsedLimit // TODO: Implement limit parsing
		}
	}

	history := registry.GetTriggerHistory(limit)
	responses := make([]TriggerHistoryResponse, 0, len(history))
	for _, t := range history {
		resp := TriggerHistoryResponse{
			ID:           t.ID,
			MotivationID: t.MotivationID,
			TriggeredAt:  t.TriggeredAt,
			TriggerData:  t.TriggerData,
			Result:       string(t.Result),
			Error:        t.Error,
			BeadCreated:  t.BeadCreated,
			AgentWoken:   t.AgentWoken,
		}
		if t.Motivation != nil {
			resp.MotivationName = t.Motivation.Name
		}
		responses = append(responses, resp)
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"history": responses,
		"count":   len(responses),
	})
}

// handleIdleState handles GET /api/v1/motivations/idle
func (s *Server) handleIdleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// For now, return a basic response
	// Full implementation would use the IdleDetector
	resp := IdleStateResponse{
		IsSystemIdle:      false,
		TotalAgents:       0,
		WorkingAgents:     0,
		IdleAgents:        0,
		PausedAgents:      0,
		TotalBeads:        0,
		OpenBeads:         0,
		InProgressBeads:   0,
		IdleProjects:      make([]ProjectIdleResponse, 0),
		LastAgentActivity: time.Now(),
		CheckedAt:         time.Now(),
	}

	// Get agent counts from AgentiCorp if available
	if s.agenticorp != nil {
		if am := s.agenticorp.GetWorkerManager(); am != nil {
			agents := am.ListAgents()
			resp.TotalAgents = len(agents)
			for _, a := range agents {
				switch a.Status {
				case "idle":
					resp.IdleAgents++
				case "working":
					resp.WorkingAgents++
				case "paused":
					resp.PausedAgents++
				}
			}
		}
	}

	s.respondJSON(w, http.StatusOK, resp)
}

// handleMotivationRoles handles GET /api/v1/motivations/roles
func (s *Server) handleMotivationRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	roles := motivation.ListAllRoles()

	// Get motivations for each role
	roleMotivations := make(map[string][]MotivationResponse)
	for _, role := range roles {
		motivations := motivation.GetMotivationsByRole(role)
		responses := make([]MotivationResponse, 0, len(motivations))
		for _, m := range motivations {
			responses = append(responses, motivationToResponse(m))
		}
		roleMotivations[role] = responses
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"roles":       roles,
		"motivations": roleMotivations,
	})
}

// handleMotivationDefaults handles POST /api/v1/motivations/defaults
func (s *Server) handleMotivationDefaults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	registry := s.getMotivationRegistry()
	if registry == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Motivation system not available")
		return
	}

	if err := motivation.RegisterDefaults(registry); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "registered",
		"count":  len(motivation.DefaultMotivations()),
	})
}

// Helper functions

func (s *Server) getMotivationRegistry() *motivation.Registry {
	if s.agenticorp == nil {
		return nil
	}
	return s.agenticorp.GetMotivationRegistry()
}

func (s *Server) getMotivationEngine() *motivation.Engine {
	if s.agenticorp == nil {
		return nil
	}
	return s.agenticorp.GetMotivationEngine()
}

func motivationToResponse(m *motivation.Motivation) MotivationResponse {
	return MotivationResponse{
		ID:              m.ID,
		Name:            m.Name,
		Description:     m.Description,
		Type:            string(m.Type),
		Condition:       string(m.Condition),
		Status:          string(m.Status),
		AgentRole:       m.AgentRole,
		AgentID:         m.AgentID,
		ProjectID:       m.ProjectID,
		Parameters:      m.Parameters,
		CooldownMinutes: int(m.CooldownPeriod.Minutes()),
		LastTriggeredAt: m.LastTriggeredAt,
		NextTriggerAt:   m.NextTriggerAt,
		TriggerCount:    m.TriggerCount,
		Priority:        m.Priority,
		CreateBead:      m.CreateBeadOnTrigger,
		WakeAgent:       m.WakeAgent,
		IsBuiltIn:       m.IsBuiltIn,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}
