import { afterEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { Route, Routes } from 'react-router-dom'
import { ChannelDetailPage } from './ChannelDetailPage'
import { renderWithProviders } from '../../test/renderWithProviders'

function mockFetch() {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (url.includes('/agents')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              items: [{ id: 'a1', name: 'review-bot' }],
              total: 1,
              page: 1,
              pageSize: 500,
              totalPages: 1,
            }),
            { status: 200 },
          ),
        )
      }
      if (url.includes('/connectors')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              items: [{ id: 'c1', name: 'forgejo' }],
              total: 1,
              page: 1,
              pageSize: 500,
              totalPages: 1,
            }),
            { status: 200 },
          ),
        )
      }
      if (url.includes('/invocations')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              invocations: [
                {
                  id: 'inv-1',
                  agent: 'review-bot',
                  agentName: 'review-bot',
                  trigger: 't1',
                  triggerName: 't1',
                  status: 'success',
                  durationMs: 12000,
                  timestamp: '2026-06-10T08:31:25Z',
                },
              ],
              total: 1,
              capacity: 1000,
              page: 1,
              pageSize: 20,
              totalPages: 1,
            }),
            { status: 200 },
          ),
        )
      }
      if (url.includes('/events')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              events: [
                {
                  id: 'evt-1',
                  timestamp: '2026-06-10T08:31:25Z',
                  connector: 'forgejo',
                  status: 'matched',
                  matches: [{ trigger: 't1', agent: 'review-bot', runStatus: 'success' }],
                },
              ],
              total: 1,
            }),
            { status: 200 },
          ),
        )
      }
      return Promise.resolve(new Response('{}', { status: 200 }))
    }),
  )
}

function renderAt(route: string) {
  return renderWithProviders(
    <Routes>
      <Route path="/channels/:name" element={<ChannelDetailPage />} />
    </Routes>,
    { route },
  )
}

describe('ChannelDetailPage', () => {
  beforeEach(mockFetch)
  afterEach(() => vi.unstubAllGlobals())

  it('renders the producing channel timeline (connector scope)', async () => {
    renderAt('/channels/forgejo')
    expect(screen.getByRole('heading', { name: /channel\s+forgejo/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByText('Timeline')).toBeInTheDocument()
      expect(screen.getByText('evt-1')).toBeInTheDocument()
    })
    // producing-only channels have no consumer panel content
    expect(screen.getByText(/no consumer on this channel/i)).toBeInTheDocument()
  })

  it('renders consumed-channel runs for an agent channel', async () => {
    renderAt('/channels/review-bot')
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /channel\s+review-bot/i })).toBeInTheDocument()
    })
    await waitFor(() => {
      expect(screen.getByText(/inv-1/)).toBeInTheDocument()
      // SUCCESS appears both for the event fan-out and the invocation run
      expect(screen.getAllByText('SUCCESS').length).toBeGreaterThanOrEqual(2)
    })
  })
})