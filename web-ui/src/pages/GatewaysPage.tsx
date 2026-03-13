import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2, Copy, Check } from 'lucide-react'
import { api } from '@/api/client'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'
import Button from '@/components/ui/Button'
import Modal from '@/components/ui/Modal'
import Input from '@/components/ui/Input'

interface Gateway {
  id: string
  network_id: string
  name: string
  public_ip: string | null
  wireguard_pubkey: string
  endpoint: string
  is_active: boolean
  priority: number
  last_seen: string | null
  created_at: string
}

interface CreateGatewayResponse extends Gateway {
  token: string
}

export default function GatewaysPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [showToken, setShowToken] = useState<string | null>(null)
  const [copiedToken, setCopiedToken] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  // Form state
  const [formName, setFormName] = useState('')
  const [formNetworkId, setFormNetworkId] = useState('')
  const [formEndpoint, setFormEndpoint] = useState('')
  const [formPublicIp, setFormPublicIp] = useState('')
  const [formPriority, setFormPriority] = useState('')

  const { data: gateways = [], isLoading, error } = useQuery({
    queryKey: ['gateways'],
    queryFn: () => api.get<Gateway[]>('/gateways'),
  })

  const createMutation = useMutation({
    mutationFn: (body: { name: string; network_id: string; endpoint: string; public_ip?: string; priority?: number }) =>
      api.post<CreateGatewayResponse>('/gateways', body),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['gateways'] })
      setShowCreate(false)
      setShowToken(data.token)
      resetForm()
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/gateways/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gateways'] })
      setDeleteId(null)
    },
  })

  function resetForm() {
    setFormName('')
    setFormNetworkId('')
    setFormEndpoint('')
    setFormPublicIp('')
    setFormPriority('')
  }

  function handleCreate(e: React.FormEvent) {
    e.preventDefault()
    createMutation.mutate({
      name: formName,
      network_id: formNetworkId,
      endpoint: formEndpoint,
      ...(formPublicIp ? { public_ip: formPublicIp } : {}),
      ...(formPriority ? { priority: parseInt(formPriority, 10) } : {}),
    })
  }

  function copyToken() {
    if (showToken) {
      navigator.clipboard.writeText(showToken)
      setCopiedToken(true)
      setTimeout(() => setCopiedToken(false), 2000)
    }
  }

  function formatLastSeen(lastSeen: string | null): string {
    if (!lastSeen) return '--'
    const diff = Date.now() - new Date(lastSeen).getTime()
    const minutes = Math.floor(diff / 60_000)
    if (minutes < 1) return 'just now'
    if (minutes < 60) return `${minutes}m ago`
    const hours = Math.floor(minutes / 60)
    if (hours < 24) return `${hours}h ago`
    const days = Math.floor(hours / 24)
    return `${days}d ago`
  }

  const columns = [
    {
      key: 'name',
      header: t('gateways.name'),
      sortable: true,
      render: (row: Gateway) => (
        <span className="font-mono text-[var(--accent)]">{row.name}</span>
      ),
    },
    {
      key: 'endpoint',
      header: t('gateways.endpoint'),
      render: (row: Gateway) => (
        <span className="font-mono text-xs">{row.endpoint}</span>
      ),
    },
    {
      key: 'public_ip',
      header: 'Public IP',
      render: (row: Gateway) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">{row.public_ip || '--'}</span>
      ),
    },
    {
      key: 'is_active',
      header: t('gateways.health'),
      render: (row: Gateway) => {
        const variant = row.is_active ? 'online' : 'offline'
        const label = row.is_active ? 'healthy' : 'offline'
        return (
          <Badge variant={variant} pulse>
            {t(`status.${label}`)}
          </Badge>
        )
      },
    },
    {
      key: 'priority',
      header: 'Priority',
      sortable: true,
      render: (row: Gateway) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">{row.priority}</span>
      ),
    },
    {
      key: 'last_seen',
      header: 'Last Seen',
      render: (row: Gateway) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">{formatLastSeen(row.last_seen)}</span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row: Gateway) => (
        <Button
          variant="ghost"
          size="sm"
          onClick={(e) => { e.stopPropagation(); setDeleteId(row.id) }}
        >
          <Trash2 size={14} className="text-[var(--danger)]" />
        </Button>
      ),
    },
  ]

  if (error) {
    return (
      <div>
        <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('gateways.title')}
        </h1>
        <div className="rounded-lg border border-[var(--danger)] bg-[var(--bg-card)] p-6 text-center text-[var(--danger)]">
          Failed to load gateways: {(error as Error).message}
        </div>
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('gateways.title')}
        </h1>
        <Button onClick={() => setShowCreate(true)}>
          <Plus size={16} />
          {t('common.create')}
        </Button>
      </div>

      {isLoading ? (
        <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-card)] p-8 text-center text-[var(--text-muted)]">
          Loading gateways...
        </div>
      ) : (
        <Table columns={columns} data={gateways} />
      )}

      {/* Create Modal */}
      <Modal open={showCreate} onClose={() => { setShowCreate(false); resetForm() }} title={t('common.create')}>
        <form className="flex flex-col gap-4" onSubmit={handleCreate}>
          <Input
            label={t('gateways.name')}
            placeholder="gw-moscow-01"
            value={formName}
            onChange={(e) => setFormName(e.target.value)}
            required
          />
          <Input
            label="Network ID"
            placeholder="network-uuid"
            value={formNetworkId}
            onChange={(e) => setFormNetworkId(e.target.value)}
            required
          />
          <Input
            label={t('gateways.endpoint')}
            placeholder="185.12.34.10:51820"
            value={formEndpoint}
            onChange={(e) => setFormEndpoint(e.target.value)}
            required
          />
          <Input
            label="Public IP"
            placeholder="185.12.34.10 (optional)"
            value={formPublicIp}
            onChange={(e) => setFormPublicIp(e.target.value)}
          />
          <Input
            label="Priority"
            placeholder="0 (optional)"
            type="number"
            value={formPriority}
            onChange={(e) => setFormPriority(e.target.value)}
          />
          {createMutation.isError && (
            <div className="text-xs text-[var(--danger)]">
              {(createMutation.error as Error).message}
            </div>
          )}
          <div className="flex gap-3 justify-end mt-2">
            <Button variant="secondary" type="button" onClick={() => { setShowCreate(false); resetForm() }}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={createMutation.isPending}>
              {createMutation.isPending ? 'Creating...' : t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Token Display Modal */}
      <Modal
        open={showToken !== null}
        onClose={() => { setShowToken(null); setCopiedToken(false) }}
        title="Gateway Token"
      >
        <div className="flex flex-col gap-4">
          <p className="text-sm text-[var(--warning)]">
            This token will only be shown once. Copy it now and store it securely.
          </p>
          <div className="flex items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] p-3">
            <code className="flex-1 text-xs font-mono text-[var(--accent)] break-all select-all">
              {showToken}
            </code>
            <Button variant="ghost" size="sm" onClick={copyToken}>
              {copiedToken ? <Check size={14} /> : <Copy size={14} />}
            </Button>
          </div>
          <div className="flex justify-end">
            <Button onClick={() => { setShowToken(null); setCopiedToken(false) }}>
              Done
            </Button>
          </div>
        </div>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        open={deleteId !== null}
        onClose={() => setDeleteId(null)}
        title="Delete Gateway"
      >
        <div className="flex flex-col gap-4">
          <p className="text-sm text-[var(--text-secondary)]">
            Are you sure you want to delete this gateway? This action cannot be undone.
          </p>
          {deleteMutation.isError && (
            <div className="text-xs text-[var(--danger)]">
              {(deleteMutation.error as Error).message}
            </div>
          )}
          <div className="flex gap-3 justify-end">
            <Button variant="secondary" onClick={() => setDeleteId(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant="danger"
              disabled={deleteMutation.isPending}
              onClick={() => deleteId && deleteMutation.mutate(deleteId)}
            >
              {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}
