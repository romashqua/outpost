<p align="center">
  <a href="README.md">English</a> | <a href="README.ru.md">Русский</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go&logoColor=white" />
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=black" />
  <img src="https://img.shields.io/badge/WireGuard-88171A?style=flat-square&logo=wireguard&logoColor=white" />
  <img src="https://img.shields.io/badge/PostgreSQL-17-4169E1?style=flat-square&logo=postgresql&logoColor=white" />
  <img src="https://img.shields.io/badge/License-Apache%202.0-green?style=flat-square" />
  <img src="https://img.shields.io/github/stars/romashqua/outpost?style=flat-square" />
</p>

<h1 align="center"><code>&gt;_</code> OUTPOST VPN</h1>

<p align="center">
  <strong>Open-Source WireGuard VPN & Zero Trust Network Access Platform</strong><br/>
  Enterprise-grade security without the enterprise price tag.
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="#features">Features</a> &middot;
  <a href="#architecture">Architecture</a> &middot;
  <a href="#api">API</a> &middot;
  <a href="#development">Development</a>
</p>

---

## Why Outpost?

Every other VPN solution either locks critical features behind an enterprise paywall or doesn't have them at all. Outpost gives you **everything** — ZTNA device posture, compliance dashboards, multi-tenant MSP mode, traffic analytics, built-in OIDC, PKI — fully open-source from day one.

**Built for:**
- Teams that need corporate VPN without vendor lock-in
- MSPs/MSSPs delivering VPN-as-a-service
- DevOps building Zero Trust networks
- Anyone tired of paying for enterprise features

## Features

### VPN & Networking
- **WireGuard tunnels** — kernel & userspace, automatic key management
- **Site-to-Site** — full mesh & hub-spoke with BGP-lite route exchange
- **NAT Traversal** — STUN/TURN relay servers for restrictive networks
- **Smart Routes** — selective domain/CIDR routing through proxy servers (SOCKS5, HTTP, Shadowsocks)
- **Real-time sync** — gRPC bidirectional streaming between core and gateways

### Zero Trust (ZTNA)
- **Device posture checks** — disk encryption, antivirus, firewall, OS version, screen lock
- **Continuous verification** — periodic re-evaluation of device trust score
- **Configurable weights & thresholds** — tune what matters for your security model
- **Protocol-level MFA** — WireGuard peer removed from gateway on session expiry

### Identity & Access
- **Built-in OIDC provider** — "Sign in with Outpost" (Authorization Code + PKCE, RS256)
- **MFA/2FA** — TOTP, WebAuthn/FIDO2, email tokens, backup codes
- **External SSO** — Google, Azure AD, Okta, Keycloak
- **LDAP/Active Directory** sync
- **SAML 2.0** Service Provider
- **SCIM 2.0** auto-provisioning (Okta, Azure AD)
- **RBAC** with granular network ACLs

### Analytics & Compliance
- **Traffic analytics** — bandwidth charts, top users, connection heatmaps
- **Compliance dashboard** — automated SOC2, ISO 27001, GDPR readiness scoring
- **Audit log** — every admin action recorded with full context
- **SIEM integration** — webhook + syslog export with HMAC signing

### Platform
- **Multi-tenant** — MSP/reseller mode with org isolation
- **Built-in PKI** — automatic WireGuard key rotation with zero downtime
- **Interactive network map** — SVG topology visualization
- **Email notifications** — SMTP for MFA, enrollment, alerts
- **Prometheus metrics** — full observability out of the box
- **Docker & Kubernetes** — compose + Helm ready

## Quick Start

```bash
git clone https://github.com/romashqua/outpost.git
cd outpost
docker compose -f deploy/docker/docker-compose.yml up -d
```

Open **http://localhost:8080** — login: `admin` / `admin`

That's it. Migrations run automatically, all services start in order.

## Architecture

```
                    ┌─────────────────────┐
                    │   outpost-core      │
                    │   :8080 HTTP/API    │
                    │   :50051 gRPC       │
                    │   React UI (embed)  │
                    └────────┬────────────┘
                             │ gRPC streaming
              ┌──────────────┼──────────────┐
              │              │              │
    ┌─────────▼──┐  ┌───────▼────┐  ┌──────▼──────┐
    │  gateway   │  │  gateway   │  │   proxy     │
    │  :51820/udp│  │  :51821/udp│  │   :8081     │
    │  WireGuard │  │  WireGuard │  │   DMZ-safe  │
    └────────────┘  └────────────┘  └─────────────┘
```

| Component | Role | Ports |
|---|---|---|
| `outpost-core` | API, OIDC provider, admin panel, gRPC control plane | 8080, 50051 |
| `outpost-gateway` | WireGuard data plane (client VPN + S2S tunnels) | 51820/udp |
| `outpost-proxy` | Public enrollment/auth proxy for DMZ deployment | 8081 |
| `outpost-client` | VPN client with MFA, posture reporting | — |
| `outpostctl` | CLI management tool | — |

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.25, Chi (HTTP), gRPC, pgx + sqlc |
| Frontend | React 19, TypeScript, Vite, Tailwind CSS 4, Zustand, TanStack Query |
| Database | PostgreSQL 17 |
| Cache | Redis 7 (sessions, pub/sub, rate limiting) |
| VPN | WireGuard (kernel + userspace via wireguard-go) |
| Auth | Built-in OIDC, SAML 2.0, LDAP, SCIM 2.0, WebAuthn, TOTP |
| Observability | Prometheus, slog, audit log, SIEM webhooks |
| Deploy | Docker, docker-compose, Helm |

## API

All endpoints under `/api/v1/` with JWT authentication:

```
Auth:        POST /auth/login, /auth/mfa/verify, /auth/refresh
Users:       GET/POST /users, GET/PUT/DELETE /users/{id}
Groups:      GET/POST /groups, members, ACLs
Networks:    GET/POST /networks, GET/PUT/DELETE /networks/{id}
Devices:     GET/POST /devices, /devices/enroll, approve, revoke, config
Gateways:    GET/POST /gateways, GET/DELETE /gateways/{id}
S2S:         GET/POST/DELETE /s2s-tunnels, members, routes
ZTNA:        GET/PUT /ztna/trust-config, GET/POST/DELETE /ztna/policies, dns-rules
Smart Routes: GET/POST/PUT/DELETE /smart-routes, entries, proxy-servers
Analytics:   GET /analytics/summary, bandwidth, top-users, heatmap
Compliance:  GET /compliance/report, soc2, iso27001, gdpr
Tenants:     GET/POST /tenants, GET/PUT/DELETE /tenants/{id}, stats
Settings:    GET/PUT /settings, POST /settings/smtp/test
Audit:       GET /audit, /audit/stats, /audit/export
```

## Development

```bash
# Build
go build ./...

# Test
go test ./... -race -count=1

# E2E tests (requires running PostgreSQL)
TEST_DATABASE_URL="postgres://..." go test -v ./tests/e2e/

# Frontend
cd web-ui && pnpm install && pnpm dev

# Full stack
docker compose -f deploy/docker/docker-compose.yml up -d --build
```

## Project Structure

```
outpost/
├── cmd/
│   ├── outpost-core/         # API + UI server
│   ├── outpost-gateway/      # WireGuard gateway agent
│   ├── outpost-proxy/        # DMZ enrollment proxy
│   ├── outpost-client/       # VPN client with MFA
│   └── outpostctl/           # CLI tool
├── internal/
│   ├── core/handler/         # REST API handlers
│   ├── auth/                 # OIDC, SAML, LDAP, SCIM, MFA, RBAC
│   ├── gateway/              # WG interface management
│   ├── wireguard/            # Kernel + userspace abstraction
│   ├── s2s/                  # Site-to-site mesh engine
│   ├── ztna/                 # Zero Trust posture checks
│   ├── analytics/            # Traffic flow records
│   ├── tenant/               # Multi-tenant isolation
│   ├── compliance/           # SOC2/ISO27001/GDPR checks
│   ├── pki/                  # Certificate lifecycle
│   └── db/                   # pgx pool, sqlc queries
├── web-ui/                   # React frontend (embedded via go:embed)
├── migrations/               # PostgreSQL migrations (golang-migrate)
├── proto/                    # Protobuf definitions (Buf)
├── tests/e2e/                # End-to-end API tests
└── deploy/
    ├── docker/               # Dockerfiles + docker-compose.yml
    └── helm/                 # Helm charts
```

## Comparison

| | Outpost | defguard | NetBird | Tailscale |
|---|:---:|:---:|:---:|:---:|
| Fully open-source | :white_check_mark: | :x: Enterprise paywall | :white_check_mark: | :x: |
| Zero Trust (ZTNA) | :white_check_mark: | :x: | Partial | :white_check_mark: |
| Site-to-Site mesh | :white_check_mark: | :x: | :white_check_mark: | :white_check_mark: |
| Built-in OIDC | :white_check_mark: | :white_check_mark: | :x: | :x: |
| Multi-tenant | :white_check_mark: | :x: | :x: | :x: |
| Traffic analytics | :white_check_mark: | :x: | :x: | :x: |
| Compliance dashboard | :white_check_mark: | :x: | :x: | :x: |
| Smart routing | :white_check_mark: | :x: | :x: | :x: |
| Self-hosted | :white_check_mark: | :white_check_mark: | :white_check_mark: | :x: |

## Contributing

We welcome contributions! Fork the repo, create a branch, and submit a PR.

```bash
git checkout -b feature/my-feature
make test && make lint
```

## License

[Apache License 2.0](LICENSE) — use it however you want, commercially or not.

All features are and will remain fully open-source. No enterprise tier. No feature gating.

---

<p align="center">
  Built by <a href="https://github.com/romashqua">Romashqua Labs</a>
</p>
