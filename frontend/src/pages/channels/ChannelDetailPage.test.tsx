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
      <Route path="/channels/:kind/:name" element={<ChannelDetailPage />} />
    </Routes>,
    { route },
  )
}

describe('ChannelDetailPage', () => {
  beforeEach(() => {
    localStorage.clear()
    mockFetch()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.clear()
  })

  it('renders a connector channel: timeline + subscribed inboxes', async () => {
    renderAt('/channels/connector/forgejo')
    expect(screen.getByRole('heading', { name: /channel\s+forgejo/i })).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByText('Where forgejo events arrive')).toBeInTheDocument()
      expect(screen.getByText('evt-1')).toBeInTheDocument()
    })
    // the review-bot inbox subscribes to this connector's channel
    await waitFor(() => {
      expect(screen.getByText(/subscribed inboxes/i)).toBeInTheDocument()
      expect(screen.getByText('on-issues')).toBeInTheDocument()
      expect(
        screen.getAllByRole('link', { name: 'review-bot' })[0],
      ).toHaveAttribute('href', '/channels/agent/review-bot')
    })
    // connector channels do not own subscription editing
    expect(screen.queryByText('Subscriptions')).not.toBeInTheDocument()
    // the relation tree shows the edge out of this channel
    expect(screen.getByText(/Relations/i)).toBeInTheDocument()
    expect(screen.getByText('via on-issues')).toBeInTheDocument()
  })

  it('renders an agent inbox channel: subscriptions, schedules, timeline, runs', async () => {
    renderAt('/channels/agent/review-bot')
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /channel\s+review-bot/i })).toBeInTheDocument()
      expect(screen.getByText('The inbox agent review-bot drains')).toBeInTheDocument()
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

  it('renders a custom channel with its bridges, tree, and gated delete', async () => {
    localStorage.setItem(
      'ainsel.customChannels.v1',
      JSON.stringify({
        channels: [
          { id: 'custom:team-inbox', name: 'team-inbox', description: 'grouped', createdAt: 'x' },
        ],
        bridges: [{ id: 'b1', from: 'connector:forgejo', to: 'custom:team-inbox', name: 'all-issues' }],
      }),
    )
    renderAt('/channels/custom/team-inbox')
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /channel\s+team-inbox/i })).toBeInTheDocument()
      expect(screen.getByText('grouped')).toBeInTheDocument()
    })
    // bridge editor lists the inbound bridge and the tree shows it
    expect(screen.getByText(/Bridges . 1/)).toBeInTheDocument()
    expect(screen.getByText('all-issues')).toBeInTheDocument()
    expect(screen.getByText('bridged locally · all-issues')).toBeInTheDocument()
    // custom channels hold no hub events
    expect(screen.queryByText(/Channel timeline/i)).not.toBeInTheDocument()
    // in use → delete is offered but disabled
    const del = screen.getByRole('button', { name: 'Delete channel' })
    expect(del).toBeDisabled()
  })

  it('deletes an unreferenced custom channel after confirmation', async () => {
    localStorage.setItem(
      'ainsel.customChannels.v1',
      JSON.stringify({
        channels: [
          { id: 'custom:scratch', name: 'scratch', description: 'tmp', createdAt: 'x' },
        ],
        bridges: [],
      }),
    )
    renderAt('/channels/custom/scratch')
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Delete channel' })).toBeEnabled()
    })
    screen.getByRole('button', { name: 'Delete channel' }).click()
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Confirm delete' })).toBeInTheDocument()
    })
    screen.getByRole('button', { name: 'Confirm delete' }).click()
    await waitFor(() => {
      const store = JSON.parse(localStorage.getItem('ainsel.customChannels.v1') ?? '{"channels":[1]}')
      expect(store.channels).toHaveLength(0)
    })
  })

  it('has no built-in channels — legacy cron/chat routes render as unknown', async () => {
    renderAt('/channels/builtin/cron')
    // Schedules and chat are direct births on agent inboxes, not channels.
    // The fallback view still shows the historical events for the label.
    await waitFor(() => {
      expect(screen.getByText(/Unknown channel/i)).toBeInTheDocument()
      expect(screen.getByText('id builtin:cron')).toBeInTheDocument()
    })
  })
})
