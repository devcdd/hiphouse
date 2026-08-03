import { API_BASE } from '@/shared/config'
import { clearToken, getRefreshToken, getToken, setTokens } from '@/shared/lib/token'

function authHeaders(): HeadersInit {
  const t = getToken()
  return t ? { Authorization: `Bearer ${t}` } : {}
}

async function handle<T>(res: Response): Promise<T> {
  if (!res.ok) {
    // 서버는 실패를 {"error": "..."}로 준다. 있으면 그 문장을 그대로 쓰고,
    // 없으면(프록시 오류, 빈 본문) 상태 코드로 떨어진다.
    const msg = await res
      .json()
      .then((d: unknown) => (d as { error?: string })?.error)
      .catch(() => undefined)
    throw new Error(msg || `${res.status} ${res.statusText}`)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

// Access tokens live an hour, so a 401 mid-session is expected. One refresh runs
// at a time — parallel requests that all 401 share the same attempt instead of
// rotating the refresh token N times and tripping the server's reuse detection.
let refreshing: Promise<boolean> | null = null

function refreshSession(): Promise<boolean> {
  refreshing ??= (async () => {
    try {
      const res = await fetch(API_BASE + '/auth/refresh', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ refresh_token: getRefreshToken() }),
      })
      if (!res.ok) {
        clearToken() // refresh token dead (expired, revoked, reused) → sign out
        return false
      }
      const data = (await res.json()) as { token: string; refresh_token: string }
      setTokens(data.token, data.refresh_token)
      return true
    } catch {
      return false // network blip: keep the tokens, let the caller see the error
    } finally {
      refreshing = null
    }
  })()
  return refreshing
}

// Single funnel for every call so the refresh-and-retry lives in one place.
async function request<T>(url: string | URL, init: RequestInit = {}, allowRetry = true): Promise<T> {
  const res = await fetch(url, { ...init, headers: { ...init.headers, ...authHeaders() } })
  if (res.status === 401 && allowRetry && getToken()) {
    if (getRefreshToken() && (await refreshSession())) return request<T>(url, init, false)
    clearToken() // no refresh token (pre-refresh-flow session) → drop the stale one
  }
  return handle<T>(res)
}

const jsonInit = (method: string, body?: unknown): RequestInit => ({
  method,
  headers: { 'Content-Type': 'application/json' },
  body: body === undefined ? undefined : JSON.stringify(body),
})

export function apiGet<T>(path: string, params?: Record<string, string | number | undefined>): Promise<T> {
  const url = new URL(API_BASE + path, window.location.origin)
  for (const [k, v] of Object.entries(params ?? {})) {
    if (v !== undefined && v !== '') url.searchParams.set(k, String(v))
  }
  return request<T>(url)
}

export const apiPost = <T,>(path: string, body?: unknown): Promise<T> =>
  request<T>(API_BASE + path, jsonInit('POST', body))

export const apiPut = <T,>(path: string, body?: unknown): Promise<T> =>
  request<T>(API_BASE + path, jsonInit('PUT', body))

export const apiDelete = <T,>(path: string): Promise<T> => request<T>(API_BASE + path, { method: 'DELETE' })
