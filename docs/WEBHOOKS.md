# GitHub Webhooks Integration

Loom integrates with GitHub webhooks to automatically respond to repository events and trigger agent workflows.

## Webhook Endpoint

**URL:** `POST /api/v1/webhooks/github`

**Authentication:** GitHub webhook signature verification (HMAC-SHA256)

## Configuration

### 1. Set Webhook Secret

Configure the webhook secret in your Loom configuration:

```yaml
# config.yaml
security:
  webhook_secret: "your-secret-here"
```

### 2. Configure GitHub Webhook

In your GitHub repository settings:

1. Go to **Settings** → **Webhooks** → **Add webhook**
2. Set **Payload URL**: `https://your-loom-instance.com/api/v1/webhooks/github`
3. Set **Content type**: `application/json`
4. Set **Secret**: Same value as `security.webhook_secret` in config
5. Select events to subscribe to (see below)

### 3. Subscribe to Events

Select the following events for full Loom integration:

**Code Review Workflow:**
- ✅ Pull requests
- ✅ Pull request reviews
- ✅ Pull request review comments

**Issue Tracking:**
- ✅ Issues
- ✅ Issue comments

**Release Management:**
- ✅ Releases

**Optional:**
- Push events (for commit tracking)
- Repository events (for project setup)

## Supported Events

### Pull Request Events

Triggers automated code review workflow.

**Event Type:** `pull_request`

**Actions Handled:**
- `opened` - PR created → Create review bead, assign to code-reviewer
- `reopened` - PR reopened → Create review bead
- `synchronize` - New commits pushed → Trigger re-review
- `ready_for_review` - Draft converted to ready → Create review bead
- `review_requested` - Review explicitly requested → Create review bead
- `closed` - PR closed/merged → Update review bead status

**Payload Example:**
```json
{
  "action": "opened",
  "number": 123,
  "pull_request": {
    "id": 456,
    "number": 123,
    "title": "Add feature X",
    "state": "open",
    "draft": false,
    "user": {"login": "author"},
    "head": {
      "ref": "feature-branch",
      "sha": "abc123"
    },
    "base": {
      "ref": "main",
      "sha": "def456"
    }
  },
  "repository": {
    "full_name": "owner/repo"
  }
}
```

**Loom Response:**
- Creates `pr-review` type bead
- Publishes `external.github_pr` event to event bus
- Triggers code review workflow
- Assigns to code-reviewer agent

### Issue Events

Tracks issues for project management and motivation system.

**Event Type:** `issues`

**Actions Handled:**
- `opened` - Issue created → Store as external event
- `closed` - Issue closed → Update tracking
- `reopened` - Issue reopened → Update tracking

**Payload Example:**
```json
{
  "action": "opened",
  "issue": {
    "number": 456,
    "title": "Bug: Login fails",
    "body": "Description...",
    "state": "open",
    "user": {"login": "reporter"},
    "labels": [
      {"name": "bug"},
      {"name": "priority:high"}
    ]
  },
  "repository": {
    "full_name": "owner/repo"
  }
}
```

**Loom Response:**
- Stores as motivation system event
- Publishes `external.github_issue` event
- Available for agent task creation

### Comment Events

Tracks comments on issues and PRs.

**Event Types:** `issue_comment`, `pull_request_review_comment`

**Actions Handled:**
- `created` - Comment added → Store for context

**Payload Example:**
```json
{
  "action": "created",
  "comment": {
    "id": 789,
    "body": "Comment text...",
    "user": {"login": "commenter"}
  },
  "issue": {
    "number": 123
  }
}
```

**Loom Response:**
- Stores comment for context
- Publishes `external.github_comment` event
- May trigger agent responses

### Release Events

Tracks releases for deployment workflows.

**Event Type:** `release`

**Actions Handled:**
- `published` - Release published → Trigger deployment workflows

**Payload Example:**
```json
{
  "action": "published",
  "release": {
    "tag_name": "v1.2.3",
    "name": "Release 1.2.3",
    "body": "Release notes...",
    "draft": false,
    "prerelease": false,
    "author": {"login": "releaser"}
  }
}
```

**Loom Response:**
- Publishes `external.release` event
- Available for deployment automation

## Event Processing Flow

```
GitHub Event
    ↓
POST /api/v1/webhooks/github
    ↓
Verify HMAC-SHA256 Signature
    ↓
Parse Event Type (X-GitHub-Event header)
    ↓
Process Event
    ├─→ Create Review Bead (PR events)
    ├─→ Store External Event (all events)
    └─→ Publish to Event Bus
         ↓
    Agent Workflows Triggered
```

## Security

### Signature Verification

All webhook payloads are verified using HMAC-SHA256:

```go
func verifyGitHubSignature(payload []byte, signature, secret string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(payload)
    expected := hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(signature), []byte(expected))
}
```

**Header:** `X-Hub-Signature-256: sha256=<signature>`

If signature verification fails, the webhook returns `401 Unauthorized`.

### Best Practices

1. **Use Strong Secrets**: Generate webhook secret with high entropy
   ```bash
   openssl rand -hex 32
   ```

2. **HTTPS Only**: Always use HTTPS for webhook endpoint in production

3. **Rate Limiting**: GitHub may send bursts of webhooks, ensure your server can handle load

4. **Idempotency**: Handle duplicate webhook deliveries gracefully

5. **Timeout Handling**: GitHub expects response within 10 seconds

## Webhook Status Endpoint

Check webhook configuration and status:

**Endpoint:** `GET /api/v1/webhooks/status`

**Response:**
```json
{
  "github_webhook_enabled": true,
  "webhook_secret_configured": true,
  "motivation_engine_available": true
}
```

## Testing Webhooks

### Manual Testing with curl

```bash
# Test PR opened event
curl -X POST http://localhost:8080/api/v1/webhooks/github \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: pull_request" \
  -H "X-Hub-Signature-256: sha256=$(echo -n '{"action":"opened","number":123}' | openssl dgst -sha256 -hmac 'your-secret' | cut -d' ' -f2)" \
  -d '{
    "action": "opened",
    "number": 123,
    "pull_request": {
      "number": 123,
      "title": "Test PR",
      "user": {"login": "testuser"},
      "head": {"ref": "feature", "sha": "abc123"},
      "base": {"ref": "main", "sha": "def456"}
    },
    "repository": {
      "full_name": "owner/repo"
    }
  }'
```

### Integration Tests

Loom includes integration tests for webhook handling:

```bash
# Run webhook tests
go test -v ./internal/api -run TestGitHubWebhook
```

### GitHub Webhook Redelivery

If webhook delivery fails, you can redeliver from GitHub:

1. Go to **Settings** → **Webhooks**
2. Click on your webhook
3. Scroll to **Recent Deliveries**
4. Click **Redeliver** on any delivery

## Monitoring

### Webhook Logs

Loom logs all webhook events:

```
[INFO] Received GitHub webhook: type=pull_request action=opened repo=owner/repo
[INFO] Created review bead: bead-abc123 for PR #123
```

### Metrics

Prometheus metrics available:

- `webhook_requests_total{type="github",event="pull_request"}`
- `webhook_processing_duration_seconds{type="github"}`
- `webhook_errors_total{type="github",reason="signature_invalid"}`

### Event Bus Monitoring

Check event bus for published webhook events:

```bash
# Query event bus logs
curl http://localhost:8080/api/v1/events?type=external.github_pr
```

## Troubleshooting

### Webhook Not Receiving Events

1. **Check GitHub webhook settings**
   - Verify payload URL is correct
   - Check recent deliveries for errors
   - Ensure webhook is active

2. **Check firewall/networking**
   - Loom endpoint must be publicly accessible
   - GitHub IPs must be allowed through firewall

3. **Check logs**
   ```bash
   # Check Loom logs for webhook errors
   docker logs loom | grep webhook
   ```

### Signature Verification Failing

1. **Verify secret matches**
   - GitHub webhook secret
   - Loom `security.webhook_secret` config

2. **Check signature header**
   - Must be `X-Hub-Signature-256` (not `X-Hub-Signature`)

3. **Verify payload**
   - Don't modify request body before verification
   - Use raw body bytes for HMAC computation

### Review Beads Not Created

1. **Check event action**
   - Only specific actions trigger bead creation
   - See "Actions Handled" for each event type

2. **Check project mapping**
   - Repository must map to valid project
   - See `getOrCreateProjectForRepo()` logic

3. **Check beads manager**
   - Verify beads manager is initialized
   - Check database connectivity

## Advanced Configuration

### Custom Event Handlers

Extend webhook handling for custom workflows:

```go
// In handlers_webhooks.go
func (s *Server) processCustomEvent(eventType string, payload *GitHubWebhookPayload) {
    // Custom event processing logic
}
```

### Event Filtering

Filter events by repository, author, or labels:

```yaml
# config.yaml
webhooks:
  filters:
    repositories:
      - owner/repo1
      - owner/repo2
    ignore_drafts: true
    ignore_authors:
      - dependabot[bot]
```

### Retry Logic

Configure webhook event retry behavior:

```yaml
# config.yaml
webhooks:
  retry:
    enabled: true
    max_attempts: 3
    backoff_seconds: 60
```

## Other Webhook Integrations

- **OpenClaw Messaging Bridge** -- Bidirectional webhook bridge for P0 decision escalations via WhatsApp, Signal, Slack, Telegram, etc. See [OpenClaw Bridge](./OPENCLAW_BRIDGE.md).

## References

- [GitHub Webhooks Documentation](https://docs.github.com/en/webhooks)
- [GitHub Webhook Events](https://docs.github.com/en/webhooks/webhook-events-and-payloads)
- [Securing Webhooks](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries)
- [Loom Code Review Workflow](./CODE_REVIEW_WORKFLOW.md)
- [OpenClaw Bridge](./OPENCLAW_BRIDGE.md)
