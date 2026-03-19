import { useState, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Bell, User, Laptop2, Network, Server, Shield, LogIn, LogOut, AlertTriangle } from 'lucide-react'
import { api } from '@/api/client'

interface NotificationItem {
  id: number
  timestamp: string
  action: string
  resource: string
  details?: Record<string, unknown>
  user_id?: string
}

interface NotificationsResponse {
  notifications: NotificationItem[]
  total: number
}

interface UnreadCountResponse {
  count: number
}

const STORAGE_KEY = 'outpost-notifications-read-at'

function getLastReadAt(): string {
  return localStorage.getItem(STORAGE_KEY) || new Date(0).toISOString()
}

function markAsRead() {
  localStorage.setItem(STORAGE_KEY, new Date().toISOString())
}

function getActionIcon(action: string) {
  const a = action.toLowerCase()
  if (a.includes('login') || a.includes('auth')) return LogIn
  if (a.includes('logout')) return LogOut
  if (a.includes('mfa') || a.includes('fail')) return AlertTriangle
  if (a.includes('device') || a.includes('approve') || a.includes('revoke')) return Laptop2
  if (a.includes('gateway') || a.includes('connect')) return Server
  if (a.includes('network')) return Network
  if (a.includes('user')) return User
  if (a.includes('ztna') || a.includes('policy')) return Shield
  return Bell
}

function getActionColor(action: string): string {
  const a = action.toLowerCase()
  if (a.includes('delete') || a.includes('revoke') || a.includes('fail') || a.includes('disconnect')) {
    return 'var(--danger)'
  }
  if (a.includes('create') || a.includes('approve') || a.includes('connect') || a.includes('login')) {
    return 'var(--accent)'
  }
  return 'var(--warning, #f59e0b)'
}

function formatTimeAgo(timestamp: string, t: (key: string, opts?: Record<string, unknown>) => string): string {
  const diff = Date.now() - new Date(timestamp).getTime()
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return t('notifications.justNow')
  if (minutes < 60) return t('notifications.minutesAgo', { count: minutes })
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return t('notifications.hoursAgo', { count: hours })
  const days = Math.floor(hours / 24)
  return t('notifications.daysAgo', { count: days })
}

function formatAction(action: string, resource: string, details: Record<string, unknown> | undefined, t: (key: string) => string): string {
  const a = action.toLowerCase()
  const name = (details?.name as string) || (details?.username as string) || ''
  const suffix = name ? ` "${name}"` : ''

  // Semantic events (e.g. "device.approved", "gateway.connected")
  if (a === 'create' || a === 'insert') return `${t('notifications.created')} ${resource}${suffix}`
  if (a === 'update') return `${t('notifications.updated')} ${resource}${suffix}`
  if (a === 'delete') return `${t('notifications.deleted')} ${resource}${suffix}`
  if (a === 'device.approved') return `${t('notifications.deviceApproved')}${suffix}`
  if (a === 'device.revoked') return `${t('notifications.deviceRevoked')}${suffix}`
  if (a === 'gateway.connected') return `${t('notifications.gatewayConnected')}${suffix}`
  if (a === 'gateway.disconnected') return `${t('notifications.gatewayDisconnected')}${suffix}`

  // Middleware format: "METHOD /api/v1/resource/..."
  const m = a.match(/^(post|put|delete|patch)\s+\/api\/v1\/(\w[\w-]*)/)
  if (m) {
    const [, method, res] = m
    const resName = res.replace(/-/g, ' ')
    if (a.includes('/login')) return t('notifications.userLoggedIn')
    if (a.includes('/logout')) return t('notifications.userLoggedOut')
    if (a.includes('/approve')) return `${t('notifications.deviceApproved')}${suffix}`
    if (a.includes('/revoke')) return `${t('notifications.deviceRevoked')}${suffix}`
    if (method === 'post') return `${t('notifications.created')} ${resName}${suffix}`
    if (method === 'delete') return `${t('notifications.deleted')} ${resName}${suffix}`
    if (method === 'put' || method === 'patch') return `${t('notifications.updated')} ${resName}${suffix}`
  }

  if (a.includes('mfa_fail')) return t('notifications.mfaFailed')

  return `${action} ${resource}${suffix}`
}

export default function NotificationDropdown() {
  const { t } = useTranslation()
  const [isOpen, setIsOpen] = useState(false)
  const [lastReadAt, setLastReadAt] = useState(getLastReadAt)
  const dropdownRef = useRef<HTMLDivElement>(null)

  const notificationsQuery = useQuery<NotificationsResponse>({
    queryKey: ['notifications'],
    queryFn: () => api.get('/notifications?limit=20'),
    refetchInterval: 30000,
  })

  const unreadCountQuery = useQuery<UnreadCountResponse>({
    queryKey: ['notifications-unread', lastReadAt],
    queryFn: () => api.get(`/notifications/unread-count?since=${encodeURIComponent(lastReadAt)}`),
    refetchInterval: 30000,
  })

  const unreadCount = unreadCountQuery.data?.count ?? 0
  const notifications = notificationsQuery.data?.notifications ?? []

  // Close dropdown on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setIsOpen(false)
      }
    }
    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside)
      return () => document.removeEventListener('mousedown', handleClickOutside)
    }
  }, [isOpen])

  const handleToggle = () => {
    if (!isOpen) {
      setIsOpen(true)
    } else {
      setIsOpen(false)
    }
  }

  const handleMarkAllRead = () => {
    markAsRead()
    setLastReadAt(new Date().toISOString())
    api.post('/notifications/mark-read').catch(() => {})
  }

  return (
    <div className="relative" ref={dropdownRef}>
      <button
        onClick={handleToggle}
        className="relative rounded-md p-2 text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] transition-colors cursor-pointer"
      >
        <Bell size={16} />
        {unreadCount > 0 && (
          <span className="absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-[var(--danger)] px-1 text-[10px] font-bold text-white">
            {unreadCount > 99 ? '99+' : unreadCount}
          </span>
        )}
      </button>

      {isOpen && (
        <div className="absolute right-0 top-full mt-2 w-96 rounded-lg border border-[var(--border)] bg-[var(--bg-secondary)] shadow-2xl z-50">
          {/* Header */}
          <div className="flex items-center justify-between border-b border-[var(--border)] px-4 py-3">
            <h3 className="text-sm font-semibold text-[var(--text-primary)]">
              {t('notifications.title')}
            </h3>
            {unreadCount > 0 && (
              <button
                onClick={handleMarkAllRead}
                className="text-xs text-[var(--accent)] hover:underline cursor-pointer"
              >
                {t('notifications.markAllRead')}
              </button>
            )}
          </div>

          {/* List */}
          <div className="max-h-96 overflow-y-auto">
            {notifications.length === 0 ? (
              <div className="px-4 py-8 text-center text-sm text-[var(--text-muted)]">
                {t('notifications.empty')}
              </div>
            ) : (
              notifications.map((n) => {
                const isUnread = new Date(n.timestamp) > new Date(lastReadAt)
                const Icon = getActionIcon(n.action)
                const color = getActionColor(n.action)

                return (
                  <div
                    key={n.id}
                    className={`flex items-start gap-3 px-4 py-3 border-b border-[var(--border)] last:border-b-0 transition-colors ${
                      isUnread ? 'bg-[var(--accent-glow)]' : 'hover:bg-[var(--bg-tertiary)]'
                    }`}
                  >
                    <div
                      className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full"
                      style={{ backgroundColor: `${color}15`, color }}
                    >
                      <Icon size={14} />
                    </div>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm text-[var(--text-primary)] leading-snug">
                        {formatAction(n.action, n.resource, n.details as Record<string, unknown> | undefined, t)}
                      </p>
                      <p className="mt-0.5 text-xs text-[var(--text-muted)]">
                        {formatTimeAgo(n.timestamp, t)}
                      </p>
                    </div>
                    {isUnread && (
                      <span className="mt-2 h-2 w-2 shrink-0 rounded-full bg-[var(--accent)]" />
                    )}
                  </div>
                )
              })
            )}
          </div>

          {/* Footer */}
          {notifications.length > 0 && (
            <div className="border-t border-[var(--border)] px-4 py-2 text-center">
              <span className="text-xs text-[var(--text-muted)]">
                {t('notifications.showingRecent', { count: notifications.length })}
              </span>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
