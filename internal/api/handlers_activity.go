package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jordanhubbard/agenticorp/internal/activity"
	"github.com/jordanhubbard/agenticorp/internal/auth"
)

// handleGetActivityFeed handles GET requests for activity feed
// GET /api/v1/activity-feed?project_id=xxx&event_type=xxx&limit=100
func (s *Server) handleGetActivityFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	activityMgr := s.agenticorp.GetActivityManager()
	if activityMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Activity manager not available")
		return
	}

	// Parse query parameters
	filters := activity.ActivityFilters{
		Limit: 100, // Default limit
	}

	if projectID := r.URL.Query().Get("project_id"); projectID != "" {
		filters.ProjectIDs = []string{projectID}
	}

	if eventType := r.URL.Query().Get("event_type"); eventType != "" {
		filters.EventType = eventType
	}

	if actorID := r.URL.Query().Get("actor_id"); actorID != "" {
		filters.ActorID = actorID
	}

	if resourceType := r.URL.Query().Get("resource_type"); resourceType != "" {
		filters.ResourceType = resourceType
	}

	if since := r.URL.Query().Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			filters.Since = t
		}
	}

	if until := r.URL.Query().Get("until"); until != "" {
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			filters.Until = t
		}
	}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			filters.Limit = l
		}
	}

	if offset := r.URL.Query().Get("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil && o >= 0 {
			filters.Offset = o
		}
	}

	if aggregated := r.URL.Query().Get("aggregated"); aggregated != "" {
		if agg, err := strconv.ParseBool(aggregated); err == nil {
			filters.Aggregated = &agg
		}
	}

	// Apply permission filtering based on authentication
	userID := auth.GetUserIDFromRequest(r)
	role := auth.GetRoleFromRequest(r)

	// If auth is enabled and no user is authenticated, return unauthorized
	if userID == "" && s.config.Security.EnableAuth {
		s.respondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// If user is not admin, apply activity filtering
	// For now, admins can see all activities, regular users see all public activities
	// TODO: Enhance to filter by user's project membership once project-user relationships are implemented
	if role != "admin" && s.config.Security.EnableAuth {
		// In future, filter by projects the user has access to:
		// userProjects := s.agenticorp.GetUserProjects(userID)
		// if len(userProjects) > 0 {
		//     filters.ProjectIDs = userProjects
		// }
	}

	activities, err := activityMgr.GetActivities(filters)
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get activities: %v", err))
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"activities": activities,
		"count":      len(activities),
		"limit":      filters.Limit,
		"offset":     filters.Offset,
	})
}

// handleActivityFeedStream handles SSE endpoint for real-time activity feed
// GET /api/v1/activity-feed/stream
func (s *Server) handleActivityFeedStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	activityMgr := s.agenticorp.GetActivityManager()
	if activityMgr == nil {
		s.respondError(w, http.StatusServiceUnavailable, "Activity manager not available")
		return
	}

	// Check authentication
	userID := auth.GetUserIDFromRequest(r)
	role := auth.GetRoleFromRequest(r)

	// If auth is enabled and no user is authenticated, return unauthorized
	if userID == "" && s.config.Security.EnableAuth {
		s.respondError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get optional filters from query params
	projectIDFilter := r.URL.Query().Get("project_id")
	eventTypeFilter := r.URL.Query().Get("event_type")
	resourceTypeFilter := r.URL.Query().Get("resource_type")

	// For non-admin users, apply project filtering (when implemented)
	isAdmin := role == "admin"

	// Create subscriber
	subscriberID := fmt.Sprintf("activity-sse-%d", time.Now().UnixNano())
	subscriber := activityMgr.Subscribe(subscriberID)
	defer activityMgr.Unsubscribe(subscriberID)

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\n")
	fmt.Fprintf(w, "data: {\"message\": \"Connected to activity feed stream\"}\n\n")
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Stream activities to client
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return
		case activity, ok := <-subscriber:
			if !ok {
				// Channel closed
				return
			}

			// Apply filters
			if projectIDFilter != "" && activity.ProjectID != projectIDFilter {
				continue
			}
			if eventTypeFilter != "" && activity.EventType != eventTypeFilter {
				continue
			}
			if resourceTypeFilter != "" && activity.ResourceType != resourceTypeFilter {
				continue
			}

			// Apply permission filtering
			// If auth is enabled and user is not admin, apply project-based filtering
			if s.config.Security.EnableAuth && !isAdmin {
				// TODO: Filter by user's accessible projects once project-user relationships are implemented
				// For now, allow all activities for authenticated non-admin users
				// In future:
				// if !userHasAccessToProject(userID, activity.ProjectID) {
				//     continue
				// }
			}

			// Send activity to client
			data, err := json.Marshal(activity)
			if err != nil {
				continue
			}

			fmt.Fprintf(w, "event: activity\n")
			fmt.Fprintf(w, "data: %s\n\n", data)

			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		case <-time.After(30 * time.Second):
			// Send keepalive ping
			fmt.Fprintf(w, ": keepalive\n\n")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}
