import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { clsx } from 'clsx'
import Card from '@/components/ui/Card'
import Input from '@/components/ui/Input'
import Button from '@/components/ui/Button'

const tabs = ['general', 'auth', 'wireguard', 'smtp', 'integrations'] as const
type Tab = typeof tabs[number]

export default function SettingsPage() {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState<Tab>('general')

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
        <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
        {t('settings.title')}
      </h1>

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
          <form className="flex flex-col gap-5 max-w-lg" onSubmit={(e) => e.preventDefault()}>
            <Input label={t('settings.orgName')} defaultValue="Outpost Corp" />
            <Input label={t('settings.domain')} defaultValue="vpn.outpost.local" />
            <Input label={t('settings.sessionTimeout')} defaultValue="3600" type="number" />
            <div className="mt-2">
              <Button>{t('settings.save')}</Button>
            </div>
          </form>
        )}

        {activeTab === 'auth' && (
          <form className="flex flex-col gap-5 max-w-lg" onSubmit={(e) => e.preventDefault()}>
            <Input label={t('settings.oidcProvider')} placeholder="https://auth.example.com" />
            <Input label={t('settings.ldapServer')} placeholder="ldap://ldap.corp.local:389" />
            <div className="flex items-center gap-3">
              <input
                type="checkbox"
                id="mfa-required"
                defaultChecked
                className="h-4 w-4 rounded border-[var(--border)] bg-[var(--bg-secondary)] accent-[var(--accent)]"
              />
              <label htmlFor="mfa-required" className="text-sm text-[var(--text-secondary)]">
                {t('settings.mfaRequired')}
              </label>
            </div>
            <div className="mt-2">
              <Button>{t('settings.save')}</Button>
            </div>
          </form>
        )}

        {activeTab === 'wireguard' && (
          <form className="flex flex-col gap-5 max-w-lg" onSubmit={(e) => e.preventDefault()}>
            <Input label={t('settings.wgPort')} defaultValue="51820" type="number" />
            <Input label={t('settings.wgMtu')} defaultValue="1420" type="number" />
            <Input label={t('settings.keepalive')} defaultValue="25" type="number" />
            <Input label={t('settings.dns')} defaultValue="1.1.1.1, 8.8.8.8" />
            <div className="mt-2">
              <Button>{t('settings.save')}</Button>
            </div>
          </form>
        )}

        {activeTab === 'smtp' && (
          <form className="flex flex-col gap-5 max-w-lg" onSubmit={(e) => e.preventDefault()}>
            <Input label={t('settings.smtpHost')} placeholder="smtp.corp.ru" />
            <Input label={t('settings.smtpPort')} defaultValue="587" type="number" />
            <Input label={t('settings.smtpFrom')} placeholder="noreply@outpost.local" />
            <div className="mt-2">
              <Button>{t('settings.save')}</Button>
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
