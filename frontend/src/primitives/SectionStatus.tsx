export type SectionState = 'idle' | 'loading' | 'unavailable' | 'error' | 'ready'

type SectionStatusProps = {
  state: SectionState
  title?: string
  detail?: string
  onRetry?: () => void
}

const DEFAULT_TITLES: Record<Exclude<SectionState, 'ready' | 'idle'>, string> = {
  loading: 'Loading…',
  unavailable: 'Telemetry not configured',
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
