import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { listUserTokens, createUserToken, revokeUserToken } from './userTokens'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('api/userTokens', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listUserTokens GETs /user-tokens', async () => {
    mockFetch().mockResolvedValue(new Response(JSON.stringify([]), { status: 200 }))
    const result = await listUserTokens()
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/user-tokens')
    expect(result).toEqual([])
  })

  it('createUserToken POSTs with name and expiresInDays', async () => {
    const created = {
      id: 'tok1', name: 'test', token: 'ainsel_abc',
      expiresAt: '2026-08-28T00:00:00Z', createdAt: '2026-05-28T00:00:00Z',
      lastUsedAt: null, revokedAt: null,
    }
    mockFetch().mockResolvedValue(new Response(JSON.stringify(created), { status: 201 }))
    const result = await createUserToken({ name: 'test', expiresInDays: 30 })
    const [url, opts] = mockFetch().mock.calls[0]
    expect(url).toContain('/user-tokens')
    expect(opts.method).toBe('POST')
    expect(JSON.parse(opts.body)).toEqual({ name: 'test', expiresInDays: 30 })
    expect(result.token).toBe('ainsel_abc')
  })

  it('revokeUserToken DELETEs /user-tokens/{id}', async () => {
    mockFetch().mockResolvedValue(new Response(null, { status: 204 }))
    await revokeUserToken('tok1')
    const [url, opts] = mockFetch().mock.calls[0]
    expect(url).toContain('/user-tokens/tok1')
    expect(opts.method).toBe('DELETE')
  })
})
