// Local-auth API calls. These intentionally use raw fetch instead of
// request() from ./client: the shared client reloads the page on any 401
// (OIDC session recovery), but for login and password change a 401 is a
// normal business outcome that must reach the calling form.

const BASE_URL =
  (import.meta.env.VITE_API_URL as string | undefined) ??
  `${import.meta.env.BASE_URL}api/v1`

export type LocalUser = {
  sub: string
  username: string
  isAdmin: boolean
}

export type LoginResponse = {
  token: string
  expiresAt: string
  user: LocalUser
}

async function rawRequest<T>(path: string, init: RequestInit, token?: string | null): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    'Content-Type': 'application/json',
  }
  if (token) headers.Authorization = `Bearer ${token}`
  const res = await fetch(`${BASE_URL}${path}`, { ...init, headers })
  const text = await res.text()
  let parsed: unknown = undefined
  try {
    parsed = text ? JSON.parse(text) : undefined
  } catch {
    parsed = text
  }
  if (!res.ok) {
    const message =
      typeof parsed === 'object' && parsed !== null && 'message' in parsed
        ? String((parsed as { message: unknown }).message)
        : res.statusText || `HTTP ${res.status}`
    throw new Error(message)
  }
  return parsed as T
}

export function login(username: string, password: string): Promise<LoginResponse> {
  return rawRequest<LoginResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export function logout(token: string | null): Promise<void> {
  // Stateless hub: best-effort notification only; the client discards the
  // token regardless of the outcome.
  return rawRequest<void>('/auth/logout', { method: 'POST' }, token).catch(() => undefined)
}

export function changePassword(token: string, current: string, next: string): Promise<void> {
  return rawRequest<{ status: string }>(
    '/auth/password',
    { method: 'PUT', body: JSON.stringify({ current, new: next }) },
    token,
  ).then(() => undefined)
}
