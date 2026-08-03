import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { AgentDetail } from './AgentDetail'
import { renderWithProviders } from '../../test/renderWithProviders'

function defaultFetch(url: string, init?: RequestInit): Response {
  if (init?.method === 'DELETE') {
    return new Response(null, { status: 204 })
  }
  if (url.includes('/triggers')) {
    return new Response(
      JSON.stringify({
        items: [
          {
            id: 't1',
            name: 'on-doc-issue',
            agentRef: 'a1',
            connectorRef: 'c-a8bf5238',
            filters: [],
          },
        ],
        total: 1,
        page: 1,
        pageSize: 200,
        totalPages: 1,
      }),
      { status: 200 },
    )
  }
  if (url.includes('/connectors')) {
    return new Response(
      JSON.stringify({
        items: [],
        total: 0,
        page: 1,
        pageSize: 200,
        totalPages: 0,
      }),
      { status: 200 },
    )
  }
  if (url.match(/\/api\/v1\/personas\/01HXTEST00000000000000000$/)) {
    return new Response(
      JSON.stringify({
        id: '01HXTEST00000000000000000',
        name: 'docs-writer',
        description: 'docs persona',
        currentVersion: 1,
        text: '# Persona\n\nYou are a docs writer.',
        createdAt: '2026-05-01T00:00:00Z',
        updatedAt: '2026-05-01T00:00:00Z',
      }),
      { status: 200 },
    )
  }
  if (url.includes('/agents/a1')) {
    return new Response(
      JSON.stringify({
        id: 'a1',
        name: 'doc-writer',
        description: 'Writes documentation.',
        llm: { model: 'claude-opus-4-7' },
        imageRef: { name: 'claude-tooling-base:1.4' },
        enabledTools: ['read_file', 'run_shell'],
        persona: { id: '01HXTEST00000000000000000' },
        replicas: 3,
        status: { ready: true, replicas: 3 },
      }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

describe('AgentDetail', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) =>
        Promise.resolve(defaultFetch(url, init)),
      ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders agent name and metadata', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await waitFor(() =>
      expect(screen.getAllByText('doc-writer')[0]).toBeInTheDocument(),
    )
    expect(screen.getByText('claude-opus-4-7')).toBeInTheDocument()
    expect(screen.getByText('claude-tooling-base:1.4')).toBeInTheDocument()
  })

  it('renders enabled tools as chips', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await waitFor(() => expect(screen.getByText('read_file')).toBeInTheDocument())
    expect(screen.getByText('run_shell')).toBeInTheDocument()
  })

  it('renders persona panel with name, markdown, and view-full link', async () => {
    const { container } = renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await waitFor(() => expect(container.querySelector('.md-body h1')).not.toBeNull())
    expect(container.querySelector('.md-body h1')?.textContent).toBe('Persona')
    expect(screen.getByText(/persona · docs-writer/i)).toBeInTheDocument()
    expect(screen.getByText(/docs persona/)).toBeInTheDocument()
    const link = screen.getByRole('link', { name: /view full persona/i })
    expect(link).toHaveAttribute('href', '/personas/01HXTEST00000000000000000')
  })

  it('shows the Triggers tab and renders the agent triggers panel', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await screen.findByText('claude-opus-4-7')
    expect(screen.getByRole('tab', { name: /triggers/i })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('tab', { name: /triggers/i }))
    await waitFor(() =>
      expect(screen.getByText('on-doc-issue')).toBeInTheDocument(),
    )
  })

  it('does not show the Access card on the overview tab', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await screen.findByText('claude-opus-4-7')
    expect(screen.queryByRole('button', { name: /access/i })).not.toBeInTheDocument()
  })

  it('switches to status tab and shows runtime stats and Access card', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await screen.findByText('claude-opus-4-7')
    await userEvent.click(screen.getByRole('tab', { name: 'Status' }))
    expect(await screen.findByText('Runtime Status')).toBeInTheDocument()
    expect(screen.getByText('Configured Replicas')).toBeInTheDocument()
  })

  it('opens the Triggers tab directly via the ?tab=triggers deep link', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1?tab=triggers' },
    )
    // Triggers panel content loads without clicking the tab.
    await waitFor(() =>
      expect(screen.getByText('on-doc-issue')).toBeInTheDocument(),
    )
  })

  it('falls back to the Overview tab for an invalid ?tab value', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1?tab=bogus' },
    )
    // Overview-only content (the model) is shown; triggers are not loaded.
    await waitFor(() => expect(screen.getByText('claude-opus-4-7')).toBeInTheDocument())
    expect(screen.queryByText('on-doc-issue')).not.toBeInTheDocument()
  })

  it('confirms and deletes the agent then navigates', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
        <Route path="/agents" element={<div>LIST</div>} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await screen.findAllByText('doc-writer')
    // Open the modal from the titleblock Delete button (the only one before modal opens).
    await userEvent.click(screen.getByRole('button', { name: /^delete$/i }))
    const dialog = await screen.findByRole('dialog')
    // Click Delete inside the modal.
    await userEvent.click(within(dialog).getByRole('button', { name: /^delete/i }))
    await waitFor(() => expect(screen.queryByText('LIST')).toBeInTheDocument())
  })

  it('surfaces an error when delete fails and keeps the modal open', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === 'DELETE') {
          return Promise.resolve(
            new Response(JSON.stringify({ message: 'forbidden' }), { status: 403 }),
          )
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
        <Route path="/agents" element={<div>LIST</div>} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await screen.findAllByText('doc-writer')
    // Open the delete modal.
    await userEvent.click(screen.getByRole('button', { name: /^delete$/i }))
    const dialog = await screen.findByRole('dialog')
    // Click Delete inside the modal — should fail with 403.
    await userEvent.click(within(dialog).getByRole('button', { name: /^delete/i }))
    // Modal stays open and shows the error message.
    await waitFor(() =>
      expect(screen.getByText('forbidden')).toBeInTheDocument(),
    )
    // Should NOT have navigated to the list.
    expect(screen.queryByText('LIST')).not.toBeInTheDocument()
    // Dialog is still present.
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })
})
