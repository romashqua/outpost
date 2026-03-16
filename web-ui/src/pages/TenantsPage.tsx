import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Building2, Search, Pencil, Trash2, BarChart3, Plus, ArrowLeft,
  Users, Network, Router, Link2, Unlink, ChevronRight,
} from 'lucide-react'
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

interface TenantUser {
  id: string
  username: string
  email: string
  role: string
  is_active: boolean
}

interface TenantNetwork {
  id: string
  name: string
  cidr: string
  is_active: boolean
}

interface TenantGateway {
  id: string
  name: string
  endpoint: string
  is_online: boolean
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

type ResourceTab = 'users' | 'networks' | 'gateways'

function TenantDetail({ tenant, onBack }: { tenant: Tenant; onBack: () => void }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)
  const [activeTab, setActiveTab] = useState<ResourceTab>('users')
  const [showAssignModal, setShowAssignModal] = useState(false)
  const [assignSearch, setAssignSearch] = useState('')

  // Tenant resources
  const { data: tenantUsers } = useQuery<TenantUser[]>({
    queryKey: ['tenant-users', tenant.id],
    queryFn: () => api.get(`/tenants/${tenant.id}/users`),
  })

  const { data: tenantNetworks } = useQuery<TenantNetwork[]>({
    queryKey: ['tenant-networks', tenant.id],
    queryFn: () => api.get(`/tenants/${tenant.id}/networks`),
  })

  const { data: tenantGateways } = useQuery<TenantGateway[]>({
    queryKey: ['tenant-gateways', tenant.id],
    queryFn: () => api.get(`/tenants/${tenant.id}/gateways`),
  })

  // All resources (for assign modal)
  const { data: allUsers } = useQuery<TenantUser[]>({
    queryKey: ['users'],
    queryFn: () => api.get<{ users: TenantUser[] }>('/users').then((r) => r.users),
    enabled: showAssignModal && activeTab === 'users',
  })

  const { data: allNetworks } = useQuery<TenantNetwork[]>({
    queryKey: ['networks'],
    queryFn: () => api.get<{ networks: TenantNetwork[] }>('/networks').then((r) => r.networks),
    enabled: showAssignModal && activeTab === 'networks',
  })

  const { data: allGateways } = useQuery<TenantGateway[]>({
    queryKey: ['gateways'],
    queryFn: () => api.get<{ gateways: TenantGateway[] }>('/gateways').then((r) => r.gateways),
    enabled: showAssignModal && activeTab === 'gateways',
  })

  const assignMutation = useMutation({
    mutationFn: ({ type, resourceId }: { type: ResourceTab; resourceId: string }) => {
      const path = type === 'users' ? 'users' : type === 'networks' ? 'networks' : 'gateways'
      return api.post(`/tenants/${tenant.id}/${path}/${resourceId}`, {})
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: [`tenant-${vars.type}`, tenant.id] })
      queryClient.invalidateQueries({ queryKey: [vars.type === 'users' ? 'users' : vars.type === 'networks' ? 'networks' : 'gateways'] })
      addToast(t('tenants.resourceAssigned'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const unassignMutation = useMutation({
    mutationFn: ({ type, resourceId }: { type: ResourceTab; resourceId: string }) => {
      const path = type === 'users' ? 'users' : type === 'networks' ? 'networks' : 'gateways'
      return api.delete(`/tenants/${tenant.id}/${path}/${resourceId}`)
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: [`tenant-${vars.type}`, tenant.id] })
      queryClient.invalidateQueries({ queryKey: [vars.type === 'users' ? 'users' : vars.type === 'networks' ? 'networks' : 'gateways'] })
      addToast(t('tenants.resourceUnassigned'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const tabs: { key: ResourceTab; label: string; icon: typeof Users }[] = [
    { key: 'users', label: t('tenants.tabUsers'), icon: Users },
    { key: 'networks', label: t('tenants.tabNetworks'), icon: Network },
    { key: 'gateways', label: t('tenants.tabGateways'), icon: Router },
  ]

  // Filter out already-assigned resources for the assign modal
  const assignedIds = new Set(
    activeTab === 'users'
      ? (tenantUsers ?? []).map((u) => u.id)
      : activeTab === 'networks'
        ? (tenantNetworks ?? []).map((n) => n.id)
        : (tenantGateways ?? []).map((g) => g.id),
  )

  const availableResources =
    activeTab === 'users'
      ? (allUsers ?? []).filter((u) => !assignedIds.has(u.id))
      : activeTab === 'networks'
        ? (allNetworks ?? []).filter((n) => !assignedIds.has(n.id))
        : (allGateways ?? []).filter((g) => !assignedIds.has(g.id))

  const filteredAvailable = availableResources.filter((r: any) =>
    (r.name || r.username || '').toLowerCase().includes(assignSearch.toLowerCase()),
  )

  return (
    <div>
      {/* Header */}
      <div className="flex items-center gap-3 mb-6">
        <Button variant="ghost" size="sm" onClick={onBack}>
          <ArrowLeft size={16} />
        </Button>
        <div>
          <h1 className="text-xl font-semibold text-[var(--text-primary)]">
            <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
            {tenant.name}
          </h1>
          <p className="text-sm text-[var(--text-muted)] font-mono">{tenant.slug}</p>
        </div>
        <Badge variant={tenant.plan === 'enterprise' ? 'info' : tenant.plan === 'pro' ? 'online' : 'default'}>
          {t(`tenants.plan_${tenant.plan}`)}
        </Badge>
        <Badge variant={tenant.is_active ? 'online' : 'offline'} pulse>
          {t(`status.${tenant.is_active ? 'active' : 'inactive'}`)}
        </Badge>
      </div>

      {/* Tabs */}
      <div className="flex items-center gap-1 border-b border-[var(--border)] mb-4">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`flex items-center gap-2 px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${
              activeTab === tab.key
                ? 'border-[var(--accent)] text-[var(--accent)]'
                : 'border-transparent text-[var(--text-muted)] hover:text-[var(--text-secondary)]'
            }`}
          >
            <tab.icon size={16} />
            {tab.label}
          </button>
        ))}
        <div className="flex-1" />
        <Button
          size="sm"
          onClick={() => {
            assignMutation.reset()
            setAssignSearch('')
            setShowAssignModal(true)
          }}
        >
          <Link2 size={14} />
          {t('tenants.assignResource')}
        </Button>
      </div>

      {/* Tab Content */}
      {activeTab === 'users' && (
        <div className="space-y-2">
          {(tenantUsers ?? []).length === 0 ? (
            <div className="text-center py-12 text-[var(--text-muted)]">{t('tenants.noUsers')}</div>
          ) : (
            <Table
              columns={[
                {
                  key: 'username',
                  header: t('users.username'),
                  render: (row: TenantUser) => (
                    <span className="font-mono text-[var(--accent)]">{row.username}</span>
                  ),
                },
                { key: 'email', header: t('users.email') },
                {
                  key: 'role',
                  header: t('users.role'),
                  render: (row: TenantUser) => (
                    <Badge variant={row.role === 'admin' ? 'info' : 'default'}>{row.role}</Badge>
                  ),
                },
                {
                  key: 'actions',
                  header: '',
                  render: (row: TenantUser) => (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => unassignMutation.mutate({ type: 'users', resourceId: row.id })}
                      disabled={unassignMutation.isPending}
                    >
                      <Unlink size={14} className="text-[var(--danger)]" />
                    </Button>
                  ),
                },
              ]}
              data={tenantUsers ?? []}
            />
          )}
        </div>
      )}

      {activeTab === 'networks' && (
        <div className="space-y-2">
          {(tenantNetworks ?? []).length === 0 ? (
            <div className="text-center py-12 text-[var(--text-muted)]">{t('tenants.noNetworks')}</div>
          ) : (
            <Table
              columns={[
                {
                  key: 'name',
                  header: t('networks.name'),
                  render: (row: TenantNetwork) => (
                    <span className="font-mono text-[var(--accent)]">{row.name}</span>
                  ),
                },
                {
                  key: 'cidr',
                  header: t('networks.cidr'),
                  render: (row: TenantNetwork) => (
                    <span className="font-mono text-xs text-[var(--text-secondary)]">{row.cidr}</span>
                  ),
                },
                {
                  key: 'actions',
                  header: '',
                  render: (row: TenantNetwork) => (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => unassignMutation.mutate({ type: 'networks', resourceId: row.id })}
                      disabled={unassignMutation.isPending}
                    >
                      <Unlink size={14} className="text-[var(--danger)]" />
                    </Button>
                  ),
                },
              ]}
              data={tenantNetworks ?? []}
            />
          )}
        </div>
      )}

      {activeTab === 'gateways' && (
        <div className="space-y-2">
          {(tenantGateways ?? []).length === 0 ? (
            <div className="text-center py-12 text-[var(--text-muted)]">{t('tenants.noGateways')}</div>
          ) : (
            <Table
              columns={[
                {
                  key: 'name',
                  header: t('gateways.name'),
                  render: (row: TenantGateway) => (
                    <span className="font-mono text-[var(--accent)]">{row.name}</span>
                  ),
                },
                {
                  key: 'endpoint',
                  header: t('gateways.endpoint'),
                  render: (row: TenantGateway) => (
                    <span className="font-mono text-xs text-[var(--text-secondary)]">{row.endpoint}</span>
                  ),
                },
                {
                  key: 'status',
                  header: t('gateways.status'),
                  render: (row: TenantGateway) => (
                    <Badge variant={row.is_online ? 'online' : 'offline'} pulse>
                      {t(`status.${row.is_online ? 'online' : 'offline'}`)}
                    </Badge>
                  ),
                },
                {
                  key: 'actions',
                  header: '',
                  render: (row: TenantGateway) => (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => unassignMutation.mutate({ type: 'gateways', resourceId: row.id })}
                      disabled={unassignMutation.isPending}
                    >
                      <Unlink size={14} className="text-[var(--danger)]" />
                    </Button>
                  ),
                },
              ]}
              data={tenantGateways ?? []}
            />
          )}
        </div>
      )}

      {/* Assign Resource Modal */}
      <Modal
        open={showAssignModal}
        onClose={() => setShowAssignModal(false)}
        title={t('tenants.assignResource')}
      >
        <p className="text-sm text-[var(--text-muted)] mb-3">
          {t(`tenants.assignHint_${activeTab}`)}
        </p>
        <Input
          placeholder={t('tenants.searchResource')}
          value={assignSearch}
          onChange={(e) => setAssignSearch(e.target.value)}
          icon={<Search size={16} />}
        />
        <div className="mt-3 max-h-64 overflow-y-auto space-y-1">
          {filteredAvailable.length === 0 ? (
            <div className="text-center py-6 text-[var(--text-muted)] text-sm">
              {t('tenants.noAvailableResources')}
            </div>
          ) : (
            filteredAvailable.map((resource: any) => (
              <div
                key={resource.id}
                className="flex items-center justify-between px-3 py-2 rounded-lg border border-[var(--border)] bg-[var(--bg-tertiary)] hover:border-[var(--accent)] transition-colors"
              >
                <div>
                  <span className="font-mono text-sm text-[var(--accent)]">
                    {resource.username || resource.name}
                  </span>
                  {resource.email && (
                    <span className="ml-2 text-xs text-[var(--text-muted)]">{resource.email}</span>
                  )}
                  {resource.cidr && (
                    <span className="ml-2 text-xs text-[var(--text-muted)]">{resource.cidr}</span>
                  )}
                  {resource.endpoint && (
                    <span className="ml-2 text-xs text-[var(--text-muted)]">{resource.endpoint}</span>
                  )}
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => assignMutation.mutate({ type: activeTab, resourceId: resource.id })}
                  disabled={assignMutation.isPending}
                >
                  <Link2 size={14} className="text-[var(--accent)]" />
                </Button>
              </div>
            ))
          )}
        </div>
        <div className="flex justify-end mt-4">
          <Button variant="secondary" onClick={() => setShowAssignModal(false)}>
            {t('common.close')}
          </Button>
        </div>
      </Modal>
    </div>
  )
}

export default function TenantsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)
  const [search, setSearch] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [editTarget, setEditTarget] = useState<Tenant | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Tenant | null>(null)
  const [selectedTenant, setSelectedTenant] = useState<Tenant | null>(null)

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

  // Show detail view if a tenant is selected
  if (selectedTenant) {
    return <TenantDetail tenant={selectedTenant} onBack={() => setSelectedTenant(null)} />
  }

  const list = tenants ?? []
  const filtered = list.filter(
    (t) =>
      (t.name || '').toLowerCase().includes(search.toLowerCase()) ||
      (t.slug || '').toLowerCase().includes(search.toLowerCase()) ||
      (t.plan || '').toLowerCase().includes(search.toLowerCase()),
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
        <span className="font-mono text-[var(--accent)] cursor-pointer hover:underline" onClick={() => setSelectedTenant(row)}>
          {row.name}
        </span>
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
              setSelectedTenant(row)
            }}
            title={t('tenants.manageResources')}
          >
            <ChevronRight size={14} className="text-[var(--accent)]" />
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
    </div>
  )
}
