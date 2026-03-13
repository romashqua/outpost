import { create } from 'zustand'

interface User {
  id: string
  username: string
  email: string
  role: string
}

interface AuthState {
  token: string | null
  user: User | null
  isAuthenticated: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
  setAuth: (token: string, user: User) => void
}

export const useAuthStore = create<AuthState>((set) => ({
  token: localStorage.getItem('outpost-token'),
  user: (() => {
    const stored = localStorage.getItem('outpost-user')
    return stored ? JSON.parse(stored) : null
  })(),
  isAuthenticated: !!localStorage.getItem('outpost-token'),

  login: async (username: string, _password: string) => {
    // In production, this calls the API. For now, simulate auth.
    const mockToken = 'outpost-session-' + Date.now()
    const mockUser: User = {
      id: '1',
      username,
      email: `${username}@outpost.local`,
      role: 'admin',
    }
    localStorage.setItem('outpost-token', mockToken)
    localStorage.setItem('outpost-user', JSON.stringify(mockUser))
    set({ token: mockToken, user: mockUser, isAuthenticated: true })
  },

  logout: () => {
    localStorage.removeItem('outpost-token')
    localStorage.removeItem('outpost-user')
    set({ token: null, user: null, isAuthenticated: false })
  },

  setAuth: (token: string, user: User) => {
    localStorage.setItem('outpost-token', token)
    localStorage.setItem('outpost-user', JSON.stringify(user))
    set({ token, user, isAuthenticated: true })
  },
}))
