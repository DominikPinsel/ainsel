import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError,
  ServiceUnavailableError,
  UnauthorizedError,
  request,
  setAuthToken,
} from './client'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('api/client', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
    setAuthToken(null)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('parses a 200 JSON response', async () => {
    mockFetch().mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }))
    const out = await request<{ ok: boolean }>('/foo')
    expect(out).toEqual({ ok: true })
  })

  it('attaches Authorization header when token set', async () => {
    setAuthToken('tok-123')
    mockFetch().mockResolvedValue(new Response('{}', { status: 200 }))
    await request('/foo')
    const init = mockFetch().mock.calls[0][1] as RequestInit
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer tok-123')
  })

  it('omits Authorization when token is null', async () => {
    mockFetch().mockResolvedValue(new Response('{}', { status: 200 }))
    await request('/foo')
    const init = mockFetch().mock.calls[0][1] as RequestInit
    expect((init.headers as Record<string, string>).Authorization).toBeUndefined()
  })

  it('serializes query params', async () => {
    mockFetch().mockResolvedValue(new Response('{}', { status: 200 }))
    await request('/foo', { query: { page: 2, pageSize: 20, agent: 'a' } })
    const url = mockFetch().mock.calls[0][0] as string
    expect(url).toContain('/foo?')
    expect(url).toContain('page=2')
    expect(url).toContain('pageSize=20')
    expect(url).toContain('agent=a')
  })

  it('skips undefined and null query values', async () => {
    mockFetch().mockResolvedValue(new Response('{}', { status: 200 }))
    await request('/foo', { query: { page: 1, agent: undefined, connector: null } })
    const url = mockFetch().mock.calls[0][0] as string
    expect(url).toContain('page=1')
    expect(url).not.toContain('agent=')
    expect(url).not.toContain('connector=')
  })

  it('throws UnauthorizedError on 401', async () => {
    mockFetch().mockResolvedValue(new Response('', { status: 401 }))
    await expect(request('/foo')).rejects.toBeInstanceOf(UnauthorizedError)
  })

  it('reloads the page on 401 so RequireAuth re-redirects to the IdP', async () => {
    mockFetch().mockResolvedValue(new Response('', { status: 401 }))
    const reload = vi.fn()
    // jsdom's `window.location` is non-configurable and its `reload` getter
    // can't be spied on. Replace the whole `location` object for this test.
    const originalLocation = window.location
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { ...originalLocation, reload },
    })
    try {
      await expect(request('/foo')).rejects.toBeInstanceOf(UnauthorizedError)
      expect(reload).toHaveBeenCalledTimes(1)
    } finally {
      Object.defineProperty(window, 'location', {
        configurable: true,
        value: originalLocation,
      })
    }
  })

  it('throws ServiceUnavailableError on 503', async () => {
    mockFetch().mockResolvedValue(new Response('', { status: 503 }))
    await expect(request('/foo')).rejects.toBeInstanceOf(ServiceUnavailableError)
  })

  it('throws ApiError on other 4xx/5xx with message from body', async () => {
    mockFetch().mockResolvedValue(
      new Response(JSON.stringify({ message: 'nope' }), { status: 400 }),
    )
    await expect(request('/foo')).rejects.toMatchObject({
      name: 'ApiError',
      status: 400,
      message: 'nope',
    })
  })

  it('extracts message from error key when message key is absent', async () => {
    mockFetch().mockResolvedValue(
      new Response(JSON.stringify({ error: 'something went wrong' }), { status: 500 }),
    )
    await expect(request('/foo')).rejects.toMatchObject({
      name: 'ApiError',
      status: 500,
      message: 'something went wrong',
    })
  })

  it('falls back to HTTP status code when statusText is empty and body has no message', async () => {
    // Response constructor defaults statusText to "" when not provided — simulates HTTP/2
    mockFetch().mockResolvedValue(new Response('{}', { status: 500 }))
    await expect(request('/foo')).rejects.toMatchObject({
      name: 'ApiError',
      status: 500,
      message: 'HTTP 500',
    })
  })

  it('sends JSON body and content-type for POST with body', async () => {
    mockFetch().mockResolvedValue(new Response('{}', { status: 200 }))
    await request('/foo', { method: 'POST', body: { x: 1 } })
    const init = mockFetch().mock.calls[0][1] as RequestInit
    expect(init.method).toBe('POST')
    expect((init.headers as Record<string, string>)['Content-Type']).toBe('application/json')
    expect(init.body).toBe(JSON.stringify({ x: 1 }))
  })

  it('returns undefined for 204 No Content', async () => {
    mockFetch().mockResolvedValue(new Response(null, { status: 204 }))
    const out = await request('/foo')
    expect(out).toBeUndefined()
  })

  it('UnauthorizedError is a subclass of ApiError with status 401', async () => {
    mockFetch().mockResolvedValue(new Response('', { status: 401 }))
    try {
      await request('/foo')
      expect.fail('should have thrown')
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError)
      expect((err as ApiError).status).toBe(401)
    }
  })
})
