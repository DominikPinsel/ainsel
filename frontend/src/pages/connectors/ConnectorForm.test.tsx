import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { ConnectorForm } from './ConnectorForm'
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
  if (init?.method === 'POST' && url.includes('/connectors')) {
    return new Response(
      JSON.stringify({
        id: 'new-id',
        name: 'fresh',
        signatureHeader: 'X-Hub-Signature-256',
        webhookEndpoint: 'https://hub.example/wh/new-id',
        webhookSecretValue: 'one-time-secret',
        disabled: false,
      }),
      { status: 200 },
    )
  }
  if (url.includes('/connectors/c1')) {
    return new Response(
      JSON.stringify({
        id: 'c1',
        name: 'insel-monorepo',
        signatureHeader: 'X-Hub-Signature-256',
        disabled: false,
      }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

describe('ConnectorForm', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) =>
        Promise.resolve(defaultFetch(url, init)),
      ),
    )
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    })
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the name field on the new connector form', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/connectors/new" element={<ConnectorForm />} />
      </Routes>,
      { route: '/connectors/new' },
    )
    expect(await screen.findByLabelText('Name')).toBeInTheDocument()
  })

  it('shows validation error when name is empty', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/connectors/new" element={<ConnectorForm />} />
      </Routes>,
      { route: '/connectors/new' },
    )
    await userEvent.click(screen.getByRole('button', { name: /create/i }))
    expect(await screen.findByText(/name is required/i)).toBeInTheDocument()
  })

  it('shows signature header preset selector', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/connectors/new" element={<ConnectorForm />} />
      </Routes>,
      { route: '/connectors/new' },
    )
    expect(await screen.findByLabelText('Signature Header')).toBeInTheDocument()
  })

  it('reveals custom header input when Custom… is selected', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/connectors/new" element={<ConnectorForm />} />
      </Routes>,
      { route: '/connectors/new' },
    )
    await userEvent.selectOptions(screen.getByLabelText('Signature Header'), 'custom')
    expect(await screen.findByLabelText('Custom Header Name')).toBeInTheDocument()
  })

  it('shows one-time secret panel after successful create', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/connectors/new" element={<ConnectorForm />} />
        <Route path="/connectors/:id" element={<div>detail-page</div>} />
      </Routes>,
      { route: '/connectors/new' },
    )
    await userEvent.type(screen.getByLabelText('Name'), 'my-connector')
    await userEvent.selectOptions(await screen.findByLabelText('Group'), 'g1')
    await userEvent.click(screen.getByRole('button', { name: /create/i }))

    expect(await screen.findByText(/this secret will not be shown again/i)).toBeInTheDocument()
    expect(screen.getByText('https://hub.example/wh/new-id')).toBeInTheDocument()
    expect(screen.getByText('one-time-secret')).toBeInTheDocument()
    expect(screen.getByText('ONE-TIME')).toBeInTheDocument()
  })

  it('submits connector with correct signatureHeader in POST body', async () => {
    const fetchMock = vi.fn((url: string, init?: RequestInit) =>
      Promise.resolve(defaultFetch(url, init)),
    )
    vi.stubGlobal('fetch', fetchMock)

    renderWithProviders(
      <Routes>
        <Route path="/connectors/new" element={<ConnectorForm />} />
        <Route path="/connectors/:id" element={<div>detail-page</div>} />
      </Routes>,
      { route: '/connectors/new' },
    )
    await userEvent.type(screen.getByLabelText('Name'), 'acme')
    await userEvent.selectOptions(await screen.findByLabelText('Group'), 'g1')
    await userEvent.click(screen.getByRole('button', { name: /create/i }))

    await screen.findByText(/this secret will not be shown again/i)

    const postCall = fetchMock.mock.calls.find(
      ([, init]) => (init as RequestInit | undefined)?.method === 'POST',
    )
    expect(postCall).toBeDefined()
    const body = JSON.parse((postCall![1] as RequestInit).body as string)
    expect(body).toMatchObject({
      name: 'acme',
      signatureHeader: 'X-Hub-Signature-256',
    })
  })

  it('pre-fills form fields in edit mode', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/connectors/:id/edit" element={<ConnectorForm />} />
      </Routes>,
      { route: '/connectors/c1/edit' },
    )
    await waitFor(() =>
      expect((screen.getByLabelText('Name') as HTMLInputElement).value).toBe(
        'insel-monorepo',
      ),
    )
  })
})
