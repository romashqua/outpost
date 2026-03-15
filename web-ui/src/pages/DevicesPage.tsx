import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { CheckCircle, XCircle, Trash2, Plus, Download, Copy, Check, FileDown, Mail } from 'lucide-react'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'
import Button from '@/components/ui/Button'
import Input from '@/components/ui/Input'
import Modal from '@/components/ui/Modal'
import { api } from '@/api/client'
import { useToastStore } from '@/store/toast'
import { useAuthStore } from '@/store/auth'

interface Device {
  id: string
  user_id: string
  name: string
  wireguard_pubkey: string
  assigned_ip: string
  is_approved: boolean
  last_handshake: string | null
  created_at: string
}

interface User {
  id: string
  username: string
  email: string
  first_name: string
  last_name: string
  is_admin: boolean
  is_active: boolean
}

interface UsersResponse {
  users: User[]
  total: number
  page: number
  per_page: number
}

interface ConfigResponse {
  config: string
  private_key: string
  public_key: string
}

export default function DevicesPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)
  const currentUser = useAuthStore((s) => s.user)
  const isAdmin = currentUser?.role === 'admin'
  const [deleteTarget, setDeleteTarget] = useState<Device | null>(null)
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [showConfigModal, setShowConfigModal] = useState(false)
  const [configText, setConfigText] = useState('')
  const [configDeviceId, setConfigDeviceId] = useState<string | null>(null)
  const [createForm, setCreateForm] = useState({
    name: '',
    user_id: '',
    network_id: '',
    autoGenerateKey: true,
    wireguard_pubkey: '',
  })
  const [copiedConfig, setCopiedConfig] = useState(false)

  interface Network {
    id: string
    name: string
    address: string
    is_active: boolean
  }
  interface NetworksResponse {
    networks: Network[]
    total: number
  }

  interface DevicesResponse {
    devices: Device[]
    total: number
    page: number
    per_page: number
  }

  const { data: devicesData, isLoading, error } = useQuery<DevicesResponse>({
    queryKey: ['devices', isAdmin ? 'all' : 'my'],
    queryFn: () => api.get(isAdmin ? '/devices' : '/devices/my'),
  })

  const devices = devicesData?.devices ?? []

  const { data: usersData } = useQuery<UsersResponse>({
    queryKey: ['users'],
    queryFn: () => api.get('/users'),
    enabled: isAdmin,
  })

  const users = usersData?.users ?? []

  const { data: networksData } = useQuery<NetworksResponse>({
    queryKey: ['networks', isAdmin ? 'all' : 'my'],
    queryFn: () => api.get(isAdmin ? '/networks' : '/networks/my'),
  })

  const networks = networksData?.networks ?? []

  function copyToClipboard(text: string) {
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(() => {
        setCopiedConfig(true)
        setTimeout(() => setCopiedConfig(false), 2000)
      }).catch(() => {
        fallbackCopy(text)
      })
    } else {
      fallbackCopy(text)
    }
  }

  function fallbackCopy(text: string) {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.style.position = 'fixed'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    try {
      document.execCommand('copy')
      setCopiedConfig(true)
      setTimeout(() => setCopiedConfig(false), 2000)
    } catch {
      addToast(t('common.copyFailed') || 'Failed to copy', 'error')
    }
    document.body.removeChild(textarea)
  }

  const approveMutation = useMutation({
    mutationFn: (id: string) => api.post(`/devices/${id}/approve`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] })
      addToast(t('devices.approve'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const revokeMutation = useMutation({
    mutationFn: (id: string) => api.post(`/devices/${id}/revoke`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] })
      addToast(t('devices.revoke'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/devices/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] })
      setDeleteTarget(null)
      addToast(t('common.delete'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const createMutation = useMutation({
    mutationFn: async (form: typeof createForm) => {
      let pubkey = form.wireguard_pubkey

      if (form.autoGenerateKey) {
        pubkey = 'auto-generated'
      }

      const device = await api.post<Device>('/devices', {
        name: form.name,
        user_id: form.user_id,
        wireguard_pubkey: pubkey,
        ...(form.network_id ? { network_id: form.network_id } : {}),
      })

      if (form.autoGenerateKey) {
        const configResp = await api.get<ConfigResponse>(`/devices/${device.id}/config`)
        return { device, config: configResp }
      }

      return { device, config: null }
    },
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ['devices'] })
      setShowCreateModal(false)
      setCreateForm({ name: '', user_id: '', network_id: '', autoGenerateKey: true, wireguard_pubkey: '' })
      addToast(t('devices.createDevice'), 'success')

      if (result.config) {
        setConfigText(result.config.config)
        setConfigDeviceId(result.device.id)
        setShowConfigModal(true)
      }
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const downloadConfigMutation = useMutation({
    mutationFn: (id: string) => api.get<ConfigResponse>(`/devices/${id}/config`),
    onSuccess: (resp, id) => {
      setConfigText(resp.config)
      setConfigDeviceId(id)
      setShowConfigModal(true)
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const sendConfigMutation = useMutation({
    mutationFn: (id: string) => api.post<{ status: string; email: string }>(`/devices/${id}/send-config`),
    onSuccess: (resp) => {
      addToast(t('devices.configSent', { email: resp.email }), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const copyConfig = () => {
    copyToClipboard(configText)
  }

  const downloadConfigFile = () => {
    const blob = new Blob([configText], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'wg-outpost.conf'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  const truncatePubkey = (key: string) => {
    if (key.length <= 16) return key
    return `${key.slice(0, 8)}...${key.slice(-6)}`
  }

  const columns = [
    {
      key: 'name',
      header: t('devices.name'),
      sortable: true,
      render: (row: Device) => (
        <span className="font-mono text-[var(--text-primary)]">{row.name}</span>
      ),
    },
    ...(isAdmin ? [{
      key: 'user_id',
      header: t('devices.owner'),
      sortable: true,
      render: (row: Device) => (
        <span className="font-mono text-[var(--accent)]">{row.user_id}</span>
      ),
    }] : []),
    {
      key: 'wireguard_pubkey',
      header: t('devices.pubkey'),
      render: (row: Device) => (
        <span
          className="font-mono text-xs text-[var(--text-muted)] bg-[var(--bg-tertiary)] px-2 py-0.5 rounded"
          title={row.wireguard_pubkey}
        >
          {truncatePubkey(row.wireguard_pubkey)}
        </span>
      ),
    },
    {
      key: 'assigned_ip',
      header: t('devices.assignedIp'),
      render: (row: Device) => (
        <span className="font-mono text-xs">{row.assigned_ip}</span>
      ),
    },
    {
      key: 'is_approved',
      header: t('devices.status'),
      render: (row: Device) => (
        <Badge variant={row.is_approved ? 'online' : 'offline'} pulse>
          {row.is_approved ? t('status.approved') : t('status.pending')}
        </Badge>
      ),
    },
    {
      key: 'last_handshake',
      header: t('devices.lastHandshake'),
      render: (row: Device) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">
          {row.last_handshake ?? '-'}
        </span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row: Device) => (
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            title={t('devices.downloadConfig')}
            disabled={downloadConfigMutation.isPending}
            onClick={(e) => {
              e.stopPropagation()
              downloadConfigMutation.mutate(row.id)
            }}
          >
            <Download size={14} className="text-[var(--accent)]" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            title={t('devices.sendToEmail')}
            disabled={sendConfigMutation.isPending}
            onClick={(e) => {
              e.stopPropagation()
              sendConfigMutation.mutate(row.id)
            }}
          >
            <Mail size={14} className="text-[var(--accent)]" />
          </Button>
          {isAdmin && (
            <>
              {!row.is_approved ? (
                <Button
                  variant="ghost"
                  size="sm"
                  title={t('devices.approve')}
                  disabled={approveMutation.isPending}
                  onClick={(e) => {
                    e.stopPropagation()
                    approveMutation.mutate(row.id)
                  }}
                >
                  <CheckCircle size={14} className="text-[var(--accent)]" />
                </Button>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  title={t('devices.revoke')}
                  disabled={revokeMutation.isPending}
                  onClick={(e) => {
                    e.stopPropagation()
                    revokeMutation.mutate(row.id)
                  }}
                >
                  <XCircle size={14} className="text-[var(--warning)]" />
                </Button>
              )}
              <Button
                variant="ghost"
                size="sm"
                title={t('common.delete')}
                onClick={(e) => {
                  e.stopPropagation()
                  deleteMutation.reset()
                  setDeleteTarget(row)
                }}
              >
                <Trash2 size={14} className="text-[var(--danger)]" />
              </Button>
            </>
          )}
        </div>
      ),
    },
  ]

  if (error) {
    return (
      <div className="text-center py-12 text-[var(--danger)]">
        {t('devices.failedToLoad')}: {(error as Error).message}
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('devices.title')}
        </h1>
        <Button onClick={() => { createMutation.reset(); setShowCreateModal(true) }}>
          <Plus size={16} className="mr-1" />
          {t('devices.addDevice')}
        </Button>
      </div>

      {isLoading ? (
        <div className="text-center py-12 text-[var(--text-muted)]">{t('common.loading')}</div>
      ) : (
        <Table columns={columns} data={devices} />
      )}

      {/* Create Device Modal */}
      <Modal
        open={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        title={t('devices.addDevice')}
      >
        <form
          onSubmit={(e) => {
            e.preventDefault()
            const userId = isAdmin ? createForm.user_id : (currentUser?.id ?? '')
            if (!createForm.name || !userId || !createForm.network_id) return
            if (!createForm.autoGenerateKey && !createForm.wireguard_pubkey) return
            createMutation.mutate({ ...createForm, user_id: userId })
          }}
        >
          <div className="flex flex-col gap-4 mb-6">
            <Input
              label={t('devices.name')}
              placeholder="e.g. laptop-work"
              value={createForm.name}
              onChange={(e) => setCreateForm((f) => ({ ...f, name: e.target.value }))}
              required
            />

            {isAdmin ? (
              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
                  {t('devices.user')}
                </label>
                <select
                  className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono glow-focus transition-all duration-150"
                  value={createForm.user_id}
                  onChange={(e) => setCreateForm((f) => ({ ...f, user_id: e.target.value }))}
                  required
                >
                  <option value="">{t('devices.selectUser')}</option>
                  {users.map((u) => (
                    <option key={u.id} value={u.id}>
                      {u.username} ({u.email})
                    </option>
                  ))}
                </select>
              </div>
            ) : null}

            <div className="flex flex-col gap-1.5">
              <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
                {t('devices.network')}
              </label>
              <select
                className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono glow-focus transition-all duration-150"
                value={createForm.network_id}
                onChange={(e) => setCreateForm((f) => ({ ...f, network_id: e.target.value }))}
                required
              >
                <option value="">{t('devices.selectNetwork')}</option>
                {networks.filter(n => n.is_active).map((n) => (
                  <option key={n.id} value={n.id}>
                    {n.name} ({n.address})
                  </option>
                ))}
              </select>
            </div>

            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="autoGenKey"
                checked={createForm.autoGenerateKey}
                onChange={(e) =>
                  setCreateForm((f) => ({ ...f, autoGenerateKey: e.target.checked }))
                }
                className="rounded border-[var(--border)] bg-[var(--bg-secondary)]"
              />
              <label
                htmlFor="autoGenKey"
                className="text-sm text-[var(--text-secondary)] cursor-pointer"
              >
                {t('devices.autoGenerateKey')}
              </label>
            </div>

            {!createForm.autoGenerateKey && (
              <Input
                label={t('devices.wireguardPubkey')}
                placeholder="Base64-encoded public key"
                value={createForm.wireguard_pubkey}
                onChange={(e) =>
                  setCreateForm((f) => ({ ...f, wireguard_pubkey: e.target.value }))
                }
                required
              />
            )}
          </div>

          {createMutation.error && (
            <p className="text-sm text-[var(--danger)] mb-4">
              {(createMutation.error as Error).message}
            </p>
          )}

          <div className="flex gap-3 justify-end">
            <Button variant="secondary" type="button" onClick={() => setShowCreateModal(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={createMutation.isPending}>
              {createMutation.isPending ? t('devices.creating') : t('devices.createDevice')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* WireGuard Config Modal */}
      <Modal
        open={showConfigModal}
        onClose={() => setShowConfigModal(false)}
        title={t('devices.wireguardConfig')}
      >
        <div className="flex flex-col gap-4">
          <div className="relative">
            <pre className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-md p-4 text-xs font-mono text-[var(--text-primary)] overflow-x-auto whitespace-pre-wrap max-h-80 overflow-y-auto">
              {configText}
            </pre>
          </div>

          <div className="flex gap-3 justify-end">
            <Button variant="secondary" onClick={copyConfig}>
              {copiedConfig ? <Check size={14} className="mr-1" /> : <Copy size={14} className="mr-1" />}
              {copiedConfig ? t('common.copied') || 'Copied' : t('devices.copy')}
            </Button>
            <Button variant="secondary" onClick={downloadConfigFile}>
              <FileDown size={14} className="mr-1" />
              {t('devices.downloadConf')}
            </Button>
            {configDeviceId && (
              <Button
                disabled={sendConfigMutation.isPending}
                onClick={() => sendConfigMutation.mutate(configDeviceId)}
              >
                <Mail size={14} className="mr-1" />
                {t('devices.sendToEmail')}
              </Button>
            )}
          </div>
        </div>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title={t('devices.deleteDevice')}
      >
        <p className="text-[var(--text-secondary)] mb-6">
          {t('devices.confirmDelete')}{' '}
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
            {deleteMutation.isPending ? t('devices.deleting') : t('common.delete')}
          </Button>
        </div>
      </Modal>
    </div>
  )
}
