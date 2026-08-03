import { useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useAuth as useOidcAuth } from 'react-oidc-context'

export function Login() {
  const oidc = useOidcAuth()
  const [searchParams] = useSearchParams()
  const error = searchParams.get('error')

  // If the user is already authenticated (e.g. returning via /login directly),
  // bounce straight in.
  useEffect(() => {
    if (oidc.isAuthenticated) {
      window.location.replace('/dashboard')
    }
  }, [oidc.isAuthenticated])

  return (
    <div style={{ padding: 32, maxWidth: 480, margin: '0 auto' }}>
      <h1>AInsel</h1>
      <p>Sign in with your AInsel account.</p>
      {error && (
        <p style={{ color: 'var(--signal, #b00)', marginTop: 16 }}>
          Sign-in failed: {error}
        </p>
      )}
      <button
        type="button"
        onClick={() => oidc.signinRedirect()}
        style={{ marginTop: 24, padding: '10px 16px' }}
      >
        Continue to login
      </button>
    </div>
  )
}
