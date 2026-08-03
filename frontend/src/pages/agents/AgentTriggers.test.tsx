import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AgentTriggers } from './AgentTriggers'
import { renderWithProviders } from '../../test/renderWithProviders'

describe('AgentTriggers', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/triggers')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    id: 't1',
                    name: 'on-pr-open',
                    agentRef: 'a1',
                    connectorRef: 'c-a8bf5238',
                    filters: [],
                  },
                  {
                    id: 't2',
                    name: 'on-issue',
                    agentRef: 'other-agent',
                    connectorRef: 'c-a8bf5238',
                    filters: [],
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
        if (url.includes('/connectors')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    id: 'c-a8bf5238',
                    name: 'forgejo-main',
                    type: 'forgejo',
                    url: 'https://forgejo.example.com',
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
        return Promise.resolve(new Response('{}', { status: 200 }))
      }),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('filters triggers by agent name (client-side until hub #63)', async () => {
    renderWithProviders(
      <AgentTriggers agentId="a1" agentName="code-reviewer-agent" />,
    )
    await waitFor(() =>
      expect(screen.getByText('on-pr-open')).toBeInTheDocument(),
    )
    expect(screen.queryByText('on-issue')).not.toBeInTheDocument()
  })

  it('shows the New trigger action', async () => {
    renderWithProviders(
      <AgentTriggers agentId="a1" agentName="code-reviewer-agent" />,
    )
    expect(
      await screen.findByRole('button', { name: /new trigger/i }),
    ).toBeInTheDocument()
  })

  it('toggles the inline TriggerForm when "New trigger" is clicked', async () => {
    renderWithProviders(
      <AgentTriggers agentId="a1" agentName="code-reviewer-agent" />,
    )
    const newBtn = await screen.findByRole('button', { name: /new trigger/i })
    await userEvent.click(newBtn)
    expect(await screen.findByText(/^new trigger$/i)).toBeInTheDocument()
    // The Agent field is pre-filled from props
    const agentInput = screen.getByLabelText('Agent') as HTMLInputElement
    expect(agentInput.value).toBe('a1')
  })

  it('shows connector name instead of connector id in the trigger table', async () => {
    renderWithProviders(
      <AgentTriggers agentId="a1" agentName="code-reviewer-agent" />,
    )
    // The trigger has connectorRef 'c-a8bf5238' and the connector with that
    // id has name 'forgejo-main'. The table should display the name.
    await waitFor(() =>
      expect(screen.getByText('forgejo-main')).toBeInTheDocument(),
    )
    // The raw id must NOT appear in the table.
    expect(screen.queryByText('c-a8bf5238')).not.toBeInTheDocument()
  })

  it('shows empty state when agent has no triggers', async () => {
    vi.unstubAllGlobals()
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              items: [],
              total: 0,
              page: 1,
              pageSize: 20,
              totalPages: 0,
            }),
            { status: 200 },
          ),
        ),
      ),
    )
    renderWithProviders(<AgentTriggers agentId="a1" agentName="lonely-agent" />)
    await waitFor(() =>
      expect(screen.getByText(/no triggers/i)).toBeInTheDocument(),
    )
  })

  it('deletes a trigger via the confirm modal', async () => {
    const mockFetch = vi.fn(
      (url: string, opts?: { method?: string }) => {
        if (opts?.method === 'DELETE') {
          return Promise.resolve(new Response(null, { status: 204 }))
        }
        if (url.includes('/triggers')) {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                items: [
                  {
                    id: 't1',
                    name: 'on-pr-open',
                    agentRef: 'a1',
                    connectorRef: 'c-a8bf5238',
                    filters: [],
                  },
                ],
                total: 1,
                page: 1,
                pageSize: 20,
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
                items: [
                  {
                    id: 'c-a8bf5238',
                    name: 'forgejo-main',
                    type: 'forgejo',
                    url: 'https://forgejo.example.com',
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
        return Promise.resolve(new Response('{}', { status: 200 }))
      },
    )
    vi.unstubAllGlobals()
    vi.stubGlobal('fetch', mockFetch)

    renderWithProviders(
      <AgentTriggers agentId="a1" agentName="code-reviewer-agent" />,
    )

    // Wait for the trigger row to appear
    await waitFor(() =>
      expect(screen.getByText('on-pr-open')).toBeInTheDocument(),
    )

    // Click the Delete button in the trigger row
    const deleteBtn = screen.getByRole('button', { name: /delete/i })
    await userEvent.click(deleteBtn)

    // The confirm modal should appear
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toBeInTheDocument()
    expect(screen.getByText('Delete trigger?')).toBeInTheDocument()

    // Click the confirm Delete button inside the modal
    const confirmBtn = screen
      .getAllByRole('button', { name: /delete/i })
      .find((btn) => dialog.contains(btn))!
    await userEvent.click(confirmBtn)

    // Assert DELETE request was fired for the correct trigger
    await waitFor(() => {
      const deleteCalls = mockFetch.mock.calls.filter(
        ([, opts]) => opts?.method === 'DELETE',
      )
      expect(deleteCalls).toHaveLength(1)
      expect(deleteCalls[0][0]).toContain('/triggers/t1')
    })
  })
})
