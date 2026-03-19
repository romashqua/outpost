import { useMemo, useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/api/client'

interface Gateway {
  id: string
  network_id: string
  name: string
  endpoint: string
  is_active: boolean
  health_status: string
  last_seen: string | null
  public_ip: string | null
}

interface Device {
  id: string
  user_id: string
  name: string
  assigned_ip: string
  is_approved: boolean
  last_handshake: string | null
}

interface S2STunnel {
  id: string
  name: string
  topology: 'mesh' | 'hub_spoke'
  is_active: boolean
  members?: { gateway_id: string }[]
}

type HealthLevel = 'healthy' | 'degraded' | 'unhealthy' | 'offline'

interface NetworkNode {
  id: string
  label: string
  x: number
  y: number
  type: 'gateway' | 'peer' | 'hub' | 'core'
  health: HealthLevel
  subtitle?: string
  detail?: string
}

interface NetworkLink {
  from: string
  to: string
  active: boolean
  label?: string
  type?: 'client' | 's2s' | 'core'
}

function resolveGatewayHealth(gw: Gateway): HealthLevel {
  if (!gw.is_active) return 'offline'
  const status = gw.health_status || 'unknown'
  if (status === 'healthy') return 'healthy'
  if (status === 'degraded') return 'degraded'
  if (status === 'unhealthy') return 'unhealthy'
  // For 'unknown', check last_seen: if never seen or >90s ago → offline
  if (!gw.last_seen) return 'offline'
  const ago = Date.now() - new Date(gw.last_seen).getTime()
  if (ago > 90_000) return 'unhealthy'
  return 'healthy'
}

function resolveDeviceHealth(dev: Device): HealthLevel {
  if (!dev.is_approved) return 'offline'
  if (!dev.last_handshake) return 'offline'
  const ago = Date.now() - new Date(dev.last_handshake).getTime()
  if (ago > 180_000) return 'unhealthy' // 3 min no handshake
  if (ago > 60_000) return 'degraded'
  return 'healthy'
}

const HEALTH_COLORS: Record<HealthLevel, string> = {
  healthy: '#00ff88',
  degraded: '#ffaa00',
  unhealthy: '#ff4444',
  offline: '#666666',
}

function layoutNodes(
  gateways: Gateway[],
  devices: Device[],
  tunnels: S2STunnel[],
): { nodes: NetworkNode[]; links: NetworkLink[] } {
  const nodes: NetworkNode[] = []
  const links: NetworkLink[] = []

  const W = 700
  const H = 400
  const centerX = W / 2
  const centerY = H / 2

  // Core node in the center
  nodes.push({
    id: 'core',
    label: 'outpost-core',
    x: centerX,
    y: centerY - 20,
    type: 'core',
    health: 'healthy',
    subtitle: 'control plane',
  })

  // Place gateways in a ring around core
  const gwRadius = 130
  gateways.forEach((gw, i) => {
    const angle = (2 * Math.PI * i) / Math.max(gateways.length, 1) - Math.PI / 2
    const x = centerX + gwRadius * Math.cos(angle)
    const y = centerY + gwRadius * Math.sin(angle)
    const health = resolveGatewayHealth(gw)

    nodes.push({
      id: `gw-${gw.id}`,
      label: gw.name,
      x,
      y,
      type: 'gateway',
      health,
      subtitle: gw.public_ip || gw.endpoint.split(':')[0],
      detail: `${gw.health_status || 'unknown'}${gw.last_seen ? ' • ' + formatAgo(gw.last_seen) : ''}`,
    })

    // Link gateway to core
    links.push({
      from: 'core',
      to: `gw-${gw.id}`,
      active: health !== 'offline',
      type: 'core',
      label: 'gRPC',
    })
  })

  // Place devices around their gateway
  const gwByNetwork: Record<string, string> = {}
  gateways.forEach((gw) => {
    if (!gwByNetwork[gw.network_id]) {
      gwByNetwork[gw.network_id] = gw.id
    }
  })

  const defaultGwId = gateways.length > 0 ? gateways[0].id : ''
  const peerRadius = 60
  const maxPeersShown = 12

  const devicesToShow = devices.slice(0, maxPeersShown)
  const gwNodeMap: Record<string, NetworkNode | undefined> = {}
  nodes.forEach((n) => { gwNodeMap[n.id] = n })

  devicesToShow.forEach((dev, i) => {
    const parentGwNodeId = `gw-${defaultGwId}`
    const parentNode = gwNodeMap[parentGwNodeId]
    if (!parentNode) return

    const angle = (2 * Math.PI * i) / Math.max(devicesToShow.length, 1) - Math.PI / 2
    const x = Math.min(W - 30, Math.max(30, parentNode.x + peerRadius * Math.cos(angle)))
    const y = Math.min(H - 30, Math.max(30, parentNode.y + peerRadius * Math.sin(angle)))

    const health = resolveDeviceHealth(dev)

    nodes.push({
      id: `dev-${dev.id}`,
      label: dev.name.length > 14 ? dev.name.slice(0, 12) + '..' : dev.name,
      x,
      y,
      type: 'peer',
      health,
      subtitle: dev.assigned_ip,
    })

    links.push({
      from: parentGwNodeId,
      to: `dev-${dev.id}`,
      active: health === 'healthy' || health === 'degraded',
      type: 'client',
    })
  })

  // S2S tunnel links between gateways
  tunnels.forEach((tunnel) => {
    if (!tunnel.members || tunnel.members.length < 2) return
    if (tunnel.topology === 'mesh') {
      for (let a = 0; a < tunnel.members.length; a++) {
        for (let b = a + 1; b < tunnel.members.length; b++) {
          links.push({
            from: `gw-${tunnel.members[a].gateway_id}`,
            to: `gw-${tunnel.members[b].gateway_id}`,
            active: tunnel.is_active,
            type: 's2s',
            label: tunnel.name,
          })
        }
      }
    } else {
      const hub = tunnel.members[0].gateway_id
      tunnel.members.slice(1).forEach((m) => {
        links.push({
          from: `gw-${hub}`,
          to: `gw-${m.gateway_id}`,
          active: tunnel.is_active,
          type: 's2s',
          label: tunnel.name,
        })
      })
    }
  })

  // Overflow node
  if (devices.length > maxPeersShown) {
    const parentNode = gwNodeMap[`gw-${defaultGwId}`]
    if (parentNode) {
      nodes.push({
        id: 'overflow',
        label: `+${devices.length - maxPeersShown} more`,
        x: parentNode.x,
        y: parentNode.y + peerRadius + 20,
        type: 'peer',
        health: 'healthy',
      })
    }
  }

  return { nodes, links }
}

function formatAgo(ts: string): string {
  const ms = Date.now() - new Date(ts).getTime()
  if (ms < 60_000) return 'just now'
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m ago`
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)}h ago`
  return `${Math.floor(ms / 86_400_000)}d ago`
}

export default function NetworkMap() {
  const [hoveredNode, setHoveredNode] = useState<string | null>(null)

  const { data: gatewaysData } = useQuery<{ gateways: Gateway[] }>({
    queryKey: ['gateways'],
    queryFn: () => api.get('/gateways'),
    staleTime: 30_000,
  })
  const gateways = gatewaysData?.gateways ?? []

  const { data: devicesData } = useQuery<{ devices: Device[] }>({
    queryKey: ['devices'],
    queryFn: () => api.get('/devices'),
    staleTime: 30_000,
  })
  const devices = devicesData?.devices ?? []

  const { data: tunnels = [] } = useQuery<S2STunnel[]>({
    queryKey: ['s2s-tunnels'],
    queryFn: () => api.get('/s2s-tunnels'),
    staleTime: 30_000,
  })

  const { nodes, links } = useMemo(
    () => layoutNodes(gateways, devices, tunnels),
    [gateways, devices, tunnels],
  )

  const getNode = useCallback((id: string) => nodes.find((n) => n.id === id), [nodes])

  const linkColor = (link: NetworkLink) => {
    if (!link.active) return 'rgba(102,102,102,0.2)'
    if (link.type === 's2s') return 'rgba(0,170,255,0.4)'
    if (link.type === 'core') return 'rgba(0,255,136,0.25)'
    return 'rgba(0,255,136,0.15)'
  }

  const linkWidth = (link: NetworkLink) => {
    if (link.type === 's2s') return 2
    if (link.type === 'core') return 1.5
    return 1
  }

  // Health summary counts
  const healthCounts = useMemo(() => {
    const counts = { healthy: 0, degraded: 0, unhealthy: 0, offline: 0 }
    nodes.forEach((n) => {
      if (n.type !== 'core' && n.id !== 'overflow') {
        counts[n.health]++
      }
    })
    return counts
  }, [nodes])

  // Empty state
  if (gateways.length === 0 && devices.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-[var(--text-muted)] font-mono">
        No gateways or devices configured
      </div>
    )
  }

  const hoveredNodeData = hoveredNode ? getNode(hoveredNode) : null

  return (
    <div className="relative w-full h-full">
      <svg
        viewBox="0 0 700 400"
        className="w-full h-full"
        style={{ background: 'var(--bg-primary)', borderRadius: '8px' }}
      >
        {/* Grid */}
        <defs>
          <pattern id="grid" width="40" height="40" patternUnits="userSpaceOnUse">
            <path d="M 40 0 L 0 0 0 40" fill="none" stroke="rgba(0,255,136,0.04)" strokeWidth="0.5" />
          </pattern>
          <filter id="glow-green">
            <feGaussianBlur stdDeviation="3" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
          <filter id="glow-red">
            <feGaussianBlur stdDeviation="4" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
          <filter id="glow-blue">
            <feGaussianBlur stdDeviation="3" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
          <filter id="glow-yellow">
            <feGaussianBlur stdDeviation="3" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>
        <rect width="700" height="400" fill="url(#grid)" />

        {/* Legend with health indicators */}
        <g transform="translate(10, 12)">
          <rect x="0" y="0" width="8" height="8" rx="1" fill="#00ff8840" stroke="#00ff88" strokeWidth="1" />
          <text x="12" y="7" fill="var(--text-muted)" fontSize="8" fontFamily="'JetBrains Mono', monospace">Gateway</text>
          <circle cx="74" cy="4" r="3" fill="#00ff8840" stroke="#00ff88" strokeWidth="1" />
          <text x="80" y="7" fill="var(--text-muted)" fontSize="8" fontFamily="'JetBrains Mono', monospace">Peer</text>
          <line x1="118" y1="4" x2="138" y2="4" stroke="rgba(0,170,255,0.6)" strokeWidth="2" strokeDasharray="4 2" />
          <text x="142" y="7" fill="var(--text-muted)" fontSize="8" fontFamily="'JetBrains Mono', monospace">S2S</text>
        </g>

        {/* Health summary (top-right) */}
        <g transform="translate(580, 12)">
          <circle cx="0" cy="4" r="3" fill="#00ff88" />
          <text x="6" y="7" fill="var(--text-muted)" fontSize="8" fontFamily="'JetBrains Mono', monospace">{healthCounts.healthy}</text>
          <circle cx="28" cy="4" r="3" fill="#ffaa00" />
          <text x="34" y="7" fill="var(--text-muted)" fontSize="8" fontFamily="'JetBrains Mono', monospace">{healthCounts.degraded}</text>
          <circle cx="56" cy="4" r="3" fill="#ff4444" />
          <text x="62" y="7" fill="var(--text-muted)" fontSize="8" fontFamily="'JetBrains Mono', monospace">{healthCounts.unhealthy}</text>
          <circle cx="84" cy="4" r="3" fill="#666666" />
          <text x="90" y="7" fill="var(--text-muted)" fontSize="8" fontFamily="'JetBrains Mono', monospace">{healthCounts.offline}</text>
        </g>

        {/* Links */}
        {links.map((link, i) => {
          const from = getNode(link.from)
          const to = getNode(link.to)
          if (!from || !to) return null
          return (
            <g key={`link-${i}`}>
              <line
                x1={from.x}
                y1={from.y}
                x2={to.x}
                y2={to.y}
                stroke={linkColor(link)}
                strokeWidth={linkWidth(link)}
                strokeDasharray={link.type === 's2s' ? '6 3' : link.active ? 'none' : '4 4'}
              />
              {link.type === 's2s' && link.label && (
                <text
                  x={(from.x + to.x) / 2}
                  y={(from.y + to.y) / 2 - 4}
                  textAnchor="middle"
                  fill="rgba(0,170,255,0.5)"
                  fontSize="7"
                  fontFamily="'JetBrains Mono', monospace"
                >
                  {link.label}
                </text>
              )}
            </g>
          )
        })}

        {/* Data flow particles on active links */}
        {links.filter((l) => l.active).map((link, i) => {
          const from = getNode(link.from)
          const to = getNode(link.to)
          if (!from || !to) return null
          const color = link.type === 's2s' ? '#00aaff' : 'var(--accent)'
          return (
            <circle key={`particle-${i}`} r="2" fill={color} opacity="0.7">
              <animateMotion
                dur={`${2 + (i % 5) * 0.7}s`}
                repeatCount="indefinite"
                path={`M${from.x},${from.y} L${to.x},${to.y}`}
              />
            </circle>
          )
        })}

        {/* Nodes */}
        {nodes.map((node) => {
          const isGateway = node.type === 'gateway' || node.type === 'hub'
          const isCore = node.type === 'core'
          const nodeColor = isCore ? '#00aaff' : HEALTH_COLORS[node.health]
          const size = isCore ? 22 : isGateway ? 16 : 10
          const isHovered = hoveredNode === node.id

          const glowFilter = isCore
            ? 'url(#glow-blue)'
            : node.health === 'healthy' ? 'url(#glow-green)'
            : node.health === 'degraded' ? 'url(#glow-yellow)'
            : node.health === 'unhealthy' ? 'url(#glow-red)'
            : 'none'

          return (
            <g
              key={node.id}
              onMouseEnter={() => setHoveredNode(node.id)}
              onMouseLeave={() => setHoveredNode(null)}
              style={{ cursor: 'pointer' }}
            >
              {/* Pulse ring for gateways and core */}
              {(isGateway || isCore) && node.health !== 'offline' && (
                <circle
                  cx={node.x}
                  cy={node.y}
                  r={size + 4}
                  fill="none"
                  stroke={nodeColor}
                  strokeWidth="1"
                  opacity="0.3"
                >
                  <animate
                    attributeName="r"
                    values={`${size + 2};${size + 10};${size + 2}`}
                    dur={node.health === 'unhealthy' ? '1.5s' : '3s'}
                    repeatCount="indefinite"
                  />
                  <animate
                    attributeName="opacity"
                    values="0.3;0;0.3"
                    dur={node.health === 'unhealthy' ? '1.5s' : '3s'}
                    repeatCount="indefinite"
                  />
                </circle>
              )}

              {/* Offline X marker for dead gateways */}
              {isGateway && node.health === 'offline' && (
                <g opacity="0.5">
                  <line
                    x1={node.x - size / 2 - 2} y1={node.y - size / 2 - 2}
                    x2={node.x + size / 2 + 2} y2={node.y + size / 2 + 2}
                    stroke="#ff4444" strokeWidth="2"
                  />
                  <line
                    x1={node.x + size / 2 + 2} y1={node.y - size / 2 - 2}
                    x2={node.x - size / 2 - 2} y2={node.y + size / 2 + 2}
                    stroke="#ff4444" strokeWidth="2"
                  />
                </g>
              )}

              {/* Node shape */}
              {isCore ? (
                <polygon
                  points={`${node.x},${node.y - size / 2} ${node.x + size / 2},${node.y} ${node.x},${node.y + size / 2} ${node.x - size / 2},${node.y}`}
                  fill={`${nodeColor}20`}
                  stroke={nodeColor}
                  strokeWidth={isHovered ? '2.5' : '1.5'}
                  filter="url(#glow-blue)"
                />
              ) : isGateway ? (
                <rect
                  x={node.x - size / 2}
                  y={node.y - size / 2}
                  width={size}
                  height={size}
                  rx="3"
                  fill={`${nodeColor}20`}
                  stroke={nodeColor}
                  strokeWidth={isHovered ? '2.5' : '1.5'}
                  filter={glowFilter}
                />
              ) : (
                <circle
                  cx={node.x}
                  cy={node.y}
                  r={size / 2}
                  fill={`${nodeColor}20`}
                  stroke={nodeColor}
                  strokeWidth={isHovered ? '2' : '1'}
                  filter={glowFilter}
                />
              )}

              {/* Label */}
              <text
                x={node.x}
                y={node.y + size / 2 + 12}
                textAnchor="middle"
                fill={node.health === 'offline' ? '#666' : 'var(--text-muted)'}
                fontSize={isCore ? '10' : '8'}
                fontFamily="'JetBrains Mono', monospace"
                fontWeight={isCore ? '600' : '400'}
                textDecoration={node.health === 'offline' && isGateway ? 'line-through' : 'none'}
              >
                {node.label}
              </text>

              {/* Subtitle */}
              {node.subtitle && (
                <text
                  x={node.x}
                  y={node.y + size / 2 + 22}
                  textAnchor="middle"
                  fill="var(--text-muted)"
                  fontSize="7"
                  fontFamily="'JetBrains Mono', monospace"
                  opacity="0.6"
                >
                  {node.subtitle}
                </text>
              )}
            </g>
          )
        })}
      </svg>

      {/* Hover tooltip */}
      {hoveredNodeData && hoveredNodeData.detail && (
        <div
          className="absolute pointer-events-none px-2.5 py-1.5 rounded text-xs font-mono border"
          style={{
            left: `${(hoveredNodeData.x / 700) * 100}%`,
            top: `${(hoveredNodeData.y / 400) * 100 - 12}%`,
            transform: 'translate(-50%, -100%)',
            background: 'var(--bg-card)',
            border: '1px solid var(--border)',
            color: HEALTH_COLORS[hoveredNodeData.health],
          }}
        >
          {hoveredNodeData.detail}
        </div>
      )}
    </div>
  )
}
