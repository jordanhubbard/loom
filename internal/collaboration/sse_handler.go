package collaboration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SSEHandler handles Server-Sent Events for real-time context updates
type SSEHandler struct {
	store *ContextStore
}

// NewSSEHandler creates a new SSE handler
func NewSSEHandler(store *ContextStore) *SSEHandler {
	return &SSEHandler{
		store: store,
	}
}

// ServeHTTP handles SSE connections for a bead
// URL format: /api/v1/beads/{bead_id}/context/stream
func (h *SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get bead ID from request (implementation depends on router)
	beadID := r.URL.Query().Get("bead_id")
	if beadID == "" {
		http.Error(w, "bead_id parameter required", http.StatusBadRequest)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Subscribe to updates
	updateChan := h.store.Subscribe(beadID)
	defer h.store.Unsubscribe(beadID, updateChan)

	// Get initial context state
	initialCtx, err := h.store.Get(r.Context(), beadID)
	if err != nil {
		// Send error event
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}

	// Send initial state
	initialCtx.mu.RLock()
	initialData, _ := json.Marshal(map[string]interface{}{
		"type":    "initial",
		"bead_id": initialCtx.BeadID,
		"context": initialCtx,
	})
	initialCtx.mu.RUnlock()

	fmt.Fprintf(w, "event: initial\ndata: %s\n\n", string(initialData))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Stream updates
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case update, ok := <-updateChan:
			if !ok {
				return
			}

			// Send update event
			updateData, _ := json.Marshal(update)
			fmt.Fprintf(w, "event: update\ndata: %s\n\n", string(updateData))

			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}

		case <-ticker.C:
			// Send keep-alive ping
			fmt.Fprintf(w, ": ping\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
}

// HandleGetContext returns the current context state as JSON
func (h *SSEHandler) HandleGetContext(w http.ResponseWriter, r *http.Request) {
	beadID := r.URL.Query().Get("bead_id")
	if beadID == "" {
		http.Error(w, "bead_id parameter required", http.StatusBadRequest)
		return
	}

	data, err := h.store.ExportContext(r.Context(), beadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// HandleJoinBead handles agent joining a bead context
func (h *SSEHandler) HandleJoinBead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		BeadID  string `json:"bead_id"`
		AgentID string `json:"agent_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.BeadID == "" || req.AgentID == "" {
		http.Error(w, "bead_id and agent_id required", http.StatusBadRequest)
		return
	}

	if err := h.store.JoinBead(r.Context(), req.BeadID, req.AgentID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "joined",
		"bead_id":  req.BeadID,
		"agent_id": req.AgentID,
	})
}

// HandleLeaveBead handles agent leaving a bead context
func (h *SSEHandler) HandleLeaveBead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		BeadID  string `json:"bead_id"`
		AgentID string `json:"agent_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.BeadID == "" || req.AgentID == "" {
		http.Error(w, "bead_id and agent_id required", http.StatusBadRequest)
		return
	}

	if err := h.store.LeaveBead(r.Context(), req.BeadID, req.AgentID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "left",
		"bead_id":  req.BeadID,
		"agent_id": req.AgentID,
	})
}

// HandleUpdateData handles updating shared data
func (h *SSEHandler) HandleUpdateData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		BeadID          string                 `json:"bead_id"`
		AgentID         string                 `json:"agent_id"`
		Key             string                 `json:"key"`
		Value           interface{}            `json:"value"`
		ExpectedVersion int64                  `json:"expected_version,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.BeadID == "" || req.AgentID == "" || req.Key == "" {
		http.Error(w, "bead_id, agent_id, and key required", http.StatusBadRequest)
		return
	}

	err := h.store.UpdateData(r.Context(), req.BeadID, req.AgentID, req.Key, req.Value, req.ExpectedVersion)
	if err != nil {
		if conflictErr, ok := err.(*ConflictError); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":            "version_conflict",
				"expected_version": conflictErr.ExpectedVersion,
				"actual_version":   conflictErr.ActualVersion,
			})
			return
		}

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get current context to return version
	ctx, err := h.store.Get(r.Context(), req.BeadID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx.mu.RLock()
	currentVersion := ctx.Version
	ctx.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "updated",
		"bead_id": req.BeadID,
		"key": req.Key,
		"version": currentVersion,
	})
}

// HandleAddActivity handles adding activity log entry
func (h *SSEHandler) HandleAddActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		BeadID       string                 `json:"bead_id"`
		AgentID      string                 `json:"agent_id"`
		ActivityType string                 `json:"activity_type"`
		Description  string                 `json:"description"`
		Data         map[string]interface{} `json:"data,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.BeadID == "" || req.AgentID == "" || req.ActivityType == "" {
		http.Error(w, "bead_id, agent_id, and activity_type required", http.StatusBadRequest)
		return
	}

	err := h.store.AddActivity(r.Context(), req.BeadID, req.AgentID, req.ActivityType, req.Description, req.Data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "activity_added",
		"bead_id": req.BeadID,
		"activity_type": req.ActivityType,
	})
}
