type SparkbarProps = {
  values: number[]
  alert?: boolean
}

export function Sparkbar({ values, alert }: SparkbarProps) {
  const max = values.reduce((m, v) => (v > m ? v : m), 0)
  const cls = alert ? 'spark alert' : 'spark'
  return (
    <div className={cls}>
      {values.map((v, i) => {
        const pct = max === 0 ? 0 : Math.round((v / max) * 100)
        return <span key={i} style={{ height: `${pct}%` }} />
      })}
    </div>
  )
}
