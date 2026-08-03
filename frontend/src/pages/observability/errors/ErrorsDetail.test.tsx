import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import { ErrorsDetail } from './ErrorsDetail'
import { renderWithProviders } from '../../../test/renderWithProviders'

const ERRORS = [
  {
    id: 'err-1',
    timestamp: new Date(Date.now() - 60_000).toISOString(),
    severity: 'error',
    source: 'router',
    message: 'No route matched',
  },
  {
    id: 'err-2',
    timestamp: new Date(Date.now() - 120_000).toISOString(),
    severity: 'warning',
    source: 'connector',
    message: 'Slow response',
  },
]

function defaultFetch(url: string): Response {
  if (url.includes('/errors')) {
    return new Response(
      JSON.stringify({ errors: ERRORS, total: ERRORS.length }),
      { status: 200 },
    )
  }
  if (url.includes('/observability/metrics/timeseries')) {
    return new Response(
      JSON.stringify({ metric: 'routing_errors', range: '24h', points: [] }),
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

describe('ErrorsDetail', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn((url: string) => Promise.resolve(defaultFetch(url))))
  })
  afterEach(() => { vi.unstubAllGlobals() })

  it('renders the page heading', () => {
    renderWithProviders(<ErrorsDetail />, { route: '/observability/errors' })
    expect(screen.getByRole('heading', { name: /error\s+detail/i })).toBeInTheDocument()
  })

  it('renders the breadcrumb link to Observability', () => {
    renderWithProviders(<ErrorsDetail />, { route: '/observability/errors' })
    expect(screen.getByRole('link', { name: /observability/i })).toBeInTheDocument()
  })

  it('renders error rows from API', async () => {
    renderWithProviders(<ErrorsDetail />, { route: '/observability/errors' })
    await waitFor(() => expect(screen.getByText('No route matched')).toBeInTheDocument())
    expect(screen.getByText('Slow response')).toBeInTheDocument()
  })

  it('filters by severity', async () => {
    renderWithProviders(<ErrorsDetail />, { route: '/observability/errors?severity=error' })
    await waitFor(() => expect(screen.getByText('No route matched')).toBeInTheDocument())
    expect(screen.queryByText('Slow response')).toBeNull()
  })

  it('filters by source', async () => {
    renderWithProviders(<ErrorsDetail />, { route: '/observability/errors?source=connector' })
    await waitFor(() => expect(screen.getByText('Slow response')).toBeInTheDocument())
    expect(screen.queryByText('No route matched')).toBeNull()
  })

  it('shows clear button when filter is active', async () => {
    renderWithProviders(<ErrorsDetail />, { route: '/observability/errors?severity=error' })
    await waitFor(() => expect(screen.getByRole('button', { name: /clear/i })).toBeInTheDocument())
  })

  it('hides clear button when no filter is active', async () => {
    renderWithProviders(<ErrorsDetail />, { route: '/observability/errors' })
    await waitFor(() => expect(screen.getByText('No route matched')).toBeInTheDocument())
    expect(screen.queryByRole('button', { name: /clear/i })).toBeNull()
  })

  it('clears filters when clear button clicked', async () => {
    const { getSearch } = withSpy(<ErrorsDetail />, '/observability/errors?severity=error')
    await waitFor(() => expect(screen.getByRole('button', { name: /clear/i })).toBeInTheDocument())
    await userEvent.click(screen.getByRole('button', { name: /clear/i }))
    await waitFor(() => expect(getSearch()).not.toContain('severity'))
  })

  it('renders view events links', async () => {
    renderWithProviders(<ErrorsDetail />, { route: '/observability/errors' })
    await waitFor(() => {
      const links = screen.getAllByText(/view events →/i)
      expect(links.length).toBeGreaterThan(0)
    })
  })

  it('view events link points to events page with error status', async () => {
    renderWithProviders(<ErrorsDetail />, { route: '/observability/errors' })
    await waitFor(() => {
      const link = screen.getAllByText(/view events →/i)[0]
      expect(link.closest('a')).toHaveAttribute('href', expect.stringContaining('/observability/events'))
      expect(link.closest('a')).toHaveAttribute('href', expect.stringContaining('status=error'))
    })
  })
})
