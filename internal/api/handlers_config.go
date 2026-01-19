package api

import (
	"context"
	"io"
	"net/http"

	arbiterpkg "github.com/jordanhubbard/arbiter/internal/arbiter"
	"github.com/jordanhubbard/arbiter/internal/temporal/eventbus"
)

// handleConfig handles GET/PUT /api/v1/config (JSON).
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		snap, err := s.arbiter.GetConfigSnapshot(context.Background())
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, snap)

	case http.MethodPut:
		var snap arbiterpkg.ConfigSnapshot
		if err := s.parseJSON(r, &snap); err != nil {
			s.respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		if err := s.arbiter.ApplyConfigSnapshot(context.Background(), &snap); err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		if eb := s.arbiter.GetEventBus(); eb != nil {
			_ = eb.Publish(&eventbus.Event{Type: eventbus.EventTypeConfigUpdated, Source: "config-api", Data: map[string]interface{}{}})
		}
		s.respondJSON(w, http.StatusOK, &snap)

	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleConfigExportYAML handles GET /api/v1/config/export.yaml.
func (s *Server) handleConfigExportYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	data, err := s.arbiter.ExportConfigSnapshotYAML(context.Background())
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handleConfigImportYAML handles POST /api/v1/config/import.yaml.
func (s *Server) handleConfigImportYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "Failed to read body")
		return
	}

	snap, err := s.arbiter.ImportConfigSnapshotYAML(context.Background(), body)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if eb := s.arbiter.GetEventBus(); eb != nil {
		_ = eb.Publish(&eventbus.Event{Type: eventbus.EventTypeConfigUpdated, Source: "config-api", Data: map[string]interface{}{}})
	}

	s.respondJSON(w, http.StatusOK, snap)
}
