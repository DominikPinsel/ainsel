import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { listConnectors } from './connectors'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('api/connectors', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listConnectors GETs /connectors with pagination params', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({ items: [], total: 0, page: 1, pageSize: 200, totalPages: 0 }),
        { status: 200 },
      ),
    )
    await listConnectors({ pageSize: 200 })
    const [url, init] = mockFetch().mock.calls[0]
    expect(url).toContain('/connectors')
    expect(url).toContain('pageSize=200')
    expect((init as RequestInit).method ?? 'GET').toBe('GET')
  })

  it('listConnectors returns the paginated envelope verbatim', async () => {
    const payload = {
      items: [
        {
          id: 'c1',
          name: 'insel-monorepo',
          type: 'github',
          status: { ready: true, webhookRegistered: true, lastEventAt: '2026-05-20T12:40:00Z' },
        },
      ],
      total: 1,
      page: 1,
      pageSize: 200,
      totalPages: 1,
    }
    mockFetch().mockResolvedValue(new Response(JSON.stringify(payload), { status: 200 }))
    const out = await listConnectors({ pageSize: 200 })
    expect(out).toEqual(payload)
  })
})
