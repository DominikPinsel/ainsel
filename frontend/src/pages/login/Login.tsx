import { useEffect, useState, type FormEvent } from 'react'
import { Navigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../../auth/AuthProvider'
import { Button } from '../../primitives/Button'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'

// Login renders per auth mode:
//   - local: username/password form against /api/v1/auth/login,
//   - oidc:  redirect button to the external IdP (legacy flow),
//   - none:  straight through to the dashboard.
export function Login() {
  const auth = useAuth()
  const [searchParams] = useSearchParams()
  const paramError = searchParams.get('error')

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // Already authenticated — bounce straight in.
  useEffect(() => {
    if (auth.ready && auth.token) {
      window.location.replace(`${import.meta.env.BASE_URL}dashboard`)
    }
  }, [auth.ready, auth.token])

  if (auth.mode === 'none') {
    return <Navigate to="/dashboard" replace />
  }

  const submitLocal = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await auth.login(username, password)
      window.location.replace(`${import.meta.env.BASE_URL}dashboard`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Sign-in failed')
      setBusy(false)
    }
  }

  const displayError = error ?? paramError

  return (
    <div style={{ padding: 32, maxWidth: 480, margin: '0 auto' }}>
      <h1>AInsel</h1>
      {auth.mode === 'local' ? (
        <>
          <p>Sign in with your local AInsel account.</p>
          {displayError && (
            <p style={{ color: 'var(--signal, #b00)', marginTop: 16 }} role="alert">
              Sign-in failed: {displayError}
            </p>
          )}
          <form onSubmit={submitLocal} style={{ marginTop: 24, display: 'grid', gap: 16 }}>
            <Field label="Username" htmlFor="login-username">
              <Input
                id="login-username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="username"
                autoFocus
                required
              />
            </Field>
            <Field label="Password" htmlFor="login-password">
              <Input
                id="login-password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                required
              />
            </Field>
            <Button type="submit" variant="primary" disabled={busy}>
              {busy ? 'Signing in…' : 'Sign in'}
            </Button>
          </form>
        </>
      ) : (
        <>
          <p>Sign in with your AInsel account.</p>
          {displayError && (
            <p style={{ color: 'var(--signal, #b00)', marginTop: 16 }}>
              Sign-in failed: {displayError}
            </p>
          )}
          <Button
            type="button"
            variant="primary"
            onClick={() => auth.signinRedirect()}
            style={{ marginTop: 24 }}
          >
            Continue to login
          </Button>
        </>
      )}
    </div>
  )
}
