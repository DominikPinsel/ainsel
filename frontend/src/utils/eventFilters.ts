export type EventFilter = { field: string; op: string; value: string }

const SEPARATOR = '|'

export function encodeEventFilters(filters: EventFilter[]): string {
  return filters
    .filter((f) => f.field && f.op && f.value)
    .map((f) => `${encodeURIComponent(f.field)}:${encodeURIComponent(f.op)}:${encodeURIComponent(f.value)}`)
    .join(SEPARATOR)
}

export function parseEventFilters(param: string | null): EventFilter[] {
  if (!param) return []
  return param
    .split(SEPARATOR)
    .map((s) => {
      const i1 = s.indexOf(':')
      if (i1 < 0) return null
      const i2 = s.indexOf(':', i1 + 1)
      if (i2 < 0) return null
      return {
        field: decodeURIComponent(s.slice(0, i1)),
        op: decodeURIComponent(s.slice(i1 + 1, i2)),
        value: decodeURIComponent(s.slice(i2 + 1)),
      }
    })
    .filter((f): f is EventFilter => f !== null && !!(f.field && f.op && f.value))
}

function getPath(obj: unknown, path: string): unknown {
  const parts = path.split('.')
  let cur: unknown = obj
  for (const part of parts) {
    if (typeof cur !== 'object' || cur === null) return undefined
    cur = (cur as Record<string, unknown>)[part]
  }
  return cur
}

export function matchesEventFilters(payload: unknown, filters: EventFilter[]): boolean {
  return filters.every((f) => {
    const raw = getPath(payload, f.field)
    const val = raw === undefined || raw === null ? '' : String(raw)
    switch (f.op) {
      case 'eq': return val === f.value
      case 'neq': return val !== f.value
      case 'contains': return val.includes(f.value)
      case 'startsWith': return val.startsWith(f.value)
      case 'endsWith': return val.endsWith(f.value)
      default: return false
    }
  })
}
