import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useRecentEvents } from '../../../api/events'
import type { ActivityEntry, ActivityStatus } from '../../../api/events'
import { useConnectors } from '../../../api/connectors'
import {
  useObservabilityTimeseries,
  useTokensByEvent,
  type Range,
} from '../../../api/observability'
import { ServiceUnavailableError } from '../../../api/client'
import { Titleblock } from '../../../layout/Titleblock'
import { Panel } from '../../../primitives/Panel'
import { Select } from '../../../primitives/Select'
import { SectionStatus } from '../../../primitives/SectionStatus'
import { TimeSeriesChart, type SeriesStyle } from '../../../primitives/TimeSeriesChart'
import { RangeSelector } from '../RangeSelector'
import { ActivityRow } from '../../activity/ActivityRow'
import { Pager } from '../../../primitives/Pager'
import { matchesEventFilters, parseEventFilters } from '../../../utils/eventFilters'
import { EventFilterBar } from './EventFilterBar'

const VALID_RANGES = ['1h', '6h', '24h', '7d'] as const
const isRange = (s: string | null): s is Range =>
  s !== null && (VALID_RANGES as readonly string[]).includes(s)

const STATUS_OPTIONS = [
  { value: 'matched', label: 'Matched' },
  { value: 'unmatched', label: 'Unmatched' },
  { value: 'error', label: 'Error' },
] as const

const PAGE_SIZE = 20
const PAGE_SIZE_OPTIONS = [20] as const

const CHART_STYLES: SeriesStyle[] = [
  { name: 'events_consumed', color: 'var(--ink)', label: 'Consumed' },
]

export function EventsDetail() {
  const [params, setParams] = useSearchParams()
  const range: Range = isRange(params.get('range')) ? (params.get('range') as Range) : '24h'
  const filterStatus = params.get('status') ?? ''
  const filterConnector = params.get('connector') ?? ''
  const page = Math.max(1, Number(params.get('page')) || 1)
  const payloadFilters = parseEventFilters(params.get('q'))

  const filterAgent = params.get('agent') ?? ''

  const setRange = (next: Range) =>
    setParams((prev) => { const p = new URLSearchParams(prev); p.set('range', next); p.delete('page'); return p }, { replace: true })

  const setStatus = (v: string) =>
    setParams((prev) => { const p = new URLSearchParams(prev); if (v) { p.set('status', v) } else { p.delete('status') } p.delete('page'); return p }, { replace: true })

  const setConnector = (v: string) =>
    setParams((prev) => { const p = new URLSearchParams(prev); if (v) { p.set('connector', v) } else { p.delete('connector') } p.delete('page'); return p }, { replace: true })

  const clearAgent = () =>
    setParams((prev) => { const p = new URLSearchParams(prev); p.delete('agent'); p.delete('page'); return p }, { replace: true })

  const setPage = (n: number) =>
    setParams((prev) => { const p = new URLSearchParams(prev); p.set('page', String(n)); return p }, { replace: true })

  const { data, isLoading, error } = useRecentEvents(500)
  const { data: connectorData } = useConnectors({ pageSize: 200 })
  const timeseries = useObservabilityTimeseries({ range, metric: 'events_consumed' })
  const tokensByEvent = useTokensByEvent(range)

  const connectorNameById = useMemo(() => {
    const map = new Map<string, string>()
    for (const c of connectorData?.items ?? []) map.set(c.id, c.name)
    return map
  }, [connectorData])

  const connectorOptions = useMemo(() => {
    const rows = data ?? []
    return Array.from(new Set(rows.map((r) => r.connector).filter((v): v is string => !!v)))
      .sort()
      .map((v) => ({ value: v, label: connectorNameById.get(v) ?? v }))
  }, [data, connectorNameById])

  const filtered: ActivityEntry[] = useMemo(() => {
    const rows = (data ?? []).slice().sort(
      (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime(),
    )
    return rows.filter((r) => {
      if (filterStatus && r.status !== (filterStatus as ActivityStatus)) return false
      if (filterConnector && r.connector !== filterConnector) return false
      if (filterAgent && !r.matches?.some((m) => m.agent === filterAgent)) return false
      if (payloadFilters.length > 0 && !matchesEventFilters(r.payload, payloadFilters)) return false
      return true
    })
  }, [data, filterStatus, filterConnector, filterAgent, payloadFilters])

  const paginated = useMemo(() => {
    const start = (page - 1) * PAGE_SIZE
    return filtered.slice(start, start + PAGE_SIZE)
  }, [filtered, page])

  // Build a lookup map from event ID → total tokens for the current range.
  const tokensMap = useMemo(() => {
    const map = new Map<string, number>()
    for (const row of tokensByEvent.data?.rows ?? []) {
      map.set(row.event, row.totalTokens)
    }
    return map
  }, [tokensByEvent.data])

  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())
  const toggleExpanded = (id: string) =>
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) { next.delete(id) } else { next.add(id) }
      return next
    })

  const chartState =
    timeseries.isLoading ? 'loading'
    : timeseries.error instanceof ServiceUnavailableError ? 'unavailable'
    : timeseries.error ? 'error'
    : 'ready'

  const chartSeries = useMemo(
    () => [{ name: 'events_consumed', points: timeseries.data?.points ?? [] }],
    [timeseries.data],
  )

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Operations / <Link to="/observability">Observability</Link> / <b>Events</b>
          </>
        }
        title={<>Events <em>Detail</em></>}
        actions={<RangeSelector value={range} onChange={setRange} />}
      />
      <div style={{ padding: '28px 32px', display: 'grid', gap: 24 }}>
        <Panel title={`Consumed · ${range}`} className="cropped">
          {chartState === 'ready' ? (
            <TimeSeriesChart
              series={chartSeries}
              styles={CHART_STYLES}
              ariaLabel={`Events consumed over ${range}`}
            />
          ) : (
            <SectionStatus state={chartState} onRetry={() => timeseries.refetch()} />
          )}
        </Panel>

        <div>
          <EventFilterBar />
          <div className="toolbar" style={{ marginBottom: 12 }}>
            {filterAgent ? (
              <div className="field">
                <span className="label">Agent</span>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span className="label" style={{ fontWeight: 600 }}>{filterAgent}</span>
                  <button type="button" className="remove" onClick={clearAgent}>×</button>
                </div>
              </div>
            ) : null}
            <div className="field">
              <span className="label">Status</span>
              <Select
                aria-label="Filter by status"
                value={filterStatus}
                onChange={setStatus}
                options={STATUS_OPTIONS}
                emptyLabel="Any status"
              />
            </div>
            <div className="field">
              <span className="label">Connector</span>
              <Select
                aria-label="Filter by connector"
                value={filterConnector}
                onChange={setConnector}
                options={connectorOptions}
                emptyLabel="Any connector"
              />
            </div>
          </div>

          <Panel
            title={`Events · ${filtered.length}${filtered.length === (data?.length ?? 0) ? '' : ` of ${data?.length ?? 0}`}`}
            className="cropped"
          >
            {isLoading ? (
              <div className="label" style={{ padding: 14 }}>Loading…</div>
            ) : error ? (
              <div className="label" style={{ padding: 14, color: 'var(--signal)' }}>
                Failed to load events.
              </div>
            ) : filtered.length === 0 ? (
              <div className="label" style={{ padding: 14 }}>
                {(data?.length ?? 0) === 0 ? 'No recent events.' : 'No events match the filter.'}
              </div>
            ) : (
              <>
                <div className="reg-wrap">
                  <table className="reg">
                    <thead>
                      <tr>
                        <th>When</th>
                        <th>Connector</th>
                        <th>Trigger</th>
                        <th>Agent</th>
                        <th>Status</th>
                        <th>Tokens</th>
                      </tr>
                    </thead>
                    <tbody>
                      {paginated.map((e) => (
                        <ActivityRow
                          key={e.id}
                          entry={e}
                          connectorName={e.connector ? connectorNameById.get(e.connector) : undefined}
                          expanded={expandedIds.has(e.id)}
                          onToggle={() => toggleExpanded(e.id)}
                          tokens={tokensMap.get(e.id) ?? 0}
                        />
                      ))}
                    </tbody>
                  </table>
                </div>
                <Pager
                  page={page}
                  pageSize={PAGE_SIZE}
                  total={filtered.length}
                  pageSizeOptions={PAGE_SIZE_OPTIONS}
                  onPageChange={setPage}
                  onPageSizeChange={() => {}}
                />
              </>
            )}
          </Panel>
        </div>
      </div>
    </>
  )
}
