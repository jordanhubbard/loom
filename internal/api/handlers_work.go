package api

import (
	"context"
	"net/http"
	"time"

	"github.com/jordanhubbard/arbiter/internal/worker"
)

// handleWork handles POST /api/v1/work for non-bead work (simple prompts).
func (s *Server) handleWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		AgentID    string `json:"agent_id"`
		ProjectID  string `json:"project_id"`
		Prompt     string `json:"prompt"`
		Context    string `json:"context"`
		TimeoutSec int    `json:"timeout_sec"`
	}
	if err := s.parseJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.AgentID == "" || req.ProjectID == "" || req.Prompt == "" {
		s.respondError(w, http.StatusBadRequest, "agent_id, project_id, and prompt are required")
		return
	}

	timeout := 10 * time.Minute
	if req.TimeoutSec > 0 {
		timeout = time.Duration(req.TimeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	task := &worker.Task{
		ID:          "prompt-" + time.Now().UTC().Format(time.RFC3339Nano),
		Description: req.Prompt,
		Context:     req.Context,
		ProjectID:   req.ProjectID,
	}

	result, err := s.arbiter.GetAgentManager().ExecuteTask(ctx, req.AgentID, task)
	if err != nil {
		s.respondError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, result)
}
