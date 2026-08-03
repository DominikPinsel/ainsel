import { useMemo } from 'react'
import type { TokensSubjectRow } from '../../api/observability'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { Tag } from '../../primitives/Tag'
import { abbreviateNumber } from '../../utils/format'

type TokensTableProps = { rows: TokensSubjectRow[] }

const columns: readonly Column<TokensSubjectRow>[] = [
  {
    key: 'repo',
    header: 'Repo / Event',
    cell: (r) => (
      <>
        <b>{r.repo ?? '—'}</b>
        {r.eventType ? (
          <>
            {' '}
            <Tag>{r.eventType}</Tag>
          </>
        ) : null}
      </>
    ),
  },
  { key: 'agent', header: 'Agent', width: 160, cell: (r) => r.agentName ?? r.agent },
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
    key: 'total',
    header: 'Total',
    width: 90,
    align: 'right',
    cell: (r) => <span className="num">{abbreviateNumber(r.totalTokens)}</span>,
  },
]

export function TokensTable({ rows }: TokensTableProps) {
  const sorted = useMemo(
    () => [...rows].sort((a, b) => b.totalTokens - a.totalTokens),
    [rows],
  )
  return (
    <RegisterTable
      rows={sorted}
      columns={columns}
      rowKey={(r) => `${r.agent}|${r.repo ?? ''}|${r.eventType ?? ''}|${r.model ?? ''}`}
      emptyLabel="No token consumption recorded for this range."
    />
  )
}
