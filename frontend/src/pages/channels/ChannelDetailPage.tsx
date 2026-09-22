import { Link, useParams } from 'react-router-dom'
import { useChannelCounts, useChannels } from '../../api/channels'
import { useEventsPage } from '../../api/events'
import type { ActivityEntry } from '../../api/events'
import {
  useInvocations,
} from '../../api/invocations'
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

export function ChannelDetailPage() {
  const { name = '' } = useParams<{ name: string }>()
  const { channels } = useChannels()
  const channel = channels.find((c) => c.name === name)
  const counts = useChannelCounts(
    channel ?? { name, displayName: name, origin: 'builtin', roles: ['produces'] },
  )

  const isProducer = channel?.roles.includes('produces') ?? true
  const scope = isProducer ? { connector: name } : { agent: name }

  const events = useEventsPage({ ...scope, limit: 50 })
  const showRuns = channel?.roles.includes('consumes') ?? false
  const invocations = useInvocations(
    { agent: name, pageSize: 20 },
    { enabled: showRuns },
  )

  const title = channel?.displayName ?? name

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
        <div className="kpi-row" style={{ gridTemplateColumns: 'repeat(4, 1fr)' }}>
          <div className="kpi">
            <div className="hd">
              <span className="label">Events 24h</span>
            </div>
            <div className="figure" style={{ fontSize: 22 }}>{counts.events24h}</div>
          </div>
          <div className="kpi">
            <div className="hd">
              <span className="label">Unmatched 24h</span>
            </div>
            <div className="figure" style={{ fontSize: 22 }}>{counts.unmatched24h}</div>
          </div>
          <div className="kpi">
            <div className="hd">
              <span className="label">Failed 24h</span>
            </div>
            <div className="figure" style={{ fontSize: 22 }}>{counts.failed24h}</div>
          </div>
          <div className="kpi">
            <div className="hd">
              <span className="label">Roles</span>
            </div>
            <div className="figure" style={{ display: 'flex', gap: 6 }}>
              {channel?.roles.includes('produces') ? <Tag solid>produces</Tag> : null}
              {channel?.roles.includes('consumes') ? <Tag variant="ok">consumes</Tag> : null}
              {!channel ? <Tag variant="stale">unknown</Tag> : null}
            </div>
          </div>
        </div>

        <Panel title="Timeline" className="cropped">
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

        <Panel title="What the agent did" className="cropped">
          {!showRuns ? (
            <div className="label" style={{ padding: '4px 0' }}>
              No consumer on this channel — events produced here wait for fan-out rules.
            </div>
          ) : invocations.isLoading ? (
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
      </div>
    </>
  )
}