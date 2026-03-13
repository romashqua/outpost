# Outpost VPN

Open-source WireGuard VPN & Zero-Trust Network Access platform.
All features are open-source under Apache 2.0 — no enterprise paywalls.
Monetization: managed cloud (SaaS), support contracts, professional services.

## Stack
- **Backend:** Go 1.24+, Chi (HTTP), gRPC (inter-service)
- **Frontend:** TypeScript, React 19, Vite, Tailwind CSS 4, Zustand, TanStack Query, Recharts
- **UI Theme:** Dark cyberpunk/hacker aesthetic (#0a0a0f bg, #00ff88 accent, JetBrains Mono)
- **Database:** PostgreSQL 17 (pgx/v5, sqlc)
- **Cache:** Redis 7 (sessions, pub/sub, rate limiting)
- **Auth:** Built-in OIDC provider, LDAP/AD sync, SAML 2.0, SCIM 2.0, TOTP, WebAuthn/FIDO2
- **VPN:** WireGuard (wireguard-go + kernel via netlink)
- **Protobuf:** Buf (gRPC + proto definitions)
- **Containerization:** Docker, docker-compose, Helm
- **CI/CD:** GitHub Actions
- **Observability:** Prometheus, slog, audit log, SIEM webhooks

## Architecture
Monorepo with single Go module. Components:

### Binaries (cmd/)
- `cmd/outpost-core` — API server, OIDC provider, admin panel, gRPC hub
- `cmd/outpost-gateway` — WireGuard gateway agent (clients + S2S tunnels)
- `cmd/outpost-proxy` — public-facing enrollment/auth proxy (DMZ-safe)
- `cmd/outpost-client` — VPN client with MFA/2FA, posture reporting, tunnel management
- `cmd/outpostctl` — CLI management tool

### Core packages (internal/)
- `internal/core` — HTTP handlers, gRPC impls, schedulers
- `internal/gateway` — WG interface management, firewall, S2S on gateway side
- `internal/auth` — OIDC, SAML, LDAP, SCIM, MFA (TOTP/WebAuthn/email), RBAC
- `internal/db` — pgx pool, sqlc queries
- `internal/wireguard` — kernel + userspace WG abstraction, config rendering
- `internal/s2s` — site-to-site: mesh, hub-spoke, route exchange, health tracking
- `internal/observability` — Prometheus metrics, audit log, SIEM integration
- `internal/client` — VPN client library: auth flow, tunnel, posture checks (cross-platform)
- `internal/mail` — email notifications with i18n templates

### Killer feature packages (internal/)
- `internal/ztna` — Zero-Trust Network Access: device posture checks, continuous verification
- `internal/analytics` — Traffic analytics: flow records, bandwidth, top users, heatmaps
- `internal/tenant` — Multi-tenant platform: MSP/reseller mode, org isolation
- `internal/compliance` — Compliance dashboard: SOC2, ISO27001, GDPR readiness checks
- `internal/pki` — Built-in PKI: automatic key rotation, certificate lifecycle

### Shared
- `pkg/pb` — generated protobuf Go code
- `pkg/version` — build version injection
- `proto/` — protobuf definitions (Buf)
- `web-ui/` — React frontend (embedded via go:embed)

## Key commands
- Build all: `make build` or `go build ./...`
- Test: `make test` or `go test ./...`
- Lint: `make lint` (golangci-lint)
- Format: `make fmt` or `go fmt ./...`
- Vet: `go vet ./...`
- Proto generate: `make proto` (requires buf)
- SQL codegen: `make sqlc` (requires sqlc)
- Dev DB: `docker compose -f deploy/docker/docker-compose.yml up -d postgres`
- Migrations: `make migrate-up` (DATABASE_URL required)
- Full stack: `make docker-up`
- Frontend dev: `cd web-ui && pnpm dev`
- Frontend build: `cd web-ui && pnpm build`
- Frontend lint: `cd web-ui && pnpm lint`

## Conventions
- All code and comments in English
- UI supports Russian and English (i18n via react-i18next)
- camelCase for TypeScript, snake_case for SQL, Go conventions for Go
- All API endpoints documented via swaggo (OpenAPI)
- Database migrations in `migrations/` directory (golang-migrate, plain SQL)
- Proto files in `proto/` directory (managed with Buf)
- Never commit secrets or .env files
- All features fully open-source — no gated enterprise modules
- Use sqlc for type-safe DB queries, no ORM
- gRPC bidirectional streaming for core <-> gateway communication
- Frontend: functional components, hooks, Zustand for global state
- Frontend: TanStack Query for server state, fetch-based API client
- Frontend: Tailwind CSS for styling, no CSS-in-JS
- Frontend: lucide-react for icons, recharts for charts

## Killer Features (differentiators from defguard/NetBird)
1. **ZTNA** — device posture checks (disk encryption, antivirus, OS version, screen lock)
2. **S2S Tunnels** — first-class mesh + hub-spoke with BGP-lite route exchange
3. **Traffic Analytics** — real-time flow visualization, bandwidth heatmaps, top talkers
4. **Multi-Tenant** — MSP mode: one instance, many organizations with isolation
5. **Compliance Dashboard** — automated SOC2/ISO27001/GDPR readiness scoring
6. **Built-in PKI** — automatic WireGuard key rotation with zero downtime
7. **WireGuard Protocol-Level MFA** — peer removed from gateway on session expiry
8. **Visual Network Map** — interactive topology visualization of entire VPN mesh

## Monetization Strategy (all code stays Apache 2.0)
- **Outpost Cloud** — managed SaaS offering (primary revenue)
- **Support contracts** — SLA-backed enterprise support
- **Professional services** — deployment, migration, custom integration
- **Training & certification** — Outpost admin certification program
