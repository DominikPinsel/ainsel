import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Input } from './Input'

describe('Input', () => {
  it('renders with input class', () => {
    render(<Input id="x" />)
    const input = document.getElementById('x') as HTMLInputElement
    expect(input).toHaveClass('input')
  })

  it('forwards type prop', () => {
    render(<Input id="x" type="password" />)
    const input = document.getElementById('x') as HTMLInputElement
    expect(input.type).toBe('password')
  })

  it('defaults type to text', () => {
    render(<Input id="x" />)
    const input = document.getElementById('x') as HTMLInputElement
    expect(input.type).toBe('text')
  })

  it('forwards value and onChange', async () => {
    const onChange = vi.fn()
    render(<Input onChange={onChange} />)
    await userEvent.type(screen.getByRole('textbox'), 'ab')
    expect(onChange).toHaveBeenCalled()
  })

  it('merges extra className with input class', () => {
    render(<Input id="x" className="extra" />)
    const input = document.getElementById('x') as HTMLInputElement
    expect(input).toHaveClass('input', 'extra')
  })
})
