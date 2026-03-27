package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/pkg/models"
)

func (s *Server) handleReviews(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetReviews(w, r)
	case http.MethodPost:
		s.handleCreateReview(w, r)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleGetReviews(w http.ResponseWriter, r *http.Request) {
	projectID := r.URL.Query().Get("project_id")
	beadID := r.URL.Query().Get("bead_id")
	status := r.URL.Query().Get("status")
	reviewType := r.URL.Query().Get("type")

	reviews := make([]*models.Review, 0)

	// TODO: filter reviews by projectID, beadID, status, reviewType when DB implemented
	_, _, _, _ = projectID, beadID, status, reviewType

	s.respondJSON(w, http.StatusOK, reviews)
}

func (s *Server) handleCreateReview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BeadID      string `json:"bead_id"`
		ProjectID   string `json:"project_id"`
		Type        string `json:"type"`
		Title       string `json:"title"`
		Description string `json:"description"`
		ReviewerID  string `json:"reviewer_id"`
		AuthorID    string `json:"author_id"`
	}

	if err := s.parseJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.BeadID == "" || req.ProjectID == "" || req.Type == "" {
		s.respondError(w, http.StatusBadRequest, "bead_id, project_id, and type are required")
		return
	}

	review := &models.Review{
		EntityMetadata: models.NewEntityMetadata(models.ReviewSchemaVersion),
		ID:             generateID("review"),
		BeadID:         req.BeadID,
		ProjectID:      req.ProjectID,
		Type:           models.ReviewType(req.Type),
		Title:          req.Title,
		Description:    req.Description,
		Status:         models.ReviewStatusPending,
		ReviewerID:     req.ReviewerID,
		AuthorID:       req.AuthorID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Comments:       make([]models.ReviewComment, 0),
		Findings:       make([]models.ReviewFinding, 0),
	}

	s.respondJSON(w, http.StatusCreated, review)
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/reviews/")
	parts := strings.Split(path, "/")
	id := parts[0]

	if len(parts) > 1 {
		action := parts[1]
		s.handleReviewAction(w, r, id, action)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetReview(w, r, id)
	case http.MethodPut:
		s.handleUpdateReview(w, r, id)
	case http.MethodDelete:
		s.handleDeleteReview(w, r, id)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleGetReview(w http.ResponseWriter, r *http.Request, id string) {
	review := &models.Review{
		ID: id,
	}
	s.respondJSON(w, http.StatusOK, review)
}

func (s *Server) handleUpdateReview(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Status      string `json:"status"`
		Verdict     string `json:"verdict"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}

	if err := s.parseJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	review := &models.Review{
		ID:          id,
		Status:      models.ReviewStatus(req.Status),
		Verdict:     req.Verdict,
		Title:       req.Title,
		Description: req.Description,
		UpdatedAt:   time.Now(),
	}

	s.respondJSON(w, http.StatusOK, review)
}

func (s *Server) handleDeleteReview(w http.ResponseWriter, r *http.Request, id string) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReviewAction(w http.ResponseWriter, r *http.Request, id, action string) {
	switch action {
	case "comments":
		s.handleReviewComments(w, r, id)
	case "findings":
		s.handleReviewFindings(w, r, id)
	case "approve":
		s.handleApproveReview(w, r, id)
	case "reject":
		s.handleRejectReview(w, r, id)
	default:
		s.respondError(w, http.StatusNotFound, "Unknown action")
	}
}

func (s *Server) handleReviewComments(w http.ResponseWriter, r *http.Request, reviewID string) {
	switch r.Method {
	case http.MethodGet:
		comments := make([]models.ReviewComment, 0)
		s.respondJSON(w, http.StatusOK, comments)

	case http.MethodPost:
		var req struct {
			AuthorID   string `json:"author_id"`
			Content    string `json:"content"`
			FilePath   string `json:"file_path"`
			LineNumber int    `json:"line_number"`
		}

		if err := s.parseJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if req.Content == "" {
			s.respondError(w, http.StatusBadRequest, "content is required")
			return
		}

		comment := models.ReviewComment{
			ID:         generateID("comment"),
			ReviewID:   reviewID,
			AuthorID:   req.AuthorID,
			Content:    req.Content,
			FilePath:   req.FilePath,
			LineNumber: req.LineNumber,
			Resolved:   false,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		s.respondJSON(w, http.StatusCreated, comment)

	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleReviewFindings(w http.ResponseWriter, r *http.Request, reviewID string) {
	switch r.Method {
	case http.MethodGet:
		findings := make([]models.ReviewFinding, 0)
		s.respondJSON(w, http.StatusOK, findings)

	case http.MethodPost:
		var req struct {
			Severity    string `json:"severity"`
			Category    string `json:"category"`
			Title       string `json:"title"`
			Description string `json:"description"`
			FilePath    string `json:"file_path"`
			LineNumber  int    `json:"line_number"`
			Suggestion  string `json:"suggestion"`
		}

		if err := s.parseJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if req.Title == "" || req.Severity == "" {
			s.respondError(w, http.StatusBadRequest, "title and severity are required")
			return
		}

		finding := models.ReviewFinding{
			ID:          generateID("finding"),
			ReviewID:    reviewID,
			Severity:    req.Severity,
			Category:    req.Category,
			Title:       req.Title,
			Description: req.Description,
			FilePath:    req.FilePath,
			LineNumber:  req.LineNumber,
			Suggestion:  req.Suggestion,
			Status:      "open",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		s.respondJSON(w, http.StatusCreated, finding)

	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleApproveReview(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Verdict string `json:"verdict"`
	}

	if err := s.parseJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	now := time.Now()
	review := &models.Review{
		ID:          id,
		Status:      models.ReviewStatusApproved,
		Verdict:     req.Verdict,
		CompletedAt: &now,
		UpdatedAt:   time.Now(),
	}

	s.respondJSON(w, http.StatusOK, review)
}

func (s *Server) handleRejectReview(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Verdict string `json:"verdict"`
	}

	if err := s.parseJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	now := time.Now()
	review := &models.Review{
		ID:          id,
		Status:      models.ReviewStatusRejected,
		Verdict:     req.Verdict,
		CompletedAt: &now,
		UpdatedAt:   time.Now(),
	}

	s.respondJSON(w, http.StatusOK, review)
}

func generateID(prefix string) string {
	return prefix + "-" + time.Now().Format("20060102150405")
}
