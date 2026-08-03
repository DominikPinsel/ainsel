import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ChatList } from './ChatList'

// --- Mocks ------------------------------------------------------------------

vi.mock('../../api/agents', () => ({
  useAgents: () => ({
    data: { items: [{ id: 'agent-1', name: 'Test Agent' }], total: 1 },
  }),
}))

const deleteChatSessionMock = vi.fn().mockResolvedValue(undefined)
vi.mock('../../api/chat', () => ({
  useChatSessions: () => ({
    data: {
      items: [
        { id: 'sess-1', name: 'sess-1', agentName: 'Test Agent', updatedAt: '2026-06-22T00:00:00Z' },
        { id: 'sess-2', name: 'sess-2', agentName: 'Other Agent', updatedAt: '2026-06-22T00:00:00Z' },
      ],
      total: 2,
      page: 1,
      pageSize: 20,
      totalPages: 1,
    },
    isLoading: false,
    error: null,
  }),
  useUpdateChatSession: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteChatSession: () => ({
    mutateAsync: deleteChatSessionMock,
    isPending: false,
  }),
}))

// --- Helpers ----------------------------------------------------------------

function renderList() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <ChatList />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  deleteChatSessionMock.mockClear()
})

afterEach(() => {
  vi.clearAllMocks()
})

// --- Tests ------------------------------------------------------------------

describe('ChatList — delete', () => {
  it('renders a Delete button for each session', () => {
    renderList()
    const deleteButtons = screen.getAllByRole('button', { name: /Delete/i })
    expect(deleteButtons).toHaveLength(2)
  })

  it('opens the confirmation modal when Delete is clicked', async () => {
    const user = userEvent.setup()
    renderList()

    expect(screen.queryByRole('dialog')).toBeNull()

    const deleteButtons = screen.getAllByRole('button', { name: /Delete/i })
    await user.click(deleteButtons[0])

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Delete chat session?')).toBeInTheDocument()
  })

  it('calls deleteChatSession with the correct session ID on confirm', async () => {
    const user = userEvent.setup()
    renderList()

    const deleteButtons = screen.getAllByRole('button', { name: /Delete/i })
    await user.click(deleteButtons[0])

    // Scope to the modal dialog to avoid matching row Delete buttons
    const dialog = screen.getByRole('dialog')
    const confirmBtn = within(dialog).getByRole('button', { name: 'Delete' })
    await user.click(confirmBtn)

    expect(deleteChatSessionMock).toHaveBeenCalledWith('sess-1')
  })

  it('closes the modal without calling the mutation on cancel', async () => {
    const user = userEvent.setup()
    renderList()

    const deleteButtons = screen.getAllByRole('button', { name: /Delete/i })
    await user.click(deleteButtons[0])

    const cancelBtn = screen.getByRole('button', { name: /Cancel/i })
    await user.click(cancelBtn)

    expect(screen.queryByRole('dialog')).toBeNull()
    expect(deleteChatSessionMock).not.toHaveBeenCalled()
  })
})