import { useTranslation } from 'react-i18next'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'

const mockDevices = [
  { id: '1', name: 'laptop-01', owner: 'ivanov', pubkey: 'aB3kX9...mQ7Zw=', status: 'connected', lastHandshake: '2s ago', endpoint: '185.12.34.56:51820', allowedIps: '10.0.1.5/32', rx: '2.4 GB', tx: '1.1 GB' },
  { id: '2', name: 'desktop-01', owner: 'petrov', pubkey: 'cD5fR2...pL8Yv=', status: 'connected', lastHandshake: '5s ago', endpoint: '91.220.15.78:51820', allowedIps: '10.0.1.12/32', rx: '890 MB', tx: '340 MB' },
  { id: '3', name: 'laptop-02', owner: 'kozlov', pubkey: 'eF7hT4...nK0Xu=', status: 'connected', lastHandshake: '12s ago', endpoint: '78.105.22.91:51820', allowedIps: '10.0.2.3/32', rx: '1.2 GB', tx: '560 MB' },
  { id: '4', name: 'phone-01', owner: 'sidorov', pubkey: 'gH9jV6...rM2Ws=', status: 'connected', lastHandshake: '8s ago', endpoint: '95.188.44.12:51820', allowedIps: '10.0.3.7/32', rx: '120 MB', tx: '45 MB' },
  { id: '5', name: 'laptop-03', owner: 'fedorov', pubkey: 'iJ1lX8...tO4Uq=', status: 'disconnected', lastHandshake: '3 days ago', endpoint: '-', allowedIps: '10.0.1.20/32', rx: '0', tx: '0' },
  { id: '6', name: 'server-dev', owner: 'morozov', pubkey: 'kL3nZ0...vQ6Sp=', status: 'connected', lastHandshake: '1s ago', endpoint: '46.29.160.5:51820', allowedIps: '10.0.1.100/32', rx: '18.5 GB', tx: '12.3 GB' },
]

export default function DevicesPage() {
  const { t } = useTranslation()

  const columns = [
    {
      key: 'name',
      header: t('devices.name'),
      sortable: true,
      render: (row: typeof mockDevices[0]) => (
        <span className="font-mono text-[var(--text-primary)]">{row.name}</span>
      ),
    },
    {
      key: 'owner',
      header: t('devices.owner'),
      sortable: true,
      render: (row: typeof mockDevices[0]) => (
        <span className="font-mono text-[var(--accent)]">{row.owner}</span>
      ),
    },
    {
      key: 'pubkey',
      header: t('devices.pubkey'),
      render: (row: typeof mockDevices[0]) => (
        <span className="font-mono text-xs text-[var(--text-muted)] bg-[var(--bg-tertiary)] px-2 py-0.5 rounded">
          {row.pubkey}
        </span>
      ),
    },
    {
      key: 'status',
      header: t('devices.status'),
      render: (row: typeof mockDevices[0]) => (
        <Badge variant={row.status === 'connected' ? 'online' : 'offline'} pulse>
          {t(`status.${row.status}`)}
        </Badge>
      ),
    },
    {
      key: 'lastHandshake',
      header: t('devices.lastHandshake'),
      render: (row: typeof mockDevices[0]) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">{row.lastHandshake}</span>
      ),
    },
    {
      key: 'endpoint',
      header: t('devices.endpoint'),
      render: (row: typeof mockDevices[0]) => (
        <span className="font-mono text-xs">{row.endpoint}</span>
      ),
    },
    {
      key: 'rx',
      header: t('devices.rx') + '/' + t('devices.tx'),
      render: (row: typeof mockDevices[0]) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">
          {row.rx} / {row.tx}
        </span>
      ),
    },
  ]

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
        <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
        {t('devices.title')}
      </h1>

      <Table columns={columns} data={mockDevices} />
    </div>
  )
}
