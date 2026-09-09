import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, vi } from 'vitest'
import { Spine } from './Spine'

function emptyAgentsResponse() {
  return new Response(
    JSON.stringify({ items: [], total: 0, page: 1, pageSize: 200, totalPages: 0 }),
    { status: 200 },
  )
}

function renderAt(path: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Spine operator="kim" />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(emptyAgentsResponse())))
})
afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Spine', () => {
  it('renders the brand mark and the operator footer', () => {
    renderAt('/dashboard')
    expect(screen.getByText('AInsel')).toBeInTheDocument()
    expect(screen.getByText('kim')).toBeInTheDocument()
  })

  it('marks the route matching the current path as active', () => {
    renderAt('/agents')
    const agents = screen.getByRole('link', { name: /Agents/i })
    expect(agents).toHaveAttribute('aria-current', 'page')
  })

  it('marks the Docs entry as active when on /docs', () => {
    renderAt('/docs')
    const docs = screen.getByRole('link', { name: /Docs/i })
    expect(docs).toHaveAttribute('aria-current', 'page')
  })

  it('marks the Docs entry as active when on /docs/mcp', () => {
    renderAt('/docs/mcp')
    const docs = screen.getByRole('link', { name: /Docs/i })
    expect(docs).toHaveAttribute('aria-current', 'page')
  })

  it('renders all nav items', () => {
    renderAt('/dashboard')
    const expected = [
      'Dashboard',
      'Activity',
      'Observability',
      'Agents',
      'Connectors',
      'Docs',
      'MCPs',
      'Skills',
    ]
    for (const name of expected) {
      expect(screen.getByRole('link', { name: new RegExp(name, 'i') })).toBeInTheDocument()
    }
  })

  it('shows the three most recently updated agents under Agents', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              items: [
                { id: 'a-old', name: 'ancient-agent', updatedAt: '2026-01-01T00:00:00Z' },
                { id: 'a-1', name: 'nightly-sweeper', updatedAt: '2026-09-01T00:00:00Z' },
                { id: 'a-2', name: 'pr-reviewer', updatedAt: '2026-09-08T00:00:00Z' },
                { id: 'a-3', name: 'issue-triager', updatedAt: '2026-09-09T00:00:00Z' },
                { id: 'a-4', name: 'doc-writer', updatedAt: '2026-09-07T00:00:00Z' },
              ],
              total: 5,
              page: 1,
              pageSize: 200,
              totalPages: 1,
            }),
            { status: 200 },
          ),
        ),
      ),
    )

    renderAt('/agents')
    const newest = await screen.findByRole('link', { name: /issue-triager/i })
    expect(newest).toHaveAttribute('href', '/agents/a-3')
    expect(screen.getByRole('link', { name: /pr-reviewer/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /doc-writer/i })).toBeInTheDocument()
    // Only the three most recent make the cut.
    expect(
      screen.queryByRole('link', { name: /nightly-sweeper/i }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: /ancient-agent/i }),
    ).not.toBeInTheDocument()
  })

  it('renders no recent agents when the user has none', async () => {
    const { container } = renderAt('/dashboard')
    // Give the agents query a tick to resolve before asserting.
    await screen.findByRole('link', { name: /agents/i })
    await waitFor(() => expect(container.querySelector('.nav-recent')).toBeNull())
  })

  it('does not render Error Log in nav (redirected to /observability/errors)', () => {
    renderAt('/dashboard')
    expect(screen.queryByRole('link', { name: /Error Log/i })).toBeNull()
  })

  it('no longer renders the Triggers entry (moved under Agent Detail)', () => {
    renderAt('/dashboard')
    expect(screen.queryByRole('link', { name: /^Triggers$/i })).toBeNull()
  })
})