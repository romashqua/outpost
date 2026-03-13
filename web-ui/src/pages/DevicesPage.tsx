import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { CheckCircle, XCircle, Trash2 } from 'lucide-react'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'
import Button from '@/components/ui/Button'
import Modal from '@/components/ui/Modal'
import { api } from '@/api/client'

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

export default function DevicesPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [deleteTarget, setDeleteTarget] = useState<Device | null>(null)

  const { data: devices = [], isLoading, error } = useQuery<Device[]>({
    queryKey: ['devices'],
    queryFn: () => api.get('/devices'),
  })

  const approveMutation = useMutation({
    mutationFn: (id: string) => api.post(`/devices/${id}/approve`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] })
    },
  })

  const revokeMutation = useMutation({
    mutationFn: (id: string) => api.post(`/devices/${id}/revoke`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/devices/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] })
      setDeleteTarget(null)
    },
  })

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
    {
      key: 'user_id',
      header: t('devices.owner'),
      sortable: true,
      render: (row: Device) => (
        <span className="font-mono text-[var(--accent)]">{row.user_id}</span>
      ),
    },
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
      header: t('devices.assignedIp') || 'Assigned IP',
      render: (row: Device) => (
        <span className="font-mono text-xs">{row.assigned_ip}</span>
      ),
    },
    {
      key: 'is_approved',
      header: t('devices.status'),
      render: (row: Device) => (
        <Badge variant={row.is_approved ? 'online' : 'offline'} pulse>
          {row.is_approved ? t('status.approved') || 'Approved' : t('status.pending') || 'Pending'}
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
          {!row.is_approved ? (
            <Button
              variant="ghost"
              size="sm"
              title="Approve"
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
              title="Revoke"
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
            title="Delete"
            onClick={(e) => {
              e.stopPropagation()
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
        Failed to load devices: {(error as Error).message}
      </div>
    )
  }

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
        <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
        {t('devices.title')}
      </h1>

      {isLoading ? (
        <div className="text-center py-12 text-[var(--text-muted)]">Loading...</div>
      ) : (
        <Table columns={columns} data={devices} />
      )}

      {/* Delete Confirmation Modal */}
      <Modal
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title={t('devices.deleteDevice') || 'Delete device'}
      >
        <p className="text-[var(--text-secondary)] mb-6">
          Are you sure you want to delete device{' '}
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
