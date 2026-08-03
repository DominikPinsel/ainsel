import { NavLink } from 'react-router-dom'
import type {
  ObservabilitySummary,
  Range,
  TokensSummary,
} from '../../api/observability'
import { Sparkbar } from '../../primitives/Sparkbar'
import { abbreviateNumber } from '../../utils/format'

type Card = {
  key: string
  label: string
  code: string
  value: number | undefined
  formatter: (n: number) => string
  sub: string
  alert?: boolean
  to?: string
  spark?: number[]
}

type SummaryCardsProps = {
  summary?: ObservabilitySummary
  tokens?: TokensSummary
  range?: Range
}

export function SummaryCards({ summary, tokens, range }: SummaryCardsProps) {
  const rangeSuffix = range ? ` · ${range}` : ''
  const cards: Card[] = [
    {
      key: 'consumed',
      label: `Events Consumed${rangeSuffix}`,
      code: 'EV·01',
      value: summary?.eventsConsumed,
      formatter: (n) => abbreviateNumber(n),
      sub: '',
      to: '/observability/events',
      spark: [4, 5, 6, 7, 8, 9],
    },
    {
      key: 'matched',
      label: `Triggers Matched${rangeSuffix}`,
      code: 'EV·02',
      value: summary?.triggersMatched,
      formatter: (n) => abbreviateNumber(n),
      sub: '',
      to: '/observability/routing',
      spark: [3, 4, 5, 5, 6, 7],
    },
    {
      key: 'routed',
      label: `Events Routed${rangeSuffix}`,
      code: 'EV·03',
      value: summary?.eventsRouted,
      formatter: (n) => abbreviateNumber(n),
      sub: '',
      to: '/observability/routing',
      spark: [3, 4, 4, 5, 6, 7],
    },
    {
      key: 'errors',
      label: `Errors${rangeSuffix}`,
      code: 'ER·04',
      value: summary?.routingErrors,
      formatter: (n) => abbreviateNumber(n),
      sub: '',
      to: '/observability/errors',
      alert: (summary?.routingErrors ?? 0) > 0,
      spark: [0, 1, 0, 2, 1, summary?.routingErrors ?? 0],
    },
    {
      key: 'tokens',
      label: `Tokens${rangeSuffix}`,
      code: 'TK·05',
      value: tokens?.totalTokens,
      formatter: (n) => abbreviateNumber(n),
      sub: '',
      to: '/observability/tokens',
      spark: [3, 4, 5, 4, 6, 7],
    },
  ]

  return (
    <div className="kpi-row" style={{ gridTemplateColumns: 'repeat(5, 1fr)' }}>
      {cards.map((c) => {
        const inner = (
          <>
            <div className="hd">
              <span className="label">{c.label}</span>
              <span className="code">{c.code}</span>
            </div>
            <div className="figure num">
              {c.value !== undefined ? c.formatter(c.value) : '—'}
            </div>
            {c.spark ? <Sparkbar values={c.spark} alert={c.alert} /> : null}
            <div className="sub">
              <span>{c.sub}</span>
            </div>
          </>
        )
        const className = c.alert ? 'kpi alert' : 'kpi'
        return c.to ? (
          <NavLink key={c.key} to={c.to} className={className}>
            {inner}
          </NavLink>
        ) : (
          <div key={c.key} className={className} style={{ cursor: 'default' }}>
            {inner}
          </div>
        )
      })}
    </div>
  )
}
