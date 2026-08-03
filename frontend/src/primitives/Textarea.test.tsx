import { render } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Textarea } from './Textarea'

describe('Textarea', () => {
  it('renders with .textarea class', () => {
    render(<Textarea id="x" />)
    const el = document.getElementById('x') as HTMLTextAreaElement
    expect(el).toHaveClass('textarea')
  })

  it('forwards value and onChange', async () => {
    const onChange = vi.fn()
    render(<Textarea id="x" onChange={onChange} />)
    const ta = document.getElementById('x') as HTMLTextAreaElement
    await userEvent.type(ta, 'hi')
    expect(onChange).toHaveBeenCalled()
  })

  it('merges extra className', () => {
    render(<Textarea id="x" className="mono" />)
    const el = document.getElementById('x') as HTMLTextAreaElement
    expect(el).toHaveClass('textarea', 'mono')
  })

  it('applies provided rows', () => {
    render(<Textarea id="x" rows={12} />)
    const el = document.getElementById('x') as HTMLTextAreaElement
    expect(el).toHaveAttribute('rows', '12')
  })
})
