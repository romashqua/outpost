import { parseJwtPayload } from '@/utils/jwt'

const BASE_URL = '/api/v1'

const TOKEN_REFRESH_THRESHOLD_S = 5 * 60 // 5 minutes

function getToken(): string | null {
  return localStorage.getItem('outpost-token')
}

/** Returns true if the token expires within the threshold. */
function isTokenExpiringSoon(token: string): boolean {
  const payload = parseJwtPayload(token)
  if (!payload || typeof payload.exp !== 'number') return false
  const nowSec = Math.floor(Date.now() / 1000)
  return payload.exp - nowSec < TOKEN_REFRESH_THRESHOLD_S
}

/** In-flight refresh promise to avoid concurrent refreshes. */
let refreshPromise: Promise<void> | null = null

interface RefreshResponse {
  token?: string
}

async function doRefresh(currentToken: string): Promise<void> {
  try {
    const response = await fetch(`${BASE_URL}/auth/refresh`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${currentToken}`,
      },
      body: JSON.stringify({ token: currentToken }),
    })
    if (!response.ok) return
    const data: RefreshResponse = await response.json()
    if (data.token) {
      localStorage.setItem('outpost-token', data.token)
      // Dispatch event so the auth store picks up the new token.
      window.dispatchEvent(new CustomEvent('auth:token-refreshed', { detail: data.token }))
    }
  } catch {
    // Refresh failed silently — the request will proceed with the current token.
  }
}

async function ensureFreshToken(): Promise<void> {
  const token = getToken()
  if (!token) return
  if (!isTokenExpiringSoon(token)) return

  if (!refreshPromise) {
    refreshPromise = doRefresh(token).finally(() => {
      refreshPromise = null
    })
  }
  await refreshPromise
}

/** Attempt token refresh and retry the original request. Returns true if retry succeeded. */
async function tryRefreshAndRetry<T>(
  path: string,
  options: RequestInit,
  resolve: (value: T) => void,
): Promise<boolean> {
  const token = getToken()
  if (!token) return false

  if (!refreshPromise) {
    refreshPromise = doRefresh(token).finally(() => {
      refreshPromise = null
    })
  }
  await refreshPromise

  // Check if we got a new token after refresh.
  const newToken = getToken()
  if (!newToken || newToken === token) return false

  // Retry the original request with the refreshed token.
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string> || {}),
  }
  if (options.body) {
    headers['Content-Type'] = 'application/json'
  }
  headers['Authorization'] = `Bearer ${newToken}`

  const retryResponse = await fetch(`${BASE_URL}${path}`, { ...options, headers })
  if (retryResponse.status === 401) return false

  if (!retryResponse.ok) {
    const error = await retryResponse.json().catch(() => ({ message: 'Request failed' }))
    throw new Error(error.message || error.error || `HTTP ${retryResponse.status}`)
  }

  if (retryResponse.status === 204) {
    resolve(undefined as T)
    return true
  }
  const text = await retryResponse.text()
  if (!text) {
    resolve(undefined as T)
    return true
  }
  resolve(JSON.parse(text))
  return true
}

async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  // Proactively refresh token if it is close to expiry.
  await ensureFreshToken()

  const token = getToken()
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string> || {}),
  }

  // Only set Content-Type for requests that carry a body.
  // GET and DELETE have no body, and some servers/proxies reject
  // Content-Type on bodyless requests.
  if (options.body) {
    headers['Content-Type'] = 'application/json'
  }

  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const response = await fetch(`${BASE_URL}${path}`, {
    ...options,
    headers,
  })

  if (response.status === 401) {
    // Don't nuke auth state on the login page — a 401 there just means
    // invalid credentials, and the caller displays the error itself.
    if (!window.location.pathname.startsWith('/login')) {
      // Try to refresh the token before giving up.
      const retried = await new Promise<T>((resolve, reject) => {
        tryRefreshAndRetry<T>(path, options, resolve)
          .then((success) => {
            if (!success) reject(new Error('__refresh_failed__'))
          })
          .catch(reject)
      }).catch((err) => {
        if (err.message === '__refresh_failed__') return null
        throw err
      })

      if (retried !== null) return retried as T

      localStorage.removeItem('outpost-token')
      localStorage.removeItem('outpost-user')
      // Dispatch event so the React auth store can update without circular imports.
      window.dispatchEvent(new Event('auth:logout'))
    }
    const body = await response.json().catch(() => ({ message: 'Not authorized' }))
    throw new Error(body.message || body.error || 'Not authorized')
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: 'Request failed' }))
    throw new Error(error.message || error.error || `HTTP ${response.status}`)
  }

  if (response.status === 204) {
    return undefined as T
  }

  // Some endpoints (e.g. addMember) return 201 with no body.
  // Use .text() first to avoid "Unexpected end of JSON input".
  const text = await response.text()
  if (!text) {
    return undefined as T
  }
  return JSON.parse(text)
}

/** requestText is like request but returns raw text instead of parsing JSON. */
async function requestText(path: string, options: RequestInit = {}): Promise<string> {
  await ensureFreshToken()

  const token = getToken()
  const headers: Record<string, string> = {
    ...(options.headers as Record<string, string> || {}),
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  const response = await fetch(`${BASE_URL}${path}`, { ...options, headers })
  if (response.status === 401) {
    if (!window.location.pathname.startsWith('/login')) {
      // Try to refresh and retry.
      const currentToken = getToken()
      if (currentToken) {
        if (!refreshPromise) {
          refreshPromise = doRefresh(currentToken).finally(() => {
            refreshPromise = null
          })
        }
        await refreshPromise
        const newToken = getToken()
        if (newToken && newToken !== currentToken) {
          const retryHeaders: Record<string, string> = {
            ...(options.headers as Record<string, string> || {}),
            'Authorization': `Bearer ${newToken}`,
          }
          const retryResponse = await fetch(`${BASE_URL}${path}`, { ...options, headers: retryHeaders })
          if (retryResponse.ok) {
            return retryResponse.text()
          }
        }
      }

      localStorage.removeItem('outpost-token')
      localStorage.removeItem('outpost-user')
      window.dispatchEvent(new Event('auth:logout'))
    }
    const body = await response.json().catch(() => ({ message: 'Not authorized' }))
    throw new Error(body.message || body.error || 'Not authorized')
  }
  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: 'Request failed' }))
    throw new Error(error.message || error.error || `HTTP ${response.status}`)
  }
  return response.text()
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  getText: (path: string) => requestText(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PATCH', body: body ? JSON.stringify(body) : undefined }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}
