package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/loom/pkg/config"
)

// newTestServerWithAuth creates a server with auth enabled
func newTestServerWithAuth() *Server {
	return &Server{
		config: &config.Config{
			Security: config.SecurityConfig{
				EnableAuth: true,
			},
		},
		apiFailureLast: make(map[string]time.Time),
	}
}

// ============================================================
// Motivation handler deep tests (all testable with nil app
// because getMotivationRegistry() returns nil for nil app)
// ============================================================

func TestHandleMotivations_GET_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/motivations", nil)
	w := httptest.NewRecorder()
	s.handleMotivations(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleMotivations_POST_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/motivations", strings.NewReader(`{"name":"test"}`))
	w := httptest.NewRecorder()
	s.handleMotivations(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleMotivation_GET_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/motivations/m1", nil)
	w := httptest.NewRecorder()
	s.handleMotivation(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleMotivation_PUT_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/motivations/m1", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.handleMotivation(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleMotivation_PATCH_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/motivations/m1", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.handleMotivation(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleMotivation_DELETE_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/motivations/m1", nil)
	w := httptest.NewRecorder()
	s.handleMotivation(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleMotivation_Enable_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/motivations/m1/enable", nil)
	w := httptest.NewRecorder()
	s.handleMotivation(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleMotivation_Enable_MethodNotAllowed(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/motivations/m1/enable", nil)
	w := httptest.NewRecorder()
	s.handleMotivation(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleMotivation_Disable_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/motivations/m1/disable", nil)
	w := httptest.NewRecorder()
	s.handleMotivation(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleMotivation_Disable_MethodNotAllowed(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/motivations/m1/disable", nil)
	w := httptest.NewRecorder()
	s.handleMotivation(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleMotivation_Trigger_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/motivations/m1/trigger", nil)
	w := httptest.NewRecorder()
	s.handleMotivation(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleMotivation_Trigger_MethodNotAllowed(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/motivations/m1/trigger", nil)
	w := httptest.NewRecorder()
	s.handleMotivation(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleMotivation_UnknownMethod(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodHead, "/api/v1/motivations/m1", nil)
	w := httptest.NewRecorder()
	s.handleMotivation(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleListMotivations_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/motivations", nil)
	w := httptest.NewRecorder()
	s.handleListMotivations(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleGetMotivation_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/motivations/m1", nil)
	w := httptest.NewRecorder()
	s.handleGetMotivation(w, req, "m1")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleCreateMotivation_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/motivations", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.handleCreateMotivation(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleUpdateMotivation_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/motivations/m1", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	s.handleUpdateMotivation(w, req, "m1")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleDeleteMotivation_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/motivations/m1", nil)
	w := httptest.NewRecorder()
	s.handleDeleteMotivation(w, req, "m1")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleEnableMotivation_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/motivations/m1/enable", nil)
	w := httptest.NewRecorder()
	s.handleEnableMotivation(w, req, "m1")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleDisableMotivation_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/motivations/m1/disable", nil)
	w := httptest.NewRecorder()
	s.handleDisableMotivation(w, req, "m1")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleTriggerMotivation_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/motivations/m1/trigger", nil)
	w := httptest.NewRecorder()
	s.handleTriggerMotivation(w, req, "m1")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleMotivationHistory_GET_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/motivations/history", nil)
	w := httptest.NewRecorder()
	s.handleMotivationHistory(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleMotivationDefaults_POST_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/motivations/defaults", nil)
	w := httptest.NewRecorder()
	s.handleMotivationDefaults(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleIdleState_GET_NilApp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/motivations/idle", nil)
	w := httptest.NewRecorder()
	s.handleIdleState(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["is_system_idle"] != false {
		t.Error("expected is_system_idle=false")
	}
}

func TestHandleMotivationRoles_GET(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/motivations/roles", nil)
	w := httptest.NewRecorder()
	s.handleMotivationRoles(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["roles"] == nil {
		t.Error("expected roles in response")
	}
}

func TestGetMotivationRegistry_NilApp(t *testing.T) {
	s := newTestServer()
	if s.getMotivationRegistry() != nil {
		t.Error("expected nil registry for nil app")
	}
}

func TestGetMotivationEngine_NilApp(t *testing.T) {
	s := newTestServer()
	if s.getMotivationEngine() != nil {
		t.Error("expected nil engine for nil app")
	}
}

// ============================================================
// handleBead deep validation tests
// ============================================================

func TestHandleBead_Claim_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/beads/b1/claim", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleBead(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleBead_Claim_EmptyAgentID(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/beads/b1/claim", strings.NewReader(`{"agent_id":""}`))
	w := httptest.NewRecorder()
	s.handleBead(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleBead_PATCH_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/beads/b1", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleBead(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleBead_DefaultNotAllowed(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodHead, "/api/v1/beads/b1", nil)
	w := httptest.NewRecorder()
	s.handleBead(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// ============================================================
// handleDecision deep validation tests
// ============================================================

func TestHandleDecision_Decide_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/decisions/d1/decide", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleDecision(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleDecision_Decide_MissingDecision(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/decisions/d1/decide", strings.NewReader(`{"rationale":"test"}`))
	w := httptest.NewRecorder()
	s.handleDecision(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleDecision_Decide_MissingRationale(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/decisions/d1/decide", strings.NewReader(`{"decision":"yes"}`))
	w := httptest.NewRecorder()
	s.handleDecision(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleDecision_Decide_MethodNotAllowed(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/decisions/d1/decide", nil)
	w := httptest.NewRecorder()
	s.handleDecision(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleDecision_GET_MethodNotAllowed(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/decisions/d1", nil)
	w := httptest.NewRecorder()
	s.handleDecision(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

// ============================================================
// handleProject routing tests
// ============================================================

func TestHandleProject_SubEndpoints_Close(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/p1/close", nil)
	w := httptest.NewRecorder()
	s.handleProject(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleProject_SubEndpoints_Reopen(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/p1/reopen", nil)
	w := httptest.NewRecorder()
	s.handleProject(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleProject_SubEndpoints_Unknown(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/p1/unknown-endpoint", nil)
	w := httptest.NewRecorder()
	s.handleProject(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleProject_PUT_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/p1", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleProject(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ============================================================
// handleAgent routing tests
// ============================================================

func TestHandleAgent_SubEndpoints_Clone_MethodNotAllowed(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a1/clone", nil)
	w := httptest.NewRecorder()
	s.handleAgent(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleAgent_SubEndpoints_Unknown(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/a1/unknown", nil)
	w := httptest.NewRecorder()
	s.handleAgent(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleAgent_PUT_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agents/a1", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleAgent(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleAgents_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleAgents(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleAgents_POST_MissingPersonaName(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"test","project_id":"p1"}`))
	w := httptest.NewRecorder()
	s.handleAgents(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleAgents_POST_MissingProjectID(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(`{"name":"test","persona_name":"engineer"}`))
	w := httptest.NewRecorder()
	s.handleAgents(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProjects_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleProjects(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProjects_POST_MissingRequired(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"name":"test"}`))
	w := httptest.NewRecorder()
	s.handleProjects(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCloneAgent_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/a1/clone", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleCloneAgent(w, req, "a1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCloneAgent_POST_MissingNewPersonaName(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/a1/clone", strings.NewReader(`{"new_agent_name":"test"}`))
	w := httptest.NewRecorder()
	s.handleCloneAgent(w, req, "a1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandlePersona_PUT_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/personas/test", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handlePersona(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleBeads_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/beads", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleBeads(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleBeads_POST_MissingTitle(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/beads", strings.NewReader(`{"project_id":"p1"}`))
	w := httptest.NewRecorder()
	s.handleBeads(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCloseProject_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/close", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleCloseProject(w, req, "p1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCloseProject_POST_MissingAuthorID(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/close", strings.NewReader(`{"comment":"closing"}`))
	w := httptest.NewRecorder()
	s.handleCloseProject(w, req, "p1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleReopenProject_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/reopen", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleReopenProject(w, req, "p1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleReopenProject_POST_MissingAuthor(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/reopen", strings.NewReader(`{"comment":"reopening"}`))
	w := httptest.NewRecorder()
	s.handleReopenProject(w, req, "p1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProjectAgents_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/agents", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleProjectAgents(w, req, "p1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProjectAgents_POST_MissingAgentID(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/agents", strings.NewReader(`{"action":"assign"}`))
	w := httptest.NewRecorder()
	s.handleProjectAgents(w, req, "p1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProjectAgents_POST_InvalidAction(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/agents", strings.NewReader(`{"agent_id":"a1","action":"destroy"}`))
	w := httptest.NewRecorder()
	s.handleProjectAgents(w, req, "p1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProjectComments_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/comments", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleProjectComments(w, req, "p1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProjectComments_POST_MissingAuthorID(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/p1/comments", strings.NewReader(`{"content":"test"}`))
	w := httptest.NewRecorder()
	s.handleProjectComments(w, req, "p1")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleConfig_PUT_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleConfig(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProvider_PUT_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/providers/p1", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleProvider(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProvider_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers/p1", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleProvider(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleFileLock_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/file-locks", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleFileLocks(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleFileLock_POST_MissingFilePath(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/file-locks", strings.NewReader(`{"agent_id":"a1","project_id":"p1"}`))
	w := httptest.NewRecorder()
	s.handleFileLocks(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSelectProvider_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/routing/select", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleSelectProvider(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleWork_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/work", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleWork(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleRepl_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/repl", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleRepl(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleRepl_POST_MissingCommand(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/repl", strings.NewReader(`{"agent_id":"a1"}`))
	w := httptest.NewRecorder()
	s.handleRepl(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleBootstrapProject_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bootstrap", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleBootstrapProject(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleAutoFileBug_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auto-file", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.HandleAutoFileBug(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleAutoFileBug_CircuitOpen(t *testing.T) {
	s := newTestServer()
	s.autoFileCircuitOpen = true
	s.autoFileCircuitOpenAt = time.Now()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auto-file", strings.NewReader(`{"error_type":"panic","error_message":"test"}`))
	w := httptest.NewRecorder()
	s.HandleAutoFileBug(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestHandleChatCompletion_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleChatCompletion(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleChatCompletion_POST_MissingProviderID(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hi"}]}`))
	w := httptest.NewRecorder()
	s.handleChatCompletion(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleStreamChatCompletion_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions/stream", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleStreamChatCompletion(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandlePairChat_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pair", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handlePairChat(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandlePairChat_POST_MissingMessage(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pair", strings.NewReader(`{"provider_id":"openai","agent_id":"a1"}`))
	w := httptest.NewRecorder()
	s.handlePairChat(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandlePairChat_POST_MissingProviderID(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pair", strings.NewReader(`{"message":"hello","agent_id":"a1"}`))
	w := httptest.NewRecorder()
	s.handlePairChat(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleExecuteCommand_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/commands", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.HandleExecuteCommand(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleExecuteCommand_POST_MissingCommand(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/commands", strings.NewReader(`{"agent_id":"a1"}`))
	w := httptest.NewRecorder()
	s.HandleExecuteCommand(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProviders_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleProviders(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProviders_POST_MissingFields(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/providers", strings.NewReader(`{"type":"openai"}`))
	w := httptest.NewRecorder()
	s.handleProviders(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCacheAnalysis_InvalidTimeWindow(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cache/analysis?time_window=not-a-duration", nil)
	w := httptest.NewRecorder()
	s.handleCacheAnalysis(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCacheAnalysis_InvalidMinSavings(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cache/analysis?min_savings=not-a-number", nil)
	w := httptest.NewRecorder()
	s.handleCacheAnalysis(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCacheAnalysis_InvalidAutoEnable(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cache/analysis?auto_enable=not-a-bool", nil)
	w := httptest.NewRecorder()
	s.handleCacheAnalysis(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleCacheOptimize_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cache/optimize", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleCacheOptimize(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProjectFiles_EmptyOps(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/p1/files", nil)
	w := httptest.NewRecorder()
	s.handleProjectFiles(w, req, "p1", []string{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleProjectFiles_UnknownOp(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/p1/files/destroy", nil)
	w := httptest.NewRecorder()
	s.handleProjectFiles(w, req, "p1", []string{"destroy"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleFederationSync_POST_InvalidJSON(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/sync", strings.NewReader(`{invalid}`))
	w := httptest.NewRecorder()
	s.handleFederationSync(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRespondJSON_UnmarshalableData(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"ch": make(chan int)})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for unmarshalable data, got %d", w.Code)
	}
}

func TestRecordAPIFailure_Tracks(t *testing.T) {
	s := &Server{
		config:         &config.Config{},
		apiFailureLast: make(map[string]time.Time),
	}
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	s.recordAPIFailure(req, http.StatusInternalServerError)
	// With nil app, failure recording is skipped (early return)
	s.apiFailureMu.Lock()
	if len(s.apiFailureLast) != 0 {
		t.Error("expected no failures recorded when app is nil")
	}
	s.apiFailureMu.Unlock()
}

func TestServerDefaultProjectID_NilApp(t *testing.T) {
	s := &Server{config: &config.Config{}}
	id := s.defaultProjectID()
	if id != "" {
		t.Errorf("expected empty project ID when app is nil, got %s", id)
	}
}

func TestAuthMiddleware_WithUserHeader(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			EnableAuth: false,
		},
	}
	s := NewServer(nil, nil, nil, cfg)

	handler := s.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := s.getUserFromContext(r)
		if user != nil {
			w.Write([]byte(user.ID))
		} else {
			w.Write([]byte("no-user"))
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-User-ID", "test-user")
	req.Header.Set("X-Role", "admin")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// When auth is disabled, middleware overrides all headers with "admin"
	if w.Body.String() != "admin" {
		t.Errorf("expected admin (auth disabled overrides headers), got %s", w.Body.String())
	}
}

func TestHandleGitHubWebhook_MissingEventType(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			WebhookSecret: "",
		},
	}
	s := &Server{config: cfg, apiFailureLast: make(map[string]time.Time)}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(`{"action":"opened"}`))
	w := httptest.NewRecorder()
	s.handleGitHubWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleGitHubWebhook_InvalidJSON(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			WebhookSecret: "",
		},
	}
	s := &Server{config: cfg, apiFailureLast: make(map[string]time.Time)}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github", strings.NewReader(`{invalid}`))
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	s.handleGitHubWebhook(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestNewServer_WithAuthManager(t *testing.T) {
	cfg := &config.Config{
		Security: config.SecurityConfig{
			EnableAuth: true,
		},
	}
	s := NewServer(nil, nil, nil, cfg)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if !s.config.Security.EnableAuth {
		t.Error("expected auth to be enabled")
	}
}

func TestHandleBeads_DELETE_MethodNotAllowed(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/beads", nil)
	w := httptest.NewRecorder()
	s.handleBeads(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
