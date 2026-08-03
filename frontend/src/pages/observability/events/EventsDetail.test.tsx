import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import { EventsDetail } from './EventsDetail'
import { renderWithProviders } from '../../../test/renderWithProviders'

function timeseriesPayload() {
  return {
    metric: 'events_consumed',
    range: '24h',
    step: '1h',
    points: Array.from({ length: 24 }, (_, i) => ({
      timestamp: new Date(Date.now() - (23 - i) * 3600_000).toISOString(),
      value: 5 + i,
    })),
  }
}

const EVENTS = [
  {
    id: 'ev-1',
    timestamp: new Date(Date.now() - 60_000).toISOString(),
    connector: 'conn-1',
    status: 'matched',
    payload: { action: 'opened' },
    matches: [{ trigger: 'on-pr', agent: 'doc-writer' }],
  },
  {
    id: 'ev-2',
    timestamp: new Date(Date.now() - 120_000).toISOString(),
    connector: 'conn-1',
    status: 'error',
    payload: { action: 'closed' },
    matches: [],
  },
]

function defaultFetch(url: string): Response {
  if (url.includes('/observability/metrics/timeseries')) {
    return new Response(JSON.stringify(timeseriesPayload()), { status: 200 })
  }
  if (url.includes('/observability/metrics/tokens/by-event')) {
    return new Response(
      JSON.stringify({
        range: '24h',
        rows: [{ event: 'ev-1', inputTokens: 100, outputTokens: 50, totalTokens: 150 }],
      }),
      { status: 200 },
    )
  }
  // match /events exactly (not timeseries query string containing "events")
  if (/\/events(\?|$)/.test(url)) {
    return new Response(
      JSON.stringify({ events: EVENTS, total: EVENTS.length }),
      { status: 200 },
    )
  }
  if (url.includes('/connectors')) {
    return new Response(
      JSON.stringify({ items: [{ id: 'conn-1', name: 'GitHub Connector' }], total: 1 }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

function withSpy(children: ReactNode, route: string) {
  let search = ''
  function Spy() { search = useLocation().search; return null }
  const result = renderWithProviders(
    <Routes>
      <Route path="*" element={<>{children}<Spy /></>} />
    </Routes>,
    { route },
  )
  return { ...result, getSearch: () => search }
}

describe('EventsDetail', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn((url: string) => Promise.resolve(defaultFetch(url))))
  })
  afterEach(() => { vi.unstubAllGlobals() })

  it('renders the page heading', () => {
    renderWithProviders(<EventsDetail />, { route: '/observability/events' })
    expect(screen.getByRole('heading', { name: /events\s+detail/i })).toBeInTheDocument()
  })

  it('renders the breadcrumb link to Observability', () => {
    renderWithProviders(<EventsDetail />, { route: '/observability/events' })
    expect(screen.getByRole('link', { name: /^observability$/i })).toBeInTheDocument()
  })

  it('renders the range selector buttons', () => {
    renderWithProviders(<EventsDetail />, { route: '/observability/events' })
    expect(screen.getByRole('button', { name: '1h' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '7d' })).toBeInTheDocument()
  })

  it('renders event rows from API data', async () => {
    renderWithProviders(<EventsDetail />, { route: '/observability/events' })
    await waitFor(() => expect(screen.getByText('on-pr')).toBeInTheDocument())
    expect(screen.getByText('doc-writer')).toBeInTheDocument()
  })

  it('filters events by status param', async () => {
    renderWithProviders(<EventsDetail />, { route: '/observability/events?status=matched' })
    await waitFor(() => expect(screen.getByText('on-pr')).toBeInTheDocument())
    // ev-2 has status=error and payload action=closed; its content should be absent
    expect(screen.queryByText('ev-2')).toBeNull()
  })

  it('sets range in URL when range button clicked', async () => {
    const { getSearch } = withSpy(<EventsDetail />, '/observability/events')
    await userEvent.click(screen.getByRole('button', { name: '7d' }))
    await waitFor(() => expect(getSearch()).toContain('range=7d'))
  })

  it('panel title shows just count when unfiltered', async () => {
    renderWithProviders(<EventsDetail />, { route: '/observability/events' })
    await waitFor(() =>
      expect(screen.getByText(`Events · ${EVENTS.length}`)).toBeInTheDocument()
    )
  })

  it('filters events by agent param', async () => {
    renderWithProviders(<EventsDetail />, { route: '/observability/events?agent=doc-writer' })
    await waitFor(() => expect(screen.getByText('on-pr')).toBeInTheDocument())
    // ev-2 has no matches, so it should be filtered out
    expect(screen.queryByText('closed')).toBeNull()
  })

  it('shows agent filter chip when agent param is present', async () => {
    renderWithProviders(<EventsDetail />, { route: '/observability/events?agent=doc-writer' })
    await waitFor(() => expect(screen.getByText('doc-writer')).toBeInTheDocument())
  })

  it('panel title shows filtered-of-total when status filter active', async () => {
    renderWithProviders(<EventsDetail />, { route: '/observability/events?status=matched' })
    await waitFor(() =>
      expect(screen.getByText(`Events · 1 of ${EVENTS.length}`)).toBeInTheDocument()
    )
  })

  it('renders a Tokens column header', async () => {
    renderWithProviders(<EventsDetail />, { route: '/observability/events' })
    await waitFor(() => expect(screen.getByText('on-pr')).toBeInTheDocument())
    expect(screen.getByRole('columnheader', { name: /tokens/i })).toBeInTheDocument()
  })

  it('shows token total for events with token data', async () => {
    renderWithProviders(<EventsDetail />, { route: '/observability/events' })
    await waitFor(() => expect(screen.getByText('150')).toBeInTheDocument())
  })

  it('renders 6 cells per row even when event has no token data', async () => {
    // ev-2 has no entry in the by-event token map, so tokensMap.get('ev-2') is
    // undefined. The row must still render 6 <td>s (with "—" for tokens) to
    // stay aligned under the 6-column header.
    renderWithProviders(<EventsDetail />, { route: '/observability/events' })
    await waitFor(() => expect(screen.getByText('on-pr')).toBeInTheDocument())
    // ev-2 has status='error' which renders as "ERR" tag, and no matches so
    // triggers/agents columns show "—". Find the row containing the ERR tag.
    const errTags = screen.getAllByText('ERR')
    expect(errTags.length).toBeGreaterThan(0)
    const row = errTags[0].closest('tr')
    expect(row).not.toBeNull()
    const cells = row!.querySelectorAll('td')
    expect(cells.length).toBe(6)
    // The last cell should show "—" (no token data for ev-2, rendered as 0)
    expect(cells[5].textContent).toBe('—')
  })
})
