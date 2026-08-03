import { useMemo } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useErrors } from '../../../api/errors'
import type { ErrorSeverity, ErrorSource, PlatformError } from '../../../api/errors'
import { useObservabilityTimeseries, type Range } from '../../../api/observability'
import { ServiceUnavailableError } from '../../../api/client'
import { Titleblock } from '../../../layout/Titleblock'
import { Panel } from '../../../primitives/Panel'
import { Button } from '../../../primitives/Button'
import { Dot } from '../../../primitives/Dot'
import { RegisterTable, type Column } from '../../../primitives/RegisterTable'
import { Select } from '../../../primitives/Select'
import { SectionStatus } from '../../../primitives/SectionStatus'
import { Tag } from '../../../primitives/Tag'
import { TimeSeriesChart, type SeriesStyle } from '../../../primitives/TimeSeriesChart'
import { RangeSelector } from '../RangeSelector'
import { formatRelative } from '../../../utils/time'
import { eventsLink } from '../../../utils/obsLinks'
import { useUrlFilters } from '../../../hooks/useUrlFilters'

const VALID_RANGES = ['1h', '6h', '24h', '7d'] as const
const isRange = (s: string | null): s is Range =>
  s !== null && (VALID_RANGES as readonly string[]).includes(s)

const SEVERITY_OPTIONS = [
  { value: 'error', label: 'Error' },
  { value: 'warning', label: 'Warning' },
] as const

const SOURCE_OPTIONS = [
  { value: 'router', label: 'Router' },
  { value: 'connector', label: 'Connector' },
  { value: 'api', label: 'API' },
  { value: 'agent', label: 'Agent' },
  { value: 'hub', label: 'Hub' },
  { value: 'gateway', label: 'Gateway' },
] as const

type ErrorFilters = { severity?: string; source?: string }

const CHART_STYLES: SeriesStyle[] = [
  { name: 'routing_errors', color: 'var(--warn)', label: 'Errors', dashed: true },
]

export function ErrorsDetail() {
  const [params, setParams] = useSearchParams()
  const range: Range = isRange(params.get('range')) ? (params.get('range') as Range) : '24h'

  const setRange = (next: Range) =>
    setParams((prev) => { const p = new URLSearchParams(prev); p.set('range', next); return p }, { replace: true })

  const { filters, setFilters, reset } = useUrlFilters<ErrorFilters>(['severity', 'source'])
  const { data, isLoading, error, refetch } = useErrors({ limit: 200 })
  const timeseries = useObservabilityTimeseries({ range, metric: 'routing_errors' })

  const filtered: PlatformError[] = useMemo(() => {
    const rows = data ?? []
    return rows.filter((row) => {
      if (filters.severity && row.severity !== filters.severity) return false
      if (filters.source && row.source !== filters.source) return false
      return true
    })
  }, [data, filters])

  const hasFilter = !!(filters.severity || filters.source)

  const chartState =
    timeseries.isLoading ? 'loading'
    : timeseries.error instanceof ServiceUnavailableError ? 'unavailable'
    : timeseries.error ? 'error'
    : 'ready'

  const chartSeries = useMemo(
    () => [{ name: 'routing_errors', points: timeseries.data?.points ?? [] }],
    [timeseries.data],
  )

  const columns: readonly Column<PlatformError>[] = [
    {
      key: 'timestamp',
      header: 'When',
      width: 140,
      cell: (r) => <span className="num">{formatRelative(r.timestamp)}</span>,
    },
    {
      key: 'severity',
      header: 'Severity',
      width: 120,
      cell: (r) => (
        <Tag variant={r.severity === 'error' ? 'err' : 'warn'}>{r.severity.toUpperCase()}</Tag>
      ),
    },
    {
      key: 'source',
      header: 'Source',
      width: 130,
      cell: (r) => (
        <>
          <Dot state={r.severity === 'error' ? 'err' : 'warn'} /> {r.source}
        </>
      ),
    },
    {
      key: 'message',
      header: 'Message',
      cell: (r) => r.message,
    },
    {
      key: 'events',
      header: '',
      width: 110,
      align: 'right',
      cell: (_r) => (
        <Link
          to={eventsLink({ status: 'error' })}
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
            Operations / <Link to="/observability">Observability</Link> / <b>Errors</b>
          </>
        }
        title={<>Error <em>Detail</em></>}
        actions={<RangeSelector value={range} onChange={setRange} />}
      />
      <div style={{ padding: '28px 32px', display: 'grid', gap: 24 }}>
        <Panel title={`Routing Errors · ${range}`} className="cropped">
          {chartState === 'ready' ? (
            <TimeSeriesChart
              series={chartSeries}
              styles={CHART_STYLES}
              ariaLabel={`Routing errors over ${range}`}
            />
          ) : (
            <SectionStatus state={chartState} onRetry={() => timeseries.refetch()} />
          )}
        </Panel>

        <div>
          <div className="toolbar" style={{ marginBottom: 12 }}>
            <div className="field">
              <span className="label">Severity</span>
              <Select
                aria-label="Filter by severity"
                value={(filters.severity as ErrorSeverity | undefined) ?? ''}
                onChange={(v) => setFilters({ severity: v || undefined })}
                options={SEVERITY_OPTIONS}
                emptyLabel="Any severity"
              />
            </div>
            <div className="field">
              <span className="label">Source</span>
              <Select
                aria-label="Filter by source"
                value={(filters.source as ErrorSource | undefined) ?? ''}
                onChange={(v) => setFilters({ source: v || undefined })}
                options={SOURCE_OPTIONS}
                emptyLabel="Any source"
              />
            </div>
            {hasFilter && (
              <Button size="sm" onClick={reset}>↺ Clear</Button>
            )}
          </div>

          <Panel className="cropped">
            {isLoading ? (
              <SectionStatus state="loading" />
            ) : error ? (
              <SectionStatus state="error" onRetry={() => refetch()} />
            ) : (
              <RegisterTable
                rows={filtered}
                columns={columns}
                rowKey={(r) => r.id}
                rowClassName={(r) => (r.severity === 'error' ? 'row-err' : undefined)}
                emptyLabel={hasFilter ? 'No errors match the filter.' : 'No errors recorded.'}
              />
            )}
          </Panel>
        </div>
      </div>
    </>
  )
}
