package api

import (
	"encoding/json"
	"net/http"
)

// handleFederationStatus handles GET /api/v1/federation/status
func (s *Server) handleFederationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	result := map[string]interface{}{
		"enabled": s.config.Beads.Federation.Enabled,
	}

	// Include Dolt coordinator status if available
	if dc := s.app.GetDoltCoordinator(); dc != nil {
		result["dolt_coordinator"] = dc.Status()
	}

	if !s.config.Beads.Federation.Enabled {
		result["message"] = "Federation is not enabled"
		s.respondJSON(w, http.StatusOK, result)
		return
	}

	output, err := s.app.GetBeadsManager().FederationStatus(r.Context())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Parse and re-encode to ensure valid JSON response
	var status interface{}
	if err := json.Unmarshal(output, &status); err != nil {
		result["raw"] = string(output)
		s.respondJSON(w, http.StatusOK, result)
		return
	}

	result["status"] = status
	s.respondJSON(w, http.StatusOK, result)
}

// handleFederationSync handles POST /api/v1/federation/sync
func (s *Server) handleFederationSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if !s.config.Beads.Federation.Enabled {
		s.respondError(w, http.StatusBadRequest, "Federation is not enabled")
		return
	}

	err := s.app.GetBeadsManager().SyncFederation(r.Context(), &s.config.Beads.Federation)
	if err != nil {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"synced": false,
			"error":  err.Error(),
		})
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"synced": true,
	})
}
