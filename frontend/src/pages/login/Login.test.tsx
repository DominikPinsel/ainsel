import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AuthProvider } from '../../auth/AuthProvider'
import { Login } from './Login'

// The OIDC provider is mocked so oidc-mode tests don't construct a real
// UserManager; local/none modes don't touch it at all.
vi.mock('react-oidc-context', () => ({
  useAuth: () => ({
    user: null,
    isLoading: false,
    isAuthenticated: false,
    signinRedirect: vi.fn(),
    signoutRedirect: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

function seedConfig(cfg: Record<string, unknown>) {
  window.__AINSEL_CONFIG__ = cfg as never
}

function renderLogin() {
  return render(
    <MemoryRouter initialEntries={['/login']}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/dashboard" element={<div>dashboard reached</div>} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>,
  )
}

describe('Login', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('renders the OIDC sign-in button in oidc mode', () => {
    seedConfig({
      oidcIssuer: 'https://oidc.example.com',
      oidcClientId: 'client',
      oidcProjectId: 'project',
    })
    renderLogin()
    expect(screen.getByRole('button').textContent).toContain('Continue to login')
  })

  it('renders a username/password form in local mode and submits', async () => {
    seedConfig({ authMode: 'local' })
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          token: 'tok',
          expiresAt: '2030-01-01T00:00:00Z',
          user: { sub: 'local:admin', username: 'admin', isAdmin: true },
        }),
        { status: 200 },
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    // jsdom's location.replace is not spy-able; swap the whole object.
    const replaceMock = vi.fn()
    Object.defineProperty(window, 'location', {
      value: { ...window.location, replace: replaceMock },
      writable: true,
      configurable: true,
    })

    renderLogin()
    await userEvent.type(screen.getByLabelText('Username'), 'admin')
    await userEvent.type(screen.getByLabelText('Password'), 'secret123')
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }))

    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    expect(String(url)).toContain('/auth/login')
    expect(JSON.parse(init.body)).toEqual({ username: 'admin', password: 'secret123' })
    localStorage.clear()
  })

  it('shows the server error when local login fails', async () => {
    seedConfig({ authMode: 'local' })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ message: 'invalid credentials' }), { status: 401 }),
      ),
    )

    renderLogin()
    await userEvent.type(screen.getByLabelText('Username'), 'admin')
    await userEvent.type(screen.getByLabelText('Password'), 'wrong')
    await userEvent.click(screen.getByRole('button', { name: 'Sign in' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('invalid credentials')
    localStorage.clear()
  })

  it('redirects straight to the dashboard in none mode', () => {
    seedConfig({
      oidcIssuer: '',
      oidcClientId: '',
      oidcProjectId: '',
    })
    renderLogin()
    expect(screen.getByText('dashboard reached')).toBeTruthy()
  })
})
