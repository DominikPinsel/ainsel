import { useState } from 'react'
import {
  useDeleteCronTrigger,
  useCronTriggers,
  type CronTriggerSummary,
} from '../../api/cronTriggers'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { Dot } from '../../primitives/Dot'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { Tag } from '../../primitives/Tag'
import { ScheduleForm } from './ScheduleForm'

type Props = { agentId: string }

function formatTime(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  return d.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function AgentSchedules({ agentId }: Props) {
  const { data, isLoading, error } = useCronTriggers({ pageSize: 200 })
  const del = useDeleteCronTrigger()
  const [editing, setEditing] = useState<CronTriggerSummary | undefined>(undefined)
  const [creating, setCreating] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<CronTriggerSummary | undefined>(undefined)

  const triggers = (data?.items ?? []).filter((t) => t.agentRef === agentId)

  const columns: readonly Column<CronTriggerSummary>[] = [
    { key: 'name', header: 'Name', cell: (t) => <b>{t.name}</b> },
    {
      key: 'schedule',
      header: 'Schedule',
      width: 140,
      cell: (t) => <code style={{ fontFamily: 'var(--mono)', fontSize: 12 }}>{t.schedule}</code>,
    },
    {
      key: 'prompt',
      header: 'Prompt',
      cell: (t) => (
        <span
          style={{
            display: 'inline-block',
            maxWidth: 400,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            color: 'var(--ink-2)',
          }}
        >
          {t.prompt}
        </span>
      ),
    },
    {
      key: 'enabled',
      header: 'Enabled',
      width: 80,
      align: 'center',
      cell: (t) => (
        <Tag variant={t.enabled ? 'ok' : 'default'}>{t.enabled ? 'ON' : 'OFF'}</Tag>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      width: 130,
      cell: (t) => {
        const valid = t.status?.agentValid !== false && t.status?.scheduleValid !== false
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
      key: 'nextRun',
      header: 'Next run',
      width: 150,
      cell: (t) => <span style={{ fontFamily: 'var(--mono)', fontSize: 11 }}>{formatTime(t.status?.nextRun)}</span>,
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
      title="Schedules"
      right={
        <Button
          variant="primary"
          size="sm"
          onClick={() => {
            setEditing(undefined)
            setCreating(true)
          }}
        >
          ＋ New schedule
        </Button>
      }
      className="cropped"
    >
      {isLoading ? (
        <p className="label">Loading schedules…</p>
      ) : error ? (
        <p className="label" style={{ color: 'var(--signal)' }}>
          Failed to load schedules.
        </p>
      ) : (
        <RegisterTable
          rows={triggers}
          columns={columns}
          rowKey={(t) => t.id}
          rowClassName={(t) =>
            t.status?.agentValid === false || t.status?.scheduleValid === false
              ? 'row-err'
              : undefined
          }
          emptyLabel="No scheduled triggers for this agent yet."
        />
      )}

      {creating ? (
        <ScheduleForm
          agentId={agentId}
          onClose={() => setCreating(false)}
          onSaved={() => setCreating(false)}
        />
      ) : null}

      {editing ? (
        <ScheduleForm
          agentId={agentId}
          trigger={editing}
          onClose={() => setEditing(undefined)}
          onSaved={() => setEditing(undefined)}
        />
      ) : null}

      <ConfirmModal
        open={deleteTarget !== undefined}
        title="Delete schedule?"
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