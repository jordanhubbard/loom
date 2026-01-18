package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jordanhubbard/arbiter/internal/models"
	"github.com/jordanhubbard/arbiter/internal/storage"
)

func TestCreateWork(t *testing.T) {
	store := storage.New()
	handler := NewHandler(store)

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
	}{
		{
			name:           "valid work creation",
			requestBody:    models.CreateWorkRequest{Description: "Test work"},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "empty description",
			requestBody:    models.CreateWorkRequest{Description: ""},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid json",
			requestBody:    "invalid",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/work", bytes.NewBuffer(body))
			w := httptest.NewRecorder()

			handler.CreateWork(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestListWork(t *testing.T) {
	store := storage.New()
	handler := NewHandler(store)

	// Create some test work
	work := &models.Work{
		ID:          "test-1",
		Description: "Test work",
		Status:      models.WorkStatusPending,
	}
	store.CreateWork(work)

	req := httptest.NewRequest(http.MethodGet, "/api/work", nil)
	w := httptest.NewRecorder()

	handler.ListWork(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var works []*models.Work
	if err := json.NewDecoder(w.Body).Decode(&works); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(works) != 1 {
		t.Errorf("expected 1 work, got %d", len(works))
	}
}

func TestListServices(t *testing.T) {
	store := storage.New()
	handler := NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	w := httptest.NewRecorder()

	handler.ListServices(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var services []*models.ServiceEndpoint
	if err := json.NewDecoder(w.Body).Decode(&services); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should have default services
	if len(services) < 1 {
		t.Errorf("expected at least 1 service, got %d", len(services))
	}
}

func TestUpdateServiceCosts(t *testing.T) {
	store := storage.New()
	handler := NewHandler(store)

	tests := []struct {
		name           string
		serviceID      string
		requestBody    models.UpdateServiceCostRequest
		expectedStatus int
	}{
		{
			name:      "update to fixed cost",
			serviceID: "ollama-local",
			requestBody: models.UpdateServiceCostRequest{
				CostType:  models.CostTypeFixed,
				FixedCost: floatPtr(0),
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "update to variable cost",
			serviceID: "ollama-local",
			requestBody: models.UpdateServiceCostRequest{
				CostType:     models.CostTypeVariable,
				CostPerToken: floatPtr(0.00003),
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "invalid service ID",
			serviceID: "nonexistent",
			requestBody: models.UpdateServiceCostRequest{
				CostType:  models.CostTypeFixed,
				FixedCost: floatPtr(0),
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPut, "/api/services/"+tt.serviceID+"/costs", bytes.NewBuffer(body))
			w := httptest.NewRecorder()

			handler.UpdateServiceCosts(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d. Response: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}

func TestGetPreferredServices(t *testing.T) {
	store := storage.New()
	handler := NewHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/services/preferred", nil)
	w := httptest.NewRecorder()

	handler.GetPreferredServices(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var services []*models.ServiceEndpoint
	if err := json.NewDecoder(w.Body).Decode(&services); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check that fixed-cost services come first
	foundVariable := false
	for _, service := range services {
		if service.CostType == models.CostTypeVariable {
			foundVariable = true
		} else if foundVariable && service.CostType == models.CostTypeFixed {
			t.Error("fixed-cost service found after variable-cost service")
		}
	}
}

func floatPtr(f float64) *float64 {
	return &f
}
