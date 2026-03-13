# Outpost VPN — Deployment Guide

## Quick Start (Development)

```bash
cd deploy/docker
docker compose up -d
```

This starts: PostgreSQL 17, Redis 7, outpost-core, outpost-gateway, outpost-proxy.

- Admin UI: http://localhost:8080
- Default credentials: `admin` / `outpost` (change immediately)
- API: http://localhost:8080/api/v1/
- gRPC: localhost:50051
- WireGuard: localhost:51820/udp
- Enrollment proxy: http://localhost:8081

## Architecture Overview

```
┌────────────────────────────────────────────────────────────┐
│                     DMZ / Internet                          │
│   ┌──────────────┐         ┌──────────────┐                │
│   │ outpost-proxy│         │  Clients     │                │
│   │    :8081     │         │  (WireGuard) │                │
│   └──────┬───────┘         └──────┬───────┘                │
└──────────┼────────────────────────┼────────────────────────┘
           │ gRPC                   │ UDP :51820
┌──────────┼────────────────────────┼────────────────────────┐
│          │      Internal Network  │                         │
│   ┌──────▼───────┐         ┌──────▼───────┐                │
│   │ outpost-core │◄───────►│outpost-gateway│               │
│   │  :8080 :50051│  gRPC   │   :51820/udp │               │
│   └──────┬───────┘  stream └──────────────┘                │
│          │                                                  │
│   ┌──────▼──────┐   ┌──────────┐                           │
│   │ PostgreSQL  │   │  Redis   │                           │
│   │    :5432    │   │  :6379   │                           │
│   └─────────────┘   └──────────┘                           │
└────────────────────────────────────────────────────────────┘
```

## Production Deployment

### Single-Node (Docker Compose)

Best for: up to ~500 peers, single location.

1. Clone the repo and configure `.env`:

```bash
cp deploy/docker/.env.example deploy/docker/.env
# Edit .env:
#   POSTGRES_PASSWORD=<strong-random>
#   REDIS_PASSWORD=<strong-random>
#   JWT_SECRET=<random-64-chars>
#   GATEWAY_TOKEN=<random-64-chars>
```

2. Start the stack:

```bash
cd deploy/docker
docker compose up -d
```

3. Configure SMTP for email notifications (optional):

```bash
export OUTPOST_SMTP_HOST=smtp.example.com
export OUTPOST_SMTP_PORT=587
export OUTPOST_SMTP_USERNAME=outpost@example.com
export OUTPOST_SMTP_PASSWORD=...
export OUTPOST_SMTP_FROM=noreply@example.com
export OUTPOST_SMTP_TLS=true
```

### Multi-Node Cluster

Best for: high availability, multiple locations, >500 peers.

#### Core Cluster (2-3 nodes)

outpost-core is stateless (all state in PostgreSQL + Redis), so it scales horizontally behind a load balancer.

```
                  ┌──────────────┐
                  │   HAProxy /  │
                  │  Nginx LB    │
                  │  :8080 :50051│
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
     │       + Redis (cluster/sentinel)   │
     └───────────────────────────────────┘
```

**Steps:**

1. Set up PostgreSQL HA (Patroni, CloudNativeDB, or managed DB):

```bash
# Using managed PostgreSQL (recommended):
OUTPOST_DB_HOST=outpost-db.example.com
OUTPOST_DB_PORT=5432
OUTPOST_DB_NAME=outpost
OUTPOST_DB_USER=outpost
OUTPOST_DB_PASSWORD=<strong>
OUTPOST_DB_SSLMODE=require
```

2. Set up Redis Sentinel or Redis Cluster:

```bash
OUTPOST_REDIS_ADDR=redis-sentinel.example.com:26379
OUTPOST_REDIS_PASSWORD=<strong>
```

3. Deploy multiple core instances:

```bash
# On each core node:
docker run -d \
  --name outpost-core \
  -e OUTPOST_HTTP_ADDR=:8080 \
  -e OUTPOST_GRPC_ADDR=:50051 \
  -e OUTPOST_DB_HOST=outpost-db.example.com \
  -e OUTPOST_DB_PASSWORD=$DB_PASSWORD \
  -e OUTPOST_REDIS_ADDR=redis.example.com:6379 \
  -e OUTPOST_JWT_SECRET=$JWT_SECRET \
  -p 8080:8080 \
  -p 50051:50051 \
  outpost/core:latest
```

4. Configure load balancer:

```nginx
# nginx.conf example
upstream outpost_http {
    least_conn;
    server core-1:8080;
    server core-2:8080;
    server core-3:8080;
}

upstream outpost_grpc {
    least_conn;
    server core-1:50051;
    server core-2:50051;
    server core-3:50051;
}

server {
    listen 443 ssl http2;
    server_name vpn.example.com;

    ssl_certificate /etc/ssl/certs/vpn.example.com.pem;
    ssl_certificate_key /etc/ssl/private/vpn.example.com.key;

    location / {
        proxy_pass http://outpost_http;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}

server {
    listen 50051 http2;

    location / {
        grpc_pass grpc://outpost_grpc;
    }
}
```

#### Gateway Deployment (Multiple Locations)

Each gateway runs on a separate machine with WireGuard access.

```bash
# On each gateway node (requires NET_ADMIN):
docker run -d \
  --name outpost-gateway \
  --cap-add=NET_ADMIN \
  --cap-add=SYS_MODULE \
  --device=/dev/net/tun \
  --sysctl net.ipv4.ip_forward=1 \
  -e OUTPOST_GATEWAY_CORE_ADDR=core-lb.example.com:50051 \
  -e OUTPOST_GATEWAY_TOKEN=$GATEWAY_TOKEN \
  -p 51820:51820/udp \
  outpost/gateway:latest
```

Register each gateway through the admin UI:
1. Navigate to **Gateways** page
2. Click **Create** — provide name, network, endpoint (public IP:51820)
3. Copy the generated token into the gateway's `OUTPOST_GATEWAY_TOKEN` env var
4. Restart the gateway container

#### Multi-Location S2S Topology

For connecting multiple office networks:

```
  Moscow Office          St. Petersburg            Novosibirsk
  ┌──────────┐          ┌──────────┐              ┌──────────┐
  │ gw-msk   │◄────────►│ gw-spb   │◄────────────►│ gw-nsk   │
  │10.1.0.0  │  S2S     │10.2.0.0  │    S2S       │10.3.0.0  │
  │  /24     │  tunnel   │  /24     │   tunnel     │  /24     │
  └──────────┘          └──────────┘              └──────────┘
       ▲                      ▲                        ▲
       │ WG                   │ WG                     │ WG
  ┌────┴────┐            ┌────┴────┐              ┌────┴────┐
  │ Clients │            │ Clients │              │ Clients │
  └─────────┘            └─────────┘              └─────────┘
```

Configure via admin UI:
1. Create a **Network** per location (e.g., `10.1.0.0/24`, `10.2.0.0/24`, `10.3.0.0/24`)
2. Deploy a **Gateway** at each location
3. Go to **S2S Tunnels** → Create tunnel → Select **mesh** or **hub-spoke** topology
4. Add gateway members with their local subnets
5. Routes are automatically exchanged between gateways via gRPC

### Kubernetes (Helm)

```bash
# Add the Outpost Helm repo
helm repo add outpost https://charts.outpost-vpn.io
helm repo update

# Install with default values
helm install outpost outpost/outpost \
  --namespace outpost \
  --create-namespace \
  --set core.replicas=3 \
  --set postgresql.auth.password=$DB_PASSWORD \
  --set redis.auth.password=$REDIS_PASSWORD \
  --set core.env.OUTPOST_JWT_SECRET=$JWT_SECRET

# Or use a values file
helm install outpost outpost/outpost \
  --namespace outpost \
  --create-namespace \
  -f values-production.yaml
```

Example `values-production.yaml`:

```yaml
core:
  replicas: 3
  resources:
    requests:
      cpu: 500m
      memory: 256Mi
    limits:
      cpu: 2000m
      memory: 1Gi
  env:
    OUTPOST_JWT_SECRET: "<secret>"
    OUTPOST_LOG_LEVEL: info
    OUTPOST_LOG_FORMAT: json
  ingress:
    enabled: true
    className: nginx
    hosts:
      - host: vpn.example.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: vpn-tls
        hosts:
          - vpn.example.com

gateway:
  # Deploy as DaemonSet on labeled nodes
  nodeSelector:
    outpost.io/gateway: "true"
  hostNetwork: true
  tolerations:
    - key: "outpost.io/gateway"
      operator: "Exists"
  env:
    OUTPOST_GATEWAY_TOKEN: "<token>"
  securityContext:
    capabilities:
      add: [NET_ADMIN, SYS_MODULE]

proxy:
  replicas: 2
  service:
    type: LoadBalancer
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: nlb

postgresql:
  # Use external managed DB in production
  enabled: false
  external:
    host: outpost-db.rds.amazonaws.com
    port: 5432
    database: outpost
    existingSecret: outpost-db-credentials

redis:
  # Use external managed Redis in production
  enabled: false
  external:
    host: outpost-redis.elasticache.amazonaws.com
    port: 6379
    existingSecret: outpost-redis-credentials

monitoring:
  serviceMonitor:
    enabled: true
    interval: 15s
  prometheusRule:
    enabled: true
```

## Environment Variables Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_HTTP_ADDR` | `:8080` | HTTP listen address |
| `OUTPOST_GRPC_ADDR` | `:9090` | gRPC listen address |
| `OUTPOST_DB_HOST` | `localhost` | PostgreSQL host |
| `OUTPOST_DB_PORT` | `5432` | PostgreSQL port |
| `OUTPOST_DB_NAME` | `outpost` | PostgreSQL database |
| `OUTPOST_DB_USER` | `outpost` | PostgreSQL user |
| `OUTPOST_DB_PASSWORD` | | PostgreSQL password |
| `OUTPOST_DB_SSLMODE` | `disable` | PostgreSQL SSL mode |
| `OUTPOST_DB_MAX_CONNS` | `20` | Max DB connections |
| `OUTPOST_REDIS_ADDR` | `localhost:6379` | Redis address |
| `OUTPOST_REDIS_PASSWORD` | | Redis password |
| `OUTPOST_JWT_SECRET` | | JWT signing secret (required) |
| `OUTPOST_TOKEN_TTL` | `15m` | JWT token TTL |
| `OUTPOST_SESSION_TTL` | `24h` | Session TTL |
| `OUTPOST_WG_INTERFACE` | `wg0` | WireGuard interface name |
| `OUTPOST_WG_LISTEN_PORT` | `51820` | WireGuard listen port |
| `OUTPOST_GATEWAY_TOKEN` | | Gateway auth token |
| `OUTPOST_GATEWAY_CORE_ADDR` | `localhost:9090` | Core gRPC address |
| `OUTPOST_SMTP_HOST` | | SMTP server (optional) |
| `OUTPOST_SMTP_PORT` | `587` | SMTP port |
| `OUTPOST_SMTP_USERNAME` | | SMTP auth user |
| `OUTPOST_SMTP_PASSWORD` | | SMTP auth password |
| `OUTPOST_SMTP_FROM` | | From email address |
| `OUTPOST_SMTP_TLS` | `false` | Enable STARTTLS |
| `OUTPOST_LOG_LEVEL` | `info` | Log level (debug/info/warn/error) |
| `OUTPOST_LOG_FORMAT` | `json` | Log format (json/text) |
| `OUTPOST_OIDC_ISSUER` | `http://localhost:8080` | OIDC issuer URL |
| `OUTPOST_SAML_ENABLED` | `false` | Enable SAML 2.0 SP |
| `OUTPOST_LDAP_ENABLED` | `false` | Enable LDAP sync |

## Backup & Recovery

### PostgreSQL

```bash
# Backup
pg_dump -h localhost -U outpost -d outpost > outpost_backup_$(date +%Y%m%d).sql

# Restore
psql -h localhost -U outpost -d outpost < outpost_backup_20260313.sql
```

### Key data to backup:
- PostgreSQL database (all state)
- Redis (optional — sessions will be recreated)
- SSL/TLS certificates
- Environment variables / secrets

## Monitoring

outpost-core exposes Prometheus metrics at `/metrics`:

```yaml
# prometheus.yml scrape config
scrape_configs:
  - job_name: 'outpost'
    static_configs:
      - targets: ['core-1:8080', 'core-2:8080', 'core-3:8080']
    metrics_path: /metrics
```

Key metrics:
- `outpost_active_peers` — currently connected WireGuard peers
- `outpost_bandwidth_bytes_total` — total traffic (rx/tx)
- `outpost_gateway_last_seen_seconds` — gateway health
- `outpost_auth_attempts_total` — authentication attempts
- `outpost_s2s_tunnel_status` — S2S tunnel health

## Troubleshooting

| Symptom | Check |
|---------|-------|
| Can't connect to VPN | `wg show` on gateway, check firewall, check device approval |
| Gateway offline | Core logs, gateway token, gRPC connectivity |
| Slow handshakes | MTU settings, DNS resolution, endpoint reachability |
| S2S tunnel down | Both gateways online, route exchange, subnet conflicts |
| MFA not working | Time sync (NTP), TOTP secret validity |
