import { render } from '@testing-library/react'
import { Cropped } from './Cropped'

describe('Cropped', () => {
  it('renders four corner marks and children', () => {
    const { container } = render(<Cropped>child</Cropped>)
    const root = container.firstChild as HTMLElement
    expect(root).toHaveClass('cropped')
    expect(root.querySelectorAll(':scope > .c1')).toHaveLength(1)
    expect(root.querySelectorAll(':scope > .c2')).toHaveLength(1)
    expect(root).toHaveTextContent('child')
  })

  it('forwards extra className', () => {
    const { container } = render(<Cropped className="panel">x</Cropped>)
    expect(container.firstChild).toHaveClass('cropped', 'panel')
  })
})
