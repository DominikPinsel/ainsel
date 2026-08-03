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
})
