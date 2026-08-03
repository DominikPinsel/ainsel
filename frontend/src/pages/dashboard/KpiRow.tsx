import { NavLink } from 'react-router-dom'
import { Sparkbar } from '../../primitives/Sparkbar'

type Plate = {
  key: string
  label: string
  code: string
  to: string
  count: number | undefined
  sub: string
  spark: number[]
  alert?: boolean
}

type KpiRowProps = {
  plates: Plate[]
}

export function KpiRow({ plates }: KpiRowProps) {
  return (
    <div className="kpi-row">
      {plates.map((p) => (
        <NavLink key={p.key} to={p.to} className={p.alert ? 'kpi alert' : 'kpi'}>
          <div className="hd">
            <span className="label">{p.label}</span>
            <span className="code">{p.code}</span>
          </div>
          <div className="figure num">{p.count ?? '—'}</div>
          <Sparkbar values={p.spark} alert={p.alert} />
          <div className="sub">
            <span>{p.sub}</span>
          </div>
        </NavLink>
      ))}
    </div>
  )
}
