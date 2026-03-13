# Outpost VPN

Open-source WireGuard VPN & Identity Access Management platform.
All features are open-source under Apache 2.0 — no enterprise paywalls.

## Stack
- **Backend:** Go 1.24+, Chi (HTTP), gRPC (inter-service)
- **Frontend:** TypeScript, React, Vite (embedded via go:embed)
- **Database:** PostgreSQL 17 (pgx/v5, sqlc)
- **Cache:** Redis 7 (sessions, pub/sub)
- **Auth:** Built-in OIDC provider, LDAP/AD sync, SAML 2.0, SCIM, TOTP, WebAuthn/FIDO2
- **VPN:** WireGuard (wireguard-go + kernel via netlink)
- **Protobuf:** Buf
- **Containerization:** Docker, docker-compose, Helm
- **CI/CD:** GitHub Actions

## Architecture
Monorepo with single Go module. Components:
- `cmd/outpost-core` — API server, business logic, OIDC provider, admin panel
- `cmd/outpost-gateway` — WireGuard gateway agent (clients + S2S tunnels)
- `cmd/outpost-proxy` — public-facing enrollment/auth proxy (DMZ-safe)
- `cmd/outpostctl` — CLI management tool
- `internal/core` — HTTP handlers, gRPC server implementations, schedulers
- `internal/gateway` — WG interface management, firewall, S2S
- `internal/auth` — OIDC, SAML, LDAP, SCIM, MFA (TOTP/WebAuthn), RBAC
- `internal/db` — pgx pool, sqlc queries
- `internal/wireguard` — kernel + userspace WG abstraction
- `internal/s2s` — site-to-site: mesh, hub-spoke, route exchange, health
- `internal/observability` — Prometheus metrics, audit log, SIEM
- `internal/mail` — email notifications
- `pkg/pb` — generated protobuf Go code
- `proto/` — protobuf definitions (Buf)
- `web-ui/` — React frontend

## Key commands
- Build: `make build` or `go build ./...`
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

## Conventions
- All code and comments in English
- UI supports Russian and English (i18n via react-i18next)
- camelCase for TypeScript, snake_case for SQL, camelCase for Go (per Go conventions)
- All API endpoints documented via swaggo (OpenAPI)
- Database migrations in `migrations/` directory (golang-migrate, plain SQL)
- Proto files in `proto/` directory (managed with Buf)
- Never commit secrets or .env files
- All features fully open-source — no gated enterprise modules
- Use sqlc for type-safe DB queries, no ORM
- gRPC bidirectional streaming for core <-> gateway communication
