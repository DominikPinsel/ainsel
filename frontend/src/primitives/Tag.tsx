import type { ReactNode } from 'react'

type Variant = 'default' | 'ok' | 'warn' | 'err' | 'stale'

type TagProps = {
  children: ReactNode
  variant?: Variant
  solid?: boolean
  className?: string
}

export function Tag({ children, variant = 'default', solid, className }: TagProps) {
  const cls = ['tag', variant !== 'default' && variant, solid && 'solid', className]
    .filter(Boolean)
    .join(' ')
  return <span className={cls}>{children}</span>
}
