import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { TokensDetail } from './TokensDetail'
import { renderWithProviders } from '../../../test/renderWithProviders'

const TOKEN_ROWS = [
  {
    agent: 'doc-writer',
    repo: 'AInsel/ainsel',
    eventType: 'pull_request.opened',
    model: 'kimi-k2.6',
    inputTokens: 500_000,
    outputTokens: 50_000,
    totalTokens: 550_000,
  },
  {
    agent: 'triage-bot',
    repo: 'AInsel/ainsel',
    model: 'kimi-k2.6',
    inputTokens: 200_000,
    outputTokens: 40_000,
    totalTokens: 240_000,
  },
]

function defaultFetch(url: string): Response {
  if (url.includes('/tokens/summary')) {
    return new Response(
      JSON.stringify({ totalTokens: 1_000_000, inputTokens: 700_000, outputTokens: 300_000 }),
      { status: 200 },
    )
  }
  if (url.includes('/tokens/by-subject')) {
    return new Response(
      JSON.stringify({ range: '24h', rows: TOKEN_ROWS }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

describe('TokensDetail', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn((url: string) => Promise.resolve(defaultFetch(url))))
  })
  afterEach(() => { vi.unstubAllGlobals() })

  it('renders the page heading', () => {
    renderWithProviders(<TokensDetail />, { route: '/observability/tokens' })
    expect(screen.getByRole('heading', { name: /token\s+detail/i })).toBeInTheDocument()
  })

  it('renders the breadcrumb link to Observability', () => {
    renderWithProviders(<TokensDetail />, { route: '/observability/tokens' })
    expect(screen.getByRole('link', { name: /observability/i })).toBeInTheDocument()
  })

  it('renders KPI cards for total, input and output', async () => {
    renderWithProviders(<TokensDetail />, { route: '/observability/tokens' })
    await waitFor(() => expect(screen.getByText('1M')).toBeInTheDocument())
    expect(screen.getByText('Total Tokens (all time)')).toBeInTheDocument()
  })

  it('renders token rows sorted by total descending', async () => {
    renderWithProviders(<TokensDetail />, { route: '/observability/tokens' })
    await waitFor(() => expect(screen.getByText('doc-writer')).toBeInTheDocument())
    expect(screen.getByText('triage-bot')).toBeInTheDocument()
    const cells = screen.getAllByText(/doc-writer|triage-bot/)
    expect(cells[0].textContent).toBe('doc-writer')
  })

  it('renders I/O ratio column', async () => {
    renderWithProviders(<TokensDetail />, { route: '/observability/tokens' })
    await waitFor(() => expect(screen.getByText('doc-writer')).toBeInTheDocument())
    expect(screen.getByText('10.0×')).toBeInTheDocument()
  })

  it('renders view events links', async () => {
    renderWithProviders(<TokensDetail />, { route: '/observability/tokens' })
    await waitFor(() => {
      const links = screen.getAllByText(/view events →/i)
      expect(links.length).toBeGreaterThan(0)
    })
  })

  it('view events link includes agent and range in URL', async () => {
    renderWithProviders(<TokensDetail />, { route: '/observability/tokens' })
    await waitFor(() => {
      const link = screen.getAllByText(/view events →/i)[0]
      const href = link.closest('a')?.getAttribute('href') ?? ''
      expect(href).toContain('/observability/events')
      expect(href).toContain('agent=doc-writer')
    })
  })
})
