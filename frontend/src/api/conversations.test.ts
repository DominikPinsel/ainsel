import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { listConversations } from './conversations'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('api/conversations', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists by invocation and unwraps messages', async () => {
    const messages = [
      {
        id: 1,
        invocationId: 'inv-1',
        agentName: 'ollie',
        role: 'user',
        content: '[]',
        createdAt: '2026-07-26T00:00:00Z',
      },
    ]
    mockFetch().mockResolvedValue(
      new Response(JSON.stringify({ messages, total: 1 }), { status: 200 }),
    )
    const result = await listConversations({ invocation: 'inv-1' })
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/observability/conversations')
    expect(url).toContain('invocation=inv-1')
    expect(result).toEqual({ messages, total: 1 })
  })

  it('returns [] when the envelope has no messages', async () => {
    mockFetch().mockResolvedValue(
      new Response(JSON.stringify({ total: 0 }), { status: 200 }),
    )
    const result = await listConversations({ invocation: 'inv-1' })
    expect(result).toEqual({ messages: [], total: 0 })
  })
})
