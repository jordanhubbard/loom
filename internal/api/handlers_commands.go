package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jordanhubbard/agenticorp/internal/executor"
)

// HandleExecuteCommand executes a shell command
func (s *Server) HandleExecuteCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req executor.ExecuteCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Command == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "command is required"})
		return
	}

	if req.AgentID == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id is required"})
		return
	}

	result, err := s.agenticorp.ExecuteShellCommand(r.Context(), req)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.respondJSON(w, http.StatusOK, result)
}

// HandleGetCommandLogs retrieves command execution logs
func (s *Server) HandleGetCommandLogs(w http.ResponseWriter, r *http.Request) {
	// Handle both GET /api/v1/commands and GET /api/v1/commands/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/commands")
	path = strings.TrimPrefix(path, "/")
	
	// If path is not empty, it's a specific command log ID
	if path != "" {
		s.handleGetCommandLog(w, r, path)
		return
	}

	// Otherwise, list command logs with filters
	filters := make(map[string]interface{})
	
	if agentID := r.URL.Query().Get("agent_id"); agentID != "" {
		filters["agent_id"] = agentID
	}
	if beadID := r.URL.Query().Get("bead_id"); beadID != "" {
		filters["bead_id"] = beadID
	}
	if projectID := r.URL.Query().Get("project_id"); projectID != "" {
		filters["project_id"] = projectID
	}

	logs, err := s.agenticorp.GetCommandLogs(filters, 100)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.respondJSON(w, http.StatusOK, logs)
}

// handleGetCommandLog retrieves a single command log by ID (internal)
func (s *Server) handleGetCommandLog(w http.ResponseWriter, r *http.Request, id string) {
	if id == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "command log ID required"})
		return
	}

	log, err := s.agenticorp.GetCommandLog(id)
	if err != nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "command log not found"})
		return
	}

	s.respondJSON(w, http.StatusOK, log)
}
