const BASE_URL = '/api/v1'

function getToken(): string | null {
  return localStorage.getItem('outpost-token')
}

async function request<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = getToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string> || {}),
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

  return response.json()
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PATCH', body: body ? JSON.stringify(body) : undefined }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
}
