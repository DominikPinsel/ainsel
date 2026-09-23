import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { Route, Routes } from 'react-router-dom'
import { ChannelsPage } from './ChannelsPage'
import { renderWithProviders } from '../../test/renderWithProviders'

const channels = [
  {
    id: 'ch-forgejo',
    kind: 'connector',
    name: 'forgejo',
    description: 'Where forgejo events arrive',
    entityRef: 'forgejo',
    counts: { events: 12, unmatched: 3, failed: 1 },
    bridges: 0,
    subscriptions: 1,
  },
  {
    id: 'ch-inbox',
    kind: 'agent',
    name: 'review-bot',
    description: 'The inbox agent review-bot drains',
    entityRef: 'review-bot',
    counts: { events: 9, unmatched: 0, failed: 2 },
    bridges: 0,
    subscriptions: 1,
  },
]

const subscriptions = [
  {
    source: 'trigger',
    refId: 'on-issues',
    name: 'on-issues',
    fromChannel: 'ch-forgejo',
    toChannel: 'ch-inbox',
  },
]

/** Routes every hub call the page makes; records mutations for assertions. */
function mockFetch(overrides: { channels?: unknown[]; subscriptions?: unknown[] } = {}) {
  const items = overrides.channels ?? channels
  const subs = overrides.subscriptions ?? subscriptions
  const calls: { method: string; url: string; body?: unknown }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = (init?.method ?? 'GET').toUpperCase()
      calls.push({ method, url, body: init?.body ? JSON.parse(String(init.body)) : undefined })
      const path = url.split('?')[0]
      if (path.endsWith('/channels')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              items,
              total: items.length,
              page: 1,
              pageSize: 500,
              totalPages: 1,
              window: '24h0m0s',
            }),
            { status: 200 },
          ),
        )
      }
      if (path.endsWith('/channel-subscriptions')) {
        return Promise.resolve(
          new Response(JSON.stringify({ items: subs, total: subs.length }), { status: 200 }),
        )
      }
      if (path.endsWith('/bridges')) {
        return Promise.resolve(
          new Response(JSON.stringify({ id: 'br-1', name: 'bridge' }), { status: 201 }),
        )
      }
      return Promise.resolve(new Response('{}', { status: 200 }))
    }),
  )
  return calls
}

function renderPage() {
  return renderWithProviders(
    <Routes>
      <Route path="/channels" element={<ChannelsPage />} />
    </Routes>,
    { route: '/channels' },
  )
}

describe('ChannelsPage', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('renders the heading and explains the model without role language', () => {
    mockFetch()
    renderPage()
    expect(screen.getByRole('heading', { name: /channels/i })).toBeInTheDocument()
    expect(screen.getByText(/transfers/i)).toBeInTheDocument()
    // produces/consumes are no longer channel properties
    expect(screen.queryByText('produces')).not.toBeInTheDocument()
    expect(screen.queryByText('consumes')).not.toBeInTheDocument()
  })

  it('renders the hub registry in a table with counts already on the row', async () => {
    const calls = mockFetch()
    renderPage()
    // Both channels appear; the connection map links them again by design.
    await waitFor(() => {
      expect(screen.getAllByRole('link', { name: 'forgejo' }).length).toBeGreaterThan(0)
      expect(screen.getAllByRole('link', { name: 'review-bot' }).length).toBeGreaterThan(0)
    })
    expect(screen.getByRole('columnheader', { name: /channel/i })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: /description/i })).toBeInTheDocument()
    expect(screen.getByText('Where forgejo events arrive')).toBeInTheDocument()
    expect(screen.getByText('The inbox agent review-bot drains')).toBeInTheDocument()
    // no built-in cron/chat channels
    expect(screen.queryByText('cron')).not.toBeInTheDocument()
    expect(screen.queryByText('chat')).not.toBeInTheDocument()
    // Counts come from the list response: no per-row event queries.
    expect(calls.filter((c) => c.url.includes('/events'))).toHaveLength(0)
    expect(calls.some((c) => c.url.includes('/agents'))).toBe(false)
    expect(calls.some((c) => c.url.includes('/connectors'))).toBe(false)
  })

  it('does not collapse same-labeled channels: identity is the id', async () => {
    const calls = mockFetch({
      channels: [
        { ...channels[0], name: 'forgejo' },
        { ...channels[1], id: 'ch-inbox2', name: 'forgejo', entityRef: 'forgejo' },
      ],
    })
    renderPage()
    // Two rows, two distinct hrefs — the label is not the identity.
    await waitFor(() => {
      expect(screen.getByText(/Channels · 2/)).toBeInTheDocument()
    })
    const links = screen.getAllByRole('link').map((a) => a.getAttribute('href'))
    expect(links).toContain('/channels/ch-forgejo')
    expect(links).toContain('/channels/ch-inbox2')
    expect(calls.length).toBeGreaterThan(0)
  })

  it('lists real channel-to-channel connections from subscriptions', async () => {
    mockFetch()
    renderPage()
    await waitFor(() => {
      expect(screen.getByText(/Connections/i)).toBeInTheDocument()
      expect(screen.getByText('on-issues')).toBeInTheDocument()
      expect(screen.getByText(/Born in/i)).toBeInTheDocument()
      expect(screen.getByText(/Transferred into/i)).toBeInTheDocument()
    })
    // The edge is labelled with the registry that owns it.
    expect(screen.getByText('trigger')).toBeInTheDocument()
  })

  it('marks a custom channel attached to a subscription as not deletable', async () => {
    mockFetch({
      channels: [
        ...channels,
        {
          id: 'ch-group',
          kind: 'custom',
          name: 'team-inbox',
          description: 'squad grouping',
          counts: { events: 0, unmatched: 0, failed: 0 },
          bridges: 1,
          subscriptions: 1,
        },
      ],
      subscriptions: [
        ...subscriptions,
        {
          source: 'bridge',
          refId: 'br-1',
          name: 'all-issues',
          fromChannel: 'ch-forgejo',
          toChannel: 'ch-group',
        },
      ],
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getAllByRole('link', { name: 'team-inbox' })[0]).toHaveAttribute(
        'href',
        '/channels/ch-group',
      )
    })
    expect(screen.getByText('squad grouping')).toBeInTheDocument()
    expect(screen.getByText('subscribed')).toBeInTheDocument()
    expect(screen.getByText('all-issues')).toBeInTheDocument()
    expect(screen.getByText('bridge')).toBeInTheDocument()
  })

  it('creates a custom channel through the hub API', async () => {
    const calls = mockFetch({ channels: [] })
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'New channel' }))
    fireEvent.change(screen.getByLabelText('Channel name'), { target: { value: 'team-inbox' } })
    fireEvent.change(screen.getByLabelText('Channel description'), {
      target: { value: 'squad grouping' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create channel' }))
    await waitFor(() => {
      const post = calls.find((c) => c.method === 'POST')
      expect(post?.url).toMatch(/\/api\/v1\/channels$/)
      expect(post?.body).toMatchObject({ name: 'team-inbox', description: 'squad grouping' })
    })
  })

  it('deletes a custom channel through the hub API', async () => {
    const calls = mockFetch({
      channels: [
        {
          id: 'ch-group',
          kind: 'custom',
          name: 'team-inbox',
          description: '',
          counts: { events: 0, unmatched: 0, failed: 0 },
          bridges: 0,
          subscriptions: 0,
        },
      ],
      subscriptions: [],
    })
    renderPage()
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /delete channel team-inbox/i })).toBeInTheDocument(),
    )
    fireEvent.click(screen.getByRole('button', { name: /delete channel team-inbox/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Confirm delete' }))
    await waitFor(() => {
      const del = calls.find((c) => c.method === 'DELETE')
      expect(del?.url).toMatch(/\/api\/v1\/channels\/ch-group$/)
    })
  })

  it('explains the empty connection map', async () => {
    mockFetch({ subscriptions: [] })
    renderPage()
    await waitFor(() => {
      expect(screen.getByText(/No subscriptions yet/i)).toBeInTheDocument()
    })
  })
})
