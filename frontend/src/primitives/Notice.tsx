import type { ReactNode } from 'react'

export type NoticeVariant = 'info' | 'warn' | 'err'

type NoticeProps = {
  /**
   * `info` — a quiet confirmation (neutral, because Fjord's `--ok` is ink, not
   * green). `warn` — recoverable, amber. `err` — the operation failed, signal.
   */
  variant?: NoticeVariant
  children: ReactNode
}

const ROLE: Record<NoticeVariant, 'status' | 'alert'> = {
  info: 'status',
  warn: 'alert',
  err: 'alert',
}

/**
 * Inline status banner for forms.
 *
 * Replaces the hand-rolled `<div role="alert" style={{…}}>` blocks that each
 * form grew on its own. Those referenced `--warning` / `--success`, which the
 * Fjord palette does not define, so they rendered hardcoded Tailwind amber and
 * green with square corners — the only saturated boxes in the console.
 */
export function Notice({ variant = 'info', children }: NoticeProps) {
  const cls = ['notice', variant !== 'info' && `notice-${variant}`].filter(Boolean).join(' ')
  return (
    <div role={ROLE[variant]} className={cls}>
      {children}
    </div>
  )
}
