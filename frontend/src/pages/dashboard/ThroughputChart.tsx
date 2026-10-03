import { ServiceUnavailableError } from '../../api/client'
import { chartUnit, useObservabilityTimeseries } from '../../api/observability'
import { Panel } from '../../primitives/Panel'
import { SectionStatus, type SectionState } from '../../primitives/SectionStatus'

const CHART_WIDTH = 400
const CHART_HEIGHT = 120
const PADDING = { top: 14, bottom: 20, left: 6, right: 6 }

export function ThroughputChart() {
  const { data, isLoading, error, refetch } = useObservabilityTimeseries({
    range: '24h',
    metric: 'events_routed',
  })

  const points = data?.points ?? []
  const max = points.reduce((m, p) => (p.value > m ? p.value : m), 0)
  const peak = max
  const innerW = CHART_WIDTH - PADDING.left - PADDING.right
  const innerH = CHART_HEIGHT - PADDING.top - PADDING.bottom
  const barCount = points.length || 24
  const barSlot = innerW / barCount
  const barWidth = Math.max(6, barSlot - 6)
  const peakY = PADDING.top + innerH * (1 - (peak > 0 ? 1 : 0))

  const state: SectionState = isLoading
    ? 'loading'
    : error
      ? error instanceof ServiceUnavailableError
        ? 'unavailable'
        : 'error'
      : 'ready'
  // The hub's own records count events per bucket; a Prometheus counter is a
  // rate. Whichever answered, the label has to say what the bars measure.
  const unit = chartUnit(data?.source, data?.step)

  return (
    <Panel
      title="Throughput · 24h"
      right={<span className="label">{unit}</span>}
      className="cropped"
    >
      {state !== 'ready' ? (
        <SectionStatus
          state={state}
          title={
            state === 'unavailable'
              ? 'No metrics source configured'
              : state === 'error'
                ? 'Failed to load throughput'
                : undefined
          }
          detail={error instanceof Error ? error.message : undefined}
          onRetry={() => refetch()}
        />
      ) : (
        <>
          <div style={{ padding: 14, borderBottom: '1px solid var(--rule-soft)' }}>
            <svg
              viewBox={`0 0 ${CHART_WIDTH} ${CHART_HEIGHT}`}
              preserveAspectRatio="none"
              style={{ display: 'block', width: '100%', height: 140 }}
              role="img"
              aria-label="Throughput over 24 hours"
            >
              <line
                x1={0}
                y1={CHART_HEIGHT - PADDING.bottom}
                x2={CHART_WIDTH}
                y2={CHART_HEIGHT - PADDING.bottom}
                stroke="var(--ink)"
                strokeWidth={1}
              />
              {peak > 0 ? (
                <>
                  <line
                    x1={0}
                    y1={peakY}
                    x2={CHART_WIDTH}
                    y2={peakY}
                    stroke="var(--signal)"
                    strokeWidth={1}
                    strokeDasharray="3 3"
                  />
                  <text
                    x={CHART_WIDTH - 8}
                    y={peakY - 4}
                    textAnchor="end"
                    fontFamily="var(--mono)"
                    fontSize={9}
                    fill="var(--signal)"
                  >
                    PEAK · {peak}
                  </text>
                </>
              ) : null}
              {points.map((p, i) => {
                const h = max > 0 ? (p.value / max) * innerH : 0
                const x = PADDING.left + i * barSlot + (barSlot - barWidth) / 2
                const y = PADDING.top + innerH - h
                const isLast = i === points.length - 1
                return (
                  <rect
                    key={p.timestamp}
                    x={x}
                    y={y}
                    width={barWidth}
                    height={h}
                    fill={isLast ? 'var(--signal)' : 'var(--ink)'}
                  />
                )
              })}
            </svg>
          </div>
          <div
            className="label"
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              padding: '8px 14px',
            }}
          >
            <span>{points[0]?.timestamp.slice(11, 16) ?? '—'}</span>
            <span>{points[Math.floor(points.length / 2)]?.timestamp.slice(11, 16) ?? '—'}</span>
            <span>NOW</span>
          </div>
        </>
      )}
    </Panel>
  )
}
