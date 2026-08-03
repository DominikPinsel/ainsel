import { useMemo } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useTokensBySubject, useTokensSummary } from '../../../api/observability'
import type { TokensSubjectRow, Range } from '../../../api/observability'
import { Titleblock } from '../../../layout/Titleblock'
import { Panel } from '../../../primitives/Panel'
import { RegisterTable, type Column } from '../../../primitives/RegisterTable'
import { Tag } from '../../../primitives/Tag'
import { SectionStatus } from '../../../primitives/SectionStatus'
import { RangeSelector } from '../RangeSelector'
import { abbreviateNumber } from '../../../utils/format'
import { eventsLink } from '../../../utils/obsLinks'

const VALID_RANGES = ['1h', '6h', '24h', '7d'] as const
const isRange = (s: string | null): s is Range =>
  s !== null && (VALID_RANGES as readonly string[]).includes(s)

export function TokensDetail() {
  const [params, setParams] = useSearchParams()
  const range: Range = isRange(params.get('range')) ? (params.get('range') as Range) : '24h'

  const setRange = (next: Range) =>
    setParams((prev) => { const p = new URLSearchParams(prev); p.set('range', next); return p }, { replace: true })

  const summary = useTokensSummary()
  const bySubject = useTokensBySubject(range)

  const rows: TokensSubjectRow[] = useMemo(
    () => [...(bySubject.data?.rows ?? [])].sort((a, b) => b.totalTokens - a.totalTokens),
    [bySubject.data],
  )

  const totals = useMemo(() => {
    const input = rows.reduce((s, r) => s + r.inputTokens, 0)
    const output = rows.reduce((s, r) => s + r.outputTokens, 0)
    return { input, output }
  }, [rows])

  const columns: readonly Column<TokensSubjectRow>[] = [
    {
      key: 'repo',
      header: 'Repo / Event',
      cell: (r) => (
        <>
          <b>{r.repo ?? '—'}</b>
          {r.eventType ? <> <Tag>{r.eventType}</Tag></> : null}
        </>
      ),
    },
    { key: 'agent', header: 'Agent', width: 160, cell: (r) => r.agent },
    {
      key: 'model',
      header: 'Model',
      width: 160,
      cell: (r) => <span className="num">{r.model ?? '—'}</span>,
    },
    {
      key: 'input',
      header: 'Input',
      width: 90,
      align: 'right',
      cell: (r) => <span className="num">{abbreviateNumber(r.inputTokens)}</span>,
    },
    {
      key: 'output',
      header: 'Output',
      width: 90,
      align: 'right',
      cell: (r) => <span className="num">{abbreviateNumber(r.outputTokens)}</span>,
    },
    {
      key: 'ratio',
      header: 'I/O',
      width: 70,
      align: 'right',
      cell: (r) =>
        r.outputTokens > 0 ? (
          <span className="num">{(r.inputTokens / r.outputTokens).toFixed(1)}×</span>
        ) : (
          <span className="label">—</span>
        ),
    },
    {
      key: 'total',
      header: 'Total',
      width: 90,
      align: 'right',
      cell: (r) => <span className="num">{abbreviateNumber(r.totalTokens)}</span>,
    },
    {
      key: 'events',
      header: '',
      width: 120,
      align: 'right',
      cell: (r) => (
        <Link
          to={eventsLink({ agent: r.agent, range })}
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
            Operations / <Link to="/observability">Observability</Link> / <b>Tokens</b>
          </>
        }
        title={<>Token <em>Detail</em></>}
        actions={<RangeSelector value={range} onChange={setRange} />}
      />
      <div style={{ padding: '28px 32px', display: 'grid', gap: 24 }}>
        <div className="kpi-row" style={{ gridTemplateColumns: 'repeat(3, 1fr)' }}>
          <div className="kpi">
            <div className="hd">
              <span className="label">Total Tokens (all time)</span>
            </div>
            <div className="figure num">
              {summary.data ? abbreviateNumber(summary.data.totalTokens) : '—'}
            </div>
          </div>
          <div className="kpi">
            <div className="hd">
              <span className="label">Input · {range}</span>
            </div>
            <div className="figure num">{abbreviateNumber(totals.input)}</div>
          </div>
          <div className="kpi">
            <div className="hd">
              <span className="label">Output · {range}</span>
            </div>
            <div className="figure num">{abbreviateNumber(totals.output)}</div>
          </div>
        </div>

        <Panel title={`By Agent · ${range}`} className="cropped">
          {bySubject.isLoading ? (
            <SectionStatus state="loading" />
          ) : bySubject.error ? (
            <SectionStatus state="error" onRetry={() => bySubject.refetch()} />
          ) : (
            <RegisterTable
              rows={rows}
              columns={columns}
              rowKey={(r) => `${r.agent}|${r.repo ?? ''}|${r.eventType ?? ''}|${r.model ?? ''}`}
              emptyLabel="No token consumption recorded for this range."
            />
          )}
        </Panel>
      </div>
    </>
  )
}
