import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { UserPlus, Search, Shield, ShieldCheck, Trash2 } from 'lucide-react'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'
import Button from '@/components/ui/Button'
import Input from '@/components/ui/Input'
import Modal from '@/components/ui/Modal'
import { api } from '@/api/client'

interface User {
  id: string
  username: string
  email: string
  first_name: string
  last_name: string
  is_admin: boolean
  is_active: boolean
  mfa_enabled: boolean
  last_login: string | null
  created_at: string
}

interface UsersResponse {
  users: User[]
  total: number
  page: number
  per_page: number
}

interface CreateUserPayload {
  username: string
  email: string
  password: string
  first_name: string
  last_name: string
  is_admin: boolean
}

export default function UsersPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null)

  const [formData, setFormData] = useState<CreateUserPayload>({
    username: '',
    email: '',
    password: '',
    first_name: '',
    last_name: '',
    is_admin: false,
  })

  const { data, isLoading, error } = useQuery<UsersResponse>({
    queryKey: ['users'],
    queryFn: () => api.get('/users?page=1&per_page=100'),
  })

  const createMutation = useMutation({
    mutationFn: (payload: CreateUserPayload) => api.post('/users', payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setShowCreate(false)
      setFormData({ username: '', email: '', password: '', first_name: '', last_name: '', is_admin: false })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/users/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['users'] })
      setDeleteTarget(null)
    },
  })

  const users = data?.users ?? []
  const filtered = users.filter(
    (u) =>
      u.username.toLowerCase().includes(search.toLowerCase()) ||
      u.email.toLowerCase().includes(search.toLowerCase()) ||
      u.first_name.toLowerCase().includes(search.toLowerCase()) ||
      u.last_name.toLowerCase().includes(search.toLowerCase()),
  )

  const handleCreateSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createMutation.mutate(formData)
  }

  const columns = [
    {
      key: 'username',
      header: t('users.username'),
      sortable: true,
      render: (row: User) => (
        <span className="font-mono text-[var(--accent)]">{row.username}</span>
      ),
    },
    { key: 'email', header: t('users.email'), sortable: true },
    {
      key: 'is_admin',
      header: t('users.role'),
      render: (row: User) => (
        <Badge variant={row.is_admin ? 'info' : 'default'}>
          {row.is_admin ? 'admin' : 'user'}
        </Badge>
      ),
    },
    {
      key: 'is_active',
      header: t('users.status'),
      render: (row: User) => (
        <Badge variant={row.is_active ? 'online' : 'offline'} pulse>
          {t(`status.${row.is_active ? 'active' : 'inactive'}`)}
        </Badge>
      ),
    },
    {
      key: 'mfa_enabled',
      header: t('users.mfaEnabled'),
      render: (row: User) =>
        row.mfa_enabled ? (
          <ShieldCheck size={16} className="text-[var(--accent)]" />
        ) : (
          <Shield size={16} className="text-[var(--text-muted)]" />
        ),
    },
    {
      key: 'last_login',
      header: t('users.lastLogin'),
      sortable: true,
      render: (row: User) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">
          {row.last_login ?? '-'}
        </span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row: User) => (
        <Button
          variant="ghost"
          size="sm"
          onClick={(e) => {
            e.stopPropagation()
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
        Failed to load users: {(error as Error).message}
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('users.title')}
        </h1>
        <Button onClick={() => setShowCreate(true)}>
          <UserPlus size={16} />
          {t('users.createUser')}
        </Button>
      </div>

      <div className="mb-4 max-w-sm">
        <Input
          placeholder={t('users.search')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          icon={<Search size={16} />}
        />
      </div>

      {isLoading ? (
        <div className="text-center py-12 text-[var(--text-muted)]">Loading...</div>
      ) : (
        <Table columns={columns} data={filtered} />
      )}

      {/* Create User Modal */}
      <Modal open={showCreate} onClose={() => setShowCreate(false)} title={t('users.createUser')}>
        <form className="flex flex-col gap-4" onSubmit={handleCreateSubmit}>
          <Input
            label={t('users.username')}
            placeholder="username"
            value={formData.username}
            onChange={(e) => setFormData({ ...formData, username: e.target.value })}
            required
          />
          <Input
            label={t('users.email')}
            placeholder="user@corp.ru"
            type="email"
            value={formData.email}
            onChange={(e) => setFormData({ ...formData, email: e.target.value })}
            required
          />
          <div className="grid grid-cols-2 gap-4">
            <Input
              label={t('users.firstName') || 'First name'}
              placeholder="First name"
              value={formData.first_name}
              onChange={(e) => setFormData({ ...formData, first_name: e.target.value })}
            />
            <Input
              label={t('users.lastName') || 'Last name'}
              placeholder="Last name"
              value={formData.last_name}
              onChange={(e) => setFormData({ ...formData, last_name: e.target.value })}
            />
          </div>
          <Input
            label={t('users.password') || 'Password'}
            placeholder="Password"
            type="password"
            value={formData.password}
            onChange={(e) => setFormData({ ...formData, password: e.target.value })}
            required
          />
          <label className="flex items-center gap-2 text-sm text-[var(--text-secondary)] cursor-pointer">
            <input
              type="checkbox"
              checked={formData.is_admin}
              onChange={(e) => setFormData({ ...formData, is_admin: e.target.checked })}
              className="rounded"
            />
            Admin
          </label>
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
              {createMutation.isPending ? 'Creating...' : t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title={t('users.deleteUser') || 'Delete user'}
      >
        <p className="text-[var(--text-secondary)] mb-6">
          Are you sure you want to deactivate user{' '}
          <span className="font-mono text-[var(--accent)]">{deleteTarget?.username}</span>?
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
