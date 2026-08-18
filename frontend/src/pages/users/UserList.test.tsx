import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { UserList } from './UserList'
import { renderWithProviders } from '../../test/renderWithProviders'

vi.mock('../../auth/AuthProvider', () => ({
  useAuth: () => ({
    mode: 'oidc',
    ready: true,
    token: 'tok',
    user: { sub: 'admin-user', username: 'admin' },
    isAdmin: true,
    login: vi.fn(),
    logout: vi.fn(),
    signinRedirect: vi.fn(),
    signoutRedirect: vi.fn(),
  }),
}))

const currentUser = {
  id: 'admin-user',
  username: 'admin',
  email: 'admin@example.com',
  isAdmin: true,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

const mockUsers = [
  currentUser,
  {
    id: '111',
    username: '111',
    email: 'alice@example.com',
    isAdmin: false,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
  },
  {
    id: '222',
    username: 'bob',
    email: 'bob@example.com',
    isAdmin: true,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
  },
]

const unsyncedUser = {
  id: '01J5XYZABCDEF0123456789AB',
  username: '',
  email: '',
  isAdmin: false,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

function defaultFetch(url: string): Response {
  if (url.includes('/users/admin-user') && !url.includes('/sync')) {
    return new Response(JSON.stringify(currentUser), { status: 200 })
  }
  if (url.includes('/users') && !url.includes('/sync')) {
    return new Response(JSON.stringify(mockUsers), { status: 200 })
  }
  return new Response('{}', { status: 200 })
}

describe('UserList', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => Promise.resolve(defaultFetch(url))),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders a Sync button for the own row and Clear cache buttons for other rows', async () => {
    renderWithProviders(<UserList />, { route: '/admin/users' })
    await waitFor(() => expect(screen.getByText('bob@example.com')).toBeInTheDocument())

    // Own row gets an active "Sync" button
    const syncButton = screen.getByRole('button', { name: /sync your profile/i })
    expect(syncButton).toBeInTheDocument()
    expect(syncButton).toHaveAttribute(
      'title',
      'Fetches your latest identity from the identity provider',
    )

    // Other rows get "Clear cache" buttons
    const clearButtons = screen.getAllByRole('button', { name: /clear cache for/i })
    expect(clearButtons).toHaveLength(2)
  })

  it('calls POST /users/me/sync when Sync is clicked on own row', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (url.includes('/users/me/sync') && init?.method === 'POST') {
        return Promise.resolve(new Response(JSON.stringify(currentUser), { status: 200 }))
      }
      return Promise.resolve(defaultFetch(url))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithProviders(<UserList />, { route: '/admin/users' })
    await waitFor(() => expect(screen.getByText('bob@example.com')).toBeInTheDocument())

    const syncButton = screen.getByRole('button', { name: /sync your profile/i })
    await userEvent.click(syncButton)

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(
        ([url, init]) => url.includes('/users/me/sync') && init?.method === 'POST',
      )
      expect(call).toBeDefined()
    })

    expect(screen.queryByText('Failed')).not.toBeInTheDocument()
  })

  it('calls POST /users/{id}/sync when Clear cache is clicked for another user', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      if (url.includes('/sync') && init?.method === 'POST') {
        const syncedUser = { ...mockUsers[1], username: '' }
        return Promise.resolve(new Response(JSON.stringify(syncedUser), { status: 200 }))
      }
      return Promise.resolve(defaultFetch(url))
    })
    vi.stubGlobal('fetch', fetchMock)

    renderWithProviders(<UserList />, { route: '/admin/users' })
    await waitFor(() => expect(screen.getByText('bob@example.com')).toBeInTheDocument())

    const clearButtons = screen.getAllByRole('button', { name: /clear cache for/i })
    await userEvent.click(clearButtons[0])

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(
        ([url, init]) => url.includes('/users/111/sync') && init?.method === 'POST',
      )
      expect(call).toBeDefined()
    })

    expect(screen.queryByText('Failed')).not.toBeInTheDocument()
  })

  it('renders "Not synced yet" placeholder for a user with no username and no email', async () => {
    const usersWithUnsynced = [...mockUsers, unsyncedUser]
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/users/admin-user') && !url.includes('/sync')) {
          return Promise.resolve(new Response(JSON.stringify(currentUser), { status: 200 }))
        }
        if (url.includes('/users') && !url.includes('/sync')) {
          return Promise.resolve(new Response(JSON.stringify(usersWithUnsynced), { status: 200 }))
        }
        return Promise.resolve(new Response('{}', { status: 200 }))
      }),
    )

    renderWithProviders(<UserList />, { route: '/admin/users' })
    await waitFor(() => expect(screen.getByText('Not synced yet')).toBeInTheDocument())

    // The raw ULID should NOT appear as visible text
    expect(screen.queryByText(unsyncedUser.id)).not.toBeInTheDocument()
  })

  it('shows an honest tooltip on the Clear cache button', async () => {
    renderWithProviders(<UserList />, { route: '/admin/users' })
    await waitFor(() => expect(screen.getByText('bob@example.com')).toBeInTheDocument())

    const clearButtons = screen.getAllByRole('button', { name: /clear cache for/i })
    expect(clearButtons[0]).toHaveAttribute(
      'title',
      "Clears cached data; name updates on the user's next login",
    )
  })
})
