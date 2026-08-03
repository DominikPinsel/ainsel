import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { listTriggers } from './triggers'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('api/triggers', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listTriggers GETs /triggers with pagination params', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({ items: [], total: 23, page: 1, pageSize: 1, totalPages: 23 }),
        { status: 200 },
      ),
    )
    const out = await listTriggers({ pageSize: 1 })
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/triggers')
    expect(out.total).toBe(23)
  })
})
