import { act, render, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'
import { AuthProvider, useAuth } from './AuthProvider'
import { UnauthorizedError, request, setUnauthorizedHandler } from '../api/client'
import type { ResolvedConfig } from '../runtime-config'

// react-oidc-context is mocked so OidcBridge can be driven with a precise
// library state (expired user, refresh token present, navigator active…)
// without standing up a real UserManager against an IdP. The shape mirrors
// the subset of the library's useAuth() state that OidcBridge reads.
type MockUser = {
  access_token: string
  expired: boolean | undefined
  refresh_token?: string
  profile: { sub?: unknown; preferred_username?: string; email?: string }
}

let mockOidc: {
  user: MockUser | null
  isLoading: boolean
  activeNavigator?: string
  signinSilent: ReturnType<typeof vi.fn>
  signinRedirect: ReturnType<typeof vi.fn>
  signoutRedirect: ReturnType<typeof vi.fn>
}

vi.mock('react-oidc-context', () => ({
  AuthProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
  useAuth: () => mockOidc,
}))

const OIDC_CFG: ResolvedConfig = {
  authMode: 'oidc',
  oidcIssuer: 'https://oidc.example.com',
  oidcClientId: 'test-client',
  oidcProjectId: 'test-project',
}

function mockUser(overrides: { expired?: boolean; refresh_token?: string } = {}): MockUser {
  return {
    access_token: 'tok-123',
    expired: overrides.expired ?? false,
    refresh_token: overrides.refresh_token,
    profile: { sub: 'user-1', preferred_username: 'alice', email: 'alice@example.com' },
  }
}

function Wrapper({ children }: { children: ReactNode }) {
  return <AuthProvider>{children}</AuthProvider>
}

function seedConfig(cfg: ResolvedConfig) {
  window.__AINSEL_CONFIG__ = cfg
}

async function triggerUnauthorized() {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('', { status: 401 })))
  await expect(request('/foo')).rejects.toBeInstanceOf(UnauthorizedError)
}

describe('AuthProvider (oidc mode)', () => {
  beforeEach(() => {
    localStorage.clear()
    setUnauthorizedHandler(null)
    mockOidc = {
      user: null,
      isLoading: false,
      signinSilent: vi.fn().mockResolvedValue(null),
      signinRedirect: vi.fn().mockResolvedValue(undefined),
      signoutRedirect: vi.fn().mockResolvedValue(undefined),
    }
  })

  it('renders children inside the OIDC context', () => {
    seedConfig(OIDC_CFG)
    const { getByTestId } = render(
      <AuthProvider>
        <div data-testid="child">hi</div>
      </AuthProvider>,
    )
    expect(getByTestId('child').textContent).toBe('hi')
  })

  it('mirrors a valid access token', () => {
    mockOidc.user = mockUser()
    seedConfig(OIDC_CFG)
    const { result } = renderHook(() => useAuth(), { wrapper: Wrapper })
    expect(result.current.token).toBe('tok-123')
  })

  it('treats an expired stored user as unauthenticated', () => {
    // Regression test for the infinite refresh loop: an expired user kept
    // its dead token mirrored, so RequireAuth never redirected to the IdP
    // and every reload produced another wave of 401s.
    mockOidc.user = mockUser({ expired: true })
    seedConfig(OIDC_CFG)
    const { result } = renderHook(() => useAuth(), { wrapper: Wrapper })
    expect(result.current.token).toBeNull()
    expect(result.current.ready).toBe(true)
  })

  it('mirrors tokens without an expiry claim (matches isAuthenticated semantics)', () => {
    mockOidc.user = mockUser({ expired: undefined })
    seedConfig(OIDC_CFG)
    const { result } = renderHook(() => useAuth(), { wrapper: Wrapper })
    expect(result.current.token).toBe('tok-123')
  })

  it('on 401 renews silently when a refresh token is available', async () => {
    mockOidc.user = mockUser({ refresh_token: 'rt-1' })
    seedConfig(OIDC_CFG)
    renderHook(() => useAuth(), { wrapper: Wrapper })
    await triggerUnauthorized()
    expect(mockOidc.signinSilent).toHaveBeenCalledTimes(1)
    expect(mockOidc.signinRedirect).not.toHaveBeenCalled()
  })

  it('on 401 redirects to the IdP when there is no refresh token', async () => {
    // Without a refresh token, silent renewal would need a hidden iframe
    // against the IdP — which cannot work when app and IdP are on separate
    // sites — so recovery goes straight to an interactive redirect.
    mockOidc.user = mockUser()
    seedConfig(OIDC_CFG)
    renderHook(() => useAuth(), { wrapper: Wrapper })
    await triggerUnauthorized()
    expect(mockOidc.signinSilent).not.toHaveBeenCalled()
    expect(mockOidc.signinRedirect).toHaveBeenCalledTimes(1)
  })

  it('on 401 falls back to signinRedirect when the silent renew fails', async () => {
    mockOidc.user = mockUser({ refresh_token: 'rt-1' })
    mockOidc.signinSilent = vi.fn().mockRejectedValue(new Error('renew failed'))
    seedConfig(OIDC_CFG)
    renderHook(() => useAuth(), { wrapper: Wrapper })
    await triggerUnauthorized()
    expect(mockOidc.signinSilent).toHaveBeenCalledTimes(1)
    expect(mockOidc.signinRedirect).toHaveBeenCalledTimes(1)
  })

  it('on 401 does nothing while a navigator is already active', async () => {
    mockOidc.user = mockUser()
    mockOidc.activeNavigator = 'signinRedirect'
    seedConfig(OIDC_CFG)
    renderHook(() => useAuth(), { wrapper: Wrapper })
    await triggerUnauthorized()
    expect(mockOidc.signinSilent).not.toHaveBeenCalled()
    expect(mockOidc.signinRedirect).not.toHaveBeenCalled()
  })

  it('only recovers once for a burst of 401s', async () => {
    mockOidc.user = mockUser()
    seedConfig(OIDC_CFG)
    renderHook(() => useAuth(), { wrapper: Wrapper })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('', { status: 401 })))
    await Promise.allSettled([request('/a'), request('/b'), request('/c')])
    expect(mockOidc.signinRedirect).toHaveBeenCalledTimes(1)
  })
})

describe('AuthProvider (local mode)', () => {
  const LOCAL_CFG: ResolvedConfig = {
    // The OIDC fields are ignored in local mode (runtimeConfig short-circuits
    // on authMode), but they are part of the config type.
    authMode: 'local',
    oidcIssuer: 'https://oidc.example.com',
    oidcClientId: 'test-client',
    oidcProjectId: 'test-project',
  }

  function seedLocalSession() {
    localStorage.setItem('ainsel.local.token', 'local-tok')
    localStorage.setItem('ainsel.local.user', JSON.stringify({ sub: 'u', username: 'bob' }))
    localStorage.setItem('ainsel.local.expiresAt', new Date(Date.now() + 60_000).toISOString())
  }

  beforeEach(() => {
    localStorage.clear()
    setUnauthorizedHandler(null)
  })

  it('restores an unexpired local session', () => {
    seedLocalSession()
    seedConfig(LOCAL_CFG)
    const { result } = renderHook(() => useAuth(), { wrapper: Wrapper })
    expect(result.current.token).toBe('local-tok')
  })

  it('on 401 drops the session so RequireAuth routes to /login', async () => {
    seedLocalSession()
    seedConfig(LOCAL_CFG)
    const { result, rerender } = renderHook(() => useAuth(), { wrapper: Wrapper })
    expect(result.current.token).toBe('local-tok')
    await act(async () => {
      await triggerUnauthorized()
    })
    expect(localStorage.getItem('ainsel.local.token')).toBeNull()
    rerender()
    expect(result.current.token).toBeNull()
  })
})
