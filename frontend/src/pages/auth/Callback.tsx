import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth as useOidcAuth } from 'react-oidc-context'

// Lands after Zitadel redirects back with ?code=…&state=…. react-oidc-context
// processes the params automatically; we just wait for `isAuthenticated`
// and bounce the user to the dashboard.
export function Callback() {
  const oidc = useOidcAuth()
  const navigate = useNavigate()

  useEffect(() => {
    if (!oidc.isLoading) {
      if (oidc.error) {
        navigate(`/login?error=${encodeURIComponent(oidc.error.message)}`, { replace: true })
        return
      }
      if (oidc.isAuthenticated) {
        navigate('/dashboard', { replace: true })
      }
    }
  }, [oidc.isLoading, oidc.error, oidc.isAuthenticated, navigate])

  return null
}
