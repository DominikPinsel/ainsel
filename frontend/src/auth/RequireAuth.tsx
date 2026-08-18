import { useEffect, type ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from './AuthProvider'

// RequireAuth gates the authenticated app shell per mode:
//   - none:  everyone passes (there is no auth),
//   - local: without a valid token the user is sent to /login,
//   - oidc:  unauthenticated users are redirected to the IdP (legacy flow).
export function RequireAuth({ children }: { children: ReactNode }) {
  const auth = useAuth()

  const oidcNeedsRedirect = auth.mode === 'oidc' && auth.ready && !auth.token
  useEffect(() => {
    if (oidcNeedsRedirect) auth.signinRedirect()
  }, [oidcNeedsRedirect, auth])

  if (auth.mode === 'none') return <>{children}</>
  if (!auth.ready) return null
  if (!auth.token) {
    if (auth.mode === 'local') return <Navigate to="/login" replace />
    return null // oidc: signinRedirect is in flight
  }
  return <>{children}</>
}
