import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { AgentForm } from './AgentForm'
import { renderWithProviders } from '../../test/renderWithProviders'

function defaultFetch(url: string, init?: RequestInit): Response {
  if ((init?.method ?? 'GET') === 'GET' && url.includes('/groups')) {
    return new Response(
      JSON.stringify([
        { id: 'g1', name: 'Team A', description: '', createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' },
      ]),
      { status: 200 },
    )
  }
  if (url.includes('/agent-images')) {
    return new Response(
      JSON.stringify({
        items: [
          { id: 'claude-tooling-base:1.4', displayName: 'claude-tooling-base:1.4' },
        ],
        total: 1,
        page: 1,
        pageSize: 200,
        totalPages: 1,
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
            name: 'test-persona',
            description: '',
            currentVersion: 1,
            createdAt: '2026-05-01T00:00:00Z',
            updatedAt: '2026-05-01T00:00:00Z',
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
  if (init?.method === 'POST' && url.includes('/agents')) {
    return new Response(
      JSON.stringify({ id: 'new-id', name: 'fresh', llm: { model: 'm' } }),
      { status: 200 },
    )
  }
  if (url.includes('/agents/a1')) {
    return new Response(
      JSON.stringify({
        id: 'a1',
        name: 'doc-writer',
        description: 'Writes docs',
        imageRef: { name: 'claude-tooling-base:1.4' },
        llm: { model: 'claude-opus-4-7' },
        persona: { id: '01HXTEST00000000000000000' },
      }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

describe('AgentForm', () => {
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

  it('shows validation errors for required fields', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/new" element={<AgentForm />} />
      </Routes>,
      { route: '/agents/new' },
    )
    await userEvent.click(screen.getByRole('button', { name: /create/i }))
    expect(await screen.findByText(/name is required/i)).toBeInTheDocument()
    expect(screen.getByText(/model is required/i)).toBeInTheDocument()
    expect(screen.getByText(/persona is required/i)).toBeInTheDocument()
  })

  it('shows a persona picker populated from usePersonas', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/new" element={<AgentForm />} />
      </Routes>,
      { route: '/agents/new' },
    )
    expect(await screen.findByLabelText(/^persona$/i)).toBeInTheDocument()
    await waitFor(() =>
      expect(
        screen.getByRole('option', { name: /test-persona/i }),
      ).toBeInTheDocument(),
    )
  })

  it('pre-fills form fields in edit mode', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/:id/edit" element={<AgentForm />} />
      </Routes>,
      { route: '/agents/a1/edit' },
    )
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe(
        'doc-writer',
      ),
    )
    expect((screen.getByLabelText('Model') as HTMLInputElement).value).toBe(
      'claude-opus-4-7',
    )
  })

  it('sends customProvider URL and API key in the POST body when Custom is selected', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) =>
      Promise.resolve(defaultFetch(url, init)),
    )
    vi.stubGlobal('fetch', fetchMock)

    renderWithProviders(
      <Routes>
        <Route path="/agents/new" element={<AgentForm />} />
      </Routes>,
      { route: '/agents/new' },
    )

    await userEvent.type(screen.getByLabelText('Name'), 'my-agent')
    await userEvent.type(screen.getByLabelText('Model'), 'gpt-4')

    await waitFor(() =>
      expect(
        screen.getByRole('option', { name: /claude-tooling-base/i }),
      ).toBeInTheDocument(),
    )
    await userEvent.selectOptions(
      screen.getByLabelText(/^image$/i),
      'claude-tooling-base:1.4',
    )
    await waitFor(() =>
      expect(
        screen.getByRole('option', { name: /test-persona/i }),
      ).toBeInTheDocument(),
    )
    await userEvent.selectOptions(
      screen.getByLabelText(/^persona$/i),
      '01HXTEST00000000000000000',
    )

    await userEvent.selectOptions(screen.getByLabelText(/^provider$/i), 'custom')
    await userEvent.type(
      await screen.findByLabelText(/provider base url/i),
      'https://api.example.com/v1',
    )
    await userEvent.type(
      screen.getByLabelText(/^api key$/i),
      'sk-test-12345',
    )

    await userEvent.selectOptions(await screen.findByLabelText('Group'), 'g1')

    await userEvent.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        ([u, init]) =>
          typeof u === 'string' &&
          u.includes('/agents') &&
          (init as RequestInit | undefined)?.method === 'POST',
      )
      expect(postCall).toBeDefined()
    })

    const postCall = fetchMock.mock.calls.find(
      ([u, init]) =>
        typeof u === 'string' &&
        u.includes('/agents') &&
        (init as RequestInit | undefined)?.method === 'POST',
    )!
    const init = postCall[1] as RequestInit
    const body = JSON.parse(init.body as string)
    expect(body.llm.provider).toBe('custom')
    expect(body.customProvider).toEqual({
      url: 'https://api.example.com/v1',
      apiKey: 'sk-test-12345',
    })
  })

  it('blocks submit and shows an error when Custom is selected without a URL', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) =>
      Promise.resolve(defaultFetch(url, init)),
    )
    vi.stubGlobal('fetch', fetchMock)

    renderWithProviders(
      <Routes>
        <Route path="/agents/new" element={<AgentForm />} />
      </Routes>,
      { route: '/agents/new' },
    )

    await userEvent.type(screen.getByLabelText('Name'), 'my-agent')
    await userEvent.type(screen.getByLabelText('Model'), 'gpt-4')
    await waitFor(() =>
      expect(
        screen.getByRole('option', { name: /claude-tooling-base/i }),
      ).toBeInTheDocument(),
    )
    await userEvent.selectOptions(
      screen.getByLabelText(/^image$/i),
      'claude-tooling-base:1.4',
    )
    await waitFor(() =>
      expect(
        screen.getByRole('option', { name: /test-persona/i }),
      ).toBeInTheDocument(),
    )
    await userEvent.selectOptions(
      screen.getByLabelText(/^persona$/i),
      '01HXTEST00000000000000000',
    )

    await userEvent.selectOptions(screen.getByLabelText(/^provider$/i), 'custom')
    // Type only an API key, leave URL empty
    await userEvent.type(
      screen.getByLabelText(/^api key$/i),
      'sk-test-12345',
    )
    await userEvent.click(screen.getByRole('button', { name: /create/i }))

    expect(
      await screen.findByText(/url is required for custom providers/i),
    ).toBeInTheDocument()

    // No POST should have been made
    const postCall = fetchMock.mock.calls.find(
      ([u, init]) =>
        typeof u === 'string' &&
        u.includes('/agents') &&
        (init as RequestInit | undefined)?.method === 'POST',
    )
    expect(postCall).toBeUndefined()
  })

  it('renders a single provider select defaulting to Ollama Cloud and allows selecting None', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/agents/new" element={<AgentForm />} />
      </Routes>,
      { route: '/agents/new' },
    )

    const providerSelect = await screen.findByLabelText(/^provider$/i)
    expect(providerSelect).toBeInTheDocument()

    // Only one Provider select in the LLM section
    expect(screen.getAllByLabelText(/^provider$/i)).toHaveLength(1)

    // Default is Ollama Cloud
    expect((providerSelect as HTMLSelectElement).value).toBe('ollama-cloud')

    // Selecting None should stick
    await userEvent.selectOptions(providerSelect, 'None')
    expect((providerSelect as HTMLSelectElement).value).toBe('')
  })
})
