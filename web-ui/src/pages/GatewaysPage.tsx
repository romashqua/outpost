import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Pencil, Trash2, Copy, Check, Search, Link2, Unlink } from 'lucide-react'
import { api } from '@/api/client'
import { useToastStore } from '@/store/toast'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'
import Button from '@/components/ui/Button'
import Modal from '@/components/ui/Modal'
import Input from '@/components/ui/Input'

interface Network {
  id: string
  name: string
  address: string
  is_active: boolean
}

interface GatewayNetwork {
  id: string
  name: string
  address: string
}

interface Gateway {
  id: string
  network_id: string | null
  name: string
  public_ip: string | null
  wireguard_pubkey: string
  endpoint: string
  is_active: boolean
  priority: number
  last_seen: string | null
  created_at: string
  network_ids: string[]
  networks: GatewayNetwork[]
}

interface CreateGatewayResponse extends Gateway {
  token: string
}

interface FormErrors {
  name?: string
  endpoint?: string
  networks?: string
}

export default function GatewaysPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)
  const [showCreate, setShowCreate] = useState(false)
  const [showToken, setShowToken] = useState<string | null>(null)
  const [copiedToken, setCopiedToken] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [editTarget, setEditTarget] = useState<Gateway | null>(null)

  // Form state (create)
  const [formName, setFormName] = useState('')
  const [formNetworkIds, setFormNetworkIds] = useState<string[]>([])
  const [formEndpoint, setFormEndpoint] = useState('')
  const [formPublicIp, setFormPublicIp] = useState('')
  const [formPriority, setFormPriority] = useState('')
  const [formErrors, setFormErrors] = useState<FormErrors>({})

  // Form state (edit)
  const [editName, setEditName] = useState('')
  const [editNetworkIds, setEditNetworkIds] = useState<string[]>([])
  const [editEndpoint, setEditEndpoint] = useState('')
  const [editPublicIp, setEditPublicIp] = useState('')
  const [editPriority, setEditPriority] = useState('')
  const [editErrors, setEditErrors] = useState<FormErrors>({})

  const { data: gatewaysData, isLoading, error } = useQuery({
    queryKey: ['gateways'],
    queryFn: () => api.get<{ gateways: Gateway[]; total: number }>('/gateways'),
  })
  const gateways = gatewaysData?.gateways ?? []

  const { data: networksData } = useQuery({
    queryKey: ['networks'],
    queryFn: () => api.get<{ networks: Network[]; total: number }>('/networks'),
  })
  const networks = networksData?.networks ?? []

  const createMutation = useMutation({
    mutationFn: (body: { name: string; network_ids: string[]; endpoint: string; public_ip?: string; priority?: number }) =>
      api.post<CreateGatewayResponse>('/gateways', body),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['gateways'] })
      setShowCreate(false)
      setShowToken(data.token)
      resetForm()
      addToast(t('gateways.gatewayCreated'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const updateMutation = useMutation({
    mutationFn: (body: { id: string; name?: string; endpoint?: string; public_ip?: string | null; priority?: number; network_ids?: string[] }) => {
      const { id, ...rest } = body
      return api.put(`/gateways/${id}`, rest)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gateways'] })
      setEditTarget(null)
      addToast(t('gateways.gatewayUpdated', 'Gateway updated'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/gateways/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gateways'] })
      setDeleteId(null)
      addToast(t('gateways.gatewayDeleted'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  function resetForm() {
    setFormName('')
    setFormNetworkIds([])
    setFormEndpoint('')
    setFormPublicIp('')
    setFormPriority('')
    setFormErrors({})
  }

  function validateCreateForm(): boolean {
    const errors: FormErrors = {}
    if (!formName.trim()) errors.name = t('gateways.nameRequired')
    if (!formEndpoint.trim()) errors.endpoint = t('gateways.endpointRequired')
    if (formNetworkIds.length === 0) errors.networks = t('gateways.networksRequired')
    setFormErrors(errors)
    return Object.keys(errors).length === 0
  }

  function validateEditForm(): boolean {
    const errors: FormErrors = {}
    if (!editName.trim()) errors.name = t('gateways.nameRequired')
    if (!editEndpoint.trim()) errors.endpoint = t('gateways.endpointRequired')
    if (editNetworkIds.length === 0) errors.networks = t('gateways.networksRequired')
    setEditErrors(errors)
    return Object.keys(errors).length === 0
  }

  function handleCreate(e: React.FormEvent) {
    e.preventDefault()
    if (!validateCreateForm()) return
    let endpoint = formEndpoint.trim()
    if (endpoint && !endpoint.includes(':')) {
      endpoint = `${endpoint}:51820`
    }
    createMutation.mutate({
      name: formName.trim(),
      network_ids: formNetworkIds,
      endpoint,
      public_ip: formPublicIp.trim(),
      ...(formPriority ? { priority: parseInt(formPriority, 10) } : {}),
    })
  }

  function handleEdit(e: React.FormEvent) {
    e.preventDefault()
    if (!editTarget || !validateEditForm()) return
    let endpoint = editEndpoint.trim()
    if (endpoint && !endpoint.includes(':')) {
      endpoint = `${endpoint}:51820`
    }
    updateMutation.mutate({
      id: editTarget.id,
      name: editName.trim(),
      endpoint,
      public_ip: editPublicIp || null,
      priority: editPriority ? parseInt(editPriority, 10) : 0,
      network_ids: editNetworkIds,
    })
  }

  function toggleNetworkId(ids: string[], setIds: (v: string[]) => void, id: string) {
    setIds(ids.includes(id) ? ids.filter(x => x !== id) : [...ids, id])
  }

  function copyToken() {
    if (!showToken) return
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(showToken).then(() => {
        setCopiedToken(true)
        setTimeout(() => setCopiedToken(false), 2000)
      }).catch(() => {
        fallbackCopyToken(showToken)
      })
    } else {
      fallbackCopyToken(showToken)
    }
  }

  function fallbackCopyToken(text: string) {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.style.position = 'fixed'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    try {
      document.execCommand('copy')
      setCopiedToken(true)
      setTimeout(() => setCopiedToken(false), 2000)
    } catch {
      addToast('Failed to copy to clipboard', 'error')
    }
    document.body.removeChild(textarea)
  }

  function formatLastSeen(lastSeen: string | null): string {
    if (!lastSeen) return '--'
    const diff = Date.now() - new Date(lastSeen).getTime()
    const minutes = Math.floor(diff / 60_000)
    if (minutes < 1) return t('gateways.justNow')
    if (minutes < 60) return t('gateways.minutesAgo', { count: minutes })
    const hours = Math.floor(minutes / 60)
    if (hours < 24) return t('gateways.hoursAgo', { count: hours })
    const days = Math.floor(hours / 24)
    return t('gateways.daysAgo', { count: days })
  }

  function NetworkPicker({
    selectedIds,
    onToggle,
    error,
  }: {
    selectedIds: string[]
    onToggle: (id: string) => void
    error?: string
  }) {
    const [search, setSearch] = useState('')
    const activeNetworks = networks.filter(n => n.is_active)
    const filtered = activeNetworks.filter(n =>
      n.name.toLowerCase().includes(search.toLowerCase()) ||
      n.address.toLowerCase().includes(search.toLowerCase())
    )
    const selected = filtered.filter(n => selectedIds.includes(n.id))
    const available = filtered.filter(n => !selectedIds.includes(n.id))

    return (
      <div className="flex flex-col gap-1.5">
        <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
          {t('gateways.networks')} <span className="text-[var(--danger)]">*</span>
        </label>
        <Input
          placeholder={t('gateways.searchNetworks', 'Search networks...')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          icon={<Search size={14} />}
        />
        <div className="max-h-48 overflow-y-auto space-y-1">
          {selected.map((n) => (
            <div
              key={n.id}
              className="flex items-center justify-between px-3 py-2 rounded-lg border border-[var(--accent)]/30 bg-[var(--accent)]/5"
            >
              <div>
                <span className="font-mono text-sm text-[var(--accent)]">{n.name}</span>
                <span className="ml-2 text-xs text-[var(--text-muted)]">{n.address}</span>
              </div>
              <Button size="sm" variant="ghost" onClick={() => onToggle(n.id)}>
                <Unlink size={14} className="text-[var(--danger)]" />
              </Button>
            </div>
          ))}
          {available.map((n) => (
            <div
              key={n.id}
              className="flex items-center justify-between px-3 py-2 rounded-lg border border-[var(--border)] bg-[var(--bg-tertiary)] hover:border-[var(--accent)] transition-colors"
            >
              <div>
                <span className="font-mono text-sm text-[var(--text-primary)]">{n.name}</span>
                <span className="ml-2 text-xs text-[var(--text-muted)]">{n.address}</span>
              </div>
              <Button size="sm" variant="ghost" onClick={() => onToggle(n.id)}>
                <Link2 size={14} className="text-[var(--accent)]" />
              </Button>
            </div>
          ))}
          {filtered.length === 0 && (
            <p className="text-center py-4 text-xs text-[var(--text-muted)]">{t('gateways.noNetworks')}</p>
          )}
        </div>
        {error && <p className="text-xs text-[var(--danger)]">{error}</p>}
      </div>
    )
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
      key: 'networks',
      header: t('gateways.networkColumn'),
      render: (row: Gateway) => (
        <div className="flex flex-wrap gap-1">
          {(row.networks ?? []).length > 0 ? (
            row.networks.map((n) => (
              <span key={n.id} className="inline-flex items-center rounded-full bg-[var(--bg-tertiary)] px-2 py-0.5 text-xs font-mono text-[var(--accent)]">
                {n.name}
              </span>
            ))
          ) : (
            <span className="text-xs text-[var(--text-muted)]">--</span>
          )}
        </div>
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
      header: t('gateways.publicIp'),
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
      header: t('gateways.priority'),
      sortable: true,
      render: (row: Gateway) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">{row.priority}</span>
      ),
    },
    {
      key: 'last_seen',
      header: t('gateways.lastSeen'),
      render: (row: Gateway) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">{formatLastSeen(row.last_seen)}</span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row: Gateway) => (
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            onClick={(e) => {
              e.stopPropagation()
              updateMutation.reset()
              setEditName(row.name)
              setEditEndpoint(row.endpoint)
              setEditPublicIp(row.public_ip || '')
              setEditPriority(row.priority.toString())
              setEditNetworkIds(row.network_ids ?? [])
              setEditErrors({})
              setEditTarget(row)
            }}
          >
            <Pencil size={14} className="text-[var(--accent)]" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={(e) => { e.stopPropagation(); deleteMutation.reset(); setDeleteId(row.id) }}
          >
            <Trash2 size={14} className="text-[var(--danger)]" />
          </Button>
        </div>
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
          {t('gateways.failedToLoad')} {(error as Error).message}
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
        <Button onClick={() => { createMutation.reset(); resetForm(); setShowCreate(true) }}>
          <Plus size={16} />
          {t('common.create')}
        </Button>
      </div>

      {isLoading ? (
        <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-card)] p-8 text-center text-[var(--text-muted)]">
          {t('gateways.loadingGateways')}
        </div>
      ) : (
        <Table columns={columns} data={gateways} />
      )}

      {/* Create Modal */}
      <Modal open={showCreate} onClose={() => { setShowCreate(false); resetForm() }} title={t('gateways.createGateway')}>
        <form className="flex flex-col gap-4" onSubmit={handleCreate}>
          <div>
            <Input
              label={t('gateways.name')}
              placeholder="gw-moscow-01"
              value={formName}
              onChange={(e) => { setFormName(e.target.value); setFormErrors(prev => ({ ...prev, name: undefined })) }}
              required
            />
            {formErrors.name && <p className="text-xs text-[var(--danger)] mt-1">{formErrors.name}</p>}
          </div>
          <NetworkPicker
            selectedIds={formNetworkIds}
            onToggle={(id: string) => { toggleNetworkId(formNetworkIds, setFormNetworkIds, id); setFormErrors(prev => ({ ...prev, networks: undefined })) }}
            error={formErrors.networks}
          />
          <div>
            <Input
              label={t('gateways.endpoint')}
              placeholder="185.12.34.10:51820"
              value={formEndpoint}
              onChange={(e) => { setFormEndpoint(e.target.value); setFormErrors(prev => ({ ...prev, endpoint: undefined })) }}
              required
            />
            <p className="text-xs text-[var(--text-muted)] mt-1">{t('gateways.endpointHint')}</p>
            {formErrors.endpoint && <p className="text-xs text-[var(--danger)] mt-1">{formErrors.endpoint}</p>}
          </div>
          <Input
            label={t('gateways.publicIp')}
            placeholder="185.12.34.10"
            value={formPublicIp}
            onChange={(e) => setFormPublicIp(e.target.value)}
            required
          />
          <Input
            label={t('gateways.priority')}
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
              {createMutation.isPending ? t('gateways.creating') : t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Token Display Modal */}
      <Modal
        open={showToken !== null}
        onClose={() => { setShowToken(null); setCopiedToken(false) }}
        title={t('gateways.gatewayToken')}
      >
        <div className="flex flex-col gap-4">
          <p className="text-sm text-[var(--warning)]">
            {t('gateways.tokenWarning')}
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
              {t('common.done')}
            </Button>
          </div>
        </div>
      </Modal>

      {/* Edit Modal */}
      <Modal open={editTarget !== null} onClose={() => setEditTarget(null)} title={t('gateways.editGateway')}>
        <form className="flex flex-col gap-4" onSubmit={handleEdit}>
          <div>
            <Input
              label={t('gateways.name')}
              value={editName}
              onChange={(e) => { setEditName(e.target.value); setEditErrors(prev => ({ ...prev, name: undefined })) }}
              required
            />
            {editErrors.name && <p className="text-xs text-[var(--danger)] mt-1">{editErrors.name}</p>}
          </div>
          <NetworkPicker
            selectedIds={editNetworkIds}
            onToggle={(id: string) => { toggleNetworkId(editNetworkIds, setEditNetworkIds, id); setEditErrors(prev => ({ ...prev, networks: undefined })) }}
            error={editErrors.networks}
          />
          <div>
            <Input
              label={t('gateways.endpoint')}
              placeholder="185.12.34.10:51820"
              value={editEndpoint}
              onChange={(e) => { setEditEndpoint(e.target.value); setEditErrors(prev => ({ ...prev, endpoint: undefined })) }}
              required
            />
            <p className="text-xs text-[var(--text-muted)] mt-1">{t('gateways.endpointHint')}</p>
            {editErrors.endpoint && <p className="text-xs text-[var(--danger)] mt-1">{editErrors.endpoint}</p>}
          </div>
          <Input
            label={t('gateways.publicIp')}
            placeholder="185.12.34.10 (optional)"
            value={editPublicIp}
            onChange={(e) => setEditPublicIp(e.target.value)}
          />
          <Input
            label={t('gateways.priority')}
            placeholder="0"
            type="number"
            value={editPriority}
            onChange={(e) => setEditPriority(e.target.value)}
          />
          {updateMutation.isError && (
            <div className="text-xs text-[var(--danger)]">
              {(updateMutation.error as Error).message}
            </div>
          )}
          <div className="flex gap-3 justify-end mt-2">
            <Button variant="secondary" type="button" onClick={() => setEditTarget(null)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={updateMutation.isPending}>
              {updateMutation.isPending ? t('common.saving', 'Saving...') : t('common.save')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        open={deleteId !== null}
        onClose={() => setDeleteId(null)}
        title={t('gateways.deleteGateway')}
      >
        <div className="flex flex-col gap-4">
          <p className="text-sm text-[var(--text-secondary)]">
            {t('gateways.confirmDelete')}
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
              {deleteMutation.isPending ? t('gateways.deleting') : t('common.delete')}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  )
}
