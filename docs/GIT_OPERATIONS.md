# Git Operations for Managed Projects

AgentiCorp manages git repositories for all registered projects, handling clone, pull, commit, and push operations within its container environment.

## Overview

When you register a project with AgentiCorp, it:

1. **Clones** the project's git repository into `/app/src/<project-id>`
2. **Loads beads** from `.beads/beads/*.yaml` in the cloned repository
3. **Assigns agents** to work on those beads
4. **Commits** agent changes with descriptive messages
5. **Pushes** completed work back to the project's remote repository

All git operations are proxied through AgentiCorp's `gitops.Manager`, ensuring proper authentication, isolation, and error handling.

## Project Git Configuration

### Basic Configuration

```yaml
id: myapp
name: My Application
git_repo: https://github.com/user/myapp
branch: main
beads_path: .beads
git_auth_method: ssh
git_credential_id: myapp-deploy-key
```

### Git Authentication Methods

AgentiCorp supports multiple authentication methods:

| Method | Use Case | Configuration |
|--------|----------|---------------|
| `none` | Public repositories | No credentials needed |
| `ssh` | Private repos with SSH keys | Requires deploy key in keymanager |
| `token` | GitHub/GitLab personal access tokens | HTTPS with token injection |
| `basic` | Username/password | Stored in secrets manager |
| `git-helper` | Use system git credential helper | Inherits from container |

### SSH Key Authentication (Recommended)

```yaml
git_auth_method: ssh
git_credential_id: myapp-deploy-key
```

**Setup:**
1. Generate SSH key pair: `ssh-keygen -t ed25519 -f ~/.ssh/myapp-deploy`
2. Add public key to GitHub/GitLab as deploy key (read/write access)
3. Store private key in AgentiCorp keymanager with ID `myapp-deploy-key`
4. Configure project to use that credential ID

### Token Authentication

```yaml
git_auth_method: token
git_credential_id: github-pat-myapp
```

**Setup:**
1. Generate personal access token (GitHub Settings → Developer Settings → PAT)
2. Grant repo access
3. Store token in keymanager with ID `github-pat-myapp`
4. Configure project to use that credential

## Project Work Directory Structure

Each project gets an isolated work directory:

```
/app/src/
├── myapp/                    # Project ID
│   ├── .git/                 # Git metadata
│   ├── .beads/              # Beads directory
│   │   └── beads/           # Work items for agents
│   │       ├── bd-001.yaml
│   │       └── bd-002.yaml
│   ├── src/                 # Project source code
│   └── ...                  # Project files
└── another-project/
    └── ...
```

## Git Operations

### Clone on Registration

When a project is registered with `git_repo` configured, AgentiCorp automatically:

```go
// Performed by gitops.Manager.CloneProject()
git clone --single-branch --branch main https://github.com/user/myapp /app/src/myapp
```

**Metadata updated:**
- `work_dir`: `/app/src/myapp`
- `last_sync_at`: Current timestamp
- `last_commit_hash`: HEAD commit SHA

### Pull for Updates

Periodically or on demand, AgentiCorp syncs with remote:

```go
// Performed by gitops.Manager.PullProject()
cd /app/src/myapp
git pull --rebase
```

**Use cases:**
- Before agent starts new work
- On schedule (e.g., every hour)
- Triggered via API/UI

### Commit Agent Work

When an agent completes a bead:

```go
// Performed by gitops.Manager.CommitChanges()
cd /app/src/myapp
git add .
git commit -m "feat: Implement authentication (bd-123)

Completed by agent-pm-001
Bead: bd-123-implement-auth
"
```

**Commit metadata:**
- Author: Agent name and ID
- Message: Descriptive with bead reference
- Timestamp: When work completed

### Push to Remote

After commit, changes are pushed back:

```go
// Performed by gitops.Manager.PushChanges()
cd /app/src/myapp
git push origin main
```

**Error handling:**
- Conflicts: Pause bead, notify human
- Auth failure: Retry with credential refresh
- Network issues: Retry with exponential backoff

## API Endpoints

### Sync Project Repository

```bash
POST /api/v1/projects/{project_id}/git/sync
```

Pulls latest changes from remote.

**Response:**
```json
{
  "success": true,
  "last_commit_hash": "abc123def456",
  "last_sync_at": "2026-01-21T01:00:00Z"
}
```

### Commit Project Changes

```bash
POST /api/v1/projects/{project_id}/git/commit
Content-Type: application/json

{
  "message": "Update configuration",
  "author_name": "Agent PM",
  "author_email": "agent-pm@agenticorp.local"
}
```

### Push Project Changes

```bash
POST /api/v1/projects/{project_id}/git/push
```

### Get Git Status

```bash
GET /api/v1/projects/{project_id}/git/status
```

**Response:**
```json
{
  "work_dir": "/app/src/myapp",
  "branch": "main",
  "last_commit_hash": "abc123",
  "last_sync_at": "2026-01-21T01:00:00Z",
  "uncommitted_changes": true,
  "ahead_by": 2,
  "behind_by": 0
}
```

## Agent Git Workflow

1. **Agent picks up bead** from project's `.beads/beads/` directory
2. **Work is performed** - agent modifies files in work directory
3. **Changes are reviewed** - agent checks git diff
4. **Commit is created** - agent commits with descriptive message
5. **Push to remote** - changes go back to project repo
6. **Bead is closed** - marked complete

## Security Considerations

### Credential Management

- **SSH keys** stored encrypted in keymanager
- **Tokens** never logged or exposed
- **Credentials** scoped per project
- **Rotation** supported without downtime

### Container Isolation

- Each project's work dir is isolated
- Git operations run with limited privileges
- No cross-project access
- Ephemeral clones on container restart (unless persisted)

### Audit Trail

All git operations are logged:
```
[gitops] Project: myapp, Operation: commit, Agent: agent-001, Hash: abc123
[gitops] Project: myapp, Operation: push, Status: success, Duration: 1.2s
```

## Troubleshooting

### Clone Failures

**Symptom**: Project shows "git clone failed" error

**Common causes:**
- Invalid repository URL
- Authentication failure (wrong key/token)
- Network connectivity
- Repository doesn't exist

**Solution:**
1. Verify `git_repo` URL is correct
2. Check credential is valid and has access
3. Test connectivity: `docker exec agenticorp ping github.com`
4. Check logs: `docker logs agenticorp | grep gitops`

### Merge Conflicts

**Symptom**: `git pull` fails with conflict errors

**Solution:**
1. AgentiCorp pauses affected beads
2. Human intervention required
3. Resolve conflicts manually in work dir
4. Resume beads after resolution

### Authentication Issues

**Symptom**: "Permission denied" or "Authentication failed"

**For SSH:**
1. Verify public key added to GitHub/GitLab
2. Check private key in keymanager
3. Ensure deploy key has write access

**For Token:**
1. Verify token hasn't expired
2. Check token has `repo` scope
3. Regenerate token if needed

## Best Practices

### Repository Organization

```
your-project/
├── .beads/
│   └── beads/          # Work items for AgentiCorp
│       ├── bd-001.yaml
│       └── bd-002.yaml
├── src/                # Source code
├── docs/               # Documentation
└── README.md
```

### Branch Strategy

- **main/master**: Protected, requires PR reviews
- **agenticorp**: Branch for agent work (recommended)
- **feature/***: Per-bead feature branches

Configure AgentiCorp to work on dedicated branch:
```yaml
git_repo: https://github.com/user/myapp
branch: agenticorp  # Agents work here, not main
```

### Commit Messages

Format: `<type>: <description> (<bead-id>)`

```
feat: Add user authentication (bd-123)
fix: Resolve login timeout issue (bd-124)
docs: Update API documentation (bd-125)
refactor: Simplify database queries (bd-126)
```

### Bead References

Always include bead ID in commits for traceability:
- Links commits to work items
- Enables impact analysis
- Supports rollback scenarios

## Integration Status

✅ **Fully Integrated** (as of v0.2)

- ✅ Clone, pull, commit, push operations
- ✅ SSH and token authentication
- ✅ Automatic clone/pull on startup for sticky projects
- ✅ API endpoints for all git operations
- ✅ Project work directory management
- ✅ Git metadata tracking (commit hash, sync time)

## Limitations (Current)

- ⚠️ No git LFS support yet
- ⚠️ No submodule support yet
- ⚠️ No merge conflict auto-resolution
- ⚠️ No branch creation/deletion APIs yet
- ⚠️ No UI controls yet (API only)

See future milestones for planned enhancements.

## Related Documentation

- [Project State Management](PROJECT_STATE_MANAGEMENT.md)
- [Beads Workflow](BEADS_WORKFLOW.md)
- [Security & Authentication](AUTH.md)
- [Agent System](WORKER_SYSTEM.md)
