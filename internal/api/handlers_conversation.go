package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/internal/database"
	"github.com/jordanhubbard/loom/pkg/models"
)

// handleConversationsList handles listing conversations
// GET /api/v1/conversations - List all conversations for a project
// Gracefully degrades: returns 200 with empty list if app/db unavailable
func (s *Server) handleConversationsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Get project ID from query parameters
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		s.respondError(w, http.StatusBadRequest, "project_id query parameter is required")
		return
	}

	// Get limit from query parameters (default to 50)
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	// Graceful degradation: if app or db is unavailable, return empty list
	if s.app == nil || s.app.GetDatabase() == nil {
		log.Printf("[WARN] Conversations list requested but app/db unavailable, returning empty list")
		s.respondJSON(w, http.StatusOK, []*models.ConversationContext{})
		return
	}

	db := s.app.GetDatabase()

	// Create a context with a timeout derived from the request context
	// Use the request context as the base, with a 30-second timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// List conversations for the project
	conversations, err := db.ListConversationContextsByProject(ctx, projectID, limit)
	if err != nil {
		log.Printf("Error listing conversations: %v", err)
		// Graceful degradation: return empty list on error
		s.respondJSON(w, http.StatusOK, []*models.ConversationContext{})
		return
	}

	if conversations == nil {
		conversations = []*models.ConversationContext{}
	}

	s.respondJSON(w, http.StatusOK, conversations)
}

// handleConversation handles operations on a specific conversation session
// GET /api/v1/conversations/{id} - Get full conversation
// DELETE /api/v1/conversations/{id} - Delete session
// POST /api/v1/conversations/{id}/reset - Reset conversation history
// POST /api/v1/conversations/{id}/inject - Inject a message into the conversation
func (s *Server) handleConversation(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/conversations/")
	parts := strings.Split(path, "/")

	// If no ID is provided, route to the list handler
	if len(parts) == 0 || parts[0] == "" {
		s.handleConversationsList(w, r)
		return
	}

	// Graceful degradation: if app or db is unavailable, return empty list for list requests
	if s.app == nil || s.app.GetDatabase() == nil {
		log.Printf("[WARN] Conversation operation requested but app/db unavailable")
		// For GET requests without a specific ID, return empty list
		if r.Method == http.MethodGet && (len(parts) == 0 || parts[0] == "") {
			s.respondJSON(w, http.StatusOK, []*models.ConversationContext{})
			return
		}
		// For other operations, return 503
		s.respondError(w, http.StatusServiceUnavailable, "Database not available")
		return
	}

	db := s.app.GetDatabase()

	sessionID := parts[0]

	// Check for reset endpoint
	if len(parts) == 2 && parts[1] == "reset" {
		if r.Method != http.MethodPost {
			s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		s.handleResetConversation(w, r, sessionID, db)
		return
	}

	// Check for inject endpoint
	if len(parts) == 2 && parts[1] == "inject" {
		if r.Method != http.MethodPost {
			s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		s.handleInjectMessage(w, r, sessionID, db)
		return
	}

	// Handle main conversation operations
	switch r.Method {
	case http.MethodGet:
		s.handleGetConversation(w, r, sessionID, db)
	case http.MethodDelete:
		s.handleDeleteConversation(w, r, sessionID, db)
	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleGetConversation retrieves a full conversation session
func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request, sessionID string, db *database.Database) {
	session, err := db.GetConversationContext(sessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.respondError(w, http.StatusNotFound, fmt.Sprintf("Conversation session not found: %s", sessionID))
			return
		}
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get conversation: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, session)
}

// handleDeleteConversation deletes a conversation session
func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request, sessionID string, db *database.Database) {
	err := db.DeleteConversationContext(sessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.respondError(w, http.StatusNotFound, fmt.Sprintf("Conversation session not found: %s", sessionID))
			return
		}
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete conversation: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":    "Conversation deleted successfully",
		"session_id": sessionID,
	})
}

// handleResetConversation clears conversation history but keeps the session
func (s *Server) handleResetConversation(w http.ResponseWriter, r *http.Request, sessionID string, db *database.Database) {
	var req struct {
		KeepSystemMessage bool `json:"keep_system_message"`
	}
	req.KeepSystemMessage = true
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			_ = err
		}
	}

	err := db.ResetConversationMessages(sessionID, req.KeepSystemMessage)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.respondError(w, http.StatusNotFound, fmt.Sprintf("Conversation session not found: %s", sessionID))
			return
		}
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to reset conversation: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":             "Conversation reset successfully",
		"session_id":          sessionID,
		"keep_system_message": req.KeepSystemMessage,
	})
}

// handleInjectMessage injects a message into the conversation
func (s *Server) handleInjectMessage(w http.ResponseWriter, r *http.Request, sessionID string, db *database.Database) {
	var req struct {
		Message string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		s.respondError(w, http.StatusBadRequest, "Invalid message payload")
		return
	}

	err := db.InjectMessageIntoConversation(sessionID, req.Message)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.respondError(w, http.StatusNotFound, fmt.Sprintf("Conversation session not found: %s", sessionID))
			return
		}
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to inject message: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":    "Message injected successfully",
		"session_id": sessionID,
	})
}

// handleBeadConversation retrieves the conversation for a specific bead
// GET /api/v1/beads/{id}/conversation - Get conversation for bead
func (s *Server) handleBeadConversation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Check if app is available before accessing it
	if s.app == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Application not available")
		return
	}

	db := s.app.GetDatabase()
	if db == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Database not available")
		return
	}

	// Extract bead ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/beads/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "conversation" {
		s.respondError(w, http.StatusBadRequest, "Invalid path")
		return
	}

	beadID := parts[0]

	session, err := db.GetConversationContextByBeadID(beadID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.respondError(w, http.StatusNotFound, fmt.Sprintf("Conversation for bead not found: %s", beadID))
			return
		}
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get conversation: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, session)
}
