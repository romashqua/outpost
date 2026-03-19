import { ReactNode } from 'react'
import { Check } from 'lucide-react'

interface CheckboxItemProps {
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
  description?: string
  compact?: boolean
  /** Extra content rendered at the right side of the label (e.g. metadata badge). */
  suffix?: ReactNode
}

export default function CheckboxItem({ checked, onChange, label, description, compact, suffix }: CheckboxItemProps) {
  if (compact) {
    return (
      <label className="flex items-center gap-2 cursor-pointer text-sm">
        <span
          className={`flex-shrink-0 w-4 h-4 rounded border-2 flex items-center justify-center transition-colors ${
            checked
              ? 'border-[var(--accent)] bg-[var(--accent)]'
              : 'border-[var(--text-muted)]/40 bg-transparent'
          }`}
        >
          {checked && <Check size={10} className="text-[var(--bg-primary)]" strokeWidth={3} />}
        </span>
        <input
          type="checkbox"
          className="sr-only"
          checked={checked}
          onChange={(e) => onChange(e.target.checked)}
        />
        <span className="text-[var(--text-secondary)]">{label}</span>
        {description && (
          <span className="text-xs text-[var(--text-muted)]">{description}</span>
        )}
        {suffix}
      </label>
    )
  }

  return (
    <label
      className={`flex items-center gap-3 px-3 py-2 rounded-lg border cursor-pointer transition-colors ${
        checked
          ? 'border-[var(--accent)]/30 bg-[var(--accent)]/5'
          : 'border-[var(--border)] bg-[var(--bg-tertiary)] hover:border-[var(--accent)]/50'
      }`}
    >
      <span
        className={`flex-shrink-0 w-4 h-4 rounded border-2 flex items-center justify-center transition-colors ${
          checked
            ? 'border-[var(--accent)] bg-[var(--accent)]'
            : 'border-[var(--text-muted)]/40 bg-transparent'
        }`}
      >
        {checked && <Check size={10} className="text-[var(--bg-primary)]" strokeWidth={3} />}
      </span>
      <input
        type="checkbox"
        className="sr-only"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span className="font-mono text-sm text-[var(--text-primary)]">{label}</span>
      {description && (
        <span className="text-xs text-[var(--text-muted)]">{description}</span>
      )}
      {suffix && <span className="ml-auto">{suffix}</span>}
    </label>
  )
}
