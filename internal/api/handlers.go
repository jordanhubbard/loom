package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jordanhubbard/arbiter/internal/models"
	"github.com/jordanhubbard/arbiter/internal/storage"
)

// Handler provides HTTP handlers for the arbiter API
type Handler struct {
	storage *storage.Storage
}

// NewHandler creates a new API handler
func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{
		storage: storage,
	}
}

// CreateWork handles POST /api/work
func (h *Handler) CreateWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.CreateWorkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.Description == "" {
		http.Error(w, "Description is required", http.StatusBadRequest)
		return
	}

	work := &models.Work{
		ID:          uuid.New().String(),
		Description: req.Description,
		Status:      models.WorkStatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := h.storage.CreateWork(work); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create work: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(work)
}

// ListWork handles GET /api/work
func (h *Handler) ListWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if we should filter for in-progress only
	inProgressOnly := r.URL.Query().Get("status") == "in_progress"

	var works []*models.Work
	if inProgressOnly {
		works = h.storage.ListInProgressWorks()
	} else {
		works = h.storage.ListWorks()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(works)
}

// ListAgents handles GET /api/agents
func (h *Handler) ListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agents := h.storage.ListAgents()
	communications := h.storage.GetRecentCommunications(100)

	response := map[string]interface{}{
		"agents":         agents,
		"communications": communications,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListServices handles GET /api/services
func (h *Handler) ListServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if we should filter for active only
	activeOnly := r.URL.Query().Get("active") == "true"

	var services []*models.ServiceEndpoint
	if activeOnly {
		services = h.storage.ListActiveServices()
	} else {
		services = h.storage.ListServices()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

// GetServiceCosts handles GET /api/services/:id/costs
func (h *Handler) GetServiceCosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract service ID from URL path
	serviceID := extractServiceID(r.URL.Path)
	if serviceID == "" {
		http.Error(w, "Service ID is required", http.StatusBadRequest)
		return
	}

	service, err := h.storage.GetService(serviceID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Service not found: %v", err), http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"id":             service.ID,
		"name":           service.Name,
		"cost_type":      service.CostType,
		"cost_per_token": service.CostPerToken,
		"fixed_cost":     service.FixedCost,
		"tokens_used":    service.TokensUsed,
		"total_cost":     service.TotalCost,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// UpdateServiceCosts handles PUT /api/services/:id/costs
func (h *Handler) UpdateServiceCosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract service ID from URL path
	serviceID := extractServiceID(r.URL.Path)
	if serviceID == "" {
		http.Error(w, "Service ID is required", http.StatusBadRequest)
		return
	}

	var req models.UpdateServiceCostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Validate cost type
	if req.CostType != models.CostTypeFixed && req.CostType != models.CostTypeVariable {
		http.Error(w, "Invalid cost_type. Must be 'fixed' or 'variable'", http.StatusBadRequest)
		return
	}

	if err := h.storage.UpdateServiceCosts(serviceID, req.CostType, req.CostPerToken, req.FixedCost); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update service costs: %v", err), http.StatusInternalServerError)
		return
	}

	service, err := h.storage.GetService(serviceID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to retrieve service: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(service)
}

// GetPreferredServices handles GET /api/services/preferred
func (h *Handler) GetPreferredServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	services := h.storage.GetPreferredServices()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
}

// SimulateUsage handles POST /api/services/:id/usage (for testing/simulation)
func (h *Handler) SimulateUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serviceID := extractServiceID(r.URL.Path)
	if serviceID == "" {
		http.Error(w, "Service ID is required", http.StatusBadRequest)
		return
	}

	var req struct {
		TokensUsed int64 `json:"tokens_used"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.TokensUsed <= 0 {
		http.Error(w, "tokens_used must be positive", http.StatusBadRequest)
		return
	}

	if err := h.storage.RecordServiceUsage(serviceID, req.TokensUsed); err != nil {
		http.Error(w, fmt.Sprintf("Failed to record usage: %v", err), http.StatusInternalServerError)
		return
	}

	service, err := h.storage.GetService(serviceID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to retrieve service: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(service)
}

// Helper function to extract service ID from URL path
func extractServiceID(path string) string {
	// Expected paths:
	// /api/services/:id/costs
	// /api/services/:id/usage
	
	// Split by "/" and filter empty strings
	parts := strings.Split(path, "/")
	
	// Filter out empty parts
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	
	// Check if we have the expected structure: api/services/:id/...
	if len(filtered) >= 4 && filtered[0] == "api" && filtered[1] == "services" {
		return filtered[2]
	}
	return ""
}
