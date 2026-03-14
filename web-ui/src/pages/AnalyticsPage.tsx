import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  AreaChart, Area, BarChart, Bar,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from 'recharts'
import { api } from '@/api/client'
import Card from '@/components/ui/Card'
import Stats from '@/components/ui/Stats'
import { Activity, TrendingUp, Users } from 'lucide-react'

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

interface HeatmapEntry {
  hour: number
  day_of_week: number
  count: number
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  const val = bytes / Math.pow(1024, i)
  return `${val.toFixed(val >= 100 ? 0 : 1)} ${units[i]}`
}

function formatBytesShort(bytes: number): string {
  if (bytes === 0) return '0'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  const val = bytes / Math.pow(1024, i)
  return `${val.toFixed(1)} ${units[i]}`
}

const days = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']

function getHeatColor(val: number, maxVal: number): string {
  if (val === 0) return 'var(--bg-tertiary)'
  const intensity = Math.min(val / Math.max(maxVal, 1), 1)
  return `rgba(0, 255, 136, ${0.1 + intensity * 0.6})`
}

export default function AnalyticsPage() {
  const { t } = useTranslation()

  const now = new Date()
  const weekAgo = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000)
  const fromParam = weekAgo.toISOString()
  const toParam = now.toISOString()

  const { data: summary } = useQuery({
    queryKey: ['analytics', 'summary'],
    queryFn: () => api.get<AnalyticsSummary>('/analytics/summary'),
  })

  const { data: bandwidth = [] } = useQuery({
    queryKey: ['analytics', 'bandwidth', fromParam, toParam],
    queryFn: () => api.get<BandwidthBucket[]>(`/analytics/bandwidth?from=${fromParam}&to=${toParam}&bucket=1h`),
  })

  const { data: topUsers = [] } = useQuery({
    queryKey: ['analytics', 'top-users'],
    queryFn: () => api.get<TopUser[]>('/analytics/top-users?limit=5'),
  })

  const { data: heatmapRaw = [] } = useQuery({
    queryKey: ['analytics', 'connections-heatmap'],
    queryFn: () => api.get<HeatmapEntry[]>('/analytics/connections-heatmap'),
  })

  // Transform bandwidth data for the chart
  const bandwidthChart = useMemo(() =>
    bandwidth.map((b) => ({
      time: new Date(b.bucket).toLocaleString(undefined, { weekday: 'short', hour: '2-digit' }),
      rx: b.rx_bytes,
      tx: b.tx_bytes,
    })),
    [bandwidth],
  )

  // Transform top users for the chart
  const topUsersChart = useMemo(() =>
    topUsers.map((u) => ({
      user: u.username,
      traffic: u.rx_bytes + u.tx_bytes,
    })),
    [topUsers],
  )

  // Build heatmap grid (7 days x 24 hours)
  const { heatmapGrid, heatmapMax } = useMemo(() => {
    const grid: number[][] = Array.from({ length: 7 }, () => Array(24).fill(0))
    let max = 0
    for (const entry of heatmapRaw) {
      const dayIdx = entry.day_of_week
      const hourIdx = entry.hour
      if (dayIdx >= 0 && dayIdx < 7 && hourIdx >= 0 && hourIdx < 24) {
        grid[dayIdx][hourIdx] = entry.count
        max = Math.max(max, entry.count)
      }
    }
    return { heatmapGrid: grid, heatmapMax: max }
  }, [heatmapRaw])

  const totalTraffic = summary ? summary.total_rx_bytes + summary.total_tx_bytes : 0

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
        <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
        {t('analytics.title')}
      </h1>

      {/* Stats */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        <Stats
          label={t('analytics.totalTraffic')}
          value={summary ? formatBytes(totalTraffic) : '--'}
          icon={<Activity size={18} />}
        />
        <Stats
          label={t('analytics.uniqueUsers')}
          value={summary ? String(summary.unique_users) : '--'}
          icon={<Users size={18} />}
        />
        <Stats
          label={t('analytics.uniqueDevices')}
          value={summary ? String(summary.unique_devices) : '--'}
          icon={<TrendingUp size={18} />}
        />
      </div>

      <div className="grid grid-cols-2 gap-4 mb-6">
        {/* Bandwidth over time */}
        <Card>
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            {t('analytics.bandwidthOverTime')}
          </h2>
          <div className="h-[250px]">
            {bandwidthChart.length === 0 ? (
              <div className="flex items-center justify-center h-full text-sm text-[var(--text-muted)]">
                {t('analytics.noBandwidthData')}
              </div>
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={bandwidthChart}>
                  <defs>
                    <linearGradient id="anaRx" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#00ff88" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="#00ff88" stopOpacity={0} />
                    </linearGradient>
                    <linearGradient id="anaTx" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#00aaff" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="#00aaff" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                  <XAxis dataKey="time" stroke="var(--text-muted)" fontSize={11} />
                  <YAxis stroke="var(--text-muted)" fontSize={11} tickFormatter={(v: number) => formatBytesShort(v)} />
                  <Tooltip
                    contentStyle={{
                      background: 'var(--bg-card)',
                      border: '1px solid var(--border)',
                      borderRadius: '6px',
                      fontFamily: "'JetBrains Mono', monospace",
                      fontSize: '11px',
                      color: 'var(--text-primary)',
                    }}
                    formatter={(val: number, name: string) => [formatBytes(val), name === 'rx' ? t('dashboard.rx') : t('dashboard.tx')]}
                  />
                  <Area type="monotone" dataKey="rx" stroke="#00ff88" fill="url(#anaRx)" strokeWidth={2} />
                  <Area type="monotone" dataKey="tx" stroke="#00aaff" fill="url(#anaTx)" strokeWidth={2} />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </div>
        </Card>

        {/* Top users */}
        <Card>
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            {t('analytics.topUsers')}
          </h2>
          <div className="h-[250px]">
            {topUsersChart.length === 0 ? (
              <div className="flex items-center justify-center h-full text-sm text-[var(--text-muted)]">
                {t('analytics.noUserData')}
              </div>
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={topUsersChart} layout="vertical">
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" horizontal={false} />
                  <XAxis type="number" stroke="var(--text-muted)" fontSize={11} tickFormatter={(v: number) => formatBytesShort(v)} />
                  <YAxis type="category" dataKey="user" stroke="var(--text-muted)" fontSize={11} width={80} />
                  <Tooltip
                    contentStyle={{
                      background: 'var(--bg-card)',
                      border: '1px solid var(--border)',
                      borderRadius: '6px',
                      fontFamily: "'JetBrains Mono', monospace",
                      fontSize: '11px',
                      color: 'var(--text-primary)',
                    }}
                    formatter={(val: number) => [formatBytes(val), t('analytics.traffic')]}
                  />
                  <Bar dataKey="traffic" fill="#00ff88" radius={[0, 4, 4, 0]} barSize={20} />
                </BarChart>
              </ResponsiveContainer>
            )}
          </div>
        </Card>
      </div>

      {/* Connection heatmap */}
      <Card>
        <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
          {t('analytics.connectionHeatmap')}
        </h2>
        {heatmapRaw.length === 0 ? (
          <div className="flex items-center justify-center py-8 text-sm text-[var(--text-muted)]">
            {t('analytics.noConnectionData')}
          </div>
        ) : (
          <div className="overflow-x-auto">
            <div className="min-w-[500px]">
              {/* Hours header */}
              <div className="flex gap-0.5 mb-1 ml-10">
                {Array.from({ length: 24 }, (_, i) => (
                  <div key={i} className="flex-1 text-center text-[9px] font-mono text-[var(--text-muted)]">
                    {i % 4 === 0 ? `${String(i).padStart(2, '0')}` : ''}
                  </div>
                ))}
              </div>
              {/* Rows */}
              {heatmapGrid.map((row, dayIdx) => (
                <div key={dayIdx} className="flex items-center gap-0.5 mb-0.5">
                  <span className="text-[10px] font-mono text-[var(--text-muted)] w-10 text-right pr-2">
                    {days[dayIdx]}
                  </span>
                  {row.map((val, hourIdx) => (
                    <div
                      key={hourIdx}
                      className="flex-1 aspect-square rounded-sm"
                      style={{ background: getHeatColor(val, heatmapMax), minWidth: '12px', minHeight: '12px' }}
                      title={`${days[dayIdx]} ${String(hourIdx).padStart(2, '0')}:00 — ${val} ${t('analytics.connections')}`}
                    />
                  ))}
                </div>
              ))}
            </div>
          </div>
        )}
      </Card>
    </div>
  )
}
