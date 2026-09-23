import { Link } from 'react-router-dom'
import {
  buildConnections,
  channelPath,
  useChannelSubscriptions,
  useChannels,
} from '../../api/channels'
import { Button } from '../../primitives/Button'
import { Panel } from '../../primitives/Panel'
import { SectionStatus } from '../../primitives/SectionStatus'
import { Tag } from '../../primitives/Tag'

type Props = { agentId: string; agentName: string }

/**
 * The agent's inbox channel, as shown on the Agent detail page: how the
 * outside world reaches this agent — which channels feed the inbox through
 * which subscriptions — plus the counts for what actually arrived.
 * Configuration (subscriptions, schedules) lives in the channel view, which
 * this links to.
 */
export function AgentChannelTab({ agentId, agentName }: Props) {
  const { data, isLoading } = useChannels({ pageSize: 500 })
  const { data: subs, isLoading: subsLoading, error } = useChannelSubscriptions()
  const channels = data?.items ?? []
  // The inbox is provisioned from this agent, so its entity ref is the stable
  // handle — the name is rename-able and may collide with a connector label.
  const channel = channels.find((c) => c.kind === 'agent' && c.entityRef === agentId)
  const channelId = channel?.id

  const feeds = channelId
    ? buildConnections(subs?.items, channels).filter((c) => c.to?.id === channelId)
    : []

  if (isLoading && !channel) {
    return (
      <Panel title="Channel" className="cropped">
        <p className="label">Loading channel…</p>
      </Panel>
    )
  }

  if (!channel) {
    return (
      <Panel title="Channel" className="cropped">
        <div className="label" style={{ color: 'var(--ink-3)' }}>
          This agent has no channel yet — the hub provisions one per agent on its next registry
          sync.
        </div>
      </Panel>
    )
  }

  const counts = channel.counts ?? { events: 0, unmatched: 0, failed: 0 }

  return (
    <div style={{ display: 'grid', gap: 20 }}>
      <Panel
        title="Channel"
        right={
          <Link to={channelPath(channel.id)}>
            <Button size="sm" variant="primary">
              Open channel
            </Button>
          </Link>
        }
        className="cropped"
      >
        <div style={{ display: 'grid', gap: 12 }}>
          <div style={{ display: 'flex', gap: 10, alignItems: 'baseline', flexWrap: 'wrap' }}>
            <b style={{ fontSize: 16 }}>{channel.name || agentName}</b>
            <Tag variant="ok">agent inbox</Tag>
            <span
              className="label"
              style={{ color: 'var(--ink-4)', fontFamily: 'var(--mono, monospace)' }}
            >
              {channel.id}
            </span>
          </div>
          <div style={{ color: 'var(--ink-2)', fontSize: 13 }}>
            {channel.description || `The inbox agent ${agentName} drains`}
          </div>
          <div style={{ display: 'flex', gap: 24, fontSize: 13, color: 'var(--ink-2)' }}>
            <span>
              <b>{counts.events}</b> events reached this inbox in 24h
            </span>
            <span>
              <b style={counts.failed > 0 ? { color: 'var(--signal)' } : undefined}>
                {counts.failed}
              </b>{' '}
              failed in 24h
            </span>
          </div>
          <div className="label" style={{ color: 'var(--ink-3)' }}>
            The agent takes <b>everything</b> from this channel — matching happens once, when a
            subscription transfers an event in. Subscriptions and schedules are owned by the
            channel; configure them in the channel view.
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
                No channel feeds this inbox yet. Nothing will arrive until a subscription transfers
                events in — or an event is born directly here.
              </div>
            ) : (
              feeds.map((f, i) => (
                <div
                  key={`${f.source}#${f.subscription}#${f.fromRef ?? i}`}
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
                      {f.from.name}
                    </Link>
                  ) : (
                    <span style={{ color: 'var(--ink-4)' }}>{f.fromRef ?? 'unknown channel'}</span>
                  )}
                  <span style={{ color: 'var(--ink-3)' }}>
                    <span style={{ color: 'var(--ink-4)' }}>
                      {f.source === 'bridge' ? 'bridged · ' : ''}
                    </span>
                    <span style={{ color: 'var(--ink-4)' }}>— subscription </span>
                    <b style={{ color: 'var(--ink-2)' }}>{f.subscription}</b>
                    <span style={{ color: 'var(--ink-4)' }}> transfers into →</span>
                  </span>
                  <Tag>{channel.name}</Tag>
                </div>
              ))
            )}
            <div className="label" style={{ color: 'var(--ink-3)', padding: '8px 0 2px' }}>
              Direct sources: <b>scheduled runs</b> and <b>chat messages</b> are born straight in
              this channel — they need no subscription. Manage them under{' '}
              <Link to={channelPath(channel.id)}>Schedules</Link> in the channel view.
            </div>
          </div>
        )}
      </Panel>
    </div>
  )
}
