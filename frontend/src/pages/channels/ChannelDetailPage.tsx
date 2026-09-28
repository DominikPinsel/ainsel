import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  buildConnections,
  buildRelationGraph,
  channelPath,
  isChannelDeletable,
  useAttachBridge,
  useChannel,
  useChannelEvents,
  useChannelSubscriptions,
  useChannels,
  useDeleteChannel,
  useDetachBridge,
  type ChannelConnection,
  type ChannelDetail,
  type ChannelKind,
  type ChannelSubscription,
  type RelationEndpoint,
} from '../../api/channels'
import { loadMermaid } from '../../primitives/mermaid'
import type { ActivityEntry } from '../../api/events'
import { useInvocations } from '../../api/invocations'
import { AgentSchedules } from '../agents/AgentSchedules'
import { AgentTriggers } from '../agents/AgentTriggers'
import { Titleblock } from '../../layout/Titleblock'
import { Panel } from '../../primitives/Panel'
import { SectionStatus } from '../../primitives/SectionStatus'
import { Tag } from '../../primitives/Tag'
import { formatISO, formatRelative } from '../../utils/time'
import './ChannelsPage.css'

function RunStateTag({ status }: { status?: string }) {
  if (status === 'success') return <Tag variant="ok">SUCCESS</Tag>
  if (status === 'failure') return <Tag variant="err">FAILURE</Tag>
  if (status === 'timeout') return <Tag variant="err">TIMEOUT</Tag>
  if (status === 'running') return <Tag variant="warn">RUNNING</Tag>
  if (status === 'error') return <Tag variant="err">ERR</Tag>
  if (status === 'matched') return <Tag variant="ok">FAN-OUT</Tag>
  if (status === 'unmatched') return <Tag variant="stale">NO MATCH</Tag>
  return <Tag>—</Tag>
}

function EventLine({ entry }: { entry: ActivityEntry }) {
  const runs = entry.matches ?? []
  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'minmax(140px, 1.2fr) 2fr auto',
        gap: 12,
        alignItems: 'baseline',
        borderTop: '1px solid var(--rule-ghost)',
        padding: '10px 2px',
      }}
    >
      <div style={{ display: 'grid', gap: 2 }}>
        <span style={{ fontSize: 13, color: 'var(--ink)' }}>{formatRelative(entry.timestamp)}</span>
        <span style={{ fontSize: 11, color: 'var(--ink-4)' }}>{formatISO(entry.timestamp)}</span>
      </div>
      <div
        style={{
          fontSize: 13,
          color: 'var(--ink-2)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
      >
        <Link
          to={`/observability/events/${encodeURIComponent(entry.id)}`}
          style={{ color: 'inherit' }}
        >
          {entry.id}
        </Link>
        {runs.length > 0 ? (
          <span style={{ color: 'var(--ink-4)' }}> → {runs.map((m) => m.agent).join(', ')}</span>
        ) : null}
      </div>
      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap' }}>
        {runs.length === 0 ? (
          <Tag variant="stale">NO MATCH</Tag>
        ) : (
          runs.map((m, i) => <RunStateTag key={i} status={m.runStatus ?? entry.status} />)
        )}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* Relation graph                                                      */
/* ------------------------------------------------------------------ */

// Mermaid needs a unique render id per diagram instance.
let diagramSeq = 0

function RelationLinks({ label, items }: { label: string; items: RelationEndpoint[] }) {
  if (items.length === 0) return null
  return (
    <div className="relation-line">
      <span className="relation-line-label">{label}</span>
      {items.map((ep, i) => (
        <span key={`${ep.id ?? 'x'}-${i}`} className="relation-line-item">
          {ep.id ? (
            <Link to={channelPath(ep.id)}>{ep.name}</Link>
          ) : (
            <span className="muted">{ep.name}</span>
          )}
          <em>{ep.edge}</em>
        </span>
      ))}
    </div>
  )
}

/**
 * This channel in its graph context: whatever feeds it (one level up, if
 * anything), and below it only the downstream tree — rendered by Mermaid so
 * the layout is a real node-link diagram instead of an indented list. The
 * summary rows carry the same first-tier facts as links, which keeps the
 * panel readable, keyboard-navigable, and testable when the SVG is not.
 */
function RelationGraph({
  channel,
  edges,
}: {
  channel: { id: string; name: string; kind: ChannelKind }
  edges: ChannelConnection[]
}) {
  const navigate = useNavigate()
  const mount = useRef<HTMLDivElement>(null)
  const model = useMemo(
    () => buildRelationGraph({ id: channel.id, name: channel.name }, edges),
    [channel.id, channel.name, edges],
  )

  useEffect(() => {
    if (model.empty) return
    const host = mount.current
    let cancelled = false
    loadMermaid()
      .then((mermaid) => mermaid.render(`channel-relations-${++diagramSeq}`, model.definition))
      .then(({ svg }) => {
        if (cancelled || !host) return
        host.innerHTML = svg
        // Click-through on diagram nodes: the graph is the navigation surface.
        host.querySelectorAll<SVGGElement>('g.node').forEach((node) => {
          const match = /^flowchart-(.+)-\d+$/.exec(node.id ?? '')
          const channelId = match ? model.nodeChannels[match[1]] : undefined
          if (!channelId || channelId === channel.id) return
          node.style.cursor = 'pointer'
          node.addEventListener('click', () => navigate(channelPath(channelId)))
        })
      })
      .catch(() => {
        // No diagram is fine — the summary rows below it stand on their own.
        if (!cancelled && host) host.innerHTML = ''
      })
    return () => {
      cancelled = true
      if (host) host.innerHTML = ''
    }
  }, [model, channel.id, navigate])

  return (
    <Panel title="Relations" className="cropped">
      {model.empty ? (
        <div className="label" style={{ color: 'var(--ink-3)', padding: '6px 0' }}>
          Nothing connects to this channel yet.
          {channel.kind === 'custom' ? ' Add a bridge below to attach other channels to it.' : ''}
        </div>
      ) : (
        <>
          <div className="relation-diagram" ref={mount} aria-hidden="true" />
          <div className="relation-summary">
            <RelationLinks label="fed by" items={model.fedBy} />
            <RelationLinks label="feeds" items={model.feeds} />
          </div>
        </>
      )}
    </Panel>
  )
}

/* ------------------------------------------------------------------ */
/* Bridges (custom channels)                                           */
/* ------------------------------------------------------------------ */

function bridgeEdges(detail: ChannelDetail): ChannelSubscription[] {
  const seen = new Set<string>()
  const out: ChannelSubscription[] = []
  for (const edge of [...detail.incoming, ...detail.outgoing]) {
    if (edge.source !== 'bridge' || seen.has(edge.refId)) continue
    seen.add(edge.refId)
    out.push(edge)
  }
  return out
}

function BridgeEditor({ detail }: { detail: ChannelDetail }) {
  const { data } = useChannels({ pageSize: 500 })
  const add = useAttachBridge()
  const remove = useDetachBridge()
  const [dir, setDir] = useState<'in' | 'out'>('in')
  const [target, setTarget] = useState('')
  const [name, setName] = useState('')

  // A bridge is stored once, on its source channel: reading the detail gives
  // both directions, and detaching must address the source side.
  const bridges = bridgeEdges(detail)
  const candidates = (data?.items ?? []).filter((c) => c.id !== detail.id)

  return (
    <Panel title={`Bridges · ${bridges.length}`} className="cropped">
      <div className="label" style={{ color: 'var(--ink-3)', marginBottom: 8 }}>
        A bridge transfers this channel&apos;s events into another one. At least one end must be a
        custom channel — a plain connector → agent pairing is a trigger, and belongs on the trigger
        screens.
      </div>
      {bridges.map((b) => {
        const inbound = b.toChannel === detail.id
        const otherId = inbound ? b.fromChannel : b.toChannel
        const other = candidates.find((c) => c.id === otherId)
        return (
          <div key={b.refId} className="bridge-row">
            <span className={`tree-arrow ${inbound ? 'in' : 'out'}`}>{inbound ? '↑' : '↓'}</span>
            <b>{b.name}</b>
            <span style={{ color: 'var(--ink-3)' }}>
              {inbound ? 'pulls from ' : 'flows into '}
              {other ? <Link to={channelPath(other.id)}>{other.name}</Link> : otherId}
            </span>
            <button
              className="bridge-remove"
              aria-label={`Remove bridge ${b.name}`}
              onClick={() => remove.mutate({ from: b.fromChannel, bridgeId: b.refId })}
            >
              ×
            </button>
          </div>
        )
      })}
      <form
        className="bridge-form"
        onSubmit={(e) => {
          e.preventDefault()
          if (!target) return
          add.mutate(
            {
              from: dir === 'in' ? target : detail.id,
              to: dir === 'in' ? detail.id : target,
              name: name.trim() || undefined,
            },
            {
              onSuccess: () => {
                setTarget('')
                setName('')
              },
            },
          )
        }}
      >
        <select
          aria-label="Bridge direction"
          value={dir}
          onChange={(e) => setDir(e.target.value as 'in' | 'out')}
        >
          <option value="in">pull into this channel from…</option>
          <option value="out">push from this channel into…</option>
        </select>
        <select
          aria-label="Bridge target channel"
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          required
        >
          <option value="">channel…</option>
          {candidates.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name} ({c.kind})
            </option>
          ))}
        </select>
        <input
          aria-label="Bridge name"
          placeholder="label (optional)"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <button type="submit" disabled={!target || add.isPending}>
          Add bridge
        </button>
        {add.isError && (
          <span style={{ color: 'var(--err)', fontSize: 12 }}>{(add.error as Error).message}</span>
        )}
      </form>
    </Panel>
  )
}

/* ------------------------------------------------------------------ */
/* Subscribers (connector channels)                                    */
/* ------------------------------------------------------------------ */

/** Which inboxes subscribe to this channel, and what they pull. */
function SubscribersPanel({
  channel,
  edges,
}: {
  channel: { id: string }
  edges: ChannelConnection[]
}) {
  const subs = edges.filter((c) => (c.from?.id ?? c.fromRef) === channel.id)
  return (
    <Panel title={`Subscribed inboxes · ${subs.length}`} className="cropped">
      {subs.length === 0 ? (
        <div className="label" style={{ padding: '4px 0' }}>
          No inbox subscribes to this channel yet — events born here stay until one does.
        </div>
      ) : (
        <div>
          {subs.map((sub) => (
            <div
              key={`${sub.source}#${sub.subscription}#${sub.toRef ?? ''}`}
              style={{
                display: 'grid',
                gridTemplateColumns: '2fr auto 1fr',
                gap: 12,
                alignItems: 'baseline',
                borderTop: '1px solid var(--rule-ghost)',
                padding: '10px 2px',
                fontSize: 13,
              }}
            >
              <span>
                <b>{sub.subscription}</b>
                <span className="label" style={{ color: 'var(--ink-4)', fontSize: 11 }}>
                  {' '}
                  {sub.source === 'bridge' ? 'bridge' : 'trigger'}
                </span>
              </span>
              <span style={{ color: 'var(--ink-4)' }}>→</span>
              <span style={{ textAlign: 'right' }}>
                {sub.to ? (
                  <Link to={channelPath(sub.to.id)}>{sub.to.name}</Link>
                ) : (
                  <span style={{ color: 'var(--ink-4)' }}>{sub.toRef ?? 'unknown inbox'}</span>
                )}
              </span>
            </div>
          ))}
        </div>
      )}
    </Panel>
  )
}

/* ------------------------------------------------------------------ */
/* Page                                                                */
/* ------------------------------------------------------------------ */

export function ChannelDetailPage() {
  const { id = '' } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: channel, isLoading, error } = useChannel(id)
  const { data: subs } = useChannelSubscriptions()
  const { data: list } = useChannels({ pageSize: 500 })
  // The tree walks the whole graph, not just this channel's own edges, so it
  // reads the subscription list rather than the detail's two directions.
  const edges = buildConnections(subs?.items, list?.items)

  const isCustom = channel?.kind === 'custom'
  const isAgentInbox = channel?.kind === 'agent'
  const events = useChannelEvents(id, { limit: 50 }, { enabled: !isCustom && !!channel })
  // Runs are keyed by the agent's registry ref — the channel's display name is
  // rename-able and may not be what the invocation records.
  const invocations = useInvocations(
    { agent: channel?.entityRef ?? '', pageSize: 20 },
    { enabled: isAgentInbox && !!channel },
  )

  const del = useDeleteChannel()
  const [confirmDelete, setConfirmDelete] = useState(false)
  const deletable = channel ? isChannelDeletable(channel, edges) : false

  if (isLoading) {
    return (
      <>
        <Titleblock
          crumbs={
            <>
              <Link to="/channels">Channels</Link> / <b>…</b>
            </>
          }
          title={<>Channel</>}
        />
        <div style={{ padding: '28px 32px' }}>
          <Panel className="cropped">
            <SectionStatus state="loading" />
          </Panel>
        </div>
      </>
    )
  }

  if (error || !channel) {
    return (
      <>
        <Titleblock
          crumbs={
            <>
              <Link to="/channels">Channels</Link> / <b>{id}</b>
            </>
          }
          title={<>Channel</>}
        />
        <div style={{ padding: '28px 32px', display: 'grid', gap: 16 }}>
          <div className="label" style={{ color: 'var(--ink-3)' }}>
            This channel is not in the registry — its source may have been deleted.{' '}
            <Tag variant="stale">id {id}</Tag>
          </div>
          <Panel className="cropped">
            {error ? (
              <SectionStatus state="error" />
            ) : (
              <div className="label" style={{ padding: '4px 0' }}>
                Nothing to show.
              </div>
            )}
          </Panel>
        </div>
      </>
    )
  }

  const counts = channel.counts ?? { events: 0, unmatched: 0, failed: 0 }

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Operations / <Link to="/channels">Channels</Link> / <b>{channel.name}</b>
          </>
        }
        title={
          <>
            Channel <em>{channel.name}</em>
          </>
        }
        actions={
          isCustom ? (
            deletable ? (
              confirmDelete ? (
                <span style={{ display: 'inline-flex', gap: 8 }}>
                  <button
                    style={{ color: 'var(--err)' }}
                    onClick={() =>
                      del.mutate(channel.id, { onSuccess: () => navigate('/channels') })
                    }
                  >
                    Confirm delete
                  </button>
                  <button onClick={() => setConfirmDelete(false)}>Keep</button>
                </span>
              ) : (
                <button onClick={() => setConfirmDelete(true)}>Delete channel</button>
              )
            ) : (
              <button disabled title="Remove all subscriptions attached to this channel first">
                Delete channel
              </button>
            )
          ) : null
        }
      />
      <div style={{ padding: '28px 32px', display: 'grid', gap: 24 }}>
        <div className="label" style={{ color: 'var(--ink-3)', marginTop: -12 }}>
          {channel.description || 'No description.'}
          {channel.orphaned && (
            <>
              {' '}
              <Tag variant="stale">orphaned</Tag> its source entity is gone; the channel is kept so
              its history stays reachable.
            </>
          )}
        </div>
        <div
          className="kpi-row"
          style={{ gridTemplateColumns: `repeat(${isAgentInbox ? 3 : isCustom ? 2 : 4}, 1fr)` }}
        >
          <div className="kpi">
            <div className="hd">
              <span className="label">{isAgentInbox ? 'Reached inbox 24h' : 'Events 24h'}</span>
            </div>
            <div className="figure" style={{ fontSize: 22 }}>
              {counts.events}
            </div>
          </div>
          {!isAgentInbox && !isCustom && (
            <div className="kpi">
              <div className="hd">
                <span className="label">Unmatched 24h</span>
              </div>
              <div className="figure" style={{ fontSize: 22 }}>
                {counts.unmatched}
              </div>
            </div>
          )}
          <div className="kpi">
            <div className="hd">
              <span className="label">Failed 24h</span>
            </div>
            <div className={`figure ${counts.failed > 0 ? 'err' : ''}`} style={{ fontSize: 22 }}>
              {counts.failed}
            </div>
          </div>
          {isCustom && (
            <div className="kpi">
              <div className="hd">
                <span className="label">Bridges</span>
              </div>
              <div className="figure" style={{ fontSize: 22 }}>
                {channel.bridges ?? 0}
              </div>
            </div>
          )}
          <div className="kpi">
            <div className="hd">
              <span className="label">Identity</span>
            </div>
            <div className="figure" style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
              <Tag variant={isAgentInbox ? 'ok' : isCustom ? 'warn' : 'default'}>
                {isAgentInbox ? 'agent inbox' : isCustom ? 'custom' : 'connector'}
              </Tag>
              <span
                style={{
                  fontSize: 12,
                  color: 'var(--ink-4)',
                  fontFamily: 'var(--mono, monospace)',
                }}
              >
                {channel.id}
              </span>
            </div>
          </div>
        </div>

        <RelationGraph channel={channel} edges={edges} />
        {isCustom && <BridgeEditor detail={channel} />}

        {isAgentInbox ? (
          channel.entityRef ? (
            <>
              <AgentTriggers agentId={channel.entityRef} agentName={channel.name} />
              <AgentSchedules agentId={channel.entityRef} />
            </>
          ) : (
            <Panel title="Subscriptions" className="cropped">
              <div className="label" style={{ color: 'var(--ink-3)' }}>
                This inbox&apos;s agent no longer exists in the registry — subscriptions were
                configured on the agent and cascade with it.
              </div>
            </Panel>
          )
        ) : channel.kind === 'connector' ? (
          <SubscribersPanel channel={channel} edges={edges} />
        ) : null}

        {!isCustom && (
          <Panel title={isAgentInbox ? 'Inbox timeline' : 'Channel timeline'} className="cropped">
            {events.isLoading ? (
              <SectionStatus state="loading" />
            ) : events.error ? (
              <SectionStatus state="error" />
            ) : (events.data?.events.length ?? 0) === 0 ? (
              <div className="label" style={{ padding: '4px 0' }}>
                No events in this channel in the recent window.
              </div>
            ) : (
              <div>
                {events.data?.events.map((e) => (
                  <EventLine key={e.id} entry={e} />
                ))}
              </div>
            )}
          </Panel>
        )}

        {isAgentInbox ? (
          <Panel title="What the agent did" className="cropped">
            {invocations.isLoading ? (
              <SectionStatus state="loading" />
            ) : invocations.error ? (
              <SectionStatus state="error" />
            ) : (invocations.data?.items.length ?? 0) === 0 ? (
              <div className="label" style={{ padding: '4px 0' }}>
                No runs recorded for this agent yet.
              </div>
            ) : (
              <div>
                {invocations.data?.items.map((inv) => (
                  <div
                    key={inv.id}
                    style={{
                      display: 'grid',
                      gridTemplateColumns: 'minmax(120px, 1fr) 2fr auto auto',
                      gap: 12,
                      alignItems: 'baseline',
                      borderTop: '1px solid var(--rule-ghost)',
                      padding: '10px 2px',
                    }}
                  >
                    <span style={{ fontSize: 13, color: 'var(--ink-2)' }}>
                      {formatRelative(inv.timestamp)}
                    </span>
                    <span
                      style={{
                        fontSize: 13,
                        color: 'var(--ink-3)',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {inv.triggerName ?? inv.trigger ?? '—'} · {inv.id}
                    </span>
                    <RunStateTag status={inv.status} />
                    <span style={{ fontSize: 12, color: 'var(--ink-3)' }}>
                      {inv.durationMs !== undefined
                        ? `${(inv.durationMs / 1000).toFixed(1)}s`
                        : '—'}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </Panel>
        ) : null}
      </div>
    </>
  )
}
