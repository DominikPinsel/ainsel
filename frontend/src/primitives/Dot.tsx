type State = 'ok' | 'warn' | 'err' | 'stale'

type DotProps = {
  state: State
  'aria-label'?: string
}

export function Dot({ state, 'aria-label': label }: DotProps) {
  const cls = `dot ${state}`
  return label ? (
    <span className={cls} role="img" aria-label={label} />
  ) : (
    <span className={cls} />
  )
}
