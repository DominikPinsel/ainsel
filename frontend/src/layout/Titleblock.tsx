import type { ReactNode } from 'react'
import './Titleblock.css'

type TitleblockProps = {
  crumbs?: ReactNode
  title: ReactNode
  actions?: ReactNode
}

export function Titleblock({ crumbs, title, actions }: TitleblockProps) {
  return (
    <header className="titleblock">
      <div>
        {crumbs ? <div className="crumbs">{crumbs}</div> : null}
        <h2>{title}</h2>
      </div>
      {actions ? <div className="actions">{actions}</div> : null}
    </header>
  )
}
