import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  listPersonas,
  getPersona,
  createPersona,
  updatePersona,
  deletePersona,
} from './personas'

describe('personas API', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        const method = init?.method ?? 'GET'

        if (url.match(/\/api\/v1\/personas(\?|$)/) && method === 'GET') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    id: '01HX1',
                    name: 'code-reviewer',
                    description: 'reviews PRs',
                    currentVersion: 1,
                    createdAt: '2026-05-01T00:00:00Z',
                    updatedAt: '2026-05-01T00:00:00Z',
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
        if (url.match(/\/api\/v1\/personas\/01HX1$/) && method === 'GET') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                id: '01HX1',
                name: 'code-reviewer',
                description: 'reviews PRs',
                currentVersion: 2,
                text: 'You are a thorough code reviewer.',
                createdAt: '2026-05-01T00:00:00Z',
                updatedAt: '2026-05-02T00:00:00Z',
              }),
              { status: 200 },
            ),
          )
        }
        if (url.match(/\/api\/v1\/personas(\?|$)/) && method === 'POST') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                id: '01HXNEW',
                name: 'docs-helper',
                description: '',
                currentVersion: 1,
                text: 'Help with docs.',
                createdAt: '2026-05-21T00:00:00Z',
                updatedAt: '2026-05-21T00:00:00Z',
              }),
              { status: 201 },
            ),
          )
        }
        if (url.match(/\/api\/v1\/personas\/01HX1$/) && method === 'PUT') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                id: '01HX1',
                name: 'code-reviewer',
                description: 'updated',
                currentVersion: 3,
                text: 'You are an even more thorough code reviewer.',
                createdAt: '2026-05-01T00:00:00Z',
                updatedAt: '2026-05-21T00:00:00Z',
              }),
              { status: 200 },
            ),
          )
        }
        if (url.match(/\/api\/v1\/personas\/01HX1$/) && method === 'DELETE') {
          return Promise.resolve(new Response(null, { status: 204 }))
        }
        return Promise.resolve(new Response('{}', { status: 200 }))
      }),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('listPersonas returns paginated summaries', async () => {
    const r = await listPersonas()
    expect(r.items).toHaveLength(1)
    expect(r.items[0].name).toBe('code-reviewer')
    expect(r.total).toBe(1)
  })

  it('getPersona returns text alongside metadata', async () => {
    const p = await getPersona('01HX1')
    expect(p.text).toContain('thorough code reviewer')
    expect(p.currentVersion).toBe(2)
  })

  it('createPersona POSTs and returns the new record', async () => {
    const p = await createPersona({ name: 'docs-helper', text: 'Help with docs.' })
    expect(p.id).toBe('01HXNEW')
  })

  it('updatePersona PUTs and returns the updated record', async () => {
    const p = await updatePersona('01HX1', {
      description: 'updated',
      text: 'You are an even more thorough code reviewer.',
    })
    expect(p.currentVersion).toBe(3)
  })

  it('deletePersona DELETEs and resolves', async () => {
    await expect(deletePersona('01HX1')).resolves.toBeUndefined()
  })
})
