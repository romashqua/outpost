# Outpost VPN — Mesh Networking Architecture

## Overview

Outpost supports two site-to-site (S2S) tunnel topologies:
- **Full Mesh** — each gateway connects directly to every other gateway
- **Hub & Spoke** — all gateways connect through a single central (hub) gateway

Both topologies use WireGuard tunnels with separate interfaces for S2S traffic, isolated from client traffic.

## Architecture

```
                        ┌──────────────────────┐
                        │    outpost-core       │
                        │  (Control Plane)      │
                        │                       │
                        │  ┌─────────────────┐  │
                        │  │ S2S API         │  │
                        │  │ /api/v1/s2s-*   │  │
                        │  └────────┬────────┘  │
                        │           │           │
                        │  ┌────────▼────────┐  │
                        │  │ Topology Engine │  │
                        │  │ Route           │  │
                        │  │ Calculator      │  │
                        │  └────────┬────────┘  │
                        │           │           │
                        │  ┌────────▼────────┐  │
                        │  │ gRPC Gateway    │  │
                        │  │ Service         │  │
                        │  └───┬────┬────┬───┘  │
                        └──────┼────┼────┼──────┘
                          gRPC │    │    │ gRPC
                 ┌─────────────┘    │    └─────────────┐
                 │                  │                   │
          ┌──────▼──────┐   ┌──────▼──────┐   ┌───────▼─────┐
          │ gw-site-a   │   │ gw-site-b   │   │ gw-site-c   │
          │ 10.1.0.0/24 │◄─►│ 10.2.0.0/24 │◄─►│ 10.3.0.0/24 │
          │             │   │             │   │             │
          │ wg0: clients│   │ wg0: clients│   │ wg0: clients│
          │ wg1: s2s    │   │ wg1: s2s    │   │ wg1: s2s    │
          └──────┬──────┘   └──────┬──────┘   └──────┬──────┘
                 │                 │                  │
          ┌──────▼──────┐  ┌──────▼──────┐   ┌──────▼──────┐
          │   Clients   │  │   Clients   │   │   Clients   │
          │ 10.1.0.x    │  │ 10.2.0.x    │   │ 10.3.0.x    │
          └─────────────┘  └─────────────┘   └─────────────┘
```

## Components

### 1. S2S API (outpost-core)

REST API for managing S2S tunnels, members, and routes.

**Endpoints:**
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/s2s-tunnels` | List all tunnels |
| POST | `/api/v1/s2s-tunnels` | Create a tunnel (name, topology, description) |
| GET | `/api/v1/s2s-tunnels/{id}` | Get tunnel details |
| DELETE | `/api/v1/s2s-tunnels/{id}` | Delete a tunnel |

**Database tables:**
- `s2s_tunnels` — tunnel metadata (name, topology, hub_gateway_id, description)
- `s2s_tunnel_members` — which gateways participate, with `local_subnets CIDR[]`
- `s2s_routes` — computed routes (destination CIDR, via_gateway, metric)

### 2. Topology Engine

When a tunnel is created or members change, the topology engine computes the required WireGuard peer configurations:

#### Full Mesh
Each gateway gets a peer entry for every other gateway in the tunnel:
```
N gateways → N×(N-1)/2 WireGuard peer pairs
```

```
  gw-A ◄────► gw-B
    ▲  ╲      ╱  ▲
    │    ╲  ╱    │
    │     ╳      │
    │   ╱  ╲     │
    ▼ ╱      ╲   ▼
  gw-C ◄────► gw-D
```

Each peer's `AllowedIPs` includes the remote gateway's `local_subnets`.

#### Hub & Spoke
All spoke gateways connect only to the hub. Traffic between spoke gateways is routed through the hub:
```
          ┌─────┐
    ┌─────┤ Hub ├─────┐
    │     └──┬──┘     │
    │        │        │
    ▼        ▼        ▼
  Spoke-A  Spoke-B  Spoke-C
```

The hub gateway gets peer entries for all spokes. Each spoke gets only one peer (the hub) with `AllowedIPs = 0.0.0.0/0` or the union of all other spoke subnets.

### 3. Route Exchange

Routes are exchanged via gRPC streaming between core and gateways:

```
Gateway → Core: AdvertiseRoutes(local_subnets)
Core → Gateway: PushRoutes(computed_route_table)
```

**Route computation process:**
1. The gateway registers its `local_subnets` with core
2. Core aggregates all advertised routes from all tunnel members
3. Core computes the route table based on topology:
   - **Mesh**: direct routes between each pair of gateways
   - **Hub-spoke**: routes through the hub for inter-spoke traffic
4. Core sends the computed route table to each gateway via the gRPC stream
5. The gateway applies routes to its S2S WireGuard interface (`wg1`)

### 4. WireGuard Interface Isolation

Each gateway runs two WireGuard interfaces:

| Interface | Purpose | Port | Peers |
|-----------|---------|------|-------|
| `wg0` | Client VPN traffic | 51820/udp | User devices |
| `wg1` | S2S tunnel traffic | 51821/udp | Other gateways |

This isolation provides:
- S2S key compromise does not affect client connections
- Separate firewall rules for each interface
- Independent MTU/keepalive settings
- No routing loops between client and S2S traffic

### 5. Health Monitoring

Gateways continuously monitor S2S tunnel health:

```
Every 10s: ICMP echo to each S2S peer's tunnel IP
             │
             ├── Response < 100ms  → HEALTHY
             ├── Response > 500ms  → DEGRADED
             └── No response (3×)  → DOWN
```

When a gateway transitions to DOWN:
1. Core marks the gateway as unhealthy in the route table
2. For mesh: routes are recalculated excluding the failed gateway
3. For hub-spoke: if the hub goes down, traffic switches to a backup hub (if configured)
4. Core sends updated routes to all remaining gateways

## Allowed Domains

S2S tunnels support domain-based filtering. This allows restricting which resources at a remote site can be accessed by clients through the S2S tunnel.

Configuration is done at the tunnel member level and works in conjunction with DNS resolution on the gateway.

## Configuration Generation

A ready-to-use WireGuard config can be generated for each tunnel member gateway:

```bash
GET /api/v1/s2s-tunnels/{tunnelId}/config/{gatewayId}
```

The response contains a full WireGuard config for the `wg1` interface:

```ini
[Interface]
PrivateKey = <gateway-private-key>
Address = 10.255.0.1/24
ListenPort = 51821

[Peer]
# gw-site-b
PublicKey = <gw-site-b-public-key>
Endpoint = site-b.vpn.company.com:51821
AllowedIPs = 10.2.0.0/24
PersistentKeepalive = 25

[Peer]
# gw-site-c
PublicKey = <gw-site-c-public-key>
Endpoint = site-c.vpn.company.com:51821
AllowedIPs = 10.3.0.0/24
PersistentKeepalive = 25
```

For mesh topology, each peer's `AllowedIPs` contains the remote gateway's `local_subnets`.
For hub-spoke, the spoke peer's `AllowedIPs` on the hub contains its subnets, and the hub peer's `AllowedIPs` on each spoke contains the union of all members' subnets.

## Configuration Example

### Creating a Full Mesh Tunnel via API

```bash
# 1. Create the tunnel
curl -X POST http://localhost:8080/api/v1/s2s-tunnels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "office-mesh",
    "description": "Office mesh: Site A, Site B, Site C",
    "topology": "mesh"
  }'

# 2. Add gateway members
curl -X POST http://localhost:8080/api/v1/s2s-tunnels/$TUNNEL_ID/members \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"gateway_id": "<gw-site-a-id>", "local_subnets": ["10.1.0.0/24"]}'

curl -X POST http://localhost:8080/api/v1/s2s-tunnels/$TUNNEL_ID/members \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"gateway_id": "<gw-site-b-id>", "local_subnets": ["10.2.0.0/24"]}'

# 3. Routes are computed automatically
# 4. Download config for a gateway
curl http://localhost:8080/api/v1/s2s-tunnels/$TUNNEL_ID/config/<gw-site-a-id> \
  -H "Authorization: Bearer $TOKEN"
```

### Creating a Hub & Spoke via the Admin Panel

1. Go to the **S2S Tunnels** page
2. Click **New Tunnel**
3. Enter a name and select the **Hub & Spoke** topology
4. Select the hub gateway
5. Add spoke gateways with their local subnets
6. Routes are automatically computed and sent to all gateways

## Network Map Visualization

The admin panel shows the S2S topology in real time in the **Dashboard > Network Topology** widget:

- **Core node** (diamond, blue) — central control plane
- **Gateways** (squares, green) — WireGuard endpoints
- **Devices** (circles) — client peers, grouped by gateway
- **S2S links** (dashed blue lines) — active S2S tunnels between gateways
- **gRPC links** (solid lines) — control plane connections

The visualization receives real-time data from:
- `GET /api/v1/gateways` — gateway list and status
- `GET /api/v1/devices` — device list
- `GET /api/v1/s2s-tunnels` — active tunnels and members

## Security Considerations

1. **Key isolation**: S2S and client WireGuard use separate key pairs
2. **Subnet validation**: Core verifies that advertised subnets do not overlap
3. **Route filtering**: Gateways accept routes only from core, never from peers
4. **Token-based authentication**: gRPC between gateway and core uses per-gateway HMAC tokens
5. **Audit logging**: All S2S configuration changes are recorded in `audit_log`

## Scaling

| Topology | Max Gateways | Peer Pairs | Notes |
|----------|-------------|------------|-------|
| Full Mesh | ~20 | N×(N-1)/2 | O(N^2) peer growth |
| Hub & Spoke | ~200 | N-1 | Hub bandwidth is the bottleneck |
| Hybrid | ~50 | Varies | Regional meshes connected via hub |

For >20 sites, hub & spoke is recommended. For large deployments, use regional hubs connected in a mesh (hybrid topology).
