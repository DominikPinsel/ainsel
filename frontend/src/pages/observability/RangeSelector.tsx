import type { Range } from '../../api/observability'

const RANGES: Range[] = ['1h', '6h', '24h', '7d']

type RangeSelectorProps = {
  value: Range
  onChange: (next: Range) => void
}

export function RangeSelector({ value, onChange }: RangeSelectorProps) {
  return (
    <div className="range-selector" role="group" aria-label="Time range">
      {RANGES.map((r) => (
        <button
          key={r}
          type="button"
          className={r === value ? 'active' : ''}
          aria-pressed={r === value}
          onClick={() => onChange(r)}
        >
          {r}
        </button>
      ))}
    </div>
  )
}
