import { useEffect, type ReactNode } from 'react'
import { useAuth as useOidcAuth } from 'react-oidc-context'

export function RequireAuth({ children }: { children: ReactNode }) {
  const oidc = useOidcAuth()

  useEffect(() => {
    if (!oidc.isLoading && !oidc.isAuthenticated && !oidc.activeNavigator) {
      oidc.signinRedirect()
    }
  }, [oidc.isLoading, oidc.isAuthenticated, oidc.activeNavigator, oidc])

  if (oidc.isLoading || oidc.activeNavigator) return null
  if (!oidc.isAuthenticated) return null
  return <>{children}</>
}
