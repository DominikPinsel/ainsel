import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './client'
import {
  listPersonas,
  getPersona,
  createPersona,
  updatePersona,
  deletePersona,
  listPersonaVersions,
  getPersonaVersion,
  rollbackPersona,
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
        if (url.match(/\/api\/v1\/personas\/01HX1\/versions(\?|$)/) && method === 'GET') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    personaId: '01HX1',
                    versionNumber: 2,
                    createdAt: '2026-05-02T00:00:00Z',
                  },
                  {
                    personaId: '01HX1',
                    versionNumber: 1,
                    createdAt: '2026-05-01T00:00:00Z',
                  },
                ],
                total: 2,
                page: 1,
                pageSize: 20,
                totalPages: 1,
              }),
              { status: 200 },
            ),
          )
        }
        if (url.match(/\/api\/v1\/personas\/01HX1\/versions\/2$/) && method === 'GET') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                personaId: '01HX1',
                versionNumber: 2,
                text: 'You are a thorough code reviewer.',
                createdAt: '2026-05-02T00:00:00Z',
              }),
              { status: 200 },
            ),
          )
        }
        if (url.match(/\/api\/v1\/personas\/01HX1\/rollback$/) && method === 'POST') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                id: '01HX1',
                name: 'code-reviewer',
                description: 'reviews PRs',
                currentVersion: 3,
                text: 'You are a thorough code reviewer.',
                createdAt: '2026-05-01T00:00:00Z',
                updatedAt: '2026-05-03T00:00:00Z',
              }),
              { status: 200 },
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

  it('listPersonaVersions returns version metadata without text', async () => {
    const r = await listPersonaVersions('01HX1')
    expect(r.total).toBe(2)
    expect(r.items.map((v) => v.versionNumber)).toEqual([2, 1])
    expect(r.items[0]).not.toHaveProperty('text')
  })

  it('listPersonaVersions forwards pagination params', async () => {
    await listPersonaVersions('01HX1', { page: 2, pageSize: 10 })
    const url = vi
      .mocked(fetch)
      .mock.calls.map((c) => String(c[0]))
      .find((u) => u.includes('/versions'))
    expect(url).toContain('page=2')
    expect(url).toContain('pageSize=10')
  })

  it('getPersonaVersion returns the stored text', async () => {
    const v = await getPersonaVersion('01HX1', 2)
    expect(v.versionNumber).toBe(2)
    expect(v.text).toContain('thorough code reviewer')
  })

  it('rollbackPersona POSTs the target version and returns the new current', async () => {
    const p = await rollbackPersona('01HX1', 2)
    expect(p.currentVersion).toBe(3)

    const call = vi
      .mocked(fetch)
      .mock.calls.find((c) => String(c[0]).includes('/rollback'))
    expect(call?.[1]?.method).toBe('POST')
    expect(JSON.parse(String(call?.[1]?.body))).toEqual({ toVersion: 2 })
  })
})

describe('personas API — version access control', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  // The hub gates the version endpoints on the persona's read/write access,
  // so a 403 is a normal outcome the UI has to render rather than a bug.
  it.each([
    ['listPersonaVersions', () => listPersonaVersions('01HX1')],
    ['getPersonaVersion', () => getPersonaVersion('01HX1', 2)],
    ['rollbackPersona', () => rollbackPersona('01HX1', 1)],
  ])('%s surfaces 403 as an ApiError', async (_name, call) => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ error: 'forbidden' }), { status: 403 }),
        ),
      ),
    )
    await expect(call()).rejects.toMatchObject({
      name: 'ApiError',
      status: 403,
    })
    await expect(call()).rejects.toBeInstanceOf(ApiError)
  })
})
