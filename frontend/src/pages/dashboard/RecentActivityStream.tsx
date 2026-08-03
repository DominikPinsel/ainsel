import { useRecentEvents } from '../../api/events'
import type { ActivityEntry } from '../../api/events'
import { Panel } from '../../primitives/Panel'
import { Tag } from '../../primitives/Tag'
import { formatRelative } from '../../utils/time'

function StatusTag({ status }: { status: ActivityEntry['status'] }) {
  if (status === 'matched') return <Tag variant="ok">MATCH</Tag>
  if (status === 'error') return <Tag variant="err">ERR</Tag>
  return <Tag variant="stale">SKIP</Tag>
}

export function RecentActivityStream() {
  const { data, isLoading, error } = useRecentEvents(5)

  const errorCount = (data ?? []).filter((e) => e.status === 'error').length

  return (
    <Panel
      title="Recent Activity · live"
      right={errorCount > 0 ? <Tag variant="err">{errorCount} error</Tag> : null}
      className="cropped"
    >
      {isLoading ? <div className="label" style={{ padding: 4 }}>Loading…</div> : null}
      {error ? (
        <div className="label" style={{ padding: 4, color: 'var(--signal)' }}>
          Failed to load activity.
        </div>
      ) : null}
      {!isLoading && !error && data ? (
        data.length === 0 ? (
          <div className="label" style={{ padding: 4 }}>
            No recent events.
          </div>
        ) : (
          data.map((e) => (
            <div key={e.id} className="stream-row">
              <span className="ts num">{formatRelative(e.timestamp)}</span>
              <span className="desc">
                {e.connector ? <b>{e.connector}</b> : null}
                {e.matches && e.matches.length > 0 ? (
                  <>
                    {' '}
                    → <b>{e.matches[0].agent}</b>
                  </>
                ) : null}
              </span>
              <StatusTag status={e.status} />
            </div>
          ))
        )
      ) : null}
    </Panel>
  )
}
