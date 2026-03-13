# Outpost VPN — Mesh Networking Architecture

## Overview

Outpost supports two site-to-site (S2S) tunnel topologies:
- **Full Mesh** — every gateway connects directly to every other gateway
- **Hub & Spoke** — all gateways connect through a single hub gateway

Both topologies use WireGuard tunnels with separate interfaces for S2S traffic, isolated from client peer traffic.

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
                        │  │ Route Calculator│  │
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
          │ gw-moscow   │   │ gw-spb      │   │ gw-nsk      │
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
| POST | `/api/v1/s2s-tunnels` | Create tunnel (name, topology, description) |
| GET | `/api/v1/s2s-tunnels/{id}` | Get tunnel details |
| DELETE | `/api/v1/s2s-tunnels/{id}` | Delete tunnel |

**Database tables:**
- `s2s_tunnels` — tunnel metadata (name, topology, hub_gateway_id, description)
- `s2s_tunnel_members` — which gateways participate, with `local_subnets CIDR[]`
- `s2s_routes` — computed routes (destination CIDR, via_gateway, metric)

### 2. Topology Engine

When a tunnel is created or members change, the topology engine computes the required WireGuard peer configurations:

#### Full Mesh
Every gateway gets a peer entry for every other gateway in the tunnel:
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
All spoke gateways only connect to the hub. Traffic between spokes routes through the hub:
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

**Route computation flow:**
1. Gateway registers its `local_subnets` with core
2. Core aggregates all advertised routes from all tunnel members
3. Core computes the route table based on topology:
   - **Mesh**: direct routes between each gateway pair
   - **Hub-spoke**: routes via hub for inter-spoke traffic
4. Core pushes the computed route table to each gateway via gRPC stream
5. Gateway applies routes to its S2S WireGuard interface (`wg1`)

### 4. WireGuard Interface Isolation

Each gateway runs two WireGuard interfaces:

| Interface | Purpose | Port | Peers |
|-----------|---------|------|-------|
| `wg0` | Client VPN traffic | 51820/udp | User devices |
| `wg1` | S2S tunnel traffic | 51821/udp | Other gateways |

This isolation ensures:
- S2S key compromise doesn't affect client connections
- Separate firewall rules per interface
- Independent MTU/keepalive settings
- No routing loops between client and S2S traffic

### 5. Health Monitoring

Gateways continuously monitor S2S tunnel health:

```
Every 10s: ICMP echo to each S2S peer's tunnel IP
             │
             ├── Response < 100ms  → HEALTHY
             ├── Response > 500ms  → DEGRADED
             └── No response (3x)  → DOWN
```

When a gateway goes DOWN:
1. Core marks the gateway as unhealthy in the route table
2. For mesh: routes are recalculated excluding the dead gateway
3. For hub-spoke: if hub is down, traffic fails over to backup hub (if configured)
4. Core sends updated routes to all remaining gateways

## Configuration Example

### Creating a Full Mesh Tunnel via API

```bash
# 1. Create the tunnel
curl -X POST http://localhost:8080/api/v1/s2s-tunnels \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "office-mesh",
    "description": "Moscow, SPb, Novosibirsk office mesh",
    "topology": "mesh"
  }'

# 2. Add gateway members (via admin UI or future /members endpoint)
# Each member specifies which local subnets it advertises

# 3. Gateways receive config via gRPC and create wg1 interfaces
```

### Creating Hub & Spoke via Admin UI

1. Navigate to **S2S Tunnels** page
2. Click **New Tunnel**
3. Enter name and select **Hub & Spoke** topology
4. Select the hub gateway
5. Add spoke gateways with their local subnets
6. Routes are automatically computed and pushed to all gateways

## Network Map Visualization

The admin UI shows real-time S2S topology on the **Dashboard → Network Topology** widget:

- **Core node** (diamond, blue) — central control plane
- **Gateways** (squares, green) — WireGuard endpoints
- **Devices** (circles) — client peers grouped by gateway
- **S2S links** (dashed blue lines) — active S2S tunnels between gateways
- **gRPC links** (solid lines) — control plane connections

The visualization fetches live data from:
- `GET /api/v1/gateways` — gateway list and status
- `GET /api/v1/devices` — device list
- `GET /api/v1/s2s-tunnels` — active tunnels and members

## Security Considerations

1. **Key isolation**: S2S and client WireGuard use separate key pairs
2. **Subnet validation**: Core validates that advertised subnets don't overlap
3. **Route filtering**: Gateways only accept routes from core, never from peers
4. **Token auth**: Gateway-to-core gRPC uses per-gateway HMAC tokens
5. **Audit logging**: All S2S configuration changes are logged to `audit_log`

## Scaling

| Topology | Max Gateways | Peer Pairs | Notes |
|----------|-------------|------------|-------|
| Full Mesh | ~20 | N×(N-1)/2 | O(N²) peer growth |
| Hub & Spoke | ~200 | N-1 | Hub bandwidth is bottleneck |
| Hybrid | ~50 | Varies | Regional meshes connected via hub |

For >20 sites, hub & spoke is recommended. For large deployments, use regional hubs connected in a mesh (hybrid topology).
