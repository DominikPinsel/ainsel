import { Link, useParams } from 'react-router-dom'
import {
  buildConnections,
  channelPath,
  useChannelCounts,
  useChannels,
  type ChannelSummary,
} from '../../api/channels'
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

/** Which agent inboxes subscribe to this channel, and what they pull. */
function SubscribersPanel({ channel }: { channel: ChannelSummary }) {
  const { channels } = useChannels()
  const { data, isLoading, error } = useTriggers({ pageSize: 200 })
  // Hub issue #63: the triggers list ignores filter params — match
  // client-side. A connection belongs to this channel when its source
  // resolves to the channel (or names its connector entity directly).
  const subs = buildConnections(channels, data?.items).filter(
    (c) =>
      c.from?.id === channel.id ||
      (channel.entityId !== undefined && c.fromRef === channel.entityId),
  )

  return (
    <Panel title={`Subscribed inboxes · ${subs.length}`} className="cropped">
      {isLoading ? (
        <SectionStatus state="loading" />
      ) : error ? (
        <SectionStatus state="error" />
      ) : subs.length === 0 ? (
        <div className="label" style={{ padding: '4px 0' }}>
          No agent inbox subscribes to this channel yet — events born here
          stay until one does.
        </div>
      ) : (
        <div>
          {subs.map((sub) => {
            const target = sub.to
            return (
              <div
                key={`${sub.subscription}#${sub.toRef ?? ''}`}
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
                </span>
                <span style={{ color: 'var(--ink-4)' }}>→</span>
                <span style={{ textAlign: 'right' }}>
                  {target ? (
                    <Link to={channelPath(target.id)}>{target.displayName}</Link>
                  ) : (
                    <span style={{ color: 'var(--ink-4)' }}>
                      {sub.toRef ? `agent ${sub.toRef}` : 'unknown inbox'}
                    </span>
                  )}
                </span>
              </div>
            )
          })}
        </div>
      )}
    </Panel>
  )
}

export function ChannelDetailPage() {
  const { origin = 'connector', name = '' } = useParams<{ origin: string; name: string }>()
  const { channels } = useChannels()
  const key = `${origin}:${name}`
  const channel = channels.find((c) => c.id === key)
  const resolved: ChannelSummary = channel ?? {
    id: key,
    name,
    displayName: name,
    description: '',
    origin: origin === 'agent' ? 'agent' : 'connector',
  }
  const counts = useChannelCounts(resolved)

  const isAgentInbox = resolved.origin === 'agent'
  const scope = isAgentInbox ? { agent: name } : { connector: name }

  const events = useEventsPage({ ...scope, limit: 50 })
  const showRuns = isAgentInbox
  const invocations = useInvocations(
    { agent: name, pageSize: 20 },
    { enabled: showRuns },
  )

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
          style={{ gridTemplateColumns: `repeat(${isAgentInbox ? 3 : 4}, 1fr)` }}
        >
          <div className="kpi">
            <div className="hd">
              <span className="label">{isAgentInbox ? 'Reached inbox 24h' : 'Events 24h'}</span>
            </div>
            <div className="figure" style={{ fontSize: 22 }}>{counts.events24h}</div>
          </div>
          {!isAgentInbox ? (
            <div className="kpi">
              <div className="hd">
                <span className="label">Unmatched 24h</span>
              </div>
              <div className="figure" style={{ fontSize: 22 }}>{counts.unmatched24h}</div>
            </div>
          ) : null}
          <div className="kpi">
            <div className="hd">
              <span className="label">Failed 24h</span>
            </div>
            <div className={`figure ${counts.failed24h > 0 ? 'err' : ''}`} style={{ fontSize: 22 }}>
              {counts.failed24h}
            </div>
          </div>
          <div className="kpi">
            <div className="hd">
              <span className="label">Identity</span>
            </div>
            <div className="figure" style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
              <Tag variant={isAgentInbox ? 'ok' : 'default'}>{isAgentInbox ? 'agent inbox' : resolved.origin}</Tag>
              <span style={{ fontSize: 12, color: 'var(--ink-4)', fontFamily: 'var(--mono, monospace)' }}>
                {resolved.id}
              </span>
            </div>
          </div>
        </div>

        {isAgentInbox ? (
          <>
            {channel?.entityId ? (
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
            )}
          </>
        ) : resolved.origin === 'connector' ? (
          <SubscribersPanel channel={resolved} />
        ) : null}

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
