import { X } from 'lucide-react'
import { clsx } from 'clsx'
import { useToastStore } from '@/store/toast'
import type { ToastType } from '@/store/toast'

const typeStyles: Record<ToastType, string> = {
  success: 'border-[rgba(0,255,136,0.3)] bg-[rgba(0,255,136,0.08)] text-[var(--accent)]',
  error: 'border-[rgba(255,68,68,0.3)] bg-[rgba(255,68,68,0.08)] text-[var(--danger)]',
  info: 'border-[rgba(0,170,255,0.3)] bg-[rgba(0,170,255,0.08)] text-[var(--info)]',
}

const iconMap: Record<ToastType, string> = {
  success: '[ok]',
  error: '[err]',
  info: '[i]',
}

export default function ToastContainer() {
  const toasts = useToastStore((s) => s.toasts)
  const removeToast = useToastStore((s) => s.removeToast)

  if (toasts.length === 0) return null

  return (
    <div className="fixed top-4 right-4 z-[100] flex flex-col gap-2 max-w-sm">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          className={clsx(
            'flex items-start gap-2 rounded-lg border px-4 py-3 shadow-lg backdrop-blur-sm animate-in slide-in-from-right',
            typeStyles[toast.type],
          )}
        >
          <span className="font-mono text-xs mt-0.5 opacity-70 shrink-0">
            {iconMap[toast.type]}
          </span>
          <p className="text-sm font-mono flex-1">{toast.message}</p>
          <button
            onClick={() => removeToast(toast.id)}
            className="shrink-0 opacity-50 hover:opacity-100 transition-opacity cursor-pointer"
          >
            <X size={14} />
          </button>
        </div>
      ))}
    </div>
  )
}
