import type { ReactNode } from 'react'

type PanelProps = {
  title?: string
  right?: ReactNode
  children: ReactNode
  className?: string
}

export function Panel({ title, right, children, className }: PanelProps) {
  const cls = ['panel', className].filter(Boolean).join(' ')
  return (
    <section className={cls}>
      {title ? (
        <header className="panel-header">
          <h3>{title}</h3>
          {right ? <div className="panel-header-right">{right}</div> : null}
        </header>
      ) : null}
      <div className="panel-body">{children}</div>
    </section>
  )
}
