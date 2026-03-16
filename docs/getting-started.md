# Getting Started with Outpost VPN

This guide walks you through setting up Outpost VPN from scratch: requirements, installation, first login, and connecting your first device.

## Requirements

| Software | Version | Purpose |
|----------|---------|---------|
| Docker | 24+ | Container runtime |
| Docker Compose | v2+ | Orchestration |
| Git | 2.40+ | Repository cloning |

For building from source (optional):

| Software | Version | Purpose |
|----------|---------|---------|
| Go | 1.24+ | Backend compilation |
| Node.js | 22+ | Frontend build |
| pnpm | 9+ | Frontend package manager |
| PostgreSQL | 17 | Database |
| Redis | 7 | Sessions, pub/sub, rate limiting |
| Buf | latest | Protobuf code generation |
| sqlc | latest | Type-safe SQL code generation |
| golangci-lint | latest | Go linter |

## Quick Start with Docker Compose

### 1. Clone the Repository

```bash
git clone https://github.com/romashqua/outpost.git
cd outpost
```

### 2. Start the Full Stack

```bash
docker compose -f deploy/docker/docker-compose.yml up -d
```

This starts five services:

| Service | Port | Description |
|---------|------|-------------|
| PostgreSQL 17 | 5432 | Database |
| Redis 7 | 6379 | Cache and pub/sub |
| outpost-core | 8080 (HTTP), 50051 (gRPC) | API server, admin panel, OIDC provider |
| outpost-gateway | 51820/udp | WireGuard gateway |
| outpost-proxy | 8081 | Public proxy for enrollment/auth (DMZ-safe) |

Database migrations run automatically on core startup (embedded via `go:embed`).

### 3. Verify Services Are Running

```bash
# Check that all containers are running
docker compose -f deploy/docker/docker-compose.yml ps

# Check the health endpoint
curl http://localhost:8080/healthz
# → {"status":"ok"}

# Check readiness (verifies DB connection)
curl http://localhost:8080/readyz
# → {"status":"ok"}
```

### 4. Open the Admin Panel

Navigate to **http://localhost:8080** in your browser.

## First Login

Log in with the default admin credentials:

- **Username:** `admin`
- **Password:** `admin`

**Change the password immediately.** The default password is created by the `000004_seed_admin_user.up.sql` migration and is not secure for any deployment beyond local development.

To change the password:
1. Log in with `admin` / `admin`
2. Go to **Settings** > **Security**
3. Set a strong password

## Creating Your First Network

A default network (`10.10.0.0/16`, port 51820) is created automatically by the seed migration. To create an additional network:

1. Go to **Networks** in the sidebar
2. Click **Create Network**
3. Fill in the details:
   - **Name:** `office` (or any descriptive name)
   - **Address (CIDR):** `10.20.0.0/24`
   - **DNS servers:** `1.1.1.1`, `8.8.8.8`
   - **Port:** `51820`
   - **Keepalive:** `25` (seconds)
4. Click **Create**

Or via the API:

```bash
# Get a JWT token
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin"}' | jq -r '.token')

# Create a network
curl -X POST http://localhost:8080/api/v1/networks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "office",
    "address": "10.20.0.0/24",
    "dns": ["1.1.1.1", "8.8.8.8"],
    "port": 51820,
    "keepalive": 25
  }'
```

## Registering a Gateway

Gateways are WireGuard endpoints that handle VPN traffic. Docker Compose includes one gateway automatically, but for additional sites:

1. Go to **Gateways** in the sidebar
2. Click **Create Gateway**
3. Fill in:
   - **Name:** `gw-office` (descriptive name)
   - **Network:** Select the network created earlier
   - **Endpoint:** `vpn.example.com:51820` (public IP/hostname + port)
4. Click **Create** — a gateway token will be generated
5. Copy the token and set it as `OUTPOST_GATEWAY_TOKEN` on the gateway machine

Deploying a gateway:

```bash
docker run -d \
  --name outpost-gateway \
  --cap-add=NET_ADMIN \
  --cap-add=SYS_MODULE \
  --device=/dev/net/tun \
  --sysctl net.ipv4.ip_forward=1 \
  -e OUTPOST_GATEWAY_CORE_ADDR=core.example.com:50051 \
  -e OUTPOST_GATEWAY_TOKEN=<paste-token-here> \
  -p 51820:51820/udp \
  outpost/gateway:latest
```

## Enrolling a Device

1. Go to **Devices** in the sidebar
2. Click **Add Device**
3. Enter a device name (e.g., `laptop-alice`)
4. The system generates a WireGuard key pair and assigns an IP
5. Click **Approve** to activate the device
6. Click **Download Config** to get the `.conf` file

The downloaded configuration file looks like this:

```ini
[Interface]
PrivateKey = <generated-private-key>
Address = 10.10.0.2/16
DNS = 1.1.1.1, 8.8.8.8

[Peer]
PublicKey = <gateway-public-key>
Endpoint = vpn.example.com:51820
AllowedIPs = 10.10.0.0/16
PersistentKeepalive = 25
```

> **Split tunneling:** `AllowedIPs` contains your VPN network CIDR (e.g., `10.10.0.0/16`), not `0.0.0.0/0`. This means only VPN traffic goes through the tunnel, while internet traffic goes direct. If you need to route all traffic through the VPN, configure a group ACL with `allowed_ips: ["0.0.0.0/0"]`.

Import this file into any WireGuard client:
- **Linux:** `wg-quick up ./outpost.conf`
- **macOS/Windows:** Import via WireGuard GUI
- **iOS/Android:** Scan QR code or import file
- **Outpost Client:** `outpost-client up` (after `login` and `enroll`)

## How Users Connect to Networks

In Outpost, users are linked to networks **through devices**:

```
User (1) → (N) Devices → (1) Network
```

A single user can have multiple devices across different networks.

**Step-by-step process:**
1. Create a **network** (Networks > Create Network, specify CIDR and DNS)
2. Create a **gateway** for the network (Gateways > Create, link to network)
3. Create a **device** for the user (Devices > Add) — it automatically gets an IP from the active network
4. **Approve** the device (Approve button)
5. **Download the config** (Download button) — the `.conf` file is ready to import into WireGuard

### Connecting via CLI Client

```bash
# Authenticate
outpost-client login --server https://vpn.example.com --username alice

# Enroll device (automatically receives config)
outpost-client enroll --name "my-laptop"

# Bring up the tunnel
outpost-client up

# Bring down the tunnel
outpost-client down
```

### Access Control

- **Groups + ACL:** Create a group > add users > configure ACL with allowed subnets
- **ZTNA policies:** Automatic control based on posture checks (disk encryption, antivirus, OS version)

## Setting Up S2S Tunnels (Site-to-Site)

S2S tunnels connect your offices/data centers via WireGuard. Everything is done in 4 steps.

### Step 1: Create Gateways at Each Site

Each site should have its own gateway linked to its own network:

```bash
# Office A
POST /api/v1/networks
{"name": "office-a", "address": "10.1.0.0/24", "dns": ["1.1.1.1"], "port": 51820}

POST /api/v1/gateways
{"name": "gw-office-a", "network_id": "<office-a-network-id>", "endpoint": "office-a.vpn.company.com:51820"}

# Office B
POST /api/v1/networks
{"name": "office-b", "address": "10.2.0.0/24", "dns": ["1.1.1.1"], "port": 51820}

POST /api/v1/gateways
{"name": "gw-office-b", "network_id": "<office-b-network-id>", "endpoint": "office-b.vpn.company.com:51820"}
```

Or via the UI: **Networks > Create Network**, then **Gateways > Create Gateway**.

### Step 2: Create an S2S Tunnel

```bash
POST /api/v1/s2s-tunnels
{"name": "office-mesh", "topology": "mesh", "description": "Office mesh network"}
```

Or via the UI: **S2S > New Tunnel**, select topology:
- **Mesh** — all gateways directly connected to each other
- **Hub & Spoke** — all gateways connected through a single central (hub) gateway

### Step 3: Add Gateways as Members

```bash
POST /api/v1/s2s-tunnels/<tunnel-id>/members
{"gateway_id": "<gw-office-a-id>", "local_subnets": ["10.1.0.0/24"]}

POST /api/v1/s2s-tunnels/<tunnel-id>/members
{"gateway_id": "<gw-office-b-id>", "local_subnets": ["10.2.0.0/24"]}
```

Or via the UI: in tunnel details, **Members** section > select gateway > **Create**.

### Step 4: Add Routes

```bash
POST /api/v1/s2s-tunnels/<tunnel-id>/routes
{"destination": "10.2.0.0/24", "via_gateway": "<gw-office-b-id>", "metric": 100}

POST /api/v1/s2s-tunnels/<tunnel-id>/routes
{"destination": "10.1.0.0/24", "via_gateway": "<gw-office-a-id>", "metric": 100}
```

Or via the UI: **Routes** section > specify CIDR, select gateway > **Create**.

### Done!

Download the WireGuard config for each gateway:

```bash
GET /api/v1/s2s-tunnels/<tunnel-id>/config/<gateway-id>
```

Or via the UI: click the download icon next to a member in the tunnel details.

The config contains an `[Interface]` with the private key and `[Peer]` sections for each other gateway with the correct `AllowedIPs` and `Endpoint`.

## Building from Source

### Backend

```bash
# Build all binaries
make build

# Or build individual components
make build-core
make build-gateway
make build-proxy
make build-client
make build-ctl

# Binaries are in the bin/ directory
ls bin/
# outpost-core  outpost-gateway  outpost-proxy  outpost-client  outpostctl
```

### Frontend

```bash
cd web-ui
pnpm install
pnpm build    # production build (output is embedded into the core binary)
pnpm dev      # dev server with HMR
```

### Database Setup (for Development)

```bash
# Start only PostgreSQL
docker compose -f deploy/docker/docker-compose.yml up -d postgres

# Run migrations manually (if not using core's auto-migration)
export DATABASE_URL="postgres://outpost:outpost-dev-password@localhost:5432/outpost?sslmode=disable"
make migrate-up
```

### Running Locally

```bash
# Start dependencies
docker compose -f deploy/docker/docker-compose.yml up -d postgres redis

# Start core (migrations run automatically)
export OUTPOST_DB_HOST=localhost
export OUTPOST_DB_PASSWORD=outpost-dev-password
export OUTPOST_REDIS_ADDR=localhost:6379
export OUTPOST_REDIS_PASSWORD=outpost-dev-password
export OUTPOST_JWT_SECRET=$(openssl rand -hex 32)
./bin/outpost-core

# In another terminal, start the frontend dev server
cd web-ui && pnpm dev
```

## Useful Make Targets

| Command | Description |
|---------|-------------|
| `make build` | Build all binaries |
| `make test` | Run all Go tests with race detector |
| `make lint` | Run golangci-lint |
| `make fmt` | Format Go code |
| `make proto` | Generate protobuf code (requires Buf) |
| `make sqlc` | Generate type-safe SQL code |
| `make docker-up` | Start the full stack via Docker Compose |
| `make docker-down` | Stop the Docker Compose stack |
| `make docker-logs` | View logs for all services |
| `make migrate-up` | Apply all pending migrations |
| `make migrate-down` | Roll back one migration |
| `make build-client-all` | Cross-compile the client for Linux, macOS, Windows |

## Next Steps

- [Architecture Overview](architecture.md) — how components work together
- [Configuration Reference](configuration.md) — all environment variables
- [Deployment Guide](deployment.md) — production deployment options
- [API Reference](API.md) — full REST API documentation
- [Features Guide](features.md) — ZTNA, S2S tunnels, Smart Routes, and more
