import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Users, Laptop2, Activity, AlertCircle, Loader2 } from 'lucide-react'
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import { api } from '@/api/client'
import Stats from '@/components/ui/Stats'
import Card from '@/components/ui/Card'
import NetworkMap from '@/components/NetworkMap'

interface AnalyticsSummary {
  total_rx_bytes: number
  total_tx_bytes: number
  unique_users: number
  unique_devices: number
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
  return (
    <div className="flex items-center justify-center py-12 text-[var(--text-muted)]">
      <Loader2 size={20} className="animate-spin mr-2" />
      <span className="text-sm font-mono">Loading...</span>
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

  const now = new Date()
  const from = new Date(now.getTime() - 24 * 60 * 60 * 1000).toISOString()
  const to = now.toISOString()

  const summaryQuery = useQuery<AnalyticsSummary>({
    queryKey: ['analytics', 'summary'],
    queryFn: () => api.get('/analytics/summary'),
  })

  const bandwidthQuery = useQuery<BandwidthBucket[]>({
    queryKey: ['analytics', 'bandwidth', from, to],
    queryFn: () => api.get(`/analytics/bandwidth?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`),
  })

  const topUsersQuery = useQuery<TopUser[]>({
    queryKey: ['analytics', 'top-users'],
    queryFn: () => api.get('/analytics/top-users?limit=5'),
  })

  const summary = summaryQuery.data
  const totalBandwidth = summary
    ? formatBytes(summary.total_rx_bytes + summary.total_tx_bytes)
    : '--'

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
          label={t('dashboard.activeUsers')}
          value={summary ? String(summary.unique_users) : '--'}
          icon={<Users size={18} />}
        />
        <Stats
          label={t('dashboard.connectedDevices')}
          value={summary ? String(summary.unique_devices) : '--'}
          icon={<Laptop2 size={18} />}
        />
        <Stats
          label={t('dashboard.totalRx')}
          value={summary ? formatBytes(summary.total_rx_bytes) : '--'}
          icon={<Activity size={18} />}
        />
        <Stats
          label={t('dashboard.bandwidth')}
          value={totalBandwidth}
          icon={<Activity size={18} />}
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
                  <Area type="monotone" dataKey="rx" stroke="#00ff88" fill="url(#rxGrad)" strokeWidth={2} name="RX" />
                  <Area type="monotone" dataKey="tx" stroke="#00aaff" fill="url(#txGrad)" strokeWidth={2} name="TX" />
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
                    <th className="text-left pb-2 font-medium">User</th>
                    <th className="text-left pb-2 font-medium">RX</th>
                    <th className="text-left pb-2 font-medium">TX</th>
                    <th className="text-left pb-2 font-medium">Total</th>
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

        {/* Summary card */}
        <Card>
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            {t('dashboard.networkMap')}
          </h2>
          <div className="h-[300px]">
            <NetworkMap />
          </div>
        </Card>
      </div>
    </div>
  )
}
