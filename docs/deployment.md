# Deployment Guide

This guide covers deploying Outpost VPN in production environments using Docker Compose, Kubernetes (Helm), or bare-metal.

## Docker Compose (Single Server)

Suitable for: up to ~500 peers, single location.

### 1. Prepare the Environment

```bash
git clone https://github.com/romashqua/outpost.git
cd outpost
```

Create an `.env` file for Docker Compose:

```bash
cat > deploy/docker/.env << 'EOF'
POSTGRES_PASSWORD=<generate-a-strong-password>
REDIS_PASSWORD=<generate-a-strong-password>
JWT_SECRET=<generate-a-64-character-random-string>
GATEWAY_TOKEN=<generate-a-64-character-random-string>
EOF
```

Generating secure values:

```bash
openssl rand -hex 32   # for POSTGRES_PASSWORD
openssl rand -hex 32   # for REDIS_PASSWORD
openssl rand -hex 32   # for JWT_SECRET
openssl rand -hex 32   # for GATEWAY_TOKEN
```

### 2. Start the Stack

```bash
docker compose -f deploy/docker/docker-compose.yml up -d
```

Services and ports:

| Service | Internal Port | Exposed Port | Network |
|---------|---------------|--------------|---------|
| PostgreSQL | 5432 | 5432 | internal |
| Redis | 6379 | 6379 | internal |
| outpost-core | 8080, 50051 | 8080, 50051 | internal + dmz |
| outpost-gateway | 51820/udp | 51820/udp | internal |
| outpost-proxy | 8080 | 8081 | dmz |

### 3. Verify

```bash
# Check that all services are running
docker compose -f deploy/docker/docker-compose.yml ps

# Check the health endpoint
curl http://localhost:8080/healthz
```

### Docker Compose Network Topology

The default `docker-compose.yml` defines two networks:

- **internal** — core, gateway, PostgreSQL, Redis (private, no internet access)
- **dmz** — core, proxy (proxy is accessible from the internet for enrollment)

```yaml
networks:
  internal:
    driver: bridge
  dmz:
    driver: bridge
```

### Docker Compose Reference

The full compose file is at `deploy/docker/docker-compose.yml`. Key configuration:

- **core** uses environment variables prefixed with `OUTPOST_` (see [Configuration Reference](configuration.md))
- **gateway** requires `NET_ADMIN` and `SYS_MODULE` capabilities, plus the `/dev/net/tun` device
- **gateway** enables sysctl `net.ipv4.ip_forward=1` and `net.ipv4.conf.all.src_valid_mark=1`
- **proxy** is stateless — no database or Redis connections

---

## Kubernetes (Helm)

The Helm chart is located at `deploy/helm/outpost/`.

### 1. Installation

```bash
# From the local chart
helm install outpost deploy/helm/outpost \
  --namespace outpost \
  --create-namespace \
  --set secret.jwtSecret=$(openssl rand -hex 32) \
  --set secret.databasePassword=$(openssl rand -hex 32)
```

Or from the Helm repository (when published):

```bash
helm repo add outpost https://charts.outpost-vpn.io
helm repo update

helm install outpost outpost/outpost \
  --namespace outpost \
  --create-namespace \
  --set secret.jwtSecret=$(openssl rand -hex 32) \
  --set secret.databasePassword=$(openssl rand -hex 32)
```

### 2. Production Values

Create `values-production.yaml`:

```yaml
core:
  replicaCount: 3
  resources:
    requests:
      cpu: 500m
      memory: 256Mi
    limits:
      cpu: 2000m
      memory: 1Gi
  env:
    OUTPOST_LOG_LEVEL: info
    OUTPOST_LOG_FORMAT: json

gateway:
  enabled: true
  # Deploy as DaemonSet on labeled nodes for multi-gateway
  nodeSelector:
    outpost.io/gateway: "true"
  env:
    OUTPOST_GATEWAY_TOKEN: "<token>"
  # Gateway requires elevated privileges
  # securityContext is set in the chart template

proxy:
  replicaCount: 2
  service:
    type: LoadBalancer

# Use an external managed DB in production
postgresql:
  enabled: false

externalDatabase:
  host: outpost-db.rds.amazonaws.com
  port: 5432
  database: outpost
  username: outpost

# Use an external managed Redis in production
redis:
  enabled: false

externalRedis:
  host: outpost-redis.elasticache.amazonaws.com
  port: 6379

secret:
  jwtSecret: ""          # set via --set or external secret manager
  databasePassword: ""   # set via --set or external secret manager
  redisPassword: ""

ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: vpn.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: outpost-tls
      hosts:
        - vpn.example.com
```

Install with production values:

```bash
helm install outpost deploy/helm/outpost \
  --namespace outpost \
  --create-namespace \
  -f values-production.yaml \
  --set secret.jwtSecret=$(openssl rand -hex 32) \
  --set secret.databasePassword=$DB_PASSWORD \
  --set secret.redisPassword=$REDIS_PASSWORD
```

### Helm Chart Values Reference

| Key | Default | Description |
|-----|---------|-------------|
| `core.replicaCount` | `1` | Number of core replicas |
| `core.image.repository` | `ghcr.io/romashqua/outpost-core` | Core image |
| `core.service.httpPort` | `8080` | HTTP API port |
| `core.service.grpcPort` | `9090` | gRPC port |
| `gateway.enabled` | `true` | Deploy gateway |
| `gateway.service.type` | `LoadBalancer` | Gateway service type |
| `gateway.service.port` | `51820` | WireGuard UDP port |
| `proxy.enabled` | `true` | Deploy proxy |
| `proxy.replicaCount` | `1` | Number of proxy replicas |
| `postgresql.enabled` | `true` | Deploy built-in PostgreSQL |
| `postgresql.storage.size` | `5Gi` | PostgreSQL PVC size |
| `redis.enabled` | `true` | Deploy built-in Redis |
| `redis.storage.size` | `1Gi` | Redis PVC size |
| `config.logLevel` | `info` | Log level |
| `config.coreUrl` | `http://outpost-core:8080` | Core URL for inter-service communication |
| `config.coreGrpcUrl` | `outpost-core:9090` | Core gRPC URL |
| `ingress.enabled` | `false` | Enable Ingress |
| `secret.jwtSecret` | `""` | JWT signing key |
| `secret.databasePassword` | `outpost` | Database password |
| `secret.redisPassword` | `""` | Redis password |

---

## Multi-Node / High-Availability Setup

outpost-core is stateless (all state is in PostgreSQL + Redis), so it scales horizontally behind a load balancer.

### Architecture

```
                  ┌──────────────┐
                  │   HAProxy /  │
                  │  Nginx LB    │
                  │ :443  :50051 │
                  └──────┬───────┘
                         │
            ┌────────────┼────────────┐
            │            │            │
     ┌──────▼──┐  ┌──────▼──┐  ┌─────▼───┐
     │ core-1  │  │ core-2  │  │ core-3  │
     └────┬────┘  └────┬────┘  └────┬────┘
          │            │            │
     ┌────▼────────────▼────────────▼────┐
     │       PostgreSQL (primary)         │
     │       + Redis (sentinel/cluster)   │
     └───────────────────────────────────┘
```

### Requirements

1. **PostgreSQL HA**: use Patroni, CloudNativePG, or a managed DB (RDS, Cloud SQL)
2. **Redis HA**: use Redis Sentinel or Redis Cluster
3. **Load balancer**: route HTTP (:8080) and gRPC (:50051) to all core instances
4. **Shared JWT_SECRET**: all core instances must use the same `OUTPOST_JWT_SECRET`

---

## TLS / Reverse Proxy

### Nginx

```nginx
upstream outpost_http {
    least_conn;
    server core-1:8080;
    server core-2:8080;
}

upstream outpost_grpc {
    least_conn;
    server core-1:50051;
    server core-2:50051;
}

# HTTP API + admin panel
server {
    listen 443 ssl http2;
    server_name vpn.example.com;

    ssl_certificate     /etc/ssl/certs/vpn.example.com.pem;
    ssl_certificate_key /etc/ssl/private/vpn.example.com.key;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;

    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options nosniff;
    add_header X-Frame-Options DENY;

    location / {
        proxy_pass http://outpost_http;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

# gRPC (for gateway connections)
server {
    listen 50051 http2;

    location / {
        grpc_pass grpc://outpost_grpc;
    }
}

# HTTP to HTTPS redirect
server {
    listen 80;
    server_name vpn.example.com;
    return 301 https://$host$request_uri;
}
```

### Caddy

```Caddyfile
vpn.example.com {
    reverse_proxy core-1:8080 core-2:8080 {
        lb_policy least_conn
        health_uri /healthz
        health_interval 10s
    }
}
```

Caddy automatically obtains and renews TLS certificates via Let's Encrypt.

---

## Multi-Site Gateway Deployment

Deploy gateways at each site, all connecting to the core cluster.

```bash
# On each gateway machine
docker run -d \
  --name outpost-gateway \
  --cap-add=NET_ADMIN \
  --cap-add=SYS_MODULE \
  --device=/dev/net/tun \
  --sysctl net.ipv4.ip_forward=1 \
  --sysctl net.ipv4.conf.all.src_valid_mark=1 \
  -e OUTPOST_GATEWAY_CORE_ADDR=core-lb.example.com:50051 \
  -e OUTPOST_GATEWAY_TOKEN=$GATEWAY_TOKEN \
  -e OUTPOST_LOG_LEVEL=info \
  -e OUTPOST_LOG_FORMAT=json \
  -p 51820:51820/udp \
  ghcr.io/romashqua/outpost-gateway:latest
```

Register each gateway via the admin panel:
1. Go to **Gateways** > **Create**
2. Specify the name, network, and public endpoint
3. Copy the generated token to the gateway's `OUTPOST_GATEWAY_TOKEN`
4. Restart the gateway container

---

## PostgreSQL Tuning

For production PostgreSQL with Outpost:

```sql
-- Recommended settings for a server with 4 GB RAM and ~500 peers
ALTER SYSTEM SET shared_buffers = '1GB';
ALTER SYSTEM SET effective_cache_size = '3GB';
ALTER SYSTEM SET work_mem = '16MB';
ALTER SYSTEM SET maintenance_work_mem = '256MB';
ALTER SYSTEM SET max_connections = 100;
ALTER SYSTEM SET checkpoint_completion_target = 0.9;
ALTER SYSTEM SET wal_buffers = '64MB';
ALTER SYSTEM SET random_page_cost = 1.1;  -- for SSD
```

The `peer_stats` table is partitioned by `recorded_at` for efficient time-series queries. Create monthly partitions:

```sql
CREATE TABLE peer_stats_2026_03 PARTITION OF peer_stats
    FOR VALUES FROM ('2026-03-01') TO ('2026-04-01');
```

---

## Redis Configuration

Recommended Redis settings:

```
maxmemory 256mb
maxmemory-policy allkeys-lru
save ""
appendonly no
```

Redis is used for:
- Session storage
- Pub/sub event broadcasting
- Rate limiting state storage

Sessions are recreated on Redis restart, so persistence is optional. Disable RDB/AOF persistence for better performance if session loss on restart is acceptable.

---

## Backup and Recovery

### PostgreSQL Backup

```bash
# Full backup
pg_dump -h localhost -U outpost -d outpost -Fc > outpost_$(date +%Y%m%d).dump

# Restore
pg_restore -h localhost -U outpost -d outpost -c outpost_20260315.dump
```

### What to Back Up

| Data | Location | Criticality |
|------|----------|-------------|
| PostgreSQL database | All application state | Critical |
| JWT secret | `OUTPOST_JWT_SECRET` environment variable | Critical (tokens are invalid without it) |
| Gateway tokens | `OUTPOST_GATEWAY_TOKEN` on each gateway | High (gateways cannot reconnect) |
| TLS certificates | Reverse proxy configuration | High |
| Redis | Sessions, rate limits | Low (recreated on restart) |

---

## Monitoring

outpost-core exposes Prometheus metrics at `/metrics` (no authentication).

### Prometheus Configuration

```yaml
scrape_configs:
  - job_name: 'outpost'
    static_configs:
      - targets: ['core-1:8080', 'core-2:8080']
    metrics_path: /metrics
    scrape_interval: 15s
```

### Key Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `outpost_active_peers` | Gauge | Currently connected WireGuard peers |
| `outpost_bandwidth_bytes_total` | Counter | Total traffic (rx/tx labels) |
| `outpost_gateway_last_seen_seconds` | Gauge | Time since last gateway heartbeat |
| `outpost_auth_attempts_total` | Counter | Authentication attempts (success/failure labels) |
| `outpost_s2s_tunnel_status` | Gauge | S2S tunnel status (1=up, 0=down) |

### Health Check Endpoints

| Endpoint | Purpose | Usage |
|----------|---------|-------|
| `GET /healthz` | Liveness check | Kubernetes liveness probe, LB health check |
| `GET /readyz` | Readiness check (verifies DB) | Kubernetes readiness probe |

---

## Troubleshooting

| Symptom | What to Check |
|---------|---------------|
| Cannot connect to VPN | `wg show` on the gateway, firewall rules, device approval status |
| Gateway shows as offline | Core logs, gateway token, gRPC connectivity, `OUTPOST_GATEWAY_CORE_ADDR` |
| Slow handshakes | MTU settings, DNS resolution, endpoint reachability |
| S2S tunnel not working | Both gateways online, route exchange, subnet conflicts |
| MFA not working | Time synchronization (NTP), TOTP secret validity |
| 429 Too Many Requests | Auth rate limit (10/min per IP), check if a proxy is missing `X-Real-IP` |
| Core won't start | Check `OUTPOST_JWT_SECRET`, DB connectivity, migration errors |
| Blank frontend page | Check if `web-ui/dist` is embedded, `go:embed` directive |
