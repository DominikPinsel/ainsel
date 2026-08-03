import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Button } from './Button'

describe('Button', () => {
  it('renders default variant', () => {
    render(<Button>Click</Button>)
    const btn = screen.getByRole('button', { name: 'Click' })
    expect(btn).toHaveClass('btn')
    expect(btn).not.toHaveClass('btn-primary', 'btn-danger', 'btn-ghost')
  })

  it.each(['primary', 'danger', 'ghost'] as const)('applies %s variant class', (variant) => {
    render(<Button variant={variant}>X</Button>)
    expect(screen.getByRole('button')).toHaveClass(`btn-${variant}`)
  })

  it('applies size modifier', () => {
    render(<Button size="sm">X</Button>)
    expect(screen.getByRole('button')).toHaveClass('btn-sm')
  })

  it('disables and ignores click when disabled', async () => {
    const onClick = vi.fn()
    render(
      <Button onClick={onClick} disabled>
        X
      </Button>,
    )
    await userEvent.click(screen.getByRole('button'))
    expect(onClick).not.toHaveBeenCalled()
  })

  it('defaults type to button (not submit)', () => {
    render(<Button>X</Button>)
    expect(screen.getByRole('button')).toHaveAttribute('type', 'button')
  })
})
