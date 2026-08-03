import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes, useNavigate } from 'react-router-dom'
import { ImageDetail } from './ImageDetail'
import { renderWithProviders } from '../../test/renderWithProviders'
import type { AgentImageRequest } from '../../api/agentImages'

function skillsFixture() {
  return {
    items: [
      { id: 'git-ops', name: 'git-ops', description: 'Git operations skill', createdAt: '', updatedAt: '' },
      { id: 'k8s-deploy', name: 'k8s-deploy', description: 'Kubernetes deploy skill', createdAt: '', updatedAt: '' },
      { id: 'pr-review', name: 'pr-review', description: 'PR review skill', createdAt: '', updatedAt: '' },
    ],
    total: 3,
    page: 1,
    pageSize: 200,
  }
}

function imageFixture() {
  return {
    id: 'claude-tooling-base:1.4',
    displayName: 'claude-tooling-base',
    description: 'Base image with shell tools',
    imageURL: 'ghcr.io/insel/claude-tooling-base:1.4',
    enabledSkills: ['git-ops'],
    tools: [
      {
        name: 'read_file',
        kind: 'shell',
        description: 'Read a file',
        examples: [{ title: 'Read README', snippet: 'read_file: README.md' }],
      },
      { name: 'run_shell', kind: 'shell', description: 'Run a command', examples: [] },
    ],
  }
}

function defaultFetch(url: string, init?: RequestInit): Response {
  if (init?.method === 'DELETE') return new Response(null, { status: 204 })
  if ((init?.method ?? 'GET') === 'GET' && url.includes('/groups')) {
    return new Response(
      JSON.stringify([
        { id: 'g1', name: 'Team A', description: '', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
      ]),
      { status: 200 },
    )
  }
  if (init?.method === 'POST' && url.includes('/agent-images') && !url.includes('%')) {
    return new Response(JSON.stringify({ ...imageFixture(), id: 'new-id' }), {
      status: 200,
    })
  }
  if (init?.method === 'PUT' && url.includes('/agent-images')) {
    return new Response(JSON.stringify(imageFixture()), { status: 200 })
  }
  if (url.includes('/agent-images/claude-tooling-base%3A1.4')) {
    return new Response(JSON.stringify(imageFixture()), { status: 200 })
  }
  if (url.includes('/mcp-servers')) {
    return new Response(JSON.stringify([]), { status: 200 })
  }
  if (url.includes('/skills')) {
    return new Response(JSON.stringify(skillsFixture()), { status: 200 })
  }
  return new Response('{}', { status: 200 })
}

describe('ImageDetail', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => Promise.resolve(defaultFetch(url, init))),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('starts with default shell tools in new mode', async () => {
    const { container } = renderWithProviders(
      <Routes>
        <Route path="/agent-images/new" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/new' },
    )
    // New images are pre-populated with common shell tools.
    const toolList = container.querySelector('.tool-list') as HTMLElement
    expect(within(toolList).getByText('bash')).toBeInTheDocument()
    expect(within(toolList).getByText('curl')).toBeInTheDocument()
  })

  it('loads existing tools in edit mode and shows the first by default', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )
    await waitFor(() => expect(screen.getAllByText('read_file').length).toBeGreaterThan(0))
    // Editor pane shows the tool detail (Name field populated)
    expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('read_file')
  })

  it('selecting a different tool updates the editor pane', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('read_file'),
    )
    // Find the run_shell tool row (button) and click it.
    const rows = screen.getAllByRole('button', { name: /Select tool/i })
    const runShellRow = rows.find((r) => r.getAttribute('aria-label')?.includes('run_shell'))!
    await userEvent.click(runShellRow)
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('run_shell'),
    )
  })

  it('re-selecting the first tool after selecting another updates the editor pane', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('read_file'),
    )
    // Select the second tool (run_shell)
    const rows = screen.getAllByRole('button', { name: /Select tool/i })
    const runShellRow = rows.find((r) => r.getAttribute('aria-label')?.includes('run_shell'))!
    await userEvent.click(runShellRow)
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('run_shell'),
    )
    // Re-select the first tool (read_file)
    const readFileRow = rows.find((r) => r.getAttribute('aria-label')?.includes('read_file'))!
    await userEvent.click(readFileRow)
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('read_file'),
    )
  })

  it('confirm-delete navigates back to the list', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
        <Route path="/agent-images" element={<div>LIST</div>} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('read_file'),
    )
    await userEvent.click(screen.getByRole('button', { name: /^delete$/i }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: /^delete/i }))
    await waitFor(() => expect(screen.queryByText('LIST')).toBeInTheDocument())
  })

  it('renders env vars from existing image and submits them', async () => {
    const user = userEvent.setup()

    const fixtureWithEnv = {
      ...imageFixture(),
      env: [{ name: 'FORGEJO_URL', value: 'https://git.example.com' }],
    }

    let capturedBody: AgentImageRequest | null = null
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === 'PUT' && url.includes('/agent-images')) {
          capturedBody = JSON.parse(init.body as string) as AgentImageRequest
          return Promise.resolve(new Response(JSON.stringify(fixtureWithEnv), { status: 200 }))
        }
        if (url.includes('/agent-images/claude-tooling-base%3A1.4')) {
          return Promise.resolve(new Response(JSON.stringify(fixtureWithEnv), { status: 200 }))
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )

    await waitFor(() => expect(screen.getByDisplayValue('FORGEJO_URL')).toBeInTheDocument())
    expect(screen.getByDisplayValue('https://git.example.com')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /save/i }))
    await waitFor(() => expect(capturedBody).not.toBeNull())
    expect(capturedBody!.env).toEqual([{ name: 'FORGEJO_URL', value: 'https://git.example.com' }])
  })

  it('can add a new env var and submit', async () => {
    const user = userEvent.setup()
    let capturedBody: AgentImageRequest | null = null
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === 'POST' && url.includes('/agent-images') && !url.includes('%')) {
          capturedBody = JSON.parse(init.body as string) as AgentImageRequest
          return Promise.resolve(
            new Response(JSON.stringify({ ...imageFixture(), id: 'new-id' }), { status: 200 }),
          )
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agent-images/new" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/new' },
    )

    await user.clear(screen.getByLabelText(/image url/i))
    await user.type(screen.getByLabelText(/image url/i), 'ghcr.io/test/image:latest')

    await user.click(screen.getByRole('button', { name: /add variable/i }))
    const nameInputs = screen.getAllByPlaceholderText('NAME')
    await user.type(nameInputs[nameInputs.length - 1], 'MY_VAR')
    const valueInputs = screen.getAllByPlaceholderText('value')
    await user.type(valueInputs[valueInputs.length - 1], 'my-value')

    await user.selectOptions(await screen.findByLabelText('Group'), 'g1')

    await user.click(screen.getByRole('button', { name: /create/i }))
    await waitFor(() => expect(capturedBody).not.toBeNull())
    expect(capturedBody!.env).toEqual([{ name: 'MY_VAR', value: 'my-value' }])
  })

  it('renders secret env vars with masked value and secret checkbox', async () => {
    const fixtureWithSecretEnv = {
      ...imageFixture(),
      env: [
        { name: 'FORGEJO_TOKEN', value: '', secret: true },
        { name: 'LOG_LEVEL', value: 'debug', secret: false },
      ],
    }

    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (url.includes('/agent-images/claude-tooling-base%3A1.4')) {
          return Promise.resolve(
            new Response(JSON.stringify(fixtureWithSecretEnv), { status: 200 }),
          )
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )

    await waitFor(() => expect(screen.getByDisplayValue('LOG_LEVEL')).toBeInTheDocument())

    // Secret env var's value field should be a password input with bullet placeholder
    const maskedInputs = screen.getAllByPlaceholderText('••••••••')
    expect(maskedInputs.length).toBeGreaterThan(0)
    const secretValueInput = maskedInputs[0] as HTMLInputElement
    expect(secretValueInput.type).toBe('password')

    // Non-secret env var's value should be visible as plain text
    expect(screen.getByDisplayValue('debug')).toBeInTheDocument()
  })

  it('Refresh MCP Tools saves the current form (including new MCP servers) before calling refresh-mcp', async () => {
    const user = userEvent.setup()
    const callOrder: string[] = []

    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        const method = init?.method ?? 'GET'
        if (method === 'POST' && url.includes('refresh-mcp')) {
          callOrder.push('POST /refresh-mcp')
          return Promise.resolve(
            new Response(
              JSON.stringify({
                ...imageFixture(),
                tools: [
                  ...imageFixture().tools,
                  { name: 'mcp_list', kind: 'mcp', description: 'List resources via MCP' },
                ],
              }),
              { status: 200 },
            ),
          )
        }
        if (method === 'PUT' && url.includes('/agent-images')) {
          callOrder.push('PUT /agent-images')
          return Promise.resolve(new Response(JSON.stringify(imageFixture()), { status: 200 }))
        }
        if (url.includes('/mcp-servers')) {
          return Promise.resolve(
            new Response(
              JSON.stringify([
                { name: 'my-mcp', displayName: 'My MCP', url: 'http://mcp.example.com' },
              ]),
              { status: 200 },
            ),
          )
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )

    // Wait for the image to load (tool name appears in the editor)
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('read_file'),
    )

    // Add a new MCP server via the dual-list picker
    await user.click(await screen.findByRole('option', { name: /My MCP/ }))
    await user.click(screen.getByRole('button', { name: /add selected to referenced by this image/i }))

    // Click Refresh MCP Tools
    await user.click(screen.getByRole('button', { name: /refresh mcp tools/i }))

    // Wait for both mutations to complete
    await waitFor(() => callOrder.includes('POST /refresh-mcp'))

    // The form must have been saved (PUT) before the refresh
    expect(callOrder.indexOf('PUT /agent-images')).toBeGreaterThanOrEqual(0)
    expect(callOrder.indexOf('PUT /agent-images')).toBeLessThan(
      callOrder.indexOf('POST /refresh-mcp'),
    )
  })

  it('can toggle secret on a new env var and submit with secret flag', async () => {
    const user = userEvent.setup()
    let capturedBody: AgentImageRequest | null = null
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === 'POST' && url.includes('/agent-images') && !url.includes('%')) {
          capturedBody = JSON.parse(init.body as string) as AgentImageRequest
          return Promise.resolve(
            new Response(JSON.stringify({ ...imageFixture(), id: 'new-id' }), { status: 200 }),
          )
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agent-images/new" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/new' },
    )

    await user.clear(screen.getByLabelText(/image url/i))
    await user.type(screen.getByLabelText(/image url/i), 'ghcr.io/test/image:latest')

    await user.click(screen.getByRole('button', { name: /add variable/i }))
    const nameInputs = screen.getAllByPlaceholderText('NAME')
    await user.type(nameInputs[nameInputs.length - 1], 'API_KEY')
    const valueInputs = screen.getAllByPlaceholderText('value')
    await user.type(valueInputs[valueInputs.length - 1], 'my-secret')

    // Toggle the secret checkbox
    const checkboxes = screen.getAllByRole('checkbox')
    await user.click(checkboxes[0])

    await user.selectOptions(await screen.findByLabelText('Group'), 'g1')

    await user.click(screen.getByRole('button', { name: /create/i }))
    await waitFor(() => expect(capturedBody).not.toBeNull())
    expect(capturedBody!.env).toEqual([{ name: 'API_KEY', value: 'my-secret', secret: true }])
  })

  it('calls PUT before POST /refresh-mcp when Refresh MCP is clicked', async () => {
    const user = userEvent.setup()
    const calls: { method: string; url: string }[] = []

    const fixtureWithMcp = {
      ...imageFixture(),
      mcpServers: [{ name: 'example-mcp', url: 'http://example-mcp:8080', tokenFromEnv: 'EXAMPLE_MCP_TOKEN' }],
    }

    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({ method: init?.method ?? 'GET', url })
        if (init?.method === 'PUT' && url.includes('/agent-images')) {
          return Promise.resolve(new Response(JSON.stringify(fixtureWithMcp), { status: 200 }))
        }
        if (init?.method === 'POST' && url.includes('/refresh-mcp')) {
          return Promise.resolve(new Response(JSON.stringify(fixtureWithMcp), { status: 200 }))
        }
        if (url.includes('/agent-images/claude-tooling-base%3A1.4')) {
          return Promise.resolve(new Response(JSON.stringify(fixtureWithMcp), { status: 200 }))
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )

    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('read_file'),
    )

    await user.click(screen.getByRole('button', { name: /refresh mcp tools/i }))
    await waitFor(() =>
      expect(calls.some((c) => c.method === 'POST' && c.url.includes('/refresh-mcp'))).toBe(true),
    )

    const putIdx = calls.findIndex((c) => c.method === 'PUT' && c.url.includes('/agent-images'))
    const refreshIdx = calls.findIndex((c) => c.method === 'POST' && c.url.includes('/refresh-mcp'))
    expect(putIdx).toBeGreaterThan(-1)
    expect(refreshIdx).toBeGreaterThan(-1)
    expect(putIdx).toBeLessThan(refreshIdx)
  })

  it('sends empty arrays when all tools are removed', async () => {
    const user = userEvent.setup()
    let capturedBody: AgentImageRequest | null = null

    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === 'PUT' && url.includes('/agent-images')) {
          capturedBody = JSON.parse(init.body as string) as AgentImageRequest
          return Promise.resolve(new Response(JSON.stringify(imageFixture()), { status: 200 }))
        }
        if (url.includes('/agent-images/claude-tooling-base%3A1.4')) {
          return Promise.resolve(new Response(JSON.stringify(imageFixture()), { status: 200 }))
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )

    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('read_file'),
    )

    // Remove the first tool (read_file), already selected
    await user.click(screen.getByRole('button', { name: /^remove$/i }))

    // Select and remove the second tool (run_shell)
    const runShellRow = screen.getByRole('button', { name: /Select tool run_shell/i })
    await user.click(runShellRow)
    await user.click(screen.getByRole('button', { name: /^remove$/i }))

    await user.click(screen.getByRole('button', { name: /save/i }))
    await waitFor(() => expect(capturedBody).not.toBeNull())
    expect(capturedBody!.tools).toEqual([])
  })

  it('sends empty arrays when all env vars are removed', async () => {
    const user = userEvent.setup()
    let capturedBody: AgentImageRequest | null = null

    const fixtureWithEnv = {
      ...imageFixture(),
      env: [{ name: 'FORGEJO_URL', value: 'https://git.example.com' }],
    }

    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === 'PUT' && url.includes('/agent-images')) {
          capturedBody = JSON.parse(init.body as string) as AgentImageRequest
          return Promise.resolve(new Response(JSON.stringify(fixtureWithEnv), { status: 200 }))
        }
        if (url.includes('/agent-images/claude-tooling-base%3A1.4')) {
          return Promise.resolve(new Response(JSON.stringify(fixtureWithEnv), { status: 200 }))
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )

    await waitFor(() => expect(screen.getByDisplayValue('FORGEJO_URL')).toBeInTheDocument())

    await user.click(screen.getByRole('button', { name: /remove variable/i }))

    await user.click(screen.getByRole('button', { name: /save/i }))
    await waitFor(() => expect(capturedBody).not.toBeNull())
    expect(capturedBody!.env).toEqual([])
  })

  it('sends empty arrays when all MCP servers are removed', async () => {
    const user = userEvent.setup()
    let capturedBody: AgentImageRequest | null = null

    const fixtureWithMcp = {
      ...imageFixture(),
      mcpServers: [{ name: 'example-mcp', url: 'http://example-mcp:8080', tokenFromEnv: 'EXAMPLE_MCP_TOKEN' }],
    }

    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === 'PUT' && url.includes('/agent-images')) {
          capturedBody = JSON.parse(init.body as string) as AgentImageRequest
          return Promise.resolve(new Response(JSON.stringify(fixtureWithMcp), { status: 200 }))
        }
        if (url.includes('/agent-images/claude-tooling-base%3A1.4')) {
          return Promise.resolve(new Response(JSON.stringify(fixtureWithMcp), { status: 200 }))
        }
        if (url.includes('/mcp-servers')) {
          return Promise.resolve(
            new Response(
              JSON.stringify([
                { name: 'example-mcp', displayName: 'example-mcp', url: 'http://example-mcp:8080', tokenFromEnv: 'EXAMPLE_MCP_TOKEN' },
              ]),
              { status: 200 },
            ),
          )
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )

    // Wait for the image to load before interacting with the picker
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('read_file'),
    )

    // Select example-mcp in the Enabled pane and move it to Available
    await user.click(await screen.findByRole('option', { name: /example-mcp/ }))
    await user.click(screen.getByRole('button', { name: /remove selected from referenced by this image/i }))

    await user.click(screen.getByRole('button', { name: /save/i }))
    await waitFor(() => expect(capturedBody).not.toBeNull())
    expect(capturedBody!.mcpServers).toEqual([])
  })

  it('toggles an MCP tool off and sends enabled: false in the PUT body', async () => {
    const user = userEvent.setup()
    let capturedBody: AgentImageRequest | null = null

    const fixtureWithMcpTool = {
      ...imageFixture(),
      tools: [
        ...imageFixture().tools,
        {
          name: 'mcp__my-mcp__list_resources',
          kind: 'mcp',
          description: 'List resources via MCP',
          mcpSource: 'my-mcp',
          enabled: true,
        },
      ],
    }

    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === 'PUT' && url.includes('/agent-images')) {
          capturedBody = JSON.parse(init.body as string) as AgentImageRequest
          return Promise.resolve(new Response(JSON.stringify(fixtureWithMcpTool), { status: 200 }))
        }
        if (url.includes('/agent-images/claude-tooling-base%3A1.4')) {
          return Promise.resolve(new Response(JSON.stringify(fixtureWithMcpTool), { status: 200 }))
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )

    // Wait for the MCP tool to appear in the list
    await waitFor(() =>
      expect(screen.getByRole('checkbox', { name: /toggle list_resources/i })).toBeInTheDocument(),
    )

    // Toggle the MCP tool off
    const toggle = screen.getByRole('checkbox', { name: /toggle list_resources/i })
    await user.click(toggle)
    expect(toggle).not.toBeChecked()

    await user.click(screen.getByRole('button', { name: /save/i }))
    await waitFor(() => expect(capturedBody).not.toBeNull())
    const mcpTool = capturedBody!.tools?.find((t) => t.name === 'mcp__my-mcp__list_resources')
    expect(mcpTool).toBeDefined()
    expect(mcpTool!.enabled).toBe(false)
  })

  it('preserves user edits across a background refetch', async () => {
    const user = userEvent.setup()
    let fetchCount = 0

    const initialFixture = imageFixture()
    const updatedFixture = {
      ...imageFixture(),
      displayName: 'updated-name-from-server',
    }

    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (
          url.includes('/agent-images/claude-tooling-base%3A1.4') &&
          !url.includes('/access') &&
          (!init || init.method === 'GET')
        ) {
          fetchCount++
          const body = fetchCount <= 1 ? initialFixture : updatedFixture
          return Promise.resolve(new Response(JSON.stringify(body), { status: 200 }))
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    const { queryClient } = renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )

    await waitFor(() =>
      expect((screen.getByLabelText('Display Name') as HTMLInputElement).value).toBe(
        'claude-tooling-base',
      ),
    )

    // User edits the display name
    const displayNameInput = screen.getByLabelText('Display Name') as HTMLInputElement
    await user.clear(displayNameInput)
    await user.type(displayNameInput, 'user-edited-name')
    expect(displayNameInput.value).toBe('user-edited-name')

    // Trigger a background refetch
    queryClient.invalidateQueries({
      queryKey: ['agent-images', 'detail', 'claude-tooling-base:1.4'],
    })
    await waitFor(() => expect(fetchCount).toBeGreaterThanOrEqual(2))

    // The user's edit must still be present
    expect((screen.getByLabelText('Display Name') as HTMLInputElement).value).toBe(
      'user-edited-name',
    )
  })

  it('toggling a skill via the dual-list picker updates the request body on save', async () => {
    const user = userEvent.setup()
    let capturedBody: AgentImageRequest | null = null

    const fixtureWithSkills = {
      ...imageFixture(),
      enabledSkills: ['git-ops', 'k8s-deploy'],
    }

    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (init?.method === 'PUT' && url.includes('/agent-images')) {
          capturedBody = JSON.parse(init.body as string) as AgentImageRequest
          return Promise.resolve(new Response(JSON.stringify(fixtureWithSkills), { status: 200 }))
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<ImageDetail />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )

    // Move 'k8s-deploy' from Available to Enabled.
    await user.click(await screen.findByRole('option', { name: /k8s-deploy/ }))
    await user.click(screen.getByRole('button', { name: /add selected to enabled/i }))
    await user.click(screen.getByRole('button', { name: /^save$/i }))
    await waitFor(() => expect(capturedBody).not.toBeNull())
    expect(capturedBody!.enabledSkills).toEqual(['git-ops', 'k8s-deploy'])
  })

  it('resets the form when navigating from one image to another', async () => {
    const user = userEvent.setup()

    const firstFixture = imageFixture()
    const secondFixture = {
      id: 'other-image:2.0',
      displayName: 'other-image',
      description: 'Another image',
      imageURL: 'ghcr.io/insel/other-image:2.0',
      tools: [{ name: 'write_file', kind: 'shell', description: 'Write a file', examples: [] }],
    }

    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (url.includes('/agent-images/claude-tooling-base%3A1.4')) {
          return Promise.resolve(new Response(JSON.stringify(firstFixture), { status: 200 }))
        }
        if (url.includes('/agent-images/other-image%3A2.0')) {
          return Promise.resolve(new Response(JSON.stringify(secondFixture), { status: 200 }))
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )

    function Wrapper() {
      const navigate = useNavigate()
      return (
        <>
          <button onClick={() => navigate('/agent-images/other-image:2.0')}>Navigate</button>
          <ImageDetail />
        </>
      )
    }

    renderWithProviders(
      <Routes>
        <Route path="/agent-images/:id" element={<Wrapper />} />
      </Routes>,
      { route: '/agent-images/claude-tooling-base:1.4' },
    )

    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('read_file'),
    )
    expect((screen.getByLabelText('Display Name') as HTMLInputElement).value).toBe(
      'claude-tooling-base',
    )

    // Navigate to the other image via React Router (same component, new id)
    await user.click(screen.getByRole('button', { name: /navigate/i }))

    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe('write_file'),
    )
    expect((screen.getByLabelText('Display Name') as HTMLInputElement).value).toBe('other-image')
  })
})
