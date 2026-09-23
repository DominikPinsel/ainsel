import { useState } from 'react'
import { Link } from 'react-router-dom'
import {
  buildConnections,
  channelPath,
  isChannelDeletable,
  useChannelCounts,
  useChannels,
  type ChannelConnection,
  type ChannelKind,
  type ChannelSummary,
} from '../../api/channels'
import { useCreateCustomChannel, useCustomChannels, useDeleteCustomChannel } from '../../api/customChannels'
import { useTriggers } from '../../api/triggers'
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

function ChannelCountsCell({ channel }: { channel: ChannelSummary }) {
  const counts = useChannelCounts(channel)
  if (counts.isCustom) {
    return (
      <span className="label" style={{ color: 'var(--ink-4)' }}>
        grouping only
      </span>
    )
  }
  return (
    <span style={{ display: 'flex', gap: 18, justifyContent: 'flex-end' }}>
      <span>
        <b>{counts.events24h}</b>
        <span className="label" style={{ color: 'var(--ink-4)' }}> 24h</span>
      </span>
      <span style={{ color: counts.failed24h > 0 ? 'var(--err)' : undefined }}>
        <b>{counts.failed24h}</b>
        <span className="label" style={{ color: 'var(--ink-4)' }}> failed</span>
      </span>
    </span>
  )
}

/** Create-form for browser-local custom channels. */
function NewCustomChannelForm({ onDone }: { onDone: () => void }) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const create = useCreateCustomChannel()
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        create.mutate({ name, description }, { onSuccess: onDone })
      }}
      style={{ display: 'grid', gap: 10 }}
    >
      <div className="label" style={{ color: 'var(--ink-3)', fontSize: 12 }}>
        Custom channels group subscriptions under one name. They live in this
        browser until the hub gains a channels API.
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
        <span style={{ color: 'var(--err)', fontSize: 12 }}>
          {(create.error as Error).message}
        </span>
      )}
      <span style={{ display: 'flex', gap: 8 }}>
        <button type="submit" disabled={!name.trim()}>
          Create channel
        </button>
        <button type="button" onClick={onDone}>
          Cancel
        </button>
      </span>
    </form>
  )
}

/** Edge cell: resolve the endpoint or show it muted when unresolvable. */
function EndpointCell({ channel, endpointRef }: { channel?: ChannelSummary; endpointRef?: string }) {
  if (channel) return <Link to={channelPath(channel.id)}>{channel.displayName}</Link>
  return <span style={{ color: 'var(--ink-4)' }}>{endpointRef ?? '—'}</span>
}

/** Every subscription in the system as a real channel → channel edge. */
function ConnectionsPanel({ connections }: { connections: ChannelConnection[] }) {
  return (
    <Panel title={`Connections · ${connections.length}`} className="cropped">
      <RegisterTable
        rows={connections}
        rowKey={(r) => `${r.source}#${r.subscription}#${r.fromRef ?? ''}#${r.toRef ?? ''}#${r.edgeId ?? ''}`}
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
                {r.source === 'local' && (
                  <span className="label" style={{ color: 'var(--ink-4)', fontSize: 11 }}>
                    local preview
                  </span>
                )}
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
  const { channels, isLoading, error } = useChannels()
  const { data: triggerData } = useTriggers({ pageSize: 200 })
  const { data: local } = useCustomChannels()
  const connections = buildConnections(channels, triggerData?.items, local?.bridges)
  const [creating, setCreating] = useState(false)
  const [confirmId, setConfirmId] = useState<string | null>(null)
  const del = useDeleteCustomChannel()

  const columns: readonly Column<ChannelSummary>[] = [
    {
      key: 'name',
      header: 'Channel',
      cell: (c) => (
        <span style={{ display: 'flex', gap: 8, alignItems: 'baseline', flexWrap: 'wrap' }}>
          <Link to={channelPath(c.id)} style={{ fontWeight: 600, color: 'var(--ink)' }}>
            {c.displayName}
          </Link>
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
      cell: (c) => <ChannelCountsCell channel={c} />,
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
              title="Channels with bridges attached cannot be deleted"
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
          <button className="channel-delete" onClick={() => setConfirmId(c.id)} aria-label={`Delete channel ${c.name}`}>
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
          Three kinds of channels: a <b>connector</b> always publishes into its own
          channel (auto-provisioned, permanent); every event in an <b>agent</b>{' '}
          channel is prompted to that agent (auto-provisioned, permanent);{' '}
          <b>custom</b> channels are yours — group subscriptions under one name,
          delete them while nothing is attached. Events move between channels when a{' '}
          <b>subscription</b> on the destination transfers them. Identity is the id —
          two channels may share a label (a connector and an agent both named{' '}
          <b>forgejo</b> are two channels).
        </div>

        {creating && (
          <Panel title="New custom channel">
            <NewCustomChannelForm onDone={() => setCreating(false)} />
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
