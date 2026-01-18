package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jordanhubbard/arbiter/internal/arbiter"
	"github.com/jordanhubbard/arbiter/pkg/config"
)

// Server represents the HTTP API server
type Server struct {
	arbiter *arbiter.Arbiter
	config  *config.Config
}

// NewServer creates a new API server
func NewServer(arb *arbiter.Arbiter, cfg *config.Config) *Server {
	return &Server{
		arbiter: arb,
		config:  cfg,
	}
}

// SetupRoutes configures HTTP routes
func (s *Server) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Serve static files
	if s.config.WebUI.Enabled {
		fs := http.FileServer(http.Dir(s.config.WebUI.StaticPath))
		mux.Handle("/static/", http.StripPrefix("/static/", fs))
		
		// Serve index.html at root
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				http.ServeFile(w, r, s.config.WebUI.StaticPath+"/index.html")
			} else {
				http.NotFound(w, r)
			}
		})
	}

	// Serve OpenAPI spec
	mux.HandleFunc("/api/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./api/openapi.yaml")
	})

	// Health check
	mux.HandleFunc("/api/v1/health", s.handleHealth)

	// Personas
	mux.HandleFunc("/api/v1/personas", s.handlePersonas)
	mux.HandleFunc("/api/v1/personas/", s.handlePersona)

	// Agents
	mux.HandleFunc("/api/v1/agents", s.handleAgents)
	mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	// Projects
	mux.HandleFunc("/api/v1/projects", s.handleProjects)
	mux.HandleFunc("/api/v1/projects/", s.handleProject)

	// Beads
	mux.HandleFunc("/api/v1/beads", s.handleBeads)
	mux.HandleFunc("/api/v1/beads/", s.handleBead)

	// Decisions
	mux.HandleFunc("/api/v1/decisions", s.handleDecisions)
	mux.HandleFunc("/api/v1/decisions/", s.handleDecision)

	// File locks
	mux.HandleFunc("/api/v1/file-locks", s.handleFileLocks)
	mux.HandleFunc("/api/v1/file-locks/", s.handleFileLock)

	// Work graph
	mux.HandleFunc("/api/v1/work-graph", s.handleWorkGraph)

	// Events (real-time updates and event bus)
	mux.HandleFunc("/api/v1/events/stream", s.handleEventStream)
	mux.HandleFunc("/api/v1/events/stats", s.handleGetEventStats)
	mux.HandleFunc("/api/v1/events", s.handleGetEvents)      // GET for history
	// POST /api/v1/events for publishing is available but should be restricted

	// Apply middleware
	handler := s.loggingMiddleware(mux)
	handler = s.corsMiddleware(handler)
	handler = s.authMiddleware(handler)

	return handler
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Middleware

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simple logging - in production, use a proper logger
		// log.Printf("%s %s %s", r.Method, r.URL.Path, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware handles CORS headers
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		if len(s.config.Security.AllowedOrigins) > 0 {
			origin := r.Header.Get("Origin")
			for _, allowedOrigin := range s.config.Security.AllowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
					break
				}
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")

		// Handle preflight
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// authMiddleware handles authentication
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health check, static files, root, and OpenAPI spec
		if r.URL.Path == "/api/v1/health" || 
		   r.URL.Path == "/" || 
		   r.URL.Path == "/api/openapi.yaml" ||
		   strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		// Skip auth if disabled
		if !s.config.Security.EnableAuth {
			next.ServeHTTP(w, r)
			return
		}

		// Check API key
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			http.Error(w, "Missing API key", http.StatusUnauthorized)
			return
		}

		// Validate API key
		valid := false
		for _, key := range s.config.Security.APIKeys {
			if key == apiKey {
				valid = true
				break
			}
		}

		if !valid {
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Helper functions

// respondJSON writes a JSON response
func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError writes an error response
func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	s.respondJSON(w, status, map[string]string{"error": message})
}

// parseJSON parses JSON request body
func (s *Server) parseJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// extractID extracts ID from URL path
func (s *Server) extractID(path, prefix string) string {
	// Remove prefix and any trailing slash
	id := strings.TrimPrefix(path, prefix)
	id = strings.TrimPrefix(id, "/")
	id = strings.TrimSuffix(id, "/")
	
	// Handle sub-paths (e.g., /api/v1/beads/123/claim)
	parts := strings.Split(id, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	
	return id
}
