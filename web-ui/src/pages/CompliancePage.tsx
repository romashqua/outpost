import { useTranslation } from 'react-i18next'
import { CheckCircle2, XCircle, AlertTriangle } from 'lucide-react'
import Card from '@/components/ui/Card'
import Badge from '@/components/ui/Badge'

const complianceScore = 87

const checklist = [
  { id: 1, framework: 'SOC 2', item: 'Access control policies defined', status: 'passed' },
  { id: 2, framework: 'SOC 2', item: 'Multi-factor authentication enforced', status: 'warning' },
  { id: 3, framework: 'SOC 2', item: 'Audit logging enabled', status: 'passed' },
  { id: 4, framework: 'SOC 2', item: 'Encryption at rest', status: 'passed' },
  { id: 5, framework: 'ISO 27001', item: 'Information security policy', status: 'passed' },
  { id: 6, framework: 'ISO 27001', item: 'Risk assessment completed', status: 'passed' },
  { id: 7, framework: 'ISO 27001', item: 'Incident response plan', status: 'warning' },
  { id: 8, framework: 'ISO 27001', item: 'Asset inventory up to date', status: 'passed' },
  { id: 9, framework: 'GDPR', item: 'Data processing agreements', status: 'passed' },
  { id: 10, framework: 'GDPR', item: 'Right to erasure procedure', status: 'failed' },
  { id: 11, framework: 'GDPR', item: 'Data breach notification process', status: 'passed' },
  { id: 12, framework: 'GDPR', item: 'Privacy impact assessment', status: 'warning' },
]

const metrics = [
  { label: 'mfaAdoption', value: 78, target: 100 },
  { label: 'devicePosture', value: 92, target: 100 },
  { label: 'encryption', value: 100, target: 100 },
]

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

  const passed = checklist.filter((c) => c.status === 'passed').length
  const failed = checklist.filter((c) => c.status === 'failed').length
  const warnings = checklist.filter((c) => c.status === 'warning').length

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
          <CircularGauge value={complianceScore} />
        </Card>

        {/* Metrics */}
        {metrics.map((m) => {
          const pct = Math.round((m.value / m.target) * 100)
          const color = pct >= 90 ? 'var(--accent)' : pct >= 70 ? 'var(--warning)' : 'var(--danger)'
          return (
            <Card key={m.label} className="flex flex-col justify-center">
              <p className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)] mb-3">
                {t(`compliance.${m.label}`)}
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
        <div className="space-y-1">
          {checklist.map((item) => (
            <div
              key={item.id}
              className="flex items-center gap-3 rounded-md px-3 py-2.5 hover:bg-[var(--bg-tertiary)] transition-colors"
            >
              <StatusIcon status={item.status} />
              <Badge variant="info" className="text-[10px] min-w-[70px] justify-center">
                {item.framework}
              </Badge>
              <span className="text-sm text-[var(--text-secondary)] flex-1">{item.item}</span>
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
      </Card>
    </div>
  )
}
