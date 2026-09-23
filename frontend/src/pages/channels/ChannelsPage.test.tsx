import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { Route, Routes } from 'react-router-dom'
import { ChannelsPage } from './ChannelsPage'
import { renderWithProviders } from '../../test/renderWithProviders'

function mockFetch(items: {
  agents?: unknown[]
  connectors?: unknown[]
  triggers?: unknown[]
}) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (url.includes('/agents')) {
        const a = items.agents ?? []
        return Promise.resolve(
          new Response(
            JSON.stringify({ items: a, total: a.length, page: 1, pageSize: 500, totalPages: 1 }),
            { status: 200 },
          ),
        )
      }
      if (url.includes('/connectors')) {
        const c = items.connectors ?? []
        return Promise.resolve(
          new Response(
            JSON.stringify({ items: c, total: c.length, page: 1, pageSize: 500, totalPages: 1 }),
            { status: 200 },
          ),
        )
      }
      if (url.includes('/triggers')) {
        const t = items.triggers ?? []
        return Promise.resolve(
          new Response(
            JSON.stringify({ items: t, total: t.length, page: 1, pageSize: 200, totalPages: 1 }),
            { status: 200 },
          ),
        )
      }
      if (url.includes('/events')) {
        return Promise.resolve(new Response(JSON.stringify({ events: [], total: 0 }), { status: 200 }))
      }
      return Promise.resolve(new Response('{}', { status: 200 }))
    }),
  )
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
    mockFetch({})
    renderPage()
    expect(screen.getByRole('heading', { name: /channels/i })).toBeInTheDocument()
    expect(screen.getByText(/transfers/i)).toBeInTheDocument()
    // produces/consumes are no longer channel properties
    expect(screen.queryByText('produces')).not.toBeInTheDocument()
    expect(screen.queryByText('consumes')).not.toBeInTheDocument()
  })

  it('shows channels in a table with descriptions', async () => {
    mockFetch({
      agents: [{ id: 'a1', name: 'review-bot' }],
      connectors: [{ id: 'c1', name: 'forgejo' }],
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getByRole('link', { name: 'forgejo' })).toBeInTheDocument()
      expect(screen.getByRole('link', { name: 'review-bot' })).toBeInTheDocument()
    })
    expect(screen.getByRole('columnheader', { name: /channel/i })).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: /description/i })).toBeInTheDocument()
    expect(screen.getByText('Where forgejo events arrive')).toBeInTheDocument()
    expect(screen.getByText('The inbox agent review-bot drains')).toBeInTheDocument()
    // no built-in cron/chat channels
    expect(screen.queryByText('cron')).not.toBeInTheDocument()
    expect(screen.queryByText('chat')).not.toBeInTheDocument()
  })

  it('does not collapse same-named channels: connector and agent are two rows', async () => {
    mockFetch({
      agents: [{ id: 'a1', name: 'forgejo' }],
      connectors: [{ id: 'c1', name: 'forgejo' }],
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getAllByRole('link', { name: 'forgejo' })).toHaveLength(2)
    })
    const links = screen.getAllByRole('link').map((a) => a.getAttribute('href'))
    expect(links).toContain('/channels/connector/forgejo')
    expect(links).toContain('/channels/agent/forgejo')
    expect(screen.getByText('Where forgejo events arrive')).toBeInTheDocument()
    expect(screen.getByText('The inbox agent forgejo drains')).toBeInTheDocument()
  })

  it('lists real channel-to-channel connections from subscriptions', async () => {
    mockFetch({
      agents: [{ id: 'a1', name: 'review-bot' }],
      connectors: [{ id: 'c1', name: 'forgejo' }],
      triggers: [
        { id: 't1', name: 'on-issues', agentRef: 'a1', connectorRef: 'c1', filters: [] },
      ],
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getByText(/Connections/i)).toBeInTheDocument()
      expect(screen.getByText('on-issues')).toBeInTheDocument()
      expect(screen.getByText(/Born in/i)).toBeInTheDocument()
      expect(screen.getByText(/Transferred into/i)).toBeInTheDocument()
    })
  })

  it('shows custom channels from local storage, marked as grouping-only', async () => {
    localStorage.setItem(
      'ainsel.customChannels.v1',
      JSON.stringify({
        channels: [
          { id: 'custom:team-inbox', name: 'team-inbox', description: 'squad grouping', createdAt: 'x' },
        ],
        bridges: [{ id: 'b1', from: 'connector:forgejo', to: 'custom:team-inbox', name: 'all-issues' }],
      }),
    )
    mockFetch({
      agents: [{ id: 'a1', name: 'review-bot' }],
      connectors: [{ id: 'c1', name: 'forgejo' }],
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getAllByRole('link', { name: 'team-inbox' })[0]).toHaveAttribute(
        'href',
        '/channels/custom/team-inbox',
      )
    })
    expect(screen.getByText('squad grouping')).toBeInTheDocument()
    expect(screen.getByText('grouping only')).toBeInTheDocument()
    // attached to a bridge → not deletable
    expect(screen.getByText('subscribed')).toBeInTheDocument()
    // the local bridge appears in the connections map, marked as preview
    expect(screen.getByText('all-issues')).toBeInTheDocument()
    expect(screen.getByText('local preview')).toBeInTheDocument()
  })

  it('creates a custom channel through the New channel form', async () => {
    mockFetch({})
    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'New channel' }))
    fireEvent.change(screen.getByLabelText('Channel name'), { target: { value: 'team-inbox' } })
    fireEvent.change(screen.getByLabelText('Channel description'), {
      target: { value: 'squad grouping' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create channel' }))
    await waitFor(() => {
      expect(screen.getAllByRole('link', { name: 'team-inbox' })[0]).toBeInTheDocument()
    })
    const store = JSON.parse(localStorage.getItem('ainsel.customChannels.v1') ?? '{"channels":[]}')
    expect(store.channels[0]).toMatchObject({
      id: 'custom:team-inbox',
      name: 'team-inbox',
      description: 'squad grouping',
    })
  })

  it('explains the empty connection map', async () => {
    mockFetch({
      agents: [{ id: 'a1', name: 'review-bot' }],
      connectors: [{ id: 'c1', name: 'forgejo' }],
    })
    renderPage()
    await waitFor(() => {
      expect(
        screen.getByText(/No subscriptions yet/i),
      ).toBeInTheDocument()
    })
  })
})
