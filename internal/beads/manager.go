package beads

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jordanhubbard/loom/internal/observability"
	"github.com/jordanhubbard/loom/pkg/config"
	"github.com/jordanhubbard/loom/pkg/models"
	"gopkg.in/yaml.v3"
)

// Manager integrates with the bd (beads) CLI tool
type Manager struct {
	bdPath          string
	beadsPath       string
	mu              sync.RWMutex
	beads           map[string]*models.Bead
	beadFiles       map[string]string
	workGraph       *models.WorkGraph
	nextID          int               // For generating IDs when bd CLI is not available
	projectPrefixes map[string]string // Project ID -> bead prefix (e.g., "loom-self" -> "ac")
	projectNextIDs  map[string]int    // Per-project next ID counter
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
		nextID:          1,
		projectPrefixes: make(map[string]string),
		projectNextIDs:  make(map[string]int),
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
	m.projectPrefixes = make(map[string]string)
	m.projectNextIDs = make(map[string]int)
}

// SetBeadsPath sets the path to the beads directory
func (m *Manager) SetBeadsPath(path string) {
	m.beadsPath = path
}

// SetProjectPrefix sets the bead ID prefix for a project
func (m *Manager) SetProjectPrefix(projectID, prefix string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.projectPrefixes[projectID] = prefix
	if _, exists := m.projectNextIDs[projectID]; !exists {
		m.projectNextIDs[projectID] = 1
	}
}

// GetProjectPrefix returns the prefix for a project, defaulting to "bd" if not set
func (m *Manager) GetProjectPrefix(projectID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if prefix, ok := m.projectPrefixes[projectID]; ok && prefix != "" {
		return prefix
	}
	return "bd" // Default prefix
}

// LoadProjectPrefixFromConfig reads the issue-prefix from a project's .beads/config.yaml
func (m *Manager) LoadProjectPrefixFromConfig(projectID, beadsPath string) error {
	configPath := filepath.Join(beadsPath, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Config doesn't exist, use default
		}
		return fmt.Errorf("failed to read beads config: %w", err)
	}

	var config struct {
		IssuePrefix string `yaml:"issue-prefix"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse beads config: %w", err)
	}

	if config.IssuePrefix != "" {
		m.SetProjectPrefix(projectID, config.IssuePrefix)
	}
	return nil
}

// CreateBead creates a new bead using bd CLI or filesystem fallback
func (m *Manager) CreateBead(title, description string, priority models.BeadPriority, beadType, projectID string) (*models.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var beadID string
	var bead *models.Bead

	// Get project-specific prefix
	prefix := "bd" // default
	if p, ok := m.projectPrefixes[projectID]; ok && p != "" {
		prefix = p
	}

	usedBD := false
	// Try bd CLI first if available
	if m.bdPath != "" {
		args := []string{"create", title, "-p", fmt.Sprintf("%d", priority)}

		if description != "" {
			args = append(args, "-d", description)
		}

		cmd := exec.Command(m.bdPath, args...)
		if dir := beadsRootDir(m.beadsPath); dir != "" {
			cmd.Dir = dir
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			outStr := strings.TrimSpace(string(output))
			// Auto-initialize if bd reports uninitialized database
			if strings.Contains(outStr, "database not initialized") || strings.Contains(outStr, "issue_prefix config is missing") {
				if initErr := m.tryAutoInitBD(prefix); initErr != nil {
					log.Printf("[Beads] bd auto-init failed: %v", initErr)
				} else {
					// Retry the create after init
					cmd2 := exec.Command(m.bdPath, args...)
					if dir := beadsRootDir(m.beadsPath); dir != "" {
						cmd2.Dir = dir
					}
					output, err = cmd2.CombinedOutput()
					if err != nil {
						log.Printf("[Beads] bd create failed after auto-init, falling back to filesystem: %v: %s", err, strings.TrimSpace(string(output)))
					}
				}
			} else {
				log.Printf("[Beads] bd create failed, falling back to filesystem: %v: %s", err, outStr)
			}
		}
		if err == nil {
			outputStr := string(output)
			beadID = m.extractBeadIDWithPrefix(outputStr, prefix)
			if beadID == "" {
				beadID = m.extractBeadID(outputStr)
			}
			if beadID == "" {
				log.Printf("[Beads] bd create did not return bead id, falling back to filesystem: %s", strings.TrimSpace(outputStr))
			} else {
				usedBD = true
			}
		}
	}

	// Fallback to filesystem-based bead creation
	if beadID == "" {
		// Get or initialize project-specific counter
		nextID := m.projectNextIDs[projectID]
		if nextID == 0 {
			nextID = 1
		}

		// Generate a new ID with project prefix
		beadID = fmt.Sprintf("%s-%03d", prefix, nextID)
		nextID++

		// Check for existing beads to avoid ID collision
		for {
			if _, exists := m.beads[beadID]; !exists {
				break
			}
			beadID = fmt.Sprintf("%s-%03d", prefix, nextID)
			nextID++
		}

		m.projectNextIDs[projectID] = nextID
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

	// Save to filesystem only when not using bd CLI
	if !usedBD {
		if err := m.SaveBeadToFilesystem(bead, m.beadsPath); err != nil {
			// Log error but don't fail - the bead is in memory
			fmt.Fprintf(os.Stderr, "Warning: failed to save bead to filesystem: %v\n", err)
		}
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

	previousAssigned := bead.AssignedTo
	assignedUpdated := false

	// Apply updates
	if status, ok := updates["status"].(models.BeadStatus); ok {
		bead.Status = status
		// Set closed_at timestamp if closing
		if status == models.BeadStatusClosed && bead.ClosedAt == nil {
			now := time.Now()
			bead.ClosedAt = &now
		}
		if status != models.BeadStatusClosed {
			bead.ClosedAt = nil
		}
	}
	if priority, ok := updates["priority"].(models.BeadPriority); ok {
		bead.Priority = priority
	}
	if title, ok := updates["title"].(string); ok {
		bead.Title = title
	}
	if beadType, ok := updates["type"].(string); ok {
		bead.Type = beadType
	}
	if projectID, ok := updates["project_id"].(string); ok {
		bead.ProjectID = projectID
	}
	if assignedTo, ok := updates["assigned_to"].(string); ok {
		bead.AssignedTo = assignedTo
		assignedUpdated = true
	}
	if description, ok := updates["description"].(string); ok {
		bead.Description = description
	}
	if parent, ok := updates["parent"].(string); ok {
		bead.Parent = parent
	}
	if tags, ok := updates["tags"].([]string); ok {
		bead.Tags = tags
	}
	if blockedBy, ok := updates["blocked_by"].([]string); ok {
		bead.BlockedBy = blockedBy
	}
	if blocks, ok := updates["blocks"].([]string); ok {
		bead.Blocks = blocks
	}
	if relatedTo, ok := updates["related_to"].([]string); ok {
		bead.RelatedTo = relatedTo
	}
	if children, ok := updates["children"].([]string); ok {
		bead.Children = children
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

	if assignedUpdated && previousAssigned != bead.AssignedTo {
		observability.Info("bead.assignment_updated", map[string]interface{}{
			"bead_id":              bead.ID,
			"project_id":           bead.ProjectID,
			"assigned_to":          bead.AssignedTo,
			"previous_assigned_to": previousAssigned,
		})
	}

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
		err := fmt.Errorf("bead not found: %s", beadID)
		observability.Error("bead.claim", map[string]interface{}{
			"agent_id": agentID,
			"bead_id":  beadID,
		}, err)
		return err
	}

	if bead.AssignedTo != "" && bead.AssignedTo != agentID {
		err := fmt.Errorf("bead already claimed by agent %s", bead.AssignedTo)
		observability.Error("bead.claim", map[string]interface{}{
			"agent_id":    agentID,
			"bead_id":     beadID,
			"assigned_to": bead.AssignedTo,
			"project_id":  bead.ProjectID,
		}, err)
		return err
	}

	bead.AssignedTo = agentID
	bead.Status = models.BeadStatusInProgress
	bead.UpdatedAt = time.Now()

	observability.Info("bead.claim", map[string]interface{}{
		"agent_id":   agentID,
		"bead_id":    bead.ID,
		"project_id": bead.ProjectID,
		"status":     "claimed",
	})

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

		// Check if all blockers are resolved.
		// Blockers not in the cache are treated as resolved (they're closed
		// beads that were excluded from the active-only load).
		allResolved := true
		for _, blockerID := range bead.BlockedBy {
			blocker, ok := m.beads[blockerID]
			if ok && blocker.Status != models.BeadStatusClosed {
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
	fields := strings.Fields(output)
	for _, field := range fields {
		cleaned := strings.Trim(field, ",.:;[](){}")
		if beadIDPattern.MatchString(cleaned) {
			return cleaned
		}
	}
	return ""
}

func (m *Manager) extractBeadIDWithPrefix(output, prefix string) string {
	// Look for pattern like "<prefix>-xxxxx" or "Created <prefix>-xxxxx"
	parts := strings.Fields(output)
	prefixDash := prefix + "-"
	for _, part := range parts {
		if strings.HasPrefix(part, prefixDash) {
			return part
		}
	}
	return ""
}

// tryAutoInitBD attempts to initialize the bd database when it's detected as uninitialized.
// Must be called with m.mu held.
func (m *Manager) tryAutoInitBD(prefix string) error {
	if m.bdPath == "" {
		return fmt.Errorf("bd path not set")
	}

	args := []string{"init", "--prefix", prefix}
	cmd := exec.Command(m.bdPath, args...)
	if dir := beadsRootDir(m.beadsPath); dir != "" {
		cmd.Dir = dir
	}
	output, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(output))
	if err != nil {
		// "already initialized" is fine
		if strings.Contains(outStr, "already initialized") {
			return nil
		}
		return fmt.Errorf("bd init --prefix %s failed: %w: %s", prefix, err, outStr)
	}
	log.Printf("[Beads] Auto-initialized bd database with prefix %q", prefix)
	return nil
}

var beadIDPattern = regexp.MustCompile(`(?i)\b[a-z0-9]{2,8}-[a-z0-9]+\b`)

func beadsRootDir(beadsPath string) string {
	if beadsPath == "" {
		return ""
	}

	cleaned := filepath.Clean(beadsPath)
	if filepath.Base(cleaned) == "beads" {
		cleaned = filepath.Dir(cleaned)
	}
	if filepath.Base(cleaned) == ".beads" {
		return filepath.Dir(cleaned)
	}
	return filepath.Dir(cleaned)
}

func (m *Manager) fetchBeadFromBD(id string) (*models.Bead, error) {
	// Execute: bd show <id> --json
	cmd := exec.Command(m.bdPath, "show", id, "--json")
	if dir := beadsRootDir(m.beadsPath); dir != "" {
		cmd.Dir = dir
	}
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

	if assignedTo, ok := filters["assigned_to"]; ok {
		switch value := assignedTo.(type) {
		case string:
			if bead.AssignedTo != value {
				return false
			}
		case []string:
			match := false
			for _, candidate := range value {
				if bead.AssignedTo == candidate {
					match = true
					break
				}
			}
			if !match {
				return false
			}
		}
	}

	return true
}

// RefreshBeads reloads the bead cache from the bd CLI / Dolt database.
// Call periodically to pick up beads created after startup.
func (m *Manager) RefreshBeads(projectID, beadsPath string) error {
	return m.LoadBeadsFromFilesystem(projectID, beadsPath)
}

// LoadBeadsFromFilesystem loads beads using bd CLI when available, with YAML fallback.
func (m *Manager) LoadBeadsFromFilesystem(projectID, beadsPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.bdPath != "" {
		if err := m.loadBeadsFromBD(projectID, beadsPath); err == nil {
			return nil
		} else {
			fmt.Fprintf(os.Stderr, "Warning: failed to load beads via bd CLI: %v\n", err)
		}
	}

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
		if bead.ProjectID == "" && projectID != "" {
			bead.ProjectID = projectID
		}
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

type bdIssue struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	Priority    int        `json:"priority"`
	IssueType   string     `json:"issue_type"`
	Assignee    string     `json:"assignee"`
	Labels      []string   `json:"labels"`
	Parent      string     `json:"parent"`
	Children    []string   `json:"children"`
	BlockedBy   []string   `json:"blocked_by"`
	Blocks      []string   `json:"blocks"`
	RelatedTo   []string   `json:"related_to"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at"`
}

func (m *Manager) loadBeadsFromBD(projectID, beadsPath string) error {
	// Only load non-closed beads to avoid loading thousands of historical entries.
	// Closed beads are not needed for dispatch, routing, or stuck detection.
	var allOutput []byte
	dir := beadsRootDir(beadsPath)
	for _, status := range []string{"open", "in_progress", "blocked"} {
		cmd := exec.Command(m.bdPath, "list", "--json", "--limit", "0", "--allow-stale", "--status="+status)
		if dir != "" {
			cmd.Dir = dir
		}
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("[BeadManager] bd list --status=%s failed: %v: %s", status, err, strings.TrimSpace(string(output)))
			continue
		}
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" || trimmed == "[]" {
			continue
		}
		// Strip array brackets and accumulate entries
		if idx := strings.Index(trimmed, "["); idx >= 0 {
			trimmed = trimmed[idx+1:]
		}
		if idx := strings.LastIndex(trimmed, "]"); idx >= 0 {
			trimmed = trimmed[:idx]
		}
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		if len(allOutput) > 0 {
			allOutput = append(allOutput, ',')
		}
		allOutput = append(allOutput, []byte(trimmed)...)
	}
	combined := "[" + string(allOutput) + "]"

	var issues []bdIssue
	if err := json.Unmarshal([]byte(combined), &issues); err != nil {
		return fmt.Errorf("failed to parse bd list output: %w", err)
	}

	for _, issue := range issues {
		beadType := issue.IssueType
		if beadType == "" {
			beadType = "task"
		}

		// If this bead was already loaded by a different project, keep the
		// original project assignment. This prevents a shared Dolt database
		// from clobbering project IDs on every refresh cycle.
		existingProjectID := projectID
		if existing, ok := m.beads[issue.ID]; ok && existing.ProjectID != "" && existing.ProjectID != projectID {
			existingProjectID = existing.ProjectID
		}

		bead := &models.Bead{
			ID:          issue.ID,
			Type:        beadType,
			Title:       issue.Title,
			Description: issue.Description,
			Status:      models.BeadStatus(issue.Status),
			Priority:    models.BeadPriority(issue.Priority),
			ProjectID:   existingProjectID,
			AssignedTo:  issue.Assignee,
			BlockedBy:   issue.BlockedBy,
			Blocks:      issue.Blocks,
			RelatedTo:   issue.RelatedTo,
			Parent:      issue.Parent,
			Children:    issue.Children,
			Tags:        issue.Labels,
			CreatedAt:   issue.CreatedAt,
			UpdatedAt:   issue.UpdatedAt,
			ClosedAt:    issue.ClosedAt,
		}

		m.beads[bead.ID] = bead
		m.workGraph.Beads[bead.ID] = bead
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

// SyncFederation syncs with all enabled federation peers.
func (m *Manager) SyncFederation(ctx context.Context, cfg *config.BeadsFederationConfig) error {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	var lastErr error
	for _, peer := range cfg.Peers {
		if !peer.Enabled {
			continue
		}
		if err := m.syncWithPeer(ctx, peer, cfg.SyncStrategy); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: federation sync with peer %s failed: %v\n", peer.Name, err)
			lastErr = err
		}
	}
	return lastErr
}

// syncWithPeer runs `bd federation sync --peer <name> [--strategy <s>]`.
func (m *Manager) syncWithPeer(ctx context.Context, peer config.FederationPeer, strategy string) error {
	args := []string{"federation", "sync", "--peer", peer.Name}
	if strategy != "" {
		args = append(args, "--strategy", strategy)
	}

	cmd := exec.CommandContext(ctx, m.bdPath, args...)
	if dir := beadsRootDir(m.beadsPath); dir != "" {
		cmd.Dir = dir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd federation sync --peer %s failed: %w: %s", peer.Name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// FederationStatus runs `bd federation status --json` and returns the raw JSON.
func (m *Manager) FederationStatus(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, m.bdPath, "federation", "status", "--json")
	if dir := beadsRootDir(m.beadsPath); dir != "" {
		cmd.Dir = dir
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("bd federation status failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return output, nil
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
