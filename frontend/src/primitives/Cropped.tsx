import type { ReactNode } from 'react'

type CroppedProps = {
  children: ReactNode
  className?: string
}

export function Cropped({ children, className }: CroppedProps) {
  const cls = ['cropped', className].filter(Boolean).join(' ')
  return (
    <div className={cls}>
      {children}
      <div className="c1" />
      <div className="c2" />
    </div>
  )
}
