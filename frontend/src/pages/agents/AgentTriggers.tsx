import { useMemo, useState } from 'react'
import { useConnectors } from '../../api/connectors'
import {
  useDeleteTrigger,
  useTriggers,
  type TriggerSummary,
} from '../../api/triggers'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { Dot } from '../../primitives/Dot'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { Tag } from '../../primitives/Tag'
import { TriggerForm } from './TriggerForm'

// Hub-side issue #63 — /api/v1/triggers ignores ?agent= today.
// Fetch a wide page and filter client-side by agentId. When #63 lands,
// switch to useTriggers({ agent: agentId }) and drop the client-side filter.
type Props = { agentId: string; agentName: string }

export function AgentTriggers({ agentId, agentName: _agentName }: Props) {
  const { data, isLoading, error } = useTriggers({ pageSize: 200 })
  const { data: connectorData } = useConnectors({ pageSize: 200 })
  const del = useDeleteTrigger()
  const [editing, setEditing] = useState<TriggerSummary | undefined>(undefined)
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<TriggerSummary | undefined>(
    undefined,
  )

  const triggers = (data?.items ?? []).filter((t) => t.agentRef === agentId)

  const connectorNameById = useMemo(() => {
    const map = new Map<string, string>()
    for (const c of connectorData?.items ?? []) {
      map.set(c.id, c.name)
    }
    return map
  }, [connectorData])

  const columns: readonly Column<TriggerSummary>[] = [
    { key: 'name', header: 'Name', cell: (t) => <b>{t.name}</b> },
    {
      key: 'connector',
      header: 'Connector',
      width: 170,
      cell: (t) =>
        t.connectorRef
          ? connectorNameById.get(t.connectorRef) ?? t.connectorRef
          : '—',
    },
    {
      key: 'filters',
      header: 'Filters',
      width: 80,
      align: 'center',
      cell: (t) => <Tag>{String(t.filters?.length ?? 0)}</Tag>,
    },
    {
      key: 'status',
      header: 'Status',
      width: 130,
      cell: (t) => {
        const valid =
          t.status?.agentValid !== false && t.status?.connectorValid !== false
        return (
          <>
            <Dot state={valid ? 'ok' : 'err'} />{' '}
            <span style={{ color: valid ? undefined : 'var(--signal)' }}>
              {valid ? 'VALID' : 'INVALID'}
            </span>
          </>
        )
      },
    },
    {
      key: 'actions',
      header: '',
      width: 140,
      align: 'right',
      cell: (t) => (
        <span style={{ display: 'inline-flex', gap: 6 }}>
          <Button
            size="sm"
            onClick={() => {
              setCreating(false)
              setEditing(t)
            }}
          >
            Edit
          </Button>
          <Button
            size="sm"
            variant="danger"
            onClick={() => setDeleteTarget(t)}
          >
            Delete
          </Button>
        </span>
      ),
    },
  ]

  const onConfirmDelete = async () => {
    if (!deleteTarget) return
    await del.mutateAsync(deleteTarget.id)
    setDeleteTarget(undefined)
  }

  return (
    <Panel
      title="Triggers"
      right={
        <Button
          variant="primary"
          size="sm"
          onClick={() => {
            setEditing(undefined)
            setCreating(true)
          }}
        >
          ＋ New trigger
        </Button>
      }
      className="cropped"
    >
      {isLoading ? (
        <p className="label">Loading triggers…</p>
      ) : error ? (
        <p className="label" style={{ color: 'var(--signal)' }}>
          Failed to load triggers.
        </p>
      ) : (
        <RegisterTable
          rows={triggers}
          columns={columns}
          rowKey={(t) => t.id}
          rowClassName={(t) =>
            t.status?.agentValid === false || t.status?.connectorValid === false
              ? 'row-err'
              : undefined
          }
          emptyLabel="No triggers for this agent yet."
        />
      )}

      {creating ? (
        <TriggerForm
          agentId={agentId}
          onClose={() => setCreating(false)}
          onSaved={() => setCreating(false)}
        />
      ) : null}

      {editing ? (
        <TriggerForm
          agentId={agentId}
          trigger={editing}
          onClose={() => setEditing(undefined)}
          onSaved={() => setEditing(undefined)}
        />
      ) : null}

      <ConfirmModal
        open={deleteTarget !== undefined}
        title="Delete trigger?"
        body={
          <>
            <b>{deleteTarget?.name}</b> will be permanently removed.
          </>
        }
        confirmLabel={del.isPending ? 'Deleting…' : 'Delete'}
        destructive
        onConfirm={onConfirmDelete}
        onCancel={() => setDeleteTarget(undefined)}
      />
    </Panel>
  )
}
