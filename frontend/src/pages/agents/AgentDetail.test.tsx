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
  const personaMatch = url.match(/\/api\/v1\/personas\/([^/?]+)$/)
  if (personaMatch) {
    const id = personaMatch[1]
    const alt = id.endsWith('1')
    return new Response(
      JSON.stringify({
        id,
        name: alt ? 'ops-triager' : 'docs-writer',
        description: 'docs persona',
        currentVersion: 1,
        text: alt
          ? '# Persona\n\nYou triage issues.'
          : '# Persona\n\nYou are a docs writer.',
        createdAt: '2026-05-01T00:00:00Z',
        updatedAt: '2026-05-01T00:00:00Z',
      }),
      { status: 200 },
    )
  }
  if (url.includes('/personas')) {
    return new Response(
      JSON.stringify({
        items: [
          {
            id: '01HXTEST00000000000000000',
            name: 'docs-writer',
            description: 'docs persona',
            currentVersion: 1,
            createdAt: '2026-05-01T00:00:00Z',
            updatedAt: '2026-05-01T00:00:00Z',
          },
          {
            id: '01HXTEST00000000000000001',
            name: 'ops-triager',
            description: 'triage persona',
            currentVersion: 1,
            createdAt: '2026-05-01T00:00:00Z',
            updatedAt: '2026-05-01T00:00:00Z',
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
  if (url.includes('/agent-images/claude-tooling-base')) {
    return new Response(
      JSON.stringify({
        id: 'claude-tooling-base:1.4',
        displayName: 'Claude Tooling Base',
        description: 'Base tooling image',
        imageURL: 'ghcr.io/ainsel/claude-tooling:1.4',
        tools: [
          { name: 'read_file', kind: 'shell' },
          { name: 'run_shell', kind: 'shell' },
        ],
        env: [],
        mcpServers: [],
        enabledSkills: [],
      }),
      { status: 200 },
    )
  }
  if (url.includes('/skills')) {
    return new Response(
      JSON.stringify({ items: [], total: 0, page: 1, pageSize: 200, totalPages: 0 }),
      { status: 200 },
    )
  }
  if (url.includes('/mcp-servers')) {
    return new Response(JSON.stringify([]), { status: 200 })
  }
  if (url.includes('/agent-images')) {
    return new Response(
      JSON.stringify({
        items: [
          {
            id: 'claude-tooling-base:1.4',
            displayName: 'Claude Tooling Base',
            imageURL: 'ghcr.io/ainsel/claude-tooling:1.4',
            toolCount: 2,
            enabledSkills: [],
          },
          {
            id: 'minimal:1.0',
            displayName: 'Minimal',
            imageURL: 'ghcr.io/ainsel/minimal:1.0',
            toolCount: 0,
            enabledSkills: [],
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

  it('renders persona panel with name, markdown, and configure button', async () => {
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
    expect(
      screen.getByRole('button', { name: /configure persona/i }),
    ).toBeInTheDocument()
  })

  it('opens the Persona tab from the overview button, loads the editor, and switches personas', async () => {
    const putCalls: Array<{ url: string; body?: string }> = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          putCalls.push({ url, body: init.body ? String(init.body) : undefined })
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await screen.findAllByText('doc-writer')
    await userEvent.click(
      await screen.findByRole('button', { name: /configure persona/i }),
    )

    // The editor loads the linked persona's content.
    const text = await screen.findByLabelText('Persona Text')
    expect((text as HTMLTextAreaElement).value).toContain('You are a docs writer.')
    expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('docs-writer')

    // Switching the linked persona patches the agent with the new reference.
    await userEvent.selectOptions(
      screen.getByLabelText('Linked persona'),
      '01HXTEST00000000000000001',
    )
    await waitFor(() => {
      const call = putCalls.find((c) => c.url.includes('/api/v1/agents/a1'))
      expect(call).toBeDefined()
      expect(JSON.parse(call!.body!)).toEqual({
        name: 'doc-writer',
        persona: { id: '01HXTEST00000000000000001' },
      })
    })
  })

  it('opens the Image tab with image identity and env, and switches images', async () => {
    const putCalls: Array<{ url: string; body?: string }> = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          putCalls.push({ url, body: init.body ? String(init.body) : undefined })
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await screen.findAllByText('doc-writer')
    await userEvent.click(screen.getByRole('tab', { name: /^image$/i }))

    // The embedded editor loads the referenced image's identity and env.
    const urlInput = await screen.findByLabelText('Image URL')
    await waitFor(() =>
      expect(urlInput).toHaveValue('ghcr.io/ainsel/claude-tooling:1.4'),
    )
    expect(
      screen.getByRole('heading', { name: 'Environment Variables' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^save$/i })).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /^cancel$/i }),
    ).not.toBeInTheDocument()

    // No tools-side or skills sections leak onto this tab.
    expect(
      screen.queryByRole('heading', { name: 'MCP Servers' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'Skills' }),
    ).not.toBeInTheDocument()

    // Switching the image patches the agent with the new reference.
    await userEvent.selectOptions(screen.getByLabelText('Agent image'), 'minimal:1.0')
    await waitFor(() => {
      const call = putCalls.find((c) => c.url.includes('/api/v1/agents/a1'))
      expect(call).toBeDefined()
      expect(JSON.parse(call!.body!)).toEqual({
        name: 'doc-writer',
        imageRef: { name: 'minimal:1.0' },
      })
    })
  })

  it('opens the Tools tab with only the tools side of the image', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await screen.findAllByText('doc-writer')
    await userEvent.click(screen.getByRole('tab', { name: /^tools$/i }))

    // MCP servers and the tool list render for the referenced image.
    expect(
      await screen.findByRole('heading', { name: 'MCP Servers' }),
    ).toBeInTheDocument()
    expect((await screen.findAllByText('read_file')).length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: /^save$/i })).toBeInTheDocument()

    // No image-side, skills, or picker sections on this tab.
    expect(screen.queryByLabelText('Image URL')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'Environment Variables' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'Skills' }),
    ).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Agent image')).not.toBeInTheDocument()
  })

  it('opens the Skills tab embedding only the skills editor', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await screen.findAllByText('doc-writer')
    await userEvent.click(screen.getByRole('tab', { name: /^skills$/i }))

    // The skills dual-list picker renders for the referenced image.
    expect(
      await screen.findByRole('button', {
        name: /add selected to enabled on this image/i,
      }),
    ).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Skills' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^save$/i })).toBeInTheDocument()

    // No tools-side sections leak onto this tab.
    expect(screen.queryByLabelText('Image URL')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'MCP Servers' }),
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('heading', { name: 'Environment Variables' }),
    ).not.toBeInTheDocument()
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

  it('shows runtime stats on the overview tab and no separate Status tab', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/:id" element={<AgentDetail />} />
      </Routes>,
      { route: '/agents/a1' },
    )
    await screen.findByText('claude-opus-4-7')
    expect(screen.getByRole('heading', { name: 'Runtime Status' })).toBeInTheDocument()
    expect(screen.getByText('Configured Replicas')).toBeInTheDocument()
    expect(
      screen.queryByRole('tab', { name: /^status$/i }),
    ).not.toBeInTheDocument()
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
