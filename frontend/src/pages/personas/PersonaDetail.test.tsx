import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { PersonaDetail } from './PersonaDetail'
import { renderWithProviders } from '../../test/renderWithProviders'

describe('PersonaDetail', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.match(/\/api\/v1\/personas\/01HX1$/)) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                id: '01HX1',
                name: 'code-reviewer',
                description: 'reviews PRs',
                currentVersion: 2,
                text: '# Persona\n\nYou are a thorough code reviewer.',
                createdAt: '2026-05-01T00:00:00Z',
                updatedAt: '2026-05-02T00:00:00Z',
              }),
              { status: 200 },
            ),
          )
        }
        if (url.match(/\/api\/v1\/personas\/01HX1\/versions(\?|$)/)) {
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
        if (url.includes('/agents')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    id: 'a1',
                    name: 'code-reviewer-agent',
                    persona: { id: '01HX1' },
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
        return Promise.resolve(new Response('{}', { status: 200 }))
      }),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders persona identity and markdown', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/personas/:id" element={<PersonaDetail />} />
      </Routes>,
      { route: '/personas/01HX1' },
    )
    await waitFor(() =>
      expect(screen.getAllByText('code-reviewer')[0]).toBeInTheDocument(),
    )
    expect(screen.getByText(/thorough code reviewer/i)).toBeInTheDocument()
  })

  it('lists referring agents', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/personas/:id" element={<PersonaDetail />} />
      </Routes>,
      { route: '/personas/01HX1' },
    )
    await waitFor(() =>
      expect(screen.getByText('code-reviewer-agent')).toBeInTheDocument(),
    )
  })

  it('shows the Edit action', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/personas/:id" element={<PersonaDetail />} />
      </Routes>,
      { route: '/personas/01HX1' },
    )
    expect(await screen.findByRole('button', { name: /^edit$/i })).toBeInTheDocument()
  })

  it('renders the real version history on the History tab', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/personas/:id" element={<PersonaDetail />} />
      </Routes>,
      { route: '/personas/01HX1' },
    )
    await waitFor(() =>
      expect(screen.getAllByText('code-reviewer')[0]).toBeInTheDocument(),
    )

    // The History tab replaces the old "coming soon" placeholder.
    expect(screen.queryByText(/coming soon/i)).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: /^history$/i }))

    expect(await screen.findByText('v2')).toBeInTheDocument()
    expect(screen.getByText('v1')).toBeInTheDocument()
    expect(screen.getByText('current')).toBeInTheDocument()
    expect(screen.queryByText(/coming soon/i)).not.toBeInTheDocument()
  })

  it('does not fetch version history until the History tab is opened', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/personas/:id" element={<PersonaDetail />} />
      </Routes>,
      { route: '/personas/01HX1' },
    )
    await waitFor(() =>
      expect(screen.getAllByText('code-reviewer')[0]).toBeInTheDocument(),
    )
    expect(
      vi.mocked(fetch).mock.calls.some((c) => String(c[0]).includes('/versions')),
    ).toBe(false)

    await userEvent.click(screen.getByRole('tab', { name: /^history$/i }))
    await screen.findByText('v2')
    expect(
      vi.mocked(fetch).mock.calls.some((c) => String(c[0]).includes('/versions')),
    ).toBe(true)
  })
})
