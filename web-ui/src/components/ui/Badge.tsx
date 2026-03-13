import { clsx } from 'clsx'

type BadgeVariant = 'online' | 'offline' | 'pending' | 'info' | 'default'

interface BadgeProps {
  variant?: BadgeVariant
  children: React.ReactNode
  pulse?: boolean
  className?: string
}

const variantStyles: Record<BadgeVariant, string> = {
  online: 'text-[var(--accent)] bg-[rgba(0,255,136,0.1)] border-[rgba(0,255,136,0.2)]',
  offline: 'text-[var(--danger)] bg-[rgba(255,68,68,0.1)] border-[rgba(255,68,68,0.2)]',
  pending: 'text-[var(--warning)] bg-[rgba(255,170,0,0.1)] border-[rgba(255,170,0,0.2)]',
  info: 'text-[var(--info)] bg-[rgba(0,170,255,0.1)] border-[rgba(0,170,255,0.2)]',
  default: 'text-[var(--text-muted)] bg-[var(--bg-tertiary)] border-[var(--border)]',
}

export default function Badge({ variant = 'default', children, pulse, className }: BadgeProps) {
  return (
    <span
      className={clsx(
        'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium font-mono',
        variantStyles[variant],
        className,
      )}
    >
      {(variant === 'online' || variant === 'offline') && (
        <span
          className={clsx(
            'h-1.5 w-1.5 rounded-full',
            variant === 'online' && 'bg-[var(--accent)]',
            variant === 'offline' && 'bg-[var(--danger)]',
            pulse && variant === 'online' && 'pulse-green',
            pulse && variant === 'offline' && 'pulse-red',
          )}
        />
      )}
      {children}
    </span>
  )
}
