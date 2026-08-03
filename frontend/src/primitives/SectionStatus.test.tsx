import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SectionStatus } from './SectionStatus'

describe('SectionStatus', () => {
  it.each(['ready', 'idle'] as const)('renders nothing for %s', (state) => {
    const { container } = render(<SectionStatus state={state} />)
    expect(container.firstChild).toBeNull()
  })

  it('renders default title for loading', () => {
    render(<SectionStatus state="loading" />)
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })

  it('renders default unavailable copy', () => {
    render(<SectionStatus state="unavailable" />)
    expect(screen.getByText('Telemetry not configured')).toBeInTheDocument()
  })

  it('renders error with retry button', async () => {
    const onRetry = vi.fn()
    render(<SectionStatus state="error" detail="500 from upstream" onRetry={onRetry} />)
    expect(screen.getByText('Failed to load')).toBeInTheDocument()
    expect(screen.getByText('500 from upstream')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /retry/i }))
    expect(onRetry).toHaveBeenCalled()
  })

  it('omits retry button when onRetry not provided', () => {
    render(<SectionStatus state="error" />)
    expect(screen.queryByRole('button', { name: /retry/i })).not.toBeInTheDocument()
  })

  it('respects custom title', () => {
    render(<SectionStatus state="loading" title="Spinning…" />)
    expect(screen.getByText('Spinning…')).toBeInTheDocument()
  })
})
