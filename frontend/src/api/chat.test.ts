import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  listChatSessions,
  getChatSession,
  createChatSession,
  updateChatSession,
  deleteChatSession,
  sendChatMessage,
} from './chat'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('api/chat', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        const method = init?.method ?? 'GET'

        if (url.match(/\/api\/v1\/chat\/sessions(\?|$)/) && method === 'GET') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    id: 's1',
                    name: 's1',
                    agentName: 'dev-agent',
                    userId: 'u1',
                    createdAt: '2026-06-24T00:00:00Z',
                    updatedAt: '2026-06-24T00:00:00Z',
                  },
                ],
                total: 1,
                page: 1,
                pageSize: 20,
                totalPages: 1,
              }),
              { status: 200 },
            ),
          )
        }

        if (url.match(/\/api\/v1\/chat\/sessions\/s1$/) && method === 'GET') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                id: 's1',
                name: 'My Chat',
                agentName: 'dev-agent',
                userId: 'u1',
                createdAt: '2026-06-24T00:00:00Z',
                updatedAt: '2026-06-24T00:00:00Z',
                messages: [],
              }),
              { status: 200 },
            ),
          )
        }

        if (url.match(/\/api\/v1\/chat\/sessions(\?|$)/) && method === 'POST') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                id: 's2',
                name: 's2',
                agentName: 'dev-agent',
                userId: 'u1',
                createdAt: '2026-06-25T00:00:00Z',
                updatedAt: '2026-06-25T00:00:00Z',
                messages: [],
              }),
              { status: 201 },
            ),
          )
        }

        if (url.match(/\/api\/v1\/chat\/sessions\/s1$/) && method === 'PATCH') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                id: 's1',
                name: 'Renamed Chat',
                agentName: 'dev-agent',
                userId: 'u1',
                createdAt: '2026-06-24T00:00:00Z',
                updatedAt: '2026-06-25T00:00:00Z',
                messages: [],
              }),
              { status: 200 },
            ),
          )
        }

        if (url.match(/\/api\/v1\/chat\/sessions\/s1$/) && method === 'DELETE') {
          return Promise.resolve(new Response(null, { status: 204 }))
        }

        if (url.match(/\/api\/v1\/chat\/sessions\/s1\/messages$/) && method === 'POST') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                id: 42,
                sessionId: 's1',
                role: 'user',
                content: 'Hello',
                tokens: 5,
                createdAt: '2026-06-25T00:00:00Z',
              }),
              { status: 201 },
            ),
          )
        }

        return Promise.resolve(new Response('{}', { status: 200 }))
      }),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listChatSessions GETs /chat/sessions with pagination params', async () => {
    const r = await listChatSessions({ page: 1, pageSize: 20 })
    const [url] = mockFetch().mock.calls[0]
    expect(url).toContain('/chat/sessions')
    expect(url).toContain('page=1')
    expect(url).toContain('pageSize=20')
    expect(r.items).toHaveLength(1)
    expect(r.items[0].name).toBe('s1')
    expect(r.total).toBe(1)
  })

  it('getChatSession GETs /chat/sessions/{id} and returns the session', async () => {
    const s = await getChatSession('s1')
    const [url, init] = mockFetch().mock.calls[0]
    expect(url).toContain('/chat/sessions/s1')
    expect((init as RequestInit).method ?? 'GET').toBe('GET')
    expect(s.id).toBe('s1')
    expect(s.name).toBe('My Chat')
    expect(s.messages).toEqual([])
  })

  it('createChatSession POSTs and returns the new session', async () => {
    const s = await createChatSession('dev-agent')
    const [url, init] = mockFetch().mock.calls[0]
    expect(url).toContain('/chat/sessions')
    expect((init as RequestInit).method).toBe('POST')
    expect(JSON.parse(init!.body as string)).toEqual({ agentName: 'dev-agent' })
    expect(s.id).toBe('s2')
    expect(s.name).toBe('s2')
  })

  it('updateChatSession PATCHes and returns the renamed session', async () => {
    const s = await updateChatSession('s1', 'Renamed Chat')
    const [url, init] = mockFetch().mock.calls[0]
    expect(url).toContain('/chat/sessions/s1')
    expect((init as RequestInit).method).toBe('PATCH')
    expect(JSON.parse(init!.body as string)).toEqual({ name: 'Renamed Chat' })
    expect(s.name).toBe('Renamed Chat')
  })

  it('deleteChatSession DELETEs and resolves', async () => {
    await expect(deleteChatSession('s1')).resolves.toBeUndefined()
    const [url, init] = mockFetch().mock.calls[0]
    expect(url).toContain('/chat/sessions/s1')
    expect((init as RequestInit).method).toBe('DELETE')
  })

  it('sendChatMessage POSTs to /chat/sessions/{id}/messages', async () => {
    const msg = await sendChatMessage('s1', 'Hello')
    const [url, init] = mockFetch().mock.calls[0]
    expect(url).toContain('/chat/sessions/s1/messages')
    expect((init as RequestInit).method).toBe('POST')
    expect(JSON.parse(init!.body as string)).toEqual({ role: 'user', content: 'Hello' })
    expect(msg.id).toBe(42)
    expect(msg.content).toBe('Hello')
  })
})