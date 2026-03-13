import { clsx } from 'clsx'
import { TrendingUp, TrendingDown } from 'lucide-react'

interface StatsProps {
  label: string
  value: string | number
  trend?: number
  icon?: React.ReactNode
  accentColor?: string
}

export default function Stats({ label, value, trend, icon, accentColor }: StatsProps) {
  const color = accentColor || 'var(--accent)'

  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-card)] p-5 card-hover">
      <div className="flex items-start justify-between">
        <div>
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)] mb-2">
            {label}
          </p>
          <p
            className="text-3xl font-bold font-mono"
            style={{ color }}
          >
            {value}
          </p>
        </div>
        {icon && (
          <div
            className="rounded-md p-2"
            style={{ background: `${color}15` }}
          >
            <span style={{ color }}>{icon}</span>
          </div>
        )}
      </div>
      {trend !== undefined && (
        <div className="mt-3 flex items-center gap-1 text-xs">
          {trend >= 0 ? (
            <>
              <TrendingUp size={14} className="text-[var(--accent)]" />
              <span className="text-[var(--accent)]">+{trend}%</span>
            </>
          ) : (
            <>
              <TrendingDown size={14} className="text-[var(--danger)]" />
              <span className="text-[var(--danger)]">{trend}%</span>
            </>
          )}
          <span className="text-[var(--text-muted)] ml-1">vs last week</span>
        </div>
      )}
    </div>
  )
}
