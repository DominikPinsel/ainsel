import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  useChatSession,
  useCreateChatSession,
  useDeleteChatSession,
  useSendChatMessage,
  useUpdateChatSession,
  type ChatMessage,
} from '../../api/chat'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { InlineEditCell } from '../../primitives/InlineEditCell'
import { Markdown } from '../../primitives/Markdown'
import { Panel } from '../../primitives/Panel'
import { Titleblock } from '../../layout/Titleblock'
import { formatRelative } from '../../utils/time'
import './chatBusy.css'
import './ChatView.css'

export function ChatView() {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string | undefined }>()
  const [searchParams] = useSearchParams()
  const agentName = searchParams.get('agent') ?? undefined

  // If the route is /chat/new?agent=<name>, create a session on mount.
  const isNew = id === 'new'
  const createMutation = useCreateChatSession()
  const [createdId, setCreatedId] = useState<string | undefined>(undefined)
  // Ref guard: survives React StrictMode's double effect invocation.
  const creatingRef = useRef(false)

  useEffect(() => {
    if (isNew && agentName && !createdId && !creatingRef.current) {
      creatingRef.current = true
      createMutation.mutate(agentName, {
        onSuccess: (session) => {
          setCreatedId(session.id)
          navigate(`/chat/${encodeURIComponent(session.id)}`, { replace: true })
        },
        onError: () => {
          creatingRef.current = false // allow retry on failure
        },
      })
    }
  }, [isNew, agentName, createdId, createMutation, navigate])

  // Use the created ID if we just created one, otherwise the route param.
  const sessionId = createdId ?? id
  const { data: session, isLoading } = useChatSession(
    sessionId && sessionId !== 'new' ? sessionId : undefined,
  )

  // Message input state
  const [input, setInput] = useState('')
  const sendMutation = useSendChatMessage()
  const scrollRef = useRef<HTMLDivElement>(null)

  // Delete confirmation state
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const deleteMutation = useDeleteChatSession()

  // Rename mutation
  const renameMutation = useUpdateChatSession()

  // Auto-scroll to bottom on new messages
  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' })
  }, [session?.messages])

  const handleSend = () => {
    if (!input.trim() || !sessionId || sessionId === 'new') return
    sendMutation.mutate({ sessionId, content: input.trim() }, { onSuccess: () => setInput('') })
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const handleRename = async (name: string) => {
    if (!sessionId || sessionId === 'new') return
    await renameMutation.mutateAsync({ id: sessionId, name })
  }

  const onConfirmDelete = async () => {
    if (!sessionId || sessionId === 'new') return
    await deleteMutation.mutateAsync(sessionId)
    setShowDeleteConfirm(false)
    navigate('/chat')
  }

  if (isNew && !createdId) {
    return (
      <>
        <Titleblock
          crumbs={
            <>
              <>
                Operations / <b>Chat</b>
              </>
            </>
          }
          title={
            <>
              <>
                Starting chat with <em>{agentName}</em>…
              </>
            </>
          }
        />
        <div style={{ padding: '28px 32px' }}>
          <Panel>
            <div className="label" style={{ padding: 14 }}>
              {createMutation.isPending
                ? 'Creating session…'
                : createMutation.error
                  ? `Failed to create session: ${createMutation.error.message}`
                  : 'Loading…'}
            </div>
          </Panel>
        </div>
      </>
    )
  }

  if (isLoading) {
    return (
      <>
        <Titleblock
          crumbs={
            <>
              <>
                Operations / <b>Chat</b>
              </>
            </>
          }
          title={
            <>
              <>Loading…</>
            </>
          }
        />
        <div style={{ padding: '28px 32px' }}>
          <Panel>
            <div className="label" style={{ padding: 14 }}>
              Loading chat…
            </div>
          </Panel>
        </div>
      </>
    )
  }

  if (!session) {
    return (
      <>
        <Titleblock
          crumbs={
            <>
              <>
                Operations / <b>Chat</b>
              </>
            </>
          }
          title={
            <>
              <>Chat not found</>
            </>
          }
        />
        <div style={{ padding: '28px 32px' }}>
          <Panel>
            <div className="label" style={{ padding: 14, color: 'var(--signal)' }}>
              Chat session not found.
            </div>
          </Panel>
        </div>
      </>
    )
  }

  const messages = session.messages ?? []
  const lastMessage = messages[messages.length - 1]
  const isBusy = sendMutation.isPending || lastMessage?.role === 'user'
  const displayName = session.name

  return (
    <div className="chat-page">
      <Titleblock
        crumbs={
          <>
            Operations /{' '}
            <b
              style={{ cursor: 'pointer', color: 'var(--accent)' }}
              onClick={() => navigate('/chat')}
            >
              Chat
            </b>{' '}
            /{' '}
            <InlineEditCell
              value={displayName}
              onCommit={handleRename}
              ariaLabel={`Rename session ${session.id}`}
              recentError={renameMutation.isError}
            />
          </>
        }
        title={
          <>
            {displayName} <em style={{ color: 'var(--ink-3)' }}>· {session.agentName}</em>
          </>
        }
        actions={
          <>
            <Button variant="danger" size="sm" onClick={() => setShowDeleteConfirm(true)}>
              Delete Chat
            </Button>
            <Button onClick={() => navigate('/chat')}>← Back to list</Button>
          </>
        }
      />
      <div className="chat-body">
        <Panel className="cropped chat-panel">
          {isBusy && (
            <div className="chat-busy" role="status" aria-live="polite">
              <span className="chat-busy-dot" />
              {sendMutation.isPending
                ? 'Sending…'
                : 'Agent may be busy and will respond when ready…'}
            </div>
          )}
          <div
            ref={scrollRef}
            style={{
              flex: 1,
              minHeight: 0,
              overflow: 'auto',
              padding: '16px 20px',
              display: 'flex',
              flexDirection: 'column',
              gap: '12px',
            }}
          >
            {messages.length === 0 ? (
              <div
                className="label"
                style={{ textAlign: 'center', marginTop: 40, color: 'var(--ink-3)' }}
              >
                No messages yet. Say something to start the conversation.
              </div>
            ) : (
              messages.map((msg) => <MessageBubble key={msg.id} msg={msg} />)
            )}
          </div>
          <div
            style={{
              borderTop: '2px solid var(--ink)',
              padding: '16px 20px',
              display: 'flex',
              gap: '12px',
              alignItems: 'flex-end',
              background: 'var(--paper-2)',
              flexShrink: 0,
            }}
          >
            <textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Type a message… (Enter to send, Shift+Enter for newline)"
              rows={1}
              style={{
                flex: 1,
                resize: 'none',
                minHeight: 44,
                maxHeight: 120,
                padding: '10px 14px',
                fontFamily: 'inherit',
                fontSize: 'var(--font-body)',
                background: 'var(--paper)',
                border: '2px solid var(--ink-4)',
                borderRadius: 'var(--radius)',
                color: 'var(--ink)',
                outline: 'none',
                boxShadow: 'inset 0 1px 3px rgba(0,0,0,0.1)',
              }}
            />
            <Button
              variant="primary"
              onClick={handleSend}
              disabled={!input.trim() || sendMutation.isPending}
            >
              Send
            </Button>
          </div>
        </Panel>
      </div>

      <ConfirmModal
        open={showDeleteConfirm}
        title="Delete chat session?"
        body={
          <>
            <b>{displayName}</b> and all its messages will be permanently removed.
          </>
        }
        confirmLabel={deleteMutation.isPending ? 'Deleting…' : 'Delete'}
        destructive
        onConfirm={onConfirmDelete}
        onCancel={() => setShowDeleteConfirm(false)}
      />
    </div>
  )
}

function MessageBubble({ msg }: { msg: ChatMessage }) {
  if (msg.role === 'status') {
    return (
      <div style={{ textAlign: 'center', color: 'var(--ink-3)', fontSize: '0.85em' }}>
        <span style={{ fontStyle: 'italic' }}>{msg.content}</span>
        <span style={{ marginLeft: 8 }}>{formatRelative(msg.createdAt)}</span>
      </div>
    )
  }

  const isUser = msg.role === 'user'
  return (
    <div
      style={{
        display: 'flex',
        justifyContent: isUser ? 'flex-end' : 'flex-start',
      }}
    >
      <div
        style={{
          maxWidth: '75%',
          padding: '10px 14px',
          borderRadius: 'var(--radius)',
          background: isUser ? 'var(--accent)' : 'var(--surface-2)',
          color: isUser ? '#fff' : 'var(--ink-1)',
          border: isUser ? 'none' : '1px solid var(--border)',
        }}
      >
        {isUser ? (
          <div style={{ whiteSpace: 'pre-wrap' }}>{msg.content}</div>
        ) : (
          <Markdown source={msg.content} />
        )}
        <div
          style={{
            fontSize: '0.75em',
            marginTop: 4,
            opacity: 0.6,
            textAlign: isUser ? 'right' : 'left',
          }}
        >
          {formatRelative(msg.createdAt)}
        </div>
      </div>
    </div>
  )
}
