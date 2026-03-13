import { useEffect, useRef, useState } from 'react'

interface NetworkNode {
  id: string
  label: string
  x: number
  y: number
  type: 'gateway' | 'peer' | 'hub'
  healthy: boolean
}

interface NetworkLink {
  from: string
  to: string
  active: boolean
}

interface NetworkMapProps {
  nodes?: NetworkNode[]
  links?: NetworkLink[]
}

const defaultNodes: NetworkNode[] = [
  { id: 'gw1', label: 'gw-moscow-01', x: 200, y: 80, type: 'gateway', healthy: true },
  { id: 'gw2', label: 'gw-spb-01', x: 500, y: 80, type: 'gateway', healthy: true },
  { id: 'gw3', label: 'gw-nsk-01', x: 350, y: 200, type: 'hub', healthy: true },
  { id: 'p1', label: 'peer-dev-01', x: 80, y: 200, type: 'peer', healthy: true },
  { id: 'p2', label: 'peer-dev-02', x: 120, y: 300, type: 'peer', healthy: false },
  { id: 'p3', label: 'peer-ops-01', x: 580, y: 200, type: 'peer', healthy: true },
  { id: 'p4', label: 'peer-ops-02', x: 620, y: 300, type: 'peer', healthy: true },
  { id: 'p5', label: 'site-kazan', x: 350, y: 340, type: 'peer', healthy: true },
]

const defaultLinks: NetworkLink[] = [
  { from: 'gw1', to: 'gw3', active: true },
  { from: 'gw2', to: 'gw3', active: true },
  { from: 'gw1', to: 'p1', active: true },
  { from: 'gw1', to: 'p2', active: false },
  { from: 'gw2', to: 'p3', active: true },
  { from: 'gw2', to: 'p4', active: true },
  { from: 'gw3', to: 'p5', active: true },
]

export default function NetworkMap({ nodes = defaultNodes, links = defaultLinks }: NetworkMapProps) {
  const [tick, setTick] = useState(0)
  const animRef = useRef<number>(0)

  useEffect(() => {
    const interval = setInterval(() => {
      setTick((t) => t + 1)
    }, 2000)
    return () => clearInterval(interval)
  }, [])

  // Cancel warning for unused var
  void tick
  void animRef

  const getNode = (id: string) => nodes.find((n) => n.id === id)

  return (
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
          <feGaussianBlur stdDeviation="3" result="blur" />
          <feMerge>
            <feMergeNode in="blur" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
      </defs>
      <rect width="700" height="400" fill="url(#grid)" />

      {/* Links */}
      {links.map((link, i) => {
        const from = getNode(link.from)
        const to = getNode(link.to)
        if (!from || !to) return null
        return (
          <line
            key={i}
            x1={from.x}
            y1={from.y}
            x2={to.x}
            y2={to.y}
            stroke={link.active ? 'rgba(0,255,136,0.3)' : 'rgba(255,68,68,0.2)'}
            strokeWidth={link.active ? 1.5 : 1}
            strokeDasharray={link.active ? 'none' : '4 4'}
          />
        )
      })}

      {/* Data flow particles on active links */}
      {links.filter(l => l.active).map((link, i) => {
        const from = getNode(link.from)
        const to = getNode(link.to)
        if (!from || !to) return null
        return (
          <circle key={`particle-${i}`} r="2" fill="var(--accent)" opacity="0.8">
            <animateMotion
              dur={`${2 + i * 0.5}s`}
              repeatCount="indefinite"
              path={`M${from.x},${from.y} L${to.x},${to.y}`}
            />
          </circle>
        )
      })}

      {/* Nodes */}
      {nodes.map((node) => {
        const isGateway = node.type === 'gateway' || node.type === 'hub'
        const nodeColor = node.healthy ? '#00ff88' : '#ff4444'
        const size = isGateway ? 18 : 12

        return (
          <g key={node.id}>
            {/* Pulse ring */}
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
                dur="3s"
                repeatCount="indefinite"
              />
              <animate
                attributeName="opacity"
                values="0.3;0;0.3"
                dur="3s"
                repeatCount="indefinite"
              />
            </circle>

            {/* Node shape */}
            {isGateway ? (
              <rect
                x={node.x - size / 2}
                y={node.y - size / 2}
                width={size}
                height={size}
                rx="3"
                fill={`${nodeColor}20`}
                stroke={nodeColor}
                strokeWidth="1.5"
                filter={node.healthy ? 'url(#glow-green)' : 'url(#glow-red)'}
              />
            ) : (
              <circle
                cx={node.x}
                cy={node.y}
                r={size / 2}
                fill={`${nodeColor}20`}
                stroke={nodeColor}
                strokeWidth="1.5"
                filter={node.healthy ? 'url(#glow-green)' : 'url(#glow-red)'}
              />
            )}

            {/* Label */}
            <text
              x={node.x}
              y={node.y + size + 12}
              textAnchor="middle"
              fill="var(--text-muted)"
              fontSize="9"
              fontFamily="'JetBrains Mono', monospace"
            >
              {node.label}
            </text>
          </g>
        )
      })}
    </svg>
  )
}
