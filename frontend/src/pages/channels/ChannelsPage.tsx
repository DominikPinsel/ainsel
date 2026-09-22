import { Link } from 'react-router-dom'
import { useChannels, useChannelCounts, type ChannelSummary } from '../../api/channels'
import { ChannelFlowDiagram } from '../../components/journey/EventJourney'
import { Titleblock } from '../../layout/Titleblock'
import { Panel } from '../../primitives/Panel'
import { SectionStatus } from '../../primitives/SectionStatus'
import { Tag } from '../../primitives/Tag'
import './ChannelsPage.css'

function RoleTags({ roles }: { roles: ChannelSummary['roles'] }) {
  return (
    <div className="ch-roles">
      {roles.includes('produces') ? <Tag solid>produces</Tag> : null}
      {roles.includes('consumes') ? <Tag variant="ok">consumes</Tag> : null}
    </div>
  )
}

function ChannelCard({ channel }: { channel: ChannelSummary }) {
  const counts = useChannelCounts(channel)
  return (
    <Link to={`/channels/${encodeURIComponent(channel.name)}`} className="channel-card">
      <div className="ch-name">
        <span
          className={`ch-mark ${channel.origin === 'builtin' ? 'builtin' : channel.origin === 'agent' ? 'agent' : ''}`}
          aria-hidden="true"
        />
        {channel.displayName}
      </div>
      <RoleTags roles={channel.roles} />
      <div className="ch-stats">
        <div>
          <div className="ch-figure">{counts.events24h}</div>
          <div className="ch-figure-label">events 24h</div>
        </div>
        <div>
          <div className={`ch-figure ${counts.unmatched24h > 0 ? 'err' : ''}`}>
            {counts.unmatched24h}
          </div>
          <div className="ch-figure-label">unmatched</div>
        </div>
        <div>
          <div className={`ch-figure ${counts.failed24h > 0 ? 'err' : ''}`}>
            {counts.failed24h}
          </div>
          <div className="ch-figure-label">failed</div>
        </div>
      </div>
    </Link>
  )
}

export function ChannelsPage() {
  const { channels, isLoading, error } = useChannels()

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Operations / <b>Channels</b>
          </>
        }
        title={<>Channels</>}
      />
      <div className="channels-page" style={{ padding: '28px 32px' }}>
        <Panel title="Flow" className="cropped">
          <ChannelFlowDiagram />
          <div className="label" style={{ marginTop: 8, color: 'var(--ink-3)' }}>
            Every event is produced into a channel and fanned out into further channels by
            rules. Open a channel to trace what flowed through it, or open an event to see its
            full journey.
          </div>
        </Panel>

        {isLoading ? (
          <Panel className="cropped">
            <SectionStatus state="loading" />
          </Panel>
        ) : error ? (
          <Panel className="cropped">
            <SectionStatus state="error" />
          </Panel>
        ) : (
          <div className="channel-grid">
            {channels.map((c) => (
              <ChannelCard key={c.name} channel={c} />
            ))}
          </div>
        )}
      </div>
    </>
  )
}