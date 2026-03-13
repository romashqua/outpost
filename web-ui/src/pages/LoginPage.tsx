import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Lock, User, Terminal } from 'lucide-react'
import { useAuthStore } from '@/store/auth'
import Input from '@/components/ui/Input'
import Button from '@/components/ui/Button'

const RAIN_COLUMNS = Array.from({ length: 20 }, (_, i) => ({
  left: `${i * 5 + Math.random() * 3}%`,
  delay: `${Math.random() * 8}s`,
  duration: `${8 + Math.random() * 12}s`,
  chars: Array.from({ length: 20 }, () =>
    String.fromCharCode(0x30A0 + Math.random() * 96)
  ).join(' '),
}))

export default function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const login = useAuthStore((s) => s.login)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!username || !password) return
    setLoading(true)
    setError('')
    try {
      await login(username, password)
      navigate('/')
    } catch {
      setError(t('login.error'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="relative flex min-h-screen items-center justify-center bg-[var(--bg-primary)] overflow-hidden">
      {/* Matrix rain background */}
      <div className="absolute inset-0 overflow-hidden">
        {RAIN_COLUMNS.map((col, i) => (
          <div
            key={i}
            className="rain-column"
            style={{
              left: col.left,
              animationDelay: col.delay,
              animationDuration: col.duration,
            }}
          >
            {col.chars}
          </div>
        ))}
      </div>

      {/* Grid overlay */}
      <div className="absolute inset-0 matrix-bg opacity-50" />

      {/* Login card */}
      <div className="relative z-10 w-full max-w-md px-4">
        <div className="rounded-xl border border-[var(--border)] bg-[var(--bg-card)] p-8 shadow-2xl backdrop-blur-sm">
          {/* Logo */}
          <div className="mb-8 text-center">
            <h1
              className="text-4xl font-bold font-mono tracking-[0.3em]"
              style={{
                color: 'var(--accent)',
                textShadow: '0 0 30px var(--accent-glow), 0 0 60px rgba(0,255,136,0.1)',
              }}
            >
              {t('login.title')}
            </h1>
            <div className="mt-2 flex items-center justify-center gap-2 text-[var(--text-muted)]">
              <Terminal size={14} />
              <span className="text-xs font-mono tracking-wider">{t('login.subtitle')}</span>
            </div>

            {/* ASCII decoration */}
            <pre className="mt-4 text-[10px] text-[var(--accent)] opacity-20 font-mono leading-tight">
{`  ╔══════════════════════╗
  ║  SECURE ACCESS POINT ║
  ╚══════════════════════╝`}
            </pre>
          </div>

          {/* Form */}
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <Input
              placeholder={t('login.username')}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              icon={<User size={16} />}
              autoFocus
              autoComplete="username"
            />
            <Input
              type="password"
              placeholder={t('login.password')}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              icon={<Lock size={16} />}
              autoComplete="current-password"
            />

            {error && (
              <div className="rounded-md bg-[rgba(255,68,68,0.1)] border border-[rgba(255,68,68,0.2)] px-3 py-2 text-xs text-[var(--danger)] font-mono">
                &gt; {error}
              </div>
            )}

            <Button type="submit" disabled={loading || !username || !password} className="mt-2 w-full">
              {loading ? (
                <span className="cursor-blink font-mono">{t('common.loading')}</span>
              ) : (
                <>
                  <span className="font-mono">&gt;_</span>
                  {t('login.signIn')}
                </>
              )}
            </Button>
          </form>

          {/* Footer */}
          <div className="mt-6 text-center">
            <button className="text-xs text-[var(--text-muted)] hover:text-[var(--accent)] transition-colors font-mono cursor-pointer">
              {t('login.forgotPassword')}
            </button>
          </div>

          {/* Version */}
          <div className="mt-4 text-center">
            <span className="text-[10px] text-[var(--text-muted)] font-mono opacity-40">
              v0.1.0 // Apache 2.0
            </span>
          </div>
        </div>
      </div>
    </div>
  )
}
