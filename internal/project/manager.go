package project

import (
	"fmt"
	"sync"
	"time"

	"github.com/jordanhubbard/arbiter/pkg/models"
)

// Manager manages projects
type Manager struct {
	projects map[string]*models.Project
	mu       sync.RWMutex
}

// NewManager creates a new project manager
func NewManager() *Manager {
	return &Manager{
		projects: make(map[string]*models.Project),
	}
}

// CreateProject creates a new project
func (m *Manager) CreateProject(name, gitRepo, branch, beadsPath string, context map[string]string) (*models.Project, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate project ID
	projectID := fmt.Sprintf("proj-%d", time.Now().Unix())

	if beadsPath == "" {
		beadsPath = ".beads"
	}

	project := &models.Project{
		ID:          projectID,
		Name:        name,
		GitRepo:     gitRepo,
		Branch:      branch,
		BeadsPath:   beadsPath,
		Context:     context,
		Status:      models.ProjectStatusOpen,
		IsPerpetual: false,
		Comments:    []models.ProjectComment{},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Agents:      []string{},
	}

	m.projects[projectID] = project

	return project, nil
}

// GetProject retrieves a project by ID
func (m *Manager) GetProject(id string) (*models.Project, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	project, ok := m.projects[id]
	if !ok {
		return nil, fmt.Errorf("project not found: %s", id)
	}

	return project, nil
}

// ListProjects returns all projects
func (m *Manager) ListProjects() []*models.Project {
	m.mu.RLock()
	defer m.mu.RUnlock()

	projects := make([]*models.Project, 0, len(m.projects))
	for _, project := range m.projects {
		projects = append(projects, project)
	}

	return projects
}

// UpdateProject updates a project
func (m *Manager) UpdateProject(id string, updates map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	project, ok := m.projects[id]
	if !ok {
		return fmt.Errorf("project not found: %s", id)
	}

	// Apply updates
	if name, ok := updates["name"].(string); ok {
		project.Name = name
	}
	if branch, ok := updates["branch"].(string); ok {
		project.Branch = branch
	}
	if context, ok := updates["context"].(map[string]string); ok {
		project.Context = context
	}

	project.UpdatedAt = time.Now()

	return nil
}

// AddAgentToProject adds an agent to a project
func (m *Manager) AddAgentToProject(projectID, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	project, ok := m.projects[projectID]
	if !ok {
		return fmt.Errorf("project not found: %s", projectID)
	}

	// Check if agent already in project
	for _, id := range project.Agents {
		if id == agentID {
			return nil // Already added
		}
	}

	project.Agents = append(project.Agents, agentID)
	project.UpdatedAt = time.Now()

	return nil
}

// RemoveAgentFromProject removes an agent from a project
func (m *Manager) RemoveAgentFromProject(projectID, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	project, ok := m.projects[projectID]
	if !ok {
		return fmt.Errorf("project not found: %s", projectID)
	}

	// Find and remove agent
	for i, id := range project.Agents {
		if id == agentID {
			project.Agents = append(project.Agents[:i], project.Agents[i+1:]...)
			project.UpdatedAt = time.Now()
			return nil
		}
	}

	return fmt.Errorf("agent not found in project: %s", agentID)
}

// DeleteProject deletes a project
func (m *Manager) DeleteProject(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.projects[id]; !ok {
		return fmt.Errorf("project not found: %s", id)
	}

	delete(m.projects, id)

	return nil
}

// LoadProjects loads projects from configuration
func (m *Manager) LoadProjects(projects []models.Project) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, project := range projects {
		// Create a copy
		p := project
		p.CreatedAt = time.Now()
		p.UpdatedAt = time.Now()
		if p.Agents == nil {
			p.Agents = []string{}
		}
		if p.Comments == nil {
			p.Comments = []models.ProjectComment{}
		}
		if p.Status == "" {
			p.Status = models.ProjectStatusOpen
		}
		m.projects[p.ID] = &p
	}

	return nil
}

// CloseProject closes a project if conditions are met
func (m *Manager) CloseProject(projectID, authorID, comment string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	project, ok := m.projects[projectID]
	if !ok {
		return fmt.Errorf("project not found: %s", projectID)
	}

	// Check if project is perpetual
	if project.IsPerpetual {
		return fmt.Errorf("cannot close perpetual project: %s", project.Name)
	}

	// Check if already closed
	if project.Status == models.ProjectStatusClosed {
		return fmt.Errorf("project already closed: %s", projectID)
	}

	// Update status
	project.Status = models.ProjectStatusClosed
	now := time.Now()
	project.ClosedAt = &now
	project.UpdatedAt = now

	// Add closure comment
	if comment != "" {
		commentID := fmt.Sprintf("comment-%d", time.Now().UnixNano())
		projectComment := models.ProjectComment{
			ID:        commentID,
			ProjectID: projectID,
			AuthorID:  authorID,
			Comment:   comment,
			Timestamp: now,
		}
		project.Comments = append(project.Comments, projectComment)
	}

	return nil
}

// ReopenProject reopens a closed project
func (m *Manager) ReopenProject(projectID, authorID, comment string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	project, ok := m.projects[projectID]
	if !ok {
		return fmt.Errorf("project not found: %s", projectID)
	}

	// Check if closed
	if project.Status != models.ProjectStatusClosed {
		return fmt.Errorf("project is not closed: %s", projectID)
	}

	// Update status
	project.Status = models.ProjectStatusReopened
	project.ClosedAt = nil
	now := time.Now()
	project.UpdatedAt = now

	// Add reopen comment
	if comment != "" {
		commentID := fmt.Sprintf("comment-%d", now.UnixNano())
		projectComment := models.ProjectComment{
			ID:        commentID,
			ProjectID: projectID,
			AuthorID:  authorID,
			Comment:   comment,
			Timestamp: now,
		}
		project.Comments = append(project.Comments, projectComment)
	}

	return nil
}

// AddComment adds a comment to a project
func (m *Manager) AddComment(projectID, authorID, comment string) (*models.ProjectComment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	project, ok := m.projects[projectID]
	if !ok {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	now := time.Now()
	commentID := fmt.Sprintf("comment-%d", now.UnixNano())
	projectComment := models.ProjectComment{
		ID:        commentID,
		ProjectID: projectID,
		AuthorID:  authorID,
		Comment:   comment,
		Timestamp: now,
	}

	project.Comments = append(project.Comments, projectComment)
	project.UpdatedAt = now

	return &projectComment, nil
}

// GetComments returns all comments for a project
func (m *Manager) GetComments(projectID string) ([]models.ProjectComment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	project, ok := m.projects[projectID]
	if !ok {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	return project.Comments, nil
}

// CanClose checks if a project can be closed (no open beads with work remaining)
func (m *Manager) CanClose(projectID string, hasOpenWork bool) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	project, ok := m.projects[projectID]
	if !ok {
		return false
	}

	// Perpetual projects can never close
	if project.IsPerpetual {
		return false
	}

	// Already closed
	if project.Status == models.ProjectStatusClosed {
		return false
	}

	// If there's open work, cannot close
	return !hasOpenWork
}

// SetPerpetual marks a project as perpetual (never closes)
func (m *Manager) SetPerpetual(projectID string, isPerpetual bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	project, ok := m.projects[projectID]
	if !ok {
		return fmt.Errorf("project not found: %s", projectID)
	}

	project.IsPerpetual = isPerpetual
	project.UpdatedAt = time.Now()

	return nil
}
