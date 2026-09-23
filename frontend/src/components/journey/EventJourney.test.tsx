import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { EventJourney } from './EventJourney'
import { renderWithProviders } from '../../test/renderWithProviders'

// The journey resolves channel ids through the hub registry, so it renders
// inside the app providers like every other screen.
function renderJourney(ui: React.ReactElement) {
  return renderWithProviders(ui)
}

describe('EventJourney', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              items: [
                {
                  id: 'ch-forgejo',
                  kind: 'connector',
                  name: 'forgejo',
                  description: '',
                  entityRef: 'forgejo',
                  counts: { events: 0, unmatched: 0, failed: 0 },
                  bridges: 0,
                  subscriptions: 0,
                },
                {
                  id: 'ch-review-bot',
                  kind: 'agent',
                  name: 'review-bot',
                  description: '',
                  entityRef: 'review-bot',
                  counts: { events: 0, unmatched: 0, failed: 0 },
                  bridges: 0,
                  subscriptions: 0,
                },
                {
                  id: 'ch-doc-writer',
                  kind: 'agent',
                  name: 'doc-writer',
                  description: '',
                  entityRef: 'doc-writer',
                  counts: { events: 0, unmatched: 0, failed: 0 },
                  bridges: 0,
                  subscriptions: 0,
                },
              ],
              total: 2,
              page: 1,
              pageSize: 500,
              totalPages: 1,
            }),
            { status: 200 },
          ),
        ),
      ),
    )
  })
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('links the home channel by the id the hub stamped and each inbox by its own id', async () => {
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
    // ids, not labels: the birth row uses channelId, the fan-out resolves the
    // agent's inbox through the registry.
    expect(await screen.findByRole('link', { name: /doc-writer/ })).toHaveAttribute(
      'href',
      '/channels/ch-doc-writer',
    )
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
    renderJourney(
      <EventJourney connector="forgejo" timestamp="2026-06-10T08:31:25Z" matches={[]} />,
    )
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
    const { container } = renderJourney(
      <EventJourney timestamp="2026-06-10T08:31:25Z" matches={[]} />,
    )
    expect(screen.getByTestId('event-journey')).toBeInTheDocument()
    expect(container.querySelectorAll('.journey-step')).toHaveLength(0)
  })

  it('renders cron/chat as direct births without a channel link', async () => {
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
    // but the destination inbox still links, by its channel id
    expect(await screen.findByRole('link', { name: /review-bot/ })).toHaveAttribute(
      'href',
      '/channels/ch-review-bot',
    )
  })
})
