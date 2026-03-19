import { create } from 'zustand'
import { api } from '@/api/client'

interface User {
  id: string
  username: string
  email: string
  role: string
}

interface LoginResponse {
  token?: string
  expires_at?: string
  mfa_required?: boolean
  mfa_token?: string
  password_must_change?: boolean
}

interface AuthState {
  token: string | null
  user: User | null
  isAuthenticated: boolean
  isInitialized: boolean
  needsMFA: boolean
  mfaToken: string | null
  passwordMustChange: boolean
  login: (username: string, password: string) => Promise<void>
  verifyMFA: (code: string, method?: string) => Promise<void>
  changePassword: (currentPassword: string, newPassword: string) => Promise<void>
  clearPasswordMustChange: () => void
  logout: () => void
  setAuth: (token: string, user: User) => void
  refreshToken: () => Promise<void>
  initialize: () => void
}

import { parseJwtPayload } from '@/utils/jwt'

function userFromToken(token: string, fallbackUsername?: string): User {
  const payload = parseJwtPayload(token)
  return {
    id: (payload?.user_id as string) ?? (payload?.sub as string) ?? '',
    username: (payload?.username as string) ?? fallbackUsername ?? '',
    email: (payload?.email as string) ?? '',
    role: (payload?.is_admin ? 'admin' : (payload?.role as string)) ?? 'user',
  }
}

function applyToken(token: string, username?: string) {
  const user = userFromToken(token, username)
  localStorage.setItem('outpost-token', token)
  localStorage.setItem('outpost-user', JSON.stringify(user))
  return user
}

// NOTE: Token is stored in localStorage which is accessible to any JS running
// on the same origin. This is a known XSS risk inherent to SPA architectures.
// Mitigations: strict CSP headers, input sanitization, and subresource integrity.
// Moving to httpOnly cookies would eliminate this vector but complicates the
// cross-origin / multi-tab auth flow. Accepted tradeoff for now.
export const useAuthStore = create<AuthState>((set) => ({
  token: localStorage.getItem('outpost-token'),
  user: (() => {
    const stored = localStorage.getItem('outpost-user')
    return stored ? JSON.parse(stored) : null
  })(),
  isAuthenticated: !!localStorage.getItem('outpost-token'),
  isInitialized: false,
  needsMFA: false,
  mfaToken: sessionStorage.getItem('outpost-mfa-token'),
  passwordMustChange: false,

  initialize: () => {
    const token = localStorage.getItem('outpost-token')
    const storedUser = localStorage.getItem('outpost-user')
    const mfaToken = sessionStorage.getItem('outpost-mfa-token')
    set({
      token,
      user: storedUser ? JSON.parse(storedUser) : null,
      isAuthenticated: !!token,
      isInitialized: true,
      needsMFA: !!mfaToken,
      mfaToken,
    })
  },

  login: async (username: string, password: string) => {
    const res = await api.post<LoginResponse>('/auth/login', { username, password })

    if (res.mfa_required && res.mfa_token) {
      sessionStorage.setItem('outpost-mfa-token', res.mfa_token)
      set({ needsMFA: true, mfaToken: res.mfa_token })
      return
    }

    if (!res.token) {
      throw new Error('No token received')
    }

    const user = applyToken(res.token, username)
    sessionStorage.removeItem('outpost-mfa-token')
    set({
      token: res.token,
      user,
      isAuthenticated: true,
      needsMFA: false,
      mfaToken: null,
      passwordMustChange: !!res.password_must_change,
    })
  },

  verifyMFA: async (code: string, method: string = 'totp') => {
    const { mfaToken } = useAuthStore.getState()
    if (!mfaToken) throw new Error('No MFA session')

    const res = await api.post<LoginResponse>('/auth/mfa/verify', {
      mfa_token: mfaToken,
      code,
      method,
    })

    if (!res.token) {
      throw new Error('No token received')
    }

    const user = applyToken(res.token)
    sessionStorage.removeItem('outpost-mfa-token')
    set({
      token: res.token,
      user,
      isAuthenticated: true,
      needsMFA: false,
      mfaToken: null,
      passwordMustChange: !!res.password_must_change,
    })
  },

  changePassword: async (currentPassword: string, newPassword: string) => {
    await api.post('/auth/change-password', {
      current_password: currentPassword,
      new_password: newPassword,
    })
    set({ passwordMustChange: false })
  },

  clearPasswordMustChange: () => {
    set({ passwordMustChange: false })
  },

  logout: () => {
    api.post('/auth/logout').catch(() => {
      // Best-effort: if the API call fails (network error, expired token, etc.),
      // we still clear local state so the user is logged out client-side.
    })
    localStorage.removeItem('outpost-token')
    localStorage.removeItem('outpost-user')
    sessionStorage.removeItem('outpost-mfa-token')
    set({
      token: null,
      user: null,
      isAuthenticated: false,
      needsMFA: false,
      mfaToken: null,
      passwordMustChange: false,
    })
  },

  refreshToken: async () => {
    const { token } = useAuthStore.getState()
    if (!token) return

    try {
      const res = await api.post<LoginResponse>('/auth/refresh', { token })
      if (res.token) {
        const user = applyToken(res.token)
        set({ token: res.token, user, isAuthenticated: true })
      }
    } catch {
      // Refresh failed — force logout.
      useAuthStore.getState().logout()
    }
  },

  setAuth: (token: string, user: User) => {
    localStorage.setItem('outpost-token', token)
    localStorage.setItem('outpost-user', JSON.stringify(user))
    set({ token, user, isAuthenticated: true })
  },
}))

// Initialize on load.
useAuthStore.getState().initialize()

// Listen for 401 events dispatched by the API client (avoids circular imports).
window.addEventListener('auth:logout', () => {
  useAuthStore.getState().logout()
})
