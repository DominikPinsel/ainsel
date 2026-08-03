import { render } from '@testing-library/react'
import { Sparkbar } from './Sparkbar'

describe('Sparkbar', () => {
  it('renders one bar per value', () => {
    const { container } = render(<Sparkbar values={[1, 2, 3, 4]} />)
    const root = container.firstChild as HTMLElement
    expect(root).toHaveClass('spark')
    expect(root.children).toHaveLength(4)
  })

  it('scales bar heights as percentage of max', () => {
    const { container } = render(<Sparkbar values={[5, 10]} />)
    const bars = (container.firstChild as HTMLElement).children
    expect((bars[0] as HTMLElement).style.height).toBe('50%')
    expect((bars[1] as HTMLElement).style.height).toBe('100%')
  })

  it('renders empty placeholder when values is empty', () => {
    const { container } = render(<Sparkbar values={[]} />)
    const root = container.firstChild as HTMLElement
    expect(root.children).toHaveLength(0)
  })

  it('handles all-zero values (no NaN)', () => {
    const { container } = render(<Sparkbar values={[0, 0, 0]} />)
    const bars = (container.firstChild as HTMLElement).children
    expect((bars[0] as HTMLElement).style.height).toBe('0%')
  })

  it('applies alert class when alert prop set', () => {
    const { container } = render(<Sparkbar values={[1, 2]} alert />)
    expect(container.firstChild).toHaveClass('spark', 'alert')
  })
})
