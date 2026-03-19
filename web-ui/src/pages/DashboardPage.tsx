import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Users, Laptop2, AlertCircle, Loader2, Network, Shield, Clock } from 'lucide-react'
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import { api } from '@/api/client'
import Stats from '@/components/ui/Stats'
import Card from '@/components/ui/Card'
import NetworkMap from '@/components/NetworkMap'

interface DashboardStats {
  active_users: number
  total_users: number
  active_devices: number
  total_devices: number
  active_gateways: number
  total_gateways: number
  active_networks: number
  s2s_tunnels: number
}

interface BandwidthBucket {
  bucket: string
  rx_bytes: number
  tx_bytes: number
}

interface TopUser {
  user_id: string
  username: string
  rx_bytes: number
  tx_bytes: number
}

interface AuditEntry {
  id: number
  timestamp: string
  user_id?: string
  action: string
  resource: string
  details?: Record<string, unknown>
  ip_address: string
  user_agent: string
}

function formatAction(action: string): string {
  // Convert "POST /api/v1/devices/xxx/approve" → "Device approved"
  const map: Record<string, string> = {
    'POST /api/v1/auth/login': 'User login',
    'POST /api/v1/auth/logout': 'User logout',
  }
  if (map[action]) return map[action]
  // Parse "METHOD /api/v1/resource..." pattern
  const m = action.match(/^(POST|PUT|DELETE|PATCH)\s+\/api\/v1\/(\w+)/)
  if (m) {
    const [, method, resource] = m
    const res = resource.replace(/-/g, ' ')
    if (action.includes('/approve')) return `${res} approved`
    if (action.includes('/revoke')) return `${res} revoked`
    if (method === 'POST') return `${res} created`
    if (method === 'DELETE') return `${res} deleted`
    if (method === 'PUT' || method === 'PATCH') return `${res} updated`
  }
  // Semantic events
  if (action.startsWith('gateway.')) return action.replace('.', ' ')
  if (action.startsWith('device.')) return action.replace('.', ' ')
  return action
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  const value = bytes / Math.pow(1024, i)
  return `${value.toFixed(1)} ${units[i]}`
}

function formatBandwidthForChart(bytes: number): number {
  // Convert to MB for chart readability
  return Math.round(bytes / (1024 * 1024))
}

function formatBucketTime(bucket: string): string {
  try {
    const d = new Date(bucket)
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  } catch {
    return bucket
  }
}

function LoadingState() {
  const { t } = useTranslation()
  return (
    <div className="flex items-center justify-center py-12 text-[var(--text-muted)]">
      <Loader2 size={20} className="animate-spin mr-2" />
      <span className="text-sm font-mono">{t('common.loading')}</span>
    </div>
  )
}

function ErrorState({ message }: { message: string }) {
  return (
    <div className="flex items-center justify-center py-12 text-[var(--danger)]">
      <AlertCircle size={18} className="mr-2" />
      <span className="text-sm font-mono">{message}</span>
    </div>
  )
}

export default function DashboardPage() {
  const { t } = useTranslation()

  const { from, to } = useMemo(() => {
    const n = new Date()
    return {
      from: new Date(n.getTime() - 24 * 60 * 60 * 1000).toISOString(),
      to: n.toISOString(),
    }
  }, [])

  const statsQuery = useQuery<DashboardStats>({
    queryKey: ['dashboard', 'stats'],
    queryFn: () => api.get('/dashboard/stats'),
  })

  const bandwidthQuery = useQuery<BandwidthBucket[]>({
    queryKey: ['analytics', 'bandwidth', from, to],
    queryFn: () => api.get(`/analytics/bandwidth?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`),
  })

  const topUsersQuery = useQuery<TopUser[]>({
    queryKey: ['analytics', 'top-users'],
    queryFn: () => api.get('/analytics/top-users?limit=5'),
  })

  const auditQuery = useQuery<{ data: AuditEntry[] }>({
    queryKey: ['audit', 'recent'],
    queryFn: () => api.get('/audit?per_page=5'),
  })

  const stats = statsQuery.data

  const chartData = (bandwidthQuery.data ?? []).map((b) => ({
    time: formatBucketTime(b.bucket),
    rx: formatBandwidthForChart(b.rx_bytes),
    tx: formatBandwidthForChart(b.tx_bytes),
  }))

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
        <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
        {t('dashboard.title')}
      </h1>

      {/* Stats row */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        <Stats
          label={t('dashboard.totalUsers')}
          value={stats ? `${stats.active_users} / ${stats.total_users}` : '--'}
          icon={<Users size={18} />}
        />
        <Stats
          label={t('dashboard.activeDevices')}
          value={stats ? `${stats.active_devices} / ${stats.total_devices}` : '--'}
          icon={<Laptop2 size={18} />}
        />
        <Stats
          label={t('dashboard.activeGateways')}
          value={stats ? `${stats.active_gateways} / ${stats.total_gateways}` : '--'}
          icon={<Shield size={18} />}
        />
        <Stats
          label={t('dashboard.networks')}
          value={stats ? String(stats.active_networks) : '--'}
          icon={<Network size={18} />}
        />
      </div>

      <div className="grid grid-cols-2 gap-4 mb-6">
        {/* Network Map */}
        <Card className="col-span-1">
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            {t('dashboard.networkMap')}
          </h2>
          <div className="h-[300px]">
            <NetworkMap />
          </div>
        </Card>

        {/* Bandwidth Chart */}
        <Card className="col-span-1">
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            {t('dashboard.bandwidthChart')}
          </h2>
          <div className="h-[300px]">
            {bandwidthQuery.isLoading ? (
              <LoadingState />
            ) : bandwidthQuery.isError ? (
              <ErrorState message={bandwidthQuery.error.message} />
            ) : chartData.length === 0 ? (
              <div className="flex items-center justify-center h-full text-[var(--text-muted)]">
                <span className="text-sm font-mono">{t('analytics.noBandwidthData')}</span>
              </div>
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={chartData}>
                  <defs>
                    <linearGradient id="rxGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#00ff88" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="#00ff88" stopOpacity={0} />
                    </linearGradient>
                    <linearGradient id="txGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#00aaff" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="#00aaff" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                  <XAxis dataKey="time" stroke="var(--text-muted)" fontSize={11} />
                  <YAxis stroke="var(--text-muted)" fontSize={11} unit=" MB" />
                  <Tooltip
                    contentStyle={{
                      background: 'var(--bg-card)',
                      border: '1px solid var(--border)',
                      borderRadius: '6px',
                      fontFamily: "'JetBrains Mono', monospace",
                      fontSize: '11px',
                      color: 'var(--text-primary)',
                    }}
                    formatter={(value: number) => [`${value} MB`]}
                  />
                  <Area type="monotone" dataKey="rx" stroke="#00ff88" fill="url(#rxGrad)" strokeWidth={2} name={t('dashboard.rx')} />
                  <Area type="monotone" dataKey="tx" stroke="#00aaff" fill="url(#txGrad)" strokeWidth={2} name={t('dashboard.tx')} />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </div>
        </Card>
      </div>

      <div className="grid grid-cols-2 gap-4">
        {/* Top Users */}
        <Card>
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            {t('dashboard.topUsers', 'Top Users')}
          </h2>
          {topUsersQuery.isLoading ? (
            <LoadingState />
          ) : topUsersQuery.isError ? (
            <ErrorState message={topUsersQuery.error.message} />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="text-[var(--text-muted)] uppercase tracking-wider">
                    <th className="text-left pb-2 font-medium">{t('dashboard.user')}</th>
                    <th className="text-left pb-2 font-medium">{t('dashboard.rx')}</th>
                    <th className="text-left pb-2 font-medium">{t('dashboard.tx')}</th>
                    <th className="text-left pb-2 font-medium">{t('dashboard.total')}</th>
                  </tr>
                </thead>
                <tbody>
                  {(topUsersQuery.data ?? []).map((user) => (
                    <tr key={user.user_id} className="border-t border-[var(--border)] hover:bg-[var(--accent-glow)] transition-colors">
                      <td className="py-2 font-mono text-[var(--accent)]">{user.username}</td>
                      <td className="py-2 font-mono text-[var(--text-muted)]">{formatBytes(user.rx_bytes)}</td>
                      <td className="py-2 font-mono text-[var(--text-muted)]">{formatBytes(user.tx_bytes)}</td>
                      <td className="py-2 font-mono text-[var(--text-secondary)]">{formatBytes(user.rx_bytes + user.tx_bytes)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>

        {/* Recent Activity */}
        <Card>
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            <Clock size={14} className="inline mr-2" />
            {t('dashboard.recentActivity')}
          </h2>
          {auditQuery.isLoading ? (
            <LoadingState />
          ) : auditQuery.isError ? (
            <ErrorState message={auditQuery.error.message} />
          ) : (auditQuery.data?.data ?? []).length === 0 ? (
            <div className="flex items-center justify-center py-12 text-[var(--text-muted)]">
              <span className="text-sm font-mono">{t('common.noData')}</span>
            </div>
          ) : (
            <div className="space-y-2">
              {(auditQuery.data?.data ?? []).map((entry) => (
                <div
                  key={entry.id}
                  className="flex items-center justify-between py-2 border-b border-[var(--border)] last:border-0"
                >
                  <div className="flex-1 min-w-0">
                    <span className="text-xs font-mono text-[var(--accent)]">{entry.user_id ? entry.user_id.slice(0, 8) : 'system'}</span>
                    <span className="text-xs text-[var(--text-muted)] mx-2">—</span>
                    <span className="text-xs font-mono text-[var(--text-secondary)]">{formatAction(entry.action)}</span>
                  </div>
                  <span className="text-xs font-mono text-[var(--text-muted)] ml-2 shrink-0">
                    {new Date(entry.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                  </span>
                </div>
              ))}
            </div>
          )}
        </Card>
      </div>
    </div>
  )
}
