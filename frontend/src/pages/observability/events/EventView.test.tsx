import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor, fireEvent } from '@testing-library/react'
import { Route, Routes } from 'react-router-dom'
import { EventView } from './EventView'
import { renderWithProviders } from '../../../test/renderWithProviders'

const sampleEvent = {
  id: 'evt-1234567890000000000',
  timestamp: '2026-06-10T08:31:25Z',
  connector: 'c-111',
  status: 'matched',
  matches: [
    { trigger: 't1', agent: 'doc-writer' },
    { trigger: 't2', agent: 'review-bot' },
  ],
  payload: { action: 'opened', repository: { full_name: 'AInsel/ainsel' } },
}

const sampleInvocations = {
  invocations: [
    {
      id: 'inv-1',
      agent: 'review-bot',
      agentName: 'review-bot',
      trigger: 't1',
      triggerName: 't1',
      status: 'success',
      durationMs: 14000,
      timestamp: '2026-06-10T08:31:25Z',
    },
  ],
  total: 1,
  capacity: 1000,
  page: 1,
  pageSize: 50,
  totalPages: 1,
}

const sampleConversation = {
  messages: [
    {
      id: 1,
      role: 'user',
      content: '[{"type":"text","text":"Handle the event"}]',
      agentName: 'review-bot',
      createdAt: '2026-06-10T08:31:26Z',
    },
    {
      id: 2,
      role: 'assistant',
      content: '[{"type":"text","text":"Done reviewing"}]',
      model: 'claude-x',
      inputTokens: 100,
      outputTokens: 50,
      stopReason: 'endTurn',
      agentName: 'review-bot',
      createdAt: '2026-06-10T08:31:30Z',
    },
  ],
  total: 2,
}

function mockFetch() {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (url.includes('/invocations') && url.includes('event=')) {
        return Promise.resolve(
          new Response(JSON.stringify(sampleInvocations), { status: 200 }),
        )
      }
      if (url.includes('/observability/conversations') && url.includes('invocation=inv-1')) {
        return Promise.resolve(
          new Response(JSON.stringify(sampleConversation), { status: 200 }),
        )
      }
      if (url.includes('/events/evt-')) {
        return Promise.resolve(
          new Response(JSON.stringify(sampleEvent), { status: 200 }),
        )
      }
      return Promise.resolve(new Response('{}', { status: 200 }))
    }),
  )
}

function renderEventView(route: string) {
  return renderWithProviders(
    <Routes>
      <Route path="/observability/events/:id" element={<EventView />} />
    </Routes>,
    { route },
  )
}

describe('EventView', () => {
  beforeEach(() => {
    mockFetch()
    Object.defineProperty(window, 'scrollY', { value: 0, configurable: true })
  })
  afterEach(() => vi.unstubAllGlobals())

  it('renders the page heading', () => {
    renderEventView('/observability/events/evt-1234567890000000000')
    expect(screen.getByRole('heading', { name: /event\s+detail/i })).toBeInTheDocument()
  })

  it('renders event metadata', async () => {
    renderEventView('/observability/events/evt-1234567890000000000')
    await waitFor(() => expect(screen.getByText('c-111')).toBeInTheDocument())
    expect(screen.getByText('MATCH')).toBeInTheDocument()
  })

  it('renders the absolute timestamp in the When KPI', async () => {
    renderEventView('/observability/events/evt-1234567890000000000')
    await waitFor(() =>
      expect(screen.getByText('2026-06-10 08:31:25Z')).toBeInTheDocument(),
    )
  })

  it('renders matches', async () => {
    renderEventView('/observability/events/evt-1234567890000000000')
    await waitFor(() => expect(screen.getByText('doc-writer')).toBeInTheDocument())
    // review-bot appears in the matches list and in the invocation summary
    expect(screen.getAllByText('review-bot').length).toBeGreaterThanOrEqual(2)
  })

  it('renders payload as formatted JSON', async () => {
    renderEventView('/observability/events/evt-1234567890000000000')
    await waitFor(() => expect(screen.getByText('Payload')).toBeInTheDocument())
    expect(screen.getByText(/opened/)).toBeInTheDocument()
  })

  it('renders the invocation summary with duration and token usage', async () => {
    renderEventView('/observability/events/evt-1234567890000000000')
    await waitFor(() => expect(screen.getByText('150')).toBeInTheDocument())
    expect(screen.getByText('14.0s')).toBeInTheDocument()
    expect(screen.getByText('Invocation inv-1 · review-bot')).toBeInTheDocument()
    const successTag = screen.getByText('success')
    expect(successTag).toBeInTheDocument()
    expect(successTag).toHaveClass('ok')
  })

  it('renders a failed invocation with the error variant', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/invocations') && url.includes('event=')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                ...sampleInvocations,
                invocations: [{ ...sampleInvocations.invocations[0], status: 'failure' }],
              }),
              { status: 200 },
            ),
          )
        }
        if (url.includes('/events/evt-')) {
          return Promise.resolve(new Response(JSON.stringify(sampleEvent), { status: 200 }))
        }
        return Promise.resolve(new Response('{}', { status: 200 }))
      }),
    )
    renderEventView('/observability/events/evt-1234567890000000000')
    const failureTag = await screen.findByText('failure')
    expect(failureTag).toHaveClass('err')
  })

  it('renders the conversation transcript for the invocation', async () => {
    renderEventView('/observability/events/evt-1234567890000000000')
    await waitFor(() => expect(screen.getByText('Done reviewing')).toBeInTheDocument())
    expect(screen.getByText('Handle the event')).toBeInTheDocument()
    expect(screen.getByText(/claude-x/)).toBeInTheDocument()
  })

  it('shows a note when the event has no invocations', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/invocations')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                invocations: [],
                total: 0,
                capacity: 1000,
                page: 1,
                pageSize: 50,
                totalPages: 0,
              }),
              { status: 200 },
            ),
          )
        }
        if (url.includes('/events/evt-')) {
          return Promise.resolve(
            new Response(JSON.stringify(sampleEvent), { status: 200 }),
          )
        }
        return Promise.resolve(new Response('{}', { status: 200 }))
      }),
    )
    renderEventView('/observability/events/evt-1234567890000000000')
    await waitFor(() =>
      expect(screen.getByText('No invocations recorded for this event.')).toBeInTheDocument(),
    )
  })

  it('shows error state when fetch fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response('{}', { status: 500 }))),
    )
    renderEventView('/observability/events/evt-1234567890000000000')
    await waitFor(() => expect(screen.getByText(/failed to load/i)).toBeInTheDocument())
  })

  it('renders invocations before the payload', async () => {
    renderEventView('/observability/events/evt-1234567890000000000')
    const invocationHeading = await screen.findByText('Invocation inv-1 · review-bot')
    const payloadHeading = screen.getByText('Payload')
    expect(
      invocationHeading.compareDocumentPosition(payloadHeading) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  })

  it('caps the payload and invocations regions with a scroll container', async () => {
    renderEventView('/observability/events/evt-1234567890000000000')
    const invocationHeading = await screen.findByText('Invocation inv-1 · review-bot')
    expect(invocationHeading.closest('.scroll-cap')).toBeInTheDocument()
    const payloadPanel = screen.getByText('Payload').closest('.panel')
    expect(payloadPanel?.querySelector('.scroll-cap')).toBeInTheDocument()
  })

  it('shows a back-to-top button only after scrolling', async () => {
    renderEventView('/observability/events/evt-1234567890000000000')
    await screen.findByText('Invocation inv-1 · review-bot')
    expect(screen.queryByRole('button', { name: /back to top/i })).not.toBeInTheDocument()

    const scrollTo = vi.fn()
    Object.defineProperty(window, 'scrollTo', {
      value: scrollTo,
      configurable: true,
      writable: true,
    })
    Object.defineProperty(window, 'scrollY', { value: 500, configurable: true })
    fireEvent.scroll(window)

    const button = await screen.findByRole('button', { name: /back to top/i })
    fireEvent.click(button)
    expect(scrollTo).toHaveBeenCalledWith({ top: 0, behavior: 'smooth' })
  })
})
