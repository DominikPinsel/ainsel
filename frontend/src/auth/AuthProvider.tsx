import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { AuthProvider as OidcProvider, useAuth as useOidcAuth } from 'react-oidc-context'
import { WebStorageStateStore } from 'oidc-client-ts'
import { setAuthToken } from '../api/client'
import { login as apiLogin } from '../api/auth'
import { runtimeConfig, type AuthMode, type ResolvedConfig } from '../runtime-config'

// AuthUser is the identity shape exposed by useAuth() in every mode.
export type AuthUser = {
  sub: string
  username: string
  email?: string
  isAdmin?: boolean
}

// AuthState is the unified auth surface. Components should only use this —
// never react-oidc-context directly — so all modes (oidc/local/none) behave
// identically from their point of view.
export type AuthState = {
  mode: AuthMode
  ready: boolean
  token: string | null
  user: AuthUser | null
  /** Informational only — admin decisions must come from the users API. */
  isAdmin: boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
  signinRedirect: () => void
  signoutRedirect: () => void
}

const AuthCtx = createContext<AuthState | null>(null)

// localStorage keys for the local-mode session.
const TOKEN_KEY = 'ainsel.local.token'
const USER_KEY = 'ainsel.local.user'
const EXPIRES_KEY = 'ainsel.local.expiresAt'

function clearStoredSession() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
  localStorage.removeItem(EXPIRES_KEY)
}

// loadStoredSession restores a local session unless it is expired. Expired
// sessions are cleared eagerly so RequireAuth routes to /login immediately
// instead of triggering one round of 401s.
function loadStoredSession(): { token: string | null; user: AuthUser | null } {
  try {
    const token = localStorage.getItem(TOKEN_KEY)
    const expiresAt = localStorage.getItem(EXPIRES_KEY)
    if (!token || !expiresAt || new Date(expiresAt).getTime() <= Date.now()) {
      clearStoredSession()
      return { token: null, user: null }
    }
    const user = JSON.parse(localStorage.getItem(USER_KEY) ?? 'null') as AuthUser | null
    return { token, user }
  } catch {
    clearStoredSession()
    return { token: null, user: null }
  }
}

function LocalAuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState(loadStoredSession)

  // Mirror the token into the API client during render (not in an effect) so
  // the first fetch wave after login already carries it — same reasoning as
  // the OIDC TokenSync below.
  setAuthToken(session.token)

  const login = useCallback(async (username: string, password: string) => {
    const res = await apiLogin(username, password)
    localStorage.setItem(TOKEN_KEY, res.token)
    localStorage.setItem(USER_KEY, JSON.stringify(res.user))
    localStorage.setItem(EXPIRES_KEY, res.expiresAt)
    setSession({ token: res.token, user: res.user })
  }, [])

  const logout = useCallback(() => {
    clearStoredSession()
    setSession({ token: null, user: null })
  }, [])

  const state: AuthState = useMemo(
    () => ({
      mode: 'local',
      ready: true,
      token: session.token,
      user: session.user,
      isAdmin: Boolean(session.user?.isAdmin),
      login,
      logout,
      signinRedirect: () => {
        window.location.href = `${import.meta.env.BASE_URL}login`
      },
      signoutRedirect: logout,
    }),
    [session, login, logout],
  )

  return <AuthCtx.Provider value={state}>{children}</AuthCtx.Provider>
}

function NoneAuthProvider({ children }: { children: ReactNode }) {
  setAuthToken(null)
  const state: AuthState = useMemo(
    () => ({
      mode: 'none',
      ready: true,
      token: null,
      user: null,
      isAdmin: false,
      login: async () => undefined,
      logout: () => undefined,
      signinRedirect: () => undefined,
      signoutRedirect: () => undefined,
    }),
    [],
  )
  return <AuthCtx.Provider value={state}>{children}</AuthCtx.Provider>
}

function oidcProviderConfig(cfg: ResolvedConfig) {
  return {
    authority: cfg.oidcIssuer,
    client_id: cfg.oidcClientId,
    redirect_uri: `${window.location.origin}${import.meta.env.BASE_URL.replace(/\/$/, '')}/auth/callback`,
    post_logout_redirect_uri: `${window.location.origin}${import.meta.env.BASE_URL}`,
    response_type: 'code',
    scope: `openid profile email urn:zitadel:iam:org:project:id:${cfg.oidcProjectId}:aud`,
    loadUserInfo: true,
    userStore: new WebStorageStateStore({ store: window.localStorage }),
    automaticSilentRenew: true,
    // After signinRedirectCallback, clear the code from the URL so it can't
    // be reused and isn't visible in history.
    onSigninCallback: () => {
      window.history.replaceState({}, document.title, window.location.pathname)
    },
  }
}

// OidcBridge adapts react-oidc-context state to the unified AuthState.
function OidcBridge({ children }: { children: ReactNode }) {
  const oidc = useOidcAuth()
  // Mirror the OIDC access token into the API client during render — not in
  // a useEffect — so that any child component's data fetch sees the token on
  // the very first render after auth completes.
  setAuthToken(oidc.user?.access_token ?? null)

  const state: AuthState = useMemo(
    () => ({
      mode: 'oidc',
      ready: !oidc.isLoading,
      token: oidc.user?.access_token ?? null,
      user: oidc.user?.profile
        ? {
            sub: String(oidc.user.profile.sub ?? ''),
            username: (oidc.user.profile.preferred_username as string | undefined) ?? '',
            email: (oidc.user.profile.email as string | undefined) ?? '',
          }
        : null,
      isAdmin: false, // resolved via the users API where it matters
      login: async () => {
        throw new Error('local login is not available in oidc mode')
      },
      logout: () => {
        void oidc.signoutRedirect()
      },
      signinRedirect: () => {
        void oidc.signinRedirect()
      },
      signoutRedirect: () => {
        void oidc.signoutRedirect()
      },
    }),
    [oidc],
  )

  return <AuthCtx.Provider value={state}>{children}</AuthCtx.Provider>
}

export function AuthProvider({ children }: { children: ReactNode }) {
  // Read config at render time, not module-load time, so tests can seed
  // window.__AINSEL_CONFIG__ before mounting the provider.
  const cfg = useMemo(() => runtimeConfig(), [])

  if (cfg.authMode === 'oidc') {
    return (
      <OidcProvider {...oidcProviderConfig(cfg)}>
        <OidcBridge>{children}</OidcBridge>
      </OidcProvider>
    )
  }
  if (cfg.authMode === 'local') {
    return <LocalAuthProvider>{children}</LocalAuthProvider>
  }
  return <NoneAuthProvider>{children}</NoneAuthProvider>
}

// useAuth returns the unified auth state for the active mode.
// eslint-disable-next-line react-refresh/only-export-components
export function useAuth(): AuthState {
  const ctx = useContext(AuthCtx)
  if (!ctx) {
    throw new Error('useAuth must be used within <AuthProvider>')
  }
  return ctx
}
