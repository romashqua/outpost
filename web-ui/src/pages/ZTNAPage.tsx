import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Shield, Plus, Trash2, Settings2 } from 'lucide-react'
import Card from '@/components/ui/Card'
import Button from '@/components/ui/Button'
import Badge from '@/components/ui/Badge'
import Table from '@/components/ui/Table'
import Modal from '@/components/ui/Modal'
import Input from '@/components/ui/Input'
import { api } from '@/api/client'
import { useToastStore } from '@/store/toast'

interface TrustScoreSummary {
  device_id: string
  device_name: string
  user_id: string
  username: string
  score: number
  level: 'high' | 'medium' | 'low' | 'critical'
  evaluated_at: string
}

interface TrustConfig {
  weight_disk_encryption: number
  weight_screen_lock: number
  weight_antivirus: number
  weight_firewall: number
  weight_os_version: number
  weight_mfa: number
  threshold_high: number
  threshold_medium: number
  threshold_low: number
  auto_restrict_below_medium: boolean
  auto_block_below_low: boolean
}

interface ZTNAPolicy {
  id: string
  name: string
  description: string | null
  is_active: boolean
  conditions: Record<string, unknown>
  action: 'allow' | 'restrict' | 'deny'
  network_ids: string[]
  priority: number
  created_at: string
  updated_at: string
}

interface DNSRule {
  id: string
  network_id: string
  domain: string
  dns_server: string
  is_active: boolean
  created_at: string
}

type Tab = 'trust' | 'policies' | 'dns'

function TrustLevelBadge({ level, score }: { level: string; score: number }) {
  const variant = level === 'high' ? 'online'
    : level === 'medium' ? 'pending'
    : level === 'low' ? 'offline'
    : 'offline'

  return (
    <div className="flex items-center gap-2">
      <div className="w-16 h-2 rounded-full bg-[var(--bg-tertiary)] overflow-hidden">
        <div
          className="h-full rounded-full transition-all"
          style={{
            width: `${score}%`,
            background: level === 'high' ? 'var(--success)'
              : level === 'medium' ? 'var(--warning)'
              : 'var(--danger)',
          }}
        />
      </div>
      <Badge variant={variant}>{score}</Badge>
    </div>
  )
}

export default function ZTNAPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)

  const [activeTab, setActiveTab] = useState<Tab>('trust')
  const [showConfigModal, setShowConfigModal] = useState(false)
  const [showCreatePolicy, setShowCreatePolicy] = useState(false)
  const [showCreateDNS, setShowCreateDNS] = useState(false)
  const [deletePolicyId, setDeletePolicyId] = useState<string | null>(null)
  const [deleteDNSId, setDeleteDNSId] = useState<string | null>(null)

  const [policyForm, setPolicyForm] = useState({ name: '', description: '', action: 'allow', priority: '100' })
  const [dnsForm, setDnsForm] = useState({ network_id: '', domain: '', dns_server: '' })

  // --- Queries ---

  const { data: trustScores = [], isLoading: scoresLoading } = useQuery<TrustScoreSummary[]>({
    queryKey: ['trust-scores'],
    queryFn: () => api.get('/ztna/trust-scores'),
  })

  const { data: trustConfig } = useQuery<TrustConfig>({
    queryKey: ['trust-config'],
    queryFn: () => api.get('/ztna/trust-config'),
  })

  const { data: policies = [], isLoading: policiesLoading } = useQuery<ZTNAPolicy[]>({
    queryKey: ['ztna-policies'],
    queryFn: () => api.get('/ztna/policies'),
  })

  const { data: dnsRules = [], isLoading: dnsLoading } = useQuery<DNSRule[]>({
    queryKey: ['dns-rules'],
    queryFn: () => api.get('/ztna/dns-rules'),
  })

  // --- Mutations ---

  const updateConfigMutation = useMutation({
    mutationFn: (config: TrustConfig) => api.put('/ztna/trust-config', config),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['trust-config'] })
      setShowConfigModal(false)
      addToast(t('ztna.configSaved'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const createPolicyMutation = useMutation({
    mutationFn: (data: { name: string; description?: string; action: string; priority: number }) =>
      api.post('/ztna/policies', data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ztna-policies'] })
      setShowCreatePolicy(false)
      setPolicyForm({ name: '', description: '', action: 'allow', priority: '100' })
      addToast(t('ztna.policyCreated'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const deletePolicyMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/ztna/policies/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ztna-policies'] })
      setDeletePolicyId(null)
      addToast(t('ztna.policyDeleted'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const createDNSMutation = useMutation({
    mutationFn: (data: { network_id: string; domain: string; dns_server: string }) =>
      api.post('/ztna/dns-rules', data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns-rules'] })
      setShowCreateDNS(false)
      setDnsForm({ network_id: '', domain: '', dns_server: '' })
      addToast(t('ztna.dnsRuleCreated'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const deleteDNSMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/ztna/dns-rules/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dns-rules'] })
      setDeleteDNSId(null)
      addToast(t('ztna.dnsRuleDeleted'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  // --- Trust score stats ---
  const highCount = trustScores.filter((s) => s.level === 'high').length
  const mediumCount = trustScores.filter((s) => s.level === 'medium').length
  const lowCount = trustScores.filter((s) => s.level === 'low').length
  const criticalCount = trustScores.filter((s) => s.level === 'critical').length
  const avgScore = trustScores.length > 0
    ? Math.round(trustScores.reduce((sum, s) => sum + s.score, 0) / trustScores.length)
    : 0

  // --- Columns ---

  const trustColumns = [
    {
      key: 'device_name',
      header: t('devices.name'),
      sortable: true,
      render: (row: TrustScoreSummary) => (
        <span className="font-mono text-[var(--text-primary)]">{row.device_name}</span>
      ),
    },
    {
      key: 'username',
      header: t('devices.owner'),
      sortable: true,
      render: (row: TrustScoreSummary) => (
        <span className="font-mono text-[var(--accent)]">{row.username}</span>
      ),
    },
    {
      key: 'score',
      header: t('ztna.trustScore'),
      sortable: true,
      render: (row: TrustScoreSummary) => <TrustLevelBadge level={row.level} score={row.score} />,
    },
    {
      key: 'level',
      header: t('ztna.trustLevel'),
      render: (row: TrustScoreSummary) => {
        const labels: Record<string, string> = {
          high: t('ztna.levelHigh'),
          medium: t('ztna.levelMedium'),
          low: t('ztna.levelLow'),
          critical: t('ztna.levelCritical'),
        }
        return <span className="text-sm">{labels[row.level] || row.level}</span>
      },
    },
    {
      key: 'evaluated_at',
      header: t('ztna.lastEvaluated'),
      render: (row: TrustScoreSummary) => (
        <span className="text-[var(--text-muted)] font-mono text-xs">
          {new Date(row.evaluated_at).toLocaleString()}
        </span>
      ),
    },
  ]

  const policyColumns = [
    {
      key: 'name',
      header: t('common.name'),
      sortable: true,
      render: (row: ZTNAPolicy) => (
        <span className="font-mono text-[var(--text-primary)]">{row.name}</span>
      ),
    },
    {
      key: 'action',
      header: t('ztna.action'),
      render: (row: ZTNAPolicy) => {
        const variant = row.action === 'allow' ? 'online' : row.action === 'restrict' ? 'pending' : 'offline'
        return <Badge variant={variant}>{row.action}</Badge>
      },
    },
    {
      key: 'priority',
      header: t('ztna.priority'),
      sortable: true,
      render: (row: ZTNAPolicy) => (
        <span className="font-mono text-xs">{row.priority}</span>
      ),
    },
    {
      key: 'is_active',
      header: t('common.status'),
      render: (row: ZTNAPolicy) => (
        <Badge variant={row.is_active ? 'online' : 'offline'}>
          {row.is_active ? t('status.active') : t('status.inactive')}
        </Badge>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row: ZTNAPolicy) => (
        <button
          onClick={(e) => { e.stopPropagation(); setDeletePolicyId(row.id) }}
          className="text-[var(--text-muted)] hover:text-[var(--danger)] transition-colors"
        >
          <Trash2 size={16} />
        </button>
      ),
    },
  ]

  const dnsColumns = [
    {
      key: 'domain',
      header: t('ztna.domain'),
      sortable: true,
      render: (row: DNSRule) => (
        <span className="font-mono text-[var(--text-primary)]">{row.domain}</span>
      ),
    },
    {
      key: 'dns_server',
      header: t('ztna.dnsServer'),
      render: (row: DNSRule) => (
        <span className="font-mono text-[var(--accent)] text-sm">{row.dns_server}</span>
      ),
    },
    {
      key: 'is_active',
      header: t('common.status'),
      render: (row: DNSRule) => (
        <Badge variant={row.is_active ? 'online' : 'offline'}>
          {row.is_active ? t('status.active') : t('status.inactive')}
        </Badge>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row: DNSRule) => (
        <button
          onClick={(e) => { e.stopPropagation(); setDeleteDNSId(row.id) }}
          className="text-[var(--text-muted)] hover:text-[var(--danger)] transition-colors"
        >
          <Trash2 size={16} />
        </button>
      ),
    },
  ]

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('ztna.title')}
        </h1>
        <div className="flex items-center gap-2">
          {activeTab === 'trust' && (
            <Button variant="secondary" onClick={() => setShowConfigModal(true)}>
              <Settings2 size={16} className="mr-1" /> {t('ztna.configure')}
            </Button>
          )}
          {activeTab === 'policies' && (
            <Button onClick={() => { createPolicyMutation.reset(); setShowCreatePolicy(true) }}>
              <Plus size={16} className="mr-1" /> {t('ztna.createPolicy')}
            </Button>
          )}
          {activeTab === 'dns' && (
            <Button onClick={() => { createDNSMutation.reset(); setShowCreateDNS(true) }}>
              <Plus size={16} className="mr-1" /> {t('ztna.addDNSRule')}
            </Button>
          )}
        </div>
      </div>

      {/* Summary cards */}
      {activeTab === 'trust' && (
        <div className="grid grid-cols-5 gap-4 mb-6">
          <Card className="p-4 text-center">
            <p className="text-2xl font-mono font-bold text-[var(--accent)]">{avgScore}</p>
            <p className="text-xs text-[var(--text-muted)] mt-1">{t('ztna.avgScore')}</p>
          </Card>
          <Card className="p-4 text-center">
            <p className="text-2xl font-mono font-bold text-[var(--success)]">{highCount}</p>
            <p className="text-xs text-[var(--text-muted)] mt-1">{t('ztna.levelHigh')}</p>
          </Card>
          <Card className="p-4 text-center">
            <p className="text-2xl font-mono font-bold text-[var(--warning)]">{mediumCount}</p>
            <p className="text-xs text-[var(--text-muted)] mt-1">{t('ztna.levelMedium')}</p>
          </Card>
          <Card className="p-4 text-center">
            <p className="text-2xl font-mono font-bold text-[var(--danger)]">{lowCount}</p>
            <p className="text-xs text-[var(--text-muted)] mt-1">{t('ztna.levelLow')}</p>
          </Card>
          <Card className="p-4 text-center">
            <p className="text-2xl font-mono font-bold text-[var(--danger)]">{criticalCount}</p>
            <p className="text-xs text-[var(--text-muted)] mt-1">{t('ztna.levelCritical')}</p>
          </Card>
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-1 mb-4 border-b border-[var(--border)]">
        {(['trust', 'policies', 'dns'] as Tab[]).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-px cursor-pointer ${
              activeTab === tab
                ? 'border-[var(--accent)] text-[var(--accent)]'
                : 'border-transparent text-[var(--text-muted)] hover:text-[var(--text-primary)]'
            }`}
          >
            {t(`ztna.tab${tab.charAt(0).toUpperCase() + tab.slice(1)}`)}
          </button>
        ))}
      </div>

      {/* Trust Tab */}
      {activeTab === 'trust' && (
        scoresLoading ? (
          <Card className="p-8 text-center text-[var(--text-muted)]">{t('common.loading')}</Card>
        ) : trustScores.length === 0 ? (
          <Card className="flex flex-col items-center justify-center py-16">
            <div className="rounded-full p-4 mb-4" style={{ background: 'rgba(0,255,136,0.08)' }}>
              <Shield size={40} className="text-[var(--accent)]" />
            </div>
            <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-2 font-mono">
              {t('ztna.noScores')}
            </h2>
            <p className="text-sm text-[var(--text-muted)]">{t('ztna.noScoresDesc')}</p>
          </Card>
        ) : (
          <Table columns={trustColumns} data={trustScores} />
        )
      )}

      {/* Policies Tab */}
      {activeTab === 'policies' && (
        policiesLoading ? (
          <Card className="p-8 text-center text-[var(--text-muted)]">{t('common.loading')}</Card>
        ) : policies.length === 0 ? (
          <Card className="flex flex-col items-center justify-center py-16">
            <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-2 font-mono">
              {t('ztna.noPolicies')}
            </h2>
            <Button className="mt-4" onClick={() => { createPolicyMutation.reset(); setShowCreatePolicy(true) }}>
              <Plus size={16} className="mr-1" /> {t('ztna.createPolicy')}
            </Button>
          </Card>
        ) : (
          <Table columns={policyColumns} data={policies} />
        )
      )}

      {/* DNS Tab */}
      {activeTab === 'dns' && (
        dnsLoading ? (
          <Card className="p-8 text-center text-[var(--text-muted)]">{t('common.loading')}</Card>
        ) : dnsRules.length === 0 ? (
          <Card className="flex flex-col items-center justify-center py-16">
            <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-2 font-mono">
              {t('ztna.noDNSRules')}
            </h2>
            <Button className="mt-4" onClick={() => { createDNSMutation.reset(); setShowCreateDNS(true) }}>
              <Plus size={16} className="mr-1" /> {t('ztna.addDNSRule')}
            </Button>
          </Card>
        ) : (
          <Table columns={dnsColumns} data={dnsRules} />
        )
      )}

      {/* Trust Config Modal */}
      {trustConfig && (
        <TrustConfigModal
          open={showConfigModal}
          config={trustConfig}
          onClose={() => setShowConfigModal(false)}
          onSave={(config) => updateConfigMutation.mutate(config)}
          isPending={updateConfigMutation.isPending}
          t={t}
        />
      )}

      {/* Create Policy Modal */}
      <Modal open={showCreatePolicy} title={t('ztna.createPolicy')} onClose={() => setShowCreatePolicy(false)}>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            createPolicyMutation.mutate({
              name: policyForm.name,
              description: policyForm.description || undefined,
              action: policyForm.action,
              priority: Number.isNaN(parseInt(policyForm.priority)) ? 100 : parseInt(policyForm.priority),
            })
          }}
          className="flex flex-col gap-4"
        >
          <Input
            label={t('common.name')}
            value={policyForm.name}
            onChange={(e) => setPolicyForm({ ...policyForm, name: e.target.value })}
            required
          />
          <Input
            label={t('common.description')}
            value={policyForm.description}
            onChange={(e) => setPolicyForm({ ...policyForm, description: e.target.value })}
          />
          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
              {t('ztna.action')}
            </label>
            <select
              value={policyForm.action}
              onChange={(e) => setPolicyForm({ ...policyForm, action: e.target.value })}
              className="w-full rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono"
            >
              <option value="allow">{t('ztna.actionAllow')}</option>
              <option value="restrict">{t('ztna.actionRestrict')}</option>
              <option value="deny">{t('ztna.actionDeny')}</option>
            </select>
          </div>
          <Input
            label={t('ztna.priority')}
            type="number"
            value={policyForm.priority}
            onChange={(e) => setPolicyForm({ ...policyForm, priority: e.target.value })}
          />
          <div className="flex justify-end gap-2 mt-2">
            <Button variant="ghost" type="button" onClick={() => setShowCreatePolicy(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={createPolicyMutation.isPending}>
              {createPolicyMutation.isPending ? t('common.loading') : t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delete Policy Modal */}
      <Modal open={!!deletePolicyId} title={t('common.delete')} onClose={() => setDeletePolicyId(null)}>
        <p className="text-sm text-[var(--text-secondary)] mb-4">{t('ztna.confirmDeletePolicy')}</p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setDeletePolicyId(null)}>{t('common.cancel')}</Button>
          <Button
            variant="danger"
            onClick={() => deletePolicyMutation.mutate(deletePolicyId!)}
            disabled={deletePolicyMutation.isPending}
          >
            {deletePolicyMutation.isPending ? t('common.loading') : t('common.delete')}
          </Button>
        </div>
      </Modal>

      {/* Create DNS Rule Modal */}
      <Modal open={showCreateDNS} title={t('ztna.addDNSRule')} onClose={() => setShowCreateDNS(false)}>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            createDNSMutation.mutate(dnsForm)
          }}
          className="flex flex-col gap-4"
        >
          <Input
            label={t('ztna.domain')}
            placeholder="*.corp.local"
            value={dnsForm.domain}
            onChange={(e) => setDnsForm({ ...dnsForm, domain: e.target.value })}
            required
          />
          <Input
            label={t('ztna.dnsServer')}
            placeholder="10.0.0.1"
            value={dnsForm.dns_server}
            onChange={(e) => setDnsForm({ ...dnsForm, dns_server: e.target.value })}
            required
          />
          <Input
            label={t('ztna.networkId')}
            placeholder="UUID"
            value={dnsForm.network_id}
            onChange={(e) => setDnsForm({ ...dnsForm, network_id: e.target.value })}
            required
          />
          <div className="flex justify-end gap-2 mt-2">
            <Button variant="ghost" type="button" onClick={() => setShowCreateDNS(false)}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={createDNSMutation.isPending}>
              {createDNSMutation.isPending ? t('common.loading') : t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delete DNS Rule Modal */}
      <Modal open={!!deleteDNSId} title={t('common.delete')} onClose={() => setDeleteDNSId(null)}>
        <p className="text-sm text-[var(--text-secondary)] mb-4">{t('ztna.confirmDeleteDNS')}</p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setDeleteDNSId(null)}>{t('common.cancel')}</Button>
          <Button
            variant="danger"
            onClick={() => deleteDNSMutation.mutate(deleteDNSId!)}
            disabled={deleteDNSMutation.isPending}
          >
            {deleteDNSMutation.isPending ? t('common.loading') : t('common.delete')}
          </Button>
        </div>
      </Modal>
    </div>
  )
}

// --- Trust Config Modal ---

function TrustConfigModal({
  open, config, onClose, onSave, isPending, t,
}: {
  open: boolean
  config: TrustConfig
  onClose: () => void
  onSave: (config: TrustConfig) => void
  isPending: boolean
  t: (key: string) => string
}) {
  const [form, setForm] = useState<TrustConfig>(config)

  const totalWeight = form.weight_disk_encryption + form.weight_screen_lock +
    form.weight_antivirus + form.weight_firewall +
    form.weight_os_version + form.weight_mfa

  return (
    <Modal open={open} title={t('ztna.trustConfig')} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          onSave(form)
        }}
        className="flex flex-col gap-4"
      >
        <p className="text-xs text-[var(--text-muted)]">{t('ztna.weightsMustSum100')}</p>

        <div className="grid grid-cols-2 gap-3">
          <Input
            label={t('ztna.weightDisk')}
            type="number"
            value={String(form.weight_disk_encryption)}
            onChange={(e) => setForm({ ...form, weight_disk_encryption: parseInt(e.target.value) || 0 })}
          />
          <Input
            label={t('ztna.weightScreen')}
            type="number"
            value={String(form.weight_screen_lock)}
            onChange={(e) => setForm({ ...form, weight_screen_lock: parseInt(e.target.value) || 0 })}
          />
          <Input
            label={t('ztna.weightAntivirus')}
            type="number"
            value={String(form.weight_antivirus)}
            onChange={(e) => setForm({ ...form, weight_antivirus: parseInt(e.target.value) || 0 })}
          />
          <Input
            label={t('ztna.weightFirewall')}
            type="number"
            value={String(form.weight_firewall)}
            onChange={(e) => setForm({ ...form, weight_firewall: parseInt(e.target.value) || 0 })}
          />
          <Input
            label={t('ztna.weightOS')}
            type="number"
            value={String(form.weight_os_version)}
            onChange={(e) => setForm({ ...form, weight_os_version: parseInt(e.target.value) || 0 })}
          />
          <Input
            label={t('ztna.weightMFA')}
            type="number"
            value={String(form.weight_mfa)}
            onChange={(e) => setForm({ ...form, weight_mfa: parseInt(e.target.value) || 0 })}
          />
        </div>

        <div className={`text-xs font-mono ${totalWeight === 100 ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>
          {t('ztna.totalWeight')}: {totalWeight}/100
        </div>

        <h4 className="text-sm font-medium text-[var(--text-primary)] mt-2">{t('ztna.thresholds')}</h4>
        <div className="grid grid-cols-3 gap-3">
          <Input
            label={t('ztna.levelHigh')}
            type="number"
            value={String(form.threshold_high)}
            onChange={(e) => setForm({ ...form, threshold_high: parseInt(e.target.value) || 0 })}
          />
          <Input
            label={t('ztna.levelMedium')}
            type="number"
            value={String(form.threshold_medium)}
            onChange={(e) => setForm({ ...form, threshold_medium: parseInt(e.target.value) || 0 })}
          />
          <Input
            label={t('ztna.levelLow')}
            type="number"
            value={String(form.threshold_low)}
            onChange={(e) => setForm({ ...form, threshold_low: parseInt(e.target.value) || 0 })}
          />
        </div>

        <div className="space-y-2 mt-2">
          <label className="flex items-center gap-2 text-sm text-[var(--text-secondary)]">
            <input
              type="checkbox"
              checked={form.auto_restrict_below_medium}
              onChange={(e) => setForm({ ...form, auto_restrict_below_medium: e.target.checked })}
              className="rounded border-[var(--border)]"
            />
            {t('ztna.autoRestrict')}
          </label>
          <label className="flex items-center gap-2 text-sm text-[var(--text-secondary)]">
            <input
              type="checkbox"
              checked={form.auto_block_below_low}
              onChange={(e) => setForm({ ...form, auto_block_below_low: e.target.checked })}
              className="rounded border-[var(--border)]"
            />
            {t('ztna.autoBlock')}
          </label>
        </div>

        <div className="flex justify-end gap-2 mt-4">
          <Button variant="ghost" type="button" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" disabled={isPending || totalWeight !== 100}>
            {isPending ? t('common.loading') : t('common.save')}
          </Button>
        </div>
      </form>
    </Modal>
  )
}
