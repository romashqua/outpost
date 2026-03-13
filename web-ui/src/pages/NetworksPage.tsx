import { useTranslation } from 'react-i18next'
import { Plus } from 'lucide-react'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'
import Button from '@/components/ui/Button'

const mockNetworks = [
  { id: '1', name: 'corp-moscow', cidr: '10.0.1.0/24', gateways: 2, peers: 34, status: 'active', description: 'Moscow office network' },
  { id: '2', name: 'corp-spb', cidr: '10.0.2.0/24', gateways: 1, peers: 18, status: 'active', description: 'Saint Petersburg office' },
  { id: '3', name: 'corp-nsk', cidr: '10.0.3.0/24', gateways: 1, peers: 12, status: 'active', description: 'Novosibirsk office' },
  { id: '4', name: 'dev-staging', cidr: '10.10.0.0/16', gateways: 2, peers: 8, status: 'active', description: 'Development staging env' },
  { id: '5', name: 'legacy-vpn', cidr: '172.16.0.0/20', gateways: 0, peers: 0, status: 'inactive', description: 'Legacy network (deprecated)' },
]

export default function NetworksPage() {
  const { t } = useTranslation()

  const columns = [
    {
      key: 'name',
      header: t('networks.name'),
      sortable: true,
      render: (row: typeof mockNetworks[0]) => (
        <span className="font-mono text-[var(--accent)]">{row.name}</span>
      ),
    },
    {
      key: 'cidr',
      header: t('networks.cidr'),
      render: (row: typeof mockNetworks[0]) => (
        <span className="font-mono text-[var(--text-primary)] bg-[var(--bg-tertiary)] px-2 py-0.5 rounded text-xs">
          {row.cidr}
        </span>
      ),
    },
    {
      key: 'gateways',
      header: t('networks.gateways'),
      sortable: true,
      render: (row: typeof mockNetworks[0]) => (
        <span className="font-mono">{row.gateways}</span>
      ),
    },
    {
      key: 'peers',
      header: t('networks.peers'),
      sortable: true,
      render: (row: typeof mockNetworks[0]) => (
        <span className="font-mono">{row.peers}</span>
      ),
    },
    {
      key: 'status',
      header: t('networks.status'),
      render: (row: typeof mockNetworks[0]) => (
        <Badge variant={row.status === 'active' ? 'online' : 'offline'} pulse>
          {t(`status.${row.status}`)}
        </Badge>
      ),
    },
    { key: 'description', header: t('common.description') },
  ]

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('networks.title')}
        </h1>
        <Button>
          <Plus size={16} />
          {t('networks.createNetwork')}
        </Button>
      </div>

      <Table columns={columns} data={mockNetworks} />
    </div>
  )
}
