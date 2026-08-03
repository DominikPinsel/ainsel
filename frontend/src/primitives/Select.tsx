type Option = {
  value: string
  label: string
}

type SelectProps = {
  id?: string
  value: string
  onChange: (next: string) => void
  options: readonly Option[]
  emptyLabel?: string
  size?: 'md' | 'sm'
  disabled?: boolean
  className?: string
  'aria-label'?: string
}

export function Select({
  id,
  value,
  onChange,
  options,
  emptyLabel,
  size = 'md',
  disabled,
  className,
  ...rest
}: SelectProps) {
  const cls = ['select', size === 'sm' && 'sm', className].filter(Boolean).join(' ')
  return (
    <select
      id={id}
      className={cls}
      value={value}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value)}
      aria-label={rest['aria-label']}
    >
      {emptyLabel !== undefined ? <option value="">{emptyLabel}</option> : null}
      {options.map((opt) => (
        <option key={opt.value} value={opt.value}>
          {opt.label}
        </option>
      ))}
    </select>
  )
}
