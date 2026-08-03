import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { listErrors } from './errors'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('api/errors', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listErrors GETs /errors with limit', async () => {
    mockFetch().mockResolvedValue(
      new Response(JSON.stringify({ errors: [], total: 0 }), { status: 200 }),
    )
    await listErrors({ limit: 200 })
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/errors')
    expect(url).toContain('limit=200')
  })

  it('unwraps the errors array from the envelope', async () => {
    const errors = [
      {
        id: 'er1',
        timestamp: '2026-05-20T12:38:00Z',
        severity: 'error',
        source: 'router',
        message: 'webhook 401',
      },
    ]
    mockFetch().mockResolvedValue(
      new Response(JSON.stringify({ errors, total: 1 }), { status: 200 }),
    )
    const out = await listErrors({ limit: 200 })
    expect(out).toEqual(errors)
  })
})
