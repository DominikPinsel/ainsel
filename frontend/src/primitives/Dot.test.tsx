import { render } from '@testing-library/react'
import { Dot } from './Dot'

describe('Dot', () => {
  it.each(['ok', 'warn', 'err', 'stale'] as const)('renders %s state', (state) => {
    const { container } = render(<Dot state={state} />)
    const el = container.firstChild as HTMLElement
    expect(el).toHaveClass('dot', state)
  })

  it('forwards aria-label when provided', () => {
    const { container } = render(<Dot state="err" aria-label="failing" />)
    const el = container.firstChild as HTMLElement
    expect(el).toHaveAttribute('aria-label', 'failing')
    expect(el).toHaveAttribute('role', 'img')
  })

  it('omits role and aria-label when no label provided', () => {
    const { container } = render(<Dot state="ok" />)
    const el = container.firstChild as HTMLElement
    expect(el).not.toHaveAttribute('role')
    expect(el).not.toHaveAttribute('aria-label')
  })
})
