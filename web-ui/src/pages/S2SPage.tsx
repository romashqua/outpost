import { useTranslation } from 'react-i18next'
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2, Network } from 'lucide-react'
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

export default function S2SPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)
  const [showCreate, setShowCreate] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)
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
        <Table columns={columns} data={tunnels} />
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
