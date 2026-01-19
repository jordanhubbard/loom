package beads

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jordanhubbard/arbiter/pkg/models"
	"gopkg.in/yaml.v3"
)

// Manager integrates with the bd (beads) CLI tool
type Manager struct {
	bdPath    string
	beadsPath string
	mu        sync.RWMutex
	beads     map[string]*models.Bead
	beadFiles map[string]string
	workGraph *models.WorkGraph
	nextID    int // For generating IDs when bd CLI is not available
}

// NewManager creates a new beads manager
func NewManager(bdPath string) *Manager {
	return &Manager{
		bdPath:    bdPath,
		beadsPath: ".beads",
		beads:     make(map[string]*models.Bead),
		beadFiles: make(map[string]string),
		workGraph: &models.WorkGraph{
			Beads:     make(map[string]*models.Bead),
			Edges:     []models.Edge{},
			UpdatedAt: time.Now(),
		},
		nextID: 1,
	}
}

// Reset clears cached beads and work graph state.
func (m *Manager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.beads = make(map[string]*models.Bead)
	m.beadFiles = make(map[string]string)
	m.workGraph = &models.WorkGraph{Beads: make(map[string]*models.Bead), Edges: []models.Edge{}, UpdatedAt: time.Now()}
	m.nextID = 1
}

// SetBeadsPath sets the path to the beads directory
func (m *Manager) SetBeadsPath(path string) {
	m.beadsPath = path
}

// CreateBead creates a new bead using bd CLI or filesystem fallback
func (m *Manager) CreateBead(title, description string, priority models.BeadPriority, beadType, projectID string) (*models.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var beadID string
	var bead *models.Bead

	// Try bd CLI first if available
	if m.bdPath != "" {
		args := []string{"create", title, "-p", fmt.Sprintf("%d", priority)}

		if description != "" {
			args = append(args, "-d", description)
		}

		cmd := exec.Command(m.bdPath, args...)
		output, err := cmd.CombinedOutput()

		if err == nil {
			// Parse output to get bead ID
			outputStr := string(output)
			beadID = m.extractBeadID(outputStr)
		}
	}

	// Fallback to filesystem-based bead creation
	if beadID == "" {
		// Generate a new ID
		beadID = fmt.Sprintf("bd-%03d", m.nextID)
		m.nextID++

		// Check for existing beads to avoid ID collision
		for {
			if _, exists := m.beads[beadID]; !exists {
				break
			}
			beadID = fmt.Sprintf("bd-%03d", m.nextID)
			m.nextID++
		}
	}

	// Create internal bead representation
	bead = &models.Bead{
		ID:          beadID,
		Type:        beadType,
		Title:       title,
		Description: description,
		Status:      models.BeadStatusOpen,
		Priority:    priority,
		ProjectID:   projectID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	m.beads[beadID] = bead
	m.workGraph.Beads[beadID] = bead
	m.workGraph.UpdatedAt = time.Now()

	// Save to filesystem
	if err := m.SaveBeadToFilesystem(bead, m.beadsPath); err != nil {
		// Log error but don't fail - the bead is in memory
		fmt.Fprintf(os.Stderr, "Warning: failed to save bead to filesystem: %v\n", err)
	}

	return bead, nil
}

// GetBead retrieves a bead by ID
func (m *Manager) GetBead(id string) (*models.Bead, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bead, ok := m.beads[id]
	if !ok {
		// Try to fetch from bd
		return m.fetchBeadFromBD(id)
	}

	return bead, nil
}

// ListBeads returns all beads, optionally filtered
func (m *Manager) ListBeads(filters map[string]interface{}) ([]*models.Bead, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// In a full implementation, this would call bd with appropriate filters
	// For now, return from cache with basic filtering

	beads := make([]*models.Bead, 0, len(m.beads))

	for _, bead := range m.beads {
		if m.matchesFilters(bead, filters) {
			beads = append(beads, bead)
		}
	}

	return beads, nil
}

// UpdateBead updates a bead
func (m *Manager) UpdateBead(id string, updates map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	bead, ok := m.beads[id]
	if !ok {
		return fmt.Errorf("bead not found: %s", id)
	}

	// Apply updates
	if status, ok := updates["status"].(models.BeadStatus); ok {
		bead.Status = status
		// Set closed_at timestamp if closing
		if status == models.BeadStatusClosed && bead.ClosedAt == nil {
			now := time.Now()
			bead.ClosedAt = &now
		}
	}
	if priority, ok := updates["priority"].(models.BeadPriority); ok {
		bead.Priority = priority
	}
	if assignedTo, ok := updates["assigned_to"].(string); ok {
		bead.AssignedTo = assignedTo
	}
	if description, ok := updates["description"].(string); ok {
		bead.Description = description
	}
	if ctxUpdates, ok := updates["context"].(map[string]string); ok {
		if bead.Context == nil {
			bead.Context = make(map[string]string)
		}
		for k, v := range ctxUpdates {
			bead.Context[k] = v
		}
	}

	bead.UpdatedAt = time.Now()
	m.workGraph.UpdatedAt = time.Now()

	// Save to filesystem
	if err := m.SaveBeadToFilesystem(bead, m.beadsPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save bead to filesystem: %v\n", err)
	}

	return nil
}

// ClaimBead assigns a bead to an agent
func (m *Manager) ClaimBead(beadID, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	bead, ok := m.beads[beadID]
	if !ok {
		return fmt.Errorf("bead not found: %s", beadID)
	}

	if bead.AssignedTo != "" && bead.AssignedTo != agentID {
		return fmt.Errorf("bead already claimed by agent %s", bead.AssignedTo)
	}

	bead.AssignedTo = agentID
	bead.Status = models.BeadStatusInProgress
	bead.UpdatedAt = time.Now()

	if err := m.SaveBeadToFilesystem(bead, m.beadsPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save bead to filesystem: %v\n", err)
	}

	return nil
}

// AddDependency adds a dependency between beads
func (m *Manager) AddDependency(childID, parentID, relationship string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	child, ok := m.beads[childID]
	if !ok {
		return fmt.Errorf("child bead not found: %s", childID)
	}

	parent, ok := m.beads[parentID]
	if !ok {
		return fmt.Errorf("parent bead not found: %s", parentID)
	}

	// Update bead relationships
	switch relationship {
	case "blocks":
		child.BlockedBy = append(child.BlockedBy, parentID)
		parent.Blocks = append(parent.Blocks, childID)
		if child.Status == models.BeadStatusInProgress {
			child.Status = models.BeadStatusBlocked
		}
	case "parent":
		child.Parent = parentID
		parent.Children = append(parent.Children, childID)
	case "related":
		child.RelatedTo = append(child.RelatedTo, parentID)
		parent.RelatedTo = append(parent.RelatedTo, childID)
	default:
		return fmt.Errorf("unknown relationship: %s", relationship)
	}

	// Update work graph
	m.workGraph.Edges = append(m.workGraph.Edges, models.Edge{
		From:         childID,
		To:           parentID,
		Relationship: relationship,
	})
	m.workGraph.UpdatedAt = time.Now()

	return nil
}

// GetReadyBeads returns beads with no open blockers
func (m *Manager) GetReadyBeads(projectID string) ([]*models.Bead, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ready := make([]*models.Bead, 0)

	for _, bead := range m.beads {
		if projectID != "" && bead.ProjectID != projectID {
			continue
		}

		if bead.Status != models.BeadStatusOpen && bead.Status != models.BeadStatusInProgress {
			continue
		}

		// Check if all blockers are resolved
		allResolved := true
		for _, blockerID := range bead.BlockedBy {
			blocker, ok := m.beads[blockerID]
			if !ok || blocker.Status != models.BeadStatusClosed {
				allResolved = false
				break
			}
		}

		if allResolved {
			ready = append(ready, bead)
		}
	}

	return ready, nil
}

// UnblockBead removes a blocking dependency
func (m *Manager) UnblockBead(beadID, blockerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	bead, ok := m.beads[beadID]
	if !ok {
		return fmt.Errorf("bead not found: %s", beadID)
	}

	// Remove blocker
	for i, id := range bead.BlockedBy {
		if id == blockerID {
			bead.BlockedBy = append(bead.BlockedBy[:i], bead.BlockedBy[i+1:]...)
			break
		}
	}

	// If no more blockers, unblock
	if len(bead.BlockedBy) == 0 && bead.Status == models.BeadStatusBlocked {
		bead.Status = models.BeadStatusOpen
	}

	bead.UpdatedAt = time.Now()
	m.workGraph.UpdatedAt = time.Now()

	return nil
}

// GetWorkGraph returns the current work graph
func (m *Manager) GetWorkGraph(projectID string) (*models.WorkGraph, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if projectID == "" {
		return m.workGraph, nil
	}

	// Filter by project
	filteredGraph := &models.WorkGraph{
		Beads:     make(map[string]*models.Bead),
		Edges:     []models.Edge{},
		UpdatedAt: m.workGraph.UpdatedAt,
	}

	for id, bead := range m.workGraph.Beads {
		if bead.ProjectID == projectID {
			filteredGraph.Beads[id] = bead
		}
	}

	for _, edge := range m.workGraph.Edges {
		if _, ok := filteredGraph.Beads[edge.From]; ok {
			if _, ok := filteredGraph.Beads[edge.To]; ok {
				filteredGraph.Edges = append(filteredGraph.Edges, edge)
			}
		}
	}

	return filteredGraph, nil
}

// Helper functions

func (m *Manager) extractBeadID(output string) string {
	// Look for pattern like "bd-xxxxx" or "Created bd-xxxxx"
	parts := strings.Fields(output)
	for _, part := range parts {
		if strings.HasPrefix(part, "bd-") {
			return part
		}
	}
	return ""
}

func (m *Manager) fetchBeadFromBD(id string) (*models.Bead, error) {
	// Execute: bd show <id> --json
	cmd := exec.Command(m.bdPath, "show", id, "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bead: %w", err)
	}

	var bead models.Bead
	if err := json.Unmarshal(output, &bead); err != nil {
		return nil, fmt.Errorf("failed to parse bead: %w", err)
	}

	return &bead, nil
}

func (m *Manager) matchesFilters(bead *models.Bead, filters map[string]interface{}) bool {
	if projectID, ok := filters["project_id"].(string); ok {
		if bead.ProjectID != projectID {
			return false
		}
	}

	if status, ok := filters["status"].(models.BeadStatus); ok {
		if bead.Status != status {
			return false
		}
	}

	if beadType, ok := filters["type"].(string); ok {
		if bead.Type != beadType {
			return false
		}
	}

	return true
}

// LoadBeadsFromFilesystem loads beads from .beads directory when bd CLI is not available
func (m *Manager) LoadBeadsFromFilesystem(beadsPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	beadsDir := filepath.Join(beadsPath, "beads")

	// Check if directory exists
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return nil // No beads directory, skip silently
	}

	// Read all YAML files in beads directory
	entries, err := os.ReadDir(beadsDir)
	if err != nil {
		return fmt.Errorf("failed to read beads directory: %w", err)
	}

	loadedCount := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		beadPath := filepath.Join(beadsDir, entry.Name())
		data, err := os.ReadFile(beadPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to read bead file %s: %v\n", entry.Name(), err)
			continue // Skip files we can't read
		}

		var bead models.Bead
		if err := yaml.Unmarshal(data, &bead); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse bead file %s: %v\n", entry.Name(), err)
			continue // Skip invalid YAML
		}

		// Add to internal cache
		m.beads[bead.ID] = &bead
		m.workGraph.Beads[bead.ID] = &bead
		m.beadFiles[bead.ID] = beadPath
		loadedCount++
	}

	if loadedCount > 0 {
		fmt.Fprintf(os.Stderr, "Loaded %d bead(s) from %s\n", loadedCount, beadsDir)
	}

	m.workGraph.UpdatedAt = time.Now()
	return nil
}

// SaveBeadToFilesystem saves a bead to the filesystem
func (m *Manager) SaveBeadToFilesystem(bead *models.Bead, beadsPath string) error {
	beadsDir := filepath.Join(beadsPath, "beads")

	// Ensure directory exists
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		return fmt.Errorf("failed to create beads directory: %w", err)
	}

	// Preserve existing bead file path if we loaded it from disk, to avoid creating duplicates.
	beadPath := ""
	if existing, ok := m.beadFiles[bead.ID]; ok && existing != "" {
		beadPath = existing
	}
	if beadPath == "" {
		// Generate filename from bead ID and title
		filename := fmt.Sprintf("%s-%s.yaml", bead.ID, sanitizeFilename(bead.Title))
		beadPath = filepath.Join(beadsDir, filename)
		m.beadFiles[bead.ID] = beadPath
	}

	// Marshal to YAML
	data, err := yaml.Marshal(bead)
	if err != nil {
		return fmt.Errorf("failed to marshal bead: %w", err)
	}

	// Write to file
	if err := os.WriteFile(beadPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write bead file: %w", err)
	}

	return nil
}

// sanitizeFilename removes characters that aren't safe for filenames
func sanitizeFilename(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)
	// Replace spaces with hyphens
	s = strings.ReplaceAll(s, " ", "-")
	// Keep only alphanumeric and hyphens
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	// Limit length
	if result.Len() > 50 {
		return result.String()[:50]
	}
	return result.String()
}
