import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { Spine } from './Spine'

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Spine operator="kim" />
    </MemoryRouter>,
  )
}

describe('Spine', () => {
  it('renders the brand mark and the operator footer', () => {
    renderAt('/dashboard')
    expect(screen.getByText('AInsel')).toBeInTheDocument()
    expect(screen.getByText('kim')).toBeInTheDocument()
  })

  it('marks the route matching the current path as active', () => {
    renderAt('/agents')
    const agents = screen.getByRole('link', { name: /Agents/i })
    expect(agents).toHaveAttribute('aria-current', 'page')
  })

  it('marks the Docs entry as active when on /docs', () => {
    renderAt('/docs')
    const docs = screen.getByRole('link', { name: /Docs/i })
    expect(docs).toHaveAttribute('aria-current', 'page')
  })

  it('marks the Docs entry as active when on /docs/mcp', () => {
    renderAt('/docs/mcp')
    const docs = screen.getByRole('link', { name: /Docs/i })
    expect(docs).toHaveAttribute('aria-current', 'page')
  })

  it('renders all nav items', () => {
    renderAt('/dashboard')
    const expected = [
      'Dashboard',
      'Activity',
      'Observability',
      'Agents',
      'Personas',
      'Agent Images',
      'Connectors',
      'Docs',
      'Skills',
      'Settings',
    ]
    for (const name of expected) {
      expect(screen.getByRole('link', { name: new RegExp(name, 'i') })).toBeInTheDocument()
    }
  })

  it('does not render Error Log in nav (redirected to /observability/errors)', () => {
    renderAt('/dashboard')
    expect(screen.queryByRole('link', { name: /Error Log/i })).toBeNull()
  })

  it('no longer renders the Triggers entry (moved under Agent Detail)', () => {
    renderAt('/dashboard')
    expect(screen.queryByRole('link', { name: /^Triggers$/i })).toBeNull()
  })
})