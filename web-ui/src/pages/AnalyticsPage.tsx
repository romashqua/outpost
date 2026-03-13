import { useTranslation } from 'react-i18next'
import {
  AreaChart, Area, BarChart, Bar,
  XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from 'recharts'
import Card from '@/components/ui/Card'
import Stats from '@/components/ui/Stats'
import { Activity, TrendingUp, Clock } from 'lucide-react'

const bandwidthTimeline = [
  { time: 'Mon', rx: 320, tx: 210 },
  { time: 'Tue', rx: 380, tx: 240 },
  { time: 'Wed', rx: 450, tx: 300 },
  { time: 'Thu', rx: 420, tx: 280 },
  { time: 'Fri', rx: 490, tx: 320 },
  { time: 'Sat', rx: 180, tx: 90 },
  { time: 'Sun', rx: 120, tx: 60 },
]

const topUsers = [
  { user: 'morozov', traffic: 18.5 },
  { user: 'ivanov', traffic: 12.4 },
  { user: 'petrov', traffic: 8.9 },
  { user: 'kozlov', traffic: 6.2 },
  { user: 'sidorov', traffic: 3.1 },
]

const heatmapData: number[][] = [
  [2, 1, 0, 0, 1, 5, 12, 28, 35, 42, 38, 30, 32, 35, 40, 38, 34, 25, 18, 12, 8, 5, 3, 2],
  [3, 1, 1, 0, 0, 4, 10, 25, 32, 40, 36, 28, 30, 33, 38, 35, 30, 22, 15, 10, 6, 4, 3, 2],
  [2, 1, 0, 0, 1, 6, 14, 30, 38, 45, 42, 35, 36, 38, 42, 40, 36, 28, 20, 14, 8, 5, 3, 2],
  [3, 2, 1, 0, 1, 5, 12, 28, 35, 42, 38, 32, 34, 36, 40, 38, 34, 26, 18, 12, 7, 4, 3, 2],
  [2, 1, 0, 0, 0, 4, 10, 24, 30, 38, 34, 28, 30, 32, 36, 34, 30, 22, 15, 10, 6, 4, 2, 1],
  [1, 0, 0, 0, 0, 1, 3, 8, 12, 15, 14, 12, 10, 8, 6, 5, 4, 3, 3, 2, 2, 1, 1, 0],
  [0, 0, 0, 0, 0, 0, 2, 5, 8, 10, 8, 6, 5, 4, 3, 3, 2, 2, 2, 1, 1, 0, 0, 0],
]

const days = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']

const locationData = [
  { location: 'Moscow', connections: 34 },
  { location: 'Saint Petersburg', connections: 18 },
  { location: 'Novosibirsk', connections: 12 },
  { location: 'Kazan', connections: 8 },
  { location: 'Remote', connections: 11 },
]

function getHeatColor(val: number): string {
  if (val === 0) return 'var(--bg-tertiary)'
  const intensity = Math.min(val / 45, 1)
  return `rgba(0, 255, 136, ${0.1 + intensity * 0.6})`
}

export default function AnalyticsPage() {
  const { t } = useTranslation()

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
        <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
        {t('analytics.title')}
      </h1>

      {/* Stats */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        <Stats label={t('analytics.totalTraffic')} value="2.4 TB" trend={15} icon={<Activity size={18} />} />
        <Stats label={t('analytics.peakBandwidth')} value="4.2 Gbps" trend={8} icon={<TrendingUp size={18} />} />
        <Stats label={t('analytics.avgLatency')} value="12ms" trend={-5} icon={<Clock size={18} />} />
      </div>

      <div className="grid grid-cols-2 gap-4 mb-6">
        {/* Bandwidth over time */}
        <Card>
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            {t('analytics.bandwidthOverTime')}
          </h2>
          <div className="h-[250px]">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={bandwidthTimeline}>
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
                <YAxis stroke="var(--text-muted)" fontSize={11} />
                <Tooltip
                  contentStyle={{
                    background: 'var(--bg-card)',
                    border: '1px solid var(--border)',
                    borderRadius: '6px',
                    fontFamily: "'JetBrains Mono', monospace",
                    fontSize: '11px',
                    color: 'var(--text-primary)',
                  }}
                />
                <Area type="monotone" dataKey="rx" stroke="#00ff88" fill="url(#anaRx)" strokeWidth={2} />
                <Area type="monotone" dataKey="tx" stroke="#00aaff" fill="url(#anaTx)" strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </Card>

        {/* Top users */}
        <Card>
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            {t('analytics.topUsers')}
          </h2>
          <div className="h-[250px]">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={topUsers} layout="vertical">
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" horizontal={false} />
                <XAxis type="number" stroke="var(--text-muted)" fontSize={11} />
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
                  formatter={(val: number) => [`${val} GB`, 'Traffic']}
                />
                <Bar dataKey="traffic" fill="#00ff88" radius={[0, 4, 4, 0]} barSize={20} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </Card>
      </div>

      <div className="grid grid-cols-2 gap-4">
        {/* Connection heatmap */}
        <Card>
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            {t('analytics.connectionHeatmap')}
          </h2>
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
              {heatmapData.map((row, dayIdx) => (
                <div key={dayIdx} className="flex items-center gap-0.5 mb-0.5">
                  <span className="text-[10px] font-mono text-[var(--text-muted)] w-10 text-right pr-2">
                    {days[dayIdx]}
                  </span>
                  {row.map((val, hourIdx) => (
                    <div
                      key={hourIdx}
                      className="flex-1 aspect-square rounded-sm"
                      style={{ background: getHeatColor(val), minWidth: '12px', minHeight: '12px' }}
                      title={`${days[dayIdx]} ${String(hourIdx).padStart(2, '0')}:00 - ${val} connections`}
                    />
                  ))}
                </div>
              ))}
            </div>
          </div>
        </Card>

        {/* Active by location */}
        <Card>
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            {t('analytics.activeByLocation')}
          </h2>
          <div className="space-y-3">
            {locationData.map((loc) => {
              const pct = Math.round((loc.connections / 83) * 100)
              return (
                <div key={loc.location}>
                  <div className="flex justify-between mb-1">
                    <span className="text-sm text-[var(--text-secondary)]">{loc.location}</span>
                    <span className="font-mono text-xs text-[var(--accent)]">{loc.connections}</span>
                  </div>
                  <div className="h-2 rounded-full bg-[var(--bg-tertiary)]">
                    <div
                      className="h-full rounded-full bg-[var(--accent)] transition-all"
                      style={{ width: `${pct}%`, opacity: 0.5 + (pct / 200) }}
                    />
                  </div>
                </div>
              )
            })}
          </div>
        </Card>
      </div>
    </div>
  )
}
