import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { AuthProvider } from './AuthProvider'

describe('AuthProvider', () => {
  it('renders children inside the OIDC context', () => {
    // Seed runtime-config so AuthProvider can construct.
    window.__AINSEL_CONFIG__ = {
      oidcIssuer: 'https://oidc.example.com',
      oidcClientId: 'test-client',
      oidcProjectId: 'test-project',
    }
    const { getByTestId } = render(
      <AuthProvider>
        <div data-testid="child">hi</div>
      </AuthProvider>,
    )
    expect(getByTestId('child').textContent).toBe('hi')
  })
})
