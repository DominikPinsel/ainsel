import { useMemo } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useInvocations, invocationStatusVariant } from '../../../api/invocations'
import { useObservabilityTimeseries, type Range } from '../../../api/observability'
import { ServiceUnavailableError } from '../../../api/client'
import { Titleblock } from '../../../layout/Titleblock'
import { Panel } from '../../../primitives/Panel'
import { RegisterTable, type Column } from '../../../primitives/RegisterTable'
import { SectionStatus } from '../../../primitives/SectionStatus'
import { Tag } from '../../../primitives/Tag'
import { Dot } from '../../../primitives/Dot'
import { TimeSeriesChart, type SeriesStyle } from '../../../primitives/TimeSeriesChart'
import { RangeSelector } from '../RangeSelector'
import { formatRelative } from '../../../utils/time'
import { eventsLink } from '../../../utils/obsLinks'
import type { InvocationEntry } from '../../../api/invocations'

const VALID_RANGES = ['1h', '6h', '24h', '7d'] as const
const isRange = (s: string | null): s is Range =>
  s !== null && (VALID_RANGES as readonly string[]).includes(s)

const CHART_STYLES: SeriesStyle[] = [
  { name: 'triggers_matched', color: 'var(--ink-3)', label: 'Matched', dashed: true },
  { name: 'events_routed', color: 'var(--signal)', label: 'Routed' },
]

// Dot has no `default` state; map the neutral fallback to `stale`.
function statusDot(s: string): 'ok' | 'err' | 'warn' | 'stale' {
  const v = invocationStatusVariant(s)
  return v === 'default' ? 'stale' : v
}

export function RoutingDetail() {
  const [params, setParams] = useSearchParams()
  const range: Range = isRange(params.get('range')) ? (params.get('range') as Range) : '24h'

  const setRange = (next: Range) =>
    setParams((prev) => { const p = new URLSearchParams(prev); p.set('range', next); return p }, { replace: true })

  const matched = useObservabilityTimeseries({ range, metric: 'triggers_matched' })
  const routed = useObservabilityTimeseries({ range, metric: 'events_routed' })
  const invocations = useInvocations({ pageSize: 100 })

  const chartState =
    matched.isLoading || routed.isLoading ? 'loading'
    : [matched.error, routed.error].some((e) => e instanceof ServiceUnavailableError) ? 'unavailable'
    : [matched.error, routed.error].some(Boolean) ? 'error'
    : 'ready'

  const chartSeries = useMemo(
    () => [
      { name: 'triggers_matched', points: matched.data?.points ?? [] },
      { name: 'events_routed', points: routed.data?.points ?? [] },
    ],
    [matched.data, routed.data],
  )

  const columns: readonly Column<InvocationEntry>[] = [
    {
      key: 'when',
      header: 'When',
      width: 140,
      cell: (r) => <span className="num">{formatRelative(r.timestamp)}</span>,
    },
    {
      key: 'agent',
      header: 'Agent',
      width: 160,
      cell: (r) => r.agentName ?? r.agent,
    },
    {
      key: 'trigger',
      header: 'Trigger',
      cell: (r) => r.triggerName ?? r.trigger ?? <span className="label">—</span>,
    },
    {
      key: 'duration',
      header: 'Duration',
      width: 100,
      align: 'right',
      cell: (r) =>
        r.durationMs !== undefined ? (
          <span className="num">{(r.durationMs / 1000).toFixed(1)}s</span>
        ) : (
          <span className="label">—</span>
        ),
    },
    {
      key: 'status',
      header: 'Status',
      width: 110,
      cell: (r) => (
        <>
          <Dot state={statusDot(r.status)} />{' '}
          <Tag variant={invocationStatusVariant(r.status)}>
            {r.status}
          </Tag>
        </>
      ),
    },
    {
      key: 'events',
      header: '',
      width: 110,
      align: 'right',
      cell: (_r) => (
        <Link
          to={eventsLink({ status: 'matched', range })}
          className="label"
          style={{ color: 'var(--ink-3)', textDecoration: 'none' }}
          onClick={(e) => e.stopPropagation()}
        >
          view events →
        </Link>
      ),
    },
  ]

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Operations / <Link to="/observability">Observability</Link> / <b>Routing</b>
          </>
        }
        title={<>Routing <em>Detail</em></>}
        actions={<RangeSelector value={range} onChange={setRange} />}
      />
      <div style={{ padding: '28px 32px', display: 'grid', gap: 24 }}>
        <Panel title={`Matched · Routed · ${range}`} className="cropped">
          {chartState === 'ready' ? (
            <TimeSeriesChart
              series={chartSeries}
              styles={CHART_STYLES}
              ariaLabel={`Routing metrics over ${range}`}
            />
          ) : (
            <SectionStatus
              state={chartState}
              onRetry={() => { matched.refetch(); routed.refetch() }}
            />
          )}
        </Panel>

        <Panel
          title={`Recent Invocations · ${invocations.data?.items.length ?? 0}`}
          className="cropped"
        >
          {invocations.isLoading ? (
            <SectionStatus state="loading" />
          ) : invocations.error ? (
            <SectionStatus state="error" onRetry={() => invocations.refetch()} />
          ) : (
            <RegisterTable
              rows={invocations.data?.items ?? []}
              columns={columns}
              rowKey={(r) => r.id}
              emptyLabel="No recent invocations."
            />
          )}
        </Panel>
      </div>
    </>
  )
}
