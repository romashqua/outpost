import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { clsx } from 'clsx'
import Card from '@/components/ui/Card'
import Input from '@/components/ui/Input'
import Button from '@/components/ui/Button'

const tabs = ['general', 'auth', 'wireguard', 'smtp', 'integrations'] as const
type Tab = typeof tabs[number]

const STORAGE_KEY = 'outpost-settings'

interface SettingsData {
  orgName: string
  domain: string
  sessionTimeout: string
  oidcProvider: string
  ldapServer: string
  mfaRequired: boolean
  wgPort: string
  wgMtu: string
  keepalive: string
  dns: string
  smtpHost: string
  smtpPort: string
  smtpFrom: string
}

const defaultSettings: SettingsData = {
  orgName: 'Outpost Corp',
  domain: 'vpn.outpost.local',
  sessionTimeout: '3600',
  oidcProvider: '',
  ldapServer: '',
  mfaRequired: true,
  wgPort: '51820',
  wgMtu: '1420',
  keepalive: '25',
  dns: '1.1.1.1, 8.8.8.8',
  smtpHost: '',
  smtpPort: '587',
  smtpFrom: '',
}

function loadSettings(): SettingsData {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (stored) return { ...defaultSettings, ...JSON.parse(stored) }
  } catch {
    // ignore
  }
  return { ...defaultSettings }
}

function saveSettings(data: SettingsData) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(data))
}

export default function SettingsPage() {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<Tab>('general')
  const [settings, setSettings] = useState<SettingsData>(loadSettings)
  const [toast, setToast] = useState<string | null>(null)

  const update = useCallback(<K extends keyof SettingsData>(key: K, value: SettingsData[K]) => {
    setSettings((prev) => ({ ...prev, [key]: value }))
  }, [])

  const handleSave = useCallback((e: React.FormEvent) => {
    e.preventDefault()
    saveSettings(settings)
    setToast(t('settings.saved', 'Settings saved'))
  }, [settings, t])

  useEffect(() => {
    if (!toast) return
    const timer = setTimeout(() => setToast(null), 3000)
    return () => clearTimeout(timer)
  }, [toast])

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
        <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
        {t('settings.title')}
      </h1>

      {/* Toast notification */}
      {toast && (
        <div className="fixed top-4 right-4 z-50 rounded-md border border-[var(--accent)] bg-[var(--bg-card)] px-4 py-2 text-sm text-[var(--accent)] font-mono shadow-lg animate-in fade-in">
          {toast}
        </div>
      )}

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
              <Button type="submit">{t('settings.save')}</Button>
            </div>
          </form>
        )}

        {activeTab === 'auth' && (
          <form className="flex flex-col gap-5 max-w-lg" onSubmit={handleSave}>
            <Input
              label={t('settings.oidcProvider')}
              placeholder="https://auth.example.com"
              value={settings.oidcProvider}
              onChange={(e) => update('oidcProvider', e.target.value)}
            />
            <Input
              label={t('settings.ldapServer')}
              placeholder="ldap://ldap.corp.local:389"
              value={settings.ldapServer}
              onChange={(e) => update('ldapServer', e.target.value)}
            />
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
            <div className="mt-2">
              <Button type="submit">{t('settings.save')}</Button>
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
              <Button type="submit">{t('settings.save')}</Button>
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
            <div className="mt-2">
              <Button type="submit">{t('settings.save')}</Button>
            </div>
          </form>
        )}

        {activeTab === 'integrations' && (
          <div className="space-y-4">
            {[
              { name: 'Slack', desc: 'Send notifications to Slack channels', connected: true },
              { name: 'Telegram', desc: 'Alert notifications via Telegram bot', connected: false },
              { name: 'Grafana', desc: 'Export metrics to Grafana', connected: true },
              { name: 'Syslog', desc: 'Forward audit logs to syslog server', connected: false },
            ].map((integ) => (
              <div
                key={integ.name}
                className="flex items-center justify-between rounded-md border border-[var(--border)] p-4 hover:border-[var(--border-hover)] transition-colors"
              >
                <div>
                  <h3 className="text-sm font-medium text-[var(--text-primary)]">{integ.name}</h3>
                  <p className="text-xs text-[var(--text-muted)] mt-0.5">{integ.desc}</p>
                </div>
                <Button
                  variant={integ.connected ? 'secondary' : 'primary'}
                  size="sm"
                >
                  {integ.connected ? 'Connected' : 'Connect'}
                </Button>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}
