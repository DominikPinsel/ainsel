import { Link } from 'react-router-dom'
import {
  buildConnections,
  channelPath,
  useChannelCounts,
  useChannels,
  type ChannelSummary,
} from '../../api/channels'
import { useTriggers } from '../../api/triggers'
import { Titleblock } from '../../layout/Titleblock'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { SectionStatus } from '../../primitives/SectionStatus'
import { Tag } from '../../primitives/Tag'
import './ChannelsPage.css'

function OriginTag({ origin }: { origin: ChannelSummary['origin'] }) {
  return (
    <Tag variant={origin === 'agent' ? 'ok' : 'default'}>
      {origin === 'agent' ? 'agent inbox' : 'connector'}
    </Tag>
  )
}

function ChannelCountsCell({ channel }: { channel: ChannelSummary }) {
  const counts = useChannelCounts(channel)
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

const columns: readonly Column<ChannelSummary>[] = [
  {
    key: 'name',
    header: 'Channel',
    cell: (c) => (
      <span style={{ display: 'flex', gap: 8, alignItems: 'baseline', flexWrap: 'wrap' }}>
        <Link
          to={channelPath(c.id)}
          style={{ fontWeight: 600, color: 'var(--ink)' }}
        >
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
  {
    key: 'origin',
    header: 'Provisioned for',
    cell: (c) => <OriginTag origin={c.origin} />,
  },
  {
    key: 'description',
    header: 'Description',
    cell: (c) => (
      <span style={{ color: 'var(--ink-2)', fontSize: 13 }}>{c.description}</span>
    ),
  },
  {
    key: 'counts',
    header: 'Last 24h',
    align: 'right',
    cell: (c) => <ChannelCountsCell channel={c} />,
  },
]

/** Every subscription in the system as a real channel → channel edge. */
function ConnectionsPanel() {
  const { channels } = useChannels()
  const { data, isLoading, error } = useTriggers({ pageSize: 200 })
  const connections = buildConnections(channels, data?.items)

  return (
    <Panel title={`Connections · ${connections.length}`} className="cropped">
      {isLoading ? (
        <SectionStatus state="loading" />
      ) : error ? (
        <SectionStatus state="error" />
      ) : (
        <RegisterTable
          rows={connections}
          rowKey={(r) => `${r.subscription}#${r.fromRef ?? ''}#${r.toRef ?? ''}`}
          withRowNumbers={false}
          emptyLabel="No subscriptions yet — events stay in the channel they were born in until an inbox subscribes to it."
          columns={[
            {
              key: 'from',
              header: 'Born in',
              cell: (r) =>
                r.from ? (
                  <Link to={channelPath(r.from.id)}>{r.from.displayName}</Link>
                ) : (
                  <span style={{ color: 'var(--ink-4)' }}>
                    {r.fromRef ? `connector ${r.fromRef}` : '—'}
                  </span>
                ),
            },
            {
              key: 'subscription',
              header: 'Subscription',
              cell: (r) => <b>{r.subscription}</b>,
            },
            {
              key: 'to',
              header: 'Transferred into',
              cell: (r) =>
                r.to ? (
                  <Link to={channelPath(r.to.id)}>{r.to.displayName}</Link>
                ) : (
                  <span style={{ color: 'var(--ink-4)' }}>
                    {r.toRef ? `agent ${r.toRef}` : '—'}
                  </span>
                ),
            },
          ]}
        />
      )}
    </Panel>
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
      <div className="channels-page" style={{ padding: '28px 32px', display: 'grid', gap: 24 }}>
        <div className="label" style={{ color: 'var(--ink-3)', marginTop: -8 }}>
          Channels are plain streams — events are <b>born</b> in a channel, a{' '}
          <b>subscription</b> on a destination channel <b>transfers</b> matching events
          into it, and an agent simply takes <b>everything</b> from its own channel.
          A channel has no producer or consumer nature; it just holds what arrived.
          Identity is the id — two channels may share a label (a connector and an
          agent both named <b>forgejo</b> are two channels).
        </div>

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

        <ConnectionsPanel />
      </div>
    </>
  )
}
