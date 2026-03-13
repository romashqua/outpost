import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'
import Button from '@/components/ui/Button'
import Input from '@/components/ui/Input'
import Modal from '@/components/ui/Modal'
import { api } from '@/api/client'

interface Network {
  id: string
  name: string
  address: string
  dns: string | null
  port: number | null
  keepalive: number | null
  is_active: boolean
  created_at: string
  updated_at: string
}

interface CreateNetworkPayload {
  name: string
  address: string
  dns?: string
  port?: number
  keepalive?: number
}

export default function NetworksPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<Network | null>(null)

  const [formData, setFormData] = useState<CreateNetworkPayload>({
    name: '',
    address: '',
    dns: '',
    port: undefined,
    keepalive: undefined,
  })

  const { data: networks = [], isLoading, error } = useQuery<Network[]>({
    queryKey: ['networks'],
    queryFn: () => api.get('/networks'),
  })

  const createMutation = useMutation({
    mutationFn: (payload: CreateNetworkPayload) => {
      const body: Record<string, unknown> = { name: payload.name, address: payload.address }
      if (payload.dns) body.dns = payload.dns
      if (payload.port) body.port = payload.port
      if (payload.keepalive) body.keepalive = payload.keepalive
      return api.post('/networks', body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['networks'] })
      setShowCreate(false)
      setFormData({ name: '', address: '', dns: '', port: undefined, keepalive: undefined })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/networks/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['networks'] })
      setDeleteTarget(null)
    },
  })

  const handleCreateSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createMutation.mutate(formData)
  }

  const columns = [
    {
      key: 'name',
      header: t('networks.name'),
      sortable: true,
      render: (row: Network) => (
        <span className="font-mono text-[var(--accent)]">{row.name}</span>
      ),
    },
    {
      key: 'address',
      header: t('networks.cidr'),
      render: (row: Network) => (
        <span className="font-mono text-[var(--text-primary)] bg-[var(--bg-tertiary)] px-2 py-0.5 rounded text-xs">
          {row.address}
        </span>
      ),
    },
    {
      key: 'dns',
      header: 'DNS',
      render: (row: Network) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">{row.dns ?? '-'}</span>
      ),
    },
    {
      key: 'port',
      header: t('networks.port') || 'Port',
      render: (row: Network) => (
        <span className="font-mono">{row.port ?? '-'}</span>
      ),
    },
    {
      key: 'is_active',
      header: t('networks.status'),
      render: (row: Network) => (
        <Badge variant={row.is_active ? 'online' : 'offline'} pulse>
          {t(`status.${row.is_active ? 'active' : 'inactive'}`)}
        </Badge>
      ),
    },
    {
      key: 'created_at',
      header: t('common.createdAt') || 'Created',
      sortable: true,
      render: (row: Network) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">
          {new Date(row.created_at).toLocaleDateString()}
        </span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row: Network) => (
        <Button
          variant="ghost"
          size="sm"
          onClick={(e) => {
            e.stopPropagation()
            setDeleteTarget(row)
          }}
        >
          <Trash2 size={14} className="text-[var(--danger)]" />
        </Button>
      ),
    },
  ]

  if (error) {
    return (
      <div className="text-center py-12 text-[var(--danger)]">
        Failed to load networks: {(error as Error).message}
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('networks.title')}
        </h1>
        <Button onClick={() => setShowCreate(true)}>
          <Plus size={16} />
          {t('networks.createNetwork')}
        </Button>
      </div>

      {isLoading ? (
        <div className="text-center py-12 text-[var(--text-muted)]">Loading...</div>
      ) : (
        <Table columns={columns} data={networks} />
      )}

      {/* Create Network Modal */}
      <Modal open={showCreate} onClose={() => setShowCreate(false)} title={t('networks.createNetwork')}>
        <form className="flex flex-col gap-4" onSubmit={handleCreateSubmit}>
          <Input
            label={t('networks.name')}
            placeholder="corp-moscow"
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            required
          />
          <Input
            label={t('networks.cidr') || 'Address (CIDR)'}
            placeholder="10.0.1.0/24"
            value={formData.address}
            onChange={(e) => setFormData({ ...formData, address: e.target.value })}
            required
          />
          <Input
            label="DNS"
            placeholder="1.1.1.1"
            value={formData.dns ?? ''}
            onChange={(e) => setFormData({ ...formData, dns: e.target.value })}
          />
          <div className="grid grid-cols-2 gap-4">
            <Input
              label={t('networks.port') || 'Port'}
              placeholder="51820"
              type="number"
              value={formData.port ?? ''}
              onChange={(e) =>
                setFormData({ ...formData, port: e.target.value ? Number(e.target.value) : undefined })
              }
            />
            <Input
              label="Keepalive"
              placeholder="25"
              type="number"
              value={formData.keepalive ?? ''}
              onChange={(e) =>
                setFormData({ ...formData, keepalive: e.target.value ? Number(e.target.value) : undefined })
              }
            />
          </div>
          {createMutation.error && (
            <p className="text-sm text-[var(--danger)]">
              {(createMutation.error as Error).message}
            </p>
          )}
          <div className="flex gap-3 justify-end mt-2">
            <Button variant="secondary" type="button" onClick={() => setShowCreate(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={createMutation.isPending}>
              {createMutation.isPending ? 'Creating...' : t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title={t('networks.deleteNetwork') || 'Delete network'}
      >
        <p className="text-[var(--text-secondary)] mb-6">
          Are you sure you want to delete network{' '}
          <span className="font-mono text-[var(--accent)]">{deleteTarget?.name}</span>?
        </p>
        {deleteMutation.error && (
          <p className="text-sm text-[var(--danger)] mb-4">
            {(deleteMutation.error as Error).message}
          </p>
        )}
        <div className="flex gap-3 justify-end">
          <Button variant="secondary" onClick={() => setDeleteTarget(null)}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="danger"
            disabled={deleteMutation.isPending}
            onClick={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
          >
            {deleteMutation.isPending ? 'Deleting...' : t('common.delete') || 'Delete'}
          </Button>
        </div>
      </Modal>
    </div>
  )
}
