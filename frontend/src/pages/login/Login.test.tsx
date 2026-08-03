import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { Login } from './Login'

vi.mock('react-oidc-context', () => ({
  useAuth: () => ({ isAuthenticated: false, signinRedirect: vi.fn() }),
}))

describe('Login', () => {
  it('renders the sign-in button', () => {
    const { getByRole } = render(
      <MemoryRouter>
        <Login />
      </MemoryRouter>,
    )
    expect(getByRole('button').textContent).toContain('Continue to login')
  })
})
