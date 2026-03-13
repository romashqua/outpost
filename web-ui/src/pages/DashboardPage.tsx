import { useTranslation } from 'react-i18next'
import { Users, Laptop2, Server, Cable, Activity } from 'lucide-react'
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import Stats from '@/components/ui/Stats'
import Card from '@/components/ui/Card'
import Badge from '@/components/ui/Badge'
import NetworkMap from '@/components/NetworkMap'

const bandwidthData = [
  { time: '00:00', rx: 120, tx: 80 },
  { time: '02:00', rx: 90, tx: 60 },
  { time: '04:00', rx: 45, tx: 30 },
  { time: '06:00', rx: 60, tx: 40 },
  { time: '08:00', rx: 180, tx: 120 },
  { time: '10:00', rx: 340, tx: 220 },
  { time: '12:00', rx: 420, tx: 280 },
  { time: '14:00', rx: 380, tx: 260 },
  { time: '16:00', rx: 450, tx: 300 },
  { time: '18:00', rx: 320, tx: 210 },
  { time: '20:00', rx: 250, tx: 170 },
  { time: '22:00', rx: 180, tx: 120 },
]

const recentActivity = [
  { id: 1, time: '14:23:01', user: 'ivanov', action: 'Connected via gw-moscow-01', type: 'connect' },
  { id: 2, time: '14:21:45', user: 'petrov', action: 'MFA challenge passed', type: 'auth' },
  { id: 3, time: '14:20:12', user: 'admin', action: 'Created user sidorov', type: 'admin' },
  { id: 4, time: '14:18:33', user: 'kozlov', action: 'Device laptop-02 registered', type: 'device' },
  { id: 5, time: '14:15:07', user: 'system', action: 'Gateway gw-spb-01 health check OK', type: 'system' },
  { id: 6, time: '14:12:55', user: 'fedorov', action: 'Disconnected from gw-nsk-01', type: 'disconnect' },
]

const liveConnections = [
  { peer: 'ivanov@laptop-01', gateway: 'gw-moscow-01', ip: '10.0.1.5', rx: '2.4 GB', tx: '1.1 GB', status: 'connected' as const },
  { peer: 'petrov@desktop-01', gateway: 'gw-moscow-01', ip: '10.0.1.12', rx: '890 MB', tx: '340 MB', status: 'connected' as const },
  { peer: 'kozlov@laptop-02', gateway: 'gw-spb-01', ip: '10.0.2.3', rx: '1.2 GB', tx: '560 MB', status: 'connected' as const },
  { peer: 'sidorov@phone-01', gateway: 'gw-nsk-01', ip: '10.0.3.7', rx: '120 MB', tx: '45 MB', status: 'connected' as const },
]

export default function DashboardPage() {
  const { t } = useTranslation()

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-6">
        <span className="font-mono text-[var(--accent)] mr-2">&gt;_</span>
        {t('dashboard.title')}
      </h1>

      {/* Stats row */}
      <div className="grid grid-cols-5 gap-4 mb-6">
        <Stats label={t('dashboard.activeUsers')} value="47" trend={12} icon={<Users size={18} />} />
        <Stats label={t('dashboard.connectedDevices')} value="83" trend={5} icon={<Laptop2 size={18} />} />
        <Stats label={t('dashboard.activeGateways')} value="6" trend={0} icon={<Server size={18} />} />
        <Stats label={t('dashboard.s2sTunnels')} value="4" trend={-2} icon={<Cable size={18} />} />
        <Stats label={t('dashboard.bandwidth')} value="2.4 Gbps" trend={18} icon={<Activity size={18} />} />
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
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={bandwidthData}>
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
                <Area type="monotone" dataKey="rx" stroke="#00ff88" fill="url(#rxGrad)" strokeWidth={2} />
                <Area type="monotone" dataKey="tx" stroke="#00aaff" fill="url(#txGrad)" strokeWidth={2} />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        </Card>
      </div>

      <div className="grid grid-cols-2 gap-4">
        {/* Recent Activity */}
        <Card>
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            {t('dashboard.recentActivity')}
          </h2>
          <div className="space-y-2">
            {recentActivity.map((entry) => (
              <div
                key={entry.id}
                className="flex items-center gap-3 rounded-md px-3 py-2 text-xs hover:bg-[var(--bg-tertiary)] transition-colors"
              >
                <span className="font-mono text-[var(--text-muted)] w-16 shrink-0">
                  {entry.time}
                </span>
                <span className="font-mono text-[var(--accent)] w-16 shrink-0 truncate">
                  {entry.user}
                </span>
                <span className="text-[var(--text-secondary)] truncate">
                  {entry.action}
                </span>
              </div>
            ))}
          </div>
        </Card>

        {/* Live Connections */}
        <Card>
          <h2 className="text-sm font-medium text-[var(--text-primary)] mb-4 font-mono">
            {t('dashboard.liveConnections')}
          </h2>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-[var(--text-muted)] uppercase tracking-wider">
                  <th className="text-left pb-2 font-medium">Peer</th>
                  <th className="text-left pb-2 font-medium">Gateway</th>
                  <th className="text-left pb-2 font-medium">IP</th>
                  <th className="text-left pb-2 font-medium">RX/TX</th>
                  <th className="text-left pb-2 font-medium">Status</th>
                </tr>
              </thead>
              <tbody>
                {liveConnections.map((conn, i) => (
                  <tr key={i} className="border-t border-[var(--border)] hover:bg-[var(--accent-glow)] transition-colors">
                    <td className="py-2 font-mono text-[var(--text-secondary)]">{conn.peer}</td>
                    <td className="py-2 font-mono text-[var(--text-muted)]">{conn.gateway}</td>
                    <td className="py-2 font-mono text-[var(--accent)]">{conn.ip}</td>
                    <td className="py-2 font-mono text-[var(--text-muted)]">{conn.rx} / {conn.tx}</td>
                    <td className="py-2">
                      <Badge variant="online" pulse>{conn.status}</Badge>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      </div>
    </div>
  )
}
