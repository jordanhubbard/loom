package gitops

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jordanhubbard/loom/internal/database"
	"github.com/jordanhubbard/loom/internal/keymanager"
	"github.com/jordanhubbard/loom/internal/observability"
	"github.com/jordanhubbard/loom/pkg/models"
)

// Manager handles git operations for managed projects
type Manager struct {
	baseWorkDir   string                    // Base directory for all project clones (e.g., /app/src)
	projectKeyDir string                    // Base directory for per-project SSH keys
	db            *database.Database        // Database for credential persistence (optional)
	keyManager    *keymanager.KeyManager    // Key manager for encryption (optional)
}

func logGitEvent(event string, project *models.Project, fields map[string]interface{}) {
	payload := make(map[string]interface{})
	if project != nil {
		payload["project_id"] = project.ID
		payload["git_repo"] = project.GitRepo
		payload["branch"] = project.Branch
	}
	for k, v := range fields {
		payload[k] = v
	}
	observability.Info(event, payload)
}

func logGitError(event string, project *models.Project, fields map[string]interface{}, err error) {
	payload := make(map[string]interface{})
	if project != nil {
		payload["project_id"] = project.ID
		payload["git_repo"] = project.GitRepo
		payload["branch"] = project.Branch
	}
	for k, v := range fields {
		payload[k] = v
	}
	observability.Error(event, payload, err)
}

func projectIDFromWorkDir(workDir string) string {
	return filepath.Base(workDir)
}

// NewManager creates a new git operations manager.
// db and km are optional — pass nil to disable database-backed key persistence.
func NewManager(baseWorkDir, projectKeyDir string, db *database.Database, km *keymanager.KeyManager) (*Manager, error) {
	// Ensure base work directory exists
	if err := os.MkdirAll(baseWorkDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base work directory: %w", err)
	}

	if projectKeyDir == "" {
		projectKeyDir = filepath.Join("/app/data", "projects")
	}
	if err := os.MkdirAll(projectKeyDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create project key directory: %w", err)
	}

	return &Manager{
		baseWorkDir:   baseWorkDir,
		projectKeyDir: projectKeyDir,
		db:            db,
		keyManager:    km,
	}, nil
}

// GetProjectKeyDir returns the base directory for per-project SSH keys.
func (m *Manager) GetProjectKeyDir() string {
	return m.projectKeyDir
}

// SetKeyManager sets the key manager for encrypted credential storage.
func (m *Manager) SetKeyManager(km *keymanager.KeyManager) {
	m.keyManager = km
}

// CloneProject clones a project's git repository into its work directory
func (m *Manager) CloneProject(ctx context.Context, project *models.Project) error {
	if project.GitRepo == "" {
		return fmt.Errorf("project %s has no git_repo configured", project.ID)
	}

	workDir := m.GetProjectWorkDir(project.ID)
	start := time.Now()
	logGitEvent("git.clone.start", project, map[string]interface{}{
		"work_dir": workDir,
	})

	// Check if already cloned
	if _, err := os.Stat(filepath.Join(workDir, ".git")); err == nil {
		return fmt.Errorf("project %s already cloned at %s", project.ID, workDir)
	}

	// Ensure work directory exists (may already contain ssh/ from key generation)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}

	// If the directory already has non-git contents (e.g. ssh/ from key generation),
	// use git init + fetch + checkout instead of clone
	dirEntries, _ := os.ReadDir(workDir)
	needsInitFetch := len(dirEntries) > 0

	var cloneErr error
	if needsInitFetch {
		// Init, add remote, fetch, checkout — works in non-empty directories
		branch := project.Branch
		if branch == "" {
			branch = "main"
		}

		steps := []struct {
			name string
			args []string
		}{
			{"init", []string{"init"}},
			{"remote add", []string{"remote", "add", "origin", project.GitRepo}},
			{"fetch", []string{"fetch", "--depth=1", "origin", branch}},
			{"checkout", []string{"checkout", "-b", branch, "FETCH_HEAD"}},
			{"set-upstream", []string{"branch", "--set-upstream-to=origin/" + branch, branch}},
		}

		for _, step := range steps {
			cmd := exec.CommandContext(ctx, "git", step.args...)
			cmd.Dir = workDir
			if err := m.configureAuth(cmd, project); err != nil {
				logGitError("git.clone.error", project, map[string]interface{}{
					"work_dir":    workDir,
					"duration_ms": time.Since(start).Milliseconds(),
					"step":        step.name,
				}, err)
				return fmt.Errorf("failed to configure git auth for %s: %w", step.name, err)
			}
			output, err := cmd.CombinedOutput()
			if err != nil {
				logGitError("git.clone.error", project, map[string]interface{}{
					"work_dir":    workDir,
					"duration_ms": time.Since(start).Milliseconds(),
					"step":        step.name,
					"output":      strings.TrimSpace(string(output)),
				}, err)
				cloneErr = fmt.Errorf("git %s failed: %w\nOutput: %s", step.name, err, string(output))
				break
			}
		}
	} else {
		// Clean directory — use normal git clone
		args := []string{"clone"}
		if project.Branch != "" {
			args = append(args, "--branch", project.Branch)
		}
		args = append(args, "--single-branch", project.GitRepo, workDir)

		cmd := exec.CommandContext(ctx, "git", args...)
		if err := m.configureAuth(cmd, project); err != nil {
			logGitError("git.clone.error", project, map[string]interface{}{
				"work_dir":    workDir,
				"duration_ms": time.Since(start).Milliseconds(),
			}, err)
			return fmt.Errorf("failed to configure git auth: %w", err)
		}

		output, err := cmd.CombinedOutput()
		if err != nil {
			logGitError("git.clone.error", project, map[string]interface{}{
				"work_dir":    workDir,
				"duration_ms": time.Since(start).Milliseconds(),
				"output":      strings.TrimSpace(string(output)),
			}, err)
			cloneErr = fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(output))
		}
	}

	if cloneErr != nil {
		return cloneErr
	}
	logGitEvent("git.clone.success", project, map[string]interface{}{
		"work_dir":    workDir,
		"duration_ms": time.Since(start).Milliseconds(),
	})

	// Update project metadata
	project.WorkDir = workDir
	project.LastSyncAt = timePtr(time.Now())

	// Get initial commit hash
	if hash, err := m.GetCurrentCommit(workDir); err == nil {
		project.LastCommitHash = hash
	}

	return nil
}

// PullProject pulls latest changes from remote
func (m *Manager) PullProject(ctx context.Context, project *models.Project) error {
	workDir := m.GetProjectWorkDir(project.ID)
	start := time.Now()
	logGitEvent("git.pull.start", project, map[string]interface{}{
		"work_dir": workDir,
	})

	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		return fmt.Errorf("project %s not cloned, call CloneProject first", project.ID)
	}

	// Stash any local changes (e.g. from bd init) before pulling
	stashCmd := exec.CommandContext(ctx, "git", "stash", "--include-untracked")
	stashCmd.Dir = workDir
	stashOutput, _ := stashCmd.CombinedOutput()
	didStash := !strings.Contains(string(stashOutput), "No local changes")

	cmd := exec.CommandContext(ctx, "git", "pull", "--rebase")
	cmd.Dir = workDir

	if err := m.configureAuth(cmd, project); err != nil {
		logGitError("git.pull.error", project, map[string]interface{}{
			"work_dir":    workDir,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return fmt.Errorf("failed to configure git auth: %w", err)
	}

	output, err := cmd.CombinedOutput()

	// Restore stashed changes regardless of pull outcome
	if didStash {
		popCmd := exec.CommandContext(ctx, "git", "stash", "pop")
		popCmd.Dir = workDir
		popCmd.CombinedOutput() // best-effort
	}

	if err != nil {
		logGitError("git.pull.error", project, map[string]interface{}{
			"work_dir":    workDir,
			"duration_ms": time.Since(start).Milliseconds(),
			"output":      strings.TrimSpace(string(output)),
		}, err)
		return fmt.Errorf("git pull failed: %w\nOutput: %s", err, string(output))
	}
	logGitEvent("git.pull.success", project, map[string]interface{}{
		"work_dir":    workDir,
		"duration_ms": time.Since(start).Milliseconds(),
	})

	// Update metadata
	project.LastSyncAt = timePtr(time.Now())
	if hash, err := m.GetCurrentCommit(workDir); err == nil {
		project.LastCommitHash = hash
	}

	return nil
}

// validateCommitMessage validates a commit message for security
// validateProjectID validates a project ID for security
func validateProjectID(projectID string) error {
	if projectID == "" {
		return fmt.Errorf("project ID is required")
	}

	// Maximum length check
	if len(projectID) > 100 {
		return fmt.Errorf("project ID too long (max 100 characters)")
	}

	// Only allow alphanumeric characters, hyphens, and underscores
	// This prevents path traversal and command injection
	for _, ch := range projectID {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			 (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return fmt.Errorf("project ID contains invalid character: %c (only alphanumeric, hyphens, and underscores allowed)", ch)
		}
	}

	// Prevent directory traversal patterns
	if strings.Contains(projectID, "..") || strings.Contains(projectID, "/.") || strings.Contains(projectID, "./") {
		return fmt.Errorf("project ID contains invalid path components")
	}

	return nil
}

// validateSSHKeyPath validates that an SSH key path is safe and within expected directory
func validateSSHKeyPath(keyPath, expectedBaseDir string) error {
	// Check the path exists
	if _, err := os.Stat(keyPath); err != nil {
		return fmt.Errorf("SSH key path does not exist: %w", err)
	}

	// Resolve to absolute path to prevent symlink attacks
	absPath, err := filepath.Abs(keyPath)
	if err != nil {
		return fmt.Errorf("failed to resolve SSH key path: %w", err)
	}

	absBaseDir, err := filepath.Abs(expectedBaseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve base directory: %w", err)
	}

	// Ensure the key is within the expected directory
	if !strings.HasPrefix(absPath, absBaseDir+string(os.PathSeparator)) {
		return fmt.Errorf("SSH key path is outside expected directory")
	}

	return nil
}

// shellEscape escapes a string for safe use in shell commands
func shellEscape(s string) string {
	// For paths in shell commands, wrap in single quotes and escape any single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func validateCommitMessage(message string) error {
	if message == "" {
		return fmt.Errorf("commit message is required")
	}

	// Maximum length check
	if len(message) > 10000 {
		return fmt.Errorf("commit message too long (max 10000 characters)")
	}

	// Check for shell metacharacters that could be dangerous in git hooks
	dangerousChars := []string{
		"$(", "`", "\n$(", "\n`", // Command substitution
		"&&", "||", ";", "|", // Command chaining
		">", "<", // Redirection (only if at start of line or after newline)
	}

	for _, char := range dangerousChars {
		if strings.Contains(message, char) {
			// Allow some cases in commit message body (after first line)
			lines := strings.Split(message, "\n")
			if len(lines) > 0 && !strings.Contains(lines[0], char) {
				// It's in the body, which is usually safer, but still check for command substitution
				if char == "$(" || char == "`" {
					return fmt.Errorf("commit message contains potentially dangerous pattern: %s", char)
				}
				continue
			}
			return fmt.Errorf("commit message contains potentially dangerous pattern: %s", char)
		}
	}

	return nil
}

// validateAuthorInfo validates author name and email for security
func validateAuthorInfo(name, email string) error {
	if name == "" && email == "" {
		return nil // Both empty is OK
	}

	if name == "" || email == "" {
		return fmt.Errorf("both author name and email must be provided together")
	}

	// Validate name (allow letters, numbers, spaces, hyphens, periods)
	if len(name) > 100 {
		return fmt.Errorf("author name too long (max 100 characters)")
	}
	if strings.ContainsAny(name, "<>;|&$`\n\r\t") {
		return fmt.Errorf("author name contains invalid characters")
	}

	// Validate email format
	if len(email) > 254 { // RFC 5321
		return fmt.Errorf("author email too long (max 254 characters)")
	}
	// Basic email validation
	if !strings.Contains(email, "@") || strings.ContainsAny(email, " \n\r\t;|&$`") {
		return fmt.Errorf("author email format invalid")
	}

	return nil
}

// CommitChanges commits all changes in the project work directory
func (m *Manager) CommitChanges(ctx context.Context, project *models.Project, message, authorName, authorEmail string) error {
	// Validate inputs for security
	if err := validateCommitMessage(message); err != nil {
		return fmt.Errorf("invalid commit message: %w", err)
	}
	if err := validateAuthorInfo(authorName, authorEmail); err != nil {
		return fmt.Errorf("invalid author info: %w", err)
	}
	workDir := m.GetProjectWorkDir(project.ID)
	start := time.Now()
	logGitEvent("git.commit.start", project, map[string]interface{}{
		"work_dir": workDir,
		"message":  message,
	})

	// Stage all changes
	if err := m.runGitCommand(ctx, workDir, "add", "."); err != nil {
		logGitError("git.commit.error", project, map[string]interface{}{
			"work_dir":    workDir,
			"duration_ms": time.Since(start).Milliseconds(),
			"step":        "add",
		}, err)
		return fmt.Errorf("git add failed: %w", err)
	}

	// Check if there are changes to commit
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCmd.Dir = workDir
	statusOutput, err := statusCmd.Output()
	if err != nil {
		logGitError("git.commit.error", project, map[string]interface{}{
			"work_dir":    workDir,
			"duration_ms": time.Since(start).Milliseconds(),
			"step":        "status",
		}, err)
		return fmt.Errorf("git status failed: %w", err)
	}

	if len(strings.TrimSpace(string(statusOutput))) == 0 {
		logGitEvent("git.commit.skipped", project, map[string]interface{}{
			"work_dir":    workDir,
			"duration_ms": time.Since(start).Milliseconds(),
			"reason":      "no_changes",
		})
		return nil // No changes to commit
	}

	// Commit with author info
	args := []string{"commit", "-m", message}
	if authorName != "" && authorEmail != "" {
		args = append(args, "--author", fmt.Sprintf("%s <%s>", authorName, authorEmail))
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir
	if authorName != "" && authorEmail != "" {
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("GIT_AUTHOR_NAME=%s", authorName),
			fmt.Sprintf("GIT_AUTHOR_EMAIL=%s", authorEmail),
			fmt.Sprintf("GIT_COMMITTER_NAME=%s", authorName),
			fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", authorEmail),
		)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		logGitError("git.commit.error", project, map[string]interface{}{
			"work_dir":    workDir,
			"duration_ms": time.Since(start).Milliseconds(),
			"output":      strings.TrimSpace(string(output)),
		}, err)
		return fmt.Errorf("git commit failed: %w\nOutput: %s", err, string(output))
	}
	logGitEvent("git.commit.success", project, map[string]interface{}{
		"work_dir":    workDir,
		"duration_ms": time.Since(start).Milliseconds(),
	})

	// Update commit hash
	if hash, err := m.GetCurrentCommit(workDir); err == nil {
		project.LastCommitHash = hash
	}

	return nil
}

// PushChanges pushes committed changes to remote
func (m *Manager) PushChanges(ctx context.Context, project *models.Project) error {
	workDir := m.GetProjectWorkDir(project.ID)
	start := time.Now()
	logGitEvent("git.push.start", project, map[string]interface{}{
		"work_dir": workDir,
	})

	cmd := exec.CommandContext(ctx, "git", "push")
	cmd.Dir = workDir

	if err := m.configureAuth(cmd, project); err != nil {
		logGitError("git.push.error", project, map[string]interface{}{
			"work_dir":    workDir,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return fmt.Errorf("failed to configure git auth: %w", err)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		logGitError("git.push.error", project, map[string]interface{}{
			"work_dir":    workDir,
			"duration_ms": time.Since(start).Milliseconds(),
			"output":      strings.TrimSpace(string(output)),
		}, err)
		return fmt.Errorf("git push failed: %w\nOutput: %s", err, string(output))
	}
	logGitEvent("git.push.success", project, map[string]interface{}{
		"work_dir":    workDir,
		"duration_ms": time.Since(start).Milliseconds(),
	})

	return nil
}

// Status returns git status for a project workdir.
func (m *Manager) Status(ctx context.Context, projectID string) (string, error) {
	workDir := m.GetProjectWorkDir(projectID)
	start := time.Now()
	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		err := fmt.Errorf("project %s not cloned", projectID)
		logGitError("git.status.error", &models.Project{ID: projectID}, map[string]interface{}{
			"work_dir": workDir,
		}, err)
		return "", err
	}
	output, err := m.runGitCommandWithOutput(ctx, workDir, "status", "-sb")
	if err != nil {
		logGitError("git.status.error", &models.Project{ID: projectID}, map[string]interface{}{
			"work_dir":    workDir,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return "", err
	}
	logGitEvent("git.status", &models.Project{ID: projectID}, map[string]interface{}{
		"work_dir":    workDir,
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return strings.TrimSpace(output), nil
}

// Diff returns git diff for a project workdir.
func (m *Manager) Diff(ctx context.Context, projectID string) (string, error) {
	workDir := m.GetProjectWorkDir(projectID)
	start := time.Now()
	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		err := fmt.Errorf("project %s not cloned", projectID)
		logGitError("git.diff.error", &models.Project{ID: projectID}, map[string]interface{}{
			"work_dir": workDir,
		}, err)
		return "", err
	}
	output, err := m.runGitCommandWithOutput(ctx, workDir, "diff")
	if err != nil {
		logGitError("git.diff.error", &models.Project{ID: projectID}, map[string]interface{}{
			"work_dir":    workDir,
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return "", err
	}
	logGitEvent("git.diff", &models.Project{ID: projectID}, map[string]interface{}{
		"work_dir":    workDir,
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return strings.TrimSpace(output), nil
}

// GetCurrentCommit returns the current commit SHA
func (m *Manager) GetCurrentCommit(workDir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetProjectWorkDir returns the work directory path for a project
func (m *Manager) GetProjectWorkDir(projectID string) string {
	// Always use baseWorkDir/projectID for cloned projects
	// The special case for loom-self was removed because in Docker,
	// the repo is cloned separately to baseWorkDir/loom-self even though
	// baseWorkDir/.git may exist from the image build.
	return filepath.Join(m.baseWorkDir, projectID)
}

// LoadBeadsFromProject loads beads from a project's cloned repository
func (m *Manager) LoadBeadsFromProject(project *models.Project) ([]models.Bead, error) {
	workDir := m.GetProjectWorkDir(project.ID)
	beadsDir := filepath.Join(workDir, project.BeadsPath, "beads")

	// Check if beads directory exists
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return nil, nil // No beads directory, return empty
	}

	// This would integrate with the existing bead loading logic
	// For now, return placeholder - actual implementation would use
	// the existing LoadBeadsFromFilesystem function
	return nil, nil
}

// configureAuth configures git authentication for a command
func (m *Manager) configureAuth(cmd *exec.Cmd, project *models.Project) error {
	switch project.GitAuthMethod {
	case models.GitAuthNone:
		// No auth needed
		return nil

	case models.GitAuthSSH:
		// Validate project ID for security (prevent command injection)
		if err := validateProjectID(project.ID); err != nil {
			return fmt.Errorf("invalid project ID: %w", err)
		}

		publicKey, err := m.EnsureProjectSSHKey(project.ID)
		if err != nil {
			return err
		}
		_ = publicKey
		sshKeyPath := m.projectPrivateKeyPath(project.ID)

		// Make absolute so it works regardless of cmd.Dir
		if !filepath.IsAbs(sshKeyPath) {
			if abs, err := filepath.Abs(sshKeyPath); err == nil {
				sshKeyPath = abs
			}
		}

		// Validate SSH key path is within expected directory (prevent path traversal)
		absKeyDir := m.projectKeyDir
		if !filepath.IsAbs(absKeyDir) {
			if abs, err := filepath.Abs(absKeyDir); err == nil {
				absKeyDir = abs
			}
		}
		if err := validateSSHKeyPath(sshKeyPath, absKeyDir); err != nil {
			return fmt.Errorf("invalid SSH key path for project %s: %w", project.ID, err)
		}

		if cmd.Env == nil {
			cmd.Env = os.Environ()
		}
		// Use shell escaping to prevent command injection in GIT_SSH_COMMAND
		escapedKeyPath := shellEscape(sshKeyPath)
		cmd.Env = append(cmd.Env,
			"GIT_TERMINAL_PROMPT=0",
			fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o IdentitiesOnly=yes -o UserKnownHostsFile=/home/loom/.ssh/known_hosts", escapedKeyPath),
		)
		return nil

	case models.GitAuthToken:
		// For HTTPS with token, we could inject into URL or use credential helper
		// This is a simplified approach - production would use credential helper
		return nil

	case models.GitAuthBasic:
		// Would integrate with secrets store for username/password
		return nil

	default:
		return fmt.Errorf("unsupported auth method: %s", project.GitAuthMethod)
	}
}

// runGitCommand is a helper to run git commands in a work directory
func (m *Manager) runGitCommand(ctx context.Context, workDir string, args ...string) error {
	start := time.Now()
	projectID := projectIDFromWorkDir(workDir)
	logGitEvent("git.command.start", &models.Project{ID: projectID}, map[string]interface{}{
		"work_dir": workDir,
		"args":     args,
	})
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		logGitError("git.command.error", &models.Project{ID: projectID}, map[string]interface{}{
			"work_dir":    workDir,
			"args":        args,
			"duration_ms": time.Since(start).Milliseconds(),
			"output":      strings.TrimSpace(string(output)),
		}, err)
		return fmt.Errorf("git command failed: %w\nOutput: %s", err, string(output))
	}
	logGitEvent("git.command.success", &models.Project{ID: projectID}, map[string]interface{}{
		"work_dir":    workDir,
		"args":        args,
		"duration_ms": time.Since(start).Milliseconds(),
	})

	return nil
}

func (m *Manager) runGitCommandWithOutput(ctx context.Context, workDir string, args ...string) (string, error) {
	start := time.Now()
	projectID := projectIDFromWorkDir(workDir)
	logGitEvent("git.command.start", &models.Project{ID: projectID}, map[string]interface{}{
		"work_dir": workDir,
		"args":     args,
	})
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		logGitError("git.command.error", &models.Project{ID: projectID}, map[string]interface{}{
			"work_dir":    workDir,
			"args":        args,
			"duration_ms": time.Since(start).Milliseconds(),
			"output":      strings.TrimSpace(string(output)),
		}, err)
		return "", fmt.Errorf("git %s failed: %w\nOutput: %s", strings.Join(args, " "), err, string(output))
	}
	logGitEvent("git.command.success", &models.Project{ID: projectID}, map[string]interface{}{
		"work_dir":    workDir,
		"args":        args,
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return string(output), nil
}

func (m *Manager) projectKeyDirForProject(projectID string) string {
	return filepath.Join(m.projectKeyDir, projectID, "ssh")
}

func (m *Manager) projectPrivateKeyPath(projectID string) string {
	return filepath.Join(m.projectKeyDirForProject(projectID), "id_ed25519")
}

func (m *Manager) projectPublicKeyPath(projectID string) string {
	return m.projectPrivateKeyPath(projectID) + ".pub"
}

// EnsureProjectSSHKey ensures an SSH keypair exists for the project and returns the public key.
func (m *Manager) EnsureProjectSSHKey(projectID string) (string, error) {
	if projectID == "" {
		return "", fmt.Errorf("project ID is required")
	}
	project := &models.Project{ID: projectID}
	start := time.Now()
	logGitEvent("git.ssh_key.ensure.start", project, map[string]interface{}{})

	keyDir := m.projectKeyDirForProject(projectID)
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		logGitError("git.ssh_key.ensure.error", project, map[string]interface{}{
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return "", fmt.Errorf("failed to create project ssh directory: %w", err)
	}

	privatePath := m.projectPrivateKeyPath(projectID)
	publicPath := m.projectPublicKeyPath(projectID)
	generated := false

	if _, err := os.Stat(privatePath); os.IsNotExist(err) {
		// Filesystem key missing — try restoring from database
		if m.restoreKeyFromDB(projectID) {
			logGitEvent("git.ssh_key.restored_from_db", project, map[string]interface{}{})
		} else {
			// No DB backup — generate new key
			if err := m.generateSSHKeyPair(privatePath); err != nil {
				logGitError("git.ssh_key.ensure.error", project, map[string]interface{}{
					"duration_ms": time.Since(start).Milliseconds(),
				}, err)
				return "", err
			}
			generated = true
		}
	}

	if _, err := os.Stat(publicPath); os.IsNotExist(err) {
		if err := m.writePublicKeyFromPrivate(privatePath, publicPath); err != nil {
			logGitError("git.ssh_key.ensure.error", project, map[string]interface{}{
				"duration_ms": time.Since(start).Milliseconds(),
			}, err)
			return "", err
		}
	}

	keyBytes, err := os.ReadFile(publicPath)
	if err != nil {
		logGitError("git.ssh_key.ensure.error", project, map[string]interface{}{
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return "", fmt.Errorf("failed to read public key: %w", err)
	}

	publicKey := strings.TrimSpace(string(keyBytes))

	// Persist newly generated key to database
	if generated {
		m.storeKeyInDB(projectID, publicKey)
	}

	logGitEvent("git.ssh_key.ensure.success", project, map[string]interface{}{
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return publicKey, nil
}

// GetProjectPublicKey returns the project's public SSH key, creating it if needed.
func (m *Manager) GetProjectPublicKey(projectID string) (string, error) {
	return m.EnsureProjectSSHKey(projectID)
}

// RotateProjectSSHKey regenerates the project's SSH keypair and returns the new public key.
func (m *Manager) RotateProjectSSHKey(projectID string) (string, error) {
	if projectID == "" {
		return "", fmt.Errorf("project ID is required")
	}
	project := &models.Project{ID: projectID}
	start := time.Now()
	logGitEvent("git.ssh_key.rotate.start", project, map[string]interface{}{})
	privatePath := m.projectPrivateKeyPath(projectID)
	publicPath := m.projectPublicKeyPath(projectID)
	_ = os.Remove(privatePath)
	_ = os.Remove(publicPath)
	if err := m.generateSSHKeyPair(privatePath); err != nil {
		logGitError("git.ssh_key.rotate.error", project, map[string]interface{}{
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return "", err
	}
	keyBytes, err := os.ReadFile(publicPath)
	if err != nil {
		logGitError("git.ssh_key.rotate.error", project, map[string]interface{}{
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return "", fmt.Errorf("failed to read public key: %w", err)
	}

	publicKey := strings.TrimSpace(string(keyBytes))

	// Update database with rotated key
	now := time.Now()
	m.storeKeyInDBWithRotation(projectID, publicKey, &now)

	logGitEvent("git.ssh_key.rotate.success", project, map[string]interface{}{
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return publicKey, nil
}

func (m *Manager) generateSSHKeyPair(privatePath string) error {
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-N", "", "-f", privatePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate ssh key: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Chmod(privatePath, 0600); err != nil {
		return fmt.Errorf("failed to set ssh key permissions: %w", err)
	}
	return nil
}

func (m *Manager) writePublicKeyFromPrivate(privatePath, publicPath string) error {
	cmd := exec.Command("ssh-keygen", "-y", "-f", privatePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to derive public key: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.WriteFile(publicPath, output, 0644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}
	return nil
}

// restoreKeyFromDB attempts to restore an SSH key from the database to the filesystem.
// Returns true if the key was successfully restored.
func (m *Manager) restoreKeyFromDB(projectID string) bool {
	if m.db == nil || m.keyManager == nil {
		return false
	}

	cred, err := m.db.GetCredentialByProjectID(projectID)
	if err != nil || cred == nil {
		return false
	}

	// Decrypt private key from KeyManager
	privateKeyData, err := m.keyManager.GetKey(cred.KeyID)
	if err != nil {
		log.Printf("[gitops] Failed to decrypt SSH key from DB for project %s: %v", projectID, err)
		return false
	}

	// Write to filesystem
	keyDir := m.projectKeyDirForProject(projectID)
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		log.Printf("[gitops] Failed to create SSH key directory for project %s: %v", projectID, err)
		return false
	}

	privatePath := m.projectPrivateKeyPath(projectID)
	publicPath := m.projectPublicKeyPath(projectID)

	if err := os.WriteFile(privatePath, []byte(privateKeyData), 0600); err != nil {
		log.Printf("[gitops] Failed to write private key for project %s: %v", projectID, err)
		return false
	}
	if err := os.WriteFile(publicPath, []byte(cred.PublicKey), 0644); err != nil {
		log.Printf("[gitops] Failed to write public key for project %s: %v", projectID, err)
		return false
	}

	return true
}

// storeKeyInDB persists the SSH key to the database via the KeyManager.
func (m *Manager) storeKeyInDB(projectID, publicKey string) {
	m.storeKeyInDBWithRotation(projectID, publicKey, nil)
}

// storeKeyInDBWithRotation persists the SSH key to DB, optionally recording a rotation timestamp.
func (m *Manager) storeKeyInDBWithRotation(projectID, publicKey string, rotatedAt *time.Time) {
	if m.db == nil || m.keyManager == nil {
		return
	}
	if !m.keyManager.IsUnlocked() {
		log.Printf("[gitops] Cannot store SSH key in DB: key manager is locked")
		return
	}

	privatePath := m.projectPrivateKeyPath(projectID)
	privateKeyBytes, err := os.ReadFile(privatePath)
	if err != nil {
		log.Printf("[gitops] Failed to read private key for DB storage (project %s): %v", projectID, err)
		return
	}

	// Store encrypted private key via KeyManager
	keyID := fmt.Sprintf("ssh-%s", projectID)
	if err := m.keyManager.StoreKey(keyID, fmt.Sprintf("SSH key for %s", projectID), "Auto-generated project deploy key", string(privateKeyBytes)); err != nil {
		log.Printf("[gitops] Failed to encrypt SSH key for project %s: %v", projectID, err)
		return
	}

	// Store credential metadata in database
	now := time.Now()
	cred := &models.Credential{
		ID:                  fmt.Sprintf("cred-%s", projectID),
		ProjectID:           projectID,
		Type:                "ssh_ed25519",
		PrivateKeyEncrypted: "keymanager", // Actual key is in KeyManager
		PublicKey:           publicKey,
		KeyID:               keyID,
		Description:         fmt.Sprintf("SSH deploy key for project %s", projectID),
		CreatedAt:           now,
		UpdatedAt:           now,
		RotatedAt:           rotatedAt,
	}

	if err := m.db.UpsertCredential(cred); err != nil {
		log.Printf("[gitops] Failed to store credential in DB for project %s: %v", projectID, err)
		return
	}

	logGitEvent("git.ssh_key.stored_in_db", &models.Project{ID: projectID}, map[string]interface{}{
		"key_id": keyID,
	})
}

// BackfillSSHCredentials checks all projects for filesystem-only SSH keys and stores them in the DB.
func (m *Manager) BackfillSSHCredentials(projects []*models.Project) {
	if m.db == nil || m.keyManager == nil {
		return
	}

	for _, p := range projects {
		// Check if credential already exists in DB
		existing, _ := m.db.GetCredentialByProjectID(p.ID)
		if existing != nil {
			continue
		}

		// Check if filesystem key exists
		privatePath := m.projectPrivateKeyPath(p.ID)
		if _, err := os.Stat(privatePath); os.IsNotExist(err) {
			continue
		}

		// Read public key
		publicKey, err := m.GetProjectPublicKey(p.ID)
		if err != nil {
			log.Printf("[gitops] Backfill: failed to read public key for project %s: %v", p.ID, err)
			continue
		}

		m.storeKeyInDB(p.ID, publicKey)
		log.Printf("[gitops] Backfill: stored SSH key for project %s in database", p.ID)
	}
}

// CheckRemoteAccess verifies that the configured git auth can access the remote.
func (m *Manager) CheckRemoteAccess(ctx context.Context, project *models.Project) error {
	if project == nil {
		return fmt.Errorf("project is required")
	}
	if project.GitRepo == "" || project.GitRepo == "." {
		return nil
	}
	start := time.Now()
	logGitEvent("git.ls_remote.start", project, map[string]interface{}{})
	cmd := exec.CommandContext(ctx, "git", "ls-remote", project.GitRepo, "HEAD")
	if err := m.configureAuth(cmd, project); err != nil {
		logGitError("git.ls_remote.error", project, map[string]interface{}{
			"duration_ms": time.Since(start).Milliseconds(),
		}, err)
		return err
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		logGitError("git.ls_remote.error", project, map[string]interface{}{
			"duration_ms": time.Since(start).Milliseconds(),
			"output":      strings.TrimSpace(string(output)),
		}, err)
		return fmt.Errorf("git ls-remote failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	logGitEvent("git.ls_remote.success", project, map[string]interface{}{
		"duration_ms": time.Since(start).Milliseconds(),
	})
	return nil
}

// Helper to create time pointer
func timePtr(t time.Time) *time.Time {
	return &t
}

// GitOperator interface implementation methods

// GetStatus returns git status as a structured response
func (m *Manager) GetStatus(ctx context.Context) (map[string]interface{}, error) {
	// For now, we don't have a way to get the current project context
	// This is a placeholder that returns an error
	// In a real implementation, we'd need to track the current project
	return nil, fmt.Errorf("GetStatus requires project context - use Status(projectID) instead")
}

// GetDiff returns git diff as a structured response
func (m *Manager) GetDiff(ctx context.Context, staged bool) (map[string]interface{}, error) {
	// For now, we don't have a way to get the current project context
	// This is a placeholder that returns an error
	return nil, fmt.Errorf("GetDiff requires project context - use Diff(projectID) instead")
}

// CreateBranch creates a new git branch for a bead
func (m *Manager) CreateBranch(ctx context.Context, beadID, description, baseBranch string) (map[string]interface{}, error) {
	// Extract projectID from beadID or use current project
	// For now, this is a placeholder implementation
	return map[string]interface{}{
		"branch":  fmt.Sprintf("bead-%s", beadID),
		"base":    baseBranch,
		"created": true,
	}, fmt.Errorf("CreateBranch not yet implemented - requires project context")
}

// Commit creates a git commit for a bead's changes
func (m *Manager) Commit(ctx context.Context, beadID, agentID, message string, files []string, allowAll bool) (map[string]interface{}, error) {
	// This would use CommitChanges internally
	// For now, this is a placeholder implementation
	return map[string]interface{}{
		"committed": true,
		"message":   message,
		"bead_id":   beadID,
		"agent_id":  agentID,
	}, fmt.Errorf("Commit not yet implemented - requires project context")
}

// Push pushes commits to remote for a bead
func (m *Manager) Push(ctx context.Context, beadID, branch string, setUpstream bool) (map[string]interface{}, error) {
	// This would use PushChanges internally
	// For now, this is a placeholder implementation
	return map[string]interface{}{
		"pushed":       true,
		"branch":       branch,
		"set_upstream": setUpstream,
		"bead_id":      beadID,
	}, fmt.Errorf("Push not yet implemented - requires project context")
}

// CreatePR creates a pull request for a bead
func (m *Manager) CreatePR(ctx context.Context, beadID, title, body, base, branch string, reviewers []string, draft bool) (map[string]interface{}, error) {
	// This would use gh CLI or GitHub API
	// For now, this is a placeholder implementation
	return map[string]interface{}{
		"created":   true,
		"bead_id":   beadID,
		"title":     title,
		"base":      base,
		"branch":    branch,
		"draft":     draft,
		"reviewers": reviewers,
	}, fmt.Errorf("CreatePR not yet implemented - requires GitHub integration")
}
