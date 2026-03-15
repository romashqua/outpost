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
import { useToastStore } from '@/store/toast'

interface Network {
  id: string
  name: string
  address: string
  dns: string[]
  port: number
  keepalive: number
  is_active: boolean
  created_at: string
  updated_at: string
}

interface CreateNetworkPayload {
  name: string
  address: string
  dns: string
  port: number
  keepalive: number
}

/**
 * Validates a CIDR string: must be valid format and no host bits set.
 * Returns null if valid, or a suggested corrected CIDR string if host bits are set.
 */
function validateCIDR(cidr: string): { valid: boolean; suggestion?: string; error?: string } {
  const match = cidr.match(/^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\/(\d{1,2})$/)
  if (!match) {
    return { valid: false, error: 'invalid' }
  }

  const parts = match[1].split('.').map(Number)
  const prefix = Number(match[2])

  if (parts.some((p) => p > 255) || prefix > 32) {
    return { valid: false, error: 'invalid' }
  }

  // Check host bits: compute network address and compare
  const ipNum = (parts[0] << 24) | (parts[1] << 16) | (parts[2] << 8) | parts[3]
  const mask = prefix === 0 ? 0 : (~0 << (32 - prefix)) >>> 0
  const networkNum = (ipNum & mask) >>> 0

  if (ipNum !== networkNum) {
    const netParts = [
      (networkNum >>> 24) & 0xff,
      (networkNum >>> 16) & 0xff,
      (networkNum >>> 8) & 0xff,
      networkNum & 0xff,
    ]
    return { valid: false, error: 'hostBits', suggestion: `${netParts.join('.')}/${prefix}` }
  }

  return { valid: true }
}

export default function NetworksPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)
  const [showCreate, setShowCreate] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<Network | null>(null)
  const [cidrError, setCidrError] = useState<string | null>(null)

  const [formData, setFormData] = useState<CreateNetworkPayload>({
    name: '',
    address: '10.0.0.0/24',
    dns: '1.1.1.1, 8.8.8.8',
    port: 51820,
    keepalive: 25,
  })

  const { data: networksData, isLoading, error } = useQuery<{ networks: Network[]; total: number }>({
    queryKey: ['networks'],
    queryFn: () => api.get('/networks'),
  })
  const networks = networksData?.networks ?? []

  const createMutation = useMutation({
    mutationFn: (payload: CreateNetworkPayload) => {
      const body: Record<string, unknown> = { name: payload.name, address: payload.address }
      if (payload.dns) {
        body.dns = payload.dns.split(',').map((s) => s.trim()).filter(Boolean)
      } else {
        body.dns = []
      }
      body.port = payload.port || 51820
      body.keepalive = payload.keepalive || 25
      return api.post('/networks', body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['networks'] })
      setShowCreate(false)
      setFormData({ name: '', address: '10.0.0.0/24', dns: '1.1.1.1, 8.8.8.8', port: 51820, keepalive: 25 })
      setCidrError(null)
      addToast(t('networks.networkCreated'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/networks/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['networks'] })
      setDeleteTarget(null)
      addToast(t('networks.networkDeleted'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const handleCidrChange = (value: string) => {
    setFormData({ ...formData, address: value })
    if (value.length > 0 && value.includes('/')) {
      const result = validateCIDR(value)
      if (!result.valid) {
        if (result.error === 'hostBits' && result.suggestion) {
          setCidrError(t('networks.cidrHostBits', { suggestion: result.suggestion }))
        } else {
          setCidrError(t('networks.cidrInvalid'))
        }
      } else {
        setCidrError(null)
      }
    } else {
      setCidrError(null)
    }
  }

  const handleCidrFix = () => {
    const result = validateCIDR(formData.address)
    if (!result.valid && result.suggestion) {
      setFormData({ ...formData, address: result.suggestion })
      setCidrError(null)
    }
  }

  const handleCreateSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    const result = validateCIDR(formData.address)
    if (!result.valid) {
      if (result.suggestion) {
        setCidrError(t('networks.cidrHostBits', { suggestion: result.suggestion }))
      } else {
        setCidrError(t('networks.cidrInvalid'))
      }
      return
    }
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
        <span className="font-mono text-xs text-[var(--text-muted)]">
          {row.dns && row.dns.length > 0 ? row.dns.join(', ') : '-'}
        </span>
      ),
    },
    {
      key: 'port',
      header: t('networks.port'),
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
      header: t('common.createdAt'),
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
            deleteMutation.reset()
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
        {t('networks.failedToLoad')}: {(error as Error).message}
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
        <Button onClick={() => { createMutation.reset(); setCidrError(null); setShowCreate(true) }}>
          <Plus size={16} />
          {t('networks.createNetwork')}
        </Button>
      </div>

      {isLoading ? (
        <div className="text-center py-12 text-[var(--text-muted)]">{t('common.loading')}</div>
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
          <div>
            <Input
              label={t('networks.address')}
              placeholder="10.0.0.0/24"
              value={formData.address}
              onChange={(e) => handleCidrChange(e.target.value)}
              required
            />
            {cidrError && (
              <div className="mt-1 flex items-center gap-2">
                <p className="text-xs text-[var(--danger)]">{cidrError}</p>
                {formData.address && validateCIDR(formData.address).suggestion && (
                  <button
                    type="button"
                    className="text-xs text-[var(--accent)] underline hover:no-underline"
                    onClick={handleCidrFix}
                  >
                    {t('networks.fixCidr')}
                  </button>
                )}
              </div>
            )}
            <p className="text-[10px] text-[var(--text-muted)] mt-1">
              {t('networks.cidrHint')}
            </p>
          </div>
          <Input
            label={t('networks.dns')}
            placeholder="1.1.1.1, 8.8.8.8"
            value={formData.dns}
            onChange={(e) => setFormData({ ...formData, dns: e.target.value })}
          />
          <div className="grid grid-cols-2 gap-4">
            <Input
              label={t('networks.port')}
              placeholder="51820"
              type="number"
              value={formData.port}
              onChange={(e) =>
                setFormData({ ...formData, port: e.target.value ? Number(e.target.value) : 51820 })
              }
            />
            <Input
              label={t('networks.keepalive')}
              placeholder="25"
              type="number"
              value={formData.keepalive}
              onChange={(e) =>
                setFormData({ ...formData, keepalive: e.target.value ? Number(e.target.value) : 25 })
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
              {createMutation.isPending ? t('networks.creating') : t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title={t('networks.deleteNetwork')}
      >
        <p className="text-[var(--text-secondary)] mb-6">
          {t('networks.confirmDelete')}
          {' '}
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
            {deleteMutation.isPending ? t('networks.deleting') : t('common.delete')}
          </Button>
        </div>
      </Modal>
    </div>
  )
}
