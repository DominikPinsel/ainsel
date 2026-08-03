type CheckProps = {
  checked: boolean
  onChange: (next: boolean) => void
  'aria-label': string
  disabled?: boolean
}

export function Check({ checked, onChange, disabled, ...rest }: CheckProps) {
  return (
    <div
      role="checkbox"
      aria-checked={checked}
      aria-disabled={disabled}
      aria-label={rest['aria-label']}
      tabIndex={disabled ? -1 : 0}
      className={`check${checked ? ' on' : ''}`}
      onClick={() => !disabled && onChange(!checked)}
      onKeyDown={(e) => {
        if (disabled) return
        if (e.key === ' ' || e.key === 'Enter') {
          e.preventDefault()
          onChange(!checked)
        }
      }}
    />
  )
}
