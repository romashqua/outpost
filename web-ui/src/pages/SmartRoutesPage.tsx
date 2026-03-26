import { useTranslation } from 'react-i18next'
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2, Route, ChevronDown, ChevronRight, Pencil, Globe, AlertTriangle, Info, Link2 } from 'lucide-react'
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

interface Network {
  id: string
  name: string
  address: string
}

interface RouteNetwork {
  network_id: string
  smart_route_id: string
  network_name?: string
  network_address?: string
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
  const [showAddNetwork, setShowAddNetwork] = useState<string | null>(null)
  const [deleteRouteId, setDeleteRouteId] = useState<string | null>(null)
  const [deleteProxyId, setDeleteProxyId] = useState<string | null>(null)
  const [editRouteTarget, setEditRouteTarget] = useState<SmartRoute | null>(null)
  const [editRouteForm, setEditRouteForm] = useState({ name: '', description: '' })
  const [expandedRoute, setExpandedRoute] = useState<string | null>(null)
  const [selectedNetworkId, setSelectedNetworkId] = useState('')

  const [routeForm, setRouteForm] = useState({ name: '', description: '' })
  const [proxyForm, setProxyForm] = useState({ name: '', type: 'socks5', address: '', port: '' })
  const [entryForm, setEntryForm] = useState({ entry_type: 'domain', value: '', action: 'direct', proxy_id: '', priority: '100' })

  // --- Data fetching ---

  const { data: routes = [], isLoading: routesLoading } = useQuery<SmartRoute[]>({
    queryKey: ['smart-routes'],
    queryFn: () => api.get('/smart-routes'),
  })

  const { data: proxies = [], isLoading: proxiesLoading } = useQuery<ProxyServer[]>({
    queryKey: ['proxy-servers'],
    queryFn: () => api.get('/smart-routes/proxy-servers'),
  })

  const { data: expandedRouteData, isLoading: expandedLoading } = useQuery<SmartRoute>({
    queryKey: ['smart-routes', expandedRoute],
    queryFn: () => api.get(`/smart-routes/${expandedRoute}`),
    enabled: !!expandedRoute,
  })

  const { data: networks = [] } = useQuery<Network[]>({
    queryKey: ['networks'],
    queryFn: () => api.get('/networks'),
  })

  const { data: routeNetworks = [] } = useQuery<RouteNetwork[]>({
    queryKey: ['smart-routes', expandedRoute, 'networks'],
    queryFn: () => api.get(`/smart-routes/${expandedRoute}/networks`),
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
      addToast(t('smartRoutes.routeCreated'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const updateRouteMutation = useMutation({
    mutationFn: (payload: { id: string; name: string; description?: string }) => {
      const { id, ...rest } = payload
      return api.put(`/smart-routes/${id}`, rest)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['smart-routes'] })
      setEditRouteTarget(null)
      addToast(t('smartRoutes.routeUpdated'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const deleteRouteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/smart-routes/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['smart-routes'] })
      setDeleteRouteId(null)
      setExpandedRoute(null)
      addToast(t('smartRoutes.routeDeleted'), 'success')
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
      addToast(t('smartRoutes.proxyCreated'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const deleteProxyMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/smart-routes/proxy-servers/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['proxy-servers'] })
      setDeleteProxyId(null)
      addToast(t('smartRoutes.proxyDeleted'), 'success')
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
      setEntryForm({ entry_type: 'domain', value: '', action: 'direct', proxy_id: '', priority: '100' })
      addToast(t('smartRoutes.entryAdded'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const deleteEntryMutation = useMutation({
    mutationFn: ({ routeId, entryId }: { routeId: string; entryId: string }) =>
      api.delete(`/smart-routes/${routeId}/entries/${entryId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['smart-routes'] })
      addToast(t('smartRoutes.entryRemoved'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const addNetworkMutation = useMutation({
    mutationFn: ({ routeId, networkId }: { routeId: string; networkId: string }) =>
      api.post(`/smart-routes/${routeId}/networks`, { network_id: networkId }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['smart-routes', expandedRoute, 'networks'] })
      setShowAddNetwork(null)
      setSelectedNetworkId('')
      addToast(t('smartRoutes.networkAdded'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const removeNetworkMutation = useMutation({
    mutationFn: ({ routeId, networkId }: { routeId: string; networkId: string }) =>
      api.delete(`/smart-routes/${routeId}/networks/${networkId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['smart-routes', expandedRoute, 'networks'] })
      addToast(t('smartRoutes.networkRemoved'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  // --- Helpers ---

  const entryTypeBadge = (type: string) => {
    switch (type) {
      case 'domain': return <Badge variant="info">{t('smartRoutes.domain')}</Badge>
      case 'cidr': return <Badge variant="pending">{t('smartRoutes.cidr')}</Badge>
      case 'domain_suffix': return <Badge variant="online">{t('smartRoutes.domainSuffix')}</Badge>
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

  const actionHint = (action: string) => {
    switch (action) {
      case 'proxy': return t('smartRoutes.actionHintProxy')
      case 'direct': return t('smartRoutes.actionHintDirect')
      case 'block': return t('smartRoutes.actionHintBlock')
      default: return ''
    }
  }

  const entryTypeHint = (type: string) => {
    switch (type) {
      case 'domain': return t('smartRoutes.entryTypeHintDomain')
      case 'cidr': return t('smartRoutes.entryTypeHintCidr')
      case 'domain_suffix': return t('smartRoutes.entryTypeHintSuffix')
      default: return ''
    }
  }

  const linkedNetworkIds = new Set(routeNetworks.map((rn) => rn.network_id))
  const availableNetworks = networks.filter((n) => !linkedNetworkIds.has(n.id))

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
      header: t('common.status'),
      render: (row: SmartRoute) => (
        <Badge variant={row.is_active ? 'online' : 'offline'}>
          {row.is_active ? t('status.active') : t('status.inactive')}
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
            onClick={(e) => { e.stopPropagation(); addEntryMutation.reset(); setShowAddEntry(row.id) }}
            className="text-[var(--text-muted)] hover:text-[var(--accent)] transition-colors"
            title={t('smartRoutes.addEntry')}
          >
            <Plus size={16} />
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation()
              updateRouteMutation.reset()
              setEditRouteForm({ name: row.name, description: row.description || '' })
              setEditRouteTarget(row)
            }}
            className="text-[var(--text-muted)] hover:text-[var(--accent)] transition-colors"
            title={t('common.edit')}
          >
            <Pencil size={16} />
          </button>
          <button
            onClick={(e) => { e.stopPropagation(); deleteRouteMutation.reset(); setDeleteRouteId(row.id) }}
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
      header: t('smartRoutes.type'),
      render: (row: ProxyServer) => (
        <Badge variant="info">{row.type.toUpperCase()}</Badge>
      ),
    },
    {
      key: 'address',
      header: t('smartRoutes.address'),
      render: (row: ProxyServer) => (
        <span className="font-mono text-[var(--text-secondary)] text-sm">{row.address}:{row.port}</span>
      ),
    },
    {
      key: 'is_active',
      header: t('common.status'),
      render: (row: ProxyServer) => (
        <Badge variant={row.is_active ? 'online' : 'offline'}>
          {row.is_active ? t('status.active') : t('status.inactive')}
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
          onClick={(e) => { e.stopPropagation(); deleteProxyMutation.reset(); setDeleteProxyId(row.id) }}
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
            <Button onClick={() => { createRouteMutation.reset(); setShowCreateRoute(true) }}>
              <Plus size={16} className="mr-1" /> {t('smartRoutes.createGroup')}
            </Button>
          )}
          {activeTab === 'proxies' && (
            <Button onClick={() => { createProxyMutation.reset(); setShowCreateProxy(true) }}>
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
              <Button className="mt-4" onClick={() => { createRouteMutation.reset(); setShowCreateRoute(true) }}>
                <Plus size={16} className="mr-1" /> {t('smartRoutes.createGroup')}
              </Button>
            </Card>
          ) : (
            <div className="space-y-0">
              <Table columns={routeColumns} data={routes} />

              {/* Expanded: entries + networks */}
              {expandedRoute && expandedLoading && (
                <Card className="mt-2 p-4 text-center text-[var(--text-muted)] text-sm">{t('common.loading')}</Card>
              )}

              {expandedRoute && expandedRouteData && !expandedLoading && (
                <Card className="mt-2 p-4 space-y-4">
                  {/* Linked networks */}
                  <div>
                    <div className="flex items-center justify-between mb-2">
                      <h3 className="text-sm font-medium text-[var(--text-secondary)] font-mono flex items-center gap-1.5">
                        <Link2 size={14} />
                        {t('smartRoutes.networks')} ({routeNetworks.length})
                      </h3>
                      <button
                        onClick={() => { addNetworkMutation.reset(); setShowAddNetwork(expandedRoute) }}
                        className="text-xs text-[var(--accent)] hover:underline font-mono"
                      >
                        + {t('smartRoutes.addNetwork')}
                      </button>
                    </div>
                    {routeNetworks.length === 0 ? (
                      <div className="flex items-center gap-2 px-3 py-2 rounded-md bg-[var(--bg-tertiary)] border border-amber-500/30">
                        <AlertTriangle size={14} className="text-amber-500 shrink-0" />
                        <span className="text-xs text-amber-400">{t('smartRoutes.noNetworks')}</span>
                      </div>
                    ) : (
                      <div className="flex flex-wrap gap-2">
                        {routeNetworks.map((rn) => (
                          <div key={rn.network_id} className="flex items-center gap-2 px-3 py-1.5 rounded-md bg-[var(--bg-tertiary)] group">
                            <Globe size={12} className="text-[var(--accent)]" />
                            <span className="font-mono text-xs text-[var(--text-primary)]">
                              {rn.network_name || rn.network_id}
                            </span>
                            {rn.network_address && (
                              <span className="text-xs text-[var(--text-muted)] font-mono">{rn.network_address}</span>
                            )}
                            <button
                              onClick={() => removeNetworkMutation.mutate({ routeId: expandedRoute!, networkId: rn.network_id })}
                              className="text-[var(--text-muted)] hover:text-[var(--danger)] transition-colors opacity-0 group-hover:opacity-100"
                            >
                              <Trash2 size={12} />
                            </button>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>

                  {/* Entries */}
                  <div>
                    <h3 className="text-sm font-medium text-[var(--text-secondary)] mb-2 font-mono">
                      {t('smartRoutes.entries')} ({entries.length})
                    </h3>
                    {entries.length === 0 ? (
                      <p className="text-center text-[var(--text-muted)] text-sm py-2">
                        {t('smartRoutes.noEntriesClickAdd')}
                      </p>
                    ) : (
                      <div className="space-y-2">
                        {entries.map((entry) => (
                          <div key={entry.id} className="flex items-center gap-3 px-3 py-2 rounded-md bg-[var(--bg-tertiary)]">
                            {entryTypeBadge(entry.entry_type)}
                            <span className="font-mono text-sm text-[var(--text-primary)] flex-1">{entry.value}</span>
                            {actionBadge(entry.action)}
                            {entry.action === 'proxy' && entry.proxy_name && (
                              <span className="text-xs text-[var(--text-muted)] font-mono">{t('smartRoutes.via')} {entry.proxy_name}</span>
                            )}
                            <span className="text-xs text-[var(--text-muted)] font-mono">p:{entry.priority}</span>
                            <button
                              onClick={() => deleteEntryMutation.mutate({ routeId: expandedRoute!, entryId: entry.id })}
                              className="text-[var(--text-muted)] hover:text-[var(--danger)] transition-colors"
                            >
                              <Trash2 size={14} />
                            </button>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
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
                {t('smartRoutes.noProxies')}
              </h2>
              <Button className="mt-4" onClick={() => { createProxyMutation.reset(); setShowCreateProxy(true) }}>
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

      {/* Edit Route Modal */}
      <Modal open={!!editRouteTarget} title={t('smartRoutes.editRoute')} onClose={() => setEditRouteTarget(null)}>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            if (editRouteTarget) updateRouteMutation.mutate({
              id: editRouteTarget.id,
              name: editRouteForm.name,
              description: editRouteForm.description || undefined,
            })
          }}
          className="flex flex-col gap-4"
        >
          <Input
            label={t('common.name')}
            value={editRouteForm.name}
            onChange={(e) => setEditRouteForm({ ...editRouteForm, name: e.target.value })}
            placeholder="e.g. bypass-blocks"
            required
          />
          <Input
            label={t('common.description')}
            value={editRouteForm.description}
            onChange={(e) => setEditRouteForm({ ...editRouteForm, description: e.target.value })}
            placeholder="Optional description"
          />
          {updateRouteMutation.isError && (
            <p className="text-sm text-[var(--danger)]">
              {(updateRouteMutation.error as Error).message}
            </p>
          )}
          <div className="flex justify-end gap-2 mt-2">
            <Button variant="ghost" type="button" onClick={() => setEditRouteTarget(null)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={updateRouteMutation.isPending}>
              {updateRouteMutation.isPending ? t('common.loading') : t('common.save')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delete Route Modal */}
      <Modal open={!!deleteRouteId} title={t('common.delete')} onClose={() => setDeleteRouteId(null)}>
        <p className="text-sm text-[var(--text-secondary)] mb-4">
          {t('smartRoutes.confirmDeleteRoute')}
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
          {/* Entry type with hint */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
              {t('smartRoutes.type')}
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
            <p className="text-xs text-[var(--text-muted)] flex items-start gap-1">
              <Info size={12} className="shrink-0 mt-0.5 text-[var(--accent)]" />
              {entryTypeHint(entryForm.entry_type)}
            </p>
          </div>

          {/* Value */}
          <Input
            label={t('smartRoutes.value')}
            value={entryForm.value}
            onChange={(e) => setEntryForm({ ...entryForm, value: e.target.value })}
            placeholder={entryForm.entry_type === 'cidr' ? 'e.g. 10.0.0.0/8' : entryForm.entry_type === 'domain_suffix' ? 'e.g. .google.com' : 'e.g. youtube.com'}
            required
          />

          {/* Action with hint */}
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
              {t('smartRoutes.action')}
            </label>
            <select
              value={entryForm.action}
              onChange={(e) => setEntryForm({ ...entryForm, action: e.target.value })}
              className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono"
            >
              <option value="direct">{t('smartRoutes.direct')}</option>
              <option value="proxy">{t('smartRoutes.proxy')}</option>
              <option value="block">{t('smartRoutes.block')}</option>
            </select>
            <p className={`text-xs flex items-start gap-1 ${
              entryForm.action === 'block' ? 'text-[var(--danger)]' :
              entryForm.action === 'proxy' ? 'text-blue-400' :
              'text-emerald-400'
            }`}>
              <Info size={12} className="shrink-0 mt-0.5" />
              {actionHint(entryForm.action)}
            </p>
          </div>

          {/* Proxy selector (only when action=proxy) */}
          {entryForm.action === 'proxy' && (
            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
                {t('smartRoutes.proxyServers')}
              </label>
              {proxies.length === 0 ? (
                <div className="flex items-center gap-2 px-3 py-2 rounded-md bg-[var(--bg-tertiary)] border border-amber-500/30">
                  <AlertTriangle size={14} className="text-amber-500 shrink-0" />
                  <span className="text-xs text-amber-400">{t('smartRoutes.noProxiesCreateFirst')}</span>
                </div>
              ) : (
                <select
                  value={entryForm.proxy_id}
                  onChange={(e) => setEntryForm({ ...entryForm, proxy_id: e.target.value })}
                  className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono"
                  required
                >
                  <option value="">{t('smartRoutes.selectProxy')}</option>
                  {proxies.filter(p => p.is_active).map((p) => (
                    <option key={p.id} value={p.id}>{p.name} ({p.type.toUpperCase()} — {p.address}:{p.port})</option>
                  ))}
                </select>
              )}
            </div>
          )}

          {/* Priority with hint */}
          <div className="flex flex-col gap-1.5">
            <Input
              label={t('smartRoutes.priority')}
              type="number"
              value={entryForm.priority}
              onChange={(e) => setEntryForm({ ...entryForm, priority: e.target.value })}
              placeholder="100"
            />
            <p className="text-xs text-[var(--text-muted)] flex items-start gap-1">
              <Info size={12} className="shrink-0 mt-0.5 text-[var(--accent)]" />
              {t('smartRoutes.priorityHint')}
            </p>
          </div>

          {addEntryMutation.isError && (
            <p className="text-sm text-[var(--danger)]">
              {(addEntryMutation.error as Error).message}
            </p>
          )}
          <div className="flex justify-end gap-2 mt-2">
            <Button variant="ghost" type="button" onClick={() => setShowAddEntry(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              type="submit"
              disabled={addEntryMutation.isPending || (entryForm.action === 'proxy' && proxies.length === 0)}
            >
              {addEntryMutation.isPending ? t('common.loading') : t('smartRoutes.addEntry')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Add Network Modal */}
      <Modal open={!!showAddNetwork} title={t('smartRoutes.addNetwork')} onClose={() => setShowAddNetwork(null)}>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            if (showAddNetwork && selectedNetworkId) {
              addNetworkMutation.mutate({ routeId: showAddNetwork, networkId: selectedNetworkId })
            }
          }}
          className="flex flex-col gap-4"
        >
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
              {t('smartRoutes.selectNetwork')}
            </label>
            {availableNetworks.length === 0 ? (
              <p className="text-sm text-[var(--text-muted)]">{t('smartRoutes.noNetworks')}</p>
            ) : (
              <select
                value={selectedNetworkId}
                onChange={(e) => setSelectedNetworkId(e.target.value)}
                className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono"
                required
              >
                <option value="">{t('smartRoutes.selectNetwork')}</option>
                {availableNetworks.map((n) => (
                  <option key={n.id} value={n.id}>{n.name} ({n.address})</option>
                ))}
              </select>
            )}
          </div>
          <div className="flex justify-end gap-2 mt-2">
            <Button variant="ghost" type="button" onClick={() => setShowAddNetwork(null)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={addNetworkMutation.isPending || !selectedNetworkId}>
              {addNetworkMutation.isPending ? t('common.loading') : t('smartRoutes.addNetwork')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Create Proxy Modal */}
      <Modal open={showCreateProxy} title={t('smartRoutes.addProxy')} onClose={() => setShowCreateProxy(false)}>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            const port = parseInt(proxyForm.port)
            if (!port || port < 1 || port > 65535) {
              addToast(t('smartRoutes.invalidPort', 'Port must be between 1 and 65535'), 'error')
              return
            }
            createProxyMutation.mutate({
              name: proxyForm.name,
              type: proxyForm.type,
              address: proxyForm.address,
              port,
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
              {t('smartRoutes.type')}
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
            label={t('smartRoutes.address')}
            value={proxyForm.address}
            onChange={(e) => setProxyForm({ ...proxyForm, address: e.target.value })}
            placeholder="e.g. 203.0.113.1"
            required
          />
          <Input
            label={t('smartRoutes.port')}
            type="number"
            min={1}
            max={65535}
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
          {t('smartRoutes.confirmDeleteProxy')}
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
