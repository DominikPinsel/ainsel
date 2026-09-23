import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { Route, Routes } from 'react-router-dom'
import { ChannelDetailPage } from './ChannelDetailPage'
import { renderWithProviders } from '../../test/renderWithProviders'

const HOME = {
  id: 'ch-forgejo',
  kind: 'connector',
  name: 'forgejo',
  description: 'Where forgejo events arrive',
  entityRef: 'forgejo',
  counts: { events: 12, unmatched: 3, failed: 1 },
  bridges: 0,
  subscriptions: 1,
}

const INBOX = {
  id: 'ch-inbox',
  kind: 'agent',
  name: 'review-bot',
  description: 'The inbox agent review-bot drains',
  entityRef: 'review-bot',
  counts: { events: 9, unmatched: 0, failed: 2 },
  bridges: 1,
  subscriptions: 1,
}

const GROUP = {
  id: 'ch-group',
  kind: 'custom',
  name: 'team-inbox',
  description: 'grouped',
  counts: { events: 4, unmatched: 0, failed: 0 },
  bridges: 1,
  subscriptions: 1,
}

const CHANNELS = [HOME, INBOX, GROUP]

const SUBS = [
  {
    source: 'trigger',
    refId: 'on-issues',
    name: 'on-issues',
    fromChannel: 'ch-forgejo',
    toChannel: 'ch-inbox',
  },
  {
    source: 'bridge',
    refId: 'br-1',
    name: 'all-issues',
    fromChannel: 'ch-forgejo',
    toChannel: 'ch-group',
  },
]

function mockFetch(over: { channels?: unknown[]; subs?: unknown[] } = {}) {
  const channels = (over.channels ?? CHANNELS) as Record<string, unknown>[]
  const subs = over.subs ?? SUBS
  const calls: { method: string; url: string; body?: unknown }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = (init?.method ?? 'GET').toUpperCase()
      calls.push({ method, url, body: init?.body ? JSON.parse(String(init.body)) : undefined })
      const path = url.split('?')[0]
      const ok = (payload: unknown, status = 200) =>
        Promise.resolve(new Response(JSON.stringify(payload), { status }))

      if (path.endsWith('/channel-subscriptions')) {
        return ok({ items: subs, total: subs.length })
      }
      const channelMatch = path.match(/\/api\/v1\/channels\/([^/]+)$/)
      if (channelMatch) {
        const id = decodeURIComponent(channelMatch[1])
        if (method === 'DELETE') return ok({}, 204)
        const found = channels.find((c) => c.id === id)
        if (!found) return ok({ error: 'channel not found' }, 404)
        return ok({
          ...found,
          incoming: (subs as Record<string, unknown>[]).filter((s) => s.toChannel === id),
          outgoing: (subs as Record<string, unknown>[]).filter((s) => s.fromChannel === id),
        })
      }
      if (path.endsWith('/bridges')) return ok({ id: 'br-new' }, 201)
      if (/\/bridges\//.test(path) && method === 'DELETE') return ok({}, 204)
      if (path.endsWith('/channels')) {
        return ok({
          items: channels,
          total: channels.length,
          page: 1,
          pageSize: 500,
          totalPages: 1,
        })
      }
      if (path.includes('/events')) {
        return ok({
          events: [
            {
              id: 'evt-1',
              timestamp: '2026-06-10T08:31:25Z',
              connector: 'forgejo',
              channelId: 'ch-forgejo',
              status: 'matched',
              matches: [{ trigger: 'on-issues', agent: 'review-bot', runStatus: 'success' }],
            },
          ],
          total: 1,
        })
      }
      if (path.includes('/invocations')) {
        return ok({
          invocations: [
            {
              id: 'inv-1',
              agent: 'review-bot',
              agentName: 'review-bot',
              trigger: 'on-issues',
              triggerName: 'on-issues',
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
        })
      }
      if (path.endsWith('/triggers')) {
        return ok({
          items: [
            {
              id: 't1',
              name: 'on-issues',
              agentRef: 'review-bot',
              connectorRef: 'forgejo',
              filters: [],
              status: { agentValid: true, connectorValid: true },
            },
          ],
          total: 1,
          page: 1,
          pageSize: 200,
          totalPages: 1,
        })
      }
      if (path.includes('/cron-triggers')) {
        return ok({ items: [], total: 0, page: 1, pageSize: 200, totalPages: 0 })
      }
      return ok({ items: [], total: 0, page: 1, pageSize: 50, totalPages: 0 })
    }),
  )
  return calls
}

function renderAt(id: string) {
  return renderWithProviders(
    <Routes>
      <Route path="/channels/:id" element={<ChannelDetailPage />} />
    </Routes>,
    { route: `/channels/${encodeURIComponent(id)}` },
  )
}

beforeEach(() => {
  localStorage.clear()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ChannelDetailPage', () => {
  it('renders a connector channel: timeline + subscribed inboxes', async () => {
    mockFetch()
    renderAt('ch-forgejo')
    await waitFor(() =>
      expect(screen.getByRole('heading', { name: /channel\s+forgejo/i })).toBeInTheDocument(),
    )
    expect(screen.getByText('Where forgejo events arrive')).toBeInTheDocument()
    await waitFor(() => expect(screen.getByText('evt-1')).toBeInTheDocument())
    // the review-bot inbox subscribes to this connector's channel — by id
    await waitFor(() => {
      expect(screen.getByText(/subscribed inboxes/i)).toBeInTheDocument()
      expect(screen.getByText('on-issues')).toBeInTheDocument()
      expect(screen.getAllByRole('link', { name: 'review-bot' })[0]).toHaveAttribute(
        'href',
        '/channels/ch-inbox',
      )
    })
    // connector channels do not own bridge editing
    expect(screen.queryByText(/Bridges/)).not.toBeInTheDocument()
    // the relation tree shows the edge out of this channel
    expect(screen.getByText(/Relations/i)).toBeInTheDocument()
    expect(screen.getByText('via on-issues')).toBeInTheDocument()
  })

  it('shows the counts the hub already computed, without extra event queries', async () => {
    const calls = mockFetch()
    renderAt('ch-forgejo')
    await waitFor(() => expect(screen.getByText('evt-1')).toBeInTheDocument())
    // One timeline request for the panel; the KPI figures ride on the detail
    // response rather than three count round-trips.
    const eventCalls = calls.filter((c) => c.url.includes('/events'))
    expect(eventCalls).toHaveLength(1)
    expect(eventCalls[0].url).toContain('/channels/ch-forgejo/events')
    expect(screen.getByText('12')).toBeInTheDocument() // events 24h
    expect(screen.getByText('3')).toBeInTheDocument() // unmatched 24h
  })

  it('renders an agent inbox channel: subscriptions, schedules, timeline, runs', async () => {
    const calls = mockFetch()
    renderAt('ch-inbox')
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
      expect(screen.getAllByText('SUCCESS').length).toBeGreaterThanOrEqual(1)
    })
    // runs are looked up by the agent's registry ref, not its display name
    const invocations = calls.find((c) => c.url.includes('/invocations'))
    expect(invocations?.url).toContain('agent=review-bot')
  })

  it('renders a custom channel with real bridges, tree, and gated delete', async () => {
    mockFetch()
    renderAt('ch-group')
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /channel\s+team-inbox/i })).toBeInTheDocument()
      expect(screen.getByText('grouped')).toBeInTheDocument()
    })
    // bridge editor lists the inbound bridge and the tree shows it
    expect(screen.getByText(/Bridges · 1/)).toBeInTheDocument()
    expect(screen.getByText('all-issues')).toBeInTheDocument()
    expect(screen.getByText('bridged · all-issues')).toBeInTheDocument()
    // custom channels do not render an event timeline panel
    expect(screen.queryByText(/Channel timeline/i)).not.toBeInTheDocument()
    // in use → delete is offered but disabled
    const del = screen.getByRole('button', { name: 'Delete channel' })
    expect(del).toBeDisabled()
  })

  it('attaches a bridge through the hub API', async () => {
    const calls = mockFetch({ subs: [] })
    renderAt('ch-group')
    await waitFor(() => expect(screen.getByLabelText('Bridge target channel')).toBeInTheDocument())
    fireEvent.change(screen.getByLabelText('Bridge direction'), { target: { value: 'out' } })
    fireEvent.change(screen.getByLabelText('Bridge target channel'), {
      target: { value: 'ch-inbox' },
    })
    fireEvent.change(screen.getByLabelText('Bridge name'), { target: { value: 'everything' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add bridge' }))
    await waitFor(() => {
      const post = calls.find((c) => c.method === 'POST' && c.url.includes('/bridges'))
      expect(post?.url).toContain('/channels/ch-group/bridges')
      expect(post?.body).toMatchObject({ to: 'ch-inbox', name: 'everything' })
    })
  })

  it('detaches a bridge by its id on the source side', async () => {
    const calls = mockFetch()
    renderAt('ch-group')
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Remove bridge all-issues' })).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByRole('button', { name: 'Remove bridge all-issues' }))
    await waitFor(() => {
      const del = calls.find((c) => c.method === 'DELETE' && c.url.includes('/bridges/'))
      expect(del?.url).toContain('/channels/ch-forgejo/bridges/br-1')
    })
  })

  it('deletes an unreferenced custom channel through the hub after confirmation', async () => {
    const calls = mockFetch({ subs: [] })
    renderAt('ch-group')
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Delete channel' })).toBeEnabled(),
    )
    screen.getByRole('button', { name: 'Delete channel' }).click()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Confirm delete' })).toBeInTheDocument(),
    )
    screen.getByRole('button', { name: 'Confirm delete' }).click()
    await waitFor(() => {
      const del = calls.find((c) => c.method === 'DELETE' && !c.url.includes('/bridges/'))
      expect(del?.url).toContain('/channels/ch-group')
    })
  })

  it('reports a channel the registry does not have', async () => {
    mockFetch()
    renderAt('ch-gone')
    await waitFor(() => {
      expect(screen.getByText(/not in the registry/i)).toBeInTheDocument()
      expect(screen.getByText('id ch-gone')).toBeInTheDocument()
    })
  })
})
