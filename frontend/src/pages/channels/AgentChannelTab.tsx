import { Link } from 'react-router-dom'
import {
  buildConnections,
  channelPath,
  useChannelCounts,
  useChannels,
} from '../../api/channels'
import { useTriggers } from '../../api/triggers'
import { Button } from '../../primitives/Button'
import { Panel } from '../../primitives/Panel'
import { SectionStatus } from '../../primitives/SectionStatus'
import { Tag } from '../../primitives/Tag'

type Props = { agentId: string; agentName: string }

/**
 * The agent's inbox channel, as shown on the Agent detail page. Shows how
 * the outside world reaches this agent: which connector channels feed it
 * through which subscriptions, plus the direct sources that are born
 * straight in the inbox. Configuration (subscriptions, schedules) lives in
 * the channel view, which this links to.
 */
export function AgentChannelTab({ agentId, agentName }: Props) {
  const { channels, isLoading } = useChannels()
  const { data, isLoading: subsLoading, error } = useTriggers({ pageSize: 200 })
  const channelId = `agent:${agentName}`
  const channel = channels.find((c) => c.id === channelId)
  const counts = useChannelCounts(
    channel ?? {
      id: channelId,
      name: agentName,
      displayName: agentName,
      description: '',
      origin: 'agent',
    },
  )

  // Connections into THIS agent's inbox: subscription edges whose source is a
  // connector channel. Match by resolved channel id, falling back to the
  // agent registry id in case the channel cannot be resolved by name.
  const feeds = buildConnections(channels, data?.items).filter(
    (c) => c.to?.id === channelId || c.toRef === agentId,
  )

  if (isLoading && !channel) {
    return (
      <Panel title="Channel" className="cropped">
        <p className="label">Loading channel…</p>
      </Panel>
    )
  }

  return (
    <div style={{ display: 'grid', gap: 20 }}>
      <Panel
        title="Channel"
        right={
          <Link to={channelPath(channelId)}>
            <Button size="sm" variant="primary">
              Open channel
            </Button>
          </Link>
        }
        className="cropped"
      >
        <div style={{ display: 'grid', gap: 12 }}>
          <div style={{ display: 'flex', gap: 10, alignItems: 'baseline', flexWrap: 'wrap' }}>
            <b style={{ fontSize: 16 }}>{agentName}</b>
            <Tag variant="ok">agent inbox</Tag>
            <span
              className="label"
              style={{ color: 'var(--ink-4)', fontFamily: 'var(--mono, monospace)' }}
            >
              {channelId}
            </span>
          </div>
          <div style={{ color: 'var(--ink-2)', fontSize: 13 }}>
            {channel?.description ?? `The inbox agent ${agentName} drains`}
          </div>
          <div style={{ display: 'flex', gap: 24, fontSize: 13, color: 'var(--ink-2)' }}>
            <span>
              <b>{counts.events24h}</b> events reached this inbox in 24h
            </span>
            <span>
              <b style={counts.failed24h > 0 ? { color: 'var(--signal)' } : undefined}>
                {counts.failed24h}
              </b>{' '}
              failed in 24h
            </span>
          </div>
          <div className="label" style={{ color: 'var(--ink-3)' }}>
            The agent takes <b>everything</b> from this channel — matching happens
            once, when a subscription transfers an event in. Subscriptions and
            schedules are owned by the channel; configure them in the channel view.
          </div>
        </div>
      </Panel>

      <Panel title={`What feeds this channel · ${feeds.length}`} className="cropped">
        {subsLoading ? (
          <SectionStatus state="loading" />
        ) : error ? (
          <SectionStatus state="error" />
        ) : (
          <div style={{ display: 'grid', gap: 2 }}>
            {feeds.length === 0 ? (
              <div className="label" style={{ padding: '4px 0' }}>
                No connector channel feeds this inbox yet. Nothing will arrive
                until a subscription transfers events in — or an event is born
                directly here.
              </div>
            ) : (
              feeds.map((f, i) => (
                <div
                  key={`${f.subscription}#${f.fromRef ?? i}`}
                  style={{
                    display: 'grid',
                    gridTemplateColumns: 'auto 1fr auto',
                    gap: 12,
                    alignItems: 'baseline',
                    borderTop: '1px solid var(--rule-ghost)',
                    padding: '10px 2px',
                    fontSize: 13,
                  }}
                >
                  {f.from ? (
                    <Link to={channelPath(f.from.id)} style={{ fontWeight: 600 }}>
                      {f.from.displayName}
                    </Link>
                  ) : (
                    <span style={{ color: 'var(--ink-4)' }}>
                      {f.fromRef ? `connector ${f.fromRef}` : 'unknown channel'}
                    </span>
                  )}
                  <span style={{ color: 'var(--ink-3)' }}>
                    <span style={{ color: 'var(--ink-4)' }}>— subscription </span>
                    <b style={{ color: 'var(--ink-2)' }}>{f.subscription}</b>
                    <span style={{ color: 'var(--ink-4)' }}> transfers into →</span>
                  </span>
                  <Tag>{channel?.displayName ?? agentName}</Tag>
                </div>
              ))
            )}
            <div className="label" style={{ color: 'var(--ink-3)', padding: '8px 0 2px' }}>
              Direct sources: <b>scheduled runs</b> and <b>chat messages</b> are born
              straight in this channel — they need no subscription. Manage them under{' '}
              <Link to={channelPath(channelId)}>Schedules</Link> in the channel view.
            </div>
          </div>
        )}
      </Panel>
    </div>
  )
}
