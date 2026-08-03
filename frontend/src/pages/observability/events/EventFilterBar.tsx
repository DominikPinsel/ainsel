import { useCallback, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Input } from '../../../primitives/Input'
import { Select } from '../../../primitives/Select'
import { encodeEventFilters, parseEventFilters, type EventFilter } from '../../../utils/eventFilters'

const OP_OPTIONS = [
  { value: 'eq', label: 'eq' },
  { value: 'neq', label: 'neq' },
  { value: 'contains', label: 'contains' },
  { value: 'startsWith', label: 'startsWith' },
  { value: 'endsWith', label: 'endsWith' },
] as const

type Row = EventFilter & { id: number }

function makeRows(filters: EventFilter[], startId: number): Row[] {
  return filters.map((f, i) => ({ ...f, id: startId + i }))
}

export function EventFilterBar() {
  const [params, setParams] = useSearchParams()
  const nextId = useRef(0)
  const [rows, setRows] = useState<Row[]>(() => {
    const parsed = parseEventFilters(params.get('q'))
    const result = makeRows(parsed, nextId.current)
    nextId.current += parsed.length
    return result
  })

  const syncUrl = useCallback(
    (next: Row[]) => {
      setParams(
        (prev) => {
          const p = new URLSearchParams(prev)
          const complete = next.filter((r) => r.field && r.op && r.value)
          const encoded = encodeEventFilters(complete)
          if (encoded) p.set('q', encoded)
          else p.delete('q')
          return p
        },
        { replace: true },
      )
    },
    [setParams],
  )

  const update = (id: number, patch: Partial<EventFilter>) => {
    setRows((prev) => {
      const next = prev.map((r) => (r.id === id ? { ...r, ...patch } : r))
      syncUrl(next)
      return next
    })
  }

  const remove = (id: number) => {
    setRows((prev) => {
      const next = prev.filter((r) => r.id !== id)
      syncUrl(next)
      return next
    })
  }

  const add = () => {
    const id = nextId.current++
    setRows((prev) => [...prev, { field: '', op: 'eq', value: '', id }])
  }

  return (
    <div style={{ marginBottom: 12 }}>
      {rows.map((f, i) => (
        <div key={f.id} className="filter-row">
          <Input
            placeholder="field  e.g. action"
            value={f.field}
            onChange={(e) => update(f.id, { field: e.target.value })}
            aria-label={`Filter ${i + 1} field`}
          />
          <Select
            value={f.op}
            onChange={(v) => update(f.id, { op: v })}
            options={OP_OPTIONS}
            aria-label={`Filter ${i + 1} operator`}
          />
          <Input
            placeholder="value"
            value={f.value}
            onChange={(e) => update(f.id, { value: e.target.value })}
            aria-label={`Filter ${i + 1} value`}
          />
          <button
            type="button"
            className="remove"
            onClick={() => remove(f.id)}
            aria-label={`Remove filter ${i + 1}`}
          >
            ×
          </button>
        </div>
      ))}
      <button type="button" className="filter-add" onClick={add}>
        ＋ Add payload filter
      </button>
      {rows.length > 0 && (
        <p className="label" style={{ marginTop: 6, color: 'var(--ink-3)' }}>
          Tip: use <code>type</code> to filter by event type,{' '}
          <code>action</code> for the action field.
        </p>
      )}
    </div>
  )
}
