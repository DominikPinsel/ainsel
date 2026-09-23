import { afterEach, describe, expect, it, vi } from 'vitest'
import { screen, render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { EventJourney } from './EventJourney'

function renderJourney(ui: React.ReactElement) {
  return render(<MemoryRouter>{ui}</MemoryRouter>)
}

describe('EventJourney', () => {
  afterEach(() => vi.restoreAllMocks())

  it('renders the home channel and one fan-out step per match', () => {
    renderJourney(
      <EventJourney
        connector="forgejo"
        timestamp="2026-06-10T08:31:25Z"
        matches={[
          { trigger: 't1', agent: 'doc-writer', runStatus: 'success', durationMs: 9000 },
          { trigger: 't2', agent: 'review-bot', runStatus: 'failure', error: 'LLM timeout' },
        ]}
      />,
    )
    expect(screen.getByTestId('event-journey')).toBeInTheDocument()
    expect(screen.getByText('forgejo')).toBeInTheDocument()
    expect(screen.getByText('doc-writer')).toBeInTheDocument()
    expect(screen.getByText('review-bot')).toBeInTheDocument()
    expect(screen.getByText('via subscription t1')).toBeInTheDocument()
    expect(screen.getByText('via subscription t2')).toBeInTheDocument()
    // outcome tags per fan-out step
    expect(screen.getByText('delivered')).toBeInTheDocument()
    expect(screen.getByText('failed')).toBeInTheDocument()
    // error message rendered
    expect(screen.getByText('LLM timeout')).toBeInTheDocument()
    // no unmatched terminal
    expect(screen.queryByText(/no subscription picked this event up/i)).toBeNull()
  })

  it('renders the unmatched terminal state', () => {
    renderJourney(<EventJourney connector="forgejo" timestamp="2026-06-10T08:31:25Z" matches={[]} />)
    expect(screen.getByText(/no subscription picked this event up/i)).toBeInTheDocument()
    expect(screen.getByText(/stays in the/i)).toBeInTheDocument()
  })

  it('renders a pulsing node for running deliveries', () => {
    const { container } = renderJourney(
      <EventJourney
        connector="forgejo"
        timestamp="2026-06-10T08:31:25Z"
        matches={[{ trigger: 't1', agent: 'review-bot', runStatus: 'running' }]}
      />,
    )
    expect(container.querySelector('.journey-dot.pulse')).not.toBeNull()
    expect(screen.getByText('running')).toBeInTheDocument()
  })

  it('renders home-only journey when there is no connector', () => {
    const { container } = renderJourney(<EventJourney timestamp="2026-06-10T08:31:25Z" matches={[]} />)
    expect(screen.getByTestId('event-journey')).toBeInTheDocument()
    expect(container.querySelectorAll('.journey-step')).toHaveLength(0)
  })

  it('renders cron/chat as direct births without a channel link', () => {
    renderJourney(
      <EventJourney
        connector="cron"
        timestamp="2026-06-10T08:31:25Z"
        matches={[{ trigger: '', agent: 'review-bot', runStatus: 'success' }]}
      />,
    )
    expect(screen.getByText(/born directly in the agent inboxes/i)).toBeInTheDocument()
    // the synthetic source is NOT a channel — the cron chip must not link
    const cronChip = screen.getByText('cron').closest('a')
    expect(cronChip).toBeNull()
    // but the destination inbox still links
    expect(
      screen.getByText('review-bot').closest('a'),
    ).toHaveAttribute('href', '/channels/agent/review-bot')
  })
})