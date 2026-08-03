import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { listEvents } from './events'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('api/events', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listEvents GETs /events with limit', async () => {
    mockFetch().mockResolvedValue(
      new Response(JSON.stringify({ events: [], total: 0 }), { status: 200 }),
    )
    await listEvents({ limit: 5 })
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/events')
    expect(url).toContain('limit=5')
  })

  it('unwraps the events array from the envelope', async () => {
    const events = [
      {
        id: 'e1',
        timestamp: '2026-05-20T12:42:00Z',
        connector: 'insel-monorepo',
        status: 'matched',
      },
    ]
    mockFetch().mockResolvedValue(
      new Response(JSON.stringify({ events, total: 1 }), { status: 200 }),
    )
    const out = await listEvents({ limit: 5 })
    expect(out).toEqual(events)
  })
})
