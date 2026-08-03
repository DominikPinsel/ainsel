import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { TokenManager } from './TokenManager'
import { renderWithProviders } from '../../test/renderWithProviders'

function makeToken(
  id: string,
  name: string,
  createdAt: string,
  revokedAt: string | null = null,
) {
  return {
    id,
    name,
    createdAt,
    expiresAt: '2026-12-31T00:00:00Z',
    lastUsedAt: null,
    revokedAt,
  }
}

const tokensFixture = [
  makeToken('tok1', 'Alpha', '2026-06-05T10:00:00Z'),
  makeToken('tok2', 'Beta', '2026-06-04T10:00:00Z'),
  makeToken('tok3', 'Gamma', '2026-06-03T10:00:00Z'),
  makeToken('tok4', 'Delta', '2026-06-02T10:00:00Z', '2026-06-05T12:00:00Z'),
  makeToken('tok5', 'Epsilon', '2026-06-01T10:00:00Z'),
  makeToken('tok6', 'Zeta', '2026-05-31T10:00:00Z', '2026-06-05T12:00:00Z'),
]

function defaultFetch(url: string): Response {
  if (url.includes('/user-tokens')) {
    return new Response(JSON.stringify(tokensFixture), { status: 200 })
  }
  return new Response('{}', { status: 200 })
}

describe('TokenManager', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn((url: string) => Promise.resolve(defaultFetch(url))))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sorts active tokens before revoked tokens', async () => {
    renderWithProviders(<TokenManager />, { route: '/profile' })
    await waitFor(() => expect(screen.getByText('Alpha')).toBeInTheDocument())

    const rows = screen.getAllByRole('row').slice(1) // skip header
    const names = rows.map((r) => within(r).getAllByText(/./)[0].textContent)

    // Active first (newest first), then revoked (newest first)
    expect(names).toEqual(['Alpha', 'Beta', 'Gamma', 'Epsilon', 'Delta'])
  })

  it('paginates at 5 tokens per page by default', async () => {
    renderWithProviders(<TokenManager />, { route: '/profile' })
    await waitFor(() => expect(screen.getByText('Alpha')).toBeInTheDocument())

    // Page 1: 5 rows
    expect(screen.getAllByRole('row').length - 1).toBe(5)
    expect(screen.queryByText('Zeta')).not.toBeInTheDocument()

    // Navigate to page 2
    await userEvent.click(screen.getByRole('button', { name: 'Page 2' }))
    await waitFor(() => expect(screen.getByText('Zeta')).toBeInTheDocument())

    // Page 2: 1 row
    expect(screen.getAllByRole('row').length - 1).toBe(1)
  })

  it('resets to page 1 when page size changes', async () => {
    renderWithProviders(<TokenManager />, { route: '/profile' })
    await waitFor(() => expect(screen.getByText('Alpha')).toBeInTheDocument())

    await userEvent.click(screen.getByRole('button', { name: 'Page 2' }))
    await waitFor(() => expect(screen.getByText('Zeta')).toBeInTheDocument())

    await userEvent.selectOptions(screen.getByLabelText('Rows per page'), '10')
    await waitFor(() => expect(screen.getByText('Alpha')).toBeInTheDocument())
    expect(screen.getByText('Zeta')).toBeInTheDocument()
  })

  it('shows no pager when there are zero tokens', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/user-tokens')) {
          return Promise.resolve(new Response(JSON.stringify([]), { status: 200 }))
        }
        return Promise.resolve(new Response('{}', { status: 200 }))
      }),
    )
    renderWithProviders(<TokenManager />, { route: '/profile' })
    await waitFor(() => expect(screen.getByText('No tokens yet.')).toBeInTheDocument())
    expect(screen.queryByLabelText('Rows per page')).not.toBeInTheDocument()
  })

  it('only shows Revoke button on active tokens', async () => {
    renderWithProviders(<TokenManager />, { route: '/profile' })
    await waitFor(() => expect(screen.getByText('Alpha')).toBeInTheDocument())

    const rows = screen.getAllByRole('row').slice(1)
    for (const row of rows) {
      const name = within(row).getAllByText(/./)[0].textContent
      const revokeBtn = within(row).queryByRole('button', { name: 'Revoke' })
      if (name === 'Delta' || name === 'Zeta') {
        expect(revokeBtn).not.toBeInTheDocument()
      } else {
        expect(revokeBtn).toBeInTheDocument()
      }
    }
  })
})
