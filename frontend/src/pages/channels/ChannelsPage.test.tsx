import { afterEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { Route, Routes } from 'react-router-dom'
import { ChannelsPage } from './ChannelsPage'
import { renderWithProviders } from '../../test/renderWithProviders'

function mockFetch(items: { agents: unknown[]; connectors: unknown[] }) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (url.includes('/agents')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({ items: items.agents, total: items.agents.length, page: 1, pageSize: 500, totalPages: 1 }),
            { status: 200 },
          ),
        )
      }
      if (url.includes('/connectors')) {
        return Promise.resolve(
          new Response(
            JSON.stringify({ items: items.connectors, total: items.connectors.length, page: 1, pageSize: 500, totalPages: 1 }),
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

  it('renders the page heading and flow illustration', () => {
    mockFetch({ agents: [], connectors: [] })
    renderPage()
    expect(screen.getByRole('heading', { name: /channels/i })).toBeInTheDocument()
    expect(screen.getAllByText(/subscriptions/i).length).toBeGreaterThan(0)
  })

  it('lists channels derived from connectors, agents and built-ins with descriptions', async () => {
    mockFetch({
      agents: [{ id: 'a1', name: 'review-bot' }],
      connectors: [{ id: 'c1', name: 'forgejo' }],
    })
    renderPage()
    await waitFor(() => {
      expect(screen.getAllByText('forgejo').length).toBeGreaterThan(0)
      expect(screen.getAllByText('review-bot').length).toBeGreaterThan(0)
      expect(screen.getAllByText('cron').length).toBeGreaterThan(0)
      expect(screen.getAllByText('chat').length).toBeGreaterThan(0)
    })
    expect(screen.getByText('Ingested from the forgejo connector')).toBeInTheDocument()
    expect(screen.getByText('Inbox of agent review-bot')).toBeInTheDocument()
    // role badges
    expect(screen.getAllByText('produces').length).toBeGreaterThanOrEqual(3)
    expect(screen.getByText('consumes')).toBeInTheDocument()
  })

  it('does not collapse same-named channels: connector and agent are two cards', async () => {
    mockFetch({
      agents: [{ id: 'a1', name: 'forgejo' }],
      connectors: [{ id: 'c1', name: 'forgejo' }],
    })
    renderPage()
    await waitFor(() => {
      const links = screen
        .getAllByText('forgejo')
        .map((el) => el.closest('a'))
        .filter((a): a is HTMLAnchorElement => a !== null)
      const hrefs = links.map((a) => a.getAttribute('href'))
      expect(hrefs).toContain('/channels/connector/forgejo')
      expect(hrefs).toContain('/channels/agent/forgejo')
    })
    // both descriptions are visible so the two cards are distinguishable
    expect(screen.getByText('Ingested from the forgejo connector')).toBeInTheDocument()
    expect(screen.getByText('Inbox of agent forgejo')).toBeInTheDocument()
  })

  it('links channel cards to the origin-namespaced detail route', async () => {
    mockFetch({
      agents: [{ id: 'a1', name: 'review-bot' }],
      connectors: [{ id: 'c1', name: 'forgejo' }],
    })
    renderPage()
    await waitFor(() => {
      const card = screen
        .getAllByText('forgejo')
        .map((el) => el.closest('a'))
        .find((a) => a?.getAttribute('href') === '/channels/connector/forgejo')
      expect(card).not.toBeUndefined()
    })
  })
})
