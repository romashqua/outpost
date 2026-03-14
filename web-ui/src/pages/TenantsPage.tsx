import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Building2, Search, Pencil, Trash2, BarChart3, Plus } from 'lucide-react'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'
import Button from '@/components/ui/Button'
import Input from '@/components/ui/Input'
import Modal from '@/components/ui/Modal'
import { api } from '@/api/client'
import { useToastStore } from '@/store/toast'

interface Tenant {
  id: string
  name: string
  slug: string
  plan: string
  max_users: number
  max_devices: number
  max_networks: number
  is_active: boolean
  created_at: string
  updated_at: string
}

interface TenantStats {
  tenant_id: string
  user_count: number
  device_count: number
  network_count: number
  gateway_count: number
}

interface CreateTenantPayload {
  name: string
  slug: string
  plan: string
  max_users: number
  max_devices: number
  max_networks: number
}

interface UpdateTenantPayload {
  name?: string
  slug?: string
  plan?: string
  max_users?: number
  max_devices?: number
  max_networks?: number
  is_active?: boolean
}

const planDefaults: Record<string, { max_users: number; max_devices: number; max_networks: number }> = {
  free: { max_users: 10, max_devices: 20, max_networks: 2 },
  pro: { max_users: 100, max_devices: 500, max_networks: 20 },
  enterprise: { max_users: 10000, max_devices: 50000, max_networks: 200 },
}

export default function TenantsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)
  const [search, setSearch] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [editTarget, setEditTarget] = useState<Tenant | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Tenant | null>(null)
  const [statsTarget, setStatsTarget] = useState<Tenant | null>(null)

  const [formData, setFormData] = useState<CreateTenantPayload>({
    name: '',
    slug: '',
    plan: 'free',
    max_users: 10,
    max_devices: 20,
    max_networks: 2,
  })

  const [editForm, setEditForm] = useState<UpdateTenantPayload & { is_active: boolean }>({
    name: '',
    slug: '',
    plan: 'free',
    max_users: 0,
    max_devices: 0,
    max_networks: 0,
    is_active: true,
  })

  const { data: tenants, isLoading, error } = useQuery<Tenant[]>({
    queryKey: ['tenants'],
    queryFn: () => api.get('/tenants'),
  })

  const { data: stats } = useQuery<TenantStats>({
    queryKey: ['tenant-stats', statsTarget?.id],
    queryFn: () => api.get(`/tenants/${statsTarget!.id}/stats`),
    enabled: !!statsTarget,
  })

  const createMutation = useMutation({
    mutationFn: (payload: CreateTenantPayload) => api.post('/tenants', payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      setShowCreate(false)
      setFormData({ name: '', slug: '', plan: 'free', max_users: 10, max_devices: 20, max_networks: 2 })
      addToast(t('tenants.tenantCreated'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const updateMutation = useMutation({
    mutationFn: (payload: { id: string } & UpdateTenantPayload) => {
      const { id, ...body } = payload
      return api.put(`/tenants/${id}`, body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      setEditTarget(null)
      addToast(t('tenants.tenantUpdated'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/tenants/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['tenants'] })
      setDeleteTarget(null)
      addToast(t('tenants.tenantDeactivated'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const list = tenants ?? []
  const filtered = list.filter(
    (t) =>
      t.name.toLowerCase().includes(search.toLowerCase()) ||
      t.slug.toLowerCase().includes(search.toLowerCase()) ||
      t.plan.toLowerCase().includes(search.toLowerCase()),
  )

  const handleCreateSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createMutation.mutate(formData)
  }

  const handlePlanChange = (plan: string) => {
    const defaults = planDefaults[plan] || planDefaults.free
    setFormData({ ...formData, plan, ...defaults })
  }

  const columns = [
    {
      key: 'name',
      header: t('tenants.name'),
      sortable: true,
      render: (row: Tenant) => (
        <span className="font-mono text-[var(--accent)]">{row.name}</span>
      ),
    },
    {
      key: 'slug',
      header: t('tenants.slug'),
      sortable: true,
      render: (row: Tenant) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">{row.slug}</span>
      ),
    },
    {
      key: 'plan',
      header: t('tenants.plan'),
      render: (row: Tenant) => (
        <Badge variant={row.plan === 'enterprise' ? 'info' : row.plan === 'pro' ? 'online' : 'default'}>
          {t(`tenants.plan_${row.plan}`)}
        </Badge>
      ),
    },
    {
      key: 'max_users',
      header: t('tenants.maxUsers'),
      render: (row: Tenant) => (
        <span className="font-mono text-xs text-[var(--text-secondary)]">{row.max_users}</span>
      ),
    },
    {
      key: 'is_active',
      header: t('tenants.status'),
      render: (row: Tenant) => (
        <Badge variant={row.is_active ? 'online' : 'offline'} pulse>
          {t(`status.${row.is_active ? 'active' : 'inactive'}`)}
        </Badge>
      ),
    },
    {
      key: 'created_at',
      header: t('tenants.createdAt'),
      sortable: true,
      render: (row: Tenant) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">
          {new Date(row.created_at).toLocaleDateString()}
        </span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row: Tenant) => (
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            onClick={(e) => {
              e.stopPropagation()
              setStatsTarget(row)
            }}
          >
            <BarChart3 size={14} className="text-[var(--text-secondary)]" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={(e) => {
              e.stopPropagation()
              updateMutation.reset()
              setEditForm({
                name: row.name,
                slug: row.slug,
                plan: row.plan,
                max_users: row.max_users,
                max_devices: row.max_devices,
                max_networks: row.max_networks,
                is_active: row.is_active,
              })
              setEditTarget(row)
            }}
          >
            <Pencil size={14} className="text-[var(--accent)]" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={(e) => {
              e.stopPropagation()
              deleteMutation.reset()
              setDeleteTarget(row)
            }}
          >
            <Trash2 size={14} className="text-[var(--danger)]" />
          </Button>
        </div>
      ),
    },
  ]

  if (error) {
    return (
      <div className="text-center py-12 text-[var(--danger)]">
        {t('tenants.failedToLoad')} {(error as Error).message}
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('tenants.title')}
        </h1>
        <Button onClick={() => { createMutation.reset(); setShowCreate(true) }}>
          <Plus size={16} />
          {t('tenants.createTenant')}
        </Button>
      </div>

      <div className="mb-4 max-w-sm">
        <Input
          placeholder={t('tenants.search')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          icon={<Search size={16} />}
        />
      </div>

      {isLoading ? (
        <div className="text-center py-12 text-[var(--text-muted)]">{t('common.loading')}</div>
      ) : (
        <Table columns={columns} data={filtered} />
      )}

      {/* Create Tenant Modal */}
      <Modal open={showCreate} onClose={() => setShowCreate(false)} title={t('tenants.createTenant')}>
        <form className="flex flex-col gap-4" onSubmit={handleCreateSubmit}>
          <Input
            label={t('tenants.name')}
            placeholder="Acme Corp"
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            required
          />
          <Input
            label={t('tenants.slug')}
            placeholder="acme-corp"
            value={formData.slug}
            onChange={(e) => setFormData({ ...formData, slug: e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, '-') })}
            required
          />
          <label className="flex flex-col gap-1 text-sm text-[var(--text-secondary)]">
            {t('tenants.plan')}
            <select
              value={formData.plan}
              onChange={(e) => handlePlanChange(e.target.value)}
              className="rounded border border-[var(--border)] bg-[var(--bg-secondary)] text-[var(--text-primary)] px-3 py-2 text-sm"
            >
              <option value="free">{t('tenants.plan_free')}</option>
              <option value="pro">{t('tenants.plan_pro')}</option>
              <option value="enterprise">{t('tenants.plan_enterprise')}</option>
            </select>
          </label>
          <div className="grid grid-cols-3 gap-4">
            <Input
              label={t('tenants.maxUsers')}
              type="number"
              value={formData.max_users.toString()}
              onChange={(e) => setFormData({ ...formData, max_users: parseInt(e.target.value) || 0 })}
            />
            <Input
              label={t('tenants.maxDevices')}
              type="number"
              value={formData.max_devices.toString()}
              onChange={(e) => setFormData({ ...formData, max_devices: parseInt(e.target.value) || 0 })}
            />
            <Input
              label={t('tenants.maxNetworks')}
              type="number"
              value={formData.max_networks.toString()}
              onChange={(e) => setFormData({ ...formData, max_networks: parseInt(e.target.value) || 0 })}
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
              {createMutation.isPending ? t('tenants.creating') : t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Edit Tenant Modal */}
      <Modal open={!!editTarget} onClose={() => setEditTarget(null)} title={t('tenants.editTenant')}>
        <form className="flex flex-col gap-4" onSubmit={(e) => {
          e.preventDefault()
          if (editTarget) updateMutation.mutate({ id: editTarget.id, ...editForm })
        }}>
          <Input
            label={t('tenants.name')}
            placeholder="Acme Corp"
            value={editForm.name || ''}
            onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
            required
          />
          <Input
            label={t('tenants.slug')}
            placeholder="acme-corp"
            value={editForm.slug || ''}
            onChange={(e) => setEditForm({ ...editForm, slug: e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, '-') })}
            required
          />
          <label className="flex flex-col gap-1 text-sm text-[var(--text-secondary)]">
            {t('tenants.plan')}
            <select
              value={editForm.plan || 'free'}
              onChange={(e) => setEditForm({ ...editForm, plan: e.target.value })}
              className="rounded border border-[var(--border)] bg-[var(--bg-secondary)] text-[var(--text-primary)] px-3 py-2 text-sm"
            >
              <option value="free">{t('tenants.plan_free')}</option>
              <option value="pro">{t('tenants.plan_pro')}</option>
              <option value="enterprise">{t('tenants.plan_enterprise')}</option>
            </select>
          </label>
          <div className="grid grid-cols-3 gap-4">
            <Input
              label={t('tenants.maxUsers')}
              type="number"
              value={(editForm.max_users ?? 0).toString()}
              onChange={(e) => setEditForm({ ...editForm, max_users: parseInt(e.target.value) || 0 })}
            />
            <Input
              label={t('tenants.maxDevices')}
              type="number"
              value={(editForm.max_devices ?? 0).toString()}
              onChange={(e) => setEditForm({ ...editForm, max_devices: parseInt(e.target.value) || 0 })}
            />
            <Input
              label={t('tenants.maxNetworks')}
              type="number"
              value={(editForm.max_networks ?? 0).toString()}
              onChange={(e) => setEditForm({ ...editForm, max_networks: parseInt(e.target.value) || 0 })}
            />
          </div>
          <label className="flex items-center gap-2 text-sm text-[var(--text-secondary)] cursor-pointer">
            <input
              type="checkbox"
              checked={editForm.is_active}
              onChange={(e) => setEditForm({ ...editForm, is_active: e.target.checked })}
              className="rounded"
            />
            {t('status.active')}
          </label>
          {updateMutation.error && (
            <p className="text-sm text-[var(--danger)]">
              {(updateMutation.error as Error).message}
            </p>
          )}
          <div className="flex gap-3 justify-end mt-2">
            <Button variant="secondary" type="button" onClick={() => setEditTarget(null)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={updateMutation.isPending}>
              {updateMutation.isPending ? t('tenants.saving') : t('common.save')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delete/Deactivate Confirmation Modal */}
      <Modal
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title={t('tenants.deactivateTenant')}
      >
        <p className="text-[var(--text-secondary)] mb-6">
          {t('tenants.confirmDeactivate')}{' '}
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
            {deleteMutation.isPending ? t('tenants.deactivating') : t('tenants.deactivate')}
          </Button>
        </div>
      </Modal>

      {/* Stats Modal */}
      <Modal
        open={!!statsTarget}
        onClose={() => setStatsTarget(null)}
        title={`${t('tenants.stats')}: ${statsTarget?.name ?? ''}`}
      >
        {stats ? (
          <div className="grid grid-cols-2 gap-4">
            <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-tertiary)] p-4">
              <p className="text-xs text-[var(--text-muted)] mb-1">{t('tenants.userCount')}</p>
              <p className="text-2xl font-mono text-[var(--accent)]">{stats.user_count}</p>
            </div>
            <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-tertiary)] p-4">
              <p className="text-xs text-[var(--text-muted)] mb-1">{t('tenants.deviceCount')}</p>
              <p className="text-2xl font-mono text-[var(--accent)]">{stats.device_count}</p>
            </div>
            <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-tertiary)] p-4">
              <p className="text-xs text-[var(--text-muted)] mb-1">{t('tenants.networkCount')}</p>
              <p className="text-2xl font-mono text-[var(--accent)]">{stats.network_count}</p>
            </div>
            <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-tertiary)] p-4">
              <p className="text-xs text-[var(--text-muted)] mb-1">{t('tenants.gatewayCount')}</p>
              <p className="text-2xl font-mono text-[var(--accent)]">{stats.gateway_count}</p>
            </div>
          </div>
        ) : (
          <div className="text-center py-8 text-[var(--text-muted)]">{t('common.loading')}</div>
        )}
        <div className="flex justify-end mt-4">
          <Button variant="secondary" onClick={() => setStatsTarget(null)}>
            {t('common.close')}
          </Button>
        </div>
      </Modal>
    </div>
  )
}
