import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentEnvSection } from './AgentEnvSection'
import type { AgentEnvVar, AgentResponse } from '../../api/agents'
import { renderWithProviders } from '../../test/renderWithProviders'

type Calls = { method: string; url: string; body?: string }[]

const imageEnv = [
  { name: 'LOG_LEVEL', value: 'info' },
  { name: 'API_TOKEN', value: '', secret: true },
]

/**
 * Stubs fetch for the Runtime tab's env section: the referenced image (which
 * carries the shared defaults) and the agent update used to pin overrides.
 */
function stubFetch(opts: { saveStatus?: number } = {}) {
  const calls: Calls = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push({ method, url, body: init?.body ? String(init.body) : undefined })

      if (url.includes('/agent-images/claude-tooling-base')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              id: 'claude-tooling-base:1.4',
              displayName: 'Claude Tooling Base',
              imageURL: 'ghcr.io/ainsel/claude-tooling:1.4',
              tools: [],
              env: imageEnv,
              mcpServers: [],
              enabledSkills: [],
            }),
            { status: 200 },
          ),
        )
      }
      if (url.includes('/agents/a1')) {
        if (method === 'PUT' || method === 'PATCH') {
          const status = opts.saveStatus ?? 200
          if (status !== 200) {
            return Promise.resolve(
              new Response(JSON.stringify({ error: 'invalid env name' }), { status }),
            )
          }
          const body = JSON.parse(String(init?.body ?? '{}')) as {
            env?: AgentEnvVar[]
          }
          // Mirror the hub: secret values come back masked.
          const env = (body.env ?? []).map((e) =>
            e.secret ? { ...e, value: '' } : e,
          )
          return Promise.resolve(
            new Response(
              JSON.stringify({ id: 'a1', name: 'doc-writer', env }),
              { status: 200 },
            ),
          )
        }
        return Promise.resolve(
          new Response(JSON.stringify({ id: 'a1', name: 'doc-writer' }), {
            status: 200,
          }),
        )
      }
      return Promise.resolve(new Response('{}', { status: 200 }))
    }),
  )
  return calls
}

function agentWith(env?: AgentEnvVar[], opts: { noImage?: boolean } = {}): AgentResponse {
  return {
    id: 'a1',
    name: 'doc-writer',
    imageRef: opts.noImage ? undefined : { name: 'claude-tooling-base:1.4' },
    env,
  } as AgentResponse
}

function lastPutBody(calls: Calls): Record<string, unknown> | undefined {
  const put = [...calls].reverse().find((c) => c.method === 'PUT' || c.method === 'PATCH')
  return put?.body ? (JSON.parse(put.body) as Record<string, unknown>) : undefined
}

describe('AgentEnvSection', () => {
  beforeEach(() => {
    stubFetch()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists the image defaults and reports that env is inherited', async () => {
    renderWithProviders(<AgentEnvSection agent={agentWith(undefined)} />)

    await screen.findByText('LOG_LEVEL')
    expect(screen.getByText('API_TOKEN')).toBeInTheDocument()
    expect(
      screen.getByText(/Currently inherited from the runtime image/i),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /save overrides/i })).toBeDisabled()
  })

  it('pins an override to the agent on save', async () => {
    const calls = stubFetch()
    const user = userEvent.setup()
    renderWithProviders(<AgentEnvSection agent={agentWith(undefined)} />)

    await screen.findByText('LOG_LEVEL')
    await user.click(screen.getByRole('button', { name: /add variable/i }))

    const nameInput = await screen.findByLabelText('Variable name')
    await user.type(nameInput, 'LOG_LEVEL')
    await user.type(screen.getByLabelText('Variable value'), 'debug')
    await user.click(screen.getByRole('button', { name: /save overrides/i }))

    await waitFor(() => expect(lastPutBody(calls)?.env).toBeDefined())
    expect(lastPutBody(calls)?.env).toEqual([
      { name: 'LOG_LEVEL', value: 'debug', secret: false },
    ])
  })

  it('marks an image default the agent overrides', async () => {
    renderWithProviders(
      <AgentEnvSection
        agent={agentWith([{ name: 'LOG_LEVEL', value: 'debug' }])}
      />,
    )

    await screen.findByText('LOG_LEVEL')
    expect(screen.getByText('overridden here')).toBeInTheDocument()
    // The inherited hint disappears once the agent has its own list.
    expect(
      screen.queryByText(/Currently inherited from the runtime image/i),
    ).not.toBeInTheDocument()
    expect(screen.getByLabelText('Variable value')).toHaveValue('debug')
  })

  it('keeps a stored secret value when resubmitted unchanged', async () => {
    const calls = stubFetch()
    const user = userEvent.setup()
    renderWithProviders(
      <AgentEnvSection
        agent={agentWith([{ name: 'API_TOKEN', value: '', secret: true }])}
      />,
    )

    const value = await screen.findByLabelText('Variable value')
    // Masked by the hub: empty field, password input, "unchanged" placeholder.
    expect(value).toHaveValue('')
    expect(value).toHaveAttribute('type', 'password')
    expect(value).toHaveAttribute('placeholder', 'unchanged')

    // Touching an unrelated field makes the form dirty without exposing the
    // secret, and saving must still send an empty value.
    await user.click(screen.getByRole('button', { name: /add variable/i }))
    await user.type(screen.getAllByLabelText('Variable name')[1], 'EXTRA')
    await user.type(screen.getAllByLabelText('Variable value')[1], '1')
    await user.click(screen.getByRole('button', { name: /save overrides/i }))

    await waitFor(() => expect(lastPutBody(calls)?.env).toBeDefined())
    expect(lastPutBody(calls)?.env).toEqual([
      { name: 'API_TOKEN', value: '', secret: true },
      { name: 'EXTRA', value: '1', secret: false },
    ])
  })

  it('clears the overrides when every row is removed', async () => {
    const calls = stubFetch()
    const user = userEvent.setup()
    renderWithProviders(
      <AgentEnvSection agent={agentWith([{ name: 'LOG_LEVEL', value: 'debug' }])} />,
    )

    await screen.findByLabelText('Variable value')
    await user.click(screen.getByRole('button', { name: /remove variable/i }))
    await user.click(screen.getByRole('button', { name: /save overrides/i }))

    await waitFor(() => expect(lastPutBody(calls)?.env).toEqual([]))
  })

  it('blocks empty and duplicate names', async () => {
    const calls = stubFetch()
    const user = userEvent.setup()
    renderWithProviders(
      <AgentEnvSection agent={agentWith([{ name: 'LOG_LEVEL', value: 'debug' }])} />,
    )

    await user.click(screen.getByRole('button', { name: /add variable/i }))
    const save = screen.getByRole('button', { name: /save overrides/i })
    expect(await screen.findByText('Name is required.')).toBeInTheDocument()
    expect(save).toBeDisabled()

    await user.type(screen.getAllByLabelText('Variable name')[1], 'LOG_LEVEL')
    // Both conflicting rows are flagged, not just the new one.
    const dupes = await screen.findAllByText('Duplicate name.')
    expect(dupes).toHaveLength(2)
    expect(save).toBeDisabled()
    expect(lastPutBody(calls)).toBeUndefined()
  })

  it('surfaces a rejected save', async () => {
    vi.unstubAllGlobals()
    stubFetch({ saveStatus: 400 })
    const user = userEvent.setup()
    renderWithProviders(<AgentEnvSection agent={agentWith(undefined)} />)

    await screen.findByText('LOG_LEVEL')
    await user.click(screen.getByRole('button', { name: /add variable/i }))
    await user.type(screen.getByLabelText('Variable name'), '1BAD')
    await user.click(screen.getByRole('button', { name: /save overrides/i }))

    expect(await screen.findByText('invalid env name')).toBeInTheDocument()
  })

  it('explains that env needs an image when none is linked', async () => {
    renderWithProviders(<AgentEnvSection agent={agentWith(undefined, { noImage: true })} />)

    expect(await screen.findByText(/No image linked yet/i)).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /environment/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /add variable/i })).not.toBeInTheDocument()
  })
})
