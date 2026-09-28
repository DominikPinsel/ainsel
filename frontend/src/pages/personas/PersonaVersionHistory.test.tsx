import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PersonaVersionHistory } from './PersonaVersionHistory'
import { renderWithProviders } from '../../test/renderWithProviders'

const VERSIONS_BODY = JSON.stringify({
  items: [
    { personaId: '01HX1', versionNumber: 3, createdAt: '2026-05-03T00:00:00Z' },
    { personaId: '01HX1', versionNumber: 2, createdAt: '2026-05-02T00:00:00Z' },
    { personaId: '01HX1', versionNumber: 1, createdAt: '2026-05-01T00:00:00Z' },
  ],
  total: 3,
  page: 1,
  pageSize: 20,
  totalPages: 1,
})

function stubFetch(overrides: { versionsStatus?: number } = {}) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      const versionsStatus = overrides.versionsStatus ?? 200

      if (url.match(/\/api\/v1\/personas\/01HX1\/versions\/(\d+)$/) && method === 'GET') {
        const n = Number(url.match(/\/versions\/(\d+)$/)?.[1])
        return Promise.resolve(
          new Response(
            JSON.stringify({
              personaId: '01HX1',
              versionNumber: n,
              text: `# Revision ${n}\n\nHistorical instructions for v${n}.`,
              createdAt: '2026-05-01T00:00:00Z',
            }),
            { status: 200 },
          ),
        )
      }
      if (url.match(/\/api\/v1\/personas\/01HX1\/versions(\?|$)/) && method === 'GET') {
        if (versionsStatus === 403) {
          return Promise.resolve(
            new Response(JSON.stringify({ error: 'forbidden' }), { status: 403 }),
          )
        }
        return Promise.resolve(new Response(VERSIONS_BODY, { status: 200 }))
      }
      if (url.match(/\/api\/v1\/personas\/01HX1\/rollback$/) && method === 'POST') {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: '01HX1',
              name: 'code-reviewer',
              description: 'reviews PRs',
              currentVersion: 4,
              text: 'Historical instructions for v2.',
              createdAt: '2026-05-01T00:00:00Z',
              updatedAt: '2026-05-04T00:00:00Z',
            }),
            { status: 200 },
          ),
        )
      }
      return Promise.resolve(new Response('{}', { status: 200 }))
    }),
  )
}

function render(currentVersion = 3) {
  return renderWithProviders(
    <PersonaVersionHistory personaId="01HX1" currentVersion={currentVersion} />,
  )
}

describe('PersonaVersionHistory', () => {
  beforeEach(() => {
    stubFetch()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists every stored version, newest first', async () => {
    render()
    expect(await screen.findByText('v3')).toBeInTheDocument()
    expect(screen.getByText('v2')).toBeInTheDocument()
    expect(screen.getByText('v1')).toBeInTheDocument()

    const rows = screen.getAllByRole('row')
    // Header row first, then one row per version in server order.
    expect(rows).toHaveLength(4)
    expect(rows[1]).toHaveTextContent('v3')
    expect(rows[3]).toHaveTextContent('v1')
  })

  it('marks the live version as current', async () => {
    render(3)
    const current = await screen.findByText('current')
    expect(within(current.closest('tr') as HTMLElement).getByText('v3')).toBeInTheDocument()
    expect(screen.queryAllByText('current')).toHaveLength(1)
  })

  it('disables rollback for the version that is already current', async () => {
    render(3)
    const current = await screen.findByRole('button', { name: /roll back to version 3/i })
    expect(current).toBeDisabled()
    expect(screen.getByRole('button', { name: /roll back to version 2/i })).toBeEnabled()
  })

  it('fetches and renders an older version’s text on demand', async () => {
    render()
    await screen.findByText('v3')

    // Nothing is fetched for a single version until it is opened.
    expect(
      vi.mocked(fetch).mock.calls.some((c) => /\/versions\/\d+$/.test(String(c[0]))),
    ).toBe(false)

    await userEvent.click(screen.getByRole('button', { name: /view version 2/i }))

    expect(await screen.findByText(/historical instructions for v2/i)).toBeInTheDocument()
    expect(screen.getByText('Version 2')).toBeInTheDocument()
  })

  it('rolls back after confirmation and posts the chosen version', async () => {
    render()
    await screen.findByText('v3')

    await userEvent.click(screen.getByRole('button', { name: /roll back to version 2/i }))

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('Roll back to version 2?')

    await userEvent.click(within(dialog).getByRole('button', { name: /^roll back$/i }))

    await waitFor(() => {
      const call = vi
        .mocked(fetch)
        .mock.calls.find((c) => String(c[0]).includes('/rollback'))
      expect(call).toBeDefined()
      expect(JSON.parse(String(call?.[1]?.body))).toEqual({ toVersion: 2 })
    })
    // The modal closes once the rollback lands.
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('cancelling the rollback does not call the API', async () => {
    render()
    await screen.findByText('v3')

    await userEvent.click(screen.getByRole('button', { name: /roll back to version 1/i }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: /^cancel$/i }))

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(
      vi.mocked(fetch).mock.calls.some((c) => String(c[0]).includes('/rollback')),
    ).toBe(false)
  })

  it('reports denied access instead of an empty history', async () => {
    vi.unstubAllGlobals()
    stubFetch({ versionsStatus: 403 })
    render()

    expect(await screen.findByText('Access denied.')).toBeInTheDocument()
    expect(screen.queryByText('v3')).not.toBeInTheDocument()
    expect(screen.queryByText(/coming soon/i)).not.toBeInTheDocument()
  })
})
