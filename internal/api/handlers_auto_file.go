package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jordanhubbard/agenticorp/pkg/models"
)

// AutoFileBugRequest represents an automatically filed bug report
type AutoFileBugRequest struct {
	Title       string                 `json:"title"`       // User-provided or auto-generated title
	Source      string                 `json:"source"`      // "frontend" or "backend"
	ErrorType   string                 `json:"error_type"`  // e.g., "javascript_error", "api_error", "panic"
	Message     string                 `json:"message"`     // Error message
	StackTrace  string                 `json:"stack_trace"` // Stack trace if available
	Context     map[string]interface{} `json:"context"`     // Additional context (URL, user agent, etc.)
	Severity    string                 `json:"severity"`    // "critical", "high", "medium", "low"
	OccurredAt  time.Time              `json:"occurred_at"` // When the error occurred
}

// HandleAutoFileBug handles automatic bug report filing
func (s *Server) HandleAutoFileBug(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AutoFileBugRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Validate required fields
	if req.Title == "" {
		req.Title = fmt.Sprintf("%s error", req.ErrorType)
	}
	if req.Source == "" {
		req.Source = "unknown"
	}
	if req.OccurredAt.IsZero() {
		req.OccurredAt = time.Now()
	}

	// Create bead with [auto-filed] prefix
	title := fmt.Sprintf("[auto-filed] [%s] %s", req.Source, req.Title)
	
	// Build description
	description := fmt.Sprintf(`## Auto-Filed Bug Report

**Source:** %s
**Error Type:** %s
**Severity:** %s
**Occurred At:** %s

### Error Message
%s

### Stack Trace
%s

### Context
%s

---
*This bug was automatically filed by the AgentiCorp error tracking system.*
*Assigned to QA Engineer for triage.*
`,
		req.Source,
		req.ErrorType,
		req.Severity,
		req.OccurredAt.Format(time.RFC3339),
		req.Message,
		req.StackTrace,
		formatContext(req.Context),
	)

	// Determine priority based on severity
	priority := models.BeadPriority(1) // Default P1 (High)
	switch req.Severity {
	case "critical":
		priority = models.BeadPriority(0) // P0 (Critical)
	case "high":
		priority = models.BeadPriority(1) // P1 (High)
	case "medium":
		priority = models.BeadPriority(2) // P2 (Normal)
	case "low":
		priority = models.BeadPriority(3) // P3 (Low)
	}

	// Get project ID (use agenticorp-self for now)
	projectID := "agenticorp-self"
	if s.agenticorp != nil {
		if pm := s.agenticorp.GetProjectManager(); pm != nil {
			if project, err := pm.GetProject("agenticorp-self"); err == nil && project != nil {
				projectID = project.ID
			}
		}
	}

	// Create the bead
	bead, err := s.agenticorp.CreateBead(title, description, priority, "bug", projectID)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create bead: %v", err))
		return
	}

	// Assign to QA Engineer
	if err := s.assignToQAEngineer(bead.ID, projectID); err != nil {
		// Log but don't fail - bead is created
		fmt.Printf("[WARN] Failed to assign auto-filed bead %s to QA Engineer: %v\n", bead.ID, err)
	}

	// Add tags
	updates := map[string]interface{}{
		"tags": []string{"auto-filed", req.Source, req.ErrorType},
	}
	if _, err := s.agenticorp.UpdateBead(bead.ID, updates); err != nil {
		fmt.Printf("[WARN] Failed to update tags for bead %s: %v\n", bead.ID, err)
	}

	s.respondJSON(w, http.StatusCreated, map[string]interface{}{
		"bead_id": bead.ID,
		"message": "Bug report filed automatically",
		"assigned_to": "qa-engineer",
	})
}

// assignToQAEngineer finds and assigns the bead to the QA Engineer agent
func (s *Server) assignToQAEngineer(beadID, projectID string) error {
	if s.agenticorp == nil {
		return fmt.Errorf("agenticorp not initialized")
	}

	// Get list of agents
	agentManager := s.agenticorp.GetAgentManager()
	if agentManager == nil {
		return fmt.Errorf("agent manager not available")
	}
	
	agents := agentManager.ListAgents()

	// Find QA Engineer for this project
	for _, agent := range agents {
		if agent.Role == "qa-engineer" && agent.ProjectID == projectID {
			// Update bead with assigned agent
			updates := map[string]interface{}{
				"assigned_to": agent.ID,
			}
			
			if _, err := s.agenticorp.UpdateBead(beadID, updates); err != nil {
				return fmt.Errorf("failed to assign bead: %w", err)
			}
			
			return nil
		}
	}

	return fmt.Errorf("no QA Engineer found for project %s", projectID)
}

// formatContext converts context map to readable string
func formatContext(ctx map[string]interface{}) string {
	if len(ctx) == 0 {
		return "No additional context"
	}

	result := "```json\n"
	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error formatting context: %v", err)
	}
	result += string(data)
	result += "\n```"
	return result
}
