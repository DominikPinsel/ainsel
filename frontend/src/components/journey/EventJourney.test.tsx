import { afterEach, describe, expect, it, vi } from 'vitest'
import { screen, render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { EventJourney, ChannelFlowDiagram } from './EventJourney'

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
    expect(screen.getByText('via trigger t1')).toBeInTheDocument()
    expect(screen.getByText('via trigger t2')).toBeInTheDocument()
    // outcome tags per fan-out step
    expect(screen.getByText('delivered')).toBeInTheDocument()
    expect(screen.getByText('failed')).toBeInTheDocument()
    // error message rendered
    expect(screen.getByText('LLM timeout')).toBeInTheDocument()
    // no unmatched terminal
    expect(screen.queryByText(/no rule picked this event up/i)).toBeNull()
  })

  it('renders the unmatched terminal state', () => {
    renderJourney(<EventJourney connector="forgejo" timestamp="2026-06-10T08:31:25Z" matches={[]} />)
    expect(screen.getByText(/no rule picked this event up/i)).toBeInTheDocument()
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
})

describe('ChannelFlowDiagram', () => {
  it('shows producers, router and consumers', () => {
    render(<MemoryRouter><ChannelFlowDiagram /></MemoryRouter>)
    expect(screen.getByText('forgejo')).toBeInTheDocument()
    expect(screen.getByText('cron')).toBeInTheDocument()
    expect(screen.getByText(/router/)).toBeInTheDocument()
    expect(screen.getByText('code-reviewer')).toBeInTheDocument()
  })
})