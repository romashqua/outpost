import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  CheckCircle2,
  XCircle,
  AlertTriangle,
  ShieldCheck,
  Info,
  ChevronDown,
  ChevronRight,
  Lock,
  HardDrive,
  Users,
  FileText,
  KeyRound,
  Timer,
} from 'lucide-react'
import { api } from '@/api/client'
import Card from '@/components/ui/Card'
import Badge from '@/components/ui/Badge'

interface ComplianceCheck {
  id: string
  name: string
  description: string
  status: string
  details: string
  framework: string
}

interface ComplianceReport {
  overall_score: number
  max_score: number
  percentage: number
  mfa_adoption: number
  encryption_rate: number
  posture_rate: number
  audit_log_enabled: boolean
  password_policy: boolean
  session_timeout: boolean
  checks: ComplianceCheck[]
}

function StatusIcon({ status, size = 16 }: { status: string; size?: number }) {
  switch (status) {
    case 'passed':
      return <CheckCircle2 size={size} className="text-[var(--accent)]" />
    case 'failed':
      return <XCircle size={size} className="text-[var(--danger)]" />
    case 'warning':
      return <AlertTriangle size={size} className="text-[var(--warning)]" />
    default:
      return null
  }
}

function CircularGauge({ value, size = 160, label }: { value: number; size?: number; label?: string }) {
  const strokeWidth = 8
  const radius = (size - strokeWidth) / 2
  const circumference = 2 * Math.PI * radius
  const offset = circumference - (value / 100) * circumference
  const color = value >= 80 ? 'var(--accent)' : value >= 60 ? 'var(--warning)' : 'var(--danger)'

  return (
    <div className="relative inline-flex flex-col items-center justify-center">
      <div className="relative" style={{ width: size, height: size }}>
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
        <div className="absolute inset-0 flex items-center justify-center text-center">
          <span className="text-3xl font-bold font-mono" style={{ color }}>
            {value}
          </span>
          <span className="text-xs text-[var(--text-muted)]">%</span>
        </div>
      </div>
      {label && (
        <span className="text-xs text-[var(--text-muted)] mt-2 font-medium">{label}</span>
      )}
    </div>
  )
}

function MiniGauge({ value, label, icon }: { value: number; label: string; icon: React.ReactNode }) {
  const color = value >= 80 ? 'var(--accent)' : value >= 60 ? 'var(--warning)' : 'var(--danger)'
  return (
    <div className="flex items-center gap-3 p-3 rounded-lg bg-[var(--bg-secondary)]">
      <div className="text-[var(--text-muted)]">{icon}</div>
      <div className="flex-1 min-w-0">
        <div className="flex items-center justify-between mb-1">
          <span className="text-xs text-[var(--text-secondary)]">{label}</span>
          <span className="text-sm font-mono font-semibold" style={{ color }}>{value.toFixed(1)}%</span>
        </div>
        <div className="h-1.5 rounded-full bg-[var(--bg-tertiary)]">
          <div
            className="h-full rounded-full transition-all"
            style={{ width: `${Math.min(value, 100)}%`, background: color }}
          />
        </div>
      </div>
    </div>
  )
}

function BooleanCheck({ enabled, label, icon }: { enabled: boolean; label: string; icon: React.ReactNode }) {
  return (
    <div className="flex items-center gap-3 p-3 rounded-lg bg-[var(--bg-secondary)]">
      <div className="text-[var(--text-muted)]">{icon}</div>
      <span className="text-xs text-[var(--text-secondary)] flex-1">{label}</span>
      {enabled ? (
        <CheckCircle2 size={16} className="text-[var(--accent)]" />
      ) : (
        <XCircle size={16} className="text-[var(--danger)]" />
      )}
    </div>
  )
}

const FRAMEWORK_INFO: Record<string, { color: string; description: string }> = {
  SOC2: { color: '#6366f1', description: 'compliance.soc2Description' },
  ISO27001: { color: '#06b6d4', description: 'compliance.iso27001Description' },
  GDPR: { color: '#f59e0b', description: 'compliance.gdprDescription' },
}

const CHECK_REMEDIATION: Record<string, string> = {
  'soc2-cc6.1-mfa': 'compliance.remediateMfa',
  'soc2-cc6.6-encryption': 'compliance.remediateEncryption',
  'soc2-cc7.2-audit': 'compliance.remediateAudit',
  'iso27001-a8.1-posture': 'compliance.remediatePosture',
  'iso27001-a9.4-rbac': 'compliance.remediateRbac',
  'iso27001-a10.1-encryption': 'compliance.remediateDiskEncryption',
  'gdpr-art5-encryption': 'compliance.remediateEncryption',
  'gdpr-art30-audit': 'compliance.remediateAudit',
  'gdpr-art32-access': 'compliance.remediateAccess',
}

function FrameworkSection({
  framework,
  checks,
  expandedChecks,
  toggleCheck,
  t,
}: {
  framework: string
  checks: ComplianceCheck[]
  expandedChecks: Set<string>
  toggleCheck: (id: string) => void
  t: (key: string) => string
}) {
  const info = FRAMEWORK_INFO[framework] || { color: '#888', description: '' }
  const passed = checks.filter((c) => c.status === 'passed').length
  const total = checks.length
  const pct = total > 0 ? Math.round((passed / total) * 100) : 0
  const color = pct >= 80 ? 'var(--accent)' : pct >= 60 ? 'var(--warning)' : 'var(--danger)'

  return (
    <Card className="overflow-hidden">
      <div className="flex items-center gap-3 mb-3">
        <div
          className="w-3 h-3 rounded-full"
          style={{ background: info.color, boxShadow: `0 0 8px ${info.color}40` }}
        />
        <h3 className="text-sm font-semibold text-[var(--text-primary)] font-mono flex-1">
          {framework}
        </h3>
        <span className="text-sm font-mono font-semibold" style={{ color }}>
          {passed}/{total}
        </span>
      </div>
      <p className="text-xs text-[var(--text-muted)] mb-4 leading-relaxed">
        {t(info.description)}
      </p>

      <div className="space-y-1">
        {checks.map((check) => {
          const isExpanded = expandedChecks.has(check.id)
          const remediation = CHECK_REMEDIATION[check.id]
          return (
            <div key={check.id}>
              <button
                className="w-full flex items-center gap-3 rounded-md px-3 py-2.5 hover:bg-[var(--bg-tertiary)] transition-colors text-left"
                onClick={() => toggleCheck(check.id)}
              >
                <StatusIcon status={check.status} />
                <div className="flex-1 min-w-0">
                  <span className="text-sm text-[var(--text-secondary)]">{check.name}</span>
                </div>
                <span className="text-xs text-[var(--text-muted)] max-w-[200px] truncate hidden sm:inline" title={check.details}>
                  {check.details}
                </span>
                <Badge
                  variant={
                    check.status === 'passed' ? 'online' :
                    check.status === 'failed' ? 'offline' : 'pending'
                  }
                >
                  {t(`compliance.${check.status}`)}
                </Badge>
                {isExpanded ? <ChevronDown size={14} className="text-[var(--text-muted)]" /> : <ChevronRight size={14} className="text-[var(--text-muted)]" />}
              </button>
              {isExpanded && (
                <div className="ml-9 mr-3 mb-2 p-3 rounded-md bg-[var(--bg-secondary)] border border-[var(--border)]">
                  <div className="flex items-start gap-2 mb-2">
                    <Info size={14} className="text-[var(--text-muted)] mt-0.5 shrink-0" />
                    <p className="text-xs text-[var(--text-secondary)] leading-relaxed">
                      {check.description}
                    </p>
                  </div>
                  <div className="flex items-start gap-2 mb-2">
                    <FileText size={14} className="text-[var(--text-muted)] mt-0.5 shrink-0" />
                    <p className="text-xs text-[var(--text-muted)] leading-relaxed">
                      <span className="font-medium text-[var(--text-secondary)]">{t('compliance.howItWorks')}:</span>{' '}
                      {t(`compliance.howCheck_${check.id.replace(/[.-]/g, '_')}`)}
                    </p>
                  </div>
                  {check.status !== 'passed' && remediation && (
                    <div className="flex items-start gap-2 mt-2 pt-2 border-t border-[var(--border)]">
                      <ShieldCheck size={14} className="text-[var(--accent)] mt-0.5 shrink-0" />
                      <p className="text-xs text-[var(--accent)] leading-relaxed">
                        <span className="font-medium">{t('compliance.remediation')}:</span>{' '}
                        {t(remediation)}
                      </p>
                    </div>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </Card>
  )
}

export default function CompliancePage() {
  const { t } = useTranslation()
  const [expandedChecks, setExpandedChecks] = useState<Set<string>>(new Set())

  const toggleCheck = (id: string) => {
    setExpandedChecks((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const { data: report, isLoading, error } = useQuery({
    queryKey: ['compliance', 'report'],
    queryFn: () => api.get<ComplianceReport>('/compliance/report'),
  })

  const checks = report?.checks ?? []
  const passed = checks.filter((c) => c.status === 'passed').length
  const failed = checks.filter((c) => c.status === 'failed').length
  const warnings = checks.filter((c) => c.status === 'warning').length

  const frameworks = ['SOC2', 'ISO27001', 'GDPR']
  const groupedChecks: Record<string, ComplianceCheck[]> = {}
  frameworks.forEach((fw) => {
    groupedChecks[fw] = checks.filter((c) => c.framework === fw)
  })

  if (error) {
    return (
      <div>
        <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
          <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
          {t('compliance.title')}
        </h1>
        <div className="rounded-lg border border-[var(--danger)] bg-[var(--bg-card)] p-6 text-center text-[var(--danger)]">
          {t('compliance.failedToLoad')}: {(error as Error).message}
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
          {t('compliance.loadingReport')}
        </div>
      </div>
    )
  }

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-2">
        <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
        {t('compliance.title')}
      </h1>
      <p className="text-sm text-[var(--text-muted)] mb-6 leading-relaxed max-w-3xl">
        {t('compliance.pageDescription')}
      </p>

      {/* Top row: Overall score + Summary */}
      <div className="grid grid-cols-1 lg:grid-cols-4 gap-4 mb-6">
        <Card className="flex flex-col items-center justify-center py-6">
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)] mb-4">
            {t('compliance.overallScore')}
          </p>
          <CircularGauge value={report?.percentage ?? 0} />
          <p className="text-xs text-[var(--text-muted)] mt-3 font-mono">
            {report?.overall_score ?? 0} / {report?.max_score ?? 0}
          </p>
        </Card>

        <Card className="lg:col-span-3">
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-3 font-mono">
            {t('compliance.systemMetrics')}
          </h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 mb-3">
            <MiniGauge value={report?.mfa_adoption ?? 0} label={t('compliance.mfaAdoption')} icon={<KeyRound size={16} />} />
            <MiniGauge value={report?.encryption_rate ?? 0} label={t('compliance.encryption')} icon={<Lock size={16} />} />
            <MiniGauge value={report?.posture_rate ?? 0} label={t('compliance.devicePosture')} icon={<HardDrive size={16} />} />
            <div className="space-y-2">
              <BooleanCheck enabled={report?.audit_log_enabled ?? false} label={t('compliance.auditLogEnabled')} icon={<FileText size={16} />} />
              <BooleanCheck enabled={report?.password_policy ?? false} label={t('compliance.passwordPolicy')} icon={<Users size={16} />} />
              <BooleanCheck enabled={report?.session_timeout ?? false} label={t('compliance.sessionTimeout')} icon={<Timer size={16} />} />
            </div>
          </div>
        </Card>
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

      {/* Framework sections */}
      <div className="space-y-4">
        {frameworks.map((fw) => (
          <FrameworkSection
            key={fw}
            framework={fw}
            checks={groupedChecks[fw] || []}
            expandedChecks={expandedChecks}
            toggleCheck={toggleCheck}
            t={t}
          />
        ))}
      </div>
    </div>
  )
}
