import { useTranslation } from 'react-i18next'
import { NavLink, useNavigate } from 'react-router-dom'
import { clsx } from 'clsx'
import {
  LayoutDashboard,
  Users,
  Laptop2,
  Network,
  Server,
  Cable,
  Route,
  BarChart3,
  ShieldCheck,
  Settings,
  LogOut,
  Globe,
  Bell,
} from 'lucide-react'
import { useAuthStore } from '@/store/auth'

const navItems = [
  { to: '/', icon: LayoutDashboard, labelKey: 'nav.dashboard' },
  { to: '/users', icon: Users, labelKey: 'nav.users' },
  { to: '/devices', icon: Laptop2, labelKey: 'nav.devices' },
  { to: '/networks', icon: Network, labelKey: 'nav.networks' },
  { to: '/gateways', icon: Server, labelKey: 'nav.gateways' },
  { to: '/s2s', icon: Cable, labelKey: 'nav.s2s' },
  { to: '/smart-routes', icon: Route, labelKey: 'nav.smartRoutes' },
  { to: '/analytics', icon: BarChart3, labelKey: 'nav.analytics' },
  { to: '/compliance', icon: ShieldCheck, labelKey: 'nav.compliance' },
  { to: '/settings', icon: Settings, labelKey: 'nav.settings' },
]

export default function Layout({ children }: { children: React.ReactNode }) {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)

  const toggleLang = () => {
    const next = i18n.language === 'en' ? 'ru' : 'en'
    i18n.changeLanguage(next)
    localStorage.setItem('outpost-lang', next)
  }

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  return (
    <div className="flex h-screen bg-[var(--bg-primary)]">
      {/* Sidebar */}
      <aside className="flex w-56 flex-col border-r border-[var(--border)] bg-[var(--bg-secondary)]">
        {/* Logo */}
        <div className="flex h-14 items-center px-5 border-b border-[var(--border)]">
          <span
            className="text-xl font-bold font-mono tracking-wider"
            style={{
              color: 'var(--accent)',
              textShadow: '0 0 20px var(--accent-glow)',
            }}
          >
            OUTPOST
          </span>
          <span className="ml-2 text-xs text-[var(--text-muted)] font-mono">vpn</span>
        </div>

        {/* Navigation */}
        <nav className="flex-1 overflow-y-auto py-3 px-2">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/'}
              className={({ isActive }) =>
                clsx(
                  'flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-all duration-150 mb-0.5',
                  isActive
                    ? 'bg-[var(--accent-glow)] text-[var(--accent)] shadow-[inset_0_0_20px_rgba(0,255,136,0.05)]'
                    : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]',
                )
              }
            >
              <item.icon size={16} />
              <span>{t(item.labelKey)}</span>
            </NavLink>
          ))}
        </nav>

        {/* User section */}
        <div className="border-t border-[var(--border)] p-3">
          <div className="flex items-center gap-3 rounded-md px-2 py-2">
            <div className="flex h-8 w-8 items-center justify-center rounded-full bg-[var(--bg-tertiary)] text-xs font-mono text-[var(--accent)]">
              {user?.username?.[0]?.toUpperCase() || '?'}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm text-[var(--text-primary)] truncate">{user?.username}</p>
              <p className="text-xs text-[var(--text-muted)] font-mono">{user?.role}</p>
            </div>
            <button
              onClick={handleLogout}
              className="rounded p-1.5 text-[var(--text-muted)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--danger)] transition-colors cursor-pointer"
              title="Logout"
            >
              <LogOut size={14} />
            </button>
          </div>
        </div>
      </aside>

      {/* Main */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Top bar */}
        <header className="flex h-14 items-center justify-end gap-3 border-b border-[var(--border)] bg-[var(--bg-secondary)] px-6">
          <button
            onClick={toggleLang}
            className="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] transition-colors font-mono cursor-pointer"
          >
            <Globe size={14} />
            {i18n.language.toUpperCase()}
          </button>
          <button className="relative rounded-md p-2 text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)] transition-colors cursor-pointer">
            <Bell size={16} />
            <span className="absolute right-1.5 top-1.5 h-1.5 w-1.5 rounded-full bg-[var(--accent)]" />
          </button>
        </header>

        {/* Content */}
        <main className="flex-1 overflow-y-auto p-6">
          {children}
        </main>
      </div>
    </div>
  )
}
