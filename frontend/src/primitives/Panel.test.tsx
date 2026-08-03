import { render, screen } from '@testing-library/react'
import { Panel } from './Panel'

describe('Panel', () => {
  it('renders header title and body children', () => {
    render(
      <Panel title="Status">
        <p>body</p>
      </Panel>,
    )
    expect(screen.getByText('Status')).toBeInTheDocument()
    expect(screen.getByText('body')).toBeInTheDocument()
  })

  it('renders without header when no title', () => {
    const { container } = render(<Panel>body</Panel>)
    expect(container.querySelector('.panel-header')).toBeNull()
  })

  it('renders header right-side slot', () => {
    render(
      <Panel title="X" right={<span data-testid="r">right</span>}>
        body
      </Panel>,
    )
    expect(screen.getByTestId('r')).toBeInTheDocument()
  })

  it('forwards className alongside .panel', () => {
    const { container } = render(<Panel className="cropped">body</Panel>)
    expect(container.firstChild).toHaveClass('panel', 'cropped')
  })
})
