package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jordanhubbard/loom/internal/loom"
	"github.com/jordanhubbard/loom/internal/analytics"
	"github.com/jordanhubbard/loom/internal/auth"
	"github.com/jordanhubbard/loom/internal/cache"
	"github.com/jordanhubbard/loom/internal/files"
	"github.com/jordanhubbard/loom/internal/keymanager"
	"github.com/jordanhubbard/loom/internal/logging"
	"github.com/jordanhubbard/loom/internal/metrics"
	"github.com/jordanhubbard/loom/pkg/config"
	"github.com/jordanhubbard/loom/pkg/models"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server represents the HTTP API server
type Server struct {
	app             *loom.Loom
	keyManager      *keymanager.KeyManager
	authManager     *auth.Manager
	analyticsLogger *analytics.Logger
	logManager      *logging.Manager
	cache           *cache.Cache
	config          *config.Config
	fileManager     *files.Manager
	metrics         *metrics.Metrics
	apiFailureMu    sync.Mutex
	apiFailureLast  map[string]time.Time

	// Circuit breaker for auto-filing API failures as beads.
	// Prevents cascading failures when the bead subsystem itself is broken.
	autoFileCBMu          sync.Mutex
	autoFileConsecFails   int
	autoFileLastFail      time.Time
	autoFileCircuitOpen   bool
	autoFileCircuitOpenAt time.Time
}

// NewServer creates a new API server
func NewServer(arb *loom.Loom, km *keymanager.KeyManager, am *auth.Manager, cfg *config.Config) *Server {
	// Initialize analytics logger with default privacy config
	var analyticsLogger *analytics.Logger
	if arb != nil && arb.GetDatabase() != nil {
		storage, err := analytics.NewDatabaseStorage(arb.GetDatabase().DB())
		if err == nil {
			analyticsLogger = analytics.NewLogger(storage, analytics.DefaultPrivacyConfig())
		}
	}

	// Initialize logging manager
	var logMgr *logging.Manager
	if arb != nil && arb.GetDatabase() != nil {
		logMgr = logging.NewManager(arb.GetDatabase().DB())
	}

	// Initialize cache with config
	var responseCache *cache.Cache
	if cfg != nil && cfg.Cache.Enabled {
		cacheConfig := &cache.Config{
			Enabled:       cfg.Cache.Enabled,
			DefaultTTL:    cfg.Cache.DefaultTTL,
			MaxSize:       cfg.Cache.MaxSize,
			MaxMemoryMB:   cfg.Cache.MaxMemoryMB,
			CleanupPeriod: cfg.Cache.CleanupPeriod,
		}
		// Use defaults if not specified
		if cacheConfig.DefaultTTL == 0 {
			cacheConfig.DefaultTTL = 1 * time.Hour
		}
		if cacheConfig.MaxSize == 0 {
			cacheConfig.MaxSize = 10000
		}
		if cacheConfig.CleanupPeriod == 0 {
			cacheConfig.CleanupPeriod = 5 * time.Minute
		}

		// Use Redis backend if configured, fallback to memory
		if cfg.Cache.Backend == "redis" && cfg.Cache.RedisURL != "" {
			redisCache, err := cache.NewRedisCache(cfg.Cache.RedisURL, cacheConfig)
			if err != nil {
				// Redis failed - fallback to in-memory cache with warning
				fmt.Printf("[WARN] Redis cache initialization failed: %v, falling back to in-memory cache\n", err)
				responseCache = cache.New(cacheConfig)
			} else {
				// Wrap Redis cache to match Cache interface
				responseCache = cache.NewFromRedis(redisCache)
			}
		} else {
			responseCache = cache.New(cacheConfig)
		}
	}

	var fileManager *files.Manager
	if arb != nil {
		fileManager = files.NewManager(arb.GetGitOpsManager())
	}

	// Initialize Prometheus metrics
	promMetrics := metrics.NewMetrics()

	return &Server{
		app:             arb,
		keyManager:      km,
		authManager:     am,
		analyticsLogger: analyticsLogger,
		logManager:      logMgr,
		cache:           responseCache,
		config:          cfg,
		fileManager:     fileManager,
		metrics:         promMetrics,
		apiFailureLast:  make(map[string]time.Time),
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

	// Prometheus metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Auth endpoints
	authHandlers := auth.NewHandlers(s.authManager)
	mux.HandleFunc("/api/v1/auth/login", authHandlers.HandleLogin)
	mux.HandleFunc("/api/v1/auth/refresh", authHandlers.HandleRefreshToken)
	mux.HandleFunc("/api/v1/auth/change-password", authHandlers.HandleChangePassword)
	mux.HandleFunc("/api/v1/auth/api-keys", authHandlers.HandleCreateAPIKey)
	mux.HandleFunc("/api/v1/auth/me", authHandlers.HandleGetCurrentUser)
	mux.HandleFunc("/api/v1/auth/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			authHandlers.HandleCreateUser(w, r)
		case http.MethodGet:
			authHandlers.HandleListUsers(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Personas
	mux.HandleFunc("/api/v1/personas", s.handlePersonas)
	mux.HandleFunc("/api/v1/personas/", s.handlePersona)

	// Agents
	mux.HandleFunc("/api/v1/agents", s.handleAgents)
	mux.HandleFunc("/api/v1/agents/", s.handleAgent)

	// Projects (includes /projects/{id}/files/*)
	mux.HandleFunc("/api/v1/projects/bootstrap", s.handleBootstrapProject)
	mux.HandleFunc("/api/v1/projects", s.handleProjects)
	mux.HandleFunc("/api/v1/projects/", s.handleProject)

	// Org Charts
	mux.HandleFunc("/api/v1/org-charts/", s.handleOrgChart)

	// Beads
	mux.HandleFunc("/api/v1/beads", s.handleBeads)
	mux.HandleFunc("/api/v1/beads/", s.handleBead)

	// Federation
	mux.HandleFunc("/api/v1/federation/status", s.handleFederationStatus)
	mux.HandleFunc("/api/v1/federation/sync", s.handleFederationSync)

	// Comments (must be registered before other /beads/ routes to avoid conflicts)
	// Note: This is already handled by handleBead which routes to specific sub-paths
	// but we also need a dedicated handler for comments
	// Actually, we'll use a pattern that matches /beads/{id}/comments
	mux.HandleFunc("/api/v1/comments/", s.handleComment)

	// Conversations
	mux.HandleFunc("/api/v1/conversations", s.handleConversationsList)
	mux.HandleFunc("/api/v1/conversations/", s.handleConversation)

	// Decisions
	mux.HandleFunc("/api/v1/decisions", s.handleDecisions)
	mux.HandleFunc("/api/v1/decisions/", s.handleDecision)

	// File locks
	mux.HandleFunc("/api/v1/file-locks", s.handleFileLocks)
	mux.HandleFunc("/api/v1/file-locks/", s.handleFileLock)

	// Work graph
	mux.HandleFunc("/api/v1/work-graph", s.handleWorkGraph)

	// Providers
	mux.HandleFunc("/api/v1/providers", s.handleProviders)
	mux.HandleFunc("/api/v1/providers/", s.handleProvider)
	mux.HandleFunc("/api/v1/routing/select", s.handleSelectProvider)
	mux.HandleFunc("/api/v1/routing/policies", s.handleGetRoutingPolicies)

	// Models
	mux.HandleFunc("/api/v1/models/recommended", s.handleRecommendedModels)

	// System
	mux.HandleFunc("/api/v1/system/status", s.handleSystemStatus)

	// Work (non-bead prompts)
	mux.HandleFunc("/api/v1/work", s.handleWork)

	// CEO REPL
	mux.HandleFunc("/api/v1/repl", s.handleRepl)

	// Shell command execution
	mux.HandleFunc("/api/v1/commands/execute", s.HandleExecuteCommand)
	mux.HandleFunc("/api/v1/commands", s.HandleGetCommandLogs)
	mux.HandleFunc("/api/v1/commands/", s.HandleGetCommandLogs)

	// Auto-filed bug reports
	mux.HandleFunc("/api/v1/beads/auto-file", s.HandleAutoFileBug)

	// Logging endpoints
	mux.HandleFunc("/api/v1/logs/recent", s.HandleLogsRecent)
	mux.HandleFunc("/api/v1/logs/stream", s.HandleLogsStream)
	mux.HandleFunc("/api/v1/logs/export", s.HandleLogsExport)

	// Chat completions (with streaming support)
	mux.HandleFunc("/api/v1/chat/completions/stream", s.handleStreamChatCompletion)
	mux.HandleFunc("/api/v1/chat/completions", s.handleChatCompletion)

	// Pair-programming chat (SSE streaming with conversation persistence)
	mux.HandleFunc("/api/v1/pair", s.handlePairChat)

	// Git operations
	mux.HandleFunc("/api/v1/projects/git/sync", s.handleGitSync)
	mux.HandleFunc("/api/v1/projects/git/commit", s.handleGitCommit)
	mux.HandleFunc("/api/v1/projects/git/push", s.handleGitPush)
	mux.HandleFunc("/api/v1/projects/git/status", s.handleGitStatus)

	// Analytics and cost tracking
	mux.HandleFunc("/api/v1/analytics/logs", s.handleGetLogs)
	mux.HandleFunc("/api/v1/analytics/stats", s.handleGetLogStats)
	mux.HandleFunc("/api/v1/analytics/export", s.handleExportLogs)
	mux.HandleFunc("/api/v1/analytics/export-stats", s.handleExportStats)
	mux.HandleFunc("/api/v1/analytics/costs", s.handleGetCostReport)
	mux.HandleFunc("/api/v1/analytics/batching", s.handleGetBatchingRecommendations)

	// Cache management
	mux.HandleFunc("/api/v1/cache/stats", s.handleGetCacheStats)
	mux.HandleFunc("/api/v1/cache/config", s.handleGetCacheConfig)
	mux.HandleFunc("/api/v1/cache/clear", s.handleClearCache)
	mux.HandleFunc("/api/v1/cache/invalidate", s.handleInvalidateCache)

	// Cache analysis and optimization
	mux.HandleFunc("/api/v1/cache/analysis", s.handleCacheAnalysis)
	mux.HandleFunc("/api/v1/cache/opportunities", s.handleCacheOpportunities)
	mux.HandleFunc("/api/v1/cache/optimize", s.handleCacheOptimize)
	mux.HandleFunc("/api/v1/cache/recommendations", s.handleCacheRecommendations)

	// Pattern analysis routes
	mux.HandleFunc("/api/v1/patterns/analysis", s.handlePatternAnalysis)
	mux.HandleFunc("/api/v1/patterns/expensive", s.handleExpensivePatterns)
	mux.HandleFunc("/api/v1/patterns/anomalies", s.handleAnomalies)
	mux.HandleFunc("/api/v1/optimizations", s.handleOptimizations)
	mux.HandleFunc("/api/v1/prompts/analysis", s.handlePromptAnalysis)
	mux.HandleFunc("/api/v1/prompts/optimizations", s.handlePromptOptimizations)
	mux.HandleFunc("/api/v1/optimizations/substitutions", s.handleSubstitutions)
	mux.HandleFunc("/api/v1/optimizations/", s.handleOptimizationActions)

	// Health check endpoints
	mux.HandleFunc("/health", s.handleHealthDetail)      // Detailed health
	mux.HandleFunc("/health/live", s.handleHealthLive)   // Liveness probe
	mux.HandleFunc("/health/ready", s.handleHealthReady) // Readiness probe

	// Configuration
	mux.HandleFunc("/api/v1/config", s.handleConfig)
	mux.HandleFunc("/api/v1/config/export.yaml", s.handleConfigExportYAML)
	mux.HandleFunc("/api/v1/config/import.yaml", s.handleConfigImportYAML)

	// Events (real-time updates and event bus)
	mux.HandleFunc("/api/v1/events/stream", s.handleEventStream)
	mux.HandleFunc("/api/v1/events/stats", s.handleGetEventStats)
	mux.HandleFunc("/api/v1/events", s.handleGetEvents) // GET for history
	// POST /api/v1/events for publishing is available but should be restricted

	// Activity feed
	mux.HandleFunc("/api/v1/activity-feed", s.handleGetActivityFeed)
	mux.HandleFunc("/api/v1/activity-feed/stream", s.handleActivityFeedStream)

	// Notifications
	mux.HandleFunc("/api/v1/notifications", s.handleGetNotifications)
	mux.HandleFunc("/api/v1/notifications/stream", s.handleNotificationStream)
	mux.HandleFunc("/api/v1/notifications/", s.handleNotificationActions)
	mux.HandleFunc("/api/v1/notifications/mark-all-read", s.handleMarkAllRead)
	mux.HandleFunc("/api/v1/notifications/preferences", s.handleNotificationPreferences)

	// Motivations
	mux.HandleFunc("/api/v1/motivations", s.handleMotivations)
	mux.HandleFunc("/api/v1/motivations/", s.handleMotivation)
	mux.HandleFunc("/api/v1/motivations/history", s.handleMotivationHistory)
	mux.HandleFunc("/api/v1/motivations/idle", s.handleIdleState)
	mux.HandleFunc("/api/v1/motivations/roles", s.handleMotivationRoles)
	mux.HandleFunc("/api/v1/motivations/defaults", s.handleMotivationDefaults)

	// Workflows (Phase 4 & 5)
	mux.HandleFunc("/api/v1/workflows", s.handleWorkflows)
	mux.HandleFunc("/api/v1/workflows/", s.handleWorkflow)
	mux.HandleFunc("/api/v1/workflows/executions", s.handleWorkflowExecutions)
	mux.HandleFunc("/api/v1/workflows/analytics", s.handleWorkflowAnalytics)
	mux.HandleFunc("/api/v1/beads/workflow", s.handleBeadWorkflow)

	// Webhooks (external event integration)
	mux.HandleFunc("/api/v1/webhooks/github", s.handleGitHubWebhook)
	mux.HandleFunc("/api/v1/webhooks/status", s.handleWebhookStatus)

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
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		s.recordAPIFailure(r, recorder.statusCode)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher to support streaming
func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.statusCode == 0 {
		r.statusCode = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (s *Server) recordAPIFailure(r *http.Request, statusCode int) {
	if statusCode < http.StatusInternalServerError {
		return
	}
	if r == nil || r.URL == nil || s.app == nil {
		return
	}

	// Don't auto-file failures on the auto-file endpoint itself (prevents loops)
	if strings.HasSuffix(r.URL.Path, "/auto-file") {
		return
	}

	// Check circuit breaker before attempting to file
	if s.isAutoFileCircuitOpen() {
		return
	}

	key := fmt.Sprintf("%s %s %d", r.Method, r.URL.Path, statusCode)
	if s.shouldThrottleFailure(key, 2*time.Minute) {
		return
	}

	// Check bead subsystem health before filing
	if !s.isBeadSubsystemHealthy() {
		return
	}

	projectID := s.defaultProjectID()
	if projectID == "" {
		return
	}

	title := fmt.Sprintf("P0 - API failure %s", key)
	description := fmt.Sprintf(
		"API request failed\n\nMethod: %s\nPath: %s\nStatus: %d\nQuery: %s\nRemote: %s\nUser: %s\nTimestamp: %s\n",
		r.Method,
		r.URL.Path,
		statusCode,
		r.URL.RawQuery,
		r.RemoteAddr,
		auth.GetUserIDFromRequest(r),
		time.Now().UTC().Format(time.RFC3339),
	)

	_, err := s.app.CreateBead(title, description, models.BeadPriority(0), "task", projectID)
	s.recordAutoFileResult(err)
}

const (
	autoFileCBMaxFails   = 3               // Trip circuit after 3 consecutive failures
	autoFileCBResetAfter = 5 * time.Minute // Re-attempt after 5 minutes
)

func (s *Server) isAutoFileCircuitOpen() bool {
	s.autoFileCBMu.Lock()
	defer s.autoFileCBMu.Unlock()

	if !s.autoFileCircuitOpen {
		return false
	}
	// Half-open: allow retry after cooldown
	if time.Since(s.autoFileCircuitOpenAt) > autoFileCBResetAfter {
		s.autoFileCircuitOpen = false
		s.autoFileConsecFails = 0
		return false
	}
	return true
}

func (s *Server) recordAutoFileResult(err error) {
	s.autoFileCBMu.Lock()
	defer s.autoFileCBMu.Unlock()

	if err != nil {
		s.autoFileConsecFails++
		s.autoFileLastFail = time.Now()
		if s.autoFileConsecFails >= autoFileCBMaxFails {
			s.autoFileCircuitOpen = true
			s.autoFileCircuitOpenAt = time.Now()
			fmt.Printf("[WARN] Auto-file circuit breaker tripped after %d consecutive failures\n", s.autoFileConsecFails)
		}
	} else {
		s.autoFileConsecFails = 0
		s.autoFileCircuitOpen = false
	}
}

func (s *Server) isBeadSubsystemHealthy() bool {
	if s.app == nil {
		return false
	}
	bm := s.app.GetBeadsManager()
	if bm == nil {
		return false
	}
	// Quick check: can we list beads without error?
	_, err := bm.ListBeads(nil)
	return err == nil
}

func (s *Server) shouldThrottleFailure(key string, window time.Duration) bool {
	if key == "" {
		return true
	}
	now := time.Now()
	s.apiFailureMu.Lock()
	defer s.apiFailureMu.Unlock()
	if last, ok := s.apiFailureLast[key]; ok && now.Sub(last) < window {
		return true
	}
	s.apiFailureLast[key] = now
	return false
}

func (s *Server) defaultProjectID() string {
	if s.app == nil {
		return ""
	}
	if pm := s.app.GetProjectManager(); pm != nil {
		if project, err := pm.GetProject("loom"); err == nil && project != nil {
			return project.ID
		}
		if projects := pm.ListProjects(); len(projects) > 0 && projects[0] != nil {
			return projects[0].ID
		}
	}
	return ""
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")

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
		// Skip auth for health check endpoints (all variants for monitoring/probes)
		if r.URL.Path == "/api/v1/health" ||
			r.URL.Path == "/health" ||
			r.URL.Path == "/health/live" ||
			r.URL.Path == "/health/ready" ||
			r.URL.Path == "/api/v1/auth/login" ||
			r.URL.Path == "/api/v1/auth/refresh" ||
			r.URL.Path == "/" ||
			r.URL.Path == "/api/openapi.yaml" ||
			r.URL.Path == "/api/v1/events/stream" ||
			r.URL.Path == "/api/v1/chat/completions/stream" ||
			r.URL.Path == "/api/v1/chat/completions" ||
			r.URL.Path == "/api/v1/pair" ||
			strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		// Skip auth if disabled
		if !s.config.Security.EnableAuth || s.authManager == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Apply JWT/API key auth
		s.authManager.Middleware("")(next).ServeHTTP(w, r)
	})
}

// Helper functions

// getUserFromContext extracts the user from request headers (set by auth middleware)
func (s *Server) getUserFromContext(r *http.Request) *auth.User {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		return nil
	}

	username := r.Header.Get("X-Username")
	role := r.Header.Get("X-Role")

	return &auth.User{
		ID:       userID,
		Username: username,
		Role:     role,
		IsActive: true,
	}
}

// respondJSON writes a JSON response
func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
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

// GetMetrics returns the Prometheus metrics instance
func (s *Server) GetMetrics() *metrics.Metrics {
	return s.metrics
}
