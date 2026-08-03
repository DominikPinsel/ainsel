type Tab = { value: string; label: string }

type TabsProps = {
  value: string
  onChange: (next: string) => void
  tabs: readonly Tab[]
  'aria-label'?: string
}

export function Tabs({ value, onChange, tabs, ...rest }: TabsProps) {
  return (
    <div className="tabs" role="tablist" aria-label={rest['aria-label']}>
      {tabs.map((t) => (
        <button
          key={t.value}
          type="button"
          role="tab"
          aria-selected={t.value === value}
          className={t.value === value ? 'tab active' : 'tab'}
          onClick={() => onChange(t.value)}
        >
          {t.label}
        </button>
      ))}
    </div>
  )
}
