import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Lock, User, Terminal, ShieldCheck } from 'lucide-react'
import { useAuthStore } from '@/store/auth'
import { useToastStore } from '@/store/toast'
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
  const addToast = useToastStore((s) => s.addToast)
  const login = useAuthStore((s) => s.login)
  const verifyMFA = useAuthStore((s) => s.verifyMFA)
  const needsMFA = useAuthStore((s) => s.needsMFA)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [mfaCode, setMfaCode] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!username || !password) return
    setLoading(true)
    setError('')
    try {
      await login(username, password)
      // If needsMFA was set, the store updated but we stay on the page
      const state = useAuthStore.getState()
      if (state.isAuthenticated) {
        navigate('/')
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('login.error')
      setError(msg)
      addToast(msg, 'error')
    } finally {
      setLoading(false)
    }
  }

  const handleMFA = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!mfaCode) return
    setLoading(true)
    setError('')
    try {
      await verifyMFA(mfaCode)
      navigate('/')
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('login.error')
      setError(msg)
      addToast(msg, 'error')
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

          {/* MFA Form */}
          {needsMFA ? (
            <form onSubmit={handleMFA} className="flex flex-col gap-4">
              <div className="text-center mb-2">
                <ShieldCheck size={24} className="inline-block text-[var(--accent)] mb-2" />
                <p className="text-sm text-[var(--text-secondary)] font-mono">
                  {t('login.mfaPrompt', 'Enter your MFA code')}
                </p>
              </div>
              <Input
                placeholder={t('login.mfaCode', 'MFA Code')}
                value={mfaCode}
                onChange={(e) => setMfaCode(e.target.value)}
                icon={<ShieldCheck size={16} />}
                autoFocus
                autoComplete="one-time-code"
              />

              {error && (
                <div className="rounded-md bg-[rgba(255,68,68,0.1)] border border-[rgba(255,68,68,0.2)] px-3 py-2 text-xs text-[var(--danger)] font-mono">
                  &gt; {error}
                </div>
              )}

              <Button type="submit" disabled={loading || !mfaCode} className="mt-2 w-full">
                {loading ? (
                  <span className="cursor-blink font-mono">{t('common.loading')}</span>
                ) : (
                  <>
                    <span className="font-mono">&gt;_</span>
                    {t('login.verify', 'Verify')}
                  </>
                )}
              </Button>
            </form>
          ) : (
            /* Login Form */
            <form onSubmit={handleLogin} className="flex flex-col gap-4">
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
          )}

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
