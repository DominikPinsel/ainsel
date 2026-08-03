import { useNavigate, useSearchParams } from 'react-router-dom'
import { useAgents } from '../../api/agents'
import { useChatSessions, useDeleteChatSession, useUpdateChatSession, type ChatSessionSummary } from '../../api/chat'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { InlineEditCell } from '../../primitives/InlineEditCell'
import { Pager } from '../../primitives/Pager'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { Tag } from '../../primitives/Tag'
import { Titleblock } from '../../layout/Titleblock'
import { formatRelative } from '../../utils/time'
import { useState } from 'react'

const PAGE_SIZE_OPTIONS = [10, 20, 50] as const

export function ChatList() {
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const page = Math.max(1, Number(params.get('page')) || 1)
  const pageSizeParam = Number(params.get('pageSize'))
  const pageSize = (PAGE_SIZE_OPTIONS as readonly number[]).includes(pageSizeParam)
    ? pageSizeParam
    : 20

  const { data, isLoading, error } = useChatSessions({ page, pageSize })
  const { data: agents } = useAgents({ pageSize: 100 })
  const remove = useDeleteChatSession()
  const rename = useUpdateChatSession()

  // New session creation state
  const [showNew, setShowNew] = useState(false)
  const [selectedAgent, setSelectedAgent] = useState('')

  // Delete confirmation state
  const [confirmId, setConfirmId] = useState<string | null>(null)
  const confirmSession = confirmId
    ? (data?.items ?? []).find((s) => s.id === confirmId) ?? null
    : null

  const setPage = (p: number) => {
    const next = new URLSearchParams(params)
    next.set('page', String(p))
    setParams(next, { replace: true })
  }
  const setPageSize = (n: number) => {
    const next = new URLSearchParams(params)
    next.set('pageSize', String(n))
    next.set('page', '1')
    setParams(next, { replace: true })
  }

  const startNewSession = () => {
    if (!selectedAgent) return
    // Navigate to a new chat view; ChatView will create the session
    navigate(`/chat/new?agent=${encodeURIComponent(selectedAgent)}`)
  }

  const onConfirmDelete = async () => {
    if (!confirmId) return
    await remove.mutateAsync(confirmId)
    setConfirmId(null)
  }

  const handleRename = async (id: string, name: string) => {
    await rename.mutateAsync({ id, name })
  }

  const columns: readonly Column<ChatSessionSummary>[] = [
    {
      key: 'name',
      header: 'Session',
      cell: (s) => (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          <InlineEditCell
            value={s.name}
            onCommit={(next) => handleRename(s.id, next)}
            ariaLabel={`Rename session ${s.id}`}
            recentError={rename.isError}
          />
          <span
            style={{ cursor: 'pointer', fontSize: '0.8em', color: 'var(--ink-3)' }}
            onClick={() => navigate(`/chat/${encodeURIComponent(s.id)}`)}
          >
            {s.id}
          </span>
        </div>
      ),
    },
    {
      key: 'agent',
      header: 'Agent',
      width: 160,
      cell: (s) => <Tag>{s.agentName}</Tag>,
    },
    {
      key: 'updated',
      header: 'Last Activity',
      width: 140,
      align: 'right',
      cell: (s) => <span className="num">{formatRelative(s.updatedAt)}</span>,
    },
    {
      key: 'actions',
      header: '',
      width: 80,
      align: 'right',
      cell: (s) => (
        <Button
          variant="danger"
          size="sm"
          onClick={(e) => {
            e.stopPropagation()
            setConfirmId(s.id)
          }}
        >
          Delete
        </Button>
      ),
    },
  ]

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Operations / <b>Chat</b>
          </>
        }
        title={
          <>
            Chat <em>Sessions</em>
          </>
        }
        actions={
          <Button variant="primary" onClick={() => setShowNew(true)}>
            ＋ New Chat
          </Button>
        }
      />
      <div style={{ padding: '28px 32px' }}>
        {showNew ? (
          <div style={{ marginBottom: 16 }}>
            <Panel title="Start a new chat" className="cropped">
            <div style={{ padding: 14, display: 'flex', gap: 12, alignItems: 'center' }}>
              <select
                value={selectedAgent}
                onChange={(e) => setSelectedAgent(e.target.value)}
                style={{ flex: 1, maxWidth: 320 }}
              >
                <option value="">Select an agent…</option>
                {(agents?.items ?? []).map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </select>
              <Button
                variant="primary"
                onClick={startNewSession}
                disabled={!selectedAgent}
              >
                Start
              </Button>
              <Button onClick={() => setShowNew(false)}>Cancel</Button>
            </div>
          </Panel>
          </div>
        ) : null}

        <Panel className="cropped">
          {isLoading ? (
            <div className="label" style={{ padding: 14 }}>
              Loading…
            </div>
          ) : error ? (
            <div className="label" style={{ padding: 14, color: 'var(--signal)' }}>
              Failed to load chat sessions.
            </div>
          ) : data ? (
            <>
              <RegisterTable
                rows={data.items}
                columns={columns}
                rowKey={(s) => s.id}
                rowClassName={() => 'clickable'}
                emptyLabel="No chat sessions yet. Start a new chat with an agent."
              />
              <Pager
                page={page}
                pageSize={pageSize}
                total={data.total}
                pageSizeOptions={PAGE_SIZE_OPTIONS}
                onPageChange={setPage}
                onPageSizeChange={setPageSize}
              />
            </>
          ) : null}
        </Panel>
      </div>

      <ConfirmModal
        open={confirmId !== null}
        title="Delete chat session?"
        body={
          <>
            <b>{confirmSession?.name ?? confirmSession?.id ?? confirmId}</b> and all its messages will be permanently removed.
          </>
        }
        confirmLabel={remove.isPending ? 'Deleting…' : 'Delete'}
        destructive
        onConfirm={onConfirmDelete}
        onCancel={() => setConfirmId(null)}
      />
    </>
  )
}