import { render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, vi } from 'vitest'
import { Spine } from './Spine'
import { recordAgentView } from '../agentRecents'

function emptyAgentsResponse() {
  return new Response(
    JSON.stringify({ items: [], total: 0, page: 1, pageSize: 200, totalPages: 0 }),
    { status: 200 },
  )
}

function renderAt(path: string, scope = 'anon') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0, staleTime: 0 } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <Spine operator="kim" recentsScope={scope} />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function agentsResponse(items: unknown[]) {
  return new Response(
    JSON.stringify({
      items,
      total: items.length,
      page: 1,
      pageSize: 200,
      totalPages: 1,
    }),
    { status: 200 },
  )
}

beforeEach(() => {
  // Recents live in localStorage, which survives between tests in a file.
  localStorage.clear()
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
      'Channels',
      'Activity',
      'Observability',
      'Agents',
      'Images',
      'Personas',
      'Connectors',
      'Docs',
      'MCPs',
      'Skills',
    ]
    for (const name of expected) {
      expect(screen.getByRole('link', { name: new RegExp(name, 'i') })).toBeInTheDocument()
    }
  })

  it('groups the shared catalogs under Library', () => {
    renderAt('/dashboard')

    const section = screen.getByText('Library').closest('div')?.parentElement
    expect(section).not.toBeNull()
    for (const name of ['Images', 'Personas', 'Skills', 'MCPs']) {
      expect(
        within(section as HTMLElement).getByRole('link', { name: new RegExp(name, 'i') }),
      ).toBeInTheDocument()
    }

    // The old Setup section is gone: its entries moved into Library.
    expect(screen.queryByText('Setup')).not.toBeInTheDocument()
  })

  it('links the library entries to their catalog routes', () => {
    renderAt('/dashboard')
    expect(screen.getByRole('link', { name: /Images/i })).toHaveAttribute(
      'href',
      '/agent-images',
    )
    expect(screen.getByRole('link', { name: /Personas/i })).toHaveAttribute(
      'href',
      '/personas',
    )
  })

  it('pads to five with the most recently updated agents when nothing was clicked', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          agentsResponse([
            { id: 'a-old', name: 'ancient-agent', updatedAt: '2026-01-01T00:00:00Z' },
            { id: 'a-1', name: 'nightly-sweeper', updatedAt: '2026-09-01T00:00:00Z' },
            { id: 'a-2', name: 'pr-reviewer', updatedAt: '2026-09-08T00:00:00Z' },
            { id: 'a-3', name: 'issue-triager', updatedAt: '2026-09-09T00:00:00Z' },
            { id: 'a-4', name: 'doc-writer', updatedAt: '2026-09-07T00:00:00Z' },
            { id: 'a-5', name: 'least-relevant', updatedAt: '2025-01-01T00:00:00Z' },
          ]),
        ),
      ),
    )

    renderAt('/agents')
    const newest = await screen.findByRole('link', { name: /issue-triager/i })
    expect(newest).toHaveAttribute('href', '/agents/a-3')
    expect(screen.getByRole('link', { name: /pr-reviewer/i })).toBeInTheDocument()
    // Five now, not three: the two oldest of the six get cut.
    expect(screen.getByRole('link', { name: /nightly-sweeper/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /ancient-agent/i })).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: /least-relevant/i }),
    ).not.toBeInTheDocument()
    // None of these were clicked, so all of them are filler.
    expect(document.querySelectorAll('.nav-sublink.padded')).toHaveLength(5)
  })

  it('lists agents this user clicked before the recently updated ones', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          agentsResponse([
            { id: 'a-1', name: 'nightly-sweeper', updatedAt: '2026-09-01T00:00:00Z' },
            { id: 'a-2', name: 'pr-reviewer', updatedAt: '2026-09-08T00:00:00Z' },
            { id: 'a-3', name: 'issue-triager', updatedAt: '2026-09-09T00:00:00Z' },
          ]),
        ),
      ),
    )
    // Click order: pr-reviewer, then nightly-sweeper. nightly-sweeper is the
    // *oldest* by updatedAt, so it only comes out on top if the click history
    // is what ordered this list — plain updatedAt would put issue-triager
    // first and would not be distinguishable from a coincidence.
    recordAgentView('kim', 'a-2')
    recordAgentView('kim', 'a-1')

    renderAt('/agents', 'kim')
    await screen.findByRole('link', { name: /nightly-sweeper/i })
    const links = Array.from(
      document.querySelectorAll('.nav-recent .nav-sublink .name'),
    ).map((el) => el.textContent)
    // issue-triager is unclicked but newest, so it pads the last slot.
    expect(links).toEqual(['nightly-sweeper', 'pr-reviewer', 'issue-triager'])
    // Clicked ones are theirs (not padded); the single pad slot is.
    expect(
      Array.from(document.querySelectorAll('.nav-recent .nav-sublink')).map((el) =>
        el.classList.contains('padded'),
      ),
    ).toEqual([false, false, true])
  })

  it('drops a remembered agent the hub no longer returns', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          agentsResponse([
            { id: 'a-1', name: 'nightly-sweeper', updatedAt: '2026-09-01T00:00:00Z' },
          ]),
        ),
      ),
    )
    // Deleted, or access revoked since: the id is still in storage, but the
    // list API is RBAC-filtered and must decide what renders.
    recordAgentView('kim', 'gone-agent')

    renderAt('/agents', 'kim')
    await screen.findByRole('link', { name: /nightly-sweeper/i })
    expect(
      screen.queryByRole('link', { name: /gone-agent/i }),
    ).not.toBeInTheDocument()
    expect(document.querySelectorAll('.nav-recent .nav-sublink')).toHaveLength(1)
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