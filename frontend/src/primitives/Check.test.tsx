import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { Check } from './Check'

function Wrapper() {
  const [v, setV] = useState(false)
  return <Check checked={v} onChange={setV} aria-label="remember me" />
}

describe('Check', () => {
  it('renders unchecked by default', () => {
    render(<Check checked={false} onChange={() => {}} aria-label="x" />)
    const root = screen.getByRole('checkbox', { name: 'x' })
    expect(root).toHaveClass('check')
    expect(root).not.toHaveClass('on')
    expect(root).toHaveAttribute('aria-checked', 'false')
  })

  it('renders checked class when checked', () => {
    render(<Check checked onChange={() => {}} aria-label="x" />)
    expect(screen.getByRole('checkbox')).toHaveClass('on')
  })

  it('calls onChange with toggled value on click', async () => {
    render(<Wrapper />)
    const cb = screen.getByRole('checkbox', { name: 'remember me' })
    await userEvent.click(cb)
    expect(cb).toHaveClass('on')
  })

  it('toggles via keyboard space', async () => {
    render(<Wrapper />)
    const cb = screen.getByRole('checkbox', { name: 'remember me' })
    cb.focus()
    await userEvent.keyboard(' ')
    expect(cb).toHaveClass('on')
  })

  it('does not toggle when disabled', async () => {
    const onChange = vi.fn()
    render(<Check checked={false} onChange={onChange} disabled aria-label="x" />)
    await userEvent.click(screen.getByRole('checkbox'))
    expect(onChange).not.toHaveBeenCalled()
  })
})
