# Load Balancing Guide

This guide explains how to configure load balancers for AgentiCorp distributed deployments.

## Overview

AgentiCorp supports various load balancing configurations for:
- **High availability** - Redundancy and failover
- **Performance** - Distribute load across instances
- **Scalability** - Handle increasing traffic
- **Geographic distribution** - Reduce latency

## Load Balancing Strategies

### 1. Round Robin

Distributes requests evenly across all instances.

**Pros:**
- Simple to configure
- Even distribution
- No state tracking needed

**Cons:**
- Doesn't consider instance load
- May send requests to busy instances

**Best for:** Stateless requests, similar instance capacity

### 2. Least Connections

Routes to the instance with fewest active connections.

**Pros:**
- Balances load dynamically
- Adapts to varying request durations
- Better for long-running requests

**Cons:**
- Requires connection tracking
- Slightly more complex

**Best for:** Mixed workloads, streaming requests

### 3. IP Hash / Session Affinity

Routes requests from same IP to same instance.

**Pros:**
- Session persistence
- Cache locality
- Simplified debugging

**Cons:**
- Uneven distribution possible
- Failover requires session migration

**Best for:** Stateful applications, caching optimization

## Nginx Configuration

### Basic Load Balancing

```nginx
upstream agenticorp {
    least_conn;  # Use least connections algorithm
    
    server backend1:8080 max_fails=3 fail_timeout=30s weight=1;
    server backend2:8080 max_fails=3 fail_timeout=30s weight=1;
    server backend3:8080 max_fails=3 fail_timeout=30s weight=1;
}

server {
    listen 80;
    server_name agenticorp.example.com;
    
    location / {
        proxy_pass http://agenticorp;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
        
        # Retry on errors
        proxy_next_upstream error timeout invalid_header http_500 http_502 http_503;
        proxy_next_upstream_tries 2;
    }
    
    # Health check endpoint
    location /health {
        proxy_pass http://agenticorp;
        access_log off;
    }
}
```

### Session Affinity (Sticky Sessions)

```nginx
upstream agenticorp {
    least_conn;
    
    server backend1:8080;
    server backend2:8080;
    server backend3:8080;
    
    # IP-based sticky sessions
    ip_hash;
}

# Or use cookies for better control:
upstream agenticorp {
    least_conn;
    
    server backend1:8080;
    server backend2:8080;
    server backend3:8080;
}

server {
    listen 80;
    
    location / {
        proxy_pass http://agenticorp;
        
        # Cookie-based sticky sessions
        proxy_set_header Cookie $http_cookie;
        proxy_cookie_path / "/; Secure; HttpOnly";
        
        # Add instance identifier header
        add_header X-Instance-ID $upstream_addr;
    }
}
```

### Health Checks

```nginx
upstream agenticorp {
    server backend1:8080;
    server backend2:8080;
    server backend3:8080;
    
    # Nginx Plus health checks
    zone agenticorp 64k;
    check interval=5s
          fail_timeout=10s
          rise=2
          fall=3
          timeout=2s
          type=http;
    check_http_send "GET /health/ready HTTP/1.0\r\n\r\n";
    check_http_expect_alive http_2xx http_3xx;
}
```

## HAProxy Configuration

### Basic Setup

```haproxy
global
    log /dev/log local0
    maxconn 4096
    
defaults
    log     global
    mode    http
    option  httplog
    option  dontlognull
    timeout connect 5s
    timeout client  50s
    timeout server  50s
    
frontend agenticorp_front
    bind *:80
    default_backend agenticorp_back
    
backend agenticorp_back
    balance leastconn
    option httpchk GET /health/ready
    http-check expect status 200
    
    server instance1 backend1:8080 check inter 5s rise 2 fall 3
    server instance2 backend2:8080 check inter 5s rise 2 fall 3
    server instance3 backend3:8080 check inter 5s rise 2 fall 3
```

### Session Affinity

```haproxy
backend agenticorp_back
    balance leastconn
    
    # Cookie-based sticky sessions
    cookie AGENTICORP_INSTANCE insert indirect nocache
    
    server instance1 backend1:8080 cookie inst1 check
    server instance2 backend2:8080 cookie inst2 check
    server instance3 backend3:8080 cookie inst3 check
```

### Advanced Health Checks

```haproxy
backend agenticorp_back
    option httpchk GET /health/ready
    http-check expect status 200
    http-check expect string \"ready\":true
    
    # Detailed health checks
    http-check send meth GET uri /health/ready ver HTTP/1.1 hdr Host agenticorp.local
    http-check expect rstatus ^2[0-9][0-9]
    
    server instance1 backend1:8080 check inter 5s fastinter 2s downinter 10s
```

## AWS Application Load Balancer

### Target Group Configuration

```yaml
TargetGroup:
  HealthCheckEnabled: true
  HealthCheckProtocol: HTTP
  HealthCheckPath: /health/ready
  HealthCheckIntervalSeconds: 30
  HealthCheckTimeoutSeconds: 5
  HealthyThresholdCount: 2
  UnhealthyThresholdCount: 3
  Matcher:
    HttpCode: 200
    
  # Sticky sessions
  TargetGroupAttributes:
    - Key: stickiness.enabled
      Value: true
    - Key: stickiness.type
      Value: lb_cookie
    - Key: stickiness.lb_cookie.duration_seconds
      Value: 3600
```

## GCP Load Balancer

```yaml
health_check:
  type: HTTP
  request_path: /health/ready
  port: 8080
  check_interval_sec: 10
  timeout_sec: 5
  healthy_threshold: 2
  unhealthy_threshold: 3

backend_service:
  protocol: HTTP
  port_name: http
  timeout_sec: 60
  
  # Session affinity
  session_affinity: CLIENT_IP
  affinity_cookie_ttl_sec: 3600
  
  # Load balancing algorithm
  load_balancing_scheme: EXTERNAL
  locality_lb_policy: ROUND_ROBIN
```

## Session Affinity

### Why Session Affinity?

Benefits:
- **Cache locality**: Same user hits same instance cache
- **Stateful operations**: WebSocket connections stay connected
- **Debugging**: Easier to trace user requests
- **Consistency**: User sees same instance state

Trade-offs:
- **Uneven load**: Popular users may overload one instance
- **Failover complexity**: Need to handle instance failures
- **Reduced elasticity**: Can't easily remove instances

### Implementation Options

#### 1. IP-Based Affinity

Routes based on client IP address.

**Nginx:**
```nginx
upstream agenticorp {
    ip_hash;
    server backend1:8080;
    server backend2:8080;
    server backend3:8080;
}
```

**HAProxy:**
```haproxy
backend agenticorp_back
    balance source  # IP-based
    hash-type consistent
```

**Pros:** Simple, no cookies needed  
**Cons:** Problems with proxies/NAT, mobile users

#### 2. Cookie-Based Affinity

Uses cookies to track instance assignment.

**Nginx:**
```nginx
# Requires nginx-sticky-module-ng
upstream agenticorp {
    sticky cookie agenticorp_route expires=1h;
    server backend1:8080;
    server backend2:8080;
    server backend3:8080;
}
```

**HAProxy:**
```haproxy
backend agenticorp_back
    cookie SERVERID insert indirect nocache
    server inst1 backend1:8080 cookie inst1
    server inst2 backend2:8080 cookie inst2
    server inst3 backend3:8080 cookie inst3
}
```

**Pros:** Accurate, survives IP changes  
**Cons:** Requires cookie support

#### 3. Header-Based Affinity

Uses custom headers for routing.

```nginx
map $http_x_instance_id $backend_server {
    "instance1" backend1:8080;
    "instance2" backend2:8080;
    "instance3" backend3:8080;
    default     "";
}

upstream agenticorp {
    server backend1:8080;
    server backend2:8080;
    server backend3:8080;
}

server {
    location / {
        set $target $backend_server;
        
        if ($target = "") {
            set $target "agenticorp";
        }
        
        proxy_pass http://$target;
    }
}
```

## Connection Draining

Ensure graceful shutdown by draining connections:

### Nginx

```nginx
upstream agenticorp {
    server backend1:8080;
    server backend2:8080 down;  # Mark as down for draining
    server backend3:8080;
}
```

During shutdown:
1. Mark instance as "down" in load balancer
2. Wait for existing connections to complete
3. Shutdown instance
4. Remove from load balancer

### HAProxy

```haproxy
# Graceful drain via admin socket
echo "set server agenticorp_back/instance2 state drain" | socat stdio /var/run/haproxy.sock
```

## Performance Tuning

### Connection Pooling

```nginx
upstream agenticorp {
    server backend1:8080;
    
    keepalive 32;  # Connection pool size
    keepalive_requests 100;
    keepalive_timeout 60s;
}

location / {
    proxy_pass http://agenticorp;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
}
```

### Request Buffering

```nginx
# Disable buffering for streaming
location /api/v1/stream {
    proxy_pass http://agenticorp;
    proxy_buffering off;
    proxy_cache off;
}

# Enable buffering for normal requests
location / {
    proxy_pass http://agenticorp;
    proxy_buffering on;
    proxy_buffer_size 4k;
    proxy_buffers 8 4k;
}
```

## Monitoring

### Nginx Status

```nginx
server {
    listen 8081;
    location /nginx_status {
        stub_status;
        allow 127.0.0.1;
        deny all;
    }
}
```

Access: `curl http://localhost:8081/nginx_status`

### HAProxy Stats

```haproxy
listen stats
    bind *:8404
    stats enable
    stats uri /stats
    stats refresh 5s
    stats auth admin:password
```

Access: `http://localhost:8404/stats`

## Troubleshooting

### Uneven Load Distribution

**Problem:** One instance gets more traffic

**Solutions:**
1. Check weights are equal
2. Verify instances have similar capacity
3. Consider least_conn algorithm
4. Review session affinity settings
5. Check for sticky sessions on specific IPs

### Instance Not Receiving Traffic

**Problem:** Instance registered but no requests

**Solutions:**
1. Check health endpoint: `curl http://backend:8080/health/ready`
2. Verify load balancer config
3. Review fail_timeout settings
4. Check instance is marked "up"
5. Review load balancer logs

### Slow Health Checks

**Problem:** Health checks taking too long

**Solutions:**
1. Optimize /health/ready endpoint
2. Increase health check timeout
3. Reduce dependency checks
4. Add caching to health checks
5. Use /health/live for simpler checks

## Best Practices

1. **Use least_conn** - Better than round_robin for varying workloads
2. **Short health check intervals** - 5-10 seconds for fast failover
3. **Multiple health check failures** - 3 failures before marking down
4. **Connection draining** - Allow 30-60s for graceful shutdown
5. **Monitoring** - Alert on instance failures
6. **Session affinity** - Only when needed (adds complexity)
7. **Health check logs** - Disable to reduce noise
8. **Timeouts** - Set appropriate connect/read/write timeouts
9. **Retry logic** - Retry on specific errors only
10. **SSL termination** - Terminate at load balancer for performance

## Examples

See `examples/load-balancing/` for complete configurations:
- `nginx.conf` - Nginx configuration
- `haproxy.cfg` - HAProxy configuration
- `docker-compose.yml` - Multi-instance setup
- `kubernetes.yaml` - K8s deployment with load balancer

## Resources

- [Distributed Deployment Guide](DISTRIBUTED_DEPLOYMENT.md)
- [Health Check Reference](HEALTH_CHECKS.md)
- [Nginx Documentation](https://nginx.org/en/docs/)
- [HAProxy Documentation](https://www.haproxy.org/)

---

**AgentiCorp is ready for enterprise-scale load balancing!** 🚀
