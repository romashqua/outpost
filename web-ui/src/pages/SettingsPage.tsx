import { useState, useCallback, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { clsx } from 'clsx'
import Card from '@/components/ui/Card'
import Input from '@/components/ui/Input'
import Button from '@/components/ui/Button'
import Badge from '@/components/ui/Badge'
import Modal from '@/components/ui/Modal'
import { api } from '@/api/client'
import { useToastStore } from '@/store/toast'

interface MfaStatus {
  totp_enabled: boolean
  webauthn_enabled: boolean
  backup_codes_remaining: number
}

interface TotpSetupResponse {
  secret: string
  qr_url: string
}

interface WebAuthnCredential {
  id: string
  name: string
  created_at: string
}

const tabs = ['general', 'auth', 'security', 'wireguard', 'smtp', 'integrations'] as const
type Tab = typeof tabs[number]

interface SettingsData {
  orgName: string
  domain: string
  sessionTimeout: string
  // OIDC
  oidcEnabled: boolean
  oidcIssuerUrl: string
  oidcClientId: string
  oidcClientSecret: string
  oidcRedirectUri: string
  // LDAP
  ldapEnabled: boolean
  ldapServerUrl: string
  ldapBindDn: string
  ldapBaseDn: string
  ldapUserFilter: string
  // SAML
  samlEnabled: boolean
  samlEntityId: string
  samlIdpMetadataUrl: string
  samlAcsUrl: string
  // General auth
  mfaRequired: boolean
  // WireGuard
  wgPort: string
  wgMtu: string
  keepalive: string
  dns: string
  // SMTP
  smtpHost: string
  smtpPort: string
  smtpFrom: string
}

const defaultSettings: SettingsData = {
  orgName: 'Outpost Corp',
  domain: 'vpn.outpost.local',
  sessionTimeout: '3600',
  oidcEnabled: false,
  oidcIssuerUrl: '',
  oidcClientId: '',
  oidcClientSecret: '',
  oidcRedirectUri: '',
  ldapEnabled: false,
  ldapServerUrl: '',
  ldapBindDn: '',
  ldapBaseDn: '',
  ldapUserFilter: '(uid={username})',
  samlEnabled: false,
  samlEntityId: '',
  samlIdpMetadataUrl: '',
  samlAcsUrl: '',
  mfaRequired: true,
  wgPort: '51820',
  wgMtu: '1420',
  keepalive: '25',
  dns: '1.1.1.1, 8.8.8.8',
  smtpHost: '',
  smtpPort: '587',
  smtpFrom: '',
}

function str(v: unknown): string {
  if (v === null || v === undefined) return ''
  return String(v)
}

function parseSettingsFromApi(data: Record<string, unknown>): SettingsData {
  return {
    orgName: str(data['orgName']) || defaultSettings.orgName,
    domain: str(data['domain']) || defaultSettings.domain,
    sessionTimeout: str(data['sessionTimeout']) || defaultSettings.sessionTimeout,
    oidcEnabled: str(data['oidcEnabled']) === 'true',
    oidcIssuerUrl: str(data['oidcIssuerUrl']),
    oidcClientId: str(data['oidcClientId']),
    oidcClientSecret: str(data['oidcClientSecret']),
    oidcRedirectUri: str(data['oidcRedirectUri']),
    ldapEnabled: str(data['ldapEnabled']) === 'true',
    ldapServerUrl: str(data['ldapServerUrl']),
    ldapBindDn: str(data['ldapBindDn']),
    ldapBaseDn: str(data['ldapBaseDn']),
    ldapUserFilter: str(data['ldapUserFilter']) || defaultSettings.ldapUserFilter,
    samlEnabled: str(data['samlEnabled']) === 'true',
    samlEntityId: str(data['samlEntityId']),
    samlIdpMetadataUrl: str(data['samlIdpMetadataUrl']),
    samlAcsUrl: str(data['samlAcsUrl']),
    mfaRequired: str(data['mfaRequired']) === 'true' || (data['mfaRequired'] === undefined && defaultSettings.mfaRequired),
    wgPort: str(data['wgPort']) || defaultSettings.wgPort,
    wgMtu: str(data['wgMtu']) || defaultSettings.wgMtu,
    keepalive: str(data['keepalive']) || defaultSettings.keepalive,
    dns: str(data['dns']) || defaultSettings.dns,
    smtpHost: str(data['smtpHost']),
    smtpPort: str(data['smtpPort']) || defaultSettings.smtpPort,
    smtpFrom: str(data['smtpFrom']),
  }
}

function settingsToPayload(data: SettingsData): Record<string, string> {
  return {
    orgName: data.orgName,
    domain: data.domain,
    sessionTimeout: data.sessionTimeout,
    oidcEnabled: String(data.oidcEnabled),
    oidcIssuerUrl: data.oidcIssuerUrl,
    oidcClientId: data.oidcClientId,
    oidcClientSecret: data.oidcClientSecret,
    oidcRedirectUri: data.oidcRedirectUri,
    ldapEnabled: String(data.ldapEnabled),
    ldapServerUrl: data.ldapServerUrl,
    ldapBindDn: data.ldapBindDn,
    ldapBaseDn: data.ldapBaseDn,
    ldapUserFilter: data.ldapUserFilter,
    samlEnabled: String(data.samlEnabled),
    samlEntityId: data.samlEntityId,
    samlIdpMetadataUrl: data.samlIdpMetadataUrl,
    samlAcsUrl: data.samlAcsUrl,
    mfaRequired: String(data.mfaRequired),
    wgPort: data.wgPort,
    wgMtu: data.wgMtu,
    keepalive: data.keepalive,
    dns: data.dns,
    smtpHost: data.smtpHost,
    smtpPort: data.smtpPort,
    smtpFrom: data.smtpFrom,
  }
}

interface WebhookSubscription {
  id: string
  url: string
  secret: string
  events: string[]
  is_active: boolean
  created_at: string
}

const integrationTemplates = [
  { name: 'Slack', desc: 'Send notifications to Slack channels via incoming webhook', urlPlaceholder: 'https://hooks.slack.com/services/T.../B.../...' },
  { name: 'Telegram', desc: 'Alert notifications via Telegram Bot API', urlPlaceholder: 'https://api.telegram.org/bot<TOKEN>/sendMessage?chat_id=<ID>' },
  { name: 'Grafana', desc: 'Forward events to Grafana OnCall', urlPlaceholder: 'https://oncall-prod.grafana.net/integrations/v1/...' },
  { name: 'Syslog / SIEM', desc: 'Forward audit logs to syslog/SIEM endpoint', urlPlaceholder: 'https://siem.corp.local/api/events' },
  { name: 'Custom', desc: 'Generic outbound webhook to any HTTP endpoint', urlPlaceholder: 'https://example.com/webhook' },
]

function IntegrationsTab() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)
  const [showAddModal, setShowAddModal] = useState(false)
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [formUrl, setFormUrl] = useState('')
  const [formSecret, setFormSecret] = useState('')
  const [formEvents, setFormEvents] = useState('*')
  const [selectedTemplate, setSelectedTemplate] = useState<string | null>(null)

  const { data: webhooks = [], isLoading } = useQuery<WebhookSubscription[]>({
    queryKey: ['webhooks'],
    queryFn: () => api.get('/webhooks'),
  })

  const createMutation = useMutation({
    mutationFn: (body: { url: string; secret: string; events: string[] }) =>
      api.post('/webhooks', body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['webhooks'] })
      setShowAddModal(false)
      resetForm()
      addToast('Webhook created', 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/webhooks/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['webhooks'] })
      setDeleteId(null)
      addToast('Webhook deleted', 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const testMutation = useMutation({
    mutationFn: (id: string) => api.post(`/webhooks/${id}/test`),
    onSuccess: () => addToast('Test event sent', 'success'),
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  function resetForm() {
    setFormUrl('')
    setFormSecret('')
    setFormEvents('*')
    setSelectedTemplate(null)
  }

  function handleAddSubmit(e: React.FormEvent) {
    e.preventDefault()
    const events = formEvents.split(',').map((s) => s.trim()).filter(Boolean)
    createMutation.mutate({ url: formUrl, secret: formSecret || 'auto', events })
  }

  return (
    <div className="space-y-6 max-w-2xl">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium text-[var(--text-primary)] font-mono">{t('settings.webhooks', 'Webhooks & Integrations')}</h3>
          <p className="text-xs text-[var(--text-muted)] mt-1">Outbound webhooks for event notifications (HMAC-SHA256 signed)</p>
        </div>
        <Button onClick={() => { createMutation.reset(); setShowAddModal(true) }}>
          + Add Webhook
        </Button>
      </div>

      {/* Quick-add templates */}
      <div className="grid grid-cols-3 gap-3">
        {integrationTemplates.map((tmpl) => (
          <button
            type="button"
            key={tmpl.name}
            className="rounded-md border border-[var(--border)] p-3 hover:border-[var(--accent)] transition-colors cursor-pointer text-left"
            onClick={() => {
              setSelectedTemplate(tmpl.name)
              setFormUrl('')
              setFormSecret('')
              setFormEvents('*')
              createMutation.reset()
              setShowAddModal(true)
            }}
          >
            <span className="text-sm font-medium text-[var(--text-primary)]">{tmpl.name}</span>
            <p className="text-[10px] text-[var(--text-muted)] mt-0.5 leading-tight">{tmpl.desc}</p>
          </button>
        ))}
      </div>

      {/* Active webhooks */}
      {isLoading ? (
        <p className="text-sm text-[var(--text-muted)]">Loading...</p>
      ) : webhooks.length === 0 ? (
        <p className="text-sm text-[var(--text-muted)] py-4 text-center">No webhooks configured. Click a template above or "Add Webhook" to create one.</p>
      ) : (
        <div className="space-y-2">
          {webhooks.map((wh) => (
            <div
              key={wh.id}
              className="flex items-center justify-between rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-4 py-3"
            >
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <Badge variant={wh.is_active ? 'online' : 'offline'} pulse>
                    {wh.is_active ? 'Active' : 'Inactive'}
                  </Badge>
                  <code className="text-xs text-[var(--accent)] truncate">{wh.url}</code>
                </div>
                <div className="flex gap-2 mt-1">
                  <span className="text-[10px] text-[var(--text-muted)]">
                    Events: {wh.events.join(', ')}
                  </span>
                  <span className="text-[10px] text-[var(--text-muted)]">
                    Created: {new Date(wh.created_at).toLocaleDateString()}
                  </span>
                </div>
              </div>
              <div className="flex gap-1 ml-3">
                <Button
                  variant="secondary"
                  size="sm"
                  disabled={testMutation.isPending}
                  onClick={() => testMutation.mutate(wh.id)}
                >
                  Test
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => { deleteMutation.reset(); setDeleteId(wh.id) }}
                >
                  {t('common.delete')}
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Add Webhook Modal */}
      <Modal open={showAddModal} onClose={() => { setShowAddModal(false); resetForm() }} title={selectedTemplate ? `Add ${selectedTemplate} Webhook` : 'Add Webhook'}>
        <form className="flex flex-col gap-4" onSubmit={handleAddSubmit}>
          <Input
            label="Webhook URL"
            placeholder={integrationTemplates.find((t) => t.name === selectedTemplate)?.urlPlaceholder || 'https://...'}
            value={formUrl}
            onChange={(e) => setFormUrl(e.target.value)}
            required
          />
          <Input
            label="Signing Secret (optional — auto-generated if blank)"
            placeholder="my-secret-key"
            value={formSecret}
            onChange={(e) => setFormSecret(e.target.value)}
          />
          <Input
            label="Events (comma-separated, * for all)"
            placeholder="*, user.created, device.approved"
            value={formEvents}
            onChange={(e) => setFormEvents(e.target.value)}
          />
          {createMutation.error && (
            <p className="text-sm text-[var(--danger)]">{(createMutation.error as Error).message}</p>
          )}
          <div className="flex gap-3 justify-end mt-2">
            <Button variant="secondary" type="button" onClick={() => { setShowAddModal(false); resetForm() }}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={createMutation.isPending}>
              {createMutation.isPending ? 'Creating...' : t('common.create')}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Delete Confirmation */}
      <Modal open={deleteId !== null} onClose={() => setDeleteId(null)} title="Delete Webhook">
        <p className="text-sm text-[var(--text-secondary)] mb-6">
          Are you sure you want to delete this webhook? This action cannot be undone.
        </p>
        {deleteMutation.error && (
          <p className="text-sm text-[var(--danger)] mb-4">{(deleteMutation.error as Error).message}</p>
        )}
        <div className="flex gap-3 justify-end">
          <Button variant="secondary" onClick={() => setDeleteId(null)}>
            {t('common.cancel')}
          </Button>
          <Button
            variant="danger"
            disabled={deleteMutation.isPending}
            onClick={() => deleteId && deleteMutation.mutate(deleteId)}
          >
            {deleteMutation.isPending ? 'Deleting...' : t('common.delete')}
          </Button>
        </div>
      </Modal>
    </div>
  )
}

export default function SettingsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const addToast = useToastStore((s) => s.addToast)
  const [activeTab, setActiveTab] = useState<Tab>('general')
  const [settings, setSettings] = useState<SettingsData>(defaultSettings)

  const { data: settingsData, isLoading } = useQuery<Record<string, unknown>>({
    queryKey: ['settings'],
    queryFn: () => api.get('/settings'),
  })

  useEffect(() => {
    if (settingsData) {
      setSettings(parseSettingsFromApi(settingsData))
    }
  }, [settingsData])

  const saveMutation = useMutation({
    mutationFn: (data: SettingsData) =>
      api.put('/settings', settingsToPayload(data)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings'] })
      addToast('Settings saved', 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const smtpTestMutation = useMutation({
    mutationFn: () => api.post('/settings/smtp/test'),
    onSuccess: () => {
      addToast('SMTP test email sent successfully', 'success')
    },
    onError: (err) => {
      addToast((err as Error).message, 'error')
    },
  })

  const update = useCallback(<K extends keyof SettingsData>(key: K, value: SettingsData[K]) => {
    setSettings((prev) => ({ ...prev, [key]: value }))
  }, [])

  const handleSave = useCallback((e: React.FormEvent) => {
    e.preventDefault()
    saveMutation.mutate(settings)
  }, [settings, saveMutation])

  // MFA / Security state
  const [totpCode, setTotpCode] = useState('')
  const [totpSetup, setTotpSetup] = useState<TotpSetupResponse | null>(null)
  const [backupCodes, setBackupCodes] = useState<string[]>([])
  const [showDisableConfirm, setShowDisableConfirm] = useState(false)

  const { data: mfaStatus, refetch: refetchMfaStatus } = useQuery<MfaStatus>({
    queryKey: ['mfa-status'],
    queryFn: () => api.get('/mfa/status'),
    enabled: activeTab === 'security',
  })

  const { data: webauthnCreds } = useQuery<WebAuthnCredential[]>({
    queryKey: ['webauthn-credentials'],
    queryFn: () => api.get('/mfa/webauthn/credentials'),
    enabled: activeTab === 'security',
  })

  const setupTotpMutation = useMutation({
    mutationFn: () => api.post<TotpSetupResponse>('/mfa/totp/setup', { issuer: 'Outpost VPN' }),
    onSuccess: (data) => {
      setTotpSetup(data)
      addToast(t('settings.scanQr'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const verifyTotpMutation = useMutation({
    mutationFn: (code: string) => api.post<{ valid: boolean }>('/mfa/totp/verify', { code }),
    onSuccess: (data) => {
      if (data.valid) {
        setTotpSetup(null)
        setTotpCode('')
        refetchMfaStatus()
        addToast(t('settings.totpEnabled'), 'success')
      } else {
        addToast('Invalid code', 'error')
      }
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const disableTotpMutation = useMutation({
    mutationFn: () => api.delete('/mfa/totp'),
    onSuccess: () => {
      setShowDisableConfirm(false)
      refetchMfaStatus()
      addToast(t('settings.totpDisabled'), 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const generateCodesMutation = useMutation({
    mutationFn: () => api.post<{ codes: string[] }>('/mfa/backup-codes'),
    onSuccess: (data) => {
      setBackupCodes(data.codes)
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const deleteWebAuthnMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/mfa/webauthn/credentials/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['webauthn-credentials'] })
      addToast('Credential removed', 'success')
    },
    onError: (err) => addToast((err as Error).message, 'error'),
  })

  const smtpConnected = !!(settings.smtpHost && settings.smtpPort)
  const oidcConnected = settings.oidcEnabled && !!settings.oidcIssuerUrl
  const ldapConnected = settings.ldapEnabled && !!settings.ldapServerUrl
  const samlConnected = settings.samlEnabled && !!settings.samlIdpMetadataUrl

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
        <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
        {t('settings.title')}
      </h1>

      {/* Connection Status Badges */}
      <div className="flex gap-3 mb-6">
        <div className="flex items-center gap-1.5">
          <span className="text-xs text-[var(--text-muted)]">SMTP:</span>
          <Badge variant={smtpConnected ? 'online' : 'offline'} pulse>
            {smtpConnected ? 'Connected' : 'Not configured'}
          </Badge>
        </div>
        <div className="flex items-center gap-1.5">
          <span className="text-xs text-[var(--text-muted)]">OIDC:</span>
          <Badge variant={oidcConnected ? 'online' : 'offline'} pulse>
            {oidcConnected ? 'Connected' : 'Not configured'}
          </Badge>
        </div>
        <div className="flex items-center gap-1.5">
          <span className="text-xs text-[var(--text-muted)]">LDAP:</span>
          <Badge variant={ldapConnected ? 'online' : 'offline'} pulse>
            {ldapConnected ? 'Connected' : 'Not configured'}
          </Badge>
        </div>
        <div className="flex items-center gap-1.5">
          <span className="text-xs text-[var(--text-muted)]">SAML:</span>
          <Badge variant={samlConnected ? 'online' : 'offline'} pulse>
            {samlConnected ? 'Connected' : 'Not configured'}
          </Badge>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 mb-6 border-b border-[var(--border)]">
        {tabs.map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={clsx(
              'px-4 py-2.5 text-sm font-medium transition-all border-b-2 -mb-px cursor-pointer',
              activeTab === tab
                ? 'border-[var(--accent)] text-[var(--accent)]'
                : 'border-transparent text-[var(--text-muted)] hover:text-[var(--text-secondary)]',
            )}
          >
            {t(`settings.${tab}`)}
          </button>
        ))}
      </div>

      {isLoading ? (
        <Card>
          <div className="text-center py-8 text-[var(--text-muted)]">Loading settings...</div>
        </Card>
      ) : (
        <Card>
          {activeTab === 'general' && (
            <form className="flex flex-col gap-5 max-w-lg" onSubmit={handleSave}>
              <Input
                label={t('settings.orgName')}
                value={settings.orgName}
                onChange={(e) => update('orgName', e.target.value)}
              />
              <Input
                label={t('settings.domain')}
                value={settings.domain}
                onChange={(e) => update('domain', e.target.value)}
              />
              <Input
                label={t('settings.sessionTimeout')}
                value={settings.sessionTimeout}
                onChange={(e) => update('sessionTimeout', e.target.value)}
                type="number"
              />
              <div className="mt-2">
                <Button type="submit" disabled={saveMutation.isPending}>
                  {saveMutation.isPending ? 'Saving...' : t('settings.save')}
                </Button>
              </div>
            </form>
          )}

          {activeTab === 'auth' && (
            <form className="flex flex-col gap-6 max-w-lg" onSubmit={handleSave}>
              {/* MFA */}
              <div className="flex items-center gap-3">
                <input
                  type="checkbox"
                  id="mfa-required"
                  checked={settings.mfaRequired}
                  onChange={(e) => update('mfaRequired', e.target.checked)}
                  className="h-4 w-4 rounded border-[var(--border)] bg-[var(--bg-secondary)] accent-[var(--accent)]"
                />
                <label htmlFor="mfa-required" className="text-sm text-[var(--text-secondary)]">
                  {t('settings.mfaRequired')}
                </label>
              </div>

              {/* OIDC Section */}
              <div className="border border-[var(--border)] rounded-lg p-4 space-y-4">
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-medium text-[var(--text-primary)] font-mono">OIDC Provider</h3>
                  <div className="flex items-center gap-2">
                    <Badge variant={oidcConnected ? 'online' : 'offline'} pulse>
                      {oidcConnected ? 'Active' : 'Inactive'}
                    </Badge>
                    <input
                      type="checkbox"
                      id="oidc-enabled"
                      checked={settings.oidcEnabled}
                      onChange={(e) => update('oidcEnabled', e.target.checked)}
                      className="h-4 w-4 rounded border-[var(--border)] bg-[var(--bg-secondary)] accent-[var(--accent)]"
                    />
                    <label htmlFor="oidc-enabled" className="text-xs text-[var(--text-muted)]">Enable</label>
                  </div>
                </div>
                <Input
                  label="Issuer URL"
                  placeholder="https://auth.example.com/realms/outpost"
                  value={settings.oidcIssuerUrl}
                  onChange={(e) => update('oidcIssuerUrl', e.target.value)}
                />
                <Input
                  label="Client ID"
                  placeholder="outpost-vpn"
                  value={settings.oidcClientId}
                  onChange={(e) => update('oidcClientId', e.target.value)}
                />
                <Input
                  label="Client Secret"
                  type="password"
                  placeholder="Enter client secret"
                  value={settings.oidcClientSecret}
                  onChange={(e) => update('oidcClientSecret', e.target.value)}
                />
                <Input
                  label="Redirect URI"
                  placeholder="https://vpn.outpost.local/oidc/callback"
                  value={settings.oidcRedirectUri}
                  onChange={(e) => update('oidcRedirectUri', e.target.value)}
                />
              </div>

              {/* LDAP Section */}
              <div className="border border-[var(--border)] rounded-lg p-4 space-y-4">
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-medium text-[var(--text-primary)] font-mono">LDAP / Active Directory</h3>
                  <div className="flex items-center gap-2">
                    <Badge variant={ldapConnected ? 'online' : 'offline'} pulse>
                      {ldapConnected ? 'Active' : 'Inactive'}
                    </Badge>
                    <input
                      type="checkbox"
                      id="ldap-enabled"
                      checked={settings.ldapEnabled}
                      onChange={(e) => update('ldapEnabled', e.target.checked)}
                      className="h-4 w-4 rounded border-[var(--border)] bg-[var(--bg-secondary)] accent-[var(--accent)]"
                    />
                    <label htmlFor="ldap-enabled" className="text-xs text-[var(--text-muted)]">Enable</label>
                  </div>
                </div>
                <Input
                  label="Server URL"
                  placeholder="ldap://ldap.corp.local:389"
                  value={settings.ldapServerUrl}
                  onChange={(e) => update('ldapServerUrl', e.target.value)}
                />
                <Input
                  label="Bind DN"
                  placeholder="cn=admin,dc=corp,dc=local"
                  value={settings.ldapBindDn}
                  onChange={(e) => update('ldapBindDn', e.target.value)}
                />
                <Input
                  label="Base DN"
                  placeholder="ou=users,dc=corp,dc=local"
                  value={settings.ldapBaseDn}
                  onChange={(e) => update('ldapBaseDn', e.target.value)}
                />
                <Input
                  label="User Filter"
                  placeholder="(uid={username})"
                  value={settings.ldapUserFilter}
                  onChange={(e) => update('ldapUserFilter', e.target.value)}
                />
              </div>

              {/* SAML Section */}
              <div className="border border-[var(--border)] rounded-lg p-4 space-y-4">
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-medium text-[var(--text-primary)] font-mono">SAML 2.0</h3>
                  <div className="flex items-center gap-2">
                    <Badge variant={samlConnected ? 'online' : 'offline'} pulse>
                      {samlConnected ? 'Active' : 'Inactive'}
                    </Badge>
                    <input
                      type="checkbox"
                      id="saml-enabled"
                      checked={settings.samlEnabled}
                      onChange={(e) => update('samlEnabled', e.target.checked)}
                      className="h-4 w-4 rounded border-[var(--border)] bg-[var(--bg-secondary)] accent-[var(--accent)]"
                    />
                    <label htmlFor="saml-enabled" className="text-xs text-[var(--text-muted)]">Enable</label>
                  </div>
                </div>
                <Input
                  label="Entity ID"
                  placeholder="https://vpn.outpost.local/saml/metadata"
                  value={settings.samlEntityId}
                  onChange={(e) => update('samlEntityId', e.target.value)}
                />
                <Input
                  label="IDP Metadata URL"
                  placeholder="https://idp.corp.local/metadata"
                  value={settings.samlIdpMetadataUrl}
                  onChange={(e) => update('samlIdpMetadataUrl', e.target.value)}
                />
                <Input
                  label="ACS URL"
                  placeholder="https://vpn.outpost.local/saml/acs"
                  value={settings.samlAcsUrl}
                  onChange={(e) => update('samlAcsUrl', e.target.value)}
                />
              </div>

              <div className="mt-2">
                <Button type="submit" disabled={saveMutation.isPending}>
                  {saveMutation.isPending ? 'Saving...' : t('settings.save')}
                </Button>
              </div>
            </form>
          )}

          {activeTab === 'wireguard' && (
            <form className="flex flex-col gap-5 max-w-lg" onSubmit={handleSave}>
              <Input
                label={t('settings.wgPort')}
                value={settings.wgPort}
                onChange={(e) => update('wgPort', e.target.value)}
                type="number"
              />
              <Input
                label={t('settings.wgMtu')}
                value={settings.wgMtu}
                onChange={(e) => update('wgMtu', e.target.value)}
                type="number"
              />
              <Input
                label={t('settings.keepalive')}
                value={settings.keepalive}
                onChange={(e) => update('keepalive', e.target.value)}
                type="number"
              />
              <Input
                label={t('settings.dns')}
                value={settings.dns}
                onChange={(e) => update('dns', e.target.value)}
              />
              <div className="mt-2">
                <Button type="submit" disabled={saveMutation.isPending}>
                  {saveMutation.isPending ? 'Saving...' : t('settings.save')}
                </Button>
              </div>
            </form>
          )}

          {activeTab === 'smtp' && (
            <form className="flex flex-col gap-5 max-w-lg" onSubmit={handleSave}>
              <Input
                label={t('settings.smtpHost')}
                placeholder="smtp.corp.ru"
                value={settings.smtpHost}
                onChange={(e) => update('smtpHost', e.target.value)}
              />
              <Input
                label={t('settings.smtpPort')}
                value={settings.smtpPort}
                onChange={(e) => update('smtpPort', e.target.value)}
                type="number"
              />
              <Input
                label={t('settings.smtpFrom')}
                placeholder="noreply@outpost.local"
                value={settings.smtpFrom}
                onChange={(e) => update('smtpFrom', e.target.value)}
              />
              <div className="flex gap-3 mt-2">
                <Button type="submit" disabled={saveMutation.isPending}>
                  {saveMutation.isPending ? 'Saving...' : t('settings.save')}
                </Button>
                <Button
                  type="button"
                  variant="secondary"
                  disabled={smtpTestMutation.isPending || !settings.smtpHost}
                  onClick={() => smtpTestMutation.mutate()}
                >
                  {smtpTestMutation.isPending ? 'Testing...' : 'Test SMTP'}
                </Button>
              </div>
            </form>
          )}

          {activeTab === 'security' && (
            <div className="space-y-6 max-w-lg">
              {/* TOTP Section */}
              <div className="border border-[var(--border)] rounded-lg p-4 space-y-4">
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-medium text-[var(--text-primary)] font-mono">TOTP</h3>
                  <Badge variant={mfaStatus?.totp_enabled ? 'online' : 'offline'} pulse>
                    {mfaStatus?.totp_enabled ? t('settings.totpEnabled') : t('settings.totpDisabled')}
                  </Badge>
                </div>

                {!mfaStatus?.totp_enabled && !totpSetup && (
                  <Button
                    onClick={() => setupTotpMutation.mutate()}
                    disabled={setupTotpMutation.isPending}
                  >
                    {setupTotpMutation.isPending ? 'Loading...' : t('settings.enableTotp')}
                  </Button>
                )}

                {totpSetup && (
                  <div className="space-y-4">
                    <p className="text-xs text-[var(--text-muted)]">{t('settings.scanQr')}</p>

                    <div className="rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] p-3 space-y-2">
                      <div>
                        <span className="text-xs text-[var(--text-muted)] block mb-1">Provisioning URI</span>
                        <code className="text-xs text-[var(--accent)] break-all select-all block">{totpSetup.qr_url}</code>
                      </div>
                      <div>
                        <span className="text-xs text-[var(--text-muted)] block mb-1">{t('settings.totpSecret')}</span>
                        <code className="text-sm text-[var(--text-primary)] font-mono tracking-widest select-all block">{totpSetup.secret}</code>
                      </div>
                    </div>

                    <div className="flex gap-3 items-end">
                      <div className="flex-1">
                        <Input
                          label={t('settings.verifyCode')}
                          placeholder="000000"
                          value={totpCode}
                          onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
                          maxLength={6}
                        />
                      </div>
                      <Button
                        onClick={() => verifyTotpMutation.mutate(totpCode)}
                        disabled={totpCode.length !== 6 || verifyTotpMutation.isPending}
                      >
                        {verifyTotpMutation.isPending ? 'Verifying...' : t('settings.verifyCode')}
                      </Button>
                    </div>
                  </div>
                )}

                {mfaStatus?.totp_enabled && (
                  <Button
                    variant="danger"
                    onClick={() => setShowDisableConfirm(true)}
                  >
                    {t('settings.disableTotp')}
                  </Button>
                )}
              </div>

              {/* Backup Codes Section */}
              <div className="border border-[var(--border)] rounded-lg p-4 space-y-4">
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-medium text-[var(--text-primary)] font-mono">{t('settings.backupCodes')}</h3>
                  {mfaStatus && mfaStatus.backup_codes_remaining > 0 && (
                    <Badge variant="info">
                      {mfaStatus.backup_codes_remaining} remaining
                    </Badge>
                  )}
                </div>

                <Button
                  variant="secondary"
                  onClick={() => generateCodesMutation.mutate()}
                  disabled={generateCodesMutation.isPending}
                >
                  {generateCodesMutation.isPending ? 'Generating...' : t('settings.generateCodes')}
                </Button>

                {backupCodes.length > 0 && (
                  <div className="space-y-3">
                    <p className="text-xs text-[var(--warning)]">
                      {t('settings.saveCodesWarning')}
                    </p>
                    <div className="grid grid-cols-2 gap-2">
                      {backupCodes.map((code, i) => (
                        <div
                          key={i}
                          className="flex items-center justify-between rounded border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2"
                        >
                          <code className="text-sm font-mono text-[var(--text-primary)]">{code}</code>
                          <button
                            type="button"
                            className="text-xs text-[var(--text-muted)] hover:text-[var(--accent)] transition-colors cursor-pointer ml-2"
                            onClick={() => {
                              navigator.clipboard.writeText(code)
                              addToast('Copied', 'success')
                            }}
                          >
                            Copy
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>

              {/* WebAuthn Section */}
              <div className="border border-[var(--border)] rounded-lg p-4 space-y-4">
                <div className="flex items-center justify-between">
                  <h3 className="text-sm font-medium text-[var(--text-primary)] font-mono">{t('settings.webauthnCredentials')}</h3>
                  <Button variant="secondary" size="sm" disabled>
                    {t('settings.registerKey')}
                  </Button>
                </div>

                {(!webauthnCreds || webauthnCreds.length === 0) ? (
                  <p className="text-xs text-[var(--text-muted)]">{t('settings.noCredentials')}</p>
                ) : (
                  <div className="space-y-2">
                    {webauthnCreds.map((cred) => (
                      <div
                        key={cred.id}
                        className="flex items-center justify-between rounded border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2"
                      >
                        <div>
                          <span className="text-sm text-[var(--text-primary)]">{cred.name || 'Unnamed key'}</span>
                          <span className="text-xs text-[var(--text-muted)] ml-2">
                            {new Date(cred.created_at).toLocaleDateString()}
                          </span>
                        </div>
                        <Button
                          variant="danger"
                          size="sm"
                          onClick={() => deleteWebAuthnMutation.mutate(cred.id)}
                          disabled={deleteWebAuthnMutation.isPending}
                        >
                          {t('common.delete')}
                        </Button>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Disable TOTP Confirmation Modal */}
              <Modal
                open={showDisableConfirm}
                onClose={() => setShowDisableConfirm(false)}
                title={t('settings.disableTotp')}
              >
                <p className="text-sm text-[var(--text-secondary)] mb-6">
                  {t('settings.confirmDisableTotp')}
                </p>
                <div className="flex gap-3 justify-end">
                  <Button variant="secondary" onClick={() => setShowDisableConfirm(false)}>
                    {t('common.cancel')}
                  </Button>
                  <Button
                    variant="danger"
                    onClick={() => disableTotpMutation.mutate()}
                    disabled={disableTotpMutation.isPending}
                  >
                    {disableTotpMutation.isPending ? 'Disabling...' : t('common.confirm')}
                  </Button>
                </div>
              </Modal>
            </div>
          )}

          {activeTab === 'integrations' && (
            <IntegrationsTab />
          )}
        </Card>
      )}
    </div>
  )
}
