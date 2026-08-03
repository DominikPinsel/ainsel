import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { PersonaList } from './PersonaList'
import { renderWithProviders } from '../../test/renderWithProviders'

describe('PersonaList', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/personas')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    id: '01HX1',
                    name: 'code-reviewer',
                    description: 'reviews PRs',
                    currentVersion: 3,
                    createdAt: '2026-05-01T00:00:00Z',
                    updatedAt: '2026-05-10T00:00:00Z',
                  },
                  {
                    id: '01HX2',
                    name: 'docs-helper',
                    description: 'helps with docs',
                    currentVersion: 1,
                    createdAt: '2026-05-05T00:00:00Z',
                    updatedAt: '2026-05-05T00:00:00Z',
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
                items: [],
                total: 0,
                page: 1,
                pageSize: 20,
                totalPages: 0,
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

  it('renders persona rows', async () => {
    renderWithProviders(<PersonaList />, { route: '/personas' })
    await waitFor(() => expect(screen.getByText('code-reviewer')).toBeInTheDocument())
    expect(screen.getByText('docs-helper')).toBeInTheDocument()
  })

  it('shows the New Persona header action', async () => {
    renderWithProviders(<PersonaList />, { route: '/personas' })
    expect(
      await screen.findByRole('button', { name: /new persona/i }),
    ).toBeInTheDocument()
  })

  it('shows empty state when no personas', async () => {
    vi.unstubAllGlobals()
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              items: [],
              total: 0,
              page: 1,
              pageSize: 20,
              totalPages: 0,
            }),
            { status: 200 },
          ),
        ),
      ),
    )
    renderWithProviders(<PersonaList />, { route: '/personas' })
    await waitFor(() => expect(screen.getByText(/no personas yet/i)).toBeInTheDocument())
  })
})
