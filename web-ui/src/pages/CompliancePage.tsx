import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, XCircle, AlertTriangle } from 'lucide-react'
import { api } from '@/api/client'
import Card from '@/components/ui/Card'
import Badge from '@/components/ui/Badge'

interface ComplianceCheck {
  name: string
  status: string
  details: string
  framework: string
}

interface ComplianceReport {
  overall_score: number
  max_score: number
  percentage: number
  checks: ComplianceCheck[]
}

function StatusIcon({ status }: { status: string }) {
  switch (status) {
    case 'passed':
      return <CheckCircle2 size={16} className="text-[var(--accent)]" />
    case 'failed':
      return <XCircle size={16} className="text-[var(--danger)]" />
    case 'warning':
      return <AlertTriangle size={16} className="text-[var(--warning)]" />
    default:
      return null
  }
}

function CircularGauge({ value, size = 160 }: { value: number; size?: number }) {
  const strokeWidth = 8
  const radius = (size - strokeWidth) / 2
  const circumference = 2 * Math.PI * radius
  const offset = circumference - (value / 100) * circumference
  const color = value >= 80 ? 'var(--accent)' : value >= 60 ? 'var(--warning)' : 'var(--danger)'

  return (
    <div className="relative inline-flex items-center justify-center" style={{ width: size, height: size }}>
      <svg width={size} height={size} className="-rotate-90">
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke="var(--bg-tertiary)"
          strokeWidth={strokeWidth}
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke={color}
          strokeWidth={strokeWidth}
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          style={{ transition: 'stroke-dashoffset 1s ease', filter: `drop-shadow(0 0 6px ${color})` }}
        />
      </svg>
      <div className="absolute text-center">
        <span className="text-3xl font-bold font-mono" style={{ color }}>
          {value}
        </span>
        <span className="text-xs text-[var(--text-muted)] block">/ 100</span>
      </div>
    </div>
  )
}

export default function CompliancePage() {
  const { t } = useTranslation()

  const { data: report, isLoading, error } = useQuery({
    queryKey: ['compliance', 'report'],
    queryFn: () => api.get<ComplianceReport>('/compliance/report'),
  })

  const checks = report?.checks ?? []
  const passed = checks.filter((c) => c.status === 'passed').length
  const failed = checks.filter((c) => c.status === 'failed').length
  const warnings = checks.filter((c) => c.status === 'warning').length

  // Group checks by framework for metrics
  const frameworks = [...new Set(checks.map((c) => c.framework))]
  const frameworkScores = frameworks.map((fw) => {
    const fwChecks = checks.filter((c) => c.framework === fw)
    const fwPassed = fwChecks.filter((c) => c.status === 'passed').length
    return {
      label: fw,
      value: fwChecks.length > 0 ? Math.round((fwPassed / fwChecks.length) * 100) : 0,
      target: 100,
    }
  })

  if (error) {
    return (
      <div>
        <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('compliance.title')}
        </h1>
        <div className="rounded-lg border border-[var(--danger)] bg-[var(--bg-card)] p-6 text-center text-[var(--danger)]">
          Failed to load compliance report: {(error as Error).message}
        </div>
      </div>
    )
  }

  if (isLoading) {
    return (
      <div>
        <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('compliance.title')}
        </h1>
        <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-card)] p-8 text-center text-[var(--text-muted)]">
          Loading compliance report...
        </div>
      </div>
    )
  }

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
        <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
        {t('compliance.title')}
      </h1>

      <div className="grid grid-cols-4 gap-4 mb-6">
        {/* Overall score */}
        <Card className="flex flex-col items-center justify-center py-6">
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)] mb-4">
            {t('compliance.overallScore')}
          </p>
          <CircularGauge value={report?.percentage ?? 0} />
        </Card>

        {/* Framework scores */}
        {frameworkScores.slice(0, 3).map((m) => {
          const pct = Math.round((m.value / m.target) * 100)
          const color = pct >= 90 ? 'var(--accent)' : pct >= 70 ? 'var(--warning)' : 'var(--danger)'
          return (
            <Card key={m.label} className="flex flex-col justify-center">
              <p className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)] mb-3">
                {m.label}
              </p>
              <p className="text-3xl font-bold font-mono mb-3" style={{ color }}>
                {m.value}%
              </p>
              <div className="h-2 rounded-full bg-[var(--bg-tertiary)]">
                <div
                  className="h-full rounded-full transition-all"
                  style={{ width: `${pct}%`, background: color }}
                />
              </div>
            </Card>
          )
        })}
      </div>

      {/* Summary badges */}
      <div className="flex gap-4 mb-6">
        <Badge variant="online">
          {passed} {t('compliance.passed')}
        </Badge>
        <Badge variant="pending">
          {warnings} {t('compliance.warning')}
        </Badge>
        <Badge variant="offline">
          {failed} {t('compliance.failed')}
        </Badge>
      </div>

      {/* Checklist */}
      <Card>
        <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
          {t('compliance.checklist')}
        </h2>
        {checks.length === 0 ? (
          <div className="py-8 text-center text-sm text-[var(--text-muted)]">
            No compliance checks available
          </div>
        ) : (
          <div className="space-y-1">
            {checks.map((item, idx) => (
              <div
                key={idx}
                className="flex items-center gap-3 rounded-md px-3 py-2.5 hover:bg-[var(--bg-tertiary)] transition-colors"
              >
                <StatusIcon status={item.status} />
                <Badge variant="info" className="text-[10px] min-w-[70px] justify-center">
                  {item.framework}
                </Badge>
                <span className="text-sm text-[var(--text-secondary)] flex-1">{item.name}</span>
                {item.details && (
                  <span className="text-xs text-[var(--text-muted)] max-w-[200px] truncate" title={item.details}>
                    {item.details}
                  </span>
                )}
                <Badge
                  variant={
                    item.status === 'passed' ? 'online' :
                    item.status === 'failed' ? 'offline' : 'pending'
                  }
                >
                  {t(`compliance.${item.status}`)}
                </Badge>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}
