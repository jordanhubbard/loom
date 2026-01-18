package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	internalmodels "github.com/jordanhubbard/arbiter/internal/models"
	"github.com/jordanhubbard/arbiter/internal/storage"
	"github.com/jordanhubbard/arbiter/pkg/models"
)

// handlePersonas handles GET /api/v1/personas
func (s *Server) handlePersonas(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	personas, err := s.arbiter.GetPersonaManager().ListPersonas()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Load full persona details
	fullPersonas := make([]*models.Persona, 0, len(personas))
	for _, name := range personas {
		persona, err := s.arbiter.GetPersonaManager().LoadPersona(name)
		if err != nil {
			continue
		}
		fullPersonas = append(fullPersonas, persona)
	}

	s.respondJSON(w, http.StatusOK, fullPersonas)
}

// handlePersona handles GET/PUT /api/v1/personas/{name}
func (s *Server) handlePersona(w http.ResponseWriter, r *http.Request) {
	name := s.extractID(r.URL.Path, "/api/v1/personas")

	switch r.Method {
	case http.MethodGet:
		persona, err := s.arbiter.GetPersonaManager().LoadPersona(name)
		if err != nil {
			s.respondError(w, http.StatusNotFound, "Persona not found")
			return
		}
		s.respondJSON(w, http.StatusOK, persona)

	case http.MethodPut:
		var persona models.Persona
		if err := s.parseJSON(r, &persona); err != nil {
			s.respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Load existing persona
		existing, err := s.arbiter.GetPersonaManager().LoadPersona(name)
		if err != nil {
			s.respondError(w, http.StatusNotFound, "Persona not found")
			return
		}

		// Update fields
		persona.Name = existing.Name
		persona.PersonaFile = existing.PersonaFile
		persona.InstructionsFile = existing.InstructionsFile
		persona.CreatedAt = existing.CreatedAt

		// Save
		if err := s.arbiter.GetPersonaManager().SavePersona(&persona); err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Invalidate cache
		s.arbiter.GetPersonaManager().InvalidateCache(name)

		s.respondJSON(w, http.StatusOK, &persona)

	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleAgents handles GET/POST /api/v1/agents
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		agents := s.arbiter.GetAgentManager().ListAgents()
		s.respondJSON(w, http.StatusOK, agents)

	case http.MethodPost:
		var req struct {
			Name        string `json:"name"`
			PersonaName string `json:"persona_name"`
			ProjectID   string `json:"project_id"`
		}
		if err := s.parseJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if req.PersonaName == "" || req.ProjectID == "" {
			s.respondError(w, http.StatusBadRequest, "persona_name and project_id are required")
			return
		}

		agent, err := s.arbiter.SpawnAgent(context.Background(), req.Name, req.PersonaName, req.ProjectID)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		s.respondJSON(w, http.StatusCreated, agent)

	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleAgent handles GET/DELETE /api/v1/agents/{id}
func (s *Server) handleAgent(w http.ResponseWriter, r *http.Request) {
	id := s.extractID(r.URL.Path, "/api/v1/agents")

	switch r.Method {
	case http.MethodGet:
		agent, err := s.arbiter.GetAgentManager().GetAgent(id)
		if err != nil {
			s.respondError(w, http.StatusNotFound, "Agent not found")
			return
		}
		s.respondJSON(w, http.StatusOK, agent)

	case http.MethodDelete:
		if err := s.arbiter.GetAgentManager().StopAgent(id); err != nil {
			s.respondError(w, http.StatusNotFound, "Agent not found")
			return
		}
		// Release all locks held by this agent
		s.arbiter.GetFileLockManager().ReleaseAgentLocks(id)
		w.WriteHeader(http.StatusNoContent)

	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleProjects handles GET/POST /api/v1/projects
func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projects := s.arbiter.GetProjectManager().ListProjects()
		s.respondJSON(w, http.StatusOK, projects)

	case http.MethodPost:
		var req struct {
			Name      string            `json:"name"`
			GitRepo   string            `json:"git_repo"`
			Branch    string            `json:"branch"`
			BeadsPath string            `json:"beads_path"`
			Context   map[string]string `json:"context"`
		}
		if err := s.parseJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if req.Name == "" || req.GitRepo == "" || req.Branch == "" {
			s.respondError(w, http.StatusBadRequest, "name, git_repo, and branch are required")
			return
		}

		project, err := s.arbiter.GetProjectManager().CreateProject(req.Name, req.GitRepo, req.Branch, req.BeadsPath, req.Context)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		s.respondJSON(w, http.StatusCreated, project)

	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleProject handles GET /api/v1/projects/{id}
func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	id := s.extractID(r.URL.Path, "/api/v1/projects")

	switch r.Method {
	case http.MethodGet:
		project, err := s.arbiter.GetProjectManager().GetProject(id)
		if err != nil {
			s.respondError(w, http.StatusNotFound, "Project not found")
			return
		}
		s.respondJSON(w, http.StatusOK, project)

	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// Handler provides HTTP handlers for the arbiter API
type Handler struct {
	storage *storage.Storage
}

// NewHandler creates a new API handler
func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{
		storage: storage,
	}
}

// CreateWork handles POST /api/work
func (h *Handler) CreateWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req internalmodels.CreateWorkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.Description == "" {
		http.Error(w, "Description is required", http.StatusBadRequest)
		return
	}

	work := &internalmodels.Work{
		ID:          uuid.New().String(),
		Description: req.Description,
		Status:      internalmodels.WorkStatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := h.storage.CreateWork(work); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create work: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(work)
}

// ListWork handles GET /api/work
func (h *Handler) ListWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if we should filter for in-progress only
	inProgressOnly := r.URL.Query().Get("status") == "in_progress"

	var works []*internalmodels.Work
	if inProgressOnly {
		works = h.storage.ListInProgressWorks()
	} else {
		works = h.storage.ListWorks()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(works)
}

// ListAgents handles GET /api/agents
func (h *Handler) ListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agents := h.storage.ListAgents()
	communications := h.storage.GetRecentCommunications(100)

	response := map[string]interface{}{
		"agents":         agents,
		"communications": communications,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListServices handles GET /api/services
func (h *Handler) ListServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if we should filter for active only
	activeOnly := r.URL.Query().Get("active") == "true"

	var services []*internalmodels.ServiceEndpoint
	if activeOnly {
		services = h.storage.ListActiveServices()
	} else {
		services = h.storage.ListServices()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

// GetServiceCosts handles GET /api/services/:id/costs
func (h *Handler) GetServiceCosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract service ID from URL path
	serviceID := extractServiceID(r.URL.Path)
	if serviceID == "" {
		http.Error(w, "Service ID is required", http.StatusBadRequest)
		return
	}

	service, err := h.storage.GetService(serviceID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Service not found: %v", err), http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"id":             service.ID,
		"name":           service.Name,
		"cost_type":      service.CostType,
		"cost_per_token": service.CostPerToken,
		"fixed_cost":     service.FixedCost,
		"tokens_used":    service.TokensUsed,
		"total_cost":     service.TotalCost,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// UpdateServiceCosts handles PUT /api/services/:id/costs
func (h *Handler) UpdateServiceCosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract service ID from URL path
	serviceID := extractServiceID(r.URL.Path)
	if serviceID == "" {
		http.Error(w, "Service ID is required", http.StatusBadRequest)
		return
	}

	var req internalmodels.UpdateServiceCostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate cost type
	if req.CostType != internalmodels.CostTypeFixed && req.CostType != internalmodels.CostTypeVariable {
		http.Error(w, "Invalid cost_type. Must be 'fixed' or 'variable'", http.StatusBadRequest)
		return
	}

	if err := h.storage.UpdateServiceCosts(serviceID, req.CostType, req.CostPerToken, req.FixedCost); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update service costs: %v", err), http.StatusInternalServerError)
		return
	}

	service, err := h.storage.GetService(serviceID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to retrieve service: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(service)
}

// GetPreferredServices handles GET /api/services/preferred
func (h *Handler) GetPreferredServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	services := h.storage.GetPreferredServices()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

// SimulateUsage handles POST /api/services/:id/usage (for testing/simulation)
func (h *Handler) SimulateUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serviceID := extractServiceID(r.URL.Path)
	if serviceID == "" {
		http.Error(w, "Service ID is required", http.StatusBadRequest)
		return
	}

	var req struct {
		TokensUsed int64 `json:"tokens_used"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.TokensUsed <= 0 {
		http.Error(w, "tokens_used must be positive", http.StatusBadRequest)
		return
	}

	if err := h.storage.RecordServiceUsage(serviceID, req.TokensUsed); err != nil {
		http.Error(w, fmt.Sprintf("Failed to record usage: %v", err), http.StatusInternalServerError)
		return
	}

	service, err := h.storage.GetService(serviceID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to retrieve service: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(service)
}

// Helper function to extract service ID from URL path
func extractServiceID(path string) string {
	// Expected paths:
	// /api/services/:id/costs
	// /api/services/:id/usage
	
	// Split by "/" and filter empty strings
	parts := strings.Split(path, "/")
	
	// Filter out empty parts
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	
	// Check if we have the expected structure: api/services/:id/...
	if len(filtered) >= 4 && filtered[0] == "api" && filtered[1] == "services" {
		return filtered[2]
	}
	return ""
}
