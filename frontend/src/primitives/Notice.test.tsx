import { render, screen } from '@testing-library/react'
import { Notice } from './Notice'

describe('Notice', () => {
  it('defaults to a polite status region with no variant modifier', () => {
    render(<Notice>MCP tools refreshed.</Notice>)
    const el = screen.getByRole('status')
    expect(el).toHaveClass('notice')
    expect(el.className).not.toMatch(/notice-(info|warn|err)/)
    expect(el).toHaveTextContent('MCP tools refreshed.')
  })

  it.each(['warn', 'err'] as const)('%s is announced as an alert', (variant) => {
    render(<Notice variant={variant}>Something happened</Notice>)
    expect(screen.getByRole('alert')).toHaveClass('notice', `notice-${variant}`)
  })

  it('renders rich children, not just a string', () => {
    render(
      <Notice variant="warn">
        <div>
          <strong>MCP refresh warnings:</strong>
          <ul>
            <li>ainsel: i/o timeout</li>
            <li>mem0: 503</li>
          </ul>
        </div>
      </Notice>,
    )
    const el = screen.getByRole('alert')
    expect(el.querySelectorAll('li')).toHaveLength(2)
    expect(el).toHaveTextContent('MCP refresh warnings:')
  })

  it('carries no inline colour of its own', () => {
    // The banners this replaced styled themselves inline with --warning and
    // --success, which the palette does not define, so they rendered hardcoded
    // Tailwind amber and green in every theme. All colour lives in .notice*.
    render(<Notice variant="err">Save failed.</Notice>)
    expect(screen.getByRole('alert')).not.toHaveAttribute('style')
  })
})
