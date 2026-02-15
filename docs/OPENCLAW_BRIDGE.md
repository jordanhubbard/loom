# OpenClaw Messaging Bridge

Loom integrates with [OpenClaw](https://openclaw.io) as a bidirectional messaging gateway so that P0 decisions and escalations reach the CEO (or other humans) on whichever platform they prefer -- WhatsApp, Signal, Slack, Telegram, etc. -- without loom needing individual connectors for each service.

## How It Works

```
Outbound (loom -> human)
========================
EventBus event (decision.created P0)
    -> OpenClaw Bridge (internal/openclaw/bridge.go)
    -> POST /hooks/agent on OpenClaw gateway
    -> OpenClaw transforms + routes
    -> CEO's messaging platform

Inbound (human -> loom)
=======================
CEO replies via messaging app
    -> OpenClaw transforms reply
    -> POST /api/v1/webhooks/openclaw on loom
    -> Webhook handler verifies HMAC
    -> processDecisionReply() resolves the decision bead
    -> EventBus: openclaw.reply_processed
```

Reply correlation uses a **session key** (`loom:decision:<id>`) embedded in the outbound message so that the CEO's response is automatically routed back to the correct decision bead.

## Configuration

Add the `openclaw` section to your `config.yaml`:

```yaml
openclaw:
  enabled: true
  gateway_url: "http://127.0.0.1:18789"   # OpenClaw gateway address
  hook_token: "${OPENCLAW_HOOK_TOKEN}"      # Bearer token for outbound POSTs
  webhook_secret: "${OPENCLAW_WEBHOOK_SECRET}"  # HMAC secret for inbound verification
  default_channel: "signal"                 # Platform: signal, whatsapp, slack, telegram, etc.
  default_recipient: "+15551234567"         # CEO phone/handle/channel
  agent_id: "loom"                          # Identifies loom in OpenClaw
  timeout: 30s
  retry_attempts: 3
  retry_delay: 2s
  escalations_only: true                    # Only P0 / CEO-escalated decisions
```

### Configuration Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Master switch. When false, no OpenClaw resources are created. |
| `gateway_url` | string | `http://127.0.0.1:18789` | Base URL of the OpenClaw gateway. |
| `hook_token` | string | (empty) | Bearer token sent in `Authorization` header on outbound requests. |
| `webhook_secret` | string | (empty) | HMAC-SHA256 secret for verifying inbound webhooks. |
| `default_channel` | string | (empty) | Messaging platform to use (e.g. `signal`, `slack`). |
| `default_recipient` | string | (empty) | Default recipient (phone number, Slack channel, etc.). |
| `agent_id` | string | `"loom"` | Agent identifier sent to OpenClaw. |
| `timeout` | duration | `30s` | HTTP client timeout for outbound requests. |
| `retry_attempts` | int | `3` | Max retries for failed outbound sends. |
| `retry_delay` | duration | `2s` | Delay between retries. |
| `escalations_only` | bool | `true` | When true, only P0 and CEO-escalated decisions trigger messages. Set to false to also forward decision resolutions and motivation events. |

### Generating Secrets

```bash
# Hook token (outbound auth)
openssl rand -hex 32

# Webhook secret (inbound HMAC verification)
openssl rand -hex 32
```

Use environment variable expansion (`${VAR}`) in config.yaml to keep secrets out of version control.

## Graceful Degradation

The bridge is designed to be invisible when disabled:

- `openclaw.enabled: false` (default): `NewClient()` and `NewBridge()` return nil. No goroutines are started, no subscriptions are created, the webhook endpoint returns 404.
- `openclaw.enabled: true` with unreachable gateway: Loom starts normally. The bridge logs send failures but decisions still work through the normal pipeline. The status endpoint reports `healthy: false`.
- No webhook secret configured: Inbound webhooks are accepted without HMAC verification (not recommended for production).

## API Endpoints

### POST /api/v1/webhooks/openclaw

Receives inbound messages from the OpenClaw gateway (CEO replies).

**Authentication:** HMAC-SHA256 signature in `X-OpenClaw-Signature` header (when `webhook_secret` is configured). This endpoint bypasses loom's standard JWT/API-key auth since it has its own verification.

**Request body:**

```json
{
  "session_key": "loom:decision:bd-dec-1707000000-1",
  "sender": "ceo@example.com",
  "channel": "signal",
  "text": "approve",
  "timestamp": "2026-02-14T12:00:00Z",
  "message_id": "oc-msg-abc123"
}
```

**Decision reply commands:**

| Reply | Maps to | Notes |
|-------|---------|-------|
| `approve`, `approved`, `yes`, `lgtm` | `approved` | Approves the decision |
| `deny`, `denied`, `no`, `reject`, `rejected` | `denied` | Denies the decision |
| `needs_more_info`, `more info`, `need more info` | `needs_more_info` | Requests more context |
| *(anything else)* | free-form text | Stored as the decision verbatim |

**Success response (decision resolved):**

```json
{
  "status": "resolved",
  "decision_id": "bd-dec-1707000000-1",
  "decision": "approved"
}
```

**Error responses:**

| Status | Reason |
|--------|--------|
| 404 | OpenClaw integration is not enabled |
| 405 | Not a POST request |
| 401 | Invalid HMAC signature |
| 400 | Missing `text` field or invalid JSON |

### GET /api/v1/openclaw/status

Reports integration health and configuration.

**Response (enabled):**

```json
{
  "enabled": true,
  "gateway_url": "http://127.0.0.1:18789",
  "escalations_only": true,
  "default_channel": "signal",
  "healthy": true
}
```

**Response (disabled):**

```json
{
  "enabled": false
}
```

The `healthy` field reflects whether the gateway responds to a HEAD request.

## Outbound Message Format

When a P0 decision is created, the bridge sends a message like:

```
P0 Decision Required

Project: my-project
Question: Should we deploy the auth refactor to production?
Recommendation: Yes, all tests pass
Requested by: agent-backend-dev

Reply with: approve / deny / needs_more_info / <your decision>
```

The session key `loom:decision:<id>` is embedded in the OpenClaw payload so replies are automatically correlated.

## EventBus Events

The bridge publishes observability events for monitoring:

| Event Type | When | Data |
|------------|------|------|
| `openclaw.message_sent` | Outbound message delivered successfully | `source_event_type`, `source_event_id`, `detail` (message_id) |
| `openclaw.message_failed` | Outbound send failed after all retries | `source_event_type`, `source_event_id`, `detail` (error) |
| `openclaw.message_received` | Inbound webhook received | `session_key`, `sender`, `channel`, `message_id` |
| `openclaw.reply_processed` | CEO reply resolved a decision | `decision_id`, `decision`, `decider_id`, `channel` |

Query these via the events API:

```bash
curl http://localhost:8080/api/v1/events?type=openclaw.message_sent
curl http://localhost:8080/api/v1/events?type=openclaw.reply_processed
```

## Security

### Outbound Authentication

Outbound requests to OpenClaw use a **Bearer token** in the `Authorization` header. Configure this in `openclaw.hook_token`.

### Inbound Authentication

Inbound webhooks from OpenClaw are verified using **HMAC-SHA256**, the same scheme used for GitHub webhooks:

1. OpenClaw computes `HMAC-SHA256(request_body, shared_secret)`
2. Sends the signature in the `X-OpenClaw-Signature: sha256=<hex>` header
3. Loom recomputes the HMAC and compares using constant-time comparison

The shared secret is configured in `openclaw.webhook_secret`.

### Separate Secrets

Outbound and inbound use **different secrets** (`hook_token` vs `webhook_secret`). This limits blast radius if one is compromised.

## Testing

### Run Unit Tests

```bash
# Client and bridge tests
go test -v ./internal/openclaw/...

# Webhook handler tests
go test -v ./internal/api/... -run TestHandleOpenClaw
go test -v ./internal/api/... -run TestVerifyOpenClaw
```

### Manual Testing with curl

```bash
# Check status
curl http://localhost:8080/api/v1/openclaw/status

# Simulate an inbound CEO reply (no signature verification)
curl -X POST http://localhost:8080/api/v1/webhooks/openclaw \
  -H "Content-Type: application/json" \
  -d '{
    "session_key": "loom:decision:bd-dec-1707000000-1",
    "sender": "ceo",
    "text": "approve"
  }'

# With HMAC signature
SECRET="your-webhook-secret"
BODY='{"session_key":"loom:decision:bd-dec-1707000000-1","sender":"ceo","text":"approve"}'
SIG=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$SECRET" | cut -d' ' -f2)

curl -X POST http://localhost:8080/api/v1/webhooks/openclaw \
  -H "Content-Type: application/json" \
  -H "X-OpenClaw-Signature: sha256=$SIG" \
  -d "$BODY"
```

### Verify Disabled Mode

Start loom with `openclaw.enabled: false` (default) and confirm:

```bash
# Should return 404
curl -X POST http://localhost:8080/api/v1/webhooks/openclaw \
  -H "Content-Type: application/json" \
  -d '{"text":"test"}'

# Should return {"enabled": false}
curl http://localhost:8080/api/v1/openclaw/status
```

## Troubleshooting

### Messages Not Reaching the CEO

1. **Check integration is enabled:**
   ```bash
   curl http://localhost:8080/api/v1/openclaw/status
   ```
   Verify `enabled: true` and `healthy: true`.

2. **Check gateway reachability:**
   ```bash
   curl -I http://127.0.0.1:18789/
   ```
   OpenClaw gateway must be running and accessible.

3. **Check hook token:**
   Verify `openclaw.hook_token` matches the token configured in OpenClaw's agent hook settings.

4. **Check event bus:**
   ```bash
   curl http://localhost:8080/api/v1/events?type=openclaw.message_failed
   ```
   Look for error details in the `detail` field.

5. **Check escalations_only filter:**
   If `escalations_only: true` (default), only P0 decisions trigger messages. Non-P0 decisions are silently skipped.

### CEO Replies Not Resolving Decisions

1. **Check session key format:**
   The reply must include `session_key: "loom:decision:<id>"` where `<id>` matches an existing pending decision.

2. **Check webhook signature:**
   If `webhook_secret` is configured, verify OpenClaw is signing requests correctly. Check for 401 responses in loom logs.

3. **Check decision exists:**
   ```bash
   curl http://localhost:8080/api/v1/decisions/<id>
   ```
   The decision must be in `open` or `in_progress` status to be resolved.

4. **Check event bus for processed replies:**
   ```bash
   curl http://localhost:8080/api/v1/events?type=openclaw.reply_processed
   ```

### Gateway Unhealthy but Loom Running Fine

This is expected behavior. The bridge logs failures but does not block the decision pipeline. Decisions still work through the normal API/web UI -- OpenClaw is an optional notification channel.

## Architecture

### Source Files

| File | Purpose |
|------|---------|
| `pkg/config/config.go` | `OpenClawConfig` struct and defaults |
| `internal/openclaw/types.go` | Shared types (`AgentRequest`, `InboundMessage`, etc.) |
| `internal/openclaw/client.go` | HTTP client for outbound POSTs to OpenClaw |
| `internal/openclaw/bridge.go` | EventBus subscriber that forwards events to the client |
| `internal/api/handlers_openclaw.go` | Inbound webhook handler and status endpoint |
| `internal/loom/loom.go` | Wires client + bridge into the Loom lifecycle |
| `internal/api/server.go` | Route registration |
| `internal/temporal/eventbus/eventbus.go` | Event type constants |

### Lifecycle

1. **Startup:** `loom.New()` creates `openclaw.Client` and `openclaw.Bridge` (both nil when disabled). The bridge subscribes to the EventBus and starts a goroutine.
2. **Runtime:** The bridge goroutine listens for `decision.created`, `decision.resolved`, and `motivation.fired` events. Matching events are formatted and sent via the client.
3. **Shutdown:** `loom.Shutdown()` calls `bridge.Close()`, which cancels the context, unsubscribes, and waits for the goroutine to exit.

## Related Documentation

- [Escalation Guide](ESCALATION_GUIDE.md) -- How decisions get escalated to P0
- [GitHub Webhooks](WEBHOOKS.md) -- GitHub webhook integration (similar patterns)
- [Consensus Decisions](CONSENSUS_DECISIONS.md) -- Multi-agent decision voting
- [Motivation System](MOTIVATION_SYSTEM.md) -- Events that can trigger bridge messages
