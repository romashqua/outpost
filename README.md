# Outpost VPN

Open-source WireGuard VPN & Identity Access Management platform with first-class site-to-site tunnel support.

## Features

- **WireGuard VPN** with kernel and userspace support
- **Site-to-Site tunnels** — full mesh and hub-and-spoke topologies
- **Built-in OIDC provider** — "Log in with Outpost"
- **MFA/2FA** — TOTP, WebAuthn/FIDO2, email tokens, backup codes
- **WireGuard protocol-level MFA** — peer removed on session expiry
- **LDAP/Active Directory sync** (bidirectional)
- **SAML 2.0** and **SCIM 2.0** support
- **External SSO** — Google, Azure AD, Okta, Keycloak
- **RBAC** with fine-grained ACLs per network
- **Gateway HA** — multiple gateways per network with failover
- **Real-time config sync** via gRPC streaming
- **Prometheus metrics**, audit log, SIEM integration
- **Docker** and **Kubernetes** (Helm) ready
- **100% open-source** under Apache 2.0

## Quick Start

```bash
# Clone the repository
git clone https://github.com/romashqua-labs/outpost.git
cd outpost

# Start the full stack
docker compose -f deploy/docker/docker-compose.yml up -d

# The API is available at http://localhost:8080
# WireGuard gateway listens on port 51820/udp
```

## Development

### Prerequisites

- Go 1.24+
- PostgreSQL 17
- Redis 7
- Node.js 22+ and pnpm (for frontend)
- [Buf](https://buf.build/) (for protobuf)

### Build

```bash
make build          # Build all binaries
make test           # Run tests
make lint           # Run linter
make proto          # Generate protobuf code
make docker-up      # Start full Docker stack
```

### Project Structure

```
outpost/
├── cmd/                  # Entry points
│   ├── outpost-core/     # API server
│   ├── outpost-gateway/  # WireGuard gateway agent
│   ├── outpost-proxy/    # Enrollment proxy
│   └── outpostctl/       # CLI tool
├── internal/             # Private packages
│   ├── core/             # HTTP + gRPC server
│   ├── gateway/          # Gateway agent logic
│   ├── auth/             # Authentication & authorization
│   ├── db/               # Database layer
│   ├── wireguard/        # WireGuard abstraction
│   ├── s2s/              # Site-to-site engine
│   └── observability/    # Metrics, audit, logging
├── proto/                # Protobuf definitions
├── migrations/           # SQL migrations
├── web-ui/               # React frontend
└── deploy/               # Docker & Helm
```

## License

Apache License 2.0 — see [LICENSE](LICENSE).
