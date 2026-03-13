import { clsx } from 'clsx'

interface CardProps {
  children: React.ReactNode
  className?: string
  glow?: boolean
  scanline?: boolean
}

export default function Card({ children, className, glow, scanline }: CardProps) {
  return (
    <div
      className={clsx(
        'rounded-lg border border-[var(--border)] bg-[var(--bg-card)] p-5 card-hover',
        glow && 'glow-box',
        scanline && 'scanline',
        className,
      )}
    >
      {children}
    </div>
  )
}
