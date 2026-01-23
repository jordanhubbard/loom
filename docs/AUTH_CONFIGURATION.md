# Authentication Configuration

## Overview

AgentiCorp supports optional authentication via JWT tokens. Authentication can be enabled or disabled via the `security.enable_auth` configuration flag in `config.yaml`.

## Configuration

### Disabling Authentication (Development Mode)

To disable authentication, set the following in `config.yaml`:

```yaml
security:
  enable_auth: false  # DISABLED FOR DEVELOPMENT - Enable in production
  pki_enabled: false
  ca_file: ""
  require_https: false
  allowed_origins:
    - "*"  # CORS - adjust in production
```

**When authentication is disabled:**
- All API endpoints are accessible without tokens
- No login required in the web UI
- All requests bypass JWT validation
- Useful for local development and testing

### Enabling Authentication (Production Mode)

To enable authentication, set the following in `config.yaml`:

```yaml
security:
  enable_auth: true   # ENABLED for production
  pki_enabled: false  # Set to true when certificates are available
  ca_file: ""
  require_https: true  # Recommended for production
  allowed_origins:
    - "https://your-domain.com"  # Restrict CORS in production
  jwt_secret: "your-secure-jwt-secret"  # Required for JWT signing
```

**When authentication is enabled:**
- API endpoints require valid JWT tokens (except health checks and login)
- Web UI shows login screen
- Default credentials: `admin` / `admin` (change in production!)
- Tokens must be passed in `Authorization: Bearer <token>` header

## Implementation Details

### Configuration Loading

The application loads configuration in the following order:

1. **File-based config**: Loads from `config.yaml` (path specified via `CONFIG_PATH` env var)
2. **Environment overrides**: Environment variables can override config values
3. **Defaults**: Falls back to hardcoded defaults if config file not found

### Docker Configuration

In `docker-compose.yml`, the config path is set via environment variable:

```yaml
environment:
  - CONFIG_PATH=/app/src/config.yaml  # Points to volume-mounted config
```

This ensures the live config file (from the host) is used instead of the build-time copy.

### Authentication Middleware

The authentication middleware (`internal/api/server.go:authMiddleware`) checks the `EnableAuth` flag:

```go
// Skip auth if disabled
if !s.config.Security.EnableAuth || s.authManager == nil {
    next.ServeHTTP(w, r)
    return
}
```

### Endpoints That Always Bypass Auth

The following endpoints are always accessible without authentication, regardless of the `enable_auth` setting:

- `/health`, `/health/live`, `/health/ready` - Health checks for Kubernetes
- `/api/v1/health` - Legacy health endpoint
- `/api/v1/auth/login` - Login endpoint
- `/api/v1/auth/refresh` - Token refresh endpoint
- `/` - Root/index page
- `/static/*` - Static files (JS, CSS, images)
- `/api/openapi.yaml` - OpenAPI specification

## Applying Configuration Changes

### Method 1: Restart Container (Recommended)

```bash
# Edit config.yaml
vi config.yaml

# Restart the container to pick up changes
docker compose restart agenticorp
```

### Method 2: Rebuild (If main.go changed)

```bash
# Rebuild container
docker compose build agenticorp

# Recreate container with new image
docker compose up -d agenticorp
```

## Security Best Practices

### Development

- ✅ **Disable authentication** for local testing
- ✅ Use HTTP (not HTTPS)
- ✅ Allow all CORS origins (`*`)
- ⚠️ Never expose development instances publicly

### Production

- ✅ **Enable authentication**
- ✅ Use HTTPS (`require_https: true`)
- ✅ Restrict CORS to specific domains
- ✅ Change default admin password immediately
- ✅ Use strong JWT secret (32+ characters, random)
- ✅ Enable PKI when certificates are available
- ✅ Set up API key rotation
- ✅ Monitor authentication failures

## Troubleshooting

### Authentication Not Disabled

**Symptom**: API still returns `401 Unauthorized` after setting `enable_auth: false`

**Solutions**:
1. Verify config file location: `docker exec agenticorp cat /app/src/config.yaml | grep enable_auth`
2. Check `CONFIG_PATH` env var: `docker exec agenticorp printenv CONFIG_PATH`
3. Restart container: `docker compose restart agenticorp`
4. Check logs: `docker logs agenticorp | grep "Loaded configuration"`

### UI Still Shows Login Screen

**Symptom**: Web UI prompts for login even with auth disabled

**Cause**: The JavaScript may have cached state or made requests before config was updated.

**Solutions**:
1. Hard refresh browser (`Cmd+Shift+R` or `Ctrl+Shift+R`)
2. Clear browser cache
3. Verify API works without auth: `curl http://localhost:8080/api/v1/projects`
4. Check browser console for JavaScript errors

### Config Changes Not Applied

**Symptom**: Changes to `config.yaml` don't take effect

**Cause**: Container may be using build-time config instead of volume-mounted config.

**Solutions**:
1. Verify `CONFIG_PATH=/app/src/config.yaml` in `docker-compose.yml`
2. Verify volume mount: `- .:/app/src:rw` in `docker-compose.yml`
3. Rebuild container if `main.go` changed: `docker compose build agenticorp`

## Related Files

- `config.yaml` - Main configuration file
- `pkg/config/config.go` - Configuration structures and loader
- `internal/api/server.go` - Authentication middleware
- `main.go` - Configuration loading on startup
- `docker-compose.yml` - Environment variables and volume mounts

## See Also

- [Security Guide](SECURITY.md) - Comprehensive security documentation
- [User Guide](USER_GUIDE.md) - Authentication from user perspective
- [API Documentation](../api/openapi.yaml) - API endpoint specifications
