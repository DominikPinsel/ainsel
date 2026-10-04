import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import { Observability } from './Observability'
import { renderWithProviders } from '../../test/renderWithProviders'

function timeseriesPayload(metric: string) {
  return {
    metric,
    range: '24h',
    step: '1h',
    points: Array.from({ length: 24 }, (_, i) => ({
      timestamp: new Date(Date.now() - (23 - i) * 3600_000).toISOString(),
      value: 10 + i,
    })),
  }
}

function defaultFetch(url: string): Response {
  if (url.includes('/observability/metrics/summary')) {
    return new Response(
      JSON.stringify({
        eventsConsumed: 1234,
        triggersMatched: 1100,
        eventsRouted: 1050,
        routingErrors: 3,
        updatedAt: '2026-05-21T00:00:00Z',
      }),
      { status: 200 },
    )
  }
  if (url.includes('/observability/metrics/tokens/summary')) {
    return new Response(
      JSON.stringify({
        totalTokens: 1_240_000,
        inputTokens: 820_000,
        outputTokens: 420_000,
      }),
      { status: 200 },
    )
  }
  if (url.includes('/observability/metrics/tokens/by-subject')) {
    return new Response(
      JSON.stringify({
        range: '24h',
        rows: [
          {
            agent: 'doc-writer',
            agentName: 'Doc Writer',
            repo: 'k-weber/insel',
            eventType: 'pull_request.opened',
            model: 'gpt-4',
            inputTokens: 500_000,
            outputTokens: 300_000,
            totalTokens: 800_000,
          },
          {
            agent: 'triage-bot',
            agentName: 'Triage Bot',
            repo: 'k-weber/insel',
            model: 'claude-3',
            inputTokens: 200_000,
            outputTokens: 100_000,
            totalTokens: 300_000,
          },
        ],
      }),
      { status: 200 },
    )
  }
  if (url.includes('/observability/metrics/timeseries')) {
    const m =
      new URL(url, 'http://x/').searchParams.get('metric') ?? 'events_routed'
    return new Response(JSON.stringify(timeseriesPayload(m)), { status: 200 })
  }
  if (url.includes('/observability/logs')) {
    const logs = [
      {
        timestamp: new Date(Date.now() - 30_000).toISOString(),
        message: 'request handled',
        agentName: 'Doc Writer',
        labels: { app: 'hub', agent: 'doc-writer', level: 'info' },
      },
    ]
    return new Response(
      JSON.stringify({ logs, total: logs.length, query: '' }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

describe('Observability', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn((url: string) => Promise.resolve(defaultFetch(url))))
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders summary cards with token total', async () => {
    renderWithProviders(<Observability />, { route: '/observability' })
    await waitFor(() => expect(screen.getByText('1.2K')).toBeInTheDocument())
    expect(screen.getByText('1.2M')).toBeInTheDocument() // token total
  })

  it('renders the chart svg with 4 series paths', async () => {
    const { container } = renderWithProviders(<Observability />, {
      route: '/observability',
    })
    await waitFor(() => {
      const paths = container.querySelectorAll('svg path')
      expect(paths.length).toBe(4)
    })
  })

  it('renders the tokens table with rows sorted by total descending', async () => {
    renderWithProviders(<Observability />, { route: '/observability' })
    await waitFor(() =>
      expect(screen.getByRole('cell', { name: 'Doc Writer' })).toBeInTheDocument(),
    )
    expect(screen.getByRole('cell', { name: 'Triage Bot' })).toBeInTheDocument()
  })

  it('changes URL when a new range is selected', async () => {
    let currentSearch = ''
    function Spy() {
      currentSearch = useLocation().search
      return null
    }
    function wrap(children: ReactNode) {
      return renderWithProviders(
        <Routes>
          <Route
            path="/observability"
            element={
              <>
                {children}
                <Spy />
              </>
            }
          />
        </Routes>,
        { route: '/observability' },
      )
    }
    wrap(<Observability />)
    await waitFor(() =>
      expect(screen.getByRole('cell', { name: 'Doc Writer' })).toBeInTheDocument(),
    )
    await userEvent.click(screen.getByRole('button', { name: '7d' }))
    await waitFor(() => expect(currentSearch).toContain('range=7d'))
  })

  it('renders range suffix on summary card labels', async () => {
    renderWithProviders(<Observability />, { route: '/observability?range=6h' })
    await waitFor(() => expect(screen.getByText('1.2K')).toBeInTheDocument())
    // All five cards should carry the range suffix.
    expect(screen.getByText(/Events Consumed · 6h/)).toBeInTheDocument()
    expect(screen.getByText(/Triggers Matched · 6h/)).toBeInTheDocument()
    expect(screen.getByText(/Events Routed · 6h/)).toBeInTheDocument()
    expect(screen.getByText(/Errors · 6h/)).toBeInTheDocument()
    expect(screen.getByText(/Tokens · 6h/)).toBeInTheDocument()
  })

  it('falls back to unavailable when summary returns 503', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/observability/metrics/summary')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                error: 'no metrics backend: set observability.prometheus.url',
              }),
              { status: 503, headers: { 'Content-Type': 'application/json' } },
            ),
          )
        }
        return Promise.resolve(defaultFetch(url))
      }),
    )
    renderWithProviders(<Observability />, { route: '/observability' })
    await waitFor(() =>
      expect(screen.getByText('No metrics source configured')).toBeInTheDocument(),
    )
    // The hub's reason is shown, not a claim about telemetry in general.
    expect(
      screen.getByText('no metrics backend: set observability.prometheus.url'),
    ).toBeInTheDocument()
  })

  it('keeps the event cards when only the token queries return 503', async () => {
    // A standalone hub has no Prometheus: the event metrics come from its own
    // records and only the token panels have nothing to show.
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/metrics/tokens/')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({ error: 'prometheus not configured' }),
              { status: 503, headers: { 'Content-Type': 'application/json' } },
            ),
          )
        }
        return Promise.resolve(defaultFetch(url))
      }),
    )
    renderWithProviders(<Observability />, { route: '/observability' })

    // Event cards come from the hub's own records, so they render. The routed
    // and matched figures are both 1.1K here, so count tiles rather than match
    // one by name.
    await waitFor(() => expect(screen.getByText('1.2K')).toBeInTheDocument())
    expect(screen.getAllByText('1.1K').length).toBe(2)
    // The token tile has nothing to show, and says which backend is missing.
    expect(screen.queryByText('1.2M')).not.toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getByText('Token metrics need Prometheus')).toBeInTheDocument(),
    )
    expect(screen.getByText(/Token usage needs Prometheus/)).toBeInTheDocument()
  })
})