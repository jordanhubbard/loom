package project

import (
	"testing"

	"github.com/jordanhubbard/arbiter/pkg/models"
)

func TestProjectStateManagement(t *testing.T) {
	manager := NewManager()

	// Test creating a project
	project, err := manager.CreateProject(
		"Test Project",
		"https://github.com/test/repo",
		"main",
		".beads",
		map[string]string{"test": "context"},
	)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	if project.Status != models.ProjectStatusOpen {
		t.Errorf("Expected status Open, got %s", project.Status)
	}

	if project.IsPerpetual {
		t.Error("Expected IsPerpetual to be false by default")
	}

	// Test adding comments
	comment, err := manager.AddComment(project.ID, "agent-1", "Initial comment")
	if err != nil {
		t.Fatalf("Failed to add comment: %v", err)
	}

	if comment.Comment != "Initial comment" {
		t.Errorf("Expected comment 'Initial comment', got %s", comment.Comment)
	}

	// Test retrieving comments
	comments, err := manager.GetComments(project.ID)
	if err != nil {
		t.Fatalf("Failed to get comments: %v", err)
	}

	if len(comments) != 1 {
		t.Errorf("Expected 1 comment, got %d", len(comments))
	}

	// Test closing project
	err = manager.CloseProject(project.ID, "agent-1", "Project complete")
	if err != nil {
		t.Fatalf("Failed to close project: %v", err)
	}

	closedProject, _ := manager.GetProject(project.ID)
	if closedProject.Status != models.ProjectStatusClosed {
		t.Errorf("Expected status Closed, got %s", closedProject.Status)
	}

	if closedProject.ClosedAt == nil {
		t.Error("Expected ClosedAt to be set")
	}

	// Test reopening project
	err = manager.ReopenProject(project.ID, "agent-2", "Found more work")
	if err != nil {
		t.Fatalf("Failed to reopen project: %v", err)
	}

	reopenedProject, _ := manager.GetProject(project.ID)
	if reopenedProject.Status != models.ProjectStatusReopened {
		t.Errorf("Expected status Reopened, got %s", reopenedProject.Status)
	}

	if reopenedProject.ClosedAt != nil {
		t.Error("Expected ClosedAt to be nil after reopening")
	}

	// Check comments were added during state changes
	finalComments, _ := manager.GetComments(project.ID)
	if len(finalComments) < 3 {
		t.Errorf("Expected at least 3 comments (initial + close + reopen), got %d", len(finalComments))
	}
}

func TestPerpetualProject(t *testing.T) {
	manager := NewManager()

	// Create a project
	project, err := manager.CreateProject(
		"Perpetual Project",
		"https://github.com/test/perpetual",
		"main",
		".beads",
		map[string]string{},
	)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}

	// Mark as perpetual
	err = manager.SetPerpetual(project.ID, true)
	if err != nil {
		t.Fatalf("Failed to set perpetual: %v", err)
	}

	perpetualProject, _ := manager.GetProject(project.ID)
	if !perpetualProject.IsPerpetual {
		t.Error("Expected project to be perpetual")
	}

	// Try to close perpetual project (should fail)
	err = manager.CloseProject(project.ID, "agent-1", "Trying to close")
	if err == nil {
		t.Error("Expected error when closing perpetual project")
	}

	// Check project is still open
	stillOpen, _ := manager.GetProject(project.ID)
	if stillOpen.Status != models.ProjectStatusOpen {
		t.Errorf("Expected status Open, got %s", stillOpen.Status)
	}
}

func TestCanClose(t *testing.T) {
	manager := NewManager()

	// Create a project
	project, _ := manager.CreateProject(
		"Test Project",
		"https://github.com/test/repo",
		"main",
		".beads",
		map[string]string{},
	)

	// Can close when no open work
	if !manager.CanClose(project.ID, false) {
		t.Error("Expected CanClose to be true when no open work")
	}

	// Cannot close when open work exists
	if manager.CanClose(project.ID, true) {
		t.Error("Expected CanClose to be false when open work exists")
	}

	// Mark as perpetual
	manager.SetPerpetual(project.ID, true)

	// Cannot close perpetual project even without open work
	if manager.CanClose(project.ID, false) {
		t.Error("Expected CanClose to be false for perpetual project")
	}
}

func TestProjectNotFound(t *testing.T) {
	manager := NewManager()

	// Test operations on non-existent project
	_, err := manager.GetProject("non-existent")
	if err == nil {
		t.Error("Expected error for non-existent project")
	}

	err = manager.CloseProject("non-existent", "agent-1", "comment")
	if err == nil {
		t.Error("Expected error when closing non-existent project")
	}

	err = manager.ReopenProject("non-existent", "agent-1", "comment")
	if err == nil {
		t.Error("Expected error when reopening non-existent project")
	}

	_, err = manager.AddComment("non-existent", "agent-1", "comment")
	if err == nil {
		t.Error("Expected error when adding comment to non-existent project")
	}
}

func TestCloseAlreadyClosed(t *testing.T) {
	manager := NewManager()

	project, _ := manager.CreateProject(
		"Test Project",
		"https://github.com/test/repo",
		"main",
		".beads",
		map[string]string{},
	)

	// Close the project
	manager.CloseProject(project.ID, "agent-1", "First close")

	// Try to close again (should fail)
	err := manager.CloseProject(project.ID, "agent-1", "Second close")
	if err == nil {
		t.Error("Expected error when closing already closed project")
	}
}

func TestReopenNotClosed(t *testing.T) {
	manager := NewManager()

	project, _ := manager.CreateProject(
		"Test Project",
		"https://github.com/test/repo",
		"main",
		".beads",
		map[string]string{},
	)

	// Try to reopen without closing first (should fail)
	err := manager.ReopenProject(project.ID, "agent-1", "Reopen")
	if err == nil {
		t.Error("Expected error when reopening project that isn't closed")
	}
}
