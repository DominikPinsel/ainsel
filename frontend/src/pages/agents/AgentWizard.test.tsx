import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { AgentWizard } from './AgentWizard'
import { renderWithProviders } from '../../test/renderWithProviders'

const IMAGE_ID = 'claude-tooling-base:1.4'
const PERSONA_ID = '01HXTEST00000000000000000'

function defaultFetch(url: string, init?: RequestInit): Response {
  const method = init?.method ?? 'GET'
  if (method === 'GET' && url.includes('/groups')) {
    return new Response(
      JSON.stringify([
        {
          id: 'g1',
          name: 'Team A',
          description: '',
          createdAt: '2026-01-01T00:00:00Z',
          updatedAt: '2026-01-01T00:00:00Z',
        },
      ]),
      { status: 200 },
    )
  }
  // Image detail (what the selected runtime brings) before the list match.
  if (url.includes(`/agent-images/${IMAGE_ID.split(':')[0]}`)) {
    return new Response(
      JSON.stringify({
        id: IMAGE_ID,
        displayName: 'Claude Tooling Base',
        imageURL: 'ghcr.io/ainsel/claude-tooling:1.4',
        tools: [{ name: 'read_file' }, { name: 'run_shell' }],
        env: [{ name: 'LOG_LEVEL', value: 'info' }],
        mcpServers: [{ name: 'github', url: 'https://mcp.github.com/sse' }],
        enabledSkills: ['git-review'],
      }),
      { status: 200 },
    )
  }
  if (url.includes('/agent-images')) {
    return new Response(
      JSON.stringify({
        items: [{ id: IMAGE_ID, displayName: 'Claude Tooling Base' }],
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
            id: PERSONA_ID,
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
  if (method === 'POST' && url.includes('/agents')) {
    return new Response(
      JSON.stringify({ id: 'new-id', name: 'fresh', llm: { model: 'm' } }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

function renderWizard(route = '/agents/new') {
  return renderWithProviders(
    <Routes>
      <Route path="/agents/new" element={<AgentWizard />} />
    </Routes>,
    { route },
  )
}

function postBody(fetchMock: ReturnType<typeof vi.fn>): Record<string, unknown> | undefined {
  const call = fetchMock.mock.calls.find(
    ([u, init]) =>
      typeof u === 'string' &&
      u.includes('/agents') &&
      (init as RequestInit | undefined)?.method === 'POST',
  )
  if (!call) return undefined
  return JSON.parse((call[1] as RequestInit).body as string) as Record<string, unknown>
}

/** Select helpers wait for the query behind each picker to land first. */
async function selectGroup(user: ReturnType<typeof userEvent.setup>) {
  await waitFor(() =>
    expect(screen.getByRole('option', { name: /Team A/i })).toBeInTheDocument(),
  )
  await user.selectOptions(screen.getByLabelText('Group'), 'g1')
}

async function selectImage(user: ReturnType<typeof userEvent.setup>) {
  await waitFor(() =>
    expect(
      screen.getByRole('option', { name: /Claude Tooling Base/i }),
    ).toBeInTheDocument(),
  )
  await user.selectOptions(screen.getByLabelText(/runtime image/i), IMAGE_ID)
}

async function selectPersona(user: ReturnType<typeof userEvent.setup>) {
  await waitFor(() =>
    expect(
      screen.getByRole('option', { name: /test-persona/i }),
    ).toBeInTheDocument(),
  )
  await user.selectOptions(screen.getByLabelText(/^persona$/i), PERSONA_ID)
}

const next = () => screen.getByRole('button', { name: /^next$/i })

/**
 * Clicks a step in the stepper and waits for that step's region to render.
 * Step changes are async (forward jumps validate first), so a synchronous
 * query right after the click races the navigation.
 */
async function goToStep(user: ReturnType<typeof userEvent.setup>, title: string) {
  await user.click(screen.getByRole('button', { name: new RegExp(title, 'i') }))
  await screen.findByRole('region', { name: new RegExp(`: ${title}$`, 'i') })
}

/** Walks identity → runtime → model → persona with valid values. */
async function fillThroughPersona(user: ReturnType<typeof userEvent.setup>) {
  await user.type(await screen.findByLabelText('Name'), 'my-agent')
  await selectGroup(user)
  await user.click(next())

  await selectImage(user)
  await user.click(next())

  await user.type(await screen.findByLabelText('Model'), 'gpt-4')
  await user.click(next())

  await selectPersona(user)
  await user.click(next())
}

describe('AgentWizard', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn((url: string, init?: RequestInit) =>
      Promise.resolve(defaultFetch(url, init)),
    )
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('starts on the identity step and shows every step in order', async () => {
    renderWizard()
    const steps = await screen.findByRole('list', { name: /creation steps/i })
    expect(
      within(steps)
        .getAllByRole('button')
        .map((b) => b.textContent?.replace(/§\d+/, '').trim()),
    ).toEqual(['Identity', 'Runtime', 'Model', 'Persona', 'Review'])
    expect(
      within(steps).getByRole('button', { name: /identity/i }),
    ).toHaveAttribute('aria-current', 'step')
    expect(screen.getByLabelText('Name')).toBeInTheDocument()
    // The visible step is exposed as a named region for screen readers.
    expect(
      screen.getByRole('region', { name: /step 1 of 5: identity/i }),
    ).toBeInTheDocument()
  })

  it('blocks Next until the required identity fields are filled', async () => {
    const user = userEvent.setup()
    renderWizard()

    await user.click(await screen.findByRole('button', { name: /^next$/i }))
    expect(await screen.findByText(/name is required/i)).toBeInTheDocument()
    expect(await screen.findByText(/group is required/i)).toBeInTheDocument()
    // Still on step one: the runtime picker never appeared.
    expect(screen.queryByLabelText(/runtime image/i)).not.toBeInTheDocument()

    await user.type(screen.getByLabelText('Name'), 'my-agent')
    await selectGroup(user)
    // Picking a group clears its error without another Next click.
    await waitFor(() =>
      expect(screen.queryByText(/group is required/i)).not.toBeInTheDocument(),
    )
    await user.click(next())
    expect(await screen.findByLabelText(/runtime image/i)).toBeInTheDocument()
  })

  it('summarises what the chosen runtime image brings', async () => {
    const user = userEvent.setup()
    renderWizard('/agents/new?step=runtime')

    await selectImage(user)
    expect(await screen.findByText(/this image brings/i)).toBeInTheDocument()
    expect(screen.getByText(/2 tools · 1 skills · 1 MCP servers · 1 env vars/)).toBeInTheDocument()
    expect(
      screen.getByText(/overridden per agent after creation/i),
    ).toBeInTheDocument()
  })

  it('validates the model step before letting the user move on', async () => {
    const user = userEvent.setup()
    renderWizard('/agents/new?step=model')

    await user.selectOptions(await screen.findByLabelText(/^provider$/i), 'custom')
    await user.type(screen.getByLabelText(/^api key$/i), 'sk-test-12345')
    await user.click(screen.getByRole('button', { name: /^next$/i }))

    expect(await screen.findByText(/model is required/i)).toBeInTheDocument()
    expect(
      await screen.findByText(/url is required for custom providers/i),
    ).toBeInTheDocument()
    expect(screen.queryByLabelText(/^persona$/i)).not.toBeInTheDocument()
  })

  it('creates the agent with every step\'s values in one POST', async () => {
    const user = userEvent.setup()
    renderWizard()

    await fillThroughPersona(user)

    // The review step restates the choices before submitting.
    expect(await screen.findByText('my-agent')).toBeInTheDocument()
    expect(screen.getByText('Claude Tooling Base')).toBeInTheDocument()
    expect(screen.getByText('test-persona')).toBeInTheDocument()
    expect(screen.getByText(/gpt-4 · ollama-cloud/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /create agent/i }))

    await waitFor(() => expect(postBody(fetchMock)).toBeDefined())
    expect(postBody(fetchMock)).toMatchObject({
      name: 'my-agent',
      groupId: 'g1',
      imageRef: { name: IMAGE_ID },
      llm: { model: 'gpt-4', provider: 'ollama-cloud' },
      persona: { id: PERSONA_ID },
      replicas: 1,
    })
  })

  it('sends llm.vision when image input is enabled on the model step', async () => {
    const user = userEvent.setup()
    renderWizard()

    await user.type(await screen.findByLabelText('Name'), 'vision-agent')
    await selectGroup(user)
    await user.click(next())

    await selectImage(user)
    await user.click(next())

    await user.type(await screen.findByLabelText('Model'), 'gpt-4-vision')

    // Text-only is the default: an unset flag must never reach the hub as true.
    const toggle = await screen.findByRole('checkbox', { name: /image input/i })
    expect(toggle).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByText('Text-only model')).toBeInTheDocument()

    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByText('Model accepts images')).toBeInTheDocument()
    await user.click(next())

    await selectPersona(user)
    await user.click(next())

    // The review step restates the choice before submitting.
    expect(
      await screen.findByText(/gpt-4-vision · ollama-cloud · accepts images/),
    ).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /create agent/i }))

    await waitFor(() => expect(postBody(fetchMock)).toBeDefined())
    expect(postBody(fetchMock)?.llm).toMatchObject({
      model: 'gpt-4-vision',
      vision: true,
    })
  })

  it('sends customProvider URL and API key when Custom is selected', async () => {
    const user = userEvent.setup()
    renderWizard()

    await fillThroughPersona(user)
    // Back to the model step to switch provider.
    await goToStep(user, 'Model')
    await user.selectOptions(await screen.findByLabelText(/^provider$/i), 'custom')
    await user.type(
      await screen.findByLabelText(/provider base url/i),
      'https://api.example.com/v1',
    )
    await user.type(screen.getByLabelText(/^api key$/i), 'sk-test-12345')
    await goToStep(user, 'Review')
    await user.click(await screen.findByRole('button', { name: /create agent/i }))

    await waitFor(() => expect(postBody(fetchMock)).toBeDefined())
    expect(postBody(fetchMock)?.customProvider).toEqual({
      url: 'https://api.example.com/v1',
      apiKey: 'sk-test-12345',
    })
    expect((postBody(fetchMock)?.llm as { provider: string }).provider).toBe('custom')
  })

  it('keeps values when stepping back and forward', async () => {
    const user = userEvent.setup()
    renderWizard()

    await user.type(await screen.findByLabelText('Name'), 'my-agent')
    await selectGroup(user)
    await user.click(next())
    await selectImage(user)
    await user.click(screen.getByRole('button', { name: /^back$/i }))

    expect(await screen.findByLabelText('Name')).toHaveValue('my-agent')
    await user.click(next())
    expect(await screen.findByLabelText(/runtime image/i)).toHaveValue(IMAGE_ID)
  })

  it('refuses to jump forward past an unfinished step', async () => {
    const user = userEvent.setup()
    renderWizard()

    await user.click(
      (await screen.findAllByRole('button', { name: /review/i }))[0],
    )
    expect(await screen.findByText(/name is required/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /create agent/i })).not.toBeInTheDocument()
  })

  it('sends the user back to the first invalid step when submitting from a deep link', async () => {
    const user = userEvent.setup()
    renderWizard('/agents/new?step=review')

    await user.click(await screen.findByRole('button', { name: /create agent/i }))

    // Nothing was filled in, so the identity step is where the first error
    // lives — and it has to be visible, not stranded on a hidden step.
    expect(await screen.findByLabelText('Name')).toBeInTheDocument()
    expect(await screen.findByText(/name is required/i)).toBeInTheDocument()
    expect(postBody(fetchMock)).toBeUndefined()
  })

  it('defaults the provider to Ollama Cloud and allows None', async () => {
    const user = userEvent.setup()
    renderWizard('/agents/new?step=model')

    const provider = await screen.findByLabelText(/^provider$/i)
    expect(screen.getAllByLabelText(/^provider$/i)).toHaveLength(1)
    expect(provider).toHaveValue('ollama-cloud')

    await user.selectOptions(provider, 'None')
    expect(provider).toHaveValue('')
    expect(screen.queryByLabelText(/^api key$/i)).not.toBeInTheDocument()
  })

  it('surfaces a rejected create without leaving the review step', async () => {
    vi.unstubAllGlobals()
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if ((init?.method ?? 'GET') === 'POST' && url.includes('/agents')) {
          return Promise.resolve(
            new Response(JSON.stringify({ error: 'persona not found' }), {
              status: 400,
            }),
          )
        }
        return Promise.resolve(defaultFetch(url, init))
      }),
    )
    const user = userEvent.setup()
    renderWizard()

    await fillThroughPersona(user)
    await user.click(await screen.findByRole('button', { name: /create agent/i }))

    expect(await screen.findByText('persona not found')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /create agent/i })).toBeInTheDocument()
  })

  it('creates an always-on agent with its floor pinned to its count', async () => {
    const user = userEvent.setup()
    renderWizard()
    await fillThroughPersona(user)

    await user.click(await screen.findByRole('button', { name: /create agent/i }))
    await waitFor(() => expect(postBody(fetchMock)).toBeDefined())
    // An absent minReplicas tells the hub "leave unchanged", so the form always
    // sends a number: here it equals the standing count, i.e. static scaling.
    expect(postBody(fetchMock)?.minReplicas).toBe(postBody(fetchMock)?.replicas)
  })

  it('creates a dormant agent when wake on demand is on', async () => {
    const user = userEvent.setup()
    renderWizard()
    await fillThroughPersona(user)

    const create = await screen.findByRole('button', { name: /create agent/i })
    expect(screen.getByText('Always running')).toBeInTheDocument()
    await user.click(screen.getByRole('checkbox', { name: /wake on demand/i }))
    expect(screen.getByLabelText('Max containers')).toBeInTheDocument()

    await user.click(create)
    await waitFor(() => expect(postBody(fetchMock)).toBeDefined())
    expect(postBody(fetchMock)?.minReplicas).toBe(0)
  })
})
