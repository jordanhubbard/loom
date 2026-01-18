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

// handleProject handles GET /api/v1/projects/{id} and state management endpoints
func (s *Server) handleProject(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/projects/")
	parts := strings.Split(path, "/")
	id := parts[0]

	// Handle sub-endpoints for project state management
	if len(parts) > 1 {
		action := parts[1]
		s.handleProjectStateEndpoints(w, r, id, action)
		return
	}

	// Default GET behavior
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
