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
}

interface AuthState {
  token: string | null
  user: User | null
  isAuthenticated: boolean
  needsMFA: boolean
  mfaToken: string | null
  login: (username: string, password: string) => Promise<void>
  verifyMFA: (code: string) => Promise<void>
  logout: () => void
  setAuth: (token: string, user: User) => void
}

function parseJwtPayload(token: string): Record<string, unknown> | null {
  try {
    const base64 = token.split('.')[1]
    const json = atob(base64.replace(/-/g, '+').replace(/_/g, '/'))
    return JSON.parse(json)
  } catch {
    return null
  }
}

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

export const useAuthStore = create<AuthState>((set) => ({
  token: localStorage.getItem('outpost-token'),
  user: (() => {
    const stored = localStorage.getItem('outpost-user')
    return stored ? JSON.parse(stored) : null
  })(),
  isAuthenticated: !!localStorage.getItem('outpost-token'),
  needsMFA: false,
  mfaToken: null,

  login: async (username: string, password: string) => {
    const res = await api.post<LoginResponse>('/auth/login', { username, password })

    if (res.mfa_required && res.mfa_token) {
      set({ needsMFA: true, mfaToken: res.mfa_token })
      return
    }

    if (!res.token) {
      throw new Error('No token received')
    }

    const user = applyToken(res.token, username)
    set({
      token: res.token,
      user,
      isAuthenticated: true,
      needsMFA: false,
      mfaToken: null,
    })
  },

  verifyMFA: async (code: string) => {
    const { mfaToken } = useAuthStore.getState()
    if (!mfaToken) throw new Error('No MFA session')

    const res = await api.post<LoginResponse>('/auth/mfa/verify', {
      mfa_token: mfaToken,
      code,
    })

    if (!res.token) {
      throw new Error('No token received')
    }

    const user = applyToken(res.token)
    set({
      token: res.token,
      user,
      isAuthenticated: true,
      needsMFA: false,
      mfaToken: null,
    })
  },

  logout: () => {
    localStorage.removeItem('outpost-token')
    localStorage.removeItem('outpost-user')
    set({
      token: null,
      user: null,
      isAuthenticated: false,
      needsMFA: false,
      mfaToken: null,
    })
  },

  setAuth: (token: string, user: User) => {
    localStorage.setItem('outpost-token', token)
    localStorage.setItem('outpost-user', JSON.stringify(user))
    set({ token, user, isAuthenticated: true })
  },
}))
