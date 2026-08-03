import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { listInvocations } from './invocations'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('api/invocations', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists with status + pageSize', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({ invocations: [], total: 0, capacity: 1000, page: 1, pageSize: 100, totalPages: 0 }),
        { status: 200 },
      ),
    )
    await listInvocations({ status: 'running', pageSize: 100 })
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/invocations')
    expect(url).toContain('status=running')
    expect(url).toContain('pageSize=100')
  })

  it('sends event filter param', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({ invocations: [], total: 0, capacity: 1000, page: 1, pageSize: 100, totalPages: 0 }),
        { status: 200 },
      ),
    )
    await listInvocations({ event: 'evt-1' })
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/invocations')
    expect(url).toContain('event=evt-1')
  })

  it('passes since param verbatim', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({ invocations: [], total: 0, capacity: 1000, page: 1, pageSize: 200, totalPages: 0 }),
        { status: 200 },
      ),
    )
    await listInvocations({ since: '2026-05-20T12:00:00Z' })
    const url = mockFetch().mock.calls[0][0] as string
    expect(decodeURIComponent(url)).toContain('since=2026-05-20T12:00:00Z')
  })
})
