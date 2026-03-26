<p align="center">
  <a href="README.md">Русский</a> | <a href="README.en.md">English</a>
</p>

<p align="center">
  <img src="assets/banner.svg" alt="Outpost VPN" width="700" />
</p>

<p align="center">
  <strong>Open-Source WireGuard VPN & Zero Trust Network Access Platform</strong><br/>
  Enterprise-grade security. No paid restrictions. Apache 2.0 forever.
</p>

<p align="center">
  <a href="https://github.com/romashqua/outpost/actions"><img src="https://img.shields.io/github/actions/workflow/status/romashqua/outpost/ci.yml?branch=main&style=flat-square&label=CI" alt="CI" /></a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=black" />
  <img src="https://img.shields.io/badge/WireGuard-88171A?style=flat-square&logo=wireguard&logoColor=white" />
  <img src="https://img.shields.io/badge/PostgreSQL-17-4169E1?style=flat-square&logo=postgresql&logoColor=white" />
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-green?style=flat-square" alt="License" /></a>
  <a href="https://github.com/romashqua/outpost/stargazers"><img src="https://img.shields.io/github/stars/romashqua/outpost?style=flat-square" alt="Stars" /></a>
  <a href="https://github.com/romashqua/outpost/releases"><img src="https://img.shields.io/github/v/release/romashqua/outpost?style=flat-square&label=Release&include_prereleases" alt="Release" /></a>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="#features">Features</a> &middot;
  <a href="#architecture">Architecture</a> &middot;
  <a href="#screenshots">Screenshots</a> &middot;
  <a href="#comparison">Comparison</a> &middot;
  <a href="docs/">Documentation</a> &middot;
  <a href="CONTRIBUTING.md">Contributing</a>
</p>

---

## Why Outpost?

Every other open-source VPN solution either hides critical features behind a paid enterprise tier or simply lacks them. Outpost gives you **everything** -- Zero Trust device posture checks, compliance dashboard, multi-tenant MSP mode, traffic analytics, built-in OIDC/SAML/SCIM, PKI with key rotation -- fully open-source under Apache 2.0 from day one.

**No enterprise tier. No feature gating. Never.**

### Who It's For

- **Teams** that need an enterprise VPN without vendor lock-in
- **MSP/MSSPs** providing VPN-as-a-service to multiple clients
- **DevOps/Platform engineers** building Zero Trust infrastructure
- **Regulated industries** that need audit logs and automated compliance checks
- **Everyone** tired of paying for features that should be standard

---

## Quick Start

Get a fully working Outpost in under 60 seconds:

```bash
git clone https://github.com/romashqua/outpost.git
cd outpost
docker compose -f deploy/docker/docker-compose.yml up -d
```

Open **http://localhost:8080** and log in with `admin` / `admin`.

That's it. Database migrations run automatically, all services start in the correct order, and the React UI is embedded in the binary.

> **Production deployment?** See the [Deployment Guide](docs/deployment.md) for Helm charts, TLS configuration, and hardening recommendations.

---

## Features

### VPN & Networking

| Feature | Description |
|---|---|
| **WireGuard Tunnels** | Kernel and userspace support with automatic key management |
| **Site-to-Site** | Full mesh and hub-spoke topologies with automatic route synchronization |
| **NAT Traversal** | Built-in STUN/TURN relay servers for restricted networks |
| **Smart Routes** | Selective domain/CIDR routing through proxy servers (SOCKS5, HTTP, Shadowsocks, VLESS) |
| **Real-Time Sync** | gRPC bidirectional streaming between core and gateways |

### Zero Trust Network Access (ZTNA)

| Feature | Description |
|---|---|
| **Device Posture Checks** | Disk encryption, antivirus, firewall, OS version, screen lock |
| **Continuous Verification** | Periodic reassessment of device trust level |
| **Configurable Policies** | Customize weights and thresholds for your security model |
| **Protocol-Level MFA** | WireGuard peer removed from gateway on session expiry |
| **DNS Access Rules** | DNS filtering per device and split-horizon DNS |

### Identity & Access Management

| Feature | Description |
|---|---|
| **Built-in OIDC Provider** | "Sign in with Outpost" (Authorization Code + PKCE, RS256) |
| **MFA/2FA** | TOTP, WebAuthn/FIDO2, email OTP, backup codes |
| **External SSO** | Google, Azure AD, Okta, Keycloak via OIDC |
| **LDAP/Active Directory** | Full directory sync with group mapping |
| **SAML 2.0** | Service Provider mode for enterprise IdPs |
| **SCIM 2.0** | Automatic user/group provisioning from Okta, Azure AD |
| **RBAC** | Role-based access control with granular network ACLs |

### Analytics & Compliance

| Feature | Description |
|---|---|
| **Traffic Analytics** | Real-time bandwidth charts, top users, connection heatmaps |
| **Compliance Dashboard** | Automated SOC 2, ISO 27001, GDPR readiness scoring |
| **Audit Log** | Every admin action recorded with full context and export |
| **SIEM Integration** | Export via webhooks and syslog with HMAC signature verification |

### Platform

| Feature | Description |
|---|---|
| **Multi-Tenancy** | MSP/reseller mode with full organization isolation |
| **Built-in PKI** | Automatic WireGuard key rotation with zero downtime |
| **Visual Network Map** | Interactive SVG topology of the entire VPN mesh |
| **Gateway HA** | Multi-gateway failover with automatic health monitoring and endpoint switching |
| **Horizontal Scaling** | Multi-core behind LB without etcd — PG + Redis Pub/Sub, zero overhead with 1 core |
| **Email Notifications** | Configurable SMTP for MFA, device enrollment, alerts (i18n) |
| **Prometheus Metrics** | Full observability with ready-made dashboards |
| **Docker & Kubernetes** | docker-compose for development, Helm charts for production |

---

## Architecture

```
                          Internet
                              │
                   ┌──────────┴──────────┐
                   │   outpost-proxy     │  DMZ-safe enrollment
                   │       :8081         │  and auth proxy
                   └──────────┬──────────┘
                              │
                   ┌──────────┴──────────┐
                   │   Load Balancer     │  L4/L7 (nginx, envoy, HAProxy)
                   └────┬───────────┬────┘
                        │           │
              ┌─────────┴──┐  ┌────┴─────────┐
              │  core-1    │  │  core-2      │  N cores (stateless)
              │ :8080 HTTP │  │ :8080 HTTP   │  Redis Pub/Sub for
              │ :50051 gRPC│  │ :50051 gRPC  │  cross-core events
              └──┬─────┬───┘  └──┬─────┬─────┘
                 │     │         │     │
          gRPC streaming    gRPC streaming
                 │     │         │     │
              ┌──┴──┐ ┌┴────┐ ┌─┴───┐
              │GW-1 │ │GW-2 │ │GW-3 │  N gateways per network
              │51820│ │51820│ │51820│  WireGuard UDP
              └──┬──┘ └──┬──┘ └──┬──┘
                 │       │       │
             Clients  Clients  Clients
```

| Component | Role | Default Ports |
|---|---|---|
| `outpost-core` | API, OIDC provider, admin panel, gRPC control plane | 8080 (HTTP), 50051 (gRPC) |
| `outpost-gateway` | WireGuard data plane -- client VPN and S2S tunnels | 51820/udp |
| `outpost-proxy` | Public enrollment/auth proxy, DMZ-safe | 8081 |
| `outpost-client` | Cross-platform VPN client with MFA and device posture reporting | -- |
| `outpostctl` | CLI management and automation tool | -- |

**Scaling:** Core is fully stateless — any number of instances behind a LB. PostgreSQL is the single source of truth. Redis Pub/Sub coordinates events between core instances (peer updates, firewall changes). No etcd or external consensus needed — PG advisory locks for singleton tasks (health monitor, cron). See [docs/architecture.md](docs/architecture.md) for details.

---

## Technology Stack

| Layer | Technology |
|---|---|
| **Backend** | Go 1.24+, Chi (HTTP router), gRPC (inter-service communication), pgx/v5 + sqlc |
| **Frontend** | React 19, TypeScript, Vite, Tailwind CSS 4, Zustand, TanStack Query, Recharts |
| **Database** | PostgreSQL 17 with golang-migrate |
| **Cache** | Redis 7 (sessions, pub/sub, rate limiting) |
| **VPN** | WireGuard (kernel via netlink + userspace via wireguard-go) |
| **Authentication** | Built-in OIDC, SAML 2.0, LDAP/AD, SCIM 2.0, WebAuthn, TOTP |
| **Protobuf** | Buf-managed proto definitions with gRPC code generation |
| **Observability** | Prometheus, structured logging (slog), audit log, SIEM webhooks |
| **Deployment** | Docker, docker-compose, Helm, GitHub Actions CI |

---

## Screenshots

> Screenshots coming soon. The UI uses a dark cyberpunk theme with `#00ff88` accent color and JetBrains Mono font.
>
> For a preview: `docker compose -f deploy/docker/docker-compose.yml up -d` and open http://localhost:8080

---

## API

Outpost provides a full-featured REST API at `/api/v1/` with JWT authentication. The complete OpenAPI specification is available at `/api/docs/openapi.yaml`.

```
Auth:         POST /auth/login, /auth/mfa/verify, /auth/refresh, /auth/logout
Users:        GET/POST /users, GET/PUT/DELETE /users/{id}
Groups:       GET/POST /groups, members, ACLs
Networks:     GET/POST /networks, GET/PUT/DELETE /networks/{id}
Devices:      GET/POST /devices, /devices/enroll, approve, revoke, download config
Gateways:     GET/POST /gateways, GET/PUT/DELETE /gateways/{id}
S2S Tunnels:  GET/POST/DELETE /s2s-tunnels, members, routes, config
Smart Routes: GET/POST/PUT/DELETE /smart-routes, entries, proxy servers
ZTNA:         GET/PUT /ztna/trust-config, policies, DNS rules
Analytics:    GET /analytics/summary, bandwidth, top-users, heatmap
Compliance:   GET /compliance/report, soc2, iso27001, gdpr
Tenants:      GET/POST /tenants, GET/PUT/DELETE /tenants/{id}, stats
Audit:        GET /audit, /audit/stats, /audit/export
SCIM 2.0:     /scim/v2/Users, /scim/v2/Groups (bearer token auth)
```

> Full [API Reference](docs/API.md) with request/response descriptions and examples.

---

## Comparison

| Feature | Outpost | defguard | NetBird | Tailscale | Firezone |
|---|:---:|:---:|:---:|:---:|:---:|
| **Fully Open Source** | Yes | Partial (enterprise paywall) | Yes | No | Partial |
| **Zero Trust (ZTNA)** | Yes | No | Partial | Yes | No |
| **Site-to-Site Mesh** | Yes | Yes (multi-location) | Yes | Yes | No |
| **Built-in OIDC Provider** | Yes | Yes | No | No | No |
| **SAML 2.0 + SCIM 2.0** | Yes | No | No | Yes (paid) | No |
| **Multi-Tenancy (MSP Mode)** | Yes | No | No | No | No |
| **Traffic Analytics** | Yes | No | No | No | No |
| **Compliance Dashboard** | Yes | No | No | No | No |
| **Smart Routing (Proxy)** | Yes | No | No | No | No |
| **Built-in PKI / Key Rotation** | Yes | No | No | Yes | No |
| **Visual Network Map** | Yes | No | No | No | No |
| **Gateway HA (Failover)** | Yes | No | No | Yes (DERP) | No |
| **Horizontal Scaling** | Yes (no etcd) | No | No | Yes (paid) | No |
| **Self-Hosted** | Yes | Yes | Yes | Limited (Headscale) | Yes |
| **Protocol-Level MFA** | Yes | Yes | No | No | No |
| **NAT Traversal** | In progress | No | Yes | Yes (DERP) | No |
| **License** | Apache 2.0 | Apache 2.0 | BSD-3 | Proprietary | Apache 2.0 |

---

## Project Structure

```
outpost/
├── cmd/
│   ├── outpost-core/         # API + UI server
│   ├── outpost-gateway/      # WireGuard gateway agent
│   ├── outpost-proxy/        # DMZ enrollment proxy
│   ├── outpost-client/       # Cross-platform VPN client
│   └── outpostctl/           # CLI management tool
├── internal/
│   ├── core/handler/         # REST API handlers (Chi)
│   ├── auth/                 # OIDC, SAML, LDAP, SCIM, MFA, RBAC
│   ├── gateway/              # WG interface management, firewall
│   ├── wireguard/            # WG abstraction: kernel + userspace
│   ├── s2s/                  # Site-to-site mesh engine
│   ├── ztna/                 # Zero Trust verification engine
│   ├── analytics/            # Traffic flow analytics
│   ├── tenant/               # Multi-tenant isolation
│   ├── compliance/           # SOC2 / ISO27001 / GDPR checks
│   ├── pki/                  # Certificate and key lifecycle
│   ├── client/               # Client VPN library
│   ├── observability/        # Prometheus, audit, SIEM
│   ├── mail/                 # Email with i18n templates
│   └── db/                   # pgx pool, sqlc queries
├── pkg/
│   ├── pb/                   # Generated protobuf Go code
│   └── version/              # Build version injection
├── web-ui/                   # React 19 frontend (embedded via go:embed)
├── proto/                    # Protobuf definitions (Buf)
├── migrations/               # PostgreSQL migrations (golang-migrate)
├── tests/e2e/                # API E2E tests
├── docs/                     # Project documentation
└── deploy/
    ├── docker/               # Dockerfiles + docker-compose.yml
    └── helm/                 # Helm charts for Kubernetes
```

---

## Development

### Requirements

- Go 1.24+
- Node.js 22+ and pnpm
- Docker and Docker Compose
- PostgreSQL 17 (or use Docker Compose)

### Building from Source

```bash
# Backend
go build ./...

# Frontend
cd web-ui && pnpm install && pnpm build

# Run tests
go test ./... -race -count=1

# E2E tests (requires PostgreSQL)
TEST_DATABASE_URL="postgres://outpost:outpost@localhost:5432/outpost_test?sslmode=disable" \
  go test -v ./tests/e2e/

# Full stack via Docker
docker compose -f deploy/docker/docker-compose.yml up -d --build
```

> See [CONTRIBUTING.md](CONTRIBUTING.md) for the full development environment setup guide.

---

## Documentation

| Document | Description |
|---|---|
| [Getting Started](docs/getting-started.md) | Initial setup and step-by-step guide |
| [Architecture](docs/architecture.md) | System design and component interaction |
| [API Reference](docs/API.md) | Complete REST API documentation |
| [Deployment](docs/deployment.md) | Production deployment guide (Docker, Helm, bare metal) |
| [Configuration](docs/configuration.md) | Environment variables and settings reference |
| [Features](docs/features.md) | Detailed feature documentation |
| [Mesh Networking](docs/mesh-networking.md) | Site-to-site and mesh topology guide |

---

## Contributing

We welcome all contributions -- bug reports, feature requests, documentation improvements, and code.

Please read our [Contributing Guide](CONTRIBUTING.md) before submitting a pull request.

---

## Security

If you discover a vulnerability, please report it responsibly. See our [Security Policy](SECURITY.md) for details.

---

## License

[Apache License 2.0](LICENSE)

All features are and will always be fully open-source. No enterprise tier. No feature gating. No catch.

Monetization is through Outpost Cloud (managed SaaS), support contracts, and professional services -- not by restricting open-source code.

---

<p align="center">
  <sub>Built by <a href="https://github.com/romashqua">Romashqua Labs</a></sub>
</p>
