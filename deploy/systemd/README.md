# Outpost VPN — systemd Service Units

systemd unit files for running Outpost VPN components as system services on Linux.

## Components

| Unit | Description |
|------|-------------|
| `outpost-core.service` | API server, OIDC provider, admin panel, gRPC hub |
| `outpost-gateway.service` | WireGuard gateway agent |
| `outpost-proxy.service` | Public-facing enrollment/auth proxy (DMZ-safe) |
| `outpost-client.service` | VPN client with auto-connect |
| `outpost-s2s@.service` | Template unit for site-to-site tunnels |

## Installation

### 1. Create the outpost system user

```bash
sudo useradd --system --shell /usr/sbin/nologin --home-dir /var/lib/outpost --create-home outpost
```

### 2. Create directories

```bash
sudo mkdir -p /etc/outpost
sudo mkdir -p /var/lib/outpost
sudo chown outpost:outpost /var/lib/outpost
```

### 3. Install binaries

Copy the compiled binaries to `/usr/local/bin/`:

```bash
sudo install -m 0755 outpost-core /usr/local/bin/
sudo install -m 0755 outpost-gateway /usr/local/bin/
sudo install -m 0755 outpost-proxy /usr/local/bin/
sudo install -m 0755 outpost-client /usr/local/bin/
```

### 4. Configure environment

Create `/etc/outpost/outpost.env` with your configuration (see example below), then lock down permissions:

```bash
sudo chmod 0640 /etc/outpost/outpost.env
sudo chown root:outpost /etc/outpost/outpost.env
```

### 5. Install unit files

```bash
sudo cp deploy/systemd/*.service /etc/systemd/system/
sudo systemctl daemon-reload
```

### 6. Enable and start services

On the **core server**:

```bash
sudo systemctl enable --now outpost-core.service
```

On a **gateway node**:

```bash
sudo systemctl enable --now outpost-gateway.service
```

On a **proxy node** (DMZ):

```bash
sudo systemctl enable --now outpost-proxy.service
```

On a **client machine**:

```bash
sudo systemctl enable --now outpost-client.service
```

## Site-to-Site Template Unit

The `outpost-s2s@.service` is a systemd template unit. The instance name (`%i`) is the tunnel ID.

Start a tunnel with ID `tunnel-us-eu`:

```bash
sudo systemctl start outpost-s2s@tunnel-us-eu.service
```

Enable it to start on boot:

```bash
sudo systemctl enable outpost-s2s@tunnel-us-eu.service
```

Run multiple tunnels simultaneously:

```bash
sudo systemctl enable --now outpost-s2s@tunnel-us-eu.service
sudo systemctl enable --now outpost-s2s@tunnel-us-asia.service
```

Check status of a specific tunnel:

```bash
sudo systemctl status outpost-s2s@tunnel-us-eu.service
```

View logs for a specific tunnel:

```bash
journalctl -u outpost-s2s@tunnel-us-eu.service -f
```

## Viewing Logs

All components log to the systemd journal:

```bash
# Follow core server logs
journalctl -u outpost-core.service -f

# Show last 100 lines from the gateway
journalctl -u outpost-gateway.service -n 100

# Show errors only across all outpost services
journalctl -u 'outpost-*' -p err
```

## Example `/etc/outpost/outpost.env`

```bash
# Database
DATABASE_URL=postgres://outpost:secretpassword@localhost:5432/outpost?sslmode=require

# Redis
REDIS_URL=redis://localhost:6379/0

# Core server
OUTPOST_HTTP_ADDR=:8080
OUTPOST_GRPC_ADDR=:50051
OUTPOST_DOMAIN=vpn.example.com
OUTPOST_SECRET_KEY=change-me-to-a-random-64-char-string

# Gateway (on gateway nodes)
OUTPOST_CORE_GRPC_ADDR=core.internal:50051
OUTPOST_GATEWAY_TOKEN=gateway-enrollment-token
OUTPOST_WG_INTERFACE=wg0
OUTPOST_WG_PORT=51820
OUTPOST_WG_ENDPOINT=gw.example.com:51820

# Proxy (on proxy nodes)
OUTPOST_CORE_URL=http://core.internal:8080
OUTPOST_PROXY_ADDR=:8081

# Client (on client machines)
OUTPOST_SERVER_URL=https://vpn.example.com

# SMTP (optional, for email notifications)
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=outpost@example.com
SMTP_PASSWORD=smtp-password
SMTP_FROM=noreply@example.com

# Logging
LOG_LEVEL=info
LOG_FORMAT=json
```

## Security Notes

All unit files include systemd security hardening:

- **ProtectSystem=strict** — mounts `/usr`, `/boot`, `/efi` read-only
- **PrivateTmp=true** — private `/tmp` and `/var/tmp`
- **NoNewPrivileges=true** — prevents privilege escalation
- **ProtectHome=true** — hides `/home`, `/root`, `/run/user`
- **ReadWritePaths** — only specific directories are writable
- **AmbientCapabilities** — gateway/client/s2s units get `CAP_NET_ADMIN` and `CAP_NET_RAW` for WireGuard interface management (no need to run as root)
