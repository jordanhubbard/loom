package git

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// GitService provides safe git operations for agents
type GitService struct {
	projectPath   string
	projectID     string
	projectKeyDir string // Base directory for per-project SSH keys
	branchPrefix  string // Configurable branch prefix (default: "agent/")
	auditLogger   *AuditLogger
}

// NewGitService creates a new git service instance.
// projectKeyDir is optional — if empty, defaults to /app/data/projects.
func NewGitService(projectPath, projectID string, projectKeyDir ...string) (*GitService, error) {
	// Validate project path
	if !isGitRepo(projectPath) {
		return nil, fmt.Errorf("not a git repository: %s", projectPath)
	}

	// Initialize audit logger
	auditLogger, err := NewAuditLogger(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize audit logger: %w", err)
	}

	keyDir := filepath.Join("/app/data", "projects")
	if len(projectKeyDir) > 0 && projectKeyDir[0] != "" {
		keyDir = projectKeyDir[0]
	}

	return &GitService{
		projectPath:   projectPath,
		projectID:     projectID,
		projectKeyDir: keyDir,
		branchPrefix:  "agent/",
		auditLogger:   auditLogger,
	}, nil
}

// SetBranchPrefix configures the branch prefix (default: "agent/").
func (s *GitService) SetBranchPrefix(prefix string) {
	if prefix != "" {
		s.branchPrefix = prefix
	}
}

// CreateBranchRequest defines parameters for branch creation
type CreateBranchRequest struct {
	BeadID      string // Bead ID for branch naming
	Description string // Human-readable description
	BaseBranch  string // Base branch (default: current)
}

// CreateBranchResult contains branch creation results
type CreateBranchResult struct {
	BranchName string `json:"branch_name"` // Full branch name
	Created    bool   `json:"created"`     // True if newly created
	Existed    bool   `json:"existed"`     // True if already existed
}

// CreateBranch creates a new agent branch with proper naming
func (s *GitService) CreateBranch(ctx context.Context, req CreateBranchRequest) (*CreateBranchResult, error) {
	startTime := time.Now()

	// Generate branch name
	branchName := s.generateBranchName(req.BeadID, req.Description)

	// Validate branch name
	if err := validateBranchNameWithPrefix(branchName, s.branchPrefix); err != nil {
		s.auditLogger.LogOperation("create_branch", req.BeadID, "", false, err)
		return nil, fmt.Errorf("invalid branch name: %w", err)
	}

	// Check if branch already exists
	exists, err := s.branchExists(ctx, branchName)
	if err != nil {
		s.auditLogger.LogOperation("create_branch", req.BeadID, branchName, false, err)
		return nil, fmt.Errorf("failed to check branch existence: %w", err)
	}

	if exists {
		s.auditLogger.LogOperation("create_branch", req.BeadID, branchName, true, nil)
		return &CreateBranchResult{
			BranchName: branchName,
			Created:    false,
			Existed:    true,
		}, nil
	}

	// Create branch
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", branchName)
	if req.BaseBranch != "" {
		cmd = exec.CommandContext(ctx, "git", "checkout", "-b", branchName, req.BaseBranch)
	}
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.auditLogger.LogOperation("create_branch", req.BeadID, branchName, false, err)
		return nil, fmt.Errorf("git checkout failed: %w\nOutput: %s", err, output)
	}

	s.auditLogger.LogOperationWithDuration("create_branch", req.BeadID, branchName, true, nil, time.Since(startTime))

	return &CreateBranchResult{
		BranchName: branchName,
		Created:    true,
		Existed:    false,
	}, nil
}

// CommitRequest defines parameters for creating a commit
type CommitRequest struct {
	BeadID   string   // Bead ID for commit attribution
	AgentID  string   // Agent ID for commit attribution
	Message  string   // Commit message (will be validated)
	Files    []string // Files to stage (empty = all changes)
	AllowAll bool     // Allow staging all files (use with caution)
}

// CommitResult contains commit creation results
type CommitResult struct {
	CommitSHA    string   `json:"commit_sha"`    // Commit hash
	FilesChanged int      `json:"files_changed"` // Number of files changed
	Insertions   int      `json:"insertions"`    // Lines added
	Deletions    int      `json:"deletions"`     // Lines removed
	Files        []string `json:"files"`         // List of changed files
}

// Commit creates a new commit with proper attribution
func (s *GitService) Commit(ctx context.Context, req CommitRequest) (*CommitResult, error) {
	startTime := time.Now()

	// Auto-inject bead and agent metadata into commit message.
	// Agents provide the summary; we append the trailers.
	req.Message = ensureCommitMetadata(req.Message, req.BeadID, req.AgentID)

	// Stage files
	if err := s.stageFiles(ctx, req.Files, req.AllowAll); err != nil {
		s.auditLogger.LogOperation("commit", req.BeadID, "", false, err)
		return nil, fmt.Errorf("failed to stage files: %w", err)
	}

	// Check for secrets
	if err := s.checkForSecrets(ctx); err != nil {
		s.auditLogger.LogOperation("commit", req.BeadID, "", false, err)
		return nil, fmt.Errorf("secret detected: %w", err)
	}

	// Create commit
	cmd := exec.CommandContext(ctx, "git", "commit", "-m", req.Message)
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.auditLogger.LogOperation("commit", req.BeadID, "", false, err)
		return nil, fmt.Errorf("git commit failed: %w\nOutput: %s", err, output)
	}

	// Get commit SHA
	commitSHA, err := s.getLastCommitSHA(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit SHA: %w", err)
	}

	// Get commit stats
	stats, err := s.getCommitStats(ctx, commitSHA)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit stats: %w", err)
	}

	s.auditLogger.LogOperationWithDuration("commit", req.BeadID, commitSHA, true, nil, time.Since(startTime))

	return stats, nil
}

// PushRequest defines parameters for pushing to remote
type PushRequest struct {
	BeadID     string // Bead ID for audit logging
	Branch     string // Branch to push (default: current)
	SetUpstream bool   // Set upstream tracking (use -u flag)
	Force      bool   // Force push (use with extreme caution)
}

// PushResult contains push operation results
type PushResult struct {
	Branch  string `json:"branch"`  // Branch that was pushed
	Remote  string `json:"remote"`  // Remote name (usually "origin")
	Success bool   `json:"success"` // True if push succeeded
}

// Push pushes commits to remote repository
func (s *GitService) Push(ctx context.Context, req PushRequest) (*PushResult, error) {
	startTime := time.Now()

	// Get current branch if not specified
	branch := req.Branch
	if branch == "" {
		var err error
		branch, err = s.getCurrentBranch(ctx)
		if err != nil {
			s.auditLogger.LogOperation("push", req.BeadID, "", false, err)
			return nil, fmt.Errorf("failed to get current branch: %w", err)
		}
	}

	// Agents can push to any branch. Protected branch enforcement
	// is left to the remote (GitHub branch protection rules).

	// Block force push unless explicitly allowed
	if req.Force {
		s.auditLogger.LogOperation("push", req.BeadID, branch, false, fmt.Errorf("force push blocked"))
		return nil, fmt.Errorf("force push is not allowed")
	}

	// Configure SSH
	if err := s.configureSSH(); err != nil {
		s.auditLogger.LogOperation("push", req.BeadID, branch, false, err)
		return nil, fmt.Errorf("failed to configure SSH: %w", err)
	}

	// Build git push command
	args := []string{"push"}
	if req.SetUpstream {
		args = append(args, "-u")
	}
	args = append(args, "origin", branch)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.projectPath
	cmd.Env = s.buildEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.auditLogger.LogOperation("push", req.BeadID, branch, false, err)
		return nil, fmt.Errorf("git push failed: %w\nOutput: %s", err, output)
	}

	s.auditLogger.LogOperationWithDuration("push", req.BeadID, branch, true, nil, time.Since(startTime))

	return &PushResult{
		Branch:  branch,
		Remote:  "origin",
		Success: true,
	}, nil
}

// GetStatus returns current git status
func (s *GitService) GetStatus(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "status")
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git status failed: %w", err)
	}
	return string(output), nil
}

// GetDiff returns current git diff
func (s *GitService) GetDiff(ctx context.Context, staged bool) (string, error) {
	args := []string{"diff"}
	if staged {
		args = append(args, "--staged")
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff failed: %w", err)
	}
	return string(output), nil
}

// Helper functions

// generateBranchName creates a branch name following the {prefix}{bead-id}/{description} pattern
func (s *GitService) generateBranchName(beadID, description string) string {
	// Slugify description
	slug := slugify(description)

	// Limit length
	if len(slug) > 40 {
		slug = slug[:40]
	}

	return fmt.Sprintf("%s%s/%s", s.branchPrefix, beadID, slug)
}

// branchExists checks if a branch exists locally
func (s *GitService) branchExists(ctx context.Context, branchName string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", branchName)
	cmd.Dir = s.projectPath
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 128 {
			return false, nil // Branch doesn't exist
		}
		return false, err
	}
	return true, nil
}

// getCurrentBranch returns the current branch name
func (s *GitService) getCurrentBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// stageFiles stages files for commit
func (s *GitService) stageFiles(ctx context.Context, files []string, allowAll bool) error {
	if len(files) == 0 && !allowAll {
		return fmt.Errorf("no files specified and allowAll is false")
	}

	var args []string
	if allowAll {
		args = []string{"add", "-A"}
	} else {
		args = append([]string{"add"}, files...)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add failed: %w\nOutput: %s", err, output)
	}
	return nil
}

// checkForSecrets scans staged files for potential secrets
func (s *GitService) checkForSecrets(ctx context.Context) error {
	// Get list of staged files
	cmd := exec.CommandContext(ctx, "git", "diff", "--staged", "--name-only")
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get staged files: %w", err)
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, file := range files {
		if file == "" {
			continue
		}

		filePath := filepath.Join(s.projectPath, file)
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue // Skip files that can't be read
		}

		if hasSecrets(content) {
			return fmt.Errorf("potential secret detected in %s", file)
		}
	}

	return nil
}

// getLastCommitSHA returns the SHA of the last commit
func (s *GitService) getLastCommitSHA(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get commit SHA: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// getCommitStats returns statistics for a commit
func (s *GitService) getCommitStats(ctx context.Context, commitSHA string) (*CommitResult, error) {
	cmd := exec.CommandContext(ctx, "git", "show", "--stat", "--format=%H", commitSHA)
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit stats: %w", err)
	}

	// Parse output for file count and changes
	lines := strings.Split(string(output), "\n")
	var files []string
	var insertions, deletions int

	for _, line := range lines {
		if strings.Contains(line, "file changed") || strings.Contains(line, "files changed") {
			// Parse summary line: "X files changed, Y insertions(+), Z deletions(-)"
			_, _ = fmt.Sscanf(line, "%d files changed, %d insertions(+), %d deletions(-)", &insertions, &deletions)
		} else if strings.Contains(line, "|") {
			// File line: "path/to/file.go | 10 +++++++++++"
			parts := strings.Split(line, "|")
			if len(parts) > 0 {
				files = append(files, strings.TrimSpace(parts[0]))
			}
		}
	}

	return &CommitResult{
		CommitSHA:    commitSHA,
		FilesChanged: len(files),
		Insertions:   insertions,
		Deletions:    deletions,
		Files:        files,
	}, nil
}

// configureSSH configures SSH for git operations
func (s *GitService) configureSSH() error {
	keyPath := filepath.Join(s.projectKeyDir, s.projectID, "ssh", "id_ed25519")

	// Resolve to absolute path (git runs from projectPath, not cwd)
	if !filepath.IsAbs(keyPath) {
		abs, err := filepath.Abs(keyPath)
		if err == nil {
			keyPath = abs
		}
	}

	// Check if key exists
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return fmt.Errorf("SSH key not found: %s", keyPath)
	}

	// Set GIT_SSH_COMMAND environment variable
	os.Setenv("GIT_SSH_COMMAND", fmt.Sprintf("ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new", keyPath))

	return nil
}

// buildEnv builds environment variables for git commands
func (s *GitService) buildEnv() []string {
	env := os.Environ()

	// Add git SSH command if configured
	if sshCmd := os.Getenv("GIT_SSH_COMMAND"); sshCmd != "" {
		env = append(env, fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
	}

	return env
}

// Validation functions

var (
	protectedBranchPatterns = []string{
		"^main$",
		"^master$",
		"^production$",
		"^release/.*",
		"^hotfix/.*",
	}

	secretPatterns = []*regexp.Regexp{
		// Match high-entropy API keys (20+ alphanumeric chars with mixed case/digits)
		regexp.MustCompile(`(?i)api[_-]?key[_-]?[:=]\s*['"][a-zA-Z0-9]{20,}['"]`),
		regexp.MustCompile(`(?i)secret[_-]?key[_-]?[:=]\s*['"][a-zA-Z0-9]{20,}['"]`),
		regexp.MustCompile(`(?i)token[_-]?[:=]\s*['"][a-zA-Z0-9]{20,}['"]`),
		// AWS access key IDs have a known format
		regexp.MustCompile(`(?i)aws[_-]?access[_-]?key[_-]?id\s*[:=]\s*['"]AKIA[0-9A-Z]{16}['"]`),
		// Private key blocks
		regexp.MustCompile(`-----BEGIN (RSA|DSA|EC|OPENSSH) PRIVATE KEY-----`),
		// Note: password patterns removed — too many false positives on default/placeholder
		// passwords in source code (e.g., "loom-default-password"). Real password leaks are
		// better caught by the API key and token patterns above.
	}
)

// validateBranchName validates that a branch name follows the expected prefix pattern
func validateBranchName(branchName string) error {
	return validateBranchNameWithPrefix(branchName, "agent/")
}

// validateBranchNameWithPrefix validates branch name with a configurable prefix
func validateBranchNameWithPrefix(branchName, prefix string) error {
	if !strings.HasPrefix(branchName, prefix) {
		return fmt.Errorf("branch name must start with '%s', got: %s", prefix, branchName)
	}

	if len(branchName) > 72 {
		return fmt.Errorf("branch name too long (max 72 chars): %s", branchName)
	}

	// Check for invalid characters
	if strings.ContainsAny(branchName, " \t\n\r") {
		return fmt.Errorf("branch name contains whitespace: %s", branchName)
	}

	return nil
}

// ensureCommitMetadata auto-appends bead/agent trailers if not present,
// and truncates the summary line if too long. Agents just provide the
// human-readable summary; we handle the metadata.
// ensureCommitMetadata auto-appends bead/agent trailers if not present,
// and truncates the summary line if too long. Agents just provide the
// human-readable summary; we handle the metadata.
func ensureCommitMetadata(message, beadID, agentID string) string {
	if message == "" {
		message = "Update from agent"
	}

	// Truncate first line to 72 chars if needed
	lines := strings.SplitN(message, "\n", 2)
	if len(lines[0]) > 72 {
		lines[0] = lines[0][:69] + "..."
	}
	message = strings.Join(lines, "\n")

	// Append trailers if not already present
	if beadID != "" && !strings.Contains(message, beadID) {
		message += fmt.Sprintf("\n\nBead: %s", beadID)
	}
	if agentID != "" && !strings.Contains(message, "Agent:") {
		message += fmt.Sprintf("\nAgent: %s", agentID)
	}

	return message
}

// isProtectedBranch checks if a branch is protected
func isProtectedBranch(branchName string) bool {
	for _, pattern := range protectedBranchPatterns {
		matched, _ := regexp.MatchString(pattern, branchName)
		if matched {
			return true
		}
	}
	return false
}

// hasSecrets checks if content contains potential secrets
func hasSecrets(content []byte) bool {
	for _, pattern := range secretPatterns {
		if pattern.Match(content) {
			return true
		}
	}
	return false
}

// slugify converts a string to a URL-safe slug
func slugify(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace spaces and underscores with hyphens
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")

	// Remove non-alphanumeric characters (except hyphens)
	reg := regexp.MustCompile(`[^a-z0-9-]+`)
	s = reg.ReplaceAllString(s, "")

	// Remove consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	s = reg.ReplaceAllString(s, "-")

	// Trim hyphens from start and end
	s = strings.Trim(s, "-")

	return s
}

// isGitRepo checks if a directory is a git repository
func isGitRepo(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	return err == nil && info.IsDir()
}

// AuditLogger logs git operations for security audit
type AuditLogger struct {
	projectID string
	logPath   string
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(projectID string) (*AuditLogger, error) {
	logDir := filepath.Join(os.Getenv("HOME"), ".loom", "projects", projectID)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logPath := filepath.Join(logDir, "git_audit.log")

	return &AuditLogger{
		projectID: projectID,
		logPath:   logPath,
	}, nil
}

// LogOperation logs a git operation
func (l *AuditLogger) LogOperation(operation, beadID, ref string, success bool, err error) {
	l.LogOperationWithDuration(operation, beadID, ref, success, err, 0)
}

// LogOperationWithDuration logs a git operation with duration
func (l *AuditLogger) LogOperationWithDuration(operation, beadID, ref string, success bool, err error, duration time.Duration) {
	entry := map[string]interface{}{
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"operation":  operation,
		"bead_id":    beadID,
		"project_id": l.projectID,
		"ref":        ref,
		"success":    success,
		"duration_ms": duration.Milliseconds(),
	}

	if err != nil {
		entry["error"] = err.Error()
	}

	// Write to log file
	data, _ := json.Marshal(entry)
	f, err := os.OpenFile(l.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.Write(data)
	_, _ = f.Write([]byte("\n"))
}

// CreatePRRequest defines parameters for creating a pull request
type CreatePRRequest struct {
	BeadID      string   // Bead ID for tracking
	Title       string   // PR title (auto-generated if empty)
	Body        string   // PR description (auto-generated if empty)
	Base        string   // Base branch (default: main)
	Branch      string   // Source branch (default: current)
	Reviewers   []string // GitHub usernames to request reviews from
	Draft       bool     // Create as draft PR
}

// CreatePRResult contains PR creation results
type CreatePRResult struct {
	Number int    // PR number
	URL    string // PR URL
	Branch string // Source branch
	Base   string // Base branch
}

// CreatePR creates a pull request using gh CLI
func (s *GitService) CreatePR(ctx context.Context, req CreatePRRequest) (*CreatePRResult, error) {
	startTime := time.Now()
	var resultRef string
	var resultErr error
	defer func() {
		success := resultErr == nil
		s.auditLogger.LogOperationWithDuration("create_pr", req.BeadID, resultRef, success, resultErr, time.Since(startTime))
	}()

	// Check if gh CLI is available
	if !isGhCLIAvailable() {
		resultErr = fmt.Errorf("gh CLI not found (install from https://cli.github.com)")
		return nil, resultErr
	}

	// Get current branch if not specified
	branch := req.Branch
	if branch == "" {
		currentBranch, err := s.getCurrentBranch(ctx)
		if err != nil {
			resultErr = fmt.Errorf("failed to get current branch: %w", err)
			return nil, resultErr
		}
		branch = currentBranch
	}

	// Validate branch is an agent branch
	if !strings.HasPrefix(branch, "agent/") {
		resultErr = fmt.Errorf("can only create PR from agent branches (agent/*), got: %s", branch)
		return nil, resultErr
	}

	// Set default base branch
	base := req.Base
	if base == "" {
		base = "main"
	}

	// Validate not creating PR to protected branch from another protected branch
	if isProtectedBranch(base) && isProtectedBranch(branch) {
		resultErr = fmt.Errorf("cannot create PR between protected branches: %s -> %s", branch, base)
		return nil, resultErr
	}

	// Build gh pr create command
	args := []string{"pr", "create"}
	args = append(args, "--base", base)
	args = append(args, "--head", branch)

	// Add title
	if req.Title != "" {
		args = append(args, "--title", req.Title)
	}

	// Add body
	if req.Body != "" {
		args = append(args, "--body", req.Body)
	} else {
		// Default body
		args = append(args, "--body", fmt.Sprintf("Automated PR from bead %s", req.BeadID))
	}

	// Add reviewers
	for _, reviewer := range req.Reviewers {
		args = append(args, "--reviewer", reviewer)
	}

	// Draft mode
	if req.Draft {
		args = append(args, "--draft")
	}

	// Execute gh pr create
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		resultErr = fmt.Errorf("gh pr create failed: %w\nOutput: %s", err, string(output))
		return nil, resultErr
	}

	// Parse PR URL from output
	prURL := strings.TrimSpace(string(output))
	resultRef = prURL

	// Extract PR number from URL (e.g., https://github.com/owner/repo/pull/123)
	prNumber := extractPRNumber(prURL)

	result := &CreatePRResult{
		Number: prNumber,
		URL:    prURL,
		Branch: branch,
		Base:   base,
	}

	return result, nil
}

// MergeRequest defines parameters for merging branches
type MergeRequest struct {
	SourceBranch string // Branch to merge from
	Message      string // Merge commit message
	NoFF         bool   // Force merge commit (--no-ff), default true
	BeadID       string // Bead ID for audit trail
}

// MergeResult contains merge operation results
type MergeResult struct {
	MergedBranch string `json:"merged_branch"`
	CommitSHA    string `json:"commit_sha"`
	Success      bool   `json:"success"`
}

// Merge merges a branch into the current branch
func (s *GitService) Merge(ctx context.Context, req MergeRequest) (*MergeResult, error) {
	startTime := time.Now()

	// Validate source branch exists
	exists, err := s.branchExists(ctx, req.SourceBranch)
	if err != nil {
		s.auditLogger.LogOperation("merge", req.BeadID, req.SourceBranch, false, err)
		return nil, fmt.Errorf("failed to check branch: %w", err)
	}
	if !exists {
		err := fmt.Errorf("source branch does not exist: %s", req.SourceBranch)
		s.auditLogger.LogOperation("merge", req.BeadID, req.SourceBranch, false, err)
		return nil, err
	}

	// Check current branch is not protected (unless we're merging INTO a non-protected branch)
	currentBranch, err := s.getCurrentBranch(ctx)
	if err != nil {
		s.auditLogger.LogOperation("merge", req.BeadID, "", false, err)
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}
	if isProtectedBranch(currentBranch) {
		err := fmt.Errorf("cannot merge into protected branch: %s", currentBranch)
		s.auditLogger.LogOperation("merge", req.BeadID, currentBranch, false, err)
		return nil, err
	}

	// Build merge command
	args := []string{"merge"}
	if req.NoFF {
		args = append(args, "--no-ff")
	}
	if req.Message != "" {
		args = append(args, "-m", req.Message)
	}
	args = append(args, req.SourceBranch)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check for merge conflicts
		if strings.Contains(string(output), "CONFLICT") {
			// Abort the merge to leave working tree clean
			abortCmd := exec.CommandContext(ctx, "git", "merge", "--abort")
			abortCmd.Dir = s.projectPath
			_ = abortCmd.Run()
			err = fmt.Errorf("merge conflict detected, merge aborted: %s", string(output))
		}
		s.auditLogger.LogOperation("merge", req.BeadID, req.SourceBranch, false, err)
		return nil, fmt.Errorf("git merge failed: %w\nOutput: %s", err, output)
	}

	commitSHA, _ := s.getLastCommitSHA(ctx)
	s.auditLogger.LogOperationWithDuration("merge", req.BeadID, req.SourceBranch, true, nil, time.Since(startTime))

	return &MergeResult{
		MergedBranch: req.SourceBranch,
		CommitSHA:    commitSHA,
		Success:      true,
	}, nil
}

// RevertRequest defines parameters for reverting commits
type RevertRequest struct {
	CommitSHAs []string // Commit SHAs to revert
	BeadID     string   // Bead ID for audit trail
	Reason     string   // Reason for revert
}

// RevertResult contains revert operation results
type RevertResult struct {
	RevertedSHAs []string `json:"reverted_shas"`
	NewCommitSHA string   `json:"new_commit_sha"`
	Success      bool     `json:"success"`
}

// Revert reverts specific commit(s)
func (s *GitService) Revert(ctx context.Context, req RevertRequest) (*RevertResult, error) {
	startTime := time.Now()

	if len(req.CommitSHAs) == 0 {
		err := fmt.Errorf("no commit SHAs provided for revert")
		s.auditLogger.LogOperation("revert", req.BeadID, "", false, err)
		return nil, err
	}

	revertedSHAs := make([]string, 0, len(req.CommitSHAs))
	for _, sha := range req.CommitSHAs {
		args := []string{"revert", "--no-edit", sha}
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = s.projectPath
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Abort revert on conflict
			abortCmd := exec.CommandContext(ctx, "git", "revert", "--abort")
			abortCmd.Dir = s.projectPath
			_ = abortCmd.Run()
			s.auditLogger.LogOperation("revert", req.BeadID, sha, false, err)
			return nil, fmt.Errorf("git revert failed for %s: %w\nOutput: %s", sha, err, output)
		}
		revertedSHAs = append(revertedSHAs, sha)
	}

	commitSHA, _ := s.getLastCommitSHA(ctx)
	s.auditLogger.LogOperationWithDuration("revert", req.BeadID, strings.Join(revertedSHAs, ","), true, nil, time.Since(startTime))

	return &RevertResult{
		RevertedSHAs: revertedSHAs,
		NewCommitSHA: commitSHA,
		Success:      true,
	}, nil
}

// DeleteBranchRequest defines parameters for deleting a branch
type DeleteBranchRequest struct {
	Branch       string // Branch to delete
	DeleteRemote bool   // Also delete remote branch
}

// DeleteBranchResult contains branch deletion results
type DeleteBranchResult struct {
	Branch       string `json:"branch"`
	DeletedLocal bool   `json:"deleted_local"`
	DeletedRemote bool  `json:"deleted_remote"`
}

// DeleteBranch deletes a local (and optionally remote) branch
func (s *GitService) DeleteBranch(ctx context.Context, req DeleteBranchRequest) (*DeleteBranchResult, error) {
	startTime := time.Now()

	// Cannot delete protected branches
	if isProtectedBranch(req.Branch) {
		err := fmt.Errorf("cannot delete protected branch: %s", req.Branch)
		s.auditLogger.LogOperation("delete_branch", "", req.Branch, false, err)
		return nil, err
	}

	// Cannot delete current branch
	currentBranch, err := s.getCurrentBranch(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}
	if currentBranch == req.Branch {
		err := fmt.Errorf("cannot delete current branch: %s", req.Branch)
		s.auditLogger.LogOperation("delete_branch", "", req.Branch, false, err)
		return nil, err
	}

	result := &DeleteBranchResult{Branch: req.Branch}

	// Delete local branch
	cmd := exec.CommandContext(ctx, "git", "branch", "-d", req.Branch)
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try force delete if not merged
		cmd = exec.CommandContext(ctx, "git", "branch", "-D", req.Branch)
		cmd.Dir = s.projectPath
		output, err = cmd.CombinedOutput()
		if err != nil {
			s.auditLogger.LogOperation("delete_branch", "", req.Branch, false, err)
			return nil, fmt.Errorf("git branch delete failed: %w\nOutput: %s", err, output)
		}
	}
	result.DeletedLocal = true

	// Delete remote branch if requested
	if req.DeleteRemote {
		cmd = exec.CommandContext(ctx, "git", "push", "origin", "--delete", req.Branch)
		cmd.Dir = s.projectPath
		cmd.Env = s.buildEnv()
		output, err = cmd.CombinedOutput()
		if err != nil {
			// Remote delete failure is non-fatal (branch may not exist remotely)
			s.auditLogger.LogOperation("delete_branch_remote", "", req.Branch, false, err)
		} else {
			result.DeletedRemote = true
		}
	}

	s.auditLogger.LogOperationWithDuration("delete_branch", "", req.Branch, true, nil, time.Since(startTime))
	return result, nil
}

// CheckoutRequest defines parameters for switching branches
type CheckoutRequest struct {
	Branch string // Branch to switch to
}

// CheckoutResult contains checkout operation results
type CheckoutResult struct {
	Branch         string `json:"branch"`
	PreviousBranch string `json:"previous_branch"`
}

// Checkout switches to a different branch
func (s *GitService) Checkout(ctx context.Context, req CheckoutRequest) (*CheckoutResult, error) {
	startTime := time.Now()

	previousBranch, _ := s.getCurrentBranch(ctx)

	// Check for dirty working tree
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCmd.Dir = s.projectPath
	statusOutput, err := statusCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to check working tree: %w", err)
	}
	if strings.TrimSpace(string(statusOutput)) != "" {
		err := fmt.Errorf("working tree is dirty, commit or stash changes before switching branches")
		s.auditLogger.LogOperation("checkout", "", req.Branch, false, err)
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "git", "checkout", req.Branch)
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.auditLogger.LogOperation("checkout", "", req.Branch, false, err)
		return nil, fmt.Errorf("git checkout failed: %w\nOutput: %s", err, output)
	}

	s.auditLogger.LogOperationWithDuration("checkout", "", req.Branch, true, nil, time.Since(startTime))
	return &CheckoutResult{
		Branch:         req.Branch,
		PreviousBranch: previousBranch,
	}, nil
}

// LogRequest defines parameters for viewing commit history
type LogRequest struct {
	Branch   string // Branch to show log for (default: current)
	MaxCount int    // Maximum entries (default: 20)
}

// LogEntry represents a single commit log entry
type LogEntry struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

// Log returns structured commit history
func (s *GitService) Log(ctx context.Context, req LogRequest) ([]LogEntry, error) {
	maxCount := req.MaxCount
	if maxCount <= 0 {
		maxCount = 20
	}
	if maxCount > 100 {
		maxCount = 100
	}

	args := []string{"log", fmt.Sprintf("--max-count=%d", maxCount),
		"--format=%H|%an|%aI|%s"}
	if req.Branch != "" {
		args = append(args, req.Branch)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w\nOutput: %s", err, output)
	}

	var entries []LogEntry
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) < 4 {
			continue
		}
		entries = append(entries, LogEntry{
			SHA:     parts[0],
			Author:  parts[1],
			Date:    parts[2],
			Subject: parts[3],
		})
	}

	s.auditLogger.LogOperation("log", "", req.Branch, true, nil)
	return entries, nil
}

// Fetch fetches remote refs
func (s *GitService) Fetch(ctx context.Context) error {
	startTime := time.Now()

	cmd := exec.CommandContext(ctx, "git", "fetch", "--prune")
	cmd.Dir = s.projectPath
	cmd.Env = s.buildEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.auditLogger.LogOperation("fetch", "", "", false, err)
		return fmt.Errorf("git fetch failed: %w\nOutput: %s", err, output)
	}

	s.auditLogger.LogOperationWithDuration("fetch", "", "", true, nil, time.Since(startTime))
	return nil
}

// BranchInfo represents a branch with metadata
type BranchInfo struct {
	Name      string `json:"name"`
	IsCurrent bool   `json:"is_current"`
	IsRemote  bool   `json:"is_remote"`
	LastCommit string `json:"last_commit"`
}

// ListBranches lists local and remote branches
func (s *GitService) ListBranches(ctx context.Context) ([]BranchInfo, error) {
	// List local branches
	cmd := exec.CommandContext(ctx, "git", "branch", "-a", "--format=%(refname:short)|%(objectname:short)|%(HEAD)")
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git branch failed: %w\nOutput: %s", err, output)
	}

	var branches []BranchInfo
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		branches = append(branches, BranchInfo{
			Name:       name,
			IsCurrent:  strings.TrimSpace(parts[2]) == "*",
			IsRemote:   strings.HasPrefix(name, "origin/"),
			LastCommit: strings.TrimSpace(parts[1]),
		})
	}

	s.auditLogger.LogOperation("list_branches", "", "", true, nil)
	return branches, nil
}

// DiffBranchesRequest defines parameters for cross-branch diff
type DiffBranchesRequest struct {
	Branch1 string // First branch
	Branch2 string // Second branch
}

// DiffBranches returns diff between two branches
func (s *GitService) DiffBranches(ctx context.Context, req DiffBranchesRequest) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", fmt.Sprintf("%s...%s", req.Branch1, req.Branch2))
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff branches failed: %w\nOutput: %s", err, output)
	}

	s.auditLogger.LogOperation("diff_branches", "", fmt.Sprintf("%s...%s", req.Branch1, req.Branch2), true, nil)
	return string(output), nil
}

// StashSave saves the current working state to the stash
func (s *GitService) StashSave(ctx context.Context, message string) error {
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "-m", message)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git stash save failed: %w\nOutput: %s", err, output)
	}

	s.auditLogger.LogOperation("stash_save", "", message, true, nil)
	return nil
}

// StashPop restores the most recent stash
func (s *GitService) StashPop(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "stash", "pop")
	cmd.Dir = s.projectPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git stash pop failed: %w\nOutput: %s", err, output)
	}

	s.auditLogger.LogOperation("stash_pop", "", "", true, nil)
	return nil
}

// isGhCLIAvailable checks if gh CLI is installed and authenticated
func isGhCLIAvailable() bool {
	cmd := exec.Command("gh", "auth", "status")
	err := cmd.Run()
	return err == nil
}

// extractPRNumber extracts PR number from GitHub PR URL
func extractPRNumber(url string) int {
	// Match pattern: /pull/123
	re := regexp.MustCompile(`/pull/(\d+)`)
	matches := re.FindStringSubmatch(url)
	if len(matches) < 2 {
		return 0
	}
	var num int
	_, _ = fmt.Sscanf(matches[1], "%d", &num)
	return num
}
