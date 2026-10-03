export type SectionState = 'idle' | 'loading' | 'unavailable' | 'error' | 'ready'

type SectionStatusProps = {
  state: SectionState
  title?: string
  detail?: string
  onRetry?: () => void
}

// SectionStatus is the body of a panel that has nothing to show yet. The
// `unavailable` default says only that no source can answer — a panel's own
// heading comes from `title`, and the hub's reason for the 503 comes from
// `detail`, because "telemetry is not configured" was wrong for every panel
// that was missing something other than Prometheus.
const DEFAULT_TITLES: Record<Exclude<SectionState, 'ready' | 'idle'>, string> = {
  loading: 'Loading…',
  unavailable: 'No data source configured',
  error: 'Failed to load',
}

export function SectionStatus({ state, title, detail, onRetry }: SectionStatusProps) {
  if (state === 'ready' || state === 'idle') return null
  const cls = state === 'error' ? 'section-status error' : 'section-status'
  const resolvedTitle = title ?? DEFAULT_TITLES[state]
  return (
    <div className={cls} role="status" aria-live="polite">
      <div className="ss-title">{resolvedTitle}</div>
      {detail ? <div className="ss-detail">{detail}</div> : null}
      {state === 'error' && onRetry ? (
        <button className="btn btn-sm" onClick={onRetry} type="button">
          Retry
        </button>
      ) : null}
    </div>
  )
}
