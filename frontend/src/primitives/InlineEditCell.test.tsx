import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { InlineEditCell } from './InlineEditCell'

describe('InlineEditCell', () => {
  it('renders the value as a button role by default', () => {
    render(<InlineEditCell value="issues.opened" onCommit={() => {}} />)
    expect(screen.getByRole('button', { name: /edit issues.opened/i })).toBeInTheDocument()
  })

  it('clicking the cell switches to input mode', async () => {
    render(<InlineEditCell value="hi" onCommit={() => {}} />)
    await userEvent.click(screen.getByRole('button'))
    expect(screen.getByRole('textbox')).toHaveValue('hi')
  })

  it('Enter commits the new value', async () => {
    const onCommit = vi.fn()
    render(<InlineEditCell value="old" onCommit={onCommit} />)
    await userEvent.click(screen.getByRole('button'))
    const input = screen.getByRole('textbox')
    await userEvent.clear(input)
    await userEvent.type(input, 'new')
    await userEvent.keyboard('{Enter}')
    expect(onCommit).toHaveBeenCalledWith('new')
  })

  it('Escape cancels and does not call onCommit', async () => {
    const onCommit = vi.fn()
    render(<InlineEditCell value="old" onCommit={onCommit} />)
    await userEvent.click(screen.getByRole('button'))
    const input = screen.getByRole('textbox')
    await userEvent.clear(input)
    await userEvent.type(input, 'changed')
    await userEvent.keyboard('{Escape}')
    expect(onCommit).not.toHaveBeenCalled()
    expect(screen.getByRole('button')).toHaveTextContent('old')
  })

  it('does not commit when value unchanged', async () => {
    const onCommit = vi.fn()
    render(<InlineEditCell value="same" onCommit={onCommit} />)
    await userEvent.click(screen.getByRole('button'))
    await userEvent.keyboard('{Enter}')
    expect(onCommit).not.toHaveBeenCalled()
  })

  it('does not commit when draft is empty', async () => {
    const onCommit = vi.fn()
    render(<InlineEditCell value="old" onCommit={onCommit} />)
    await userEvent.click(screen.getByRole('button'))
    await userEvent.clear(screen.getByRole('textbox'))
    await userEvent.keyboard('{Enter}')
    expect(onCommit).not.toHaveBeenCalled()
  })

  it('applies error class when recentError set', () => {
    render(<InlineEditCell value="x" onCommit={() => {}} recentError />)
    expect(screen.getByRole('button')).toHaveClass('inline-edit', 'error')
  })

  it('keyboard activation (Enter on the cell) opens the editor', async () => {
    render(<InlineEditCell value="x" onCommit={() => {}} />)
    const cell = screen.getByRole('button')
    cell.focus()
    await userEvent.keyboard('{Enter}')
    expect(screen.getByRole('textbox')).toBeInTheDocument()
  })
})
