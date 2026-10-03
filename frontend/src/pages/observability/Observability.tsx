import { useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { ServiceUnavailableError } from '../../api/client'
import {
  formatStep,
  unavailableDetail,
  useObservabilitySummary,
  useObservabilityTimeseries,
  useTokensBySubject,
  useTokensSummary,
  type MetricsSource,
  type Range,
  type TimeseriesParams,
} from '../../api/observability'
import { Panel } from '../../primitives/Panel'
import { SectionStatus, type SectionState } from '../../primitives/SectionStatus'
import { TimeSeriesChart, type SeriesStyle } from '../../primitives/TimeSeriesChart'
import { Titleblock } from '../../layout/Titleblock'
import { LogStream } from './LogStream'
import { RangeSelector } from './RangeSelector'
import { SummaryCards } from './SummaryCards'
import { TokensTable } from './TokensTable'

const VALID_RANGES = ['1h', '6h', '24h', '7d'] as const
const isRange = (s: string | null): s is Range =>
  s !== null && (VALID_RANGES as readonly string[]).includes(s)

const CHART_STYLES: SeriesStyle[] = [
  { name: 'events_consumed', color: 'var(--ink)', label: 'Consumed' },
  { name: 'triggers_matched', color: 'var(--ink-3)', label: 'Matched', dashed: true },
  { name: 'events_routed', color: 'var(--signal)', label: 'Routed' },
  { name: 'routing_errors', color: 'var(--warn)', label: 'Errors', dashed: true },
]

const METRICS: TimeseriesParams['metric'][] = [
  'events_consumed',
  'triggers_matched',
  'events_routed',
  'routing_errors',
]

function deriveState(
  isLoading: boolean,
  error: unknown,
  hasData: boolean,
): SectionState {
  if (isLoading) return 'loading'
  if (error instanceof ServiceUnavailableError) return 'unavailable'
  if (error) return 'error'
  if (hasData) return 'ready'
  return 'idle'
}

// chartUnit names what one bar of the throughput chart holds. The two metric
// backends answer in different units, so the label follows whichever responded:
// Prometheus gives a per-second rate, the hub's records give a count per bucket.
function chartUnit(source?: MetricsSource, step?: string): string {
  if (source === 'postgres') return `events / ${formatStep(step) ?? 'bucket'}`
  return 'events / period'
}

export function Observability() {
  const [params, setParams] = useSearchParams()
  const range: Range = isRange(params.get('range')) ? (params.get('range') as Range) : '24h'

  const setRange = (next: Range) => {
    const updated = new URLSearchParams(params)
    updated.set('range', next)
    setParams(updated, { replace: true })
  }

  const summary = useObservabilitySummary(range)
  const tokensSummary = useTokensSummary(range)
  const tokensSubject = useTokensBySubject(range)

  // One timeseries query per metric so each can fail independently.
  // (TanStack Query already deduplicates; we'd usually keep them together,
  //  but the spec says each section can be 503'd independently, and the
  //  chart is one section while the summary cards are another.)
  const consumed = useObservabilityTimeseries({ range, metric: 'events_consumed' })
  const matched = useObservabilityTimeseries({ range, metric: 'triggers_matched' })
  const routed = useObservabilityTimeseries({ range, metric: 'events_routed' })
  const errors = useObservabilityTimeseries({ range, metric: 'routing_errors' })

  // The KPI cards read the hub's event counters, which the hub answers from its
  // own records when there is no Prometheus. Token usage is the exception: it is
  // published by the agent runtime to Prometheus only, so a missing Prometheus
  // must not blank the event cards — it costs the one token tile.
  const summaryState = deriveState(
    summary.isLoading,
    summary.error,
    Boolean(summary.data),
  )
  const tokensUnavailable = tokensSummary.error instanceof ServiceUnavailableError

  const chartLoadingOrError =
    consumed.isLoading || matched.isLoading || routed.isLoading || errors.isLoading
      ? 'loading'
      : [consumed.error, matched.error, routed.error, errors.error].some(
            (e) => e instanceof ServiceUnavailableError,
          )
        ? 'unavailable'
        : [consumed.error, matched.error, routed.error, errors.error].some(Boolean)
          ? 'error'
          : 'ready'
  const chartState: SectionState = chartLoadingOrError

  const consumedData = consumed.data
  const matchedData = matched.data
  const routedData = routed.data
  const errorsData = errors.data
  const chartSeries = useMemo(
    () =>
      METRICS.map((name) => {
        const data =
          name === 'events_consumed'
            ? consumedData
            : name === 'triggers_matched'
              ? matchedData
              : name === 'events_routed'
                ? routedData
                : errorsData
        return { name, points: data?.points ?? [] }
      }),
    [consumedData, matchedData, routedData, errorsData],
  )

  const tokensState = deriveState(
    tokensSubject.isLoading,
    tokensSubject.error,
    Boolean(tokensSubject.data),
  )

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Operations / <b>Observability</b>
          </>
        }
        title={
          <>
            Telemetry <em>Console</em>
          </>
        }
        actions={<RangeSelector value={range} onChange={setRange} />}
      />
      <div style={{ padding: '28px 32px', display: 'grid', gap: 24 }}>
        <section>
          {summaryState === 'ready' ? (
            <>
              <SummaryCards summary={summary.data} tokens={tokensSummary.data} range={range} />
              {tokensUnavailable ? (
                <p className="label" style={{ marginTop: 8 }}>
                  Token usage needs Prometheus: the agent runtime publishes it as a
                  metric, so that tile stays empty without one.
                </p>
              ) : null}
            </>
          ) : (
            <Panel className="cropped">
              <SectionStatus
                state={summaryState}
                title={
                  summaryState === 'unavailable' ? 'No metrics source configured' : undefined
                }
                detail={unavailableDetail(summary.error)}
                onRetry={() => {
                  summary.refetch()
                  tokensSummary.refetch()
                }}
              />
            </Panel>
          )}
        </section>

        <Panel
          title={`Throughput · ${range}`}
          right={
            <span className="label">{chartUnit(consumed.data?.source, consumed.data?.step)}</span>
          }
          className="cropped"
        >
          {chartState === 'ready' ? (
            <TimeSeriesChart
              series={chartSeries}
              styles={CHART_STYLES}
              ariaLabel={`Telemetry chart for ${range}`}
            />
          ) : (
            <SectionStatus
              state={chartState}
              title={chartState === 'unavailable' ? 'No metrics source configured' : undefined}
              detail={unavailableDetail(
                [consumed.error, matched.error, routed.error, errors.error].find(
                  (e) => e instanceof ServiceUnavailableError,
                ),
              )}
              onRetry={() => {
                consumed.refetch()
                matched.refetch()
                routed.refetch()
                errors.refetch()
              }}
            />
          )}
        </Panel>

        <Panel
          title={`Token Consumption · ${range}`}
          right={<span className="label">by agent · subject</span>}
          className="cropped"
        >
          {tokensState === 'ready' && tokensSubject.data ? (
            <TokensTable rows={tokensSubject.data.rows} />
          ) : (
            <SectionStatus
              state={tokensState}
              title={tokensState === 'unavailable' ? 'Token metrics need Prometheus' : undefined}
              detail={unavailableDetail(tokensSubject.error)}
              onRetry={() => tokensSubject.refetch()}
            />
          )}
        </Panel>

        <LogStream range={range} />
      </div>
    </>
  )
}
