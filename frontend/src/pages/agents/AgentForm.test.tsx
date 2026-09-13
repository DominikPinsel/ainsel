import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { AgentForm } from './AgentForm'
import { renderWithProviders } from '../../test/renderWithProviders'

const IMAGE_ID = 'claude-tooling-base:1.4'
const PERSONA_ID = '01HXTEST00000000000000000'

function defaultFetch(url: string, init?: RequestInit): Response {
  const method = init?.method ?? 'GET'
  if (url.includes('/agent-images')) {
    return new Response(
      JSON.stringify({
        items: [{ id: IMAGE_ID, displayName: IMAGE_ID }],
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
  if (url.includes('/agents/a1')) {
    if (method === 'PUT' || method === 'PATCH') {
      const body = JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown>
      return new Response(
        JSON.stringify({ id: 'a1', llm: { model: 'm' }, ...body }),
        { status: 200 },
      )
    }
    return new Response(
      JSON.stringify({
        id: 'a1',
        name: 'doc-writer',
        description: 'Writes docs',
        imageRef: { name: IMAGE_ID },
        llm: { model: 'claude-opus-4-7', provider: 'ollama-cloud' },
        persona: { id: PERSONA_ID },
        replicas: 2,
      }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

function renderEditForm() {
  return renderWithProviders(
    <Routes>
      <Route path="/agents/:id/edit" element={<AgentForm />} />
    </Routes>,
    { route: '/agents/a1/edit' },
  )
}

/** Waits until the stored agent has been reset into the form. */
async function waitForPrefill() {
  await waitFor(() =>
    expect(screen.getByLabelText('Name')).toHaveValue('doc-writer'),
  )
}

function putBody(fetchMock: ReturnType<typeof vi.fn>): Record<string, unknown> | undefined {
  const call = fetchMock.mock.calls.find(
    ([u, init]) =>
      typeof u === 'string' &&
      u.includes('/agents/a1') &&
      ['PUT', 'PATCH'].includes((init as RequestInit | undefined)?.method ?? ''),
  )
  if (!call) return undefined
  return JSON.parse((call[1] as RequestInit).body as string) as Record<string, unknown>
}

describe('AgentForm (edit)', () => {
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

  it('pre-fills every field from the stored agent', async () => {
    renderEditForm()
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe(
        'doc-writer',
      ),
    )
    expect((screen.getByLabelText('Model') as HTMLInputElement).value).toBe(
      'claude-opus-4-7',
    )
    expect(screen.getByLabelText(/^provider$/i)).toHaveValue('ollama-cloud')
    expect(screen.getByLabelText(/^persona$/i)).toHaveValue(PERSONA_ID)
    expect(screen.getByLabelText('Replicas')).toHaveValue(2)
    expect(screen.getByLabelText('Image')).toHaveValue(IMAGE_ID)
  })

  it('offers no group field, since an agent keeps its group', async () => {
    renderEditForm()
    await waitFor(() => expect(screen.getByLabelText('Name')).toBeInTheDocument())
    expect(screen.queryByLabelText('Group')).not.toBeInTheDocument()
  })

  it('shows a persona picker populated from usePersonas', async () => {
    renderEditForm()
    expect(await screen.findByLabelText(/^persona$/i)).toBeInTheDocument()
    await waitFor(() =>
      expect(
        screen.getByRole('option', { name: /test-persona/i }),
      ).toBeInTheDocument(),
    )
  })

  it('shows validation errors instead of saving when a required field is cleared', async () => {
    const user = userEvent.setup()
    renderEditForm()
    await waitForPrefill()

    await user.clear(screen.getByLabelText('Name'))
    await user.click(screen.getByRole('button', { name: /^save$/i }))

    expect(await screen.findByText(/name is required/i)).toBeInTheDocument()
    expect(putBody(fetchMock)).toBeUndefined()
  })

  it('saves with a PUT and never sends a groupId', async () => {
    const user = userEvent.setup()
    renderEditForm()
    await waitForPrefill()

    await user.clear(screen.getByLabelText('Description'))
    await user.type(screen.getByLabelText('Description'), 'Writes better docs')
    await user.click(screen.getByRole('button', { name: /^save$/i }))

    await waitFor(() => expect(putBody(fetchMock)).toBeDefined())
    expect(putBody(fetchMock)).toMatchObject({
      name: 'doc-writer',
      description: 'Writes better docs',
      imageRef: { name: IMAGE_ID },
      persona: { id: PERSONA_ID },
      replicas: 2,
    })
    expect(putBody(fetchMock)).not.toHaveProperty('groupId')
  })

  it('sends customProvider URL and API key in the PUT body when Custom is selected', async () => {
    const user = userEvent.setup()
    renderEditForm()
    await waitForPrefill()

    await user.selectOptions(
      screen.getByLabelText(/^provider$/i),
      'custom',
    )
    await user.type(
      await screen.findByLabelText(/provider base url/i),
      'https://api.example.com/v1',
    )
    await user.type(screen.getByLabelText(/^api key$/i), 'sk-test-12345')
    await user.click(screen.getByRole('button', { name: /^save$/i }))

    await waitFor(() => expect(putBody(fetchMock)).toBeDefined())
    expect(putBody(fetchMock)?.customProvider).toEqual({
      url: 'https://api.example.com/v1',
      apiKey: 'sk-test-12345',
    })
  })

  it('omits the credential block when the API key is left blank', async () => {
    const user = userEvent.setup()
    renderEditForm()
    await waitForPrefill()

    await user.clear(screen.getByLabelText('Model'))
    await user.type(screen.getByLabelText('Model'), 'claude-opus-4-8')
    await user.click(screen.getByRole('button', { name: /^save$/i }))

    await waitFor(() => expect(putBody(fetchMock)).toBeDefined())
    // Blank key = keep the stored one, so no block is sent at all.
    expect(putBody(fetchMock)).not.toHaveProperty('ollamaCloud')
    expect(putBody(fetchMock)?.llm).toMatchObject({ model: 'claude-opus-4-8' })
  })

  it('points the API key field at the stored key rather than demanding a new one', async () => {
    renderEditForm()
    await waitForPrefill()
    expect(screen.getByLabelText(/ollama cloud api key/i)).toHaveAttribute(
      'placeholder',
      'Leave blank to keep existing key',
    )
  })
})
