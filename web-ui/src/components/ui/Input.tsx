import { clsx } from 'clsx'
import { forwardRef } from 'react'

interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string
  error?: string
  icon?: React.ReactNode
}

const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ label, error, icon, className, ...props }, ref) => {
    return (
      <div className="flex flex-col gap-1.5">
        {label && (
          <label className="text-xs font-medium uppercase tracking-wider text-[var(--text-muted)]">
            {label}
          </label>
        )}
        <div className="relative">
          {icon && (
            <span className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-muted)]">
              {icon}
            </span>
          )}
          <input
            ref={ref}
            className={clsx(
              'w-full rounded-md border border-[var(--border)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] font-mono',
              'placeholder:text-[var(--text-muted)]',
              'glow-focus transition-all duration-150',
              icon && 'pl-9',
              error && 'border-[var(--danger)]',
              className,
            )}
            {...props}
          />
        </div>
        {error && <span className="text-xs text-[var(--danger)]">{error}</span>}
      </div>
    )
  },
)

Input.displayName = 'Input'

export default Input
