import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { ConnectorList } from './ConnectorList'
import { renderWithProviders } from '../../test/renderWithProviders'

const mockFetch = () => globalThis.fetch as ReturnType<typeof vi.fn>

describe('ConnectorList', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders rows', async () => {
    mockFetch().mockResolvedValue(
      new Response(
        JSON.stringify({
          items: [
            {
              id: 'c1',
              name: 'insel-monorepo',
              signatureHeader: 'X-Hub-Signature-256',
              webhookEndpoint: 'https://hub.example/wh/c1',
              disabled: false,
              status: { ready: true },
            },
          ],
          total: 1,
          page: 1,
          pageSize: 20,
          totalPages: 1,
        }),
        { status: 200 },
      ),
    )
    renderWithProviders(<ConnectorList />, { route: '/connectors' })
    await waitFor(() => expect(screen.getByText('insel-monorepo')).toBeInTheDocument())
  })

  it('shows the New Connector header action', async () => {
    renderWithProviders(<ConnectorList />, { route: '/connectors' })
    expect(await screen.findByRole('button', { name: /new connector/i })).toBeInTheDocument()
  })

  it('shows disabled state for a paused connector', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/connectors')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    id: 'c1',
                    name: 'paused-connector',
                    signatureHeader: 'X-Forgejo-Signature',
                    disabled: true,
                    status: { ready: false },
                  },
                ],
                total: 1,
                page: 1,
                pageSize: 20,
                totalPages: 1,
              }),
              { status: 200 },
            ),
          )
        }
        return Promise.resolve(new Response('{}', { status: 200 }))
      }),
    )
    renderWithProviders(<ConnectorList />, { route: '/connectors' })
    expect(await screen.findByText('Disabled')).toBeInTheDocument()
  })
})
