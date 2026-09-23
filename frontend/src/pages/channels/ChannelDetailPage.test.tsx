import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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
      if (url.includes('/triggers')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              items: [
                {
                  id: 't1',
                  name: 'on-issues',
                  agentRef: 'a1',
                  connectorRef: 'c1',
                  filters: [],
                  status: { agentValid: true, connectorValid: true },
                },
              ],
              total: 1,
              page: 1,
              pageSize: 200,
              totalPages: 1,
            }),
            { status: 200 },
          ),
        )
      }
      if (url.includes('/cron-triggers')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({ items: [], total: 0, page: 1, pageSize: 200, totalPages: 0 }),
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
      <Route path="/channels/:origin/:name" element={<ChannelDetailPage />} />
    </Routes>,
    { route },
  )
}

describe('ChannelDetailPage', () => {
  beforeEach(mockFetch)
  afterEach(() => vi.unstubAllGlobals())

  it('renders a connector channel: timeline + subscribed inboxes', async () => {
    renderAt('/channels/connector/forgejo')
    expect(screen.getByRole('heading', { name: /channel\s+forgejo/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByText('Ingested from the forgejo connector')).toBeInTheDocument()
      expect(screen.getByText('evt-1')).toBeInTheDocument()
    })
    // the review-bot inbox subscribes to this connector's channel
    await waitFor(() => {
      expect(screen.getByText(/subscribed inboxes/i)).toBeInTheDocument()
      expect(screen.getByText('on-issues')).toBeInTheDocument()
      expect(
        screen.getByRole('link', { name: 'review-bot' }),
      ).toHaveAttribute('href', '/channels/agent/review-bot')
    })
    // connector channels do not own subscription editing
    expect(screen.queryByText('Subscriptions')).not.toBeInTheDocument()
  })

  it('renders an agent inbox channel: subscriptions, schedules, timeline, runs', async () => {
    renderAt('/channels/agent/review-bot')
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /channel\s+review-bot/i })).toBeInTheDocument()
      expect(screen.getByText('Inbox of agent review-bot')).toBeInTheDocument()
    })
    // the inbox owns its subscription + schedule configuration
    await waitFor(() => {
      expect(screen.getByText('Subscriptions')).toBeInTheDocument()
      expect(screen.getByText('on-issues')).toBeInTheDocument()
      expect(screen.getByText('Schedules')).toBeInTheDocument()
    })
    await waitFor(() => {
      expect(screen.getByText(/inv-1/)).toBeInTheDocument()
      // SUCCESS appears both for the event fan-out and the invocation run
      expect(screen.getAllByText('SUCCESS').length).toBeGreaterThanOrEqual(2)
    })
  })

  it('marks built-in channels and links their timelines', async () => {
    renderAt('/channels/builtin/cron')
    expect(screen.getByRole('heading', { name: /channel\s+cron/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByText(/born directly on the consuming agent/i)).toBeInTheDocument()
    })
  })
})
