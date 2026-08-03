import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import { ConnectorDetail } from './ConnectorDetail'
import { renderWithProviders } from '../../test/renderWithProviders'

function defaultFetch(url: string, init?: RequestInit): Response {
  if (init?.method === 'DELETE') return new Response(null, { status: 204 })
  if (init?.method === 'PUT' && url.includes('/connectors/')) {
    return new Response(
      JSON.stringify({
        id: 'c1',
        name: 'insel-monorepo',
        signatureHeader: 'X-Hub-Signature-256',
        disabled: true,
        status: { ready: false },
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
        status: {
          ready: true,
          conditions: [{ type: 'WebhookHealthy', status: 'True', reason: 'OK' }],
        },
        webhookEndpoint: 'https://hub.example/wh/c1',
        webhookSecretValue: 'abc-secret-value-123',
      }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

function disabledFetch(url: string, init?: RequestInit): Response {
  if (init?.method === 'DELETE') return new Response(null, { status: 204 })
  if (url.includes('/connectors/c2')) {
    return new Response(
      JSON.stringify({
        id: 'c2',
        name: 'disabled-connector',
        signatureHeader: 'X-Forgejo-Signature',
        disabled: true,
        status: {
          ready: false,
          conditions: [
            { type: 'Ready', status: 'False', reason: 'Disabled' },
            { type: 'Disabled', status: 'True', reason: 'SpecDisabled' },
          ],
        },
      }),
      { status: 200 },
    )
  }
  return new Response('{}', { status: 200 })
}

describe('ConnectorDetail', () => {
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

  it('renders connector metadata', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/connectors/:id" element={<ConnectorDetail />} />
      </Routes>,
      { route: '/connectors/c1' },
    )
    await waitFor(() =>
      expect(screen.getAllByText('insel-monorepo')[0]).toBeInTheDocument(),
    )
    expect(screen.getByText('X-Hub-Signature-256')).toBeInTheDocument()
  })

  it('displays webhook endpoint and one-time secret', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/connectors/:id" element={<ConnectorDetail />} />
      </Routes>,
      { route: '/connectors/c1' },
    )
    await waitFor(() =>
      expect(screen.getByText('https://hub.example/wh/c1')).toBeInTheDocument(),
    )
    expect(screen.getByText('abc-secret-value-123')).toBeInTheDocument()
    expect(screen.getByText('ONE-TIME')).toBeInTheDocument()
  })

  it('confirm-delete navigates to list', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/connectors/:id" element={<ConnectorDetail />} />
        <Route path="/connectors" element={<div>LIST</div>} />
      </Routes>,
      { route: '/connectors/c1' },
    )
    await screen.findAllByText('insel-monorepo')
    await userEvent.click(screen.getByRole('button', { name: /^delete$/i }))
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: /^delete/i }))
    await waitFor(() => expect(screen.queryByText('LIST')).toBeInTheDocument())
  })

  it('shows disable button for an enabled connector', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/connectors/:id" element={<ConnectorDetail />} />
      </Routes>,
      { route: '/connectors/c1' },
    )
    await screen.findAllByText('insel-monorepo')
    expect(screen.getByRole('button', { name: /disable/i })).toBeInTheDocument()
  })
})

describe('ConnectorDetail disabled', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) =>
        Promise.resolve(disabledFetch(url, init)),
      ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows enable button for a disabled connector', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/connectors/:id" element={<ConnectorDetail />} />
      </Routes>,
      { route: '/connectors/c2' },
    )
    await screen.findAllByText('disabled-connector')
    expect(screen.getByRole('button', { name: /enable/i })).toBeInTheDocument()
  })

  it('shows Disabled status indicator for a paused connector', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/connectors/:id" element={<ConnectorDetail />} />
      </Routes>,
      { route: '/connectors/c2' },
    )
    await screen.findAllByText('disabled-connector')
    expect(screen.getAllByText('Disabled').length).toBeGreaterThanOrEqual(1)
  })
})
