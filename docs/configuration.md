# Configuration Reference

Outpost VPN is configured entirely via environment variables prefixed with `OUTPOST_`. Missing variables use sensible defaults. Configuration is loaded at startup in `internal/config/config.go`.

## outpost-core

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_HTTP_ADDR` | `:8080` | HTTP server address (API + admin panel) |
| `OUTPOST_GRPC_ADDR` | `:9090` | gRPC server address (gateway communication) |
| `OUTPOST_TLS_CERT` | _(empty)_ | Path to TLS certificate file (enables HTTPS) |
| `OUTPOST_TLS_KEY` | _(empty)_ | Path to TLS private key file |

Note: in Docker Compose, core uses `OUTPOST_SERVER_HTTP_ADDR` and `OUTPOST_SERVER_GRPC_ADDR` (with `SERVER_` prefix) for the same purpose. The config loader reads `OUTPOST_HTTP_ADDR`.

### Database (PostgreSQL)

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_DB_HOST` | `localhost` | PostgreSQL host |
| `OUTPOST_DB_PORT` | `5432` | PostgreSQL port |
| `OUTPOST_DB_NAME` | `outpost` | Database name |
| `OUTPOST_DB_USER` | `outpost` | Database user |
| `OUTPOST_DB_PASSWORD` | _(empty)_ | Database password |
| `OUTPOST_DB_SSLMODE` | `disable` | PostgreSQL SSL mode (`disable`, `require`, `verify-ca`, `verify-full`) |
| `OUTPOST_DB_MAX_CONNS` | `20` | Maximum database connections |
| `OUTPOST_DB_MIN_CONNS` | `2` | Minimum idle connections |

### Redis

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_REDIS_ADDR` | `localhost:6379` | Redis address (host:port) |
| `OUTPOST_REDIS_PASSWORD` | _(empty)_ | Redis password |
| `OUTPOST_REDIS_DB` | `0` | Redis database number |

### Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_JWT_SECRET` | _(auto-generated)_ | JWT signing secret. **Critical: set in production.** If not set, a random 32-byte hex string is generated on each startup, and tokens will not survive restarts. |
| `OUTPOST_TOKEN_TTL` | `15m` | JWT token lifetime (Go duration format: `15m`, `1h`, `24h`) |
| `OUTPOST_SESSION_TTL` | `24h` | Session lifetime |

### OIDC Provider

Outpost includes a built-in OpenID Connect identity provider.

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_OIDC_ISSUER` | `http://localhost:8080` | OIDC issuer URL. Must match the public URL of your Outpost instance. |
| `OUTPOST_OIDC_SIGNING_KEY` | _(empty)_ | Path to RSA private key PEM file for OIDC token signing. If empty, a key is generated at startup. |

### SAML 2.0

Outpost can act as a SAML 2.0 Service Provider, delegating authentication to an external Identity Provider (Okta, Azure AD, OneLogin, etc.).

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_SAML_ENABLED` | `false` | Enable SAML 2.0 SP |
| `OUTPOST_SAML_ENTITY_ID` | _(empty)_ | SP Entity ID (unique identifier for this SP) |
| `OUTPOST_SAML_ACS_URL` | _(empty)_ | Assertion Consumer Service URL (e.g., `https://vpn.example.com/saml/acs`) |
| `OUTPOST_SAML_IDP_METADATA_URL` | _(empty)_ | IDP metadata URL (fetched at startup to configure the SP) |
| `OUTPOST_SAML_CERT_FILE` | _(empty)_ | Path to SP X.509 certificate |
| `OUTPOST_SAML_KEY_FILE` | _(empty)_ | Path to SP private key |

### LDAP / Active Directory

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_LDAP_ENABLED` | `false` | Enable LDAP/AD sync |
| `OUTPOST_LDAP_URL` | _(empty)_ | LDAP server URL (e.g., `ldap://ad.example.com:389` or `ldaps://ad.example.com:636`) |
| `OUTPOST_LDAP_BIND_DN` | _(empty)_ | Bind DN for LDAP queries (e.g., `cn=outpost,ou=service,dc=example,dc=com`) |
| `OUTPOST_LDAP_BIND_PASSWORD` | _(empty)_ | Bind password |
| `OUTPOST_LDAP_BASE_DN` | _(empty)_ | Base DN for user/group searches (e.g., `dc=example,dc=com`) |
| `OUTPOST_LDAP_USER_FILTER` | `(objectClass=person)` | LDAP filter for user searches |
| `OUTPOST_LDAP_GROUP_FILTER` | `(objectClass=group)` | LDAP filter for group searches |
| `OUTPOST_LDAP_TLS` | `false` | Enable STARTTLS |
| `OUTPOST_LDAP_SKIP_VERIFY` | `false` | Skip TLS certificate verification (not recommended for production) |
| `OUTPOST_LDAP_SYNC_INTERVAL` | `15m` | User/group sync interval from LDAP |

### WireGuard

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_WG_INTERFACE` | `wg0` | WireGuard interface name |
| `OUTPOST_WG_LISTEN_PORT` | `51820` | WireGuard UDP port |
| `OUTPOST_WG_MTU` | `1420` | WireGuard interface MTU |

### SMTP (Email Notifications)

Email is optional. If `OUTPOST_SMTP_HOST` is empty, email features are disabled.

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_SMTP_HOST` | _(empty)_ | SMTP server host |
| `OUTPOST_SMTP_PORT` | `587` | SMTP server port |
| `OUTPOST_SMTP_USERNAME` | _(empty)_ | SMTP authentication username |
| `OUTPOST_SMTP_PASSWORD` | _(empty)_ | SMTP authentication password |
| `OUTPOST_SMTP_FROM` | _(empty)_ | Sender email address (e.g., `noreply@example.com`) |
| `OUTPOST_SMTP_FROM_NAME` | `Outpost VPN` | Sender display name |
| `OUTPOST_SMTP_TLS` | `false` | Enable STARTTLS |

#### Example: Gmail SMTP

```bash
OUTPOST_SMTP_HOST=smtp.gmail.com
OUTPOST_SMTP_PORT=587
OUTPOST_SMTP_USERNAME=yourapp@gmail.com
OUTPOST_SMTP_PASSWORD=app-specific-password
OUTPOST_SMTP_FROM=yourapp@gmail.com
OUTPOST_SMTP_TLS=true
```

#### Example: AWS SES

```bash
OUTPOST_SMTP_HOST=email-smtp.us-east-1.amazonaws.com
OUTPOST_SMTP_PORT=587
OUTPOST_SMTP_USERNAME=AKIA...
OUTPOST_SMTP_PASSWORD=...
OUTPOST_SMTP_FROM=noreply@example.com
OUTPOST_SMTP_TLS=true
```

### NAT Traversal

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_NAT_ENABLED` | `false` | Enable NAT traversal (STUN/TURN) |
| `OUTPOST_STUN_PORT` | `3478` | STUN server port |
| `OUTPOST_TURN_PORT` | `3479` | TURN server port |
| `OUTPOST_TURN_REALM` | `outpost` | TURN server realm |
| `OUTPOST_EXTERNAL_IP` | _(empty)_ | External IP for NAT traversal (auto-detected if empty) |

### Logging

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `OUTPOST_LOG_FORMAT` | `json` | Log format: `json` (production), `text` (development) |

---

## outpost-gateway

The gateway binary uses the same configuration structure but primarily uses the following variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_GATEWAY_TOKEN` | _(empty)_ | Authentication token for connecting to core. Generated when creating a gateway in the admin panel. |
| `OUTPOST_GATEWAY_CORE_ADDR` | `localhost:9090` | gRPC address of outpost-core (e.g., `core.example.com:50051`) |
| `OUTPOST_LOG_LEVEL` | `info` | Log level |
| `OUTPOST_LOG_FORMAT` | `json` | Log format |

The gateway also reads `OUTPOST_WG_INTERFACE`, `OUTPOST_WG_LISTEN_PORT`, and `OUTPOST_WG_MTU` for WireGuard configuration.

### System Requirements

The gateway requires:
- `NET_ADMIN` capability (WireGuard interface management)
- `SYS_MODULE` capability (loading the WireGuard kernel module)
- Access to `/dev/net/tun` device
- sysctl `net.ipv4.ip_forward=1` (IP forwarding)
- sysctl `net.ipv4.conf.all.src_valid_mark=1` (source validation)

---

## outpost-proxy

The proxy is a lightweight stateless component for DMZ deployment.

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_PROXY_LISTEN_ADDR` | `:8081` | HTTP server address |
| `OUTPOST_PROXY_CORE_ADDR` | `localhost:9090` | gRPC address of outpost-core |
| `OUTPOST_LOG_LEVEL` | `info` | Log level |
| `OUTPOST_LOG_FORMAT` | `json` | Log format |

The proxy has no database or Redis connections. It forwards enrollment and authentication requests to core via gRPC.

---

## outpost-client

VPN client. Configured on first run via the `login` and `enroll` commands.

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTPOST_CLIENT_SERVER` | _(empty)_ | Outpost server URL (e.g., `https://vpn.example.com`) |
| `OUTPOST_LOG_LEVEL` | `info` | Log level |
| `OUTPOST_LOG_FORMAT` | `text` | Log format |

The client stores configuration (token, keys, tunnel parameters) locally after a successful `login` and `enroll`.

---

## Duration Format

Variables accepting durations use the Go `time.Duration` format:

| Format | Example | Meaning |
|--------|---------|---------|
| `s` | `30s` | 30 seconds |
| `m` | `15m` | 15 minutes |
| `h` | `24h` | 24 hours |
| Combined | `1h30m` | 1 hour 30 minutes |

---

## Configuration Priority

1. Environment variables (highest priority)
2. Built-in defaults (lowest priority)

There is no configuration file. All configuration is via environment variables. This follows the twelve-factor app methodology and works well with Docker, Kubernetes, and systemd.

---

## Security Recommendations

1. **Always set `OUTPOST_JWT_SECRET`** in production. If not set, a random secret is generated, and all JWT tokens become invalid on restart.

2. **Use `OUTPOST_DB_SSLMODE=require`** (or `verify-full`) when connecting to a remote database.

3. **Never expose PostgreSQL or Redis ports** to the internet. Only outpost-core should have access to them.

4. **Rotate gateway tokens periodically.** Generate a new token in the admin panel and update the gateway's environment variable.

5. **Enable SMTP** for email notifications about password resets and device enrollments.

6. **Set `OUTPOST_LOG_LEVEL=info`** in production. Use `debug` only for troubleshooting — it logs sensitive request details.
