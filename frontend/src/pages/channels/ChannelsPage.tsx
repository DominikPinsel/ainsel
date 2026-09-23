import { useState } from 'react'
import { Link } from 'react-router-dom'
import {
  buildConnections,
  channelPath,
  isChannelDeletable,
  useChannelSubscriptions,
  useChannels,
  useCreateChannel,
  useDeleteChannel,
  type ChannelConnection,
  type ChannelKind,
  type ChannelView,
} from '../../api/channels'
import { Titleblock } from '../../layout/Titleblock'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { SectionStatus } from '../../primitives/SectionStatus'
import { Tag } from '../../primitives/Tag'
import './ChannelsPage.css'

const KIND_LABEL: Record<ChannelKind, string> = {
  connector: 'connector',
  agent: 'agent inbox',
  custom: 'custom',
}

function KindTag({ kind }: { kind: ChannelKind }) {
  return (
    <Tag variant={kind === 'agent' ? 'ok' : kind === 'custom' ? 'warn' : 'default'}>
      {KIND_LABEL[kind]}
    </Tag>
  )
}

/** Create-form for a hub-side custom (grouping) channel. */
function NewChannelForm({ onDone }: { onDone: () => void }) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const create = useCreateChannel()
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        create.mutate(
          { name: name.trim(), description: description.trim() || undefined },
          { onSuccess: onDone },
        )
      }}
      style={{ display: 'grid', gap: 10 }}
    >
      <div className="label" style={{ color: 'var(--ink-3)', fontSize: 12 }}>
        A custom channel groups subscriptions under one name and hands them to other agents.
        Connector and agent channels are provisioned by the platform.
      </div>
      <input
        aria-label="Channel name"
        placeholder="team-inbox"
        value={name}
        onChange={(e) => setName(e.target.value)}
        autoFocus
      />
      <input
        aria-label="Channel description"
        placeholder="what is this channel for? (optional)"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
      />
      {create.isError && (
        <span style={{ color: 'var(--err)', fontSize: 12 }}>{(create.error as Error).message}</span>
      )}
      <span style={{ display: 'flex', gap: 8 }}>
        <button type="submit" disabled={!name.trim() || create.isPending}>
          Create channel
        </button>
        <button type="button" onClick={onDone}>
          Cancel
        </button>
      </span>
    </form>
  )
}

/** Edge cell: link the endpoint, or show its id muted when unresolvable. */
function EndpointCell({
  channel,
  endpointRef,
}: {
  channel?: { id: string; name: string }
  endpointRef?: string
}) {
  if (channel) return <Link to={channelPath(channel.id)}>{channel.name}</Link>
  return <span style={{ color: 'var(--ink-4)' }}>{endpointRef ?? '—'}</span>
}

/** Every subscription in the system as a channel → channel edge. */
function ConnectionsPanel({ connections }: { connections: ChannelConnection[] }) {
  return (
    <Panel title={`Connections · ${connections.length}`} className="cropped">
      <RegisterTable
        rows={connections}
        rowKey={(r) =>
          `${r.source}#${r.subscription}#${r.fromRef ?? ''}#${r.toRef ?? ''}#${r.edgeId ?? ''}`
        }
        withRowNumbers={false}
        emptyLabel="No subscriptions yet — events stay in the channel they were born in until an inbox subscribes to it."
        columns={[
          {
            key: 'from',
            header: 'Born in',
            cell: (r) => <EndpointCell channel={r.from} endpointRef={r.fromRef} />,
          },
          {
            key: 'subscription',
            header: 'Subscription',
            cell: (r) => (
              <span style={{ display: 'flex', gap: 6, alignItems: 'baseline' }}>
                <b>{r.subscription}</b>
                <span className="label" style={{ color: 'var(--ink-4)', fontSize: 11 }}>
                  {r.source === 'bridge' ? 'bridge' : 'trigger'}
                </span>
              </span>
            ),
          },
          {
            key: 'to',
            header: 'Transferred into',
            cell: (r) => <EndpointCell channel={r.to} endpointRef={r.toRef} />,
          },
        ]}
      />
    </Panel>
  )
}

export function ChannelsPage() {
  const { data, isLoading, error } = useChannels({ pageSize: 500 })
  const { data: subs } = useChannelSubscriptions()
  const channels = data?.items ?? []
  const connections = buildConnections(subs?.items, channels)
  const [creating, setCreating] = useState(false)
  const [confirmId, setConfirmId] = useState<string | null>(null)
  const del = useDeleteChannel()

  const columns: readonly Column<ChannelView>[] = [
    {
      key: 'name',
      header: 'Channel',
      cell: (c) => (
        <span style={{ display: 'flex', gap: 8, alignItems: 'baseline', flexWrap: 'wrap' }}>
          <Link to={channelPath(c.id)} style={{ fontWeight: 600, color: 'var(--ink)' }}>
            {c.name}
          </Link>
          {c.orphaned && <Tag variant="stale">orphaned</Tag>}
          <span
            className="label"
            style={{ color: 'var(--ink-4)', fontFamily: 'var(--mono, monospace)', fontSize: 11 }}
          >
            {c.id}
          </span>
        </span>
      ),
      sortable: true,
    },
    { key: 'kind', header: 'Kind', cell: (c) => <KindTag kind={c.kind} /> },
    {
      key: 'description',
      header: 'Description',
      cell: (c) => <span style={{ color: 'var(--ink-2)', fontSize: 13 }}>{c.description}</span>,
    },
    {
      key: 'counts',
      header: 'Last 24h',
      align: 'right',
      cell: (c) => (
        <span style={{ display: 'flex', gap: 18, justifyContent: 'flex-end' }}>
          <span>
            <b>{c.counts?.events ?? 0}</b>
            <span className="label" style={{ color: 'var(--ink-4)' }}>
              {' '}
              events
            </span>
          </span>
          {c.kind === 'connector' && (
            <span>
              <b>{c.counts?.unmatched ?? 0}</b>
              <span className="label" style={{ color: 'var(--ink-4)' }}>
                {' '}
                unmatched
              </span>
            </span>
          )}
          <span style={{ color: (c.counts?.failed ?? 0) > 0 ? 'var(--err)' : undefined }}>
            <b>{c.counts?.failed ?? 0}</b>
            <span className="label" style={{ color: 'var(--ink-4)' }}>
              {' '}
              failed
            </span>
          </span>
        </span>
      ),
    },
    {
      key: 'subscriptions',
      header: 'Subscriptions',
      align: 'right',
      cell: (c) => (
        <span style={{ fontSize: 13, color: 'var(--ink-3)' }}>
          {c.subscriptions ?? 0}
          {c.kind === 'custom' && c.bridges ? (
            <span className="label" style={{ color: 'var(--ink-4)' }}>
              {' '}
              ({c.bridges} bridge)
            </span>
          ) : null}
        </span>
      ),
    },
    {
      key: 'actions',
      header: '',
      align: 'right',
      cell: (c) => {
        if (c.kind !== 'custom') return null
        if (!isChannelDeletable(c, connections)) {
          return (
            <span
              className="label"
              title="Channels with subscriptions attached cannot be deleted"
              style={{ color: 'var(--ink-4)', fontSize: 12 }}
            >
              subscribed
            </span>
          )
        }
        if (confirmId === c.id) {
          return (
            <span style={{ display: 'inline-flex', gap: 6 }} onClick={(e) => e.stopPropagation()}>
              <button
                onClick={() => del.mutate(c.id, { onSettled: () => setConfirmId(null) })}
                style={{ color: 'var(--err)' }}
              >
                Confirm delete
              </button>
              <button onClick={() => setConfirmId(null)}>Keep</button>
            </span>
          )
        }
        return (
          <button
            className="channel-delete"
            onClick={() => setConfirmId(c.id)}
            aria-label={`Delete channel ${c.name}`}
          >
            Delete
          </button>
        )
      },
    },
  ]

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Operations / <b>Channels</b>
          </>
        }
        title={<>Channels</>}
        actions={
          <button onClick={() => setCreating((v) => !v)} disabled={creating}>
            New channel
          </button>
        }
      />
      <div className="channels-page" style={{ padding: '28px 32px', display: 'grid', gap: 24 }}>
        <div className="label" style={{ color: 'var(--ink-3)', marginTop: -8 }}>
          Three kinds of channels: a <b>connector</b> always publishes into its own channel
          (provisioned, permanent); every event in an <b>agent</b> channel is prompted to that agent
          (provisioned, permanent); <b>custom</b> channels are yours — group subscriptions under one
          name, delete them while nothing is attached. Events move between channels when a{' '}
          <b>subscription</b> on the destination transfers them. Identity is the id — two channels
          may share a label (a connector and an agent both named <b>forgejo</b> are two channels).
        </div>

        {creating && (
          <Panel title="New custom channel">
            <NewChannelForm onDone={() => setCreating(false)} />
          </Panel>
        )}

        {isLoading ? (
          <Panel className="cropped">
            <SectionStatus state="loading" />
          </Panel>
        ) : error ? (
          <Panel className="cropped">
            <SectionStatus state="error" />
          </Panel>
        ) : (
          <Panel title={`Channels · ${channels.length}`} className="cropped">
            <RegisterTable
              rows={channels}
              columns={columns}
              rowKey={(c) => c.id}
              withRowNumbers={false}
              emptyLabel="No channels yet — each connector and each agent provisions one automatically."
            />
          </Panel>
        )}

        <ConnectionsPanel connections={connections} />
      </div>
    </>
  )
}
