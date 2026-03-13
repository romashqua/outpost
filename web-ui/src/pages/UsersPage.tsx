import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { UserPlus, Search, Shield, ShieldCheck } from 'lucide-react'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'
import Button from '@/components/ui/Button'
import Input from '@/components/ui/Input'
import Modal from '@/components/ui/Modal'

const mockUsers = [
  { id: '1', username: 'ivanov', email: 'ivanov@corp.ru', role: 'admin', status: 'active', lastLogin: '2026-03-13 14:23', mfaEnabled: true, groups: 'Engineering, VPN-Full' },
  { id: '2', username: 'petrov', email: 'petrov@corp.ru', role: 'user', status: 'active', lastLogin: '2026-03-13 14:21', mfaEnabled: true, groups: 'Engineering' },
  { id: '3', username: 'sidorov', email: 'sidorov@corp.ru', role: 'user', status: 'active', lastLogin: '2026-03-13 12:05', mfaEnabled: false, groups: 'Sales' },
  { id: '4', username: 'kozlov', email: 'kozlov@corp.ru', role: 'user', status: 'active', lastLogin: '2026-03-13 10:44', mfaEnabled: true, groups: 'Operations' },
  { id: '5', username: 'fedorov', email: 'fedorov@corp.ru', role: 'auditor', status: 'inactive', lastLogin: '2026-03-10 09:12', mfaEnabled: false, groups: 'Compliance' },
  { id: '6', username: 'morozov', email: 'morozov@corp.ru', role: 'user', status: 'active', lastLogin: '2026-03-13 13:55', mfaEnabled: true, groups: 'Engineering' },
]

export default function UsersPage() {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [showCreate, setShowCreate] = useState(false)

  const filtered = mockUsers.filter(
    (u) =>
      u.username.includes(search.toLowerCase()) ||
      u.email.includes(search.toLowerCase()),
  )

  const columns = [
    {
      key: 'username',
      header: t('users.username'),
      sortable: true,
      render: (row: typeof mockUsers[0]) => (
        <span className="font-mono text-[var(--accent)]">{row.username}</span>
      ),
    },
    { key: 'email', header: t('users.email'), sortable: true },
    {
      key: 'role',
      header: t('users.role'),
      render: (row: typeof mockUsers[0]) => (
        <Badge variant={row.role === 'admin' ? 'info' : 'default'}>{row.role}</Badge>
      ),
    },
    {
      key: 'status',
      header: t('users.status'),
      render: (row: typeof mockUsers[0]) => (
        <Badge variant={row.status === 'active' ? 'online' : 'offline'} pulse>
          {t(`status.${row.status}`)}
        </Badge>
      ),
    },
    {
      key: 'mfaEnabled',
      header: t('users.mfaEnabled'),
      render: (row: typeof mockUsers[0]) =>
        row.mfaEnabled ? (
          <ShieldCheck size={16} className="text-[var(--accent)]" />
        ) : (
          <Shield size={16} className="text-[var(--text-muted)]" />
        ),
    },
    {
      key: 'lastLogin',
      header: t('users.lastLogin'),
      sortable: true,
      render: (row: typeof mockUsers[0]) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">{row.lastLogin}</span>
      ),
    },
    { key: 'groups', header: t('users.groups') },
  ]

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

      <Table columns={columns} data={filtered} />

      <Modal open={showCreate} onClose={() => setShowCreate(false)} title={t('users.createUser')}>
        <form className="flex flex-col gap-4" onSubmit={(e) => { e.preventDefault(); setShowCreate(false) }}>
          <Input label={t('users.username')} placeholder="username" />
          <Input label={t('users.email')} placeholder="user@corp.ru" type="email" />
          <Input label={t('users.role')} placeholder="user" />
          <div className="flex gap-3 justify-end mt-2">
            <Button variant="secondary" type="button" onClick={() => setShowCreate(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit">{t('common.create')}</Button>
          </div>
        </form>
      </Modal>
    </div>
  )
}
