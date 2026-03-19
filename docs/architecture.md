# Architecture Overview

Outpost VPN is a monorepo with a single Go module. It consists of four main binaries, a CLI tool, a React frontend, and shared libraries.

## Component Diagram

```
                        Internet / DMZ
  ┌──────────────────────────────────────────────────────────┐
  │                                                          │
  │   ┌────────────────┐           ┌────────────────┐       │
  │   │  outpost-proxy │           │  VPN clients   │       │
  │   │     :8081      │           │  (WireGuard)   │       │
  │   └───────┬────────┘           └───────┬────────┘       │
  │           │                            │                │
  └───────────┼────────────────────────────┼────────────────┘
              │ gRPC                       │ UDP :51820
  ┌───────────┼────────────────────────────┼────────────────┐
  │           │      Internal Network      │                │
  │   ┌───────▼────────┐          ┌────────▼───────┐       │
  │   │  outpost-core  │◄────────►│ outpost-gateway│       │
  │   │ :8080    :50051│  gRPC    │   :51820/udp   │       │
  │   │                │  bidir.  │                │       │
  │   │  HTTP API      │  stream  │  WireGuard     │       │
  │   │  Admin panel   │          │  Firewall      │       │
  │   │  OIDC provider │          │  S2S tunnels   │       │
  │   │  gRPC hub      │          │                │       │
  │   └───────┬────────┘          └────────────────┘       │
  │           │                                             │
  │   ┌───────▼──────┐    ┌──────────┐                     │
  │   │  PostgreSQL  │    │  Redis   │                     │
  │   │    :5432     │    │  :6379   │                     │
  │   └──────────────┘    └──────────┘                     │
  └─────────────────────────────────────────────────────────┘
```

## Binaries

### outpost-core

The central control plane. Runs the HTTP REST API (Chi router), serves the embedded React frontend, provides a built-in OIDC identity provider, and hosts the gRPC hub that gateways connect to.

**Responsibilities:**
- HTTP API for all management operations (users, devices, networks, gateways, etc.)
- JWT authentication with MFA support (TOTP, WebAuthn, backup codes)
- OIDC provider, SAML 2.0 SP, LDAP/AD sync, SCIM 2.0 provisioning
- gRPC server for gateway communication (bidirectional streaming)
- StreamHub for broadcasting peer updates to all connected gateways
- Embedded database migrations (auto-run on startup via `go:embed`)
- Embedded React frontend (SPA via `go:embed`)
- Prometheus metrics at `/metrics`
- Audit logging of all mutating operations

**Source:** `cmd/outpost-core/main.go` + `internal/core/`

### outpost-gateway

The WireGuard data plane agent. Runs on machines with WireGuard support and manages WireGuard interfaces, peer configurations, and firewall rules.

**Responsibilities:**
- Maintains a persistent bidirectional gRPC stream to outpost-core
- Applies peer configurations received from core (add/remove/update WireGuard peers)
- Manages two WireGuard interfaces: `wg0` for client traffic, `wg1` for S2S tunnels
- Sends peer statistics (rx/tx bytes, handshake time) back to core
- Enforces firewall rules and NAT configurations
- Site-to-site tunnel management with route exchange

**Source:** `cmd/outpost-gateway/main.go` + `internal/gateway/`

### outpost-proxy

A lightweight DMZ-safe proxy. Designed for device enrollment and authentication. Intended to be exposed to the internet while outpost-core stays in the internal network.

**Responsibilities:**
- Proxies enrollment and authentication requests to core via gRPC
- Stateless: no database or Redis connections
- Minimal attack surface for DMZ deployment

**Source:** `cmd/outpost-proxy/main.go` + `internal/proxy/`

### outpost-client

The VPN client. Cross-platform (Linux, macOS, Windows) with MFA support, device posture reporting, and tunnel management.

**Responsibilities:**
- Authentication with core (JWT + MFA flow)
- Establishing WireGuard tunnels
- Device posture reporting (OS version, disk encryption, antivirus, screen lock)
- Tunnel lifecycle management (connect, disconnect, reconnect)
- Multi-gateway HA failover (automatic endpoint switching on handshake loss)

**Source:** `cmd/outpost-client/main.go` + `internal/client/`

### outpostctl

CLI tool for administrative operations without the web interface.

**Source:** `cmd/outpostctl/main.go`

## Internal Package Structure

```
internal/
├── analytics/      Traffic analytics: flow records, bandwidth, top users, heatmaps
├── auth/           Authentication subsystem
│   ├── mfa/        TOTP, WebAuthn/FIDO2, backup codes
│   ├── oidc/       Built-in OpenID Connect provider
│   ├── saml/       SAML 2.0 Service Provider
│   └── scim/       SCIM 2.0 user/group provisioning
├── client/         VPN client library (cross-platform)
├── compliance/     SOC2, ISO27001, GDPR readiness checks
├── config/         Configuration from environment variables
├── core/           HTTP handlers, gRPC implementation, StreamHub
│   └── handler/    REST API handlers (Chi)
├── db/             PostgreSQL connection pool (pgx/v5), sqlc queries
├── gateway/        WireGuard interface management, firewall, S2S
├── mail/           Email notifications with i18n templates
├── nat/            NAT traversal (STUN/TURN relay management)
├── observability/  Prometheus metrics, audit logging, SIEM integration
├── pki/            Built-in PKI: key rotation, certificate lifecycle
├── proxy/          Enrollment/auth proxy logic
├── s2s/            Site-to-site: mesh, hub-spoke, route exchange
├── session/        Session management
├── tenant/         Multi-tenant platform (MSP/reseller mode)
├── webhook/        Webhook dispatcher
├── wireguard/      WireGuard abstraction: kernel + userspace
└── ztna/           Zero-Trust: device posture checks, continuous verification
```

## Communication: Bidirectional gRPC Streaming

Core-gateway communication uses bidirectional gRPC streaming, providing real-time, low-latency updates in both directions.

```
outpost-core                        outpost-gateway
     │                                    │
     │◄── GatewayEvent (stream) ─────────│  (stats, health, route advertisements)
     │                                    │
     │────── CoreEvent (stream) ─────────►│  (peer updates, config changes)
     │                                    │
```

**GatewayEvent** (gateway -> core):
- Peer statistics (rx/tx bytes, last handshake)
- Health reports
- S2S route advertisements (`local_subnets`)

**CoreEvent** (core -> gateway):
- Peer updates (add/remove/modify WireGuard peers)
- Configuration changes
- S2S route tables (computed by the topology engine)

### StreamHub Broadcasting Pattern

`StreamHub` (`internal/core/stream_hub.go`) manages all active gateway gRPC streams:

```go
type StreamHub struct {
    streams map[string]streamSender  // key is gateway ID
}
```

When an API action requires sending changes to gateways (e.g., approving a device), the handler calls `StreamHub.BroadcastPeerUpdate()`, which sends the event to all connected gateways simultaneously. For targeted updates, `StreamHub.SendTo(gatewayID, event)` sends to a specific gateway.

**Flow: device approval triggers peer sync**
```
Admin approves device via API
        │
        ▼
DeviceHandler updates DB (is_approved=true)
        │
        ▼
DeviceHandler calls StreamHub.BroadcastPeerUpdate()
        │
        ▼
StreamHub iterates over all connected gateway streams
        │
        ▼
Each gateway receives PeerUpdate, applies WireGuard configuration
```

## Database Schema Overview

PostgreSQL 17 with the `pgcrypto` extension. All IDs are UUIDs. Timestamps are `TIMESTAMPTZ`.

### Core Tables

| Table | Purpose |
|-------|---------|
| `users` | User accounts (username, email, password hash, MFA status, LDAP/SCIM links) |
| `groups` | User groups (everyone, admins, custom) |
| `user_groups` | User-group many-to-many association |
| `roles` | RBAC roles (admin, user, viewer) with JSON permissions |
| `user_roles` | User-role many-to-many association |
| `networks` | WireGuard networks (CIDR, DNS, port, keepalive) |
| `devices` | WireGuard peers (public key, assigned IP, approval status) |
| `gateways` | WireGuard gateways (endpoint, public key, token hash, last seen, health_status, consecutive_failures) |
| `network_acls` | Group-based network access ACLs |
| `sessions` | User sessions (indexed by user_id and expires_at) |
| `settings` | Application settings in key-value format (JSONB) |
| `audit_log` | Immutable audit log (action, resource, details, IP) |

### Authentication Tables

| Table | Purpose |
|-------|---------|
| `mfa_totp` | TOTP secrets (per user, verification flag) |
| `mfa_webauthn` | WebAuthn/FIDO2 credentials (credential_id, public_key, sign_count) |
| `mfa_backup_codes` | One-time backup codes (hashed, used flag) |
| `oidc_clients` | OIDC relying party clients |
| `oidc_auth_codes` | OIDC authorization codes (with PKCE support) |
| `enrollment_tokens` | Device enrollment tokens (hashed, expiry, used flag) |

### Site-to-Site Tables

| Table | Purpose |
|-------|---------|
| `s2s_tunnels` | Tunnel metadata (name, topology: mesh/hub_spoke, hub_gateway_id) |
| `s2s_tunnel_members` | Gateway membership with `local_subnets CIDR[]` |
| `s2s_routes` | Computed routes (destination CIDR, via_gateway, metric) |

### Analytics and Monitoring

| Table | Purpose |
|-------|---------|
| `peer_stats` | WireGuard peer traffic statistics (partitioned by `recorded_at`) |

### Additional Tables (from later migrations)

| Table | Purpose |
|-------|---------|
| `smart_routes` | Selective proxy routing groups |
| `smart_route_entries` | Route entries (domain, CIDR, domain_suffix) with actions |
| `proxy_servers` | Upstream proxy servers (SOCKS5, HTTP, Shadowsocks, VLESS) |
| `network_smart_routes` | Network-smart route associations |
| `webhooks` | Webhook endpoint configurations |
| `trust_scores` | ZTNA device trust scores |
| `nat_relays` | NAT traversal relay servers |
| `password_resets` | Password reset tokens |

## Assignment Model: Device → Network

A user gains access to a VPN network **through a device**:

```
User (1) ──► (N) Devices ──► (1) Network
                                    │
                               IP from network CIDR
```

- A single user can have multiple devices across different networks
- Each device is bound to one network and receives an IP address from that network's CIDR
- A device requires admin approval (`is_approved`)
- When a device is approved, core broadcasts a `PeerUpdate` to all gateways via StreamHub

## Bandwidth Statistics Pipeline

Traffic data flows from gateway to dashboard through the following path:

```
outpost-gateway                outpost-core                 PostgreSQL
     │                              │                           │
     │  GatewayEvent (gRPC stream)  │                           │
     │  {peer_stats: [              │                           │
     │    {public_key, rx, tx,      │                           │
     │     last_handshake}          │                           │
     │  ]}                          │                           │
     │─────────────────────────────►│                           │
     │                              │  INSERT INTO peer_stats   │
     │                              │  (partitioned by          │
     │                              │   recorded_at)            │
     │                              │──────────────────────────►│
     │                              │                           │
     │                              │                           │
                                    │                           │
     Frontend (dashboard)           │                           │
     │                              │                           │
     │  GET /analytics/bandwidth    │                           │
     │─────────────────────────────►│  SELECT ... FROM          │
     │                              │  peer_stats               │
     │                              │  WHERE recorded_at        │
     │                              │  BETWEEN $from AND $to    │
     │                              │──────────────────────────►│
     │                              │◄──────────────────────────│
     │◄── {data: [{time, rx, tx}]} ─│                           │
     │                              │                           │
```

1. **Gateway** periodically sends peer statistics (rx/tx bytes, handshake time) via the gRPC stream
2. **Core** stores the statistics in the `peer_stats` table, partitioned by `recorded_at`
3. **Analytics API** aggregates data by the requested interval (`GET /analytics/bandwidth?interval=1h`)
4. **Dashboard** displays the data as an area chart (Recharts) and a sparkline

Other analytics endpoints (`/analytics/top-users`, `/analytics/connections-heatmap`, `/analytics/summary`) also operate on the `peer_stats` table.

## Authentication Flow

### Login (JWT + MFA)

```
Client                    outpost-core                    PostgreSQL
  │                            │                              │
  │  POST /api/v1/auth/login   │                              │
  │  {username, password}      │                              │
  │───────────────────────────►│                              │
  │                            │  Verify password (bcrypt)    │
  │                            │─────────────────────────────►│
  │                            │◄─────────────────────────────│
  │                            │                              │
  │  (if MFA enabled)          │                              │
  │◄── {mfa_required: true} ───│                              │
  │                            │                              │
  │  POST /api/v1/auth/mfa/verify                             │
  │  {code, session_token}     │                              │
  │───────────────────────────►│                              │
  │                            │  Verify TOTP/WebAuthn        │
  │                            │─────────────────────────────►│
  │◄── {token: "eyJ..."} ─────│                              │
  │                            │                              │
```

**Token details:**
- JWT signed with `OUTPOST_JWT_SECRET` (HMAC-SHA256)
- Default TTL: 15 minutes (`OUTPOST_TOKEN_TTL`)
- Session TTL: 24 hours (`OUTPOST_SESSION_TTL`)
- Refresh via `POST /api/v1/auth/refresh`
- Rate limiting: 10 requests/minute per IP for auth endpoints

### JWT Middleware

All protected API routes pass through `auth.JWTMiddleware(secret)`, which:
1. Extracts the Bearer token from the `Authorization` header
2. Validates the JWT signature and expiration
3. Adds user claims to the request context
4. Admin-only routes additionally check `auth.RequireAdmin`

## VPN Connection Pipeline

The full flow from user enrollment to an active VPN connection:

```
1. ENROLLMENT
   Admin creates a user → User receives an enrollment link/token
   User enrolls a device → Device generates a WireGuard key pair
   Device sends public key → Core saves the device (is_approved=false)

2. APPROVAL
   Admin approves device → Core sets is_approved=true in the DB
   Core calls StreamHub.BroadcastPeerUpdate()
   All gateways receive PeerUpdate with the new peer configuration

3. CONFIGURATION
   User downloads WireGuard config (GET /api/v1/devices/{id}/config)
   Config includes: private key, assigned IP, gateway endpoint, allowed IPs

4. CONNECTION
   User imports config into WireGuard client
   WireGuard handshake with gateway
   Gateway verifies the peer's public key against its peer list

5. PEER SYNC (continuous)
   Gateway sends peer stats to core (rx/tx, handshake time)
   Core stores stats in the peer_stats table (partitioned)
   Dashboard shows real-time connection status
```

### Session Expiry and Protocol-Level MFA

Outpost implements MFA at the WireGuard protocol level: when a user's session expires or MFA re-verification fails, the peer is removed from the WireGuard configuration on the gateway. This is more secure than application-level MFA because the VPN tunnel itself is torn down, not just the application session.

```
Session expires or trust score drops
        │
        ▼
Core sends PeerUpdate (action=REMOVE) via StreamHub
        │
        ▼
Gateway removes peer from WireGuard interface
        │
        ▼
Client loses VPN connectivity immediately
        │
        ▼
Client must re-authenticate with MFA to reconnect
```

## Gateway High Availability

Outpost supports multi-gateway deployments for eliminating single points of failure. Multiple gateways can serve the same network, and clients automatically failover when a gateway becomes unresponsive.

```
                     outpost-core
                          │
              ┌───────────┼───────────┐
              │           │           │
        heartbeat    heartbeat    heartbeat
        (30s)        (30s)        (30s)
              │           │           │
       ┌──────▼──┐  ┌─────▼───┐  ┌───▼──────┐
       │gateway-1│  │gateway-2│  │gateway-3 │
       │pri: 100 │  │pri: 90  │  │pri: 80   │
       │healthy  │  │healthy  │  │unhealthy │
       └────┬────┘  └────┬────┘  └──────────┘
            │             │
     VPN clients failover automatically
     from gateway-1 to gateway-2
     if no handshake for 45s
```

**Health monitoring flow:**
1. Each gateway sends a heartbeat every 30 seconds via gRPC stream
2. Core updates `last_seen`, sets `health_status = 'healthy'`, resets `consecutive_failures = 0`
3. `MonitorGatewayHealth()` goroutine runs every 60 seconds, checks for stale gateways (`last_seen > 90s`)
4. Stale gateways get `consecutive_failures++`; after 3 failures → `health_status = 'unhealthy'`
5. On transition to unhealthy, core pushes a FullResync to peer gateways on the same network

**Client-side failover flow:**
1. Client receives ordered list of gateways at enrollment (sorted by priority DESC)
2. Health ticker (15s) checks WireGuard last handshake time
3. If no handshake for >45s (3 consecutive checks), client switches peer endpoint to next gateway
4. WireGuard peer is reconfigured in-place without tearing down the TUN interface

## Network Topology

### Docker Compose Networks

The default Docker Compose deployment uses two networks:

- **internal:** Core, gateway, PostgreSQL, Redis (private)
- **dmz:** Core, proxy (accessible from the internet)

The proxy sits in the DMZ network and communicates with core via gRPC, while the gateway is in the internal network for security.

### Multi-Site Deployment

```
  Site A                   Site B                  Site C
  ┌──────────┐            ┌──────────┐            ┌──────────┐
  │ gateway-a│◄──────────►│ gateway-b│◄──────────►│ gateway-c│
  │10.1.0/24 │  S2S (wg1) │10.2.0/24 │  S2S (wg1) │10.3.0/24 │
  │          │            │          │            │          │
  │  wg0:    │            │  wg0:    │            │  wg0:    │
  │  clients │            │  clients │            │  clients │
  └────┬─────┘            └────┬─────┘            └────┬─────┘
       │                       │                       │
  ┌────▼─────┐            ┌────▼─────┐            ┌────▼─────┐
  │ Clients  │            │ Clients  │            │ Clients  │
  └──────────┘            └──────────┘            └──────────┘
```

Each gateway runs two WireGuard interfaces:
- `wg0` (port 51820) — client VPN traffic
- `wg1` (port 51821) — site-to-site tunnel traffic

## Observability

### Metrics

Prometheus metrics are available at `/metrics` on the core HTTP server (no authentication).

Key metrics:
- `outpost_active_peers` — currently connected WireGuard peers
- `outpost_bandwidth_bytes_total` — total traffic (rx/tx labels)
- `outpost_gateway_last_seen_seconds` — gateway health
- `outpost_auth_attempts_total` — authentication attempts (success/failure labels)
- `outpost_s2s_tunnel_status` — S2S tunnel status

### Audit Logging

All mutating HTTP requests (POST, PUT, PATCH, DELETE) are logged to the `audit_log` table via `observability.AuditMiddleware`. Each entry contains:
- Timestamp, user ID, action, resource type
- Request details (JSONB)
- Client IP address and user agent

Audit logs are available via `GET /api/v1/audit` with filtering and export capabilities.

### Structured Logging

All components use the Go `slog` package with configurable level and format:
- Levels: `debug`, `info`, `warn`, `error`
- Formats: `json` (production), `text` (development)

## Horizontal Scaling (Multi-Core)

Outpost is designed to run multiple `outpost-core` instances behind a load balancer for both performance and fault tolerance. No etcd or external consensus system is required — PostgreSQL and Redis (already in the stack) provide all necessary coordination.

### What already scales without changes

| Component | Why it scales |
|-----------|--------------|
| HTTP API | Fully stateless — all state lives in PostgreSQL. Any core can serve any request. |
| Gateways | Already reconnect with exponential backoff when the gRPC stream drops. If one core goes down, gateways reconnect to another. |
| Sessions | Already stored in Redis (shared across cores). |
| JWT auth | Symmetric HMAC-SHA256 — any core with the same `OUTPOST_JWT_SECRET` can validate tokens. |
| Frontend | Static SPA served via `go:embed` — no server-side state. |
| Database | PostgreSQL is the single source of truth. Connection pooling via pgx is per-core. |

### What needs Redis Pub/Sub (one change)

The `StreamHub` holds gRPC streams in memory. When Core-1 approves a device, only gateways connected to Core-1 receive the `PeerUpdate`. Gateways connected to Core-2 don't know about it until the next periodic FullResync.

**Solution:** When a core broadcasts a peer/firewall/config update, it also publishes the event to a Redis Pub/Sub channel (`outpost:events`). Other cores subscribe to this channel and forward the event to their locally connected gateways.

```
Admin approves device on Core-1
    → Core-1 writes to PostgreSQL
    → Core-1 sends PeerUpdate to its gateways (StreamHub)
    → Core-1 publishes event to Redis "outpost:events"
    → Core-2 receives event from Redis
    → Core-2 sends PeerUpdate to its gateways (StreamHub)
```

If a Redis Pub/Sub message is lost (Redis is fire-and-forget), the existing periodic FullResync ensures eventual consistency. This gives us strong consistency in the common case and eventual consistency as a fallback.

### Singleton tasks (leader election)

Some goroutines must run on exactly one core to avoid duplicate work:

| Task | Current | Multi-core solution |
|------|---------|-------------------|
| `MonitorGatewayHealth` | Runs on the single core | `pg_try_advisory_lock(1)` — only one core acquires |
| `managePeerStatsPartitions` | Runs on the single core | `pg_try_advisory_lock(2)` — only one core acquires |
| Token blacklist cleanup | Runs on the single core | `pg_try_advisory_lock(3)` — only one core acquires |

PostgreSQL advisory locks are released automatically when the connection drops, so if the leader core dies, another core acquires the lock on the next tick.

### Rate limiting

The in-memory `ipRateLimiter` must move to Redis (`INCR` + `EXPIRE`) so that rate limits are enforced globally across all cores, not per-core.

### Deployment topology

```
                    Load Balancer (L4 for gRPC, L7 for HTTP)
                         │
              ┌──────────┼──────────┐
              │          │          │
         ┌────▼───┐ ┌────▼───┐ ┌────▼───┐
         │Core-1  │ │Core-2  │ │Core-3  │
         │StreamHub│ │StreamHub│ │StreamHub│
         └──┬───┬─┘ └──┬───┬─┘ └──┬───┬─┘
            │   │       │   │       │   │
            │   └───────┤   ├───────┘   │
            │     Redis Pub/Sub         │
            │           │               │
        ┌───▼──┐   ┌───▼──┐       ┌───▼──┐
        │ GW-1 │   │ GW-2 │       │ GW-3 │
        └──────┘   └──────┘       └──────┘

         ┌──────────┐    ┌────────┐
         │PostgreSQL│    │ Redis  │
         │ (truth)  │    │(pubsub)│
         └──────────┘    └────────┘
```

**Small (dev/testing):** 1 core, 1 gateway, 1 PG, 1 Redis — works as-is, no changes needed.

**HA (production):** 2–3 cores behind a load balancer, 2+ gateways per network, PostgreSQL with streaming replica, Redis Sentinel.

**Load balancer requirements:**
- HTTP: standard L7 (nginx, HAProxy, cloud ALB)
- gRPC: L4 or gRPC-aware L7 (envoy, nginx with `grpc_pass`, cloud NLB)
- Gateway gRPC connections are long-lived; use connection-based (L4) balancing, not per-request

### Why not etcd / NATS / Kafka?

| Option | Verdict | Reason |
|--------|---------|--------|
| etcd | Overkill | 3+ nodes for quorum, TLS certs, compaction — massive operational burden for what we need |
| NATS | Unnecessary | Adds a new dependency; Redis Pub/Sub covers our use case |
| Kafka | Way overkill | We need fire-and-forget event notification, not durable streaming |
| Redis Pub/Sub | Chosen | Already deployed, zero new dependencies, simple, adequate with FullResync fallback |
| PostgreSQL LISTEN/NOTIFY | Alternative | Works but 8KB payload limit; Redis is more flexible and already handles sessions |

### TLS for gRPC

In production multi-core deployments, gRPC traffic between gateways and cores should use TLS:

```bash
# Generate CA and certificates
outpostctl pki generate-ca --out /etc/outpost/pki/
outpostctl pki generate-cert --ca /etc/outpost/pki/ca.crt --ca-key /etc/outpost/pki/ca.key \
    --cn outpost-core --san "core-1.outpost.local,core-2.outpost.local" \
    --out /etc/outpost/pki/core.crt --key-out /etc/outpost/pki/core.key
outpostctl pki generate-cert --ca /etc/outpost/pki/ca.crt --ca-key /etc/outpost/pki/ca.key \
    --cn outpost-gateway --out /etc/outpost/pki/gw.crt --key-out /etc/outpost/pki/gw.key
```

Environment variables:
- `OUTPOST_GRPC_TLS_CERT` — path to core TLS certificate
- `OUTPOST_GRPC_TLS_KEY` — path to core TLS private key
- `OUTPOST_GRPC_TLS_CA` — path to CA certificate (for gateway verification)
- `OUTPOST_GATEWAY_TLS_CERT` — gateway certificate
- `OUTPOST_GATEWAY_TLS_KEY` — gateway private key
- `OUTPOST_GATEWAY_TLS_CA` — CA certificate (for core verification)

When TLS env vars are set, gRPC server uses `credentials.NewTLS()` instead of `grpc.Creds(insecure)`. Gateways verify the core certificate against the CA. Mutual TLS (mTLS) is supported — core verifies gateway certificates when `OUTPOST_GRPC_TLS_CA` is set.

## Technology Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| HTTP router | Chi | Lightweight, idiomatic Go, convenient middleware |
| DB driver | pgx/v5 | Pure Go, connection pooling, native PostgreSQL types |
| SQL code generation | sqlc | Type-safe queries without ORM overhead |
| gRPC | google.golang.org/grpc | Bidirectional streaming for real-time gateway communication |
| Frontend state | Zustand + TanStack Query | Zustand for UI state, TanStack for server state with caching |
| Frontend styling | Tailwind CSS 4 | Utility-first approach, no CSS-in-JS runtime cost |
| Protobuf tooling | Buf | Linting, breaking change detection, code generation |
