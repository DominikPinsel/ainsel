// API base URL. Defaults to `${BASE_URL}api/v1` so the app calls relative to
// its mount point — `/api/v1` at root, `/ainsel-dev/api/v1` when mounted at
// /ainsel-dev/. The ingress strips the prefix and forwards to hub-backend.
// import.meta.env.BASE_URL always ends in `/`, so plain concatenation
// produces the right shape.
// Override with VITE_API_URL=<absolute-url> at build time when needed.
const BASE_URL =
  (import.meta.env.VITE_API_URL as string | undefined) ??
  `${import.meta.env.BASE_URL}api/v1`

let authToken: string | null = null

export function setAuthToken(token: string | null) {
  authToken = token
}

export class ApiError extends Error {
  status: number
  body: unknown
  constructor(status: number, message: string, body: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.body = body
  }
}

export class UnauthorizedError extends ApiError {
  constructor(body: unknown) {
    super(401, 'Unauthorized', body)
    this.name = 'UnauthorizedError'
  }
}

export class ServiceUnavailableError extends ApiError {
  constructor(body: unknown) {
    super(503, 'Service Unavailable', body)
    this.name = 'ServiceUnavailableError'
  }
}

type Query = Record<string, string | number | boolean | undefined | null>

type RequestOptions = {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'
  body?: unknown
  query?: Query
  signal?: AbortSignal
}

function buildUrl(path: string, query?: Query): string {
  if (!query) return `${BASE_URL}${path}`
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null) continue
    params.set(k, String(v))
  }
  const qs = params.toString()
  return qs ? `${BASE_URL}${path}?${qs}` : `${BASE_URL}${path}`
}

function safeJsonParse(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

export async function request<T = unknown>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { method = 'GET', body, query, signal } = options
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (authToken) headers.Authorization = `Bearer ${authToken}`
  if (body !== undefined) headers['Content-Type'] = 'application/json'

  const res = await fetch(buildUrl(path, query), {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    signal,
  })

  if (res.status === 204) return undefined as T

  const text = await res.text()
  const parsed: unknown = text ? safeJsonParse(text) : undefined

  if (res.ok) return parsed as T

  const message = (() => {
    if (typeof parsed === 'object' && parsed !== null) {
      if ('message' in parsed) return String((parsed as { message: unknown }).message)
      if ('error' in parsed) return String((parsed as { error: unknown }).error)
    }
    return res.statusText || `HTTP ${res.status}`
  })()

  if (res.status === 401) {
    // Triggers RequireAuth → signinRedirect on next render.
    window.location.reload()
    throw new UnauthorizedError(parsed)
  }
  if (res.status === 503) throw new ServiceUnavailableError(parsed)
  throw new ApiError(res.status, message, parsed)
}
