package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// handleGitSync handles git pull for a project
func (s *Server) handleGitSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		http.Error(w, "project_id required", http.StatusBadRequest)
		return
	}

	// Get project
	project, err := s.agenticorp.GetProjectManager().GetProject(projectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Project not found: %v", err), http.StatusNotFound)
		return
	}

	if project.GitRepo == "" || project.GitRepo == "." {
		http.Error(w, "Project does not have a remote git repository", http.StatusBadRequest)
		return
	}

	// Pull latest changes
	gitops := s.agenticorp.GetGitopsManager()
	if err := gitops.PullProject(r.Context(), project); err != nil {
		http.Error(w, fmt.Sprintf("Failed to pull: %v", err), http.StatusInternalServerError)
		return
	}

	// Update project in database
	if err := s.agenticorp.GetProjectManager().UpdateProject(projectID, map[string]interface{}{
		"work_dir":         project.WorkDir,
		"last_sync_at":     project.LastSyncAt,
		"last_commit_hash": project.LastCommitHash,
	}); err != nil {
		// Log but don't fail
		fmt.Fprintf(w, "Warning: Failed to update project metadata: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":          true,
		"project_id":       projectID,
		"last_commit_hash": project.LastCommitHash,
		"last_sync_at":     project.LastSyncAt,
	})
}

// handleGitCommit handles committing changes for a project
func (s *Server) handleGitCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		http.Error(w, "project_id required", http.StatusBadRequest)
		return
	}

	var req struct {
		Message     string `json:"message"`
		AuthorName  string `json:"author_name"`
		AuthorEmail string `json:"author_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "commit message required", http.StatusBadRequest)
		return
	}

	// Get project
	project, err := s.agenticorp.GetProjectManager().GetProject(projectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Project not found: %v", err), http.StatusNotFound)
		return
	}

	if project.GitRepo == "" || project.GitRepo == "." {
		http.Error(w, "Project does not have a remote git repository", http.StatusBadRequest)
		return
	}

	// Commit changes
	gitops := s.agenticorp.GetGitopsManager()
	if err := gitops.CommitChanges(r.Context(), project, req.Message, req.AuthorName, req.AuthorEmail); err != nil {
		http.Error(w, fmt.Sprintf("Failed to commit: %v", err), http.StatusInternalServerError)
		return
	}

	// Update project in database
	if err := s.agenticorp.GetProjectManager().UpdateProject(projectID, map[string]interface{}{
		"last_commit_hash": project.LastCommitHash,
	}); err != nil {
		// Log but don't fail
		fmt.Fprintf(w, "Warning: Failed to update project metadata: %v\n", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":          true,
		"project_id":       projectID,
		"last_commit_hash": project.LastCommitHash,
	})
}

// handleGitPush handles pushing changes for a project
func (s *Server) handleGitPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		http.Error(w, "project_id required", http.StatusBadRequest)
		return
	}

	// Get project
	project, err := s.agenticorp.GetProjectManager().GetProject(projectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Project not found: %v", err), http.StatusNotFound)
		return
	}

	if project.GitRepo == "" || project.GitRepo == "." {
		http.Error(w, "Project does not have a remote git repository", http.StatusBadRequest)
		return
	}

	// Push changes
	gitops := s.agenticorp.GetGitopsManager()
	if err := gitops.PushChanges(r.Context(), project); err != nil {
		http.Error(w, fmt.Sprintf("Failed to push: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"project_id": projectID,
	})
}

// handleGitStatus handles getting git status for a project
func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		http.Error(w, "project_id required", http.StatusBadRequest)
		return
	}

	// Get project
	project, err := s.agenticorp.GetProjectManager().GetProject(projectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Project not found: %v", err), http.StatusNotFound)
		return
	}

	// Check if project has git repo
	if project.GitRepo == "" || project.GitRepo == "." {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"project_id": projectID,
			"has_git":    false,
		})
		return
	}

	gitops := s.agenticorp.GetGitopsManager()
	workDir := gitops.GetProjectWorkDir(projectID)

	// Get current commit hash
	commitHash, err := gitops.GetCurrentCommit(workDir)
	if err != nil {
		commitHash = "unknown"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"project_id":       projectID,
		"has_git":          true,
		"work_dir":         project.WorkDir,
		"branch":           project.Branch,
		"git_repo":         project.GitRepo,
		"last_commit_hash": commitHash,
		"last_sync_at":     project.LastSyncAt,
		"git_auth_method":  project.GitAuthMethod,
	})
}
