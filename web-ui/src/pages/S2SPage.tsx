import { useTranslation } from 'react-i18next'
import { Construction } from 'lucide-react'
import Card from '@/components/ui/Card'

export default function S2SPage() {
  const { t } = useTranslation()

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('s2s.title')}
        </h1>
      </div>

      <Card className="flex flex-col items-center justify-center py-16">
        <div className="rounded-full p-4 mb-4" style={{ background: 'rgba(0,255,136,0.08)' }}>
          <Construction size={40} className="text-[var(--accent)]" />
        </div>
        <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-2 font-mono">
          Coming Soon
        </h2>
        <p className="text-sm text-[var(--text-muted)] text-center max-w-md">
          Site-to-site tunnels are currently under development. The database schema is ready,
          and API endpoints will be available in a future release.
        </p>
        <p className="text-xs text-[var(--text-muted)] mt-4 font-mono">
          No tunnels configured
        </p>
      </Card>
    </div>
  )
}
