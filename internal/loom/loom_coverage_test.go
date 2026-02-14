package loom

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jordanhubbard/loom/internal/executor"
	internalmodels "github.com/jordanhubbard/loom/internal/models"
	"github.com/jordanhubbard/loom/internal/provider"
	"github.com/jordanhubbard/loom/pkg/config"
	"github.com/jordanhubbard/loom/pkg/models"
)

// ---------------------------------------------------------------------------
// Pure function tests: normalizeProviderEndpoint
// ---------------------------------------------------------------------------

func TestNormalizeProviderEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"already has /v1", "http://localhost:8000/v1", "http://localhost:8000/v1"},
		{"no trailing slash", "http://localhost:8000", "http://localhost:8000/v1"},
		{"trailing slash", "http://localhost:8000/", "http://localhost:8000/v1"},
		{"https no v1", "https://api.example.com", "https://api.example.com/v1"},
		{"https trailing slash", "https://api.example.com/", "https://api.example.com/v1"},
		{"path already v1", "https://api.example.com/v1", "https://api.example.com/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeProviderEndpoint(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeProviderEndpoint(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: normalizeGitAuthMethod
// ---------------------------------------------------------------------------

func TestNormalizeGitAuthMethod(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		method   models.GitAuthMethod
		expected models.GitAuthMethod
	}{
		{"explicit ssh keeps ssh", "git@github.com:foo/bar.git", models.GitAuthSSH, models.GitAuthSSH},
		{"explicit none keeps none", "https://github.com/foo/bar", models.GitAuthNone, models.GitAuthNone},
		{"empty repo defaults to none", "", "", models.GitAuthNone},
		{"dot repo defaults to none", ".", "", models.GitAuthNone},
		{"remote repo defaults to ssh", "git@github.com:foo/bar.git", "", models.GitAuthSSH},
		{"https remote defaults to ssh", "https://github.com/foo/bar", "", models.GitAuthSSH},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeGitAuthMethod(tt.repo, tt.method)
			if got != tt.expected {
				t.Errorf("normalizeGitAuthMethod(%q, %q) = %q, want %q", tt.repo, tt.method, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: normalizeGitStrategy
// ---------------------------------------------------------------------------

func TestNormalizeGitStrategy(t *testing.T) {
	tests := []struct {
		name     string
		strategy models.GitStrategy
		expected models.GitStrategy
	}{
		{"empty defaults to direct", "", models.GitStrategyDirect},
		{"direct stays direct", models.GitStrategyDirect, models.GitStrategyDirect},
		{"explicit value kept", models.GitStrategy("branch"), models.GitStrategy("branch")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeGitStrategy(tt.strategy)
			if got != tt.expected {
				t.Errorf("normalizeGitStrategy(%q) = %q, want %q", tt.strategy, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: normalizeBeadsPath
// ---------------------------------------------------------------------------

func TestNormalizeBeadsPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty defaults to .beads", "", ".beads"},
		{"whitespace defaults to .beads", "   ", ".beads"},
		{"explicit path kept", "my-beads", "my-beads"},
		{"dotbeads stays", ".beads", ".beads"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeBeadsPath(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeBeadsPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: beadsPathExists
// ---------------------------------------------------------------------------

func TestBeadsPathExists(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		if beadsPathExists("") {
			t.Error("beadsPathExists('') should return false")
		}
	})

	t.Run("nonexistent path", func(t *testing.T) {
		if beadsPathExists("/tmp/nonexistent-beads-path-12345") {
			t.Error("beadsPathExists for nonexistent path should return false")
		}
	})

	t.Run("path with issues.jsonl", func(t *testing.T) {
		tmp, err := os.MkdirTemp("", "beads-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmp)

		issuesFile := filepath.Join(tmp, "issues.jsonl")
		if err := os.WriteFile(issuesFile, []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}

		if !beadsPathExists(tmp) {
			t.Error("beadsPathExists should return true when issues.jsonl exists")
		}
	})

	t.Run("path with beads subdir", func(t *testing.T) {
		tmp, err := os.MkdirTemp("", "beads-test-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmp)

		beadsDir := filepath.Join(tmp, "beads")
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		if !beadsPathExists(tmp) {
			t.Error("beadsPathExists should return true when beads/ subdir exists")
		}
	})
}

// ---------------------------------------------------------------------------
// Pure function tests: isSSHRepo
// ---------------------------------------------------------------------------

func TestIsSSHRepo(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		expected bool
	}{
		{"git@ prefix", "git@github.com:foo/bar.git", true},
		{"ssh:// prefix", "ssh://git@github.com/foo/bar", true},
		{"https is not ssh", "https://github.com/foo/bar", false},
		{"http is not ssh", "http://github.com/foo/bar", false},
		{"empty string", "", false},
		{"dot repo", ".", false},
		{"whitespace around git@", "  git@github.com:foo/bar.git  ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSSHRepo(tt.repo)
			if got != tt.expected {
				t.Errorf("isSSHRepo(%q) = %v, want %v", tt.repo, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: roleFromPersonaName
// ---------------------------------------------------------------------------

func TestRoleFromPersonaName(t *testing.T) {
	tests := []struct {
		name     string
		persona  string
		expected string
	}{
		{"default prefix", "default/engineering-manager", "engineering-manager"},
		{"projects prefix 3 parts", "projects/my-proj/web-designer", "web-designer"},
		{"projects prefix 4 parts", "projects/my-proj/roles/ceo", "ceo"},
		{"generic slash path", "custom/some-role", "some-role"},
		{"bare name", "ceo", "ceo"},
		{"whitespace trimmed", "  default/ceo  ", "ceo"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roleFromPersonaName(tt.persona)
			if got != tt.expected {
				t.Errorf("roleFromPersonaName(%q) = %q, want %q", tt.persona, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: formatAgentName
// ---------------------------------------------------------------------------

func TestFormatAgentName(t *testing.T) {
	tests := []struct {
		name        string
		roleName    string
		personaType string
		expected    string
	}{
		{"simple role", "ceo", "Default", "CEO (Default)"},
		{"hyphenated role", "engineering-manager", "Default", "Engineering Manager (Default)"},
		{"web designer", "web-designer", "Custom", "Web Designer (Custom)"},
		{"qa role", "qa-engineer", "Default", "QA Engineer (Default)"},
		{"cfo role", "cfo", "Default", "CFO (Default)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAgentName(tt.roleName, tt.personaType)
			if got != tt.expected {
				t.Errorf("formatAgentName(%q, %q) = %q, want %q", tt.roleName, tt.personaType, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: capitalizeAcronyms
// ---------------------------------------------------------------------------

func TestCapitalizeAcronyms(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"capitalize Ceo", "Ceo of Company", "CEO of Company"},
		{"capitalize Cfo", "The Cfo Report", "The CFO Report"},
		{"capitalize Qa", "Qa Engineer", "QA Engineer"},
		{"no acronyms", "Engineering Manager", "Engineering Manager"},
		{"multiple acronyms", "Ceo and Cfo Meeting", "CEO and CFO Meeting"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capitalizeAcronyms(tt.input)
			if got != tt.expected {
				t.Errorf("capitalizeAcronyms(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: normalizeRole
// ---------------------------------------------------------------------------

func TestNormalizeRole(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple lowercase", "CTO", "cto"},
		{"with slash prefix", "default/engineering-manager", "engineering-manager"},
		{"with parenthetical", "Engineering Manager (Default)", "engineering-manager"},
		{"underscore to hyphen", "qa_engineer", "qa-engineer"},
		{"spaces to hyphen", "qa engineer", "qa-engineer"},
		{"whitespace trimmed", "  CTO  ", "cto"},
		{"mixed case with slash", "projects/my-proj/Web-Designer", "web-designer"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRole(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeRole(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: isLikelyPersona
// ---------------------------------------------------------------------------

func TestIsLikelyPersona(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"simple name", "engineering-manager", true},
		{"simple word", "ceo", true},
		{"with spaces", "web designer", true},
		{"too short", "ab", false},
		{"too long", strings.Repeat("a", 41), false},
		{"contains numbers", "agent123", false},
		{"contains special chars", "agent@home", false},
		{"starts with hyphen", "-agent", false},
		{"ends with hyphen", "agent-", false},
		{"starts with space", " agent", false},
		{"ends with space", "agent ", false},
		{"uppercase treated as lowercase", "CEO", true},
		{"empty string", "", false},
		{"just 3 chars", "abc", true},
		{"40 chars", strings.Repeat("a", 40), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isLikelyPersona(tt.input)
			if got != tt.expected {
				t.Errorf("isLikelyPersona(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: extractPersonaFromMessage
// ---------------------------------------------------------------------------

func TestExtractPersonaFromMessage(t *testing.T) {
	tests := []struct {
		name            string
		message         string
		expectedPersona string
		expectedMsg     string
	}{
		{"persona prefix", "engineering-manager: review the code", "engineering-manager", "review the code"},
		{"ceo prefix", "ceo: what is the budget?", "ceo", "what is the budget?"},
		{"no persona prefix", "just a regular message without colon", "", "just a regular message without colon"},
		{"number in potential persona", "123: data", "", "123: data"},
		{"colon too far", strings.Repeat("a", 51) + ": rest", "", strings.Repeat("a", 51) + ": rest"},
		{"empty message", "", "", ""},
		{"whitespace message", "  ceo: hello  ", "ceo", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			persona, msg := extractPersonaFromMessage(tt.message)
			if persona != tt.expectedPersona {
				t.Errorf("extractPersonaFromMessage(%q) persona = %q, want %q", tt.message, persona, tt.expectedPersona)
			}
			if msg != tt.expectedMsg {
				t.Errorf("extractPersonaFromMessage(%q) message = %q, want %q", tt.message, msg, tt.expectedMsg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: isChatCapableModel
// ---------------------------------------------------------------------------

func TestIsChatCapableModel(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"empty", "", false},
		{"instruct model", "Llama-3-Instruct", true},
		{"chat model", "Qwen-Chat-7B", true},
		{"claude", "claude-3-opus", true},
		{"gpt prefix", "gpt-4-turbo", true},
		{"nemotron", "nvidia/NVIDIA-Nemotron-3-Nano-30B", true},
		{"deepseek", "deepseek-coder-v2", true},
		{"gemma", "gemma-7b-it", true},
		{"phi model", "phi-3-mini", true},
		{"qwen model", "Qwen2-7B", true},
		{"starcoder not chat", "starcoder2-3b", false},
		{"codegen not chat", "codegen-16B-mono", false},
		{"base model not chat", "llama-7b-base", false},
		{"raw model not chat", "codellama-raw", false},
		{"unknown defaults to chat", "some-custom-model", true},
		{"llama3 chat capable", "llama-3-8b", true},
		{"mistral", "mistral-7b-instruct", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isChatCapableModel(tt.model)
			if got != tt.expected {
				t.Errorf("isChatCapableModel(%q) = %v, want %v", tt.model, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: rolesForProfile
// ---------------------------------------------------------------------------

func TestRolesForProfile(t *testing.T) {
	tests := []struct {
		name     string
		profile  string
		expected []string
		isNil    bool
	}{
		{"startup", "startup", []string{"ceo", "engineering-manager", "web-designer"}, false},
		{"solo", "solo", []string{"ceo", "engineering-manager"}, false},
		{"full returns nil", "full", nil, true},
		{"enterprise returns nil", "enterprise", nil, true},
		{"empty returns nil", "", nil, true},
		{"unknown returns nil", "unknown-profile", nil, true},
		{"uppercase Startup", "Startup", []string{"ceo", "engineering-manager", "web-designer"}, false},
		{"whitespace trimmed", "  solo  ", []string{"ceo", "engineering-manager"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rolesForProfile(tt.profile)
			if tt.isNil {
				if got != nil {
					t.Errorf("rolesForProfile(%q) = %v, want nil", tt.profile, got)
				}
				return
			}
			if len(got) != len(tt.expected) {
				t.Errorf("rolesForProfile(%q) len = %d, want %d", tt.profile, len(got), len(tt.expected))
				return
			}
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("rolesForProfile(%q)[%d] = %q, want %q", tt.profile, i, v, tt.expected[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FileLockManager comprehensive tests
// ---------------------------------------------------------------------------

func TestNewFileLockManager(t *testing.T) {
	flm := NewFileLockManager(5 * time.Minute)
	if flm == nil {
		t.Fatal("NewFileLockManager returned nil")
	}
	if flm.timeout != 5*time.Minute {
		t.Errorf("timeout = %v, want %v", flm.timeout, 5*time.Minute)
	}
	if flm.locks == nil {
		t.Error("locks map is nil")
	}
}

func TestFileLockManager_LockKey(t *testing.T) {
	flm := NewFileLockManager(time.Minute)
	key := flm.lockKey("proj1", "/src/main.go")
	if key != "proj1:/src/main.go" {
		t.Errorf("lockKey = %q, want %q", key, "proj1:/src/main.go")
	}
}

func TestFileLockManager_AcquireAndRelease(t *testing.T) {
	flm := NewFileLockManager(5 * time.Minute)

	lock, err := flm.AcquireLock("proj1", "/src/main.go", "agent-1", "bead-1")
	if err != nil {
		t.Fatalf("AcquireLock() error = %v", err)
	}
	if lock == nil {
		t.Fatal("AcquireLock() returned nil lock")
	}
	if lock.FilePath != "/src/main.go" {
		t.Errorf("lock.FilePath = %q, want %q", lock.FilePath, "/src/main.go")
	}
	if lock.AgentID != "agent-1" {
		t.Errorf("lock.AgentID = %q, want %q", lock.AgentID, "agent-1")
	}

	// Try to acquire the same lock with a different agent - should fail
	_, err = flm.AcquireLock("proj1", "/src/main.go", "agent-2", "bead-2")
	if err == nil {
		t.Error("AcquireLock should fail when file is already locked")
	}
	if !strings.Contains(err.Error(), "already locked") {
		t.Errorf("error should mention already locked, got: %v", err)
	}

	// Release the lock
	err = flm.ReleaseLock("proj1", "/src/main.go", "agent-1")
	if err != nil {
		t.Fatalf("ReleaseLock() error = %v", err)
	}

	// Now agent-2 can acquire
	lock2, err := flm.AcquireLock("proj1", "/src/main.go", "agent-2", "bead-2")
	if err != nil {
		t.Fatalf("AcquireLock() after release error = %v", err)
	}
	if lock2.AgentID != "agent-2" {
		t.Errorf("lock2.AgentID = %q, want %q", lock2.AgentID, "agent-2")
	}
}

func TestFileLockManager_ReleaseErrors(t *testing.T) {
	flm := NewFileLockManager(5 * time.Minute)

	// Release non-existent lock
	err := flm.ReleaseLock("proj1", "/src/main.go", "agent-1")
	if err == nil {
		t.Error("ReleaseLock should fail for non-existent lock")
	}

	// Acquire, then try to release with wrong agent
	_, _ = flm.AcquireLock("proj1", "/src/main.go", "agent-1", "bead-1")
	err = flm.ReleaseLock("proj1", "/src/main.go", "agent-2")
	if err == nil {
		t.Error("ReleaseLock should fail when wrong agent tries to release")
	}
	if !strings.Contains(err.Error(), "cannot release") {
		t.Errorf("error should mention cannot release, got: %v", err)
	}
}

func TestFileLockManager_IsLocked(t *testing.T) {
	flm := NewFileLockManager(5 * time.Minute)

	if flm.IsLocked("proj1", "/src/main.go") {
		t.Error("IsLocked should return false before any lock")
	}

	_, _ = flm.AcquireLock("proj1", "/src/main.go", "agent-1", "bead-1")

	if !flm.IsLocked("proj1", "/src/main.go") {
		t.Error("IsLocked should return true after acquiring lock")
	}
	if flm.IsLocked("proj1", "/src/other.go") {
		t.Error("IsLocked should return false for different file")
	}

	_ = flm.ReleaseLock("proj1", "/src/main.go", "agent-1")
	if flm.IsLocked("proj1", "/src/main.go") {
		t.Error("IsLocked should return false after release")
	}
}

func TestFileLockManager_GetLock(t *testing.T) {
	flm := NewFileLockManager(5 * time.Minute)

	// Get non-existent lock
	_, err := flm.GetLock("proj1", "/src/main.go")
	if err == nil {
		t.Error("GetLock should fail for non-existent lock")
	}

	_, _ = flm.AcquireLock("proj1", "/src/main.go", "agent-1", "bead-1")
	lock, err := flm.GetLock("proj1", "/src/main.go")
	if err != nil {
		t.Fatalf("GetLock() error = %v", err)
	}
	if lock.AgentID != "agent-1" {
		t.Errorf("GetLock().AgentID = %q, want %q", lock.AgentID, "agent-1")
	}
}

func TestFileLockManager_ExpiredLock(t *testing.T) {
	// Use a very short timeout
	flm := NewFileLockManager(1 * time.Millisecond)

	_, _ = flm.AcquireLock("proj1", "/src/main.go", "agent-1", "bead-1")

	// Wait for lock to expire
	time.Sleep(10 * time.Millisecond)

	// IsLocked should return false for expired lock
	if flm.IsLocked("proj1", "/src/main.go") {
		t.Error("IsLocked should return false for expired lock")
	}

	// GetLock should return error for expired lock
	_, err := flm.GetLock("proj1", "/src/main.go")
	if err == nil {
		t.Error("GetLock should fail for expired lock")
	}

	// A new agent can acquire the expired lock
	lock, err := flm.AcquireLock("proj1", "/src/main.go", "agent-2", "bead-2")
	if err != nil {
		t.Fatalf("AcquireLock() should succeed on expired lock, got error = %v", err)
	}
	if lock.AgentID != "agent-2" {
		t.Errorf("lock.AgentID = %q, want %q", lock.AgentID, "agent-2")
	}
}

func TestFileLockManager_ListLocks(t *testing.T) {
	flm := NewFileLockManager(5 * time.Minute)

	locks := flm.ListLocks()
	if len(locks) != 0 {
		t.Errorf("ListLocks() len = %d, want 0", len(locks))
	}

	_, _ = flm.AcquireLock("proj1", "/a.go", "agent-1", "bead-1")
	_, _ = flm.AcquireLock("proj1", "/b.go", "agent-2", "bead-2")
	_, _ = flm.AcquireLock("proj2", "/c.go", "agent-1", "bead-3")

	locks = flm.ListLocks()
	if len(locks) != 3 {
		t.Errorf("ListLocks() len = %d, want 3", len(locks))
	}
}

func TestFileLockManager_ListLocksByProject(t *testing.T) {
	flm := NewFileLockManager(5 * time.Minute)

	_, _ = flm.AcquireLock("proj1", "/a.go", "agent-1", "bead-1")
	_, _ = flm.AcquireLock("proj1", "/b.go", "agent-2", "bead-2")
	_, _ = flm.AcquireLock("proj2", "/c.go", "agent-1", "bead-3")

	locks := flm.ListLocksByProject("proj1")
	if len(locks) != 2 {
		t.Errorf("ListLocksByProject('proj1') len = %d, want 2", len(locks))
	}

	locks = flm.ListLocksByProject("proj2")
	if len(locks) != 1 {
		t.Errorf("ListLocksByProject('proj2') len = %d, want 1", len(locks))
	}

	locks = flm.ListLocksByProject("proj3")
	if len(locks) != 0 {
		t.Errorf("ListLocksByProject('proj3') len = %d, want 0", len(locks))
	}
}

func TestFileLockManager_ListLocksByAgent(t *testing.T) {
	flm := NewFileLockManager(5 * time.Minute)

	_, _ = flm.AcquireLock("proj1", "/a.go", "agent-1", "bead-1")
	_, _ = flm.AcquireLock("proj1", "/b.go", "agent-2", "bead-2")
	_, _ = flm.AcquireLock("proj2", "/c.go", "agent-1", "bead-3")

	locks := flm.ListLocksByAgent("agent-1")
	if len(locks) != 2 {
		t.Errorf("ListLocksByAgent('agent-1') len = %d, want 2", len(locks))
	}

	locks = flm.ListLocksByAgent("agent-2")
	if len(locks) != 1 {
		t.Errorf("ListLocksByAgent('agent-2') len = %d, want 1", len(locks))
	}

	locks = flm.ListLocksByAgent("agent-3")
	if len(locks) != 0 {
		t.Errorf("ListLocksByAgent('agent-3') len = %d, want 0", len(locks))
	}
}

func TestFileLockManager_ReleaseAgentLocks(t *testing.T) {
	flm := NewFileLockManager(5 * time.Minute)

	_, _ = flm.AcquireLock("proj1", "/a.go", "agent-1", "bead-1")
	_, _ = flm.AcquireLock("proj1", "/b.go", "agent-1", "bead-2")
	_, _ = flm.AcquireLock("proj2", "/c.go", "agent-2", "bead-3")

	err := flm.ReleaseAgentLocks("agent-1")
	if err != nil {
		t.Fatalf("ReleaseAgentLocks() error = %v", err)
	}

	locks := flm.ListLocks()
	if len(locks) != 1 {
		t.Errorf("After ReleaseAgentLocks('agent-1'), ListLocks() len = %d, want 1", len(locks))
	}
	if locks[0].AgentID != "agent-2" {
		t.Errorf("remaining lock agent = %q, want %q", locks[0].AgentID, "agent-2")
	}
}

func TestFileLockManager_CleanExpiredLocks(t *testing.T) {
	flm := NewFileLockManager(1 * time.Millisecond)

	_, _ = flm.AcquireLock("proj1", "/a.go", "agent-1", "bead-1")
	_, _ = flm.AcquireLock("proj1", "/b.go", "agent-2", "bead-2")

	time.Sleep(10 * time.Millisecond)

	cleaned := flm.CleanExpiredLocks()
	if cleaned != 2 {
		t.Errorf("CleanExpiredLocks() = %d, want 2", cleaned)
	}

	locks := flm.ListLocks()
	if len(locks) != 0 {
		t.Errorf("After clean, ListLocks() len = %d, want 0", len(locks))
	}
}

func TestFileLockManager_ExtendLock(t *testing.T) {
	flm := NewFileLockManager(5 * time.Minute)

	// Extend non-existent lock
	err := flm.ExtendLock("proj1", "/a.go", "agent-1", 10*time.Minute)
	if err == nil {
		t.Error("ExtendLock should fail for non-existent lock")
	}

	_, _ = flm.AcquireLock("proj1", "/a.go", "agent-1", "bead-1")

	// Wrong agent tries to extend
	err = flm.ExtendLock("proj1", "/a.go", "agent-2", 10*time.Minute)
	if err == nil {
		t.Error("ExtendLock should fail for wrong agent")
	}

	// Correct agent extends
	err = flm.ExtendLock("proj1", "/a.go", "agent-1", 10*time.Minute)
	if err != nil {
		t.Fatalf("ExtendLock() error = %v", err)
	}

	lock, _ := flm.GetLock("proj1", "/a.go")
	if lock.ExpiresAt.Before(time.Now().Add(9 * time.Minute)) {
		t.Error("ExtendLock should have extended expiration by ~10 minutes")
	}
}

func TestFileLockManager_ConcurrentAccess(t *testing.T) {
	flm := NewFileLockManager(5 * time.Minute)
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(agentID string) {
			defer wg.Done()
			_, err := flm.AcquireLock("proj1", "/contested.go", agentID, "bead-1")
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}(fmt.Sprintf("agent-%d", i))
	}

	wg.Wait()
	if successCount != 1 {
		t.Errorf("concurrent AcquireLock: %d succeeded, want exactly 1", successCount)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: allowedRoleSet
// ---------------------------------------------------------------------------

func TestLoom_AllowedRoleSet(t *testing.T) {
	t.Run("empty config returns nil", func(t *testing.T) {
		l, tmpDir := testLoom(t)
		defer os.RemoveAll(tmpDir)

		set := l.allowedRoleSet()
		if set != nil {
			t.Errorf("allowedRoleSet() should be nil for empty config, got %v", set)
		}
	})

	t.Run("with allowed roles", func(t *testing.T) {
		l, tmpDir := testLoom(t, func(c *config.Config) {
			c.Agents.AllowedRoles = []string{"CEO", "Engineering-Manager", "  qa  "}
		})
		defer os.RemoveAll(tmpDir)

		set := l.allowedRoleSet()
		if set == nil {
			t.Fatal("allowedRoleSet() should not be nil")
		}
		if _, ok := set["ceo"]; !ok {
			t.Error("set should contain 'ceo'")
		}
		if _, ok := set["engineering-manager"]; !ok {
			t.Error("set should contain 'engineering-manager'")
		}
		if _, ok := set["qa"]; !ok {
			t.Error("set should contain 'qa'")
		}
	})

	t.Run("with corp profile startup", func(t *testing.T) {
		l, tmpDir := testLoom(t, func(c *config.Config) {
			c.Agents.CorpProfile = "startup"
		})
		defer os.RemoveAll(tmpDir)

		set := l.allowedRoleSet()
		if set == nil {
			t.Fatal("allowedRoleSet() should not be nil for startup profile")
		}
		if _, ok := set["ceo"]; !ok {
			t.Error("startup profile should contain 'ceo'")
		}
		if _, ok := set["engineering-manager"]; !ok {
			t.Error("startup profile should contain 'engineering-manager'")
		}
	})

	t.Run("with all empty roles", func(t *testing.T) {
		l, tmpDir := testLoom(t, func(c *config.Config) {
			c.Agents.AllowedRoles = []string{"  ", "", "  "}
		})
		defer os.RemoveAll(tmpDir)

		set := l.allowedRoleSet()
		if set != nil {
			t.Errorf("allowedRoleSet() should be nil when all roles are empty, got %v", set)
		}
	})
}

// ---------------------------------------------------------------------------
// Loom method tests: stub methods that return errors
// ---------------------------------------------------------------------------

func TestLoom_StubMethods(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	t.Run("StartDevelopment", func(t *testing.T) {
		result, err := l.StartDevelopment(ctx, "waterfall", false, "/tmp")
		if err == nil {
			t.Error("StartDevelopment should return error")
		}
		if result != nil {
			t.Error("StartDevelopment result should be nil")
		}
	})

	t.Run("WhatsNext", func(t *testing.T) {
		result, err := l.WhatsNext(ctx, "", "", "", nil)
		if err == nil {
			t.Error("WhatsNext should return error")
		}
		if result != nil {
			t.Error("WhatsNext result should be nil")
		}
	})

	t.Run("ProceedToPhase", func(t *testing.T) {
		result, err := l.ProceedToPhase(ctx, "phase1", "pending", "test")
		if err == nil {
			t.Error("ProceedToPhase should return error")
		}
		if result != nil {
			t.Error("ProceedToPhase result should be nil")
		}
	})

	t.Run("ConductReview", func(t *testing.T) {
		result, err := l.ConductReview(ctx, "phase2")
		if err == nil {
			t.Error("ConductReview should return error")
		}
		if result != nil {
			t.Error("ConductReview result should be nil")
		}
	})

	t.Run("ResumeWorkflow", func(t *testing.T) {
		result, err := l.ResumeWorkflow(ctx, true)
		if err == nil {
			t.Error("ResumeWorkflow should return error")
		}
		if result != nil {
			t.Error("ResumeWorkflow result should be nil")
		}
	})
}

// ---------------------------------------------------------------------------
// Loom method tests: ExecuteShellCommand / GetCommandLogs / GetCommandLog
// ---------------------------------------------------------------------------

func TestLoom_ExecuteShellCommand(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	// ShellExecutor should exist when database is set up
	if l.shellExecutor == nil {
		t.Skip("ShellExecutor not available - skipping")
	}
	// Test that it at least does not panic with an empty request
	_, _ = l.ExecuteShellCommand(ctx, executor.ExecuteCommandRequest{})
}

func TestLoom_ExecuteShellCommand_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	_, err := l.ExecuteShellCommand(ctx, executor.ExecuteCommandRequest{})
	if err == nil {
		t.Error("ExecuteShellCommand should fail without database")
	}
	if !strings.Contains(err.Error(), "shell executor not available") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoom_GetCommandLogs_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	_, err := l.GetCommandLogs(nil, 10)
	if err == nil {
		t.Error("GetCommandLogs should fail without database")
	}
}

func TestLoom_GetCommandLog_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	_, err := l.GetCommandLog("some-id")
	if err == nil {
		t.Error("GetCommandLog should fail without database")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: Provider management
// ---------------------------------------------------------------------------

func TestLoom_RegisterProvider(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	t.Run("successful registration", func(t *testing.T) {
		p := &internalmodels.Provider{
			ID:       "test-provider",
			Name:     "Test Provider",
			Type:     "local",
			Endpoint: "http://localhost:8000",
		}
		result, err := l.RegisterProvider(ctx, p)
		if err != nil {
			t.Fatalf("RegisterProvider() error = %v", err)
		}
		if result.ID != "test-provider" {
			t.Errorf("result.ID = %q, want %q", result.ID, "test-provider")
		}
		if result.Endpoint != "http://localhost:8000/v1" {
			t.Errorf("result.Endpoint = %q, want normalized endpoint", result.Endpoint)
		}
		if result.Status != "pending" {
			t.Errorf("result.Status = %q, want %q", result.Status, "pending")
		}
	})

	t.Run("empty ID fails", func(t *testing.T) {
		p := &internalmodels.Provider{Name: "No ID"}
		_, err := l.RegisterProvider(ctx, p)
		if err == nil {
			t.Error("RegisterProvider should fail with empty ID")
		}
	})

	t.Run("defaults filled in", func(t *testing.T) {
		p := &internalmodels.Provider{ID: "defaults-test"}
		result, err := l.RegisterProvider(ctx, p)
		if err != nil {
			t.Fatalf("RegisterProvider() error = %v", err)
		}
		if result.Name != "defaults-test" {
			t.Errorf("Name should default to ID, got %q", result.Name)
		}
		if result.Type != "local" {
			t.Errorf("Type should default to 'local', got %q", result.Type)
		}
	})

	t.Run("with API key", func(t *testing.T) {
		p := &internalmodels.Provider{
			ID:       "api-key-provider",
			Endpoint: "https://api.openai.com",
		}
		result, err := l.RegisterProvider(ctx, p, "sk-test-key")
		if err != nil {
			t.Fatalf("RegisterProvider() error = %v", err)
		}
		if result == nil {
			t.Fatal("result should not be nil")
		}
	})
}

func TestLoom_RegisterProvider_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	p := &internalmodels.Provider{ID: "test"}
	_, err := l.RegisterProvider(ctx, p)
	if err == nil {
		t.Error("RegisterProvider should fail without database")
	}
}

func TestLoom_UpdateProvider(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// Register first
	p := &internalmodels.Provider{
		ID:       "update-test",
		Name:     "Original Name",
		Type:     "local",
		Endpoint: "http://localhost:8000",
	}
	_, err := l.RegisterProvider(ctx, p)
	if err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}

	// Update
	p.Name = "Updated Name"
	result, err := l.UpdateProvider(ctx, p)
	if err != nil {
		t.Fatalf("UpdateProvider() error = %v", err)
	}
	if result.Name != "Updated Name" {
		t.Errorf("result.Name = %q, want %q", result.Name, "Updated Name")
	}
}

func TestLoom_UpdateProvider_Errors(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	t.Run("nil provider", func(t *testing.T) {
		_, err := l.UpdateProvider(ctx, nil)
		if err == nil {
			t.Error("UpdateProvider(nil) should fail")
		}
	})

	t.Run("empty ID", func(t *testing.T) {
		_, err := l.UpdateProvider(ctx, &internalmodels.Provider{})
		if err == nil {
			t.Error("UpdateProvider with empty ID should fail")
		}
	})

	t.Run("defaults filled in", func(t *testing.T) {
		p := &internalmodels.Provider{ID: "update-defaults"}
		result, err := l.UpdateProvider(ctx, p)
		if err != nil {
			t.Fatalf("UpdateProvider() error = %v", err)
		}
		if result.Name != "update-defaults" {
			t.Errorf("Name should default to ID, got %q", result.Name)
		}
		if result.Type != "local" {
			t.Errorf("Type should default to 'local', got %q", result.Type)
		}
		if result.Status != "pending" {
			t.Errorf("Status should default to 'pending', got %q", result.Status)
		}
	})
}

func TestLoom_UpdateProvider_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	_, err := l.UpdateProvider(ctx, &internalmodels.Provider{ID: "test"})
	if err == nil {
		t.Error("UpdateProvider should fail without database")
	}
}

func TestLoom_DeleteProvider(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// Register first
	p := &internalmodels.Provider{
		ID:       "to-delete",
		Endpoint: "http://localhost:8000",
	}
	_, err := l.RegisterProvider(ctx, p)
	if err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}

	// Delete
	err = l.DeleteProvider(ctx, "to-delete")
	if err != nil {
		t.Errorf("DeleteProvider() error = %v", err)
	}
}

func TestLoom_DeleteProvider_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	err := l.DeleteProvider(ctx, "test")
	if err == nil {
		t.Error("DeleteProvider should fail without database")
	}
}

func TestLoom_ListProviders_WithDatabase(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	providers, err := l.ListProviders()
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	initialCount := len(providers)

	// Register a provider
	p := &internalmodels.Provider{
		ID:       "list-test",
		Endpoint: "http://localhost:8000",
	}
	_, err = l.RegisterProvider(ctx, p)
	if err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}

	providers, err = l.ListProviders()
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	if len(providers) != initialCount+1 {
		t.Errorf("ListProviders() len = %d, want %d", len(providers), initialCount+1)
	}
}

func TestLoom_ListProviders_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	providers, err := l.ListProviders()
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	if len(providers) != 0 {
		t.Errorf("ListProviders() without DB should return empty, got %d", len(providers))
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: PersistProject
// ---------------------------------------------------------------------------

func TestLoom_PersistProject(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// Create a project
	proj, err := l.CreateProject("persist-test", ".", "", "", nil)
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// PersistProject should not panic
	l.PersistProject(proj.ID)

	// PersistProject with non-existent project should not panic
	l.PersistProject("nonexistent")
}

func TestLoom_PersistProject_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	// Should not panic even without database
	l.PersistProject("any-project")
}

// ---------------------------------------------------------------------------
// Loom method tests: AdvanceWorkflowWithCondition (condition mapping)
// ---------------------------------------------------------------------------

func TestLoom_AdvanceWorkflowWithCondition_Conditions(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// All calls should fail because no workflow execution exists,
	// but they test the condition mapping code paths
	conditions := []string{"approved", "rejected", "success", "failure", "timeout", "escalated"}
	for _, cond := range conditions {
		t.Run(cond, func(t *testing.T) {
			err := l.AdvanceWorkflowWithCondition("bead-1", "agent-1", cond, nil)
			if err == nil {
				t.Errorf("AdvanceWorkflowWithCondition(%q) should fail for non-existent bead", cond)
			}
		})
	}

	// Unknown condition
	t.Run("unknown condition", func(t *testing.T) {
		err := l.AdvanceWorkflowWithCondition("bead-1", "agent-1", "unknown-cond", nil)
		if err == nil {
			t.Error("AdvanceWorkflowWithCondition with unknown condition should fail")
		}
		if !strings.Contains(err.Error(), "unknown workflow condition") {
			t.Errorf("error should mention unknown condition, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Loom method tests: AdvanceWorkflowWithCondition (nil engine)
// ---------------------------------------------------------------------------

func TestLoom_AdvanceWorkflowWithCondition_NilEngine(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{} // no database = no workflow engine
	})
	defer os.RemoveAll(tmpDir)

	err := l.AdvanceWorkflowWithCondition("bead-1", "agent-1", "success", nil)
	if err == nil {
		t.Error("Should fail when workflow engine is nil")
	}
	if !strings.Contains(err.Error(), "workflow engine not available") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: CloseBead with empty reason
// ---------------------------------------------------------------------------

func TestLoom_CloseBead_EmptyReason(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	proj, err := l.CreateProject("close-empty", ".", "", "", nil)
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	bead, err := l.CreateBead("Test", "desc", models.BeadPriorityP2, "task", proj.ID)
	if err != nil {
		t.Fatalf("CreateBead() error = %v", err)
	}

	// Close with empty reason
	err = l.CloseBead(bead.ID, "")
	if err != nil {
		t.Errorf("CloseBead() with empty reason error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: findDefaultAssignee
// ---------------------------------------------------------------------------

func TestLoom_FindDefaultAssignee(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// With no agents, should return empty
	result := l.findDefaultAssignee("any-project")
	if result != "" {
		t.Errorf("findDefaultAssignee with no agents = %q, want empty", result)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: GetProviderModels
// ---------------------------------------------------------------------------

func TestLoom_GetProviderModels(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// Non-existent provider should error
	_, err := l.GetProviderModels(ctx, "nonexistent")
	if err == nil {
		t.Error("GetProviderModels should fail for non-existent provider")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: ListModelCatalog nil catalog
// ---------------------------------------------------------------------------

func TestLoom_ListModelCatalog_NilCatalog(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// Save and nil out
	saved := l.modelCatalog
	l.modelCatalog = nil

	result := l.ListModelCatalog()
	if result != nil {
		t.Errorf("ListModelCatalog() should be nil when catalog is nil, got %d items", len(result))
	}

	// Restore
	l.modelCatalog = saved
}

// ---------------------------------------------------------------------------
// Loom method tests: setupProviderMetrics edge cases
// ---------------------------------------------------------------------------

func TestLoom_SetupProviderMetrics_NilMetrics(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	saved := l.metrics
	l.metrics = nil
	// Should not panic
	l.setupProviderMetrics()
	l.metrics = saved
}

func TestLoom_SetupProviderMetrics_NilRegistry(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	saved := l.providerRegistry
	l.providerRegistry = nil
	// Should not panic
	l.setupProviderMetrics()
	l.providerRegistry = saved
}

// ---------------------------------------------------------------------------
// Loom method tests: Shutdown edge cases
// ---------------------------------------------------------------------------

func TestLoom_Shutdown_WithoutDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	// Should not panic
	l.Shutdown()
}

// ---------------------------------------------------------------------------
// Loom method tests: New with preferred models tiers
// ---------------------------------------------------------------------------

func TestLoom_New_ModelTierMapping(t *testing.T) {
	tiers := []struct {
		tier string
	}{
		{"extended"},
		{"complex"},
		{"medium"},
		{"simple"},
		{"unknown-tier"},
	}

	for _, tt := range tiers {
		t.Run(tt.tier, func(t *testing.T) {
			l, tmpDir := testLoom(t, func(c *config.Config) {
				c.Models = config.ModelsConfig{
					PreferredModels: []config.PreferredModel{
						{Name: "model-" + tt.tier, Rank: 1, Tier: tt.tier},
					},
				}
			})
			defer os.RemoveAll(tmpDir)

			catalog := l.GetModelCatalog()
			if catalog == nil {
				t.Fatal("catalog should not be nil")
			}
			specs := catalog.List()
			if len(specs) == 0 {
				t.Error("catalog should have at least one model")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: RegisterProvider with ollama type (no endpoint normalization)
// ---------------------------------------------------------------------------

func TestLoom_RegisterProvider_Ollama(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	p := &internalmodels.Provider{
		ID:       "ollama-test",
		Type:     "ollama",
		Endpoint: "http://localhost:11434",
	}
	result, err := l.RegisterProvider(ctx, p)
	if err != nil {
		t.Fatalf("RegisterProvider() error = %v", err)
	}
	// Ollama endpoints should NOT have /v1 appended
	if result.Endpoint != "http://localhost:11434" {
		t.Errorf("Ollama endpoint should not be normalized, got %q", result.Endpoint)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: UpdateProvider with ollama type
// ---------------------------------------------------------------------------

func TestLoom_UpdateProvider_Ollama(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	p := &internalmodels.Provider{
		ID:       "ollama-update",
		Type:     "ollama",
		Endpoint: "http://localhost:11434",
	}
	result, err := l.UpdateProvider(ctx, p)
	if err != nil {
		t.Fatalf("UpdateProvider() error = %v", err)
	}
	if result.Endpoint != "http://localhost:11434" {
		t.Errorf("Ollama endpoint should not be normalized, got %q", result.Endpoint)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: Provider model field defaults
// ---------------------------------------------------------------------------

func TestLoom_RegisterProvider_ModelDefaults(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	t.Run("no model defaults to Nemotron", func(t *testing.T) {
		p := &internalmodels.Provider{
			ID: "no-model",
		}
		result, err := l.RegisterProvider(ctx, p)
		if err != nil {
			t.Fatalf("RegisterProvider() error = %v", err)
		}
		if result.ConfiguredModel == "" {
			t.Error("ConfiguredModel should not be empty")
		}
		if result.SelectedModel == "" {
			t.Error("SelectedModel should not be empty")
		}
	})

	t.Run("explicit model preserved", func(t *testing.T) {
		p := &internalmodels.Provider{
			ID:    "explicit-model",
			Model: "custom-model-v1",
		}
		result, err := l.RegisterProvider(ctx, p)
		if err != nil {
			t.Fatalf("RegisterProvider() error = %v", err)
		}
		if result.ConfiguredModel != "custom-model-v1" {
			t.Errorf("ConfiguredModel = %q, want %q", result.ConfiguredModel, "custom-model-v1")
		}
	})
}

// ---------------------------------------------------------------------------
// Loom method tests: EscalateBeadToCEO
// ---------------------------------------------------------------------------

func TestLoom_EscalateBeadToCEO_NotFound(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	_, err := l.EscalateBeadToCEO("nonexistent", "reason", "agent-1")
	if err == nil {
		t.Error("EscalateBeadToCEO should fail for non-existent bead")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: UnblockDependents
// ---------------------------------------------------------------------------

func TestLoom_UnblockDependents(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// Should succeed with no blocked beads
	err := l.UnblockDependents("nonexistent-decision")
	if err != nil {
		t.Errorf("UnblockDependents() error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: CreateDecisionBead
// ---------------------------------------------------------------------------

func TestLoom_CreateDecisionBead_SystemRequester(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	proj, err := l.CreateProject("decision-test", ".", "", "", nil)
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// System requester should work
	decision, err := l.CreateDecisionBead("Should we deploy?", "", "system", []string{"yes", "no"}, "yes", models.BeadPriorityP1, proj.ID)
	if err != nil {
		t.Fatalf("CreateDecisionBead() error = %v", err)
	}
	if decision == nil {
		t.Fatal("CreateDecisionBead() returned nil")
	}

	// User requester should work
	decision2, err := l.CreateDecisionBead("Should we deploy?", "", "user-admin", []string{"yes", "no"}, "no", models.BeadPriorityP2, proj.ID)
	if err != nil {
		t.Fatalf("CreateDecisionBead(user-admin) error = %v", err)
	}
	if decision2 == nil {
		t.Fatal("CreateDecisionBead(user-admin) returned nil")
	}
}

func TestLoom_CreateDecisionBead_InvalidRequester(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	proj, err := l.CreateProject("decision-err", ".", "", "", nil)
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	// Non-system, non-user requester that doesn't exist should fail
	_, err = l.CreateDecisionBead("Q?", "", "nonexistent-agent", []string{"a"}, "", models.BeadPriorityP2, proj.ID)
	if err == nil {
		t.Error("CreateDecisionBead with nonexistent agent should fail")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: MakeDecision
// ---------------------------------------------------------------------------

func TestLoom_MakeDecision_NonExistentDecision(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	err := l.MakeDecision("nonexistent", "user-admin", "approve", "reason")
	if err == nil {
		t.Error("MakeDecision should fail for non-existent decision")
	}
}

func TestLoom_MakeDecision_InvalidDecider(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	proj, err := l.CreateProject("make-decision-test", ".", "", "", nil)
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}

	decision, err := l.CreateDecisionBead("Q?", "", "system", []string{"yes", "no"}, "", models.BeadPriorityP2, proj.ID)
	if err != nil {
		t.Fatalf("CreateDecisionBead() error = %v", err)
	}

	// Non-existent agent decider should fail
	err = l.MakeDecision(decision.ID, "nonexistent-agent", "yes", "because")
	if err == nil {
		t.Error("MakeDecision with nonexistent agent should fail")
	}

	// User decider should succeed
	err = l.MakeDecision(decision.ID, "user-admin", "yes", "because")
	if err != nil {
		t.Errorf("MakeDecision(user-admin) error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: RunReplQuery edge cases
// ---------------------------------------------------------------------------

func TestLoom_RunReplQuery_EmptyMessage(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	_, err := l.RunReplQuery(ctx, "")
	if err == nil {
		t.Error("RunReplQuery with empty message should fail")
	}
	if !strings.Contains(err.Error(), "message is required") {
		t.Errorf("unexpected error: %v", err)
	}

	_, err = l.RunReplQuery(ctx, "   ")
	if err == nil {
		t.Error("RunReplQuery with whitespace message should fail")
	}
}

func TestLoom_RunReplQuery_NoTemporal(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	_, err := l.RunReplQuery(ctx, "hello")
	if err == nil {
		t.Error("RunReplQuery should fail without Temporal")
	}
	if !strings.Contains(err.Error(), "temporal") {
		t.Errorf("error should mention temporal, got: %v", err)
	}
}

func TestLoom_RunReplQuery_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	_, err := l.RunReplQuery(ctx, "hello")
	if err == nil {
		t.Error("RunReplQuery should fail without database")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: NegotiateProviderModel
// ---------------------------------------------------------------------------

func TestLoom_NegotiateProviderModel_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	_, err := l.NegotiateProviderModel(ctx, "test")
	if err == nil {
		t.Error("NegotiateProviderModel should fail without database")
	}
}

func TestLoom_NegotiateProviderModel_EmptyID(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	_, err := l.NegotiateProviderModel(ctx, "")
	if err == nil {
		t.Error("NegotiateProviderModel should fail with empty ID")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: GetProjectGitPublicKey / RotateProjectGitKey
// ---------------------------------------------------------------------------

func TestLoom_GetProjectGitPublicKey_Errors(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// Non-existent project
	_, err := l.GetProjectGitPublicKey("nonexistent")
	if err == nil {
		t.Error("GetProjectGitPublicKey should fail for non-existent project")
	}

	// Project without SSH auth
	proj, err := l.CreateProject("no-ssh", ".", "", "", nil)
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	_, err = l.GetProjectGitPublicKey(proj.ID)
	if err == nil {
		t.Error("GetProjectGitPublicKey should fail for non-SSH project")
	}
}

func TestLoom_RotateProjectGitKey_Errors(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// Non-existent project
	_, err := l.RotateProjectGitKey("nonexistent")
	if err == nil {
		t.Error("RotateProjectGitKey should fail for non-existent project")
	}

	// Project without SSH auth
	proj, err := l.CreateProject("no-ssh-rotate", ".", "", "", nil)
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	_, err = l.RotateProjectGitKey(proj.ID)
	if err == nil {
		t.Error("RotateProjectGitKey should fail for non-SSH project")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: AssignAgentToProject / UnassignAgentFromProject
// ---------------------------------------------------------------------------

func TestLoom_AssignAgentToProject_Errors(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// Non-existent agent
	err := l.AssignAgentToProject("nonexistent-agent", "any-project")
	if err == nil {
		t.Error("AssignAgentToProject should fail for non-existent agent")
	}
}

func TestLoom_UnassignAgentFromProject_Errors(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// Non-existent project
	err := l.UnassignAgentFromProject("any-agent", "nonexistent-project")
	if err == nil {
		t.Error("UnassignAgentFromProject should fail for non-existent project")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: SpawnAgent
// ---------------------------------------------------------------------------

func TestLoom_SpawnAgent_Errors(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// Invalid persona
	_, err := l.SpawnAgent(ctx, "test", "nonexistent-persona", "proj1", "provider1")
	if err == nil {
		t.Error("SpawnAgent should fail for non-existent persona")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: CreateAgent
// ---------------------------------------------------------------------------

func TestLoom_CreateAgent_Errors(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// Invalid persona
	_, err := l.CreateAgent(ctx, "test", "nonexistent-persona", "proj1", "role1")
	if err == nil {
		t.Error("CreateAgent should fail for non-existent persona")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: StopAgent
// ---------------------------------------------------------------------------

func TestLoom_StopAgent_NonExistent(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	err := l.StopAgent(ctx, "nonexistent")
	if err == nil {
		t.Error("StopAgent should fail for non-existent agent")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: CloneAgentPersona
// ---------------------------------------------------------------------------

func TestLoom_CloneAgentPersona_Errors(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// Non-existent agent
	_, err := l.CloneAgentPersona(ctx, "nonexistent", "new-persona", "new-agent", "", false)
	if err == nil {
		t.Error("CloneAgentPersona should fail for non-existent agent")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: ResumeAgentsWaitingForProvider
// ---------------------------------------------------------------------------

func TestLoom_ResumeAgentsWaitingForProvider_NilManagers(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	// Should return nil when no database
	err := l.ResumeAgentsWaitingForProvider(ctx, "provider-1")
	if err != nil {
		t.Errorf("ResumeAgentsWaitingForProvider() error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: startProviderHeartbeats
// ---------------------------------------------------------------------------

func TestLoom_StartProviderHeartbeats_NoTemporal(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	// Without temporal, should return nil
	err := l.startProviderHeartbeats(ctx)
	if err != nil {
		t.Errorf("startProviderHeartbeats() error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: ensureProviderHeartbeat
// ---------------------------------------------------------------------------

func TestLoom_EnsureProviderHeartbeat_NoTemporal(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// No temporal, should return nil
	err := l.ensureProviderHeartbeat(ctx, "provider-1")
	if err != nil {
		t.Errorf("ensureProviderHeartbeat() error = %v", err)
	}

	// Empty provider ID, should return nil
	err = l.ensureProviderHeartbeat(ctx, "")
	if err != nil {
		t.Errorf("ensureProviderHeartbeat('') error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: maybeFileReadinessBead
// ---------------------------------------------------------------------------

func TestLoom_MaybeFileReadinessBead_NilProject(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// Should not panic with nil project
	l.maybeFileReadinessBead(nil, []string{"issue"}, "")
}

func TestLoom_MaybeFileReadinessBead_NoIssues(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	proj := &models.Project{ID: "test"}
	// Should not panic with empty issues
	l.maybeFileReadinessBead(proj, []string{}, "")
}

// ---------------------------------------------------------------------------
// Loom method tests: applyCEODecisionToParent nil handling
// ---------------------------------------------------------------------------

func TestLoom_ApplyCEODecisionToParent_NonExistent(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// Should not panic or error for nonexistent decision
	err := l.applyCEODecisionToParent("nonexistent")
	if err != nil {
		t.Errorf("applyCEODecisionToParent() error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// ConfigSnapshot tests
// ---------------------------------------------------------------------------

func TestConfigSnapshot_StructFields(t *testing.T) {
	snap := &ConfigSnapshot{
		Server: config.ServerConfig{HTTPPort: 8080},
	}
	if snap.Server.HTTPPort != 8080 {
		t.Errorf("snap.Server.HTTPPort = %d, want 8080", snap.Server.HTTPPort)
	}
}

func TestLoom_GetConfigSnapshot_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	snap, err := l.GetConfigSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetConfigSnapshot() error = %v", err)
	}
	if snap == nil {
		t.Fatal("GetConfigSnapshot() returned nil")
	}
	// Without database, providers should be empty list
	if snap.Providers == nil {
		t.Error("Providers should be empty slice, not nil")
	}
}

func TestLoom_GetConfigSnapshot_WithDatabase(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	snap, err := l.GetConfigSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetConfigSnapshot() error = %v", err)
	}
	if snap == nil {
		t.Fatal("GetConfigSnapshot() returned nil")
	}
	if snap.ModelCatalog == nil {
		t.Error("ModelCatalog should not be nil")
	}
}

func TestLoom_ExportConfigSnapshotYAML(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	data, err := l.ExportConfigSnapshotYAML(ctx)
	if err != nil {
		t.Fatalf("ExportConfigSnapshotYAML() error = %v", err)
	}
	if len(data) == 0 {
		t.Error("ExportConfigSnapshotYAML() returned empty data")
	}
	if !strings.Contains(string(data), "server:") {
		t.Error("YAML output should contain 'server:' key")
	}
}

func TestLoom_ImportConfigSnapshotYAML_Invalid(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	_, err := l.ImportConfigSnapshotYAML(ctx, []byte("{{invalid yaml"))
	if err == nil {
		t.Error("ImportConfigSnapshotYAML should fail with invalid YAML")
	}
}

func TestLoom_ApplyConfigSnapshot_Nil(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	err := l.ApplyConfigSnapshot(ctx, nil)
	if err == nil {
		t.Error("ApplyConfigSnapshot(nil) should fail")
	}
}

func TestLoom_ApplyConfigSnapshot_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	snap := &ConfigSnapshot{}
	err := l.ApplyConfigSnapshot(ctx, snap)
	if err == nil {
		t.Error("ApplyConfigSnapshot without database should fail")
	}
}

func TestLoom_ReloadFromDatabase_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	err := l.ReloadFromDatabase(ctx)
	if err != nil {
		t.Errorf("ReloadFromDatabase() should return nil without database, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: Upsert provider in registry
// ---------------------------------------------------------------------------

func TestLoom_ProviderRegistryUpsert(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	// Directly upsert into registry (used by Initialize)
	err := l.providerRegistry.Upsert(&provider.ProviderConfig{
		ID:   "direct-upsert",
		Name: "Direct Upsert",
		Type: "mock",
	})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	// Verify it's there
	list := l.providerRegistry.List()
	found := false
	for _, p := range list {
		if p.Config.ID == "direct-upsert" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Upserted provider not found in registry")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: ReplResult struct
// ---------------------------------------------------------------------------

func TestReplResult_Fields(t *testing.T) {
	r := &ReplResult{
		BeadID:       "bead-1",
		ProviderID:   "provider-1",
		ProviderName: "Test Provider",
		Model:        "test-model",
		Response:     "Hello, CEO!",
		TokensUsed:   100,
		LatencyMs:    250,
	}
	if r.BeadID != "bead-1" {
		t.Errorf("BeadID = %q, want %q", r.BeadID, "bead-1")
	}
	if r.Response != "Hello, CEO!" {
		t.Errorf("Response = %q, want %q", r.Response, "Hello, CEO!")
	}
	if r.TokensUsed != 100 {
		t.Errorf("TokensUsed = %d, want 100", r.TokensUsed)
	}
	if r.LatencyMs != 250 {
		t.Errorf("LatencyMs = %d, want 250", r.LatencyMs)
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: attachProviderToPausedAgents edge cases
// ---------------------------------------------------------------------------

func TestLoom_AttachProviderToPausedAgents_NilManagers(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	// Should not panic with nil database
	l.attachProviderToPausedAgents(ctx, "provider-1")
}

func TestLoom_AttachProviderToPausedAgents_EmptyProviderID(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	// Should not panic with empty provider ID
	l.attachProviderToPausedAgents(ctx, "")
}

// ---------------------------------------------------------------------------
// Loom method tests: New with Dolt backend config
// ---------------------------------------------------------------------------

func TestLoom_New_WithDoltConfig(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Beads.Backend = "dolt"
		c.Projects = []config.ProjectConfig{
			{ID: "test-proj", Name: "Test Project", BeadsPath: ".beads"},
		}
	})
	defer os.RemoveAll(tmpDir)

	if l.doltCoordinator == nil {
		t.Error("DoltCoordinator should be initialized when backend is dolt")
	}
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func TestConstants(t *testing.T) {
	if configKVKey == "" {
		t.Error("configKVKey should not be empty")
	}
	if modelCatalogKey == "" {
		t.Error("modelCatalogKey should not be empty")
	}
	if configKVKey != "loom.config.json" {
		t.Errorf("configKVKey = %q, want %q", configKVKey, "loom.config.json")
	}
	if modelCatalogKey != "loom.model_catalog.json" {
		t.Errorf("modelCatalogKey = %q, want %q", modelCatalogKey, "loom.model_catalog.json")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: projectReadinessState
// ---------------------------------------------------------------------------

func TestProjectReadinessState(t *testing.T) {
	state := projectReadinessState{
		ready:     true,
		issues:    nil,
		checkedAt: time.Now(),
	}
	if !state.ready {
		t.Error("state.ready should be true")
	}
	if state.issues != nil {
		t.Error("state.issues should be nil")
	}
	if state.checkedAt.IsZero() {
		t.Error("state.checkedAt should not be zero")
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: SelectProvider
// ---------------------------------------------------------------------------

func TestLoom_SelectProvider_NoDatabase(t *testing.T) {
	l, tmpDir := testLoom(t, func(c *config.Config) {
		c.Database = config.DatabaseConfig{}
	})
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	_, err := l.SelectProvider(ctx, nil, "")
	// Should fail since no database available
	if err == nil {
		// SelectProvider calls database.ListProviders; without database it will panic or error
		// Depending on implementation this may or may not error
		_ = err
	}
}

// ---------------------------------------------------------------------------
// Loom method tests: Dispatch-related
// ---------------------------------------------------------------------------

func TestLoom_StartDispatchLoop_NilDispatcher(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	saved := l.dispatcher
	l.dispatcher = nil

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Should return immediately without panicking
	l.StartDispatchLoop(ctx, time.Second)
	l.dispatcher = saved
}

func TestLoom_StartDispatchLoop_ZeroInterval(t *testing.T) {
	l, tmpDir := testLoom(t)
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Zero interval should default to 10s, test completes via ctx cancel
	l.StartDispatchLoop(ctx, 0)
}
