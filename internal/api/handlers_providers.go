package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	internalmodels "github.com/jordanhubbard/loom/internal/models"
)

// ProviderRequest is a request wrapper for provider registration with API key
type ProviderRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Endpoint    string `json:"endpoint"`
	APIKey      string `json:"api_key"`
	Model       string `json:"model"`
	Description string `json:"description"`
}

// handleProviders handles GET/POST /api/v1/providers
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		providers, err := s.app.ListProviders()
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, providers)

	case http.MethodPost:
		var req ProviderRequest
		if err := s.parseJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		provider := &internalmodels.Provider{
			ID:          req.ID,
			Name:        req.Name,
			Type:        req.Type,
			Endpoint:    req.Endpoint,
			Model:       req.Model,
			Description: req.Description,
		}

		// Store API key if provided
		apiKey := req.APIKey
		if apiKey != "" {
			keyID := fmt.Sprintf("%s-api-key", req.ID)
			if s.keyManager != nil && s.keyManager.IsUnlocked() {
				if err := s.keyManager.StoreKey(keyID, req.Name, fmt.Sprintf("API key for %s", req.Name), apiKey); err != nil {
					s.respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to store API key: %v", err))
					return
				}
				provider.KeyID = keyID
			}
			provider.RequiresKey = true
		}

		created, err := s.app.RegisterProvider(context.Background(), provider, apiKey)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.respondJSON(w, http.StatusCreated, created)

	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// handleProvider handles GET/DELETE /api/v1/providers/{id} and GET /api/v1/providers/{id}/models
func (s *Server) handleProvider(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/providers/")
	parts := strings.Split(path, "/")
	providerID := parts[0]

	if providerID == "" {
		s.respondError(w, http.StatusBadRequest, "Missing provider id")
		return
	}

	if len(parts) > 1 && parts[1] == "models" {
		if r.Method != http.MethodGet {
			s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		models, err := s.app.GetProviderModels(context.Background(), providerID)
		if err != nil {
			s.respondError(w, http.StatusBadGateway, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, map[string]interface{}{"models": models})
		return
	}
	if len(parts) > 1 && parts[1] == "negotiate" {
		if r.Method != http.MethodPost {
			s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		updated, err := s.app.NegotiateProviderModel(context.Background(), providerID)
		if err != nil {
			s.respondError(w, http.StatusBadGateway, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, updated)
		return
	}

	switch r.Method {
	case http.MethodGet:
		providers, err := s.app.ListProviders()
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, p := range providers {
			if p.ID == providerID {
				s.respondJSON(w, http.StatusOK, p)
				return
			}
		}
		s.respondError(w, http.StatusNotFound, "Provider not found")

	case http.MethodDelete:
		if err := s.app.DeleteProvider(context.Background(), providerID); err != nil {
			s.respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodPut:
		var req internalmodels.Provider
		if err := s.parseJSON(r, &req); err != nil {
			s.respondError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		req.ID = providerID
		updated, err := s.app.UpdateProvider(context.Background(), &req)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.respondJSON(w, http.StatusOK, updated)

	default:
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}
