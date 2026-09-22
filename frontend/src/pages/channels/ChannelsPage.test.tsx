import { afterEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { Route, Routes } from 'react-router-dom'
import { ChannelsPage } from './ChannelsPage'
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
      if (url.includes('/events')) {
        return Promise.resolve(
          new Response(JSON.stringify({ events: [], total: 0 }), { status: 200 }),
        )
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
  beforeEach(mockFetch)
  afterEach(() => vi.unstubAllGlobals())

  it('renders the page heading and flow illustration', () => {
    renderPage()
    expect(screen.getByRole('heading', { name: /channels/i })).toBeInTheDocument()
    expect(screen.getByText(/router/)).toBeInTheDocument()
  })

  it('lists channels derived from connectors, agents and built-ins', async () => {
    renderPage()
    await waitFor(() => {
      expect(screen.getAllByText('forgejo').length).toBeGreaterThan(0)
      expect(screen.getAllByText('review-bot').length).toBeGreaterThan(0)
      expect(screen.getAllByText('Cron').length).toBeGreaterThan(0)
      expect(screen.getAllByText('Chat').length).toBeGreaterThan(0)
    })
    // role badges
    expect(screen.getAllByText('produces').length).toBeGreaterThanOrEqual(3)
    expect(screen.getByText('consumes')).toBeInTheDocument()
  })

  it('renders channel cards as links to the channel detail', async () => {
    renderPage()
    await waitFor(() => expect(screen.getAllByText('forgejo').length).toBeGreaterThan(0))
    const card = screen
      .getAllByText('forgejo')
      .map((el) => el.closest('a'))
      .find((a) => a?.getAttribute('href') === '/channels/forgejo')
    expect(card).not.toBeNull()
  })
})