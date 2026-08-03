import { useMemo, type ReactNode } from 'react'
import { AuthProvider as OidcProvider, useAuth as useOidcAuth } from 'react-oidc-context'
import { WebStorageStateStore } from 'oidc-client-ts'
import { setAuthToken } from '../api/client'
import { runtimeConfig } from '../runtime-config'

// Mirrors the OIDC access token into the API client during render — not in a
// useEffect — so that any child component's data fetch (react-query, etc.)
// sees the token on the very first render after auth completes. Effects in
// children fire before effects in this wrapper, so an effect-based sync would
// let the first fetch wave race ahead unauthenticated and produce a flood of
// 401s + reload loop.
function TokenSync({ children }: { children: ReactNode }) {
  const oidc = useOidcAuth()
  setAuthToken(oidc.user?.access_token ?? null)
  return <>{children}</>
}

export function AuthProvider({ children }: { children: ReactNode }) {
  // Read config at render time, not module-load time, so tests can seed
  // window.__AINSEL_CONFIG__ before mounting the provider.
  const oidcConfig = useMemo(() => {
    const { oidcIssuer, oidcClientId, oidcProjectId } = runtimeConfig()
    return {
      authority: oidcIssuer,
      client_id: oidcClientId,
      redirect_uri: `${window.location.origin}${import.meta.env.BASE_URL.replace(/\/$/, '')}/auth/callback`,
      post_logout_redirect_uri: `${window.location.origin}${import.meta.env.BASE_URL}`,
      response_type: 'code',
      scope: `openid profile email urn:zitadel:iam:org:project:id:${oidcProjectId}:aud`,
      loadUserInfo: true,
      userStore: new WebStorageStateStore({ store: window.localStorage }),
      automaticSilentRenew: true,
      // After signinRedirectCallback, clear the code from the URL so it can't
      // be reused and isn't visible in history.
      onSigninCallback: () => {
        window.history.replaceState({}, document.title, window.location.pathname)
      },
    }
  }, [])

  return (
    <OidcProvider {...oidcConfig}>
      <TokenSync>{children}</TokenSync>
    </OidcProvider>
  )
}

// useAuth: the shape consumers in this codebase expect.
// Backed by the OIDC provider. Token sync into the API client is handled by
// the TokenSync wrapper above — this hook is read-only.
// eslint-disable-next-line react-refresh/only-export-components
export function useAuth() {
  const oidc = useOidcAuth()
  return {
    token: oidc.user?.access_token ?? null,
    user: oidc.user?.profile
      ? {
          username: (oidc.user.profile.preferred_username as string | undefined) ?? '',
          email: (oidc.user.profile.email as string | undefined) ?? '',
        }
      : null,
    ready: !oidc.isLoading,
    signinRedirect: () => oidc.signinRedirect(),
    signoutRedirect: () => oidc.signoutRedirect(),
  }
}
