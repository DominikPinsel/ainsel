import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
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
import { useAddBridge, useCustomChannels, useDeleteCustomChannel, useRemoveBridge } from '../../api/customChannels'
import { useEventsPage } from '../../api/events'
import type { ActivityEntry } from '../../api/events'
import { useInvocations } from '../../api/invocations'
import { useTriggers } from '../../api/triggers'
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
          <span style={{ color: 'var(--ink-4)' }}>
            {' '}
            → {runs.map((m) => m.agent).join(', ')}
          </span>
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
/* Relation tree                                                       */
/* ------------------------------------------------------------------ */

type Neighbor = {
  key: string
  dir: 'in' | 'out'
  subscription: string
  local: boolean
  channelId?: string
  channelName?: string
  endpointRef?: string
}

function neighborsOf(id: string, edges: ChannelConnection[]): Neighbor[] {
  const out: Neighbor[] = []
  for (const e of edges) {
    const fromId = e.from?.id ?? e.fromRef
    const toId = e.to?.id ?? e.toRef
    if (toId === id) {
      out.push({
        key: `in#${e.source}#${e.subscription}#${fromId ?? ''}`,
        dir: 'in',
        subscription: e.subscription,
        local: e.source === 'local',
        channelId: e.from?.id,
        channelName: e.from?.displayName,
        endpointRef: e.from ? undefined : fromId,
      })
    }
    if (fromId === id) {
      out.push({
        key: `out#${e.source}#${e.subscription}#${toId ?? ''}`,
        dir: 'out',
        subscription: e.subscription,
        local: e.source === 'local',
        channelId: e.to?.id,
        channelName: e.to?.displayName,
        endpointRef: e.to ? undefined : toId,
      })
    }
  }
  return out
}

const MAX_DEPTH = 3

function TreeLevel({
  id,
  edges,
  depth,
  visited,
}: {
  id: string
  edges: ChannelConnection[]
  depth: number
  visited: Set<string>
}) {
  if (depth > MAX_DEPTH) return null
  const ns = neighborsOf(id, edges).filter(
    (n) => !n.channelId || !visited.has(n.channelId) || depth === 1,
  )
  if (ns.length === 0) return null
  return (
    <ul className="channel-tree">
      {ns.map((n) => {
        const cycle = n.channelId !== undefined && visited.has(n.channelId)
        return (
          <li key={n.key}>
            <span className={`tree-arrow ${n.dir}`}>{n.dir === 'in' ? '↑' : '↓'}</span>
            {n.channelId ? (
              <Link to={channelPath(n.channelId)} className="tree-node">
                {n.channelName}
              </Link>
            ) : (
              <span className="tree-node muted">{n.endpointRef ?? 'unknown'}</span>
            )}
            <span className="tree-edge">
              {n.local ? `bridged locally · ${n.subscription}` : `via ${n.subscription}`}
            </span>
            {n.channelId && !cycle && (
              <TreeLevel
                id={n.channelId}
                edges={edges}
                depth={depth + 1}
                visited={new Set([...visited, n.channelId])}
              />
            )}
            {cycle && <span className="tree-edge">↺</span>}
          </li>
        )
      })}
    </ul>
  )
}

/** This channel and everything reachable from it — ↑ feeds it, ↓ it
 *  feeds — clickable to walk between channels. */
function RelationTree({ channel, edges }: { channel: ChannelSummary; edges: ChannelConnection[] }) {
  const ns = neighborsOf(channel.id, edges)
  return (
    <Panel title="Relations" className="cropped">
      <div className="tree-root">
        <b>{channel.displayName}</b>{' '}
        <span className="tree-edge">this channel</span>
      </div>
      {ns.length === 0 ? (
        <div className="label" style={{ color: 'var(--ink-3)', padding: '6px 0' }}>
          Nothing connects to this channel yet.
          {channel.kind === 'custom'
            ? ' Add a bridge below to attach other channels to it.'
            : ''}
        </div>
      ) : (
        <TreeLevel id={channel.id} edges={edges} depth={1} visited={new Set([channel.id])} />
      )}
    </Panel>
  )
}

/* ------------------------------------------------------------------ */
/* Bridges (custom channels only, local until the hub API exists)      */
/* ------------------------------------------------------------------ */

function BridgeEditor({ channel, all }: { channel: ChannelSummary; all: ChannelSummary[] }) {
  const { data: local } = useCustomChannels()
  const add = useAddBridge()
  const remove = useRemoveBridge()
  const [dir, setDir] = useState<'in' | 'out'>('in')
  const [target, setTarget] = useState('')
  const [name, setName] = useState('')

  const bridges = local?.bridges ?? []
  const pullFrom = bridges.filter((b) => b.to === channel.id)
  const feeds = bridges.filter((b) => b.from === channel.id)
  const candidates = all.filter((c) => c.id !== channel.id)

  return (
    <Panel title={`Bridges · ${pullFrom.length + feeds.length}`} className="cropped">
      <div className="label" style={{ color: 'var(--ink-3)', marginBottom: 8 }}>
        Local preview — the hub does not know channels yet, so these edges
        visualise the grouping but move no real events.
      </div>
      {pullFrom.map((b) => {
        const other = candidates.find((c) => c.id === b.from)
        return (
          <div key={b.id} className="bridge-row">
            <span className="tree-arrow in">↑</span>
            <b>{b.name}</b>
            <span style={{ color: 'var(--ink-3)' }}>
              pulls from {other ? <Link to={channelPath(b.from)}>{other.displayName}</Link> : b.from}
            </span>
            <button className="bridge-remove" aria-label={`Remove bridge ${b.name}`} onClick={() => remove.mutate(b.id)}>
              ×
            </button>
          </div>
        )
      })}
      {feeds.map((b) => {
        const other = candidates.find((c) => c.id === b.to)
        return (
          <div key={b.id} className="bridge-row">
            <span className="tree-arrow out">↓</span>
            <b>{b.name}</b>
            <span style={{ color: 'var(--ink-3)' }}>
              flows into {other ? <Link to={channelPath(b.to)}>{other.displayName}</Link> : b.to}
            </span>
            <button className="bridge-remove" aria-label={`Remove bridge ${b.name}`} onClick={() => remove.mutate(b.id)}>
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
              from: dir === 'in' ? target : channel.id,
              to: dir === 'in' ? channel.id : target,
              name,
            },
            { onSuccess: () => { setTarget(''); setName('') } },
          )
        }}
      >
        <select aria-label="Bridge direction" value={dir} onChange={(e) => setDir(e.target.value as 'in' | 'out')}>
          <option value="in">pull into this channel from…</option>
          <option value="out">push from this channel into…</option>
        </select>
        <select aria-label="Bridge target channel" value={target} onChange={(e) => setTarget(e.target.value)} required>
          <option value="">channel…</option>
          {candidates.map((c) => (
            <option key={c.id} value={c.id}>
              {c.displayName} ({c.kind})
            </option>
          ))}
        </select>
        <input
          aria-label="Bridge name"
          placeholder="label (optional)"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <button type="submit" disabled={!target}>
          Add bridge
        </button>
      </form>
    </Panel>
  )
}

/* ------------------------------------------------------------------ */
/* Subscribers (connector channels)                                    */
/* ------------------------------------------------------------------ */

/** Which inboxes subscribe to this channel, and what they pull. */
function SubscribersPanel({ channel, edges }: { channel: ChannelSummary; edges: ChannelConnection[] }) {
  const subs = edges.filter((c) => (c.from?.id ?? c.fromRef) === channel.id)
  return (
    <Panel title={`Subscribed inboxes · ${subs.length}`} className="cropped">
      {subs.length === 0 ? (
        <div className="label" style={{ padding: '4px 0' }}>
          No inbox subscribes to this channel yet — events born here stay
          until one does.
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
                {sub.source === 'local' && (
                  <span className="label" style={{ color: 'var(--ink-4)', fontSize: 11 }}> local</span>
                )}
              </span>
              <span style={{ color: 'var(--ink-4)' }}>→</span>
              <span style={{ textAlign: 'right' }}>
                {sub.to ? (
                  <Link to={channelPath(sub.to.id)}>{sub.to.displayName}</Link>
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

const KINDS: ChannelKind[] = ['connector', 'agent', 'custom']

export function ChannelDetailPage() {
  const { kind = 'connector', name = '' } = useParams<{ kind: string; name: string }>()
  const navigate = useNavigate()
  const { channels } = useChannels()
  const { data: triggerData } = useTriggers({ pageSize: 200 })
  const { data: local } = useCustomChannels()
  const edges = buildConnections(channels, triggerData?.items, local?.bridges)

  const key = `${kind}:${name}`
  const channel = channels.find((c) => c.id === key)
  const resolved: ChannelSummary = channel ?? {
    id: key,
    name,
    displayName: name,
    description: '',
    kind: (KINDS as string[]).includes(kind) ? (kind as ChannelKind) : 'connector',
  }
  const isCustom = resolved.kind === 'custom'
  const counts = useChannelCounts(resolved)

  const isAgentInbox = resolved.kind === 'agent'
  const scope = isAgentInbox ? { agent: name } : { connector: name }

  const events = useEventsPage({ ...scope, limit: 50 })
  const showRuns = isAgentInbox
  const invocations = useInvocations({ agent: name, pageSize: 20 }, { enabled: showRuns })

  const del = useDeleteCustomChannel()
  const [confirmDelete, setConfirmDelete] = useState(false)
  const deletable = isChannelDeletable(resolved, edges)

  const title = resolved.displayName

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Operations / <Link to="/channels">Channels</Link> / <b>{title}</b>
          </>
        }
        title={
          <>
            Channel <em>{title}</em>
          </>
        }
        actions={
          isCustom ? (
            deletable ? (
              confirmDelete ? (
                <span style={{ display: 'inline-flex', gap: 8 }}>
                  <button
                    style={{ color: 'var(--err)' }}
                    onClick={() => del.mutate(resolved.id, { onSuccess: () => navigate('/channels') })}
                  >
                    Confirm delete
                  </button>
                  <button onClick={() => setConfirmDelete(false)}>Keep</button>
                </span>
              ) : (
                <button onClick={() => setConfirmDelete(true)}>Delete channel</button>
              )
            ) : (
              <button disabled title="Remove all bridges attached to this channel first">
                Delete channel
              </button>
            )
          ) : null
        }
      />
      <div style={{ padding: '28px 32px', display: 'grid', gap: 24 }}>
        <div className="label" style={{ color: 'var(--ink-3)', marginTop: -12 }}>
          {channel ? resolved.description || 'No description.' : (
            <>
              Unknown channel — its source may have been deleted. Historical events below
              keep their id reference. <Tag variant="stale">id {resolved.id}</Tag>
            </>
          )}
        </div>
        <div
          className="kpi-row"
          style={{ gridTemplateColumns: `repeat(${isAgentInbox ? 3 : isCustom ? 2 : 4}, 1fr)` }}
        >
          {!isCustom && (
            <div className="kpi">
              <div className="hd">
                <span className="label">{isAgentInbox ? 'Reached inbox 24h' : 'Events 24h'}</span>
              </div>
              <div className="figure" style={{ fontSize: 22 }}>{counts.events24h}</div>
            </div>
          )}
          {!isAgentInbox && !isCustom && (
            <div className="kpi">
              <div className="hd">
                <span className="label">Unmatched 24h</span>
              </div>
              <div className="figure" style={{ fontSize: 22 }}>{counts.unmatched24h}</div>
            </div>
          )}
          {!isCustom && (
            <div className="kpi">
              <div className="hd">
                <span className="label">Failed 24h</span>
              </div>
              <div className={`figure ${counts.failed24h > 0 ? 'err' : ''}`} style={{ fontSize: 22 }}>
                {counts.failed24h}
              </div>
            </div>
          )}
          {isCustom && (
            <div className="kpi">
              <div className="hd">
                <span className="label">Bridges</span>
              </div>
              <div className="figure" style={{ fontSize: 22 }}>
                {(local?.bridges ?? []).filter((b) => b.from === resolved.id || b.to === resolved.id).length}
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
              <span style={{ fontSize: 12, color: 'var(--ink-4)', fontFamily: 'var(--mono, monospace)' }}>
                {resolved.id}
              </span>
            </div>
          </div>
        </div>

        <RelationTree channel={resolved} edges={edges} />
        {isCustom && <BridgeEditor channel={resolved} all={channels} />}

        {isAgentInbox ? (
          channel?.entityId ? (
            <>
              <AgentTriggers agentId={channel.entityId} agentName={name} />
              <AgentSchedules agentId={channel.entityId} />
            </>
          ) : (
            <Panel title="Subscriptions" className="cropped">
              <div className="label" style={{ color: 'var(--ink-3)' }}>
                This inbox&apos;s agent no longer exists in the registry — subscriptions
                were configured on the agent and cascade with it.
              </div>
            </Panel>
          )
        ) : resolved.kind === 'connector' ? (
          <SubscribersPanel channel={resolved} edges={edges} />
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
                {events.data?.events.map((e) => <EventLine key={e.id} entry={e} />)}
              </div>
            )}
          </Panel>
        )}

        {showRuns ? (
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
                    <span style={{ fontSize: 13, color: 'var(--ink-3)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {inv.triggerName ?? inv.trigger ?? '—'} · {inv.id}
                    </span>
                    <RunStateTag status={inv.status} />
                    <span style={{ fontSize: 12, color: 'var(--ink-3)' }}>
                      {inv.durationMs !== undefined ? `${(inv.durationMs / 1000).toFixed(1)}s` : '—'}
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
