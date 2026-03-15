import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Trash2, Users, Shield, Search } from 'lucide-react'
import Table from '@/components/ui/Table'
import Badge from '@/components/ui/Badge'
import Button from '@/components/ui/Button'
import Input from '@/components/ui/Input'
import Modal from '@/components/ui/Modal'
import { api } from '@/api/client'
import { useToastStore } from '@/store/toast'

interface Group {
  id: string
  name: string
  description: string
  is_system: boolean
  member_count: number
  created_at: string
}

interface GroupMember {
  user_id: string
  username: string
  email: string
}

interface GroupAcl {
  id: string
  network_id: string
  network_name: string
  allowed_ips: string[]
}

interface GroupDetail {
  id: string
  name: string
  description: string
  is_system: boolean
  members: GroupMember[]
  acls: GroupAcl[]
}

interface UserOption {
  id: string
  username: string
  email: string
}

interface NetworkOption {
  id: string
  name: string
  address: string
}

export default function GroupsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)
  const [search, setSearch] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<Group | null>(null)
  const [detailTarget, setDetailTarget] = useState<string | null>(null)
  const [detailTab, setDetailTab] = useState<'members' | 'acls'>('members')
  const [addMemberUserId, setAddMemberUserId] = useState('')
  const [addAclNetworkId, setAddAclNetworkId] = useState('')
  const [addAclAllowedIps, setAddAclAllowedIps] = useState('0.0.0.0/0')

  const [formData, setFormData] = useState({ name: '', description: '' })

  const { data: groups = [], isLoading, error } = useQuery<Group[]>({
    queryKey: ['groups'],
    queryFn: () => api.get('/groups'),
  })

  const { data: groupDetail, isLoading: detailLoading } = useQuery<GroupDetail>({
    queryKey: ['groups', detailTarget],
    queryFn: () => api.get(`/groups/${detailTarget}`),
    enabled: !!detailTarget,
  })

  const { data: usersData } = useQuery<{ users: UserOption[] }>({
    queryKey: ['users'],
    queryFn: () => api.get('/users?page=1&per_page=100'),
    enabled: !!detailTarget,
  })

  const { data: networksData } = useQuery<{ networks: NetworkOption[] }>({
    queryKey: ['networks'],
    queryFn: () => api.get('/networks'),
    enabled: !!detailTarget,
  })
  const networks = networksData?.networks ?? []

  const createMutation = useMutation({
    mutationFn: (payload: { name: string; description: string }) =>
      api.post('/groups', payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      setShowCreate(false)
      setFormData({ name: '', description: '' })
      addToast(t('groups.groupCreated'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/groups/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      setDeleteTarget(null)
      addToast(t('groups.groupDeleted'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const addMemberMutation = useMutation({
    mutationFn: (payload: { groupId: string; user_id: string }) =>
      api.post(`/groups/${payload.groupId}/members`, { user_id: payload.user_id }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups', detailTarget] })
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      setAddMemberUserId('')
      addToast(t('groups.memberAdded'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const removeMemberMutation = useMutation({
    mutationFn: (payload: { groupId: string; userId: string }) =>
      api.delete(`/groups/${payload.groupId}/members/${payload.userId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups', detailTarget] })
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      addToast(t('groups.memberRemoved'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const addAclMutation = useMutation({
    mutationFn: (payload: { groupId: string; network_id: string; allowed_ips: string[] }) =>
      api.post(`/groups/${payload.groupId}/acls`, {
        network_id: payload.network_id,
        allowed_ips: payload.allowed_ips,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups', detailTarget] })
      setAddAclNetworkId('')
      setAddAclAllowedIps('0.0.0.0/0')
      addToast(t('groups.aclAdded'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const removeAclMutation = useMutation({
    mutationFn: (payload: { groupId: string; aclId: string }) =>
      api.delete(`/groups/${payload.groupId}/acls/${payload.aclId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups', detailTarget] })
      addToast(t('groups.aclRemoved'), 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const filtered = groups.filter(
    (g) =>
      g.name.toLowerCase().includes(search.toLowerCase()) ||
      g.description.toLowerCase().includes(search.toLowerCase()),
  )

  const handleCreateSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    createMutation.mutate(formData)
  }

  const handleAddMember = () => {
    if (!detailTarget || !addMemberUserId) return
    addMemberMutation.mutate({ groupId: detailTarget, user_id: addMemberUserId })
  }

  const handleAddAcl = () => {
    if (!detailTarget || !addAclNetworkId) return
    const ips = addAclAllowedIps
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    addAclMutation.mutate({
      groupId: detailTarget,
      network_id: addAclNetworkId,
      allowed_ips: ips.length > 0 ? ips : ['0.0.0.0/0'],
    })
  }

  const availableUsers = (usersData?.users ?? []).filter(
    (u) => !groupDetail?.members.some((m) => m.user_id === u.id),
  )

  const availableNetworks = networks.filter(
    (n) => !groupDetail?.acls.some((a) => a.network_id === n.id),
  )

  const columns = [
    {
      key: 'name',
      header: t('groups.name'),
      sortable: true,
      render: (row: Group) => (
        <span className="font-mono text-[var(--accent)]">{row.name}</span>
      ),
    },
    {
      key: 'description',
      header: t('groups.description'),
      render: (row: Group) => (
        <span className="text-[var(--text-secondary)] text-sm truncate max-w-[200px] inline-block">
          {row.description || '-'}
        </span>
      ),
    },
    {
      key: 'member_count',
      header: t('groups.memberCount'),
      sortable: true,
      render: (row: Group) => (
        <div className="flex items-center gap-1.5">
          <Users size={14} className="text-[var(--text-muted)]" />
          <span className="font-mono text-sm">{row.member_count}</span>
        </div>
      ),
    },
    {
      key: 'is_system',
      header: t('groups.type'),
      render: (row: Group) =>
        row.is_system ? (
          <Badge variant="info">
            <Shield size={12} className="mr-1" />
            {t('groups.system')}
          </Badge>
        ) : (
          <Badge variant="online">{t('groups.userCreated')}</Badge>
        ),
    },
    {
      key: 'created_at',
      header: t('common.createdAt'),
      sortable: true,
      render: (row: Group) => (
        <span className="font-mono text-xs text-[var(--text-muted)]">
          {new Date(row.created_at).toLocaleDateString()}
        </span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row: Group) => (
        <Button
          variant="ghost"
          size="sm"
          onClick={(e) => {
            e.stopPropagation()
            if (row.is_system) {
              addToast(t('groups.cannotDeleteSystem'), 'error')
              return
            }
            deleteMutation.reset()
            setDeleteTarget(row)
          }}
          disabled={row.is_system}
        >
          <Trash2 size={14} className={row.is_system ? 'text-[var(--text-muted)]' : 'text-[var(--danger)]'} />
        </Button>
      ),
    },
  ]

  const handleRowClick = (row: Group) => {
    setDetailTab('members')
    setAddMemberUserId('')
    setAddAclNetworkId('')
    setAddAclAllowedIps('0.0.0.0/0')
    setDetailTarget(row.id)
  }

  if (error) {
    return (
      <div className="text-center py-12 text-[var(--danger)]">
        {t('groups.failedToLoad')}: {(error as Error).message}
      </div>
    )
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('groups.title')}
        </h1>
        <Button onClick={() => { createMutation.reset(); setShowCreate(true) }}>
          <Plus size={16} />
          {t('groups.createGroup')}
        </Button>
      </div>

      <div className="mb-4 max-w-sm">
        <Input
          placeholder={t('groups.search')}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          icon={<Search size={16} />}
        />
      </div>

      {isLoading ? (
        <div className="text-center py-12 text-[var(--text-muted)]">{t('common.loading')}</div>
      ) : (
        <Table columns={columns} data={filtered} onRowClick={handleRowClick} />
      )}

      {/* Create Group Modal */}
      <Modal open={showCreate} onClose={() => setShowCreate(false)} title={t('groups.createGroup')}>
        <form className="flex flex-col gap-4" onSubmit={handleCreateSubmit}>
          <Input
            label={t('groups.name')}
            placeholder="engineering"
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            required
          />
          <Input
            label={t('groups.description')}
            placeholder={t('groups.descriptionPlaceholder')}
            value={formData.description}
            onChange={(e) => setFormData({ ...formData, description: e.target.value })}
          />
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
              {createMutation.isPending ? t('groups.creating') : t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title={t('groups.deleteGroup')}
      >
        <p className="text-[var(--text-secondary)] mb-6">
          {t('groups.confirmDelete')}{' '}
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
            {deleteMutation.isPending ? t('groups.deleting') : t('common.delete')}
          </Button>
        </div>
      </Modal>

      {/* Group Detail Modal */}
      <Modal
        open={!!detailTarget}
        onClose={() => setDetailTarget(null)}
        title={groupDetail ? groupDetail.name : t('common.loading')}
      >
        {detailLoading || !groupDetail ? (
          <div className="text-center py-8 text-[var(--text-muted)]">{t('common.loading')}</div>
        ) : (
          <div className="flex flex-col gap-4">
            {/* Group Info */}
            <div className="flex items-center gap-3">
              <span className="font-mono text-[var(--accent)] text-lg">{groupDetail.name}</span>
              {groupDetail.is_system && (
                <Badge variant="info">
                  <Shield size={12} className="mr-1" />
                  {t('groups.system')}
                </Badge>
              )}
            </div>
            {groupDetail.description && (
              <p className="text-sm text-[var(--text-secondary)]">{groupDetail.description}</p>
            )}

            {/* Tabs */}
            <div className="flex border-b border-[var(--border)]">
              <button
                type="button"
                className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
                  detailTab === 'members'
                    ? 'border-[var(--accent)] text-[var(--accent)]'
                    : 'border-transparent text-[var(--text-muted)] hover:text-[var(--text-primary)]'
                }`}
                onClick={() => setDetailTab('members')}
              >
                <Users size={14} className="inline mr-1.5 -mt-0.5" />
                {t('groups.members')} ({groupDetail.members.length})
              </button>
              <button
                type="button"
                className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
                  detailTab === 'acls'
                    ? 'border-[var(--accent)] text-[var(--accent)]'
                    : 'border-transparent text-[var(--text-muted)] hover:text-[var(--text-primary)]'
                }`}
                onClick={() => setDetailTab('acls')}
              >
                <Shield size={14} className="inline mr-1.5 -mt-0.5" />
                {t('groups.networkAccess')} ({groupDetail.acls.length})
              </button>
            </div>

            {/* Members Tab */}
            {detailTab === 'members' && (
              <div className="flex flex-col gap-3">
                {/* Add Member */}
                <div className="flex items-end gap-2">
                  <label className="flex flex-col gap-1 text-sm text-[var(--text-secondary)] flex-1">
                    {t('groups.addMember')}
                    <select
                      value={addMemberUserId}
                      onChange={(e) => setAddMemberUserId(e.target.value)}
                      className="rounded border border-[var(--border)] bg-[var(--bg-secondary)] text-[var(--text-primary)] px-3 py-2 text-sm"
                    >
                      <option value="">{t('groups.selectUser')}</option>
                      {availableUsers.map((u) => (
                        <option key={u.id} value={u.id}>
                          {u.username} ({u.email})
                        </option>
                      ))}
                    </select>
                  </label>
                  <Button
                    size="sm"
                    onClick={handleAddMember}
                    disabled={!addMemberUserId || addMemberMutation.isPending}
                  >
                    <Plus size={14} />
                    {t('common.add')}
                  </Button>
                </div>

                {/* Members List */}
                {groupDetail.members.length === 0 ? (
                  <p className="text-sm text-[var(--text-muted)] text-center py-4">
                    {t('groups.noMembers')}
                  </p>
                ) : (
                  <div className="border border-[var(--border)] rounded divide-y divide-[var(--border)] max-h-60 overflow-y-auto">
                    {groupDetail.members.map((member) => (
                      <div
                        key={member.user_id}
                        className="flex items-center justify-between px-3 py-2"
                      >
                        <div className="flex flex-col">
                          <span className="font-mono text-sm text-[var(--accent)]">
                            {member.username}
                          </span>
                          <span className="text-xs text-[var(--text-muted)]">{member.email}</span>
                        </div>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() =>
                            removeMemberMutation.mutate({
                              groupId: groupDetail.id,
                              userId: member.user_id,
                            })
                          }
                          disabled={removeMemberMutation.isPending}
                        >
                          <Trash2 size={14} className="text-[var(--danger)]" />
                        </Button>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Network Access Tab */}
            {detailTab === 'acls' && (
              <div className="flex flex-col gap-3">
                {/* Add ACL */}
                <div className="flex flex-col gap-2">
                  <div className="flex items-end gap-2">
                    <label className="flex flex-col gap-1 text-sm text-[var(--text-secondary)] flex-1">
                      {t('groups.network')}
                      <select
                        value={addAclNetworkId}
                        onChange={(e) => setAddAclNetworkId(e.target.value)}
                        className="rounded border border-[var(--border)] bg-[var(--bg-secondary)] text-[var(--text-primary)] px-3 py-2 text-sm"
                      >
                        <option value="">{t('groups.selectNetwork')}</option>
                        {availableNetworks.map((n) => (
                          <option key={n.id} value={n.id}>
                            {n.name} ({n.address})
                          </option>
                        ))}
                      </select>
                    </label>
                    <div className="flex-1">
                      <Input
                        label={t('groups.allowedIps')}
                        placeholder="0.0.0.0/0"
                        value={addAclAllowedIps}
                        onChange={(e) => setAddAclAllowedIps(e.target.value)}
                      />
                    </div>
                    <Button
                      size="sm"
                      onClick={handleAddAcl}
                      disabled={!addAclNetworkId || addAclMutation.isPending}
                    >
                      <Plus size={14} />
                      {t('common.add')}
                    </Button>
                  </div>
                  <p className="text-[10px] text-[var(--text-muted)]">
                    {t('groups.allowedIpsHint')}
                  </p>
                </div>

                {/* ACLs List */}
                {groupDetail.acls.length === 0 ? (
                  <p className="text-sm text-[var(--text-muted)] text-center py-4">
                    {t('groups.noAcls')}
                  </p>
                ) : (
                  <div className="border border-[var(--border)] rounded divide-y divide-[var(--border)] max-h-60 overflow-y-auto">
                    {groupDetail.acls.map((acl) => (
                      <div
                        key={acl.id}
                        className="flex items-center justify-between px-3 py-2"
                      >
                        <div className="flex flex-col">
                          <span className="font-mono text-sm text-[var(--accent)]">
                            {acl.network_name}
                          </span>
                          <span className="font-mono text-xs text-[var(--text-muted)]">
                            {acl.allowed_ips.join(', ')}
                          </span>
                        </div>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() =>
                            removeAclMutation.mutate({
                              groupId: groupDetail.id,
                              aclId: acl.id,
                            })
                          }
                          disabled={removeAclMutation.isPending}
                        >
                          <Trash2 size={14} className="text-[var(--danger)]" />
                        </Button>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </Modal>
    </div>
  )
}
