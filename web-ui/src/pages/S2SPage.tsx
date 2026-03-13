import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Plus } from 'lucide-react'
import Card from '@/components/ui/Card'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'
import Button from '@/components/ui/Button'
import Modal from '@/components/ui/Modal'
import Input from '@/components/ui/Input'

const mockTunnels = [
  { id: '1', name: 'msk-spb-mesh', topology: 'mesh', sites: 'Moscow, SPB', health: 'healthy', latency: '8ms', throughput: '940 Mbps' },
  { id: '2', name: 'msk-nsk-hub', topology: 'hub-spoke', sites: 'Moscow (hub), NSK', health: 'healthy', latency: '45ms', throughput: '520 Mbps' },
  { id: '3', name: 'msk-kazan-hub', topology: 'hub-spoke', sites: 'Moscow (hub), Kazan', health: 'degraded', latency: '32ms', throughput: '380 Mbps' },
  { id: '4', name: 'staging-mesh', topology: 'mesh', sites: 'MSK-Staging, SPB-Staging', health: 'healthy', latency: '3ms', throughput: '980 Mbps' },
]

const tunnelNodes = [
  { id: 'msk', label: 'Moscow', x: 150, y: 100, healthy: true },
  { id: 'spb', label: 'SPB', x: 350, y: 60, healthy: true },
  { id: 'nsk', label: 'NSK', x: 500, y: 150, healthy: true },
  { id: 'kazan', label: 'Kazan', x: 350, y: 220, healthy: false },
]

const tunnelLinks = [
  { from: 'msk', to: 'spb', active: true },
  { from: 'msk', to: 'nsk', active: true },
  { from: 'msk', to: 'kazan', active: false },
  { from: 'spb', to: 'nsk', active: true },
]

export default function S2SPage() {
  const { t } = useTranslation()
  const [showCreate, setShowCreate] = useState(false)

  const getNode = (id: string) => tunnelNodes.find((n) => n.id === id)

  const columns = [
    {
      key: 'name',
      header: t('s2s.tunnelName'),
      sortable: true,
      render: (row: typeof mockTunnels[0]) => (
        <span className="font-mono text-[var(--accent)]">{row.name}</span>
      ),
    },
    {
      key: 'topology',
      header: t('s2s.topology'),
      render: (row: typeof mockTunnels[0]) => (
        <Badge variant="info">{row.topology === 'mesh' ? t('s2s.mesh') : t('s2s.hubSpoke')}</Badge>
      ),
    },
    { key: 'sites', header: t('s2s.sites') },
    {
      key: 'health',
      header: t('s2s.health'),
      render: (row: typeof mockTunnels[0]) => {
        const v = row.health === 'healthy' ? 'online' : row.health === 'degraded' ? 'pending' : 'offline'
        return <Badge variant={v} pulse>{t(`status.${row.health}`)}</Badge>
      },
    },
    {
      key: 'latency',
      header: t('s2s.latency'),
      render: (row: typeof mockTunnels[0]) => (
        <span className="font-mono text-xs">{row.latency}</span>
      ),
    },
    {
      key: 'throughput',
      header: t('s2s.throughput'),
      render: (row: typeof mockTunnels[0]) => (
        <span className="font-mono text-xs">{row.throughput}</span>
      ),
    },
  ]

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('s2s.title')}
        </h1>
        <Button onClick={() => setShowCreate(true)}>
          <Plus size={16} />
          {t('s2s.createTunnel')}
        </Button>
      </div>

      {/* Topology diagram */}
      <Card className="mb-6">
        <h2 className="text-sm font-medium text-[var(--text-primary)] mb-3 font-mono">
          Site Topology
        </h2>
        <svg viewBox="0 0 650 300" className="w-full h-48" style={{ background: 'var(--bg-primary)', borderRadius: '6px' }}>
          <defs>
            <pattern id="s2s-grid" width="30" height="30" patternUnits="userSpaceOnUse">
              <path d="M 30 0 L 0 0 0 30" fill="none" stroke="rgba(0,255,136,0.03)" strokeWidth="0.5" />
            </pattern>
          </defs>
          <rect width="650" height="300" fill="url(#s2s-grid)" />

          {tunnelLinks.map((link, i) => {
            const from = getNode(link.from)
            const to = getNode(link.to)
            if (!from || !to) return null
            return (
              <g key={i}>
                <line
                  x1={from.x} y1={from.y} x2={to.x} y2={to.y}
                  stroke={link.active ? 'rgba(0,255,136,0.3)' : 'rgba(255,68,68,0.2)'}
                  strokeWidth={link.active ? 2 : 1}
                  strokeDasharray={link.active ? 'none' : '6 4'}
                />
                {link.active && (
                  <circle r="3" fill="var(--accent)" opacity="0.7">
                    <animateMotion dur={`${2 + i}s`} repeatCount="indefinite" path={`M${from.x},${from.y} L${to.x},${to.y}`} />
                  </circle>
                )}
              </g>
            )
          })}

          {tunnelNodes.map((node) => (
            <g key={node.id}>
              <circle
                cx={node.x} cy={node.y} r="18"
                fill={`${node.healthy ? '#00ff88' : '#ff4444'}15`}
                stroke={node.healthy ? '#00ff88' : '#ff4444'}
                strokeWidth="1.5"
              />
              <circle cx={node.x} cy={node.y} r="18" fill="none" stroke={node.healthy ? '#00ff88' : '#ff4444'} strokeWidth="1" opacity="0.3">
                <animate attributeName="r" values="18;26;18" dur="3s" repeatCount="indefinite" />
                <animate attributeName="opacity" values="0.3;0;0.3" dur="3s" repeatCount="indefinite" />
              </circle>
              <text x={node.x} y={node.y + 4} textAnchor="middle" fill={node.healthy ? '#00ff88' : '#ff4444'} fontSize="10" fontFamily="'JetBrains Mono', monospace" fontWeight="600">
                {node.label.substring(0, 3).toUpperCase()}
              </text>
              <text x={node.x} y={node.y + 36} textAnchor="middle" fill="var(--text-muted)" fontSize="10" fontFamily="'JetBrains Mono', monospace">
                {node.label}
              </text>
            </g>
          ))}
        </svg>
      </Card>

      <Table columns={columns} data={mockTunnels} />

      <Modal open={showCreate} onClose={() => setShowCreate(false)} title={t('s2s.createTunnel')}>
        <form className="flex flex-col gap-4" onSubmit={(e) => { e.preventDefault(); setShowCreate(false) }}>
          <Input label={t('s2s.tunnelName')} placeholder="tunnel-name" />
          <Input label={t('s2s.topology')} placeholder="mesh / hub-spoke" />
          <Input label={t('s2s.sites')} placeholder="site1, site2" />
          <div className="flex gap-3 justify-end mt-2">
            <Button variant="secondary" type="button" onClick={() => setShowCreate(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit">{t('common.create')}</Button>
          </div>
        </form>
      </Modal>
    </div>
  )
}
