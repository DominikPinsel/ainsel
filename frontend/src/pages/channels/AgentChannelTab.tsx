import { Link } from 'react-router-dom'
import { channelPath, useChannelCounts, useChannels } from '../../api/channels'
import { Button } from '../../primitives/Button'
import { Panel } from '../../primitives/Panel'
import { Tag } from '../../primitives/Tag'

type Props = { agentId: string; agentName: string }

/**
 * The agent's inbox channel, as shown on the Agent detail page. The channel
 * is addressed by id and owns the agent's subscriptions and schedules —
 * configuration happens in the channel view, which this links to.
 */
export function AgentChannelTab({ agentId: _agentId, agentName }: Props) {
  const { channels, isLoading } = useChannels()
  const channel = channels.find((c) => c.id === `agent:${agentName}`)
  const counts = useChannelCounts(
    channel ?? {
      id: `agent:${agentName}`,
      name: agentName,
      displayName: agentName,
      description: '',
      origin: 'agent',
      roles: ['consumes'],
    },
  )

  if (isLoading && !channel) {
    return (
      <Panel title="Channel" className="cropped">
        <p className="label">Loading channel…</p>
      </Panel>
    )
  }

  return (
    <Panel
      title="Channel"
      right={
        <Link to={channelPath(`agent:${agentName}`)}>
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
            agent:{agentName}
          </span>
        </div>
        <div style={{ color: 'var(--ink-2)', fontSize: 13 }}>
          {channel?.description ?? `Inbox of agent ${agentName}`}
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
          Subscriptions (what flows in) and schedules (when the agent is triggered) are
          owned by the channel — configure them in the channel view.
        </div>
      </div>
    </Panel>
  )
}
