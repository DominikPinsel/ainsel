import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { listAgents } from './agents'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('api/agents', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listAgents GETs /agents with pagination params', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({ items: [], total: 14, page: 1, pageSize: 1, totalPages: 14 }),
        { status: 200 },
      ),
    )
    const out = await listAgents({ pageSize: 1 })
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/agents')
    expect(url).toContain('pageSize=1')
    expect(out.total).toBe(14)
  })
})
