import { render, screen } from '@testing-library/react'
import { Tag } from './Tag'

describe('Tag', () => {
  it('renders default variant', () => {
    render(<Tag>Ready</Tag>)
    const el = screen.getByText('Ready')
    expect(el).toHaveClass('tag')
    expect(el.className).not.toMatch(/(ok|warn|err|stale|solid)/)
  })

  it.each(['ok', 'warn', 'err', 'stale'] as const)('applies %s variant', (variant) => {
    render(<Tag variant={variant}>X</Tag>)
    expect(screen.getByText('X')).toHaveClass('tag', variant)
  })

  it('applies solid modifier', () => {
    render(<Tag solid>X</Tag>)
    expect(screen.getByText('X')).toHaveClass('tag', 'solid')
  })

  it('combines solid with err variant', () => {
    render(
      <Tag variant="err" solid>
        ERROR
      </Tag>,
    )
    expect(screen.getByText('ERROR')).toHaveClass('tag', 'err', 'solid')
  })
})
