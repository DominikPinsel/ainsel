import { ServiceUnavailableError } from '../../api/client'
import { useObservabilityTimeseries } from '../../api/observability'
import { Panel } from '../../primitives/Panel'

const CHART_WIDTH = 400
const CHART_HEIGHT = 120
const PADDING = { top: 14, bottom: 20, left: 6, right: 6 }

export function ThroughputChart() {
  const { data, isLoading, error } = useObservabilityTimeseries({
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

  return (
    <Panel
      title="Throughput · 24h"
      right={<span className="label">events / hour</span>}
      className="cropped"
    >
      {isLoading ? <div className="label" style={{ padding: 14 }}>Loading…</div> : null}
      {error instanceof ServiceUnavailableError ? (
        <div className="label" style={{ padding: 14 }}>
          Telemetry not configured.
        </div>
      ) : error ? (
        <div className="label" style={{ padding: 14, color: 'var(--signal)' }}>
          Failed to load throughput.
        </div>
      ) : null}
      {!isLoading && !error ? (
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
      ) : null}
    </Panel>
  )
}
