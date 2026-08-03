import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Profile } from './Profile'
import { renderWithProviders } from '../../test/renderWithProviders'

vi.mock('react-oidc-context', () => ({
  useAuth: () => ({
    user: { profile: { sub: 'user-123' } },
    isLoading: false,
  }),
}))

function defaultFetch(url: string): Response {
  if (url.includes('/users/user-123')) {
    return new Response(
      JSON.stringify({
        id: 'user-123',
        username: 'kim',
        email: 'kim@example.com',
        isAdmin: true,
        createdAt: '2026-05-22T00:00:00Z',
        updatedAt: '2026-05-22T00:00:00Z',
      }),
      { status: 200 },
    )
  }
  if (url.includes('/user-tokens')) {
    return new Response(JSON.stringify([]), { status: 200 })
  }
  return new Response('{}', { status: 200 })
}

describe('Profile', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => Promise.resolve(defaultFetch(url))),
    )
    vi.stubGlobal(
      'matchMedia',
      vi.fn((query: string) => ({
        matches: query === '(prefers-color-scheme: dark)',
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    )
    localStorage.clear()
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.clear()
  })

  it('renders the Account panel with user details', async () => {
    renderWithProviders(<Profile />, { route: '/profile' })
    await waitFor(() => expect(screen.getByText('kim')).toBeInTheDocument())
    expect(screen.getByText('kim@example.com')).toBeInTheDocument()
    expect(screen.getByText('Admin')).toBeInTheDocument()
  })

  it('renders the Appearance panel with a theme selector', async () => {
    renderWithProviders(<Profile />, { route: '/profile' })
    expect(screen.getByText('Appearance')).toBeInTheDocument()
    const select = screen.getByLabelText('Theme')
    expect(select).toBeInTheDocument()
    expect(select).toHaveValue('light')
  })

  it('stores the selected theme in localStorage', async () => {
    renderWithProviders(<Profile />, { route: '/profile' })
    const select = screen.getByLabelText('Theme')
    await userEvent.selectOptions(select, 'dark')
    await waitFor(() => {
      expect(localStorage.getItem('ainsel-theme')).toBe('dark')
    })
    expect(document.documentElement.dataset.theme).toBe('dark')
  })

  it('renders a Sync button in the Account panel', async () => {
    renderWithProviders(<Profile />, { route: '/profile' })
    await waitFor(() => expect(screen.getByText('kim')).toBeInTheDocument())
    expect(screen.getByRole('button', { name: /sync username/i })).toBeInTheDocument()
  })

  it('calls POST /users/me/sync when Sync is clicked', async () => {
    let syncCalled = false
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (url.includes('/users/me/sync') && init?.method === 'POST') {
        syncCalled = true
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: 'user-123',
              username: 'dpinsel',
              email: 'kim@example.com',
              isAdmin: true,
              createdAt: '2026-05-22T00:00:00Z',
              updatedAt: '2026-05-22T00:00:00Z',
            }),
            { status: 200 },
          ),
        )
      }
      if (url.includes('/users/user-123')) {
        if (syncCalled) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                id: 'user-123',
                username: 'dpinsel',
                email: 'kim@example.com',
                isAdmin: true,
                createdAt: '2026-05-22T00:00:00Z',
                updatedAt: '2026-05-22T00:00:00Z',
              }),
              { status: 200 },
            ),
          )
        }
      }
      return Promise.resolve(defaultFetch(url))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithProviders(<Profile />, { route: '/profile' })
    await waitFor(() => expect(screen.getByText('kim')).toBeInTheDocument())

    await userEvent.click(screen.getByRole('button', { name: /sync username/i }))

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(
        ([url, init]) => url.includes('/users/me/sync') && init?.method === 'POST',
      )
      expect(call).toBeDefined()
    })

    await waitFor(() => expect(screen.getByText('dpinsel')).toBeInTheDocument())
  })

  it('lists peat and tallow as theme options', async () => {
    renderWithProviders(<Profile />, { route: '/profile' })
    const select = screen.getByLabelText('Theme')
    const options = Array.from(select.querySelectorAll('option')).map(o => o.value)
    expect(options).toContain('peat')
    expect(options).toContain('tallow')
  })

  it('stores peat in localStorage when selected', async () => {
    renderWithProviders(<Profile />, { route: '/profile' })
    const select = screen.getByLabelText('Theme')
    await userEvent.selectOptions(select, 'peat')
    await waitFor(() => {
      expect(localStorage.getItem('ainsel-theme')).toBe('peat')
    })
    expect(document.documentElement.dataset.theme).toBe('peat')
  })

  it('stores tallow in localStorage when selected', async () => {
    renderWithProviders(<Profile />, { route: '/profile' })
    const select = screen.getByLabelText('Theme')
    await userEvent.selectOptions(select, 'tallow')
    await waitFor(() => {
      expect(localStorage.getItem('ainsel-theme')).toBe('tallow')
    })
    expect(document.documentElement.dataset.theme).toBe('tallow')
  })
})
