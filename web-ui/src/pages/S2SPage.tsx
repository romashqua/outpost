import { useTranslation } from 'react-i18next'
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2, Network, Globe, Route, Server, Download } from 'lucide-react'
import { api } from '@/api/client'
import { useToastStore } from '@/store/toast'
import Card from '@/components/ui/Card'
import Button from '@/components/ui/Button'
import Badge from '@/components/ui/Badge'
import Table from '@/components/ui/Table'
import Modal from '@/components/ui/Modal'
import Input from '@/components/ui/Input'

interface S2STunnel {
  id: string
  name: string
  description: string
  topology: string
  hub_gateway_id: string | null
  is_active: boolean
  created_at: string
  updated_at: string
}

interface Gateway {
  id: string
  name: string
  endpoint?: string
  public_ip?: string
}

interface TunnelMember {
  id?: string
  gateway_id: string
  gateway_name?: string
  name?: string
}

interface TunnelRoute {
  id: string
  cidr: string
  description?: string
}

interface AllowedDomain {
  id: string
  domain: string
}

function TunnelDetailPanel({
  tunnel,
  onClose,
}: {
  tunnel: S2STunnel
  onClose: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)

  const [showAddMember, setShowAddMember] = useState(false)
  const [selectedGatewayId, setSelectedGatewayId] = useState('')
  const [showAddRoute, setShowAddRoute] = useState(false)
  const [routeForm, setRouteForm] = useState({ cidr: '', description: '' })
  const [showAddDomain, setShowAddDomain] = useState(false)
  const [domainName, setDomainName] = useState('')
  const [domains, setDomains] = useState<AllowedDomain[]>([])
  const [domainsApiUnavailable, setDomainsApiUnavailable] = useState(false)

  // Fetch members
  const { data: members = [], isLoading: membersLoading } = useQuery<TunnelMember[]>({
    queryKey: ['s2s-tunnels', tunnel.id, 'members'],
    queryFn: () => api.get(`/s2s-tunnels/${tunnel.id}/members`),
  })

  // Fetch routes
  const { data: routes = [], isLoading: routesLoading } = useQuery<TunnelRoute[]>({
    queryKey: ['s2s-tunnels', tunnel.id, 'routes'],
    queryFn: () => api.get(`/s2s-tunnels/${tunnel.id}/routes`),
  })

  // Fetch available gateways
  const { data: gateways = [] } = useQuery<Gateway[]>({
    queryKey: ['gateways'],
    queryFn: () => api.get('/gateways'),
  })

  // Add member mutation
  const addMemberMutation = useMutation({
    mutationFn: (gatewayId: string) =>
      api.post(`/s2s-tunnels/${tunnel.id}/members`, { gateway_id: gatewayId }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['s2s-tunnels', tunnel.id, 'members'] })
      setShowAddMember(false)
      setSelectedGatewayId('')
      addToast(t('s2s.addMember'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  // Remove member mutation
  const removeMemberMutation = useMutation({
    mutationFn: (gatewayId: string) =>
      api.delete(`/s2s-tunnels/${tunnel.id}/members/${gatewayId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['s2s-tunnels', tunnel.id, 'members'] })
      addToast(t('s2s.removeMember'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  // Add route mutation
  const addRouteMutation = useMutation({
    mutationFn: (data: { cidr: string; description?: string }) =>
      api.post(`/s2s-tunnels/${tunnel.id}/routes`, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['s2s-tunnels', tunnel.id, 'routes'] })
      setShowAddRoute(false)
      setRouteForm({ cidr: '', description: '' })
      addToast(t('s2s.addRoute'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  // Remove route mutation
  const removeRouteMutation = useMutation({
    mutationFn: (routeId: string) =>
      api.delete(`/s2s-tunnels/${tunnel.id}/routes/${routeId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['s2s-tunnels', tunnel.id, 'routes'] })
      addToast(t('s2s.removeRoute'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  // Filter out gateways already in the tunnel
  const memberGatewayIds = new Set(members.map((m) => m.gateway_id))
  const availableGateways = gateways.filter((g) => !memberGatewayIds.has(g.id))

  const handleAddDomain = () => {
    if (!domainName.trim()) return
    if (domainsApiUnavailable) {
      // Local-only mode
      setDomains((prev) => [
        ...prev,
        { id: crypto.randomUUID(), domain: domainName.trim() },
      ])
      setDomainName('')
      setShowAddDomain(false)
      return
    }
    // Try the API; fall back to local state on 404
    api
      .post<AllowedDomain>(`/s2s-tunnels/${tunnel.id}/domains`, {
        domain: domainName.trim(),
      })
      .then((newDomain) => {
        setDomains((prev) => [...prev, newDomain])
        setDomainName('')
        setShowAddDomain(false)
        addToast(t('s2s.addDomain'), 'success')
      })
      .catch((err) => {
        if ((err as Error).message?.includes('404') || (err as Error).message?.includes('Not Found')) {
          setDomainsApiUnavailable(true)
          // Still add locally
          setDomains((prev) => [
            ...prev,
            { id: crypto.randomUUID(), domain: domainName.trim() },
          ])
          setDomainName('')
          setShowAddDomain(false)
        } else {
          addToast((err as Error).message, 'error')
        }
      })
  }

  const handleRemoveDomain = (domainId: string) => {
    if (domainsApiUnavailable) {
      setDomains((prev) => prev.filter((d) => d.id !== domainId))
      return
    }
    api
      .delete(`/s2s-tunnels/${tunnel.id}/domains/${domainId}`)
      .then(() => {
        setDomains((prev) => prev.filter((d) => d.id !== domainId))
        addToast(t('s2s.removeDomain'), 'success')
      })
      .catch((err) => {
        if ((err as Error).message?.includes('404') || (err as Error).message?.includes('Not Found')) {
          setDomainsApiUnavailable(true)
          setDomains((prev) => prev.filter((d) => d.id !== domainId))
        } else {
          addToast((err as Error).message, 'error')
        }
      })
  }

  const sectionHeader = 'text-sm font-semibold uppercase tracking-wider text-[var(--text-muted)] mb-2 flex items-center gap-2'
  const listItem = 'flex items-center justify-between px-3 py-2 rounded-md border border-[var(--border)] bg-[var(--bg-secondary)]'

  return (
    <Modal
      open
      title={t('s2s.tunnelDetails')}
      onClose={onClose}
      className="max-w-2xl max-h-[85vh] overflow-y-auto"
    >
      <div className="flex flex-col gap-6">
        {/* Tunnel info */}
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-3">
            <span className="font-mono text-lg text-[var(--text-primary)]">{tunnel.name}</span>
            <Badge variant={tunnel.is_active ? 'online' : 'offline'}>
              {tunnel.is_active ? t('status.active') : t('status.inactive')}
            </Badge>
            <Badge variant={tunnel.topology === 'mesh' ? 'online' : 'pending'}>
              {tunnel.topology === 'mesh' ? t('s2s.mesh') : t('s2s.hubSpoke')}
            </Badge>
          </div>
          {tunnel.description && (
            <p className="text-sm text-[var(--text-muted)]">{tunnel.description}</p>
          )}
          <p className="text-xs text-[var(--text-muted)] font-mono">
            {t('s2s.created')}: {new Date(tunnel.created_at).toLocaleString()}
          </p>
        </div>

        <hr className="border-[var(--border)]" />

        {/* Members section */}
        <div>
          <div className={sectionHeader}>
            <Server size={14} />
            {t('s2s.members')}
          </div>
          {membersLoading ? (
            <p className="text-sm text-[var(--text-muted)]">{t('common.loading')}</p>
          ) : members.length === 0 ? (
            <p className="text-sm text-[var(--text-muted)] italic">{t('s2s.noMembers')}</p>
          ) : (
            <div className="flex flex-col gap-2">
              {members.map((member) => (
                <div key={member.gateway_id} className={listItem}>
                  <span className="text-sm font-mono text-[var(--text-primary)] flex-1">
                    {member.gateway_name || member.name || member.gateway_id}
                  </span>
                  <a
                    href={`/api/v1/s2s-tunnels/${tunnel.id}/config/${member.gateway_id}`}
                    download={`${tunnel.name}-${member.gateway_name || member.gateway_id}.conf`}
                    className="text-[var(--text-muted)] hover:text-[var(--accent)] transition-colors"
                    title={t('s2s.downloadConfig')}
                  >
                    <Download size={14} />
                  </a>
                  <button
                    onClick={() => removeMemberMutation.mutate(member.gateway_id)}
                    disabled={removeMemberMutation.isPending}
                    className="text-[var(--text-muted)] hover:text-[var(--danger)] transition-colors"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              ))}
            </div>
          )}
          {showAddMember ? (
            <div className="mt-2 flex items-center gap-2">
              <select
                value={selectedGatewayId}
                onChange={(e) => setSelectedGatewayId(e.target.value)}
                className="flex-1 rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono"
              >
                <option value="">{t('s2s.selectGateway')}</option>
                {availableGateways.map((gw) => (
                  <option key={gw.id} value={gw.id}>
                    {gw.name}
                  </option>
                ))}
              </select>
              <Button
                size="sm"
                disabled={!selectedGatewayId || addMemberMutation.isPending}
                onClick={() => addMemberMutation.mutate(selectedGatewayId)}
              >
                {addMemberMutation.isPending ? t('s2s.adding') : t('common.create')}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setShowAddMember(false)}>
                {t('common.cancel')}
              </Button>
            </div>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              className="mt-2"
              onClick={() => {
                addMemberMutation.reset()
                setShowAddMember(true)
              }}
            >
              <Plus size={14} className="mr-1" /> {t('s2s.addMember')}
            </Button>
          )}
        </div>

        <hr className="border-[var(--border)]" />

        {/* Routes section */}
        <div>
          <div className={sectionHeader}>
            <Route size={14} />
            {t('s2s.routes')}
          </div>
          {routesLoading ? (
            <p className="text-sm text-[var(--text-muted)]">{t('common.loading')}</p>
          ) : routes.length === 0 ? (
            <p className="text-sm text-[var(--text-muted)] italic">{t('s2s.noRoutes')}</p>
          ) : (
            <div className="flex flex-col gap-2">
              {routes.map((route) => (
                <div key={route.id} className={listItem}>
                  <div className="flex flex-col">
                    <span className="text-sm font-mono text-[var(--text-primary)]">
                      {route.cidr}
                    </span>
                    {route.description && (
                      <span className="text-xs text-[var(--text-muted)]">
                        {route.description}
                      </span>
                    )}
                  </div>
                  <button
                    onClick={() => removeRouteMutation.mutate(route.id)}
                    disabled={removeRouteMutation.isPending}
                    className="text-[var(--text-muted)] hover:text-[var(--danger)] transition-colors"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              ))}
            </div>
          )}
          {showAddRoute ? (
            <form
              className="mt-2 flex flex-col gap-2"
              onSubmit={(e) => {
                e.preventDefault()
                addRouteMutation.mutate({
                  cidr: routeForm.cidr,
                  description: routeForm.description || undefined,
                })
              }}
            >
              <div className="flex items-center gap-2">
                <Input
                  placeholder={t('s2s.routeCidr')}
                  value={routeForm.cidr}
                  onChange={(e) => setRouteForm({ ...routeForm, cidr: e.target.value })}
                  required
                  className="flex-1"
                />
                <Input
                  placeholder={t('s2s.routeDescription')}
                  value={routeForm.description}
                  onChange={(e) => setRouteForm({ ...routeForm, description: e.target.value })}
                  className="flex-1"
                />
              </div>
              <div className="flex gap-2">
                <Button size="sm" type="submit" disabled={addRouteMutation.isPending}>
                  {addRouteMutation.isPending ? t('s2s.adding') : t('common.create')}
                </Button>
                <Button variant="ghost" size="sm" type="button" onClick={() => setShowAddRoute(false)}>
                  {t('common.cancel')}
                </Button>
              </div>
            </form>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              className="mt-2"
              onClick={() => {
                addRouteMutation.reset()
                setShowAddRoute(true)
              }}
            >
              <Plus size={14} className="mr-1" /> {t('s2s.addRoute')}
            </Button>
          )}
        </div>

        <hr className="border-[var(--border)]" />

        {/* Allowed Domains section */}
        <div>
          <div className={sectionHeader}>
            <Globe size={14} />
            {t('s2s.allowedDomains')}
          </div>
          {domainsApiUnavailable && (
            <p className="text-xs text-[var(--accent)] bg-[var(--accent-glow)] rounded px-2 py-1 mb-2">
              {t('s2s.comingSoon')}
            </p>
          )}
          {domains.length === 0 ? (
            <p className="text-sm text-[var(--text-muted)] italic">{t('s2s.noDomains')}</p>
          ) : (
            <div className="flex flex-col gap-2">
              {domains.map((d) => (
                <div key={d.id} className={listItem}>
                  <span className="text-sm font-mono text-[var(--text-primary)]">
                    {d.domain}
                  </span>
                  <button
                    onClick={() => handleRemoveDomain(d.id)}
                    className="text-[var(--text-muted)] hover:text-[var(--danger)] transition-colors"
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              ))}
            </div>
          )}
          {showAddDomain ? (
            <div className="mt-2 flex items-center gap-2">
              <Input
                placeholder={t('s2s.domainName')}
                value={domainName}
                onChange={(e) => setDomainName(e.target.value)}
                className="flex-1"
              />
              <Button
                size="sm"
                disabled={!domainName.trim()}
                onClick={handleAddDomain}
              >
                {t('common.create')}
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setShowAddDomain(false)}>
                {t('common.cancel')}
              </Button>
            </div>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              className="mt-2"
              onClick={() => setShowAddDomain(true)}
            >
              <Plus size={14} className="mr-1" /> {t('s2s.addDomain')}
            </Button>
          )}
        </div>

        {/* Close button at bottom */}
        <div className="flex justify-end pt-2">
          <Button variant="ghost" onClick={onClose}>
            {t('common.done')}
          </Button>
        </div>
      </div>
    </Modal>
  )
}

export default function S2SPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)
  const [showCreate, setShowCreate] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [selectedTunnel, setSelectedTunnel] = useState<S2STunnel | null>(null)
  const [form, setForm] = useState({ name: '', topology: 'mesh', description: '' })

  const { data: tunnels = [], isLoading, error } = useQuery<S2STunnel[]>({
    queryKey: ['s2s-tunnels'],
    queryFn: () => api.get('/s2s-tunnels'),
  })

  const createMutation = useMutation({
    mutationFn: (data: { name: string; topology: string; description?: string }) =>
      api.post('/s2s-tunnels', data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['s2s-tunnels'] })
      setShowCreate(false)
      setForm({ name: '', topology: 'mesh', description: '' })
      addToast(t('s2s.createTunnel'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/s2s-tunnels/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['s2s-tunnels'] })
      setDeleteId(null)
      addToast(t('s2s.deleteTunnel'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const columns = [
    {
      key: 'name',
      header: t('common.name'),
      sortable: true,
      render: (row: S2STunnel) => (
        <span className="font-mono text-[var(--text-primary)]">{row.name}</span>
      ),
    },
    {
      key: 'topology',
      header: t('s2s.topology'),
      render: (row: S2STunnel) => (
        <Badge variant={row.topology === 'mesh' ? 'online' : 'pending'}>
          {row.topology === 'mesh' ? t('s2s.mesh') : t('s2s.hubSpoke')}
        </Badge>
      ),
    },
    {
      key: 'is_active',
      header: t('common.status'),
      render: (row: S2STunnel) => (
        <Badge variant={row.is_active ? 'online' : 'offline'}>
          {row.is_active ? t('status.active') : t('status.inactive')}
        </Badge>
      ),
    },
    {
      key: 'created_at',
      header: t('s2s.created'),
      render: (row: S2STunnel) => (
        <span className="text-[var(--text-muted)] font-mono text-xs">
          {new Date(row.created_at).toLocaleDateString()}
        </span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row: S2STunnel) => (
        <button
          onClick={(e) => { e.stopPropagation(); deleteMutation.reset(); setDeleteId(row.id) }}
          className="text-[var(--text-muted)] hover:text-[var(--danger)] transition-colors"
        >
          <Trash2 size={16} />
        </button>
      ),
    },
  ]

  if (error) {
    return (
      <Card className="p-8 text-center text-[var(--danger)]">
        {t('s2s.failedToLoad')}
      </Card>
    )
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('s2s.title')}
        </h1>
        <Button onClick={() => { createMutation.reset(); setShowCreate(true) }}>
          <Plus size={16} className="mr-1" /> {t('s2s.newTunnel')}
        </Button>
      </div>

      {isLoading ? (
        <Card className="p-8 text-center text-[var(--text-muted)]">{t('common.loading')}</Card>
      ) : tunnels.length === 0 ? (
        <Card className="flex flex-col items-center justify-center py-16">
          <div className="rounded-full p-4 mb-4" style={{ background: 'rgba(0,255,136,0.08)' }}>
            <Network size={40} className="text-[var(--accent)]" />
          </div>
          <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-2 font-mono">
            {t('s2s.noTunnels')}
          </h2>
          <p className="text-sm text-[var(--text-muted)] text-center max-w-md">
            {t('s2s.noTunnelsDescription')}
          </p>
          <Button className="mt-4" onClick={() => { createMutation.reset(); setShowCreate(true) }}>
            <Plus size={16} className="mr-1" /> {t('s2s.createFirstTunnel')}
          </Button>
        </Card>
      ) : (
        <Table
          columns={columns}
          data={tunnels}
          onRowClick={(row) => setSelectedTunnel(row)}
        />
      )}

      {/* Tunnel detail panel */}
      {selectedTunnel && (
        <TunnelDetailPanel
          tunnel={selectedTunnel}
          onClose={() => setSelectedTunnel(null)}
        />
      )}

      <Modal open={showCreate} title={t('s2s.createS2STunnel')} onClose={() => setShowCreate(false)}>
          <form
            onSubmit={(e) => {
              e.preventDefault()
              createMutation.mutate(form)
            }}
            className="flex flex-col gap-4"
          >
            <Input
              label={t('common.name')}
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder="e.g. datacenter-mesh"
              required
            />
            <Input
              label={t('s2s.description')}
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              placeholder={t('common.description')}
            />
            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
                {t('s2s.topology')}
              </label>
              <select
                value={form.topology}
                onChange={(e) => setForm({ ...form, topology: e.target.value })}
                className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono"
              >
                <option value="mesh">{t('s2s.mesh')}</option>
                <option value="hub_spoke">{t('s2s.hubSpoke')}</option>
              </select>
            </div>
            <div className="flex justify-end gap-2 mt-2">
              <Button variant="ghost" type="button" onClick={() => setShowCreate(false)}>
                {t('common.cancel')}
              </Button>
              <Button type="submit" disabled={createMutation.isPending}>
                {createMutation.isPending ? t('s2s.creating') : t('common.create')}
              </Button>
            </div>
          </form>
        </Modal>

      <Modal open={!!deleteId} title={t('s2s.deleteTunnel')} onClose={() => setDeleteId(null)}>
          <p className="text-sm text-[var(--text-secondary)] mb-4">
            {t('s2s.confirmDelete')}
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setDeleteId(null)}>{t('common.cancel')}</Button>
            <Button
              variant="danger"
              onClick={() => deleteMutation.mutate(deleteId!)}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending ? t('s2s.deleting') : t('common.delete')}
            </Button>
          </div>
        </Modal>
    </div>
  )
}
