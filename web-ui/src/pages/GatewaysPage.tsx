import { useTranslation } from 'react-i18next'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'

const mockGateways = [
  { id: '1', name: 'gw-moscow-01', endpoint: '185.12.34.10:51820', connectedPeers: 18, uptime: '45d 12h', health: 'healthy', version: '0.1.0', load: '23%' },
  { id: '2', name: 'gw-moscow-02', endpoint: '185.12.34.11:51820', connectedPeers: 16, uptime: '45d 12h', health: 'healthy', version: '0.1.0', load: '19%' },
  { id: '3', name: 'gw-spb-01', endpoint: '91.220.15.10:51820', connectedPeers: 18, uptime: '30d 8h', health: 'healthy', version: '0.1.0', load: '31%' },
  { id: '4', name: 'gw-nsk-01', endpoint: '78.105.22.10:51820', connectedPeers: 12, uptime: '15d 3h', health: 'degraded', version: '0.1.0', load: '67%' },
  { id: '5', name: 'gw-kazan-01', endpoint: '95.188.44.10:51820', connectedPeers: 8, uptime: '7d 22h', health: 'healthy', version: '0.1.0', load: '12%' },
  { id: '6', name: 'gw-staging-01', endpoint: '10.10.0.1:51820', connectedPeers: 8, uptime: '90d 1h', health: 'healthy', version: '0.1.0', load: '8%' },
]

export default function GatewaysPage() {
  const { t } = useTranslation()

  const columns = [
    {
      key: 'name',
      header: t('gateways.name'),
      sortable: true,
      render: (row: typeof mockGateways[0]) => (
        <span className="font-mono text-[var(--accent)]">{row.name}</span>
      ),
    },
    {
      key: 'endpoint',
      header: t('gateways.endpoint'),
      render: (row: typeof mockGateways[0]) => (
        <span className="font-mono text-xs">{row.endpoint}</span>
      ),
    },
    {
      key: 'connectedPeers',
      header: t('gateways.connectedPeers'),
      sortable: true,
      render: (row: typeof mockGateways[0]) => (
        <span className="font-mono text-[var(--text-primary)]">{row.connectedPeers}</span>
      ),
    },
    {
      key: 'uptime',
      header: t('gateways.uptime'),
      render: (row: typeof mockGateways[0]) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">{row.uptime}</span>
      ),
    },
    {
      key: 'health',
      header: t('gateways.health'),
      render: (row: typeof mockGateways[0]) => {
        const variant = row.health === 'healthy' ? 'online' : row.health === 'degraded' ? 'pending' : 'offline'
        return (
          <Badge variant={variant} pulse>
            {t(`status.${row.health}`)}
          </Badge>
        )
      },
    },
    {
      key: 'load',
      header: t('gateways.load'),
      sortable: true,
      render: (row: typeof mockGateways[0]) => {
        const pct = parseInt(row.load)
        const color = pct > 60 ? 'var(--warning)' : pct > 80 ? 'var(--danger)' : 'var(--accent)'
        return (
          <div className="flex items-center gap-2">
            <div className="h-1.5 w-16 rounded-full bg-[var(--bg-tertiary)]">
              <div
                className="h-full rounded-full transition-all"
                style={{ width: row.load, background: color }}
              />
            </div>
            <span className="font-mono text-xs text-[var(--text-muted)]">{row.load}</span>
          </div>
        )
      },
    },
    {
      key: 'version',
      header: t('gateways.version'),
      render: (row: typeof mockGateways[0]) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">v{row.version}</span>
      ),
    },
  ]

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
        <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
        {t('gateways.title')}
      </h1>

      <Table columns={columns} data={mockGateways} />
    </div>
  )
}
