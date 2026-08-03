import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Activity } from './Activity'
import { renderWithProviders } from '../../test/renderWithProviders'

const sampleConnectors = [
  { id: 'c-111', name: 'Monorepo Connector' },
  { id: 'c-222', name: 'Infra Tooling' },
  { id: 'c-333', name: 'Docs Public' },
]

const now = Date.now()

// sampleEvents are intentionally out of order so we can verify sort behaviour
const sampleEvents = [
  {
    id: 'e1',
    timestamp: new Date(now - 30_000).toISOString(),   // newest
    connector: 'c-111',
    status: 'matched',
    matches: [{ trigger: 't1', agent: 'doc-writer' }],
    payload: { action: 'opened', pull_request: { number: 123 } },
  },
  {
    id: 'e2',
    timestamp: new Date(now - 240_000).toISOString(),  // oldest
    connector: 'c-222',
    status: 'error',
    payload: { action: 'push', ref: 'refs/heads/main' },
  },
  {
    id: 'e3',
    timestamp: new Date(now - 120_000).toISOString(),  // middle
    connector: 'c-333',
    status: 'unmatched',
    matches: [{ trigger: 't2', agent: 'infra-bot' }, { trigger: 't3', agent: 'sec-bot' }],
  },
]

function mockFetch(events: Record<string, unknown>[] = sampleEvents) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (url.includes('/events')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({ events, total: events.length }),
            { status: 200 },
          ),
        )
      }
      if (url.includes('/connectors')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              items: sampleConnectors,
              total: sampleConnectors.length,
            }),
            { status: 200 },
          ),
        )
      }
      return Promise.resolve(new Response('{}', { status: 200 }))
    }),
  )
}

describe('Activity', () => {
  beforeEach(() => mockFetch())
  afterEach(() => vi.unstubAllGlobals())

  it('renders all three events with status tags', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() =>
      expect(container.querySelector('tbody tr')).not.toBeNull(),
    )
    const body = container.querySelector('tbody') as HTMLElement
    expect(body.textContent).toContain('Monorepo Connector')
    expect(body.textContent).toContain('Infra Tooling')
    expect(body.textContent).toContain('Docs Public')
    expect(screen.getByText('MATCH')).toBeInTheDocument()
    expect(screen.getByText('ERR')).toBeInTheDocument()
    expect(screen.getByText('SKIP')).toBeInTheDocument()
  })

  it('shows newest event first', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())
    const rows = container.querySelectorAll('tbody tr.activity-row')
    // e1 is newest (30s ago), e3 is middle (120s ago), e2 is oldest (240s ago)
    expect(rows[0].textContent).toContain('Monorepo Connector')
    expect(rows[1].textContent).toContain('Docs Public')
    expect(rows[2].textContent).toContain('Infra Tooling')
  })

  it('expands a row to show details when the chevron is clicked', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())
    const chevron = screen.getAllByRole('button', { name: /expand event details/i })[0]
    await userEvent.click(chevron)
    const details = container.querySelector('.activity-details')
    expect(details).not.toBeNull()
    const matchRow = container.querySelector('.activity-matches .match')!
    expect(matchRow.textContent).toMatch(/doc-writer/)
  })

  it('filters by status client-side', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())
    await userEvent.selectOptions(screen.getByLabelText(/filter by status/i), 'error')
    await waitFor(() => {
      const body = container.querySelector('tbody') as HTMLElement
      expect(body.textContent).toContain('Infra Tooling')
      expect(body.textContent).not.toContain('Monorepo Connector')
    })
  })

  it('renders resolved connector names in table and filter dropdown', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() =>
      expect(container.querySelector('tbody tr')).not.toBeNull(),
    )
    const body = container.querySelector('tbody') as HTMLElement
    expect(body.textContent).toContain('Monorepo Connector')
    expect(body.textContent).toContain('Infra Tooling')
    expect(body.textContent).toContain('Docs Public')

    const connectorSelect = screen.getByLabelText(/filter by connector/i)
    expect(connectorSelect).toBeInTheDocument()
    const options = Array.from(connectorSelect.querySelectorAll('option'))
    const labels = options.map((o) => o.textContent)
    expect(labels).toContain('Monorepo Connector')
    expect(labels).toContain('Infra Tooling')
    expect(labels).toContain('Docs Public')
    expect(labels).not.toContain('c-111')
  })

  it('filters by connector id while showing names in UI', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())
    await userEvent.selectOptions(screen.getByLabelText(/filter by connector/i), 'c-111')
    await waitFor(() => {
      const body = container.querySelector('tbody') as HTMLElement
      expect(body.textContent).toContain('Monorepo Connector')
      expect(body.textContent).not.toContain('Infra Tooling')
    })
  })

  it('renders trigger and agent columns with comma-separated matches', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() =>
      expect(container.querySelector('tbody tr')).not.toBeNull(),
    )
    const rows = container.querySelectorAll('tbody tr.activity-row')
    // e1: single match
    expect(rows[0].textContent).toContain('t1')
    expect(rows[0].textContent).toContain('doc-writer')
    // e3: multiple matches, comma-separated
    expect(rows[1].textContent).toContain('t2, t3')
    expect(rows[1].textContent).toContain('infra-bot, sec-bot')
    // e2: no matches → dash
    expect(rows[2].textContent).toContain('—')
  })

  it('resolves trigger and agent names to links in the activity table', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/events')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({ events: sampleEvents, total: sampleEvents.length }),
              { status: 200 },
            ),
          )
        }
        if (url.includes('/connectors')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({ items: sampleConnectors, total: sampleConnectors.length }),
              { status: 200 },
            ),
          )
        }
        if (url.includes('/triggers')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  { id: 't1', name: 'On doc issue' },
                  { id: 't2', name: 'On infra issue' },
                  { id: 't3', name: 'On sec issue' },
                ],
                total: 3,
              }),
              { status: 200 },
            ),
          )
        }
        if (url.includes('/agents')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  { id: 'doc-writer', name: 'Doc Writer' },
                  { id: 'infra-bot', name: 'Infra Bot' },
                  { id: 'sec-bot', name: 'Sec Bot' },
                ],
                total: 3,
              }),
              { status: 200 },
            ),
          )
        }
        return Promise.resolve(new Response('{}', { status: 200 }))
      }),
    )

    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    // e1 (newest): single match resolves to names
    const triggerLink = await screen.findByRole('link', { name: 'Open trigger On doc issue' })
    expect(triggerLink).toHaveAttribute('href', '/agents/doc-writer?tab=triggers')
    const agentLink = screen.getByRole('link', { name: 'Open agent Doc Writer' })
    expect(agentLink).toHaveAttribute('href', '/agents/doc-writer')

    // e3: multiple matches resolve to names
    expect(screen.getByRole('link', { name: 'Open agent Infra Bot' })).toHaveAttribute(
      'href',
      '/agents/infra-bot',
    )
    expect(screen.getByRole('link', { name: 'Open agent Sec Bot' })).toHaveAttribute(
      'href',
      '/agents/sec-bot',
    )
  })

  it('paginates and shows the pager when there are more rows than the page size', async () => {
    const manyEvents = Array.from({ length: 30 }, (_, i) => ({
      id: `e${i}`,
      timestamp: new Date(now - i * 10_000).toISOString(),
      connector: 'c-111',
      status: 'matched' as const,
    }))
    mockFetch(manyEvents)

    const { container } = renderWithProviders(<Activity />, {
      route: '/activity?pageSize=25',
    })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    const rows = container.querySelectorAll('tbody tr.activity-row')
    expect(rows.length).toBe(25)

    expect(screen.getByLabelText('Next page')).toBeInTheDocument()
    expect(screen.getByLabelText('Previous page')).toBeDisabled()
    expect(screen.getByLabelText('Next page')).not.toBeDisabled()
  })

  it('filters events by search query matching payload content', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    const searchInput = screen.getByLabelText(/search events/i)
    await userEvent.type(searchInput, 'pull_request')

    await waitFor(() => {
      const body = container.querySelector('tbody') as HTMLElement
      expect(body.textContent).toContain('Monorepo Connector')
      expect(body.textContent).not.toContain('Infra Tooling')
      expect(body.textContent).not.toContain('Docs Public')
    })
  })

  it('narrows results with multiple AND terms', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    const searchInput = screen.getByLabelText(/search events/i)
    await userEvent.type(searchInput, 'pull_request 123')

    await waitFor(() => {
      const body = container.querySelector('tbody') as HTMLElement
      expect(body.textContent).toContain('Monorepo Connector')
      expect(body.textContent).not.toContain('Infra Tooling')
    })

    // A term that matches nothing should show empty state
    await userEvent.clear(searchInput)
    await userEvent.type(searchInput, 'pull_request 999')

    await waitFor(() => {
      expect(container.querySelector('tbody tr')).toBeNull()
      expect(screen.getByText('No events match the filter.')).toBeInTheDocument()
    })
  })

  it('search is case-insensitive', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    const searchInput = screen.getByLabelText(/search events/i)
    await userEvent.type(searchInput, 'PULL_REQUEST')

    await waitFor(() => {
      const body = container.querySelector('tbody') as HTMLElement
      expect(body.textContent).toContain('Monorepo Connector')
      expect(body.textContent).not.toContain('Infra Tooling')
    })
  })

  it('empty or whitespace-only query shows all rows', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    const searchInput = screen.getByLabelText(/search events/i)
    await userEvent.type(searchInput, '   ')

    const rows = container.querySelectorAll('tbody tr.activity-row')
    expect(rows.length).toBe(3)
  })

  it('search matches connector names and status', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    const searchInput = screen.getByLabelText(/search events/i)
    await userEvent.type(searchInput, 'infra tooling')

    await waitFor(() => {
      const body = container.querySelector('tbody') as HTMLElement
      expect(body.textContent).toContain('Infra Tooling')
      expect(body.textContent).not.toContain('Monorepo Connector')
    })
  })

  it('search combines with status filter', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    // Filter by status=matched first
    await userEvent.selectOptions(screen.getByLabelText(/filter by status/i), 'matched')
    // Then search for something in the error event's payload
    const searchInput = screen.getByLabelText(/search events/i)
    await userEvent.type(searchInput, 'push')

    await waitFor(() => {
      expect(container.querySelector('tbody tr')).toBeNull()
      expect(screen.getByText('No events match the filter.')).toBeInTheDocument()
    })
  })

  it('persists search query in the URL q param', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity?q=pull_request' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    const body = container.querySelector('tbody') as HTMLElement
    expect(body.textContent).toContain('Monorepo Connector')
    expect(body.textContent).not.toContain('Infra Tooling')

    const searchInput = screen.getByLabelText(/search events/i) as HTMLInputElement
    expect(searchInput.value).toBe('pull_request')
  })

  it('renders agent filter dropdown with options from event matches', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    const agentSelect = screen.getByLabelText(/filter by agent/i)
    expect(agentSelect).toBeInTheDocument()
    const options = Array.from(agentSelect.querySelectorAll('option'))
    const values = options.map((o) => o.value).filter(Boolean)
    // Should contain agent IDs from matches across all events
    expect(values).toContain('doc-writer')
    expect(values).toContain('infra-bot')
    expect(values).toContain('sec-bot')
  })

  it('filters by agent to show only events with that agent', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    // Filter by doc-writer — only e1 has this agent
    await userEvent.selectOptions(screen.getByLabelText(/filter by agent/i), 'doc-writer')
    await waitFor(() => {
      const body = container.querySelector('tbody') as HTMLElement
      expect(body.textContent).toContain('Monorepo Connector')
      expect(body.textContent).not.toContain('Infra Tooling')
      expect(body.textContent).not.toContain('Docs Public')
    })
  })

  it('agent filter shows events with multiple matches when any match has the agent', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    // Filter by infra-bot — only e3 has this agent (in a multi-match)
    await userEvent.selectOptions(screen.getByLabelText(/filter by agent/i), 'infra-bot')
    await waitFor(() => {
      const body = container.querySelector('tbody') as HTMLElement
      expect(body.textContent).toContain('Docs Public')
      expect(body.textContent).not.toContain('Monorepo Connector')
      expect(body.textContent).not.toContain('Infra Tooling')
    })
  })

  it('agent filter excludes unmatched events', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    // Filter by sec-bot — e3 matches, e2 (unmatched/error) should be excluded
    await userEvent.selectOptions(screen.getByLabelText(/filter by agent/i), 'sec-bot')
    await waitFor(() => {
      const rows = container.querySelectorAll('tbody tr.activity-row')
      expect(rows.length).toBe(1)
      expect(rows[0].textContent).toContain('Docs Public')
    })
  })

  it('selecting Any agent resets the filter', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    // Apply agent filter
    await userEvent.selectOptions(screen.getByLabelText(/filter by agent/i), 'doc-writer')
    await waitFor(() => {
      const rows = container.querySelectorAll('tbody tr.activity-row')
      expect(rows.length).toBe(1)
    })

    // Reset to "Any agent"
    await userEvent.selectOptions(screen.getByLabelText(/filter by agent/i), '')
    await waitFor(() => {
      const rows = container.querySelectorAll('tbody tr.activity-row')
      expect(rows.length).toBe(3)
    })
  })

  it('agent filter combines with status filter using AND logic', async () => {
    const { container } = renderWithProviders(<Activity />, { route: '/activity' })
    await waitFor(() => expect(container.querySelector('tbody tr')).not.toBeNull())

    // Filter by agent=doc-writer AND status=error → no results (e1 is matched, not error)
    await userEvent.selectOptions(screen.getByLabelText(/filter by agent/i), 'doc-writer')
    await userEvent.selectOptions(screen.getByLabelText(/filter by status/i), 'error')

    await waitFor(() => {
      expect(container.querySelector('tbody tr')).toBeNull()
      expect(screen.getByText('No events match the filter.')).toBeInTheDocument()
    })
  })
})
