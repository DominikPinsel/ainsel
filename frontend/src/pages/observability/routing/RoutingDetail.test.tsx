import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { RoutingDetail } from './RoutingDetail'
import { renderWithProviders } from '../../../test/renderWithProviders'

function timeseriesPayload(metric: string) {
  return {
    metric,
    range: '24h',
    step: '1h',
    points: Array.from({ length: 24 }, (_, i) => ({
      timestamp: new Date(Date.now() - (23 - i) * 3600_000).toISOString(),
      value: i,
    })),
  }
}

const INVOCATIONS = [
  {
    id: 'inv-1',
    timestamp: new Date(Date.now() - 60_000).toISOString(),
    agent: 'agent-a',
    agentName: 'Doc Writer',
    trigger: 'trig-1',
    triggerName: 'On PR',
    status: 'success',
    durationMs: 3200,
  },
  {
    id: 'inv-2',
    timestamp: new Date(Date.now() - 120_000).toISOString(),
    agent: 'agent-b',
    agentName: 'Triage Bot',
    trigger: 'trig-2',
    triggerName: 'On Push',
    status: 'failure',
    durationMs: 1100,
  },
  {
    id: 'inv-3',
    timestamp: new Date(Date.now() - 180_000).toISOString(),
    agent: 'agent-c',
    agentName: 'Slow Runner',
    trigger: 'trig-3',
    triggerName: 'On Schedule',
    status: 'timeout',
    durationMs: 30000,
  },
]

function defaultFetch(url: string): Response {
  if (url.includes('/invocations')) {
    return new Response(
      JSON.stringify({ invocations: INVOCATIONS, total: INVOCATIONS.length, page: 1, pageSize: 50, capacity: 1000 }),
      { status: 200 },
    )
  }
  if (url.includes('/observability/metrics/timeseries')) {
    const metric = new URL(url, 'http://x').searchParams.get('metric') ?? 'events_routed'
    return new Response(JSON.stringify(timeseriesPayload(metric)), { status: 200 })
  }
  return new Response('{}', { status: 200 })
}

describe('RoutingDetail', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn((url: string) => Promise.resolve(defaultFetch(url))))
  })
  afterEach(() => { vi.unstubAllGlobals() })

  it('renders the page heading', () => {
    renderWithProviders(<RoutingDetail />, { route: '/observability/routing' })
    expect(screen.getByRole('heading', { name: /routing\s+detail/i })).toBeInTheDocument()
  })

  it('renders the breadcrumb link to Observability', () => {
    renderWithProviders(<RoutingDetail />, { route: '/observability/routing' })
    expect(screen.getByRole('link', { name: /observability/i })).toBeInTheDocument()
  })

  it('renders invocation rows from API', async () => {
    renderWithProviders(<RoutingDetail />, { route: '/observability/routing' })
    await waitFor(() => expect(screen.getByText('Doc Writer')).toBeInTheDocument())
    expect(screen.getByText('Triage Bot')).toBeInTheDocument()
  })

  it('renders trigger names', async () => {
    renderWithProviders(<RoutingDetail />, { route: '/observability/routing' })
    await waitFor(() => expect(screen.getByText('On PR')).toBeInTheDocument())
    expect(screen.getByText('On Push')).toBeInTheDocument()
  })

  it('renders formatted duration', async () => {
    renderWithProviders(<RoutingDetail />, { route: '/observability/routing' })
    await waitFor(() => expect(screen.getByText('3.2s')).toBeInTheDocument())
  })

  it('styles a failure invocation with the error variant', async () => {
    renderWithProviders(<RoutingDetail />, { route: '/observability/routing' })
    const failureTag = await screen.findByText('failure')
    expect(failureTag).toHaveClass('err')
  })

  it('styles a success invocation with the ok variant', async () => {
    renderWithProviders(<RoutingDetail />, { route: '/observability/routing' })
    const successTag = await screen.findByText('success')
    expect(successTag).toHaveClass('ok')
  })

  it('styles a timeout invocation with the stale variant', async () => {
    renderWithProviders(<RoutingDetail />, { route: '/observability/routing' })
    const timeoutTag = await screen.findByText('timeout')
    expect(timeoutTag).toHaveClass('stale')
  })

  it('renders view events links', async () => {
    renderWithProviders(<RoutingDetail />, { route: '/observability/routing' })
    await waitFor(() => {
      const links = screen.getAllByText(/view events →/i)
      expect(links.length).toBeGreaterThan(0)
    })
  })

  it('view events link points to events page with matched status', async () => {
    renderWithProviders(<RoutingDetail />, { route: '/observability/routing' })
    await waitFor(() => {
      const link = screen.getAllByText(/view events →/i)[0]
      expect(link.closest('a')).toHaveAttribute('href', expect.stringContaining('/observability/events'))
      expect(link.closest('a')).toHaveAttribute('href', expect.stringContaining('status=matched'))
    })
  })
})
