package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/pkg/models"
)

func (s *Server) handlePerformanceReviews(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetPerformanceReviews(w, r)
	case http.MethodPost:
		s.handleCreatePerformanceReview(w, r)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleGetPerformanceReviews(w http.ResponseWriter, r *http.Request) {
	personaID := r.URL.Query().Get("persona_id")
	reviewer := r.URL.Query().Get("reviewer_id")
	period := r.URL.Query().Get("review_period")

	reviews := make([]*models.PerformanceReview, 0)

	// Filter reviews based on query parameters
	// TODO: query DB by personaID, reviewer, period when implemented
	_, _, _ = personaID, reviewer, period

	s.respondJSON(w, http.StatusOK, reviews)
}

func (s *Server) handleCreatePerformanceReview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PersonaID       string   `json:"persona_id"`
		ReviewerID      string   `json:"reviewer_id"`
		ReviewPeriod    string   `json:"review_period"`
		Grade           string   `json:"grade"`
		Narrative       string   `json:"narrative"`
		Strengths       []string `json:"strengths"`
		Weaknesses      []string `json:"weaknesses"`
		Recommendations []string `json:"recommendations"`
	}

	if err := s.parseJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.PersonaID == "" || req.ReviewerID == "" {
		s.respondError(w, http.StatusBadRequest, "persona_id and reviewer_id are required")
		return
	}

	review := &models.PerformanceReview{
		EntityMetadata:  models.NewEntityMetadata(models.PerformanceReviewSchemaVersion),
		ID:              generateID("perf-review"),
		PersonaID:       req.PersonaID,
		ReviewerID:      req.ReviewerID,
		ReviewPeriod:    req.ReviewPeriod,
		Grade:           req.Grade,
		Narrative:       req.Narrative,
		Strengths:       req.Strengths,
		Weaknesses:      req.Weaknesses,
		Recommendations: req.Recommendations,
		ReviewDate:      time.Now(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	s.respondJSON(w, http.StatusCreated, review)
}

func (s *Server) handlePerformanceReview(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/performance-reviews/")
	parts := strings.Split(path, "/")
	id := parts[0]

	if len(parts) > 1 {
		action := parts[1]
		s.handlePerformanceReviewAction(w, r, id, action)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetPerformanceReview(w, r, id)
	case http.MethodPut:
		s.handleUpdatePerformanceReview(w, r, id)
	case http.MethodDelete:
		s.handleDeletePerformanceReview(w, r, id)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleGetPerformanceReview(w http.ResponseWriter, r *http.Request, id string) {
	review := &models.PerformanceReview{
		ID: id,
	}
	s.respondJSON(w, http.StatusOK, review)
}

func (s *Server) handleUpdatePerformanceReview(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Grade           string   `json:"grade"`
		Narrative       string   `json:"narrative"`
		Strengths       []string `json:"strengths"`
		Weaknesses      []string `json:"weaknesses"`
		Recommendations []string `json:"recommendations"`
		ActionTaken     string   `json:"action_taken"`
	}

	if err := s.parseJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	review := &models.PerformanceReview{
		ID:              id,
		Grade:           req.Grade,
		Narrative:       req.Narrative,
		Strengths:       req.Strengths,
		Weaknesses:      req.Weaknesses,
		Recommendations: req.Recommendations,
		ActionTaken:     req.ActionTaken,
		UpdatedAt:       time.Now(),
	}

	s.respondJSON(w, http.StatusOK, review)
}

func (s *Server) handleDeletePerformanceReview(w http.ResponseWriter, r *http.Request, id string) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePerformanceReviewAction(w http.ResponseWriter, r *http.Request, id, action string) {
	switch action {
	case "trends":
		s.handlePerformanceTrends(w, r, id)
	case "history":
		s.handlePerformanceHistory(w, r, id)
	default:
		s.respondError(w, http.StatusNotFound, "Unknown action")
	}
}

func (s *Server) handlePerformanceTrends(w http.ResponseWriter, r *http.Request, personaID string) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	trends := map[string]interface{}{
		"persona_id": personaID,
		"trends":     []interface{}{},
	}

	s.respondJSON(w, http.StatusOK, trends)
}

func (s *Server) handlePerformanceHistory(w http.ResponseWriter, r *http.Request, personaID string) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	history := map[string]interface{}{
		"persona_id": personaID,
		"history":    []interface{}{},
	}

	s.respondJSON(w, http.StatusOK, history)
}
