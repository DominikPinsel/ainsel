import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ChatView } from './ChatView'

// --- Mocks ------------------------------------------------------------------

const deleteChatSessionMock = vi.fn().mockResolvedValue(undefined)

const mockUseChatSession = vi.fn()
const mockUseSendChatMessage = vi.fn()

vi.mock('../../api/chat', () => ({
  useChatSession: (...args: unknown[]) => mockUseChatSession(...args),
  useCreateChatSession: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateChatSession: () => ({ mutate: vi.fn(), isPending: false }),
  useDeleteChatSession: () => ({
    mutateAsync: deleteChatSessionMock,
    isPending: false,
  }),
  useSendChatMessage: (...args: unknown[]) => mockUseSendChatMessage(...args),
}))

// Default session data: last message is `assistant` (not busy)
const defaultSession = {
  id: 'sess-abc',
  name: 'sess-abc',
  agentName: 'Test Agent',
  userId: 'user-1',
  createdAt: '2026-06-22T00:00:00Z',
  updatedAt: '2026-06-22T00:00:00Z',
  messages: [
    { id: 1, sessionId: 'sess-abc', role: 'user', content: 'Hello', tokens: 5, createdAt: '2026-06-22T00:00:00Z' },
    { id: 2, sessionId: 'sess-abc', role: 'assistant', content: 'Hi there!', tokens: 10, createdAt: '2026-06-22T00:00:01Z' },
  ],
}

// --- Helpers ----------------------------------------------------------------

function renderView() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/chat/sess-abc']}>
        <Routes>
          <Route path="/chat/:id" element={<ChatView />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  deleteChatSessionMock.mockClear()
  mockUseChatSession.mockReturnValue({ data: defaultSession, isLoading: false })
  mockUseSendChatMessage.mockReturnValue({ mutate: vi.fn(), isPending: false })
})

afterEach(() => {
  vi.clearAllMocks()
})

// --- Tests ------------------------------------------------------------------

describe('ChatView — delete', () => {
  it('renders a Delete Chat button in the titleblock actions', () => {
    renderView()
    expect(screen.getByRole('button', { name: /Delete Chat/i })).toBeInTheDocument()
  })

  it('opens the confirmation modal when Delete Chat is clicked', async () => {
    const user = userEvent.setup()
    renderView()

    expect(screen.queryByRole('dialog')).toBeNull()

    await user.click(screen.getByRole('button', { name: /Delete Chat/i }))

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Delete chat session?')).toBeInTheDocument()
  })

  it('calls deleteChatSession with the session ID on confirm', async () => {
    const user = userEvent.setup()
    renderView()

    await user.click(screen.getByRole('button', { name: /Delete Chat/i }))

    // Scope to the modal dialog to find the confirm button
    const dialog = screen.getByRole('dialog')
    const confirmBtn = within(dialog).getByRole('button', { name: 'Delete' })
    await user.click(confirmBtn)

    expect(deleteChatSessionMock).toHaveBeenCalledWith('sess-abc')
  })

  it('closes the modal without calling the mutation on cancel', async () => {
    const user = userEvent.setup()
    renderView()

    await user.click(screen.getByRole('button', { name: /Delete Chat/i }))

    const cancelBtn = screen.getByRole('button', { name: /Cancel/i })
    await user.click(cancelBtn)

    expect(screen.queryByRole('dialog')).toBeNull()
    expect(deleteChatSessionMock).not.toHaveBeenCalled()
  })
})

describe('ChatView — busy notification', () => {
  it('hides the busy notification when the last message is from the assistant', () => {
    renderView()
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('shows the busy notification when the last message is from the user', () => {
    mockUseChatSession.mockReturnValue({
      data: {
        ...defaultSession,
        messages: [
          { id: 1, sessionId: 'sess-abc', role: 'user', content: 'Hello', tokens: 5, createdAt: '2026-06-22T00:00:00Z' },
        ],
      },
      isLoading: false,
    })
    renderView()
    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.getByText(/Agent may be busy and will respond when ready/i)).toBeInTheDocument()
  })

  it('shows "Sending…" when sendMutation is pending', () => {
    mockUseSendChatMessage.mockReturnValue({ mutate: vi.fn(), isPending: true })
    renderView()
    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.getByText(/Sending/i)).toBeInTheDocument()
  })
})