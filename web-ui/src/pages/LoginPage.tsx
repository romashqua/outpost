import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Lock, User, Terminal, ShieldCheck, Mail, KeyRound } from 'lucide-react'
import OutpostLogo from '@/components/OutpostLogo'
import { useAuthStore } from '@/store/auth'
import { useToastStore } from '@/store/toast'
import { api } from '@/api/client'
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

type View = 'login' | 'mfa' | 'forgot' | 'changePassword'

export default function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const addToast = useToastStore((s) => s.addToast)
  const login = useAuthStore((s) => s.login)
  const verifyMFA = useAuthStore((s) => s.verifyMFA)
  const changePassword = useAuthStore((s) => s.changePassword)
  const needsMFA = useAuthStore((s) => s.needsMFA)
  const passwordMustChange = useAuthStore((s) => s.passwordMustChange)

  const [view, setView] = useState<View>('login')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [mfaCode, setMfaCode] = useState('')
  const [forgotEmail, setForgotEmail] = useState('')
  const [currentPwd, setCurrentPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [confirmPwd, setConfirmPwd] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [forgotSent, setForgotSent] = useState(false)

  // MFA and changePassword take priority over manual view state.
  const activeView: View = needsMFA ? 'mfa' : passwordMustChange ? 'changePassword' : view

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!username || !password) return
    setLoading(true)
    setError('')
    try {
      await login(username, password)
      const state = useAuthStore.getState()
      if (state.isAuthenticated && !state.passwordMustChange) {
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
      const state = useAuthStore.getState()
      if (state.isAuthenticated && !state.passwordMustChange) {
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

  const handleForgotPassword = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!forgotEmail) return
    setLoading(true)
    setError('')
    try {
      await api.post('/auth/forgot-password', { email: forgotEmail })
      setForgotSent(true)
      addToast(t('login.resetSent', 'Password reset link sent to your email'), 'success')
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to send reset email'
      setError(msg)
      addToast(msg, 'error')
    } finally {
      setLoading(false)
    }
  }

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!currentPwd || !newPwd || !confirmPwd) return
    if (newPwd !== confirmPwd) {
      setError(t('login.passwordMismatch', 'Passwords do not match'))
      return
    }
    if (newPwd.length < 8) {
      setError(t('login.passwordTooShort', 'Password must be at least 8 characters'))
      return
    }
    setLoading(true)
    setError('')
    try {
      await changePassword(currentPwd, newPwd)
      addToast(t('login.passwordChanged', 'Password changed successfully'), 'success')
      navigate('/')
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to change password'
      setError(msg)
      addToast(msg, 'error')
    } finally {
      setLoading(false)
    }
  }

  const renderError = () =>
    error ? (
      <div className="rounded-md bg-[rgba(255,68,68,0.1)] border border-[rgba(255,68,68,0.2)] px-3 py-2 text-xs text-[var(--danger)] font-mono">
        &gt; {error}
      </div>
    ) : null

  const renderForm = () => {
    switch (activeView) {
      case 'mfa':
        return (
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
            {renderError()}
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
        )

      case 'forgot':
        return (
          <form onSubmit={handleForgotPassword} className="flex flex-col gap-4">
            <div className="text-center mb-2">
              <Mail size={24} className="inline-block text-[var(--accent)] mb-2" />
              <p className="text-sm text-[var(--text-secondary)] font-mono">
                {t('login.forgotPrompt', 'Enter your email to receive a reset link')}
              </p>
            </div>
            {forgotSent ? (
              <div className="rounded-md bg-[rgba(0,255,136,0.1)] border border-[rgba(0,255,136,0.2)] px-3 py-3 text-xs text-[var(--accent)] font-mono text-center">
                {t('login.resetSent', 'If the email exists, a reset link has been sent. Check your inbox.')}
              </div>
            ) : (
              <>
                <Input
                  type="email"
                  placeholder={t('login.emailPlaceholder', 'Email address')}
                  value={forgotEmail}
                  onChange={(e) => setForgotEmail(e.target.value)}
                  icon={<Mail size={16} />}
                  autoFocus
                  autoComplete="email"
                />
                {renderError()}
                <Button type="submit" disabled={loading || !forgotEmail} className="mt-2 w-full">
                  {loading ? (
                    <span className="cursor-blink font-mono">{t('common.loading')}</span>
                  ) : (
                    <>
                      <span className="font-mono">&gt;_</span>
                      {t('login.sendReset', 'Send Reset Link')}
                    </>
                  )}
                </Button>
              </>
            )}
            <button
              type="button"
              onClick={() => { setView('login'); setError(''); setForgotSent(false) }}
              className="text-xs text-[var(--text-muted)] hover:text-[var(--accent)] transition-colors font-mono cursor-pointer"
            >
              {t('login.backToLogin', 'Back to login')}
            </button>
          </form>
        )

      case 'changePassword':
        return (
          <form onSubmit={handleChangePassword} className="flex flex-col gap-4">
            <div className="text-center mb-2">
              <KeyRound size={24} className="inline-block text-[var(--warning)] mb-2" />
              <p className="text-sm text-[var(--text-secondary)] font-mono">
                {t('login.changePasswordPrompt', 'You must change your password before continuing')}
              </p>
            </div>
            <Input
              type="password"
              placeholder={t('login.currentPassword', 'Current password')}
              value={currentPwd}
              onChange={(e) => setCurrentPwd(e.target.value)}
              icon={<Lock size={16} />}
              autoFocus
              autoComplete="current-password"
            />
            <Input
              type="password"
              placeholder={t('login.newPassword', 'New password')}
              value={newPwd}
              onChange={(e) => setNewPwd(e.target.value)}
              icon={<KeyRound size={16} />}
              autoComplete="new-password"
            />
            <Input
              type="password"
              placeholder={t('login.confirmPassword', 'Confirm new password')}
              value={confirmPwd}
              onChange={(e) => setConfirmPwd(e.target.value)}
              icon={<KeyRound size={16} />}
              autoComplete="new-password"
            />
            {renderError()}
            <Button
              type="submit"
              disabled={loading || !currentPwd || !newPwd || !confirmPwd}
              className="mt-2 w-full"
            >
              {loading ? (
                <span className="cursor-blink font-mono">{t('common.loading')}</span>
              ) : (
                <>
                  <span className="font-mono">&gt;_</span>
                  {t('login.changePassword', 'Change Password')}
                </>
              )}
            </Button>
          </form>
        )

      default:
        return (
          <>
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
              {renderError()}
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
            <div className="mt-6 text-center">
              <button
                type="button"
                onClick={() => { setView('forgot'); setError('') }}
                className="text-xs text-[var(--text-muted)] hover:text-[var(--accent)] transition-colors font-mono cursor-pointer"
              >
                {t('login.forgotPassword')}
              </button>
            </div>
          </>
        )
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
            <div className="flex justify-center mb-4">
              <OutpostLogo size={72} />
            </div>
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
          </div>

          {renderForm()}

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
