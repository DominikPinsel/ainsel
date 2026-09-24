import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentList } from './AgentList'
import { renderWithProviders } from '../../test/renderWithProviders'

describe('AgentList', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/agents')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    id: 'a1',
                    name: 'doc-writer',
                    llm: { model: 'claude-opus-4-7' },
                    imageRef: { name: 'claude-tooling-base:1.4' },
                    status: { ready: true, replicas: 3 },
                  },
                  {
                    id: 'a2',
                    name: 'triage-bot',
                    llm: { model: 'claude-sonnet-4-6' },
                    imageRef: { name: 'claude-tooling-base:1.4' },
                    status: { ready: false, replicas: 0 },
                  },
                ],
                total: 2,
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
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders rows from the agents query', async () => {
    renderWithProviders(<AgentList />, { route: '/agents' })
    await waitFor(() => expect(screen.getByText('doc-writer')).toBeInTheDocument())
    expect(screen.getByText('triage-bot')).toBeInTheDocument()
    expect(screen.getByText('claude-opus-4-7')).toBeInTheDocument()
  })

  it('shows the New Agent header action', async () => {
    renderWithProviders(<AgentList />, { route: '/agents' })
    expect(await screen.findByRole('button', { name: /new agent/i })).toBeInTheDocument()
  })

  it('shows pager info for the current page', async () => {
    renderWithProviders(<AgentList />, { route: '/agents' })
    await waitFor(() => expect(screen.getByText('doc-writer')).toBeInTheDocument())
    const info = screen.getByText(/of/).parentElement as HTMLElement
    expect(info.textContent).toMatch(/01/)
    expect(info.textContent).toMatch(/02/)
    expect(info.textContent).toMatch(/of\s+2/)
  })

  it('changing page size updates URL', async () => {
    renderWithProviders(<AgentList />, { route: '/agents' })
    await screen.findByText('doc-writer')
    const select = screen.getByRole('combobox', { name: /rows per page/i })
    await userEvent.selectOptions(select, '50')
    await waitFor(() =>
      expect(
        (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.some(
          ([url]) => String(url).includes('pageSize=50'),
        ),
      ).toBe(true),
    )
  })

  it('calls a drained queue-scaled agent asleep instead of pending', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if ((init?.method ?? 'GET') === 'GET' && url.includes('/api/v1/agents')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    id: 'a1',
                    name: 'doc-writer',
                    imageRef: { name: 'claude-tooling-base:1.4' },
                    persona: { id: 'p1' },
                    llm: { model: 'glm-5.1:cloud' },
                    replicas: 2,
                    minReplicas: 0,
                    status: {
                      ready: false,
                      replicas: 0,
                      desired: 0,
                      mode: 'queue',
                      reason: 'ScaledToZero',
                    },
                    updatedAt: '2026-06-08T00:00:00Z',
                  },
                  {
                    id: 'a2',
                    name: 'crash-bot',
                    imageRef: { name: 'claude-tooling-base:1.4' },
                    persona: { id: 'p2' },
                    llm: { model: 'glm-5.1:cloud' },
                    replicas: 1,
                    status: { ready: false, replicas: 0 },
                    updatedAt: '2026-06-08T00:00:00Z',
                  },
                ],
                total: 2,
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

    renderWithProviders(<AgentList />, { route: '/agents' })

    // Both agents have zero containers. The opted-in one is parked on purpose;
    // the other one is broken, and the fleet view must not blur them together.
    const row = await screen.findByText('doc-writer')
    expect(row).toBeInTheDocument()
    expect(await screen.findByText('Asleep')).toBeInTheDocument()
    expect(screen.getByText('Pending')).toBeInTheDocument()
  })
})
