import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { Dashboard } from './Dashboard'
import { renderWithProviders } from '../../test/renderWithProviders'

function routeResponse(url: string): Response {
  if (url.includes('/observability/metrics/summary')) {
    return new Response(
      JSON.stringify({
        eventsConsumed: 100,
        triggersMatched: 90,
        eventsRouted: 88,
        routingErrors: 3,
        updatedAt: '2026-05-21T00:00:00Z',
      }),
      { status: 200 },
    )
  }
  if (url.includes('/observability/metrics/timeseries')) {
    const points = Array.from({ length: 24 }, (_, i) => ({
      timestamp: new Date(Date.now() - (23 - i) * 3600_000).toISOString(),
      value: 10 + i * 2,
    }))
    return new Response(
      JSON.stringify({
        metric: 'events_routed',
        range: '24h',
        step: '1h',
        points,
      }),
      { status: 200 },
    )
  }
  if (url.includes('/agents')) {
    return new Response(
      JSON.stringify({ items: [], total: 14, page: 1, pageSize: 1, totalPages: 14 }),
      { status: 200 },
    )
  }
  if (url.includes('/triggers')) {
    return new Response(
      JSON.stringify({ items: [], total: 23, page: 1, pageSize: 1, totalPages: 23 }),
      { status: 200 },
    )
  }
  if (url.includes('/connectors')) {
    return new Response(
      JSON.stringify({
        items: [
          {
            id: 'c1',
            name: 'insel-monorepo',
            type: 'github',
            url: 'k-weber/insel',
            status: {
              ready: true,
              webhookRegistered: true,
              lastEventAt: new Date(Date.now() - 12_000).toISOString(),
            },
          },
          {
            id: 'c2',
            name: 'infra-tooling',
            type: 'github',
            url: 'insel-ops/infra',
            status: {
              ready: false,
              webhookRegistered: false,
              lastEventAt: new Date(Date.now() - 14 * 60_000).toISOString(),
            },
          },
        ],
        total: 2,
        page: 1,
        pageSize: 200,
        totalPages: 1,
      }),
      { status: 200 },
    )
  }
  if (url.includes('/events')) {
    const events = [
      {
        id: 'e1',
        timestamp: new Date(Date.now() - 9_000).toISOString(),
        connector: 'insel-monorepo',
        status: 'matched',
        matches: [{ trigger: 't1', agent: 'doc-writer' }],
      },
      {
        id: 'e2',
        timestamp: new Date(Date.now() - 4 * 60_000).toISOString(),
        connector: 'infra-tooling',
        status: 'error',
      },
    ]
    return new Response(
      JSON.stringify({ events, total: events.length }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

describe('Dashboard', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => Promise.resolve(routeResponse(url))),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders KPI counts from queries', async () => {
    renderWithProviders(<Dashboard />)
    await waitFor(() => expect(screen.getByText('14')).toBeInTheDocument())
    expect(screen.getByText('23')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('marks the errors plate as alerting when errors > 0', async () => {
    renderWithProviders(<Dashboard />)
    await waitFor(() => expect(screen.getByText('3')).toBeInTheDocument())
    const errorsPlate = screen.getByText('3').closest('.kpi')
    expect(errorsPlate).toHaveClass('alert')
  })

  it('renders the connector register with both connectors', async () => {
    renderWithProviders(<Dashboard />)
    // The register renders connector names as <a> links; the activity stream
    // renders them as <b>. Scope to the link form to disambiguate.
    await waitFor(() => expect(screen.getByRole('link', { name: 'insel-monorepo' })).toBeInTheDocument())
    expect(screen.getByRole('link', { name: 'infra-tooling' })).toBeInTheDocument()
  })

  it('marks a not-ready connector row with row-err class', async () => {
    renderWithProviders(<Dashboard />)
    const link = await screen.findByRole('link', { name: 'infra-tooling' })
    const row = link.closest('tr')
    expect(row).toHaveClass('row-err')
  })

  it('renders the recent activity stream entries', async () => {
    renderWithProviders(<Dashboard />)
    await waitFor(() => expect(screen.getByText('MATCH')).toBeInTheDocument())
    expect(screen.getByText('ERR')).toBeInTheDocument()
  })

  it('renders the throughput chart SVG', async () => {
    const { container } = renderWithProviders(<Dashboard />)
    await waitFor(() => {
      expect(
        container.querySelector('svg[aria-label="Throughput over 24 hours"]'),
      ).toBeInTheDocument()
    })
    const bars = container.querySelectorAll('svg rect')
    expect(bars.length).toBeGreaterThanOrEqual(24)
  })

  // The throughput panel is the one place outside /observability that shows the
  // hub's own 503 reason. `unavailableDetail` is the rule that decides which
  // failures get to say something, so both branches are pinned here: a 503
  // quotes the hub, anything else stays silent rather than passing a raw
  // upstream error body through to the console.
  //
  // Both cases look inside the throughput panel's own <section> rather than the
  // whole page. Today it is the only Dashboard child that renders a
  // `.ss-detail`, so a page-wide query happens to be exact — but a sibling that
  // grows one later would otherwise fail these tests for a reason unrelated to
  // the rule they pin.
  function throughputPanel(): HTMLElement {
    const panel = screen.getByText('Throughput · 24h').closest('.panel')
    expect(panel).not.toBeNull()
    return panel as HTMLElement
  }

  it('shows the hub reason when the throughput query returns 503', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/observability/metrics/timeseries')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                error: 'prometheus not configured: set observability.prometheus.url',
              }),
              { status: 503, headers: { 'Content-Type': 'application/json' } },
            ),
          )
        }
        return Promise.resolve(routeResponse(url))
      }),
    )
    renderWithProviders(<Dashboard />)
    await waitFor(() =>
      expect(screen.getByText('No metrics source configured')).toBeInTheDocument(),
    )
    expect(throughputPanel().querySelector('.ss-detail')).toHaveTextContent(
      'prometheus not configured: set observability.prometheus.url',
    )
  })

  it('shows no detail line for a throughput failure that is not a 503', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/observability/metrics/timeseries')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({ error: 'query failed: context deadline exceeded' }),
              { status: 500, headers: { 'Content-Type': 'application/json' } },
            ),
          )
        }
        return Promise.resolve(routeResponse(url))
      }),
    )
    renderWithProviders(<Dashboard />)
    await waitFor(() =>
      expect(screen.getByText('Failed to load throughput')).toBeInTheDocument(),
    )
    expect(
      screen.queryByText('query failed: context deadline exceeded'),
    ).not.toBeInTheDocument()
    expect(throughputPanel().querySelector('.ss-detail')).toBeNull()
  })
})
