import { useTranslation } from 'react-i18next'
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2, Route, ChevronDown, ChevronRight } from 'lucide-react'
import { api } from '@/api/client'
import { useToastStore } from '@/store/toast'
import Card from '@/components/ui/Card'
import Button from '@/components/ui/Button'
import Badge from '@/components/ui/Badge'
import Table from '@/components/ui/Table'
import Modal from '@/components/ui/Modal'
import Input from '@/components/ui/Input'

interface SmartRouteEntry {
  id: string
  smart_route_id: string
  entry_type: 'domain' | 'cidr' | 'domain_suffix'
  value: string
  action: 'proxy' | 'direct' | 'block'
  proxy_id: string | null
  proxy_name: string | null
  priority: number
  created_at: string
}

interface SmartRoute {
  id: string
  name: string
  description: string | null
  is_active: boolean
  created_at: string
  updated_at: string
  entries?: SmartRouteEntry[]
}

interface ProxyServer {
  id: string
  name: string
  type: 'socks5' | 'http' | 'shadowsocks' | 'vless'
  address: string
  port: number
  username: string | null
  password: string | null
  extra_config: string | null
  is_active: boolean
  created_at: string
  updated_at: string
}

type Tab = 'routes' | 'proxies'

export default function SmartRoutesPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)

  const [activeTab, setActiveTab] = useState<Tab>('routes')
  const [showCreateRoute, setShowCreateRoute] = useState(false)
  const [showCreateProxy, setShowCreateProxy] = useState(false)
  const [showAddEntry, setShowAddEntry] = useState<string | null>(null)
  const [deleteRouteId, setDeleteRouteId] = useState<string | null>(null)
  const [deleteProxyId, setDeleteProxyId] = useState<string | null>(null)
  const [expandedRoute, setExpandedRoute] = useState<string | null>(null)

  const [routeForm, setRouteForm] = useState({ name: '', description: '' })
  const [proxyForm, setProxyForm] = useState({ name: '', type: 'socks5', address: '', port: '' })
  const [entryForm, setEntryForm] = useState({ entry_type: 'domain', value: '', action: 'proxy', proxy_id: '', priority: '100' })

  // --- Data fetching ---

  const { data: routes = [], isLoading: routesLoading } = useQuery<SmartRoute[]>({
    queryKey: ['smart-routes'],
    queryFn: () => api.get('/smart-routes'),
  })

  const { data: proxies = [], isLoading: proxiesLoading } = useQuery<ProxyServer[]>({
    queryKey: ['proxy-servers'],
    queryFn: () => api.get('/smart-routes/proxy-servers'),
  })

  const { data: expandedRouteData } = useQuery<SmartRoute>({
    queryKey: ['smart-routes', expandedRoute],
    queryFn: () => api.get(`/smart-routes/${expandedRoute}`),
    enabled: !!expandedRoute,
  })

  // --- Mutations ---

  const createRouteMutation = useMutation({
    mutationFn: (data: { name: string; description?: string }) =>
      api.post('/smart-routes', data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['smart-routes'] })
      setShowCreateRoute(false)
      setRouteForm({ name: '', description: '' })
      addToast('Route group created', 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const deleteRouteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/smart-routes/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['smart-routes'] })
      setDeleteRouteId(null)
      setExpandedRoute(null)
      addToast('Route group deleted', 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const createProxyMutation = useMutation({
    mutationFn: (data: { name: string; type: string; address: string; port: number }) =>
      api.post('/smart-routes/proxy-servers', data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['proxy-servers'] })
      setShowCreateProxy(false)
      setProxyForm({ name: '', type: 'socks5', address: '', port: '' })
      addToast('Proxy server created', 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const deleteProxyMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/smart-routes/proxy-servers/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['proxy-servers'] })
      setDeleteProxyId(null)
      addToast('Proxy server deleted', 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const addEntryMutation = useMutation({
    mutationFn: (data: { routeId: string; entry_type: string; value: string; action: string; proxy_id?: string; priority: number }) => {
      const { routeId, ...body } = data
      const payload: Record<string, unknown> = { ...body }
      if (!payload.proxy_id) delete payload.proxy_id
      return api.post(`/smart-routes/${routeId}/entries`, payload)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['smart-routes'] })
      setShowAddEntry(null)
      setEntryForm({ entry_type: 'domain', value: '', action: 'proxy', proxy_id: '', priority: '100' })
      addToast('Entry added', 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const deleteEntryMutation = useMutation({
    mutationFn: ({ routeId, entryId }: { routeId: string; entryId: string }) =>
      api.delete(`/smart-routes/${routeId}/entries/${entryId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['smart-routes'] })
      addToast('Entry removed', 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  // --- Helpers ---

  const entryTypeBadge = (type: string) => {
    switch (type) {
      case 'domain': return <Badge variant="info">domain</Badge>
      case 'cidr': return <Badge variant="pending">cidr</Badge>
      case 'domain_suffix': return <Badge variant="online">suffix</Badge>
      default: return <Badge>{type}</Badge>
    }
  }

  const actionBadge = (action: string) => {
    switch (action) {
      case 'proxy': return <Badge variant="info">{t('smartRoutes.proxy')}</Badge>
      case 'direct': return <Badge variant="online">{t('smartRoutes.direct')}</Badge>
      case 'block': return <Badge variant="offline">{t('smartRoutes.block')}</Badge>
      default: return <Badge>{action}</Badge>
    }
  }

  const toggleExpand = (id: string) => {
    setExpandedRoute(expandedRoute === id ? null : id)
  }

  // --- Routes Tab ---

  const routeColumns = [
    {
      key: 'expand',
      header: '',
      render: (row: SmartRoute) => (
        <button onClick={(e) => { e.stopPropagation(); toggleExpand(row.id) }} className="text-[var(--text-muted)] hover:text-[var(--text-primary)] transition-colors">
          {expandedRoute === row.id ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
        </button>
      ),
    },
    {
      key: 'name',
      header: t('common.name'),
      sortable: true,
      render: (row: SmartRoute) => (
        <span className="font-mono text-[var(--text-primary)]">{row.name}</span>
      ),
    },
    {
      key: 'description',
      header: t('common.description'),
      render: (row: SmartRoute) => (
        <span className="text-[var(--text-muted)] text-sm">{row.description || '—'}</span>
      ),
    },
    {
      key: 'is_active',
      header: 'Status',
      render: (row: SmartRoute) => (
        <Badge variant={row.is_active ? 'online' : 'offline'}>
          {row.is_active ? 'Active' : 'Inactive'}
        </Badge>
      ),
    },
    {
      key: 'created_at',
      header: t('common.createdAt'),
      render: (row: SmartRoute) => (
        <span className="text-[var(--text-muted)] font-mono text-xs">
          {new Date(row.created_at).toLocaleDateString()}
        </span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row: SmartRoute) => (
        <div className="flex items-center gap-2">
          <button
            onClick={(e) => { e.stopPropagation(); setShowAddEntry(row.id) }}
            className="text-[var(--text-muted)] hover:text-[var(--accent)] transition-colors"
            title={t('smartRoutes.addEntry')}
          >
            <Plus size={16} />
          </button>
          <button
            onClick={(e) => { e.stopPropagation(); setDeleteRouteId(row.id) }}
            className="text-[var(--text-muted)] hover:text-[var(--danger)] transition-colors"
          >
            <Trash2 size={16} />
          </button>
        </div>
      ),
    },
  ]

  const proxyColumns = [
    {
      key: 'name',
      header: t('common.name'),
      sortable: true,
      render: (row: ProxyServer) => (
        <span className="font-mono text-[var(--text-primary)]">{row.name}</span>
      ),
    },
    {
      key: 'type',
      header: 'Type',
      render: (row: ProxyServer) => (
        <Badge variant="info">{row.type.toUpperCase()}</Badge>
      ),
    },
    {
      key: 'address',
      header: 'Address',
      render: (row: ProxyServer) => (
        <span className="font-mono text-[var(--text-secondary)] text-sm">{row.address}:{row.port}</span>
      ),
    },
    {
      key: 'is_active',
      header: 'Status',
      render: (row: ProxyServer) => (
        <Badge variant={row.is_active ? 'online' : 'offline'}>
          {row.is_active ? 'Active' : 'Inactive'}
        </Badge>
      ),
    },
    {
      key: 'created_at',
      header: t('common.createdAt'),
      render: (row: ProxyServer) => (
        <span className="text-[var(--text-muted)] font-mono text-xs">
          {new Date(row.created_at).toLocaleDateString()}
        </span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row: ProxyServer) => (
        <button
          onClick={(e) => { e.stopPropagation(); setDeleteProxyId(row.id) }}
          className="text-[var(--text-muted)] hover:text-[var(--danger)] transition-colors"
        >
          <Trash2 size={16} />
        </button>
      ),
    },
  ]

  const entries = expandedRouteData?.entries || []

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('smartRoutes.title')}
        </h1>
        <div className="flex items-center gap-2">
          {activeTab === 'routes' && (
            <Button onClick={() => setShowCreateRoute(true)}>
              <Plus size={16} className="mr-1" /> {t('smartRoutes.createGroup')}
            </Button>
          )}
          {activeTab === 'proxies' && (
            <Button onClick={() => setShowCreateProxy(true)}>
              <Plus size={16} className="mr-1" /> {t('smartRoutes.addProxy')}
            </Button>
          )}
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 mb-4 border-b border-[var(--border)]">
        <button
          onClick={() => setActiveTab('routes')}
          className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px cursor-pointer ${
            activeTab === 'routes'
              ? 'border-[var(--accent)] text-[var(--accent)]'
              : 'border-transparent text-[var(--text-muted)] hover:text-[var(--text-primary)]'
          }`}
        >
          {t('smartRoutes.title')}
        </button>
        <button
          onClick={() => setActiveTab('proxies')}
          className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px cursor-pointer ${
            activeTab === 'proxies'
              ? 'border-[var(--accent)] text-[var(--accent)]'
              : 'border-transparent text-[var(--text-muted)] hover:text-[var(--text-primary)]'
          }`}
        >
          {t('smartRoutes.proxyServers')}
        </button>
      </div>

      {/* Routes Tab */}
      {activeTab === 'routes' && (
        <>
          {routesLoading ? (
            <Card className="p-8 text-center text-[var(--text-muted)]">{t('common.loading')}</Card>
          ) : routes.length === 0 ? (
            <Card className="flex flex-col items-center justify-center py-16">
              <div className="rounded-full p-4 mb-4" style={{ background: 'rgba(0,255,136,0.08)' }}>
                <Route size={40} className="text-[var(--accent)]" />
              </div>
              <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-2 font-mono">
                {t('smartRoutes.noRoutes')}
              </h2>
              <Button className="mt-4" onClick={() => setShowCreateRoute(true)}>
                <Plus size={16} className="mr-1" /> {t('smartRoutes.createGroup')}
              </Button>
            </Card>
          ) : (
            <div className="space-y-0">
              <Table columns={routeColumns} data={routes} />

              {/* Expanded entries */}
              {expandedRoute && entries.length > 0 && (
                <Card className="mt-2 p-4">
                  <h3 className="text-sm font-medium text-[var(--text-secondary)] mb-3 font-mono">
                    Entries ({entries.length})
                  </h3>
                  <div className="space-y-2">
                    {entries.map((entry) => (
                      <div key={entry.id} className="flex items-center gap-3 px-3 py-2 rounded-md bg-[var(--bg-tertiary)]">
                        {entryTypeBadge(entry.entry_type)}
                        <span className="font-mono text-sm text-[var(--text-primary)] flex-1">{entry.value}</span>
                        {actionBadge(entry.action)}
                        {entry.action === 'proxy' && entry.proxy_name && (
                          <span className="text-xs text-[var(--text-muted)] font-mono">via {entry.proxy_name}</span>
                        )}
                        <span className="text-xs text-[var(--text-muted)] font-mono">p:{entry.priority}</span>
                        <button
                          onClick={() => deleteEntryMutation.mutate({ routeId: expandedRoute, entryId: entry.id })}
                          className="text-[var(--text-muted)] hover:text-[var(--danger)] transition-colors"
                        >
                          <Trash2 size={14} />
                        </button>
                      </div>
                    ))}
                  </div>
                </Card>
              )}

              {expandedRoute && entries.length === 0 && expandedRouteData && (
                <Card className="mt-2 p-4 text-center text-[var(--text-muted)] text-sm">
                  No entries yet. Click <Plus size={14} className="inline" /> to add one.
                </Card>
              )}
            </div>
          )}
        </>
      )}

      {/* Proxies Tab */}
      {activeTab === 'proxies' && (
        <>
          {proxiesLoading ? (
            <Card className="p-8 text-center text-[var(--text-muted)]">{t('common.loading')}</Card>
          ) : proxies.length === 0 ? (
            <Card className="flex flex-col items-center justify-center py-16">
              <div className="rounded-full p-4 mb-4" style={{ background: 'rgba(0,255,136,0.08)' }}>
                <Route size={40} className="text-[var(--accent)]" />
              </div>
              <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-2 font-mono">
                No proxy servers configured
              </h2>
              <Button className="mt-4" onClick={() => setShowCreateProxy(true)}>
                <Plus size={16} className="mr-1" /> {t('smartRoutes.addProxy')}
              </Button>
            </Card>
          ) : (
            <Table columns={proxyColumns} data={proxies} />
          )}
        </>
      )}

      {/* Create Route Modal */}
      <Modal open={showCreateRoute} title={t('smartRoutes.createGroup')} onClose={() => setShowCreateRoute(false)}>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            createRouteMutation.mutate({
              name: routeForm.name,
              description: routeForm.description || undefined,
            })
          }}
          className="flex flex-col gap-4"
        >
          <Input
            label={t('common.name')}
            value={routeForm.name}
            onChange={(e) => setRouteForm({ ...routeForm, name: e.target.value })}
            placeholder="e.g. bypass-blocks"
            required
          />
          <Input
            label={t('common.description')}
            value={routeForm.description}
            onChange={(e) => setRouteForm({ ...routeForm, description: e.target.value })}
            placeholder="Optional description"
          />
          <div className="flex justify-end gap-2 mt-2">
            <Button variant="ghost" type="button" onClick={() => setShowCreateRoute(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={createRouteMutation.isPending}>
              {createRouteMutation.isPending ? t('common.loading') : t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delete Route Modal */}
      <Modal open={!!deleteRouteId} title={t('common.delete')} onClose={() => setDeleteRouteId(null)}>
        <p className="text-sm text-[var(--text-secondary)] mb-4">
          Are you sure you want to delete this route group and all its entries? This action cannot be undone.
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setDeleteRouteId(null)}>{t('common.cancel')}</Button>
          <Button
            variant="danger"
            onClick={() => deleteRouteMutation.mutate(deleteRouteId!)}
            disabled={deleteRouteMutation.isPending}
          >
            {deleteRouteMutation.isPending ? t('common.loading') : t('common.delete')}
          </Button>
        </div>
      </Modal>

      {/* Add Entry Modal */}
      <Modal open={!!showAddEntry} title={t('smartRoutes.addEntry')} onClose={() => setShowAddEntry(null)}>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            addEntryMutation.mutate({
              routeId: showAddEntry!,
              entry_type: entryForm.entry_type,
              value: entryForm.value,
              action: entryForm.action,
              proxy_id: entryForm.action === 'proxy' ? entryForm.proxy_id : undefined,
              priority: parseInt(entryForm.priority) || 100,
            })
          }}
          className="flex flex-col gap-4"
        >
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
              Type
            </label>
            <select
              value={entryForm.entry_type}
              onChange={(e) => setEntryForm({ ...entryForm, entry_type: e.target.value })}
              className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono"
            >
              <option value="domain">{t('smartRoutes.domain')}</option>
              <option value="cidr">{t('smartRoutes.cidr')}</option>
              <option value="domain_suffix">{t('smartRoutes.domainSuffix')}</option>
            </select>
          </div>
          <Input
            label="Value"
            value={entryForm.value}
            onChange={(e) => setEntryForm({ ...entryForm, value: e.target.value })}
            placeholder={entryForm.entry_type === 'cidr' ? 'e.g. 10.0.0.0/8' : entryForm.entry_type === 'domain_suffix' ? 'e.g. .google.com' : 'e.g. youtube.com'}
            required
          />
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
              Action
            </label>
            <select
              value={entryForm.action}
              onChange={(e) => setEntryForm({ ...entryForm, action: e.target.value })}
              className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono"
            >
              <option value="proxy">{t('smartRoutes.proxy')}</option>
              <option value="direct">{t('smartRoutes.direct')}</option>
              <option value="block">{t('smartRoutes.block')}</option>
            </select>
          </div>
          {entryForm.action === 'proxy' && (
            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
                {t('smartRoutes.proxyServers')}
              </label>
              <select
                value={entryForm.proxy_id}
                onChange={(e) => setEntryForm({ ...entryForm, proxy_id: e.target.value })}
                className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono"
                required
              >
                <option value="">Select proxy...</option>
                {proxies.map((p) => (
                  <option key={p.id} value={p.id}>{p.name} ({p.type} - {p.address}:{p.port})</option>
                ))}
              </select>
            </div>
          )}
          <Input
            label="Priority"
            type="number"
            value={entryForm.priority}
            onChange={(e) => setEntryForm({ ...entryForm, priority: e.target.value })}
            placeholder="100"
          />
          <div className="flex justify-end gap-2 mt-2">
            <Button variant="ghost" type="button" onClick={() => setShowAddEntry(null)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={addEntryMutation.isPending}>
              {addEntryMutation.isPending ? t('common.loading') : t('smartRoutes.addEntry')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Create Proxy Modal */}
      <Modal open={showCreateProxy} title={t('smartRoutes.addProxy')} onClose={() => setShowCreateProxy(false)}>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            createProxyMutation.mutate({
              name: proxyForm.name,
              type: proxyForm.type,
              address: proxyForm.address,
              port: parseInt(proxyForm.port) || 0,
            })
          }}
          className="flex flex-col gap-4"
        >
          <Input
            label={t('common.name')}
            value={proxyForm.name}
            onChange={(e) => setProxyForm({ ...proxyForm, name: e.target.value })}
            placeholder="e.g. my-socks5-proxy"
            required
          />
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
              Type
            </label>
            <select
              value={proxyForm.type}
              onChange={(e) => setProxyForm({ ...proxyForm, type: e.target.value })}
              className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono"
            >
              <option value="socks5">SOCKS5</option>
              <option value="http">HTTP</option>
              <option value="shadowsocks">Shadowsocks</option>
              <option value="vless">VLESS</option>
            </select>
          </div>
          <Input
            label="Address"
            value={proxyForm.address}
            onChange={(e) => setProxyForm({ ...proxyForm, address: e.target.value })}
            placeholder="e.g. 203.0.113.1"
            required
          />
          <Input
            label="Port"
            type="number"
            value={proxyForm.port}
            onChange={(e) => setProxyForm({ ...proxyForm, port: e.target.value })}
            placeholder="e.g. 1080"
            required
          />
          <div className="flex justify-end gap-2 mt-2">
            <Button variant="ghost" type="button" onClick={() => setShowCreateProxy(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={createProxyMutation.isPending}>
              {createProxyMutation.isPending ? t('common.loading') : t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delete Proxy Modal */}
      <Modal open={!!deleteProxyId} title={t('common.delete')} onClose={() => setDeleteProxyId(null)}>
        <p className="text-sm text-[var(--text-secondary)] mb-4">
          Are you sure you want to delete this proxy server? This action cannot be undone.
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setDeleteProxyId(null)}>{t('common.cancel')}</Button>
          <Button
            variant="danger"
            onClick={() => deleteProxyMutation.mutate(deleteProxyId!)}
            disabled={deleteProxyMutation.isPending}
          >
            {deleteProxyMutation.isPending ? t('common.loading') : t('common.delete')}
          </Button>
        </div>
      </Modal>
    </div>
  )
}
