import { describe, expect, it } from 'vitest'
import { encodeEventFilters, matchesEventFilters, parseEventFilters } from './eventFilters'

describe('encodeEventFilters / parseEventFilters round-trip', () => {
  it('round-trips a single simple filter', () => {
    const filters = [{ field: 'action', op: 'eq', value: 'opened' }]
    expect(parseEventFilters(encodeEventFilters(filters))).toEqual(filters)
  })

  it('round-trips multiple filters', () => {
    const filters = [
      { field: 'action', op: 'eq', value: 'opened' },
      { field: 'headers.X-Forgejo-Event', op: 'contains', value: 'push' },
    ]
    expect(parseEventFilters(encodeEventFilters(filters))).toEqual(filters)
  })

  it('handles values containing special chars (colon and pipe)', () => {
    const filters = [{ field: 'body', op: 'contains', value: 'a:b|c' }]
    expect(parseEventFilters(encodeEventFilters(filters))).toEqual(filters)
  })

  it('skips filters with empty field or value during encode', () => {
    const encoded = encodeEventFilters([{ field: '', op: 'eq', value: 'x' }])
    expect(encoded).toBe('')
  })

  it('encodeEventFilters returns empty string for empty array', () => {
    expect(encodeEventFilters([])).toBe('')
  })

  it('parseEventFilters returns empty array for null', () => {
    expect(parseEventFilters(null)).toEqual([])
  })

  it('parseEventFilters returns empty array for empty string', () => {
    expect(parseEventFilters('')).toEqual([])
  })

  it('parseEventFilters ignores malformed segments', () => {
    expect(parseEventFilters('nocolons')).toEqual([])
  })
})

describe('matchesEventFilters', () => {
  const payload = {
    action: 'opened',
    status: 200,
    nested: { key: 'value' },
  }

  it('returns true for empty filter list', () => {
    expect(matchesEventFilters(payload, [])).toBe(true)
  })

  it('eq matches exact value', () => {
    expect(matchesEventFilters(payload, [{ field: 'action', op: 'eq', value: 'opened' }])).toBe(true)
  })

  it('eq does not match different value', () => {
    expect(matchesEventFilters(payload, [{ field: 'action', op: 'eq', value: 'closed' }])).toBe(false)
  })

  it('neq matches when value differs', () => {
    expect(matchesEventFilters(payload, [{ field: 'action', op: 'neq', value: 'closed' }])).toBe(true)
  })

  it('neq does not match when value is equal', () => {
    expect(matchesEventFilters(payload, [{ field: 'action', op: 'neq', value: 'opened' }])).toBe(false)
  })

  it('contains matches substring', () => {
    expect(matchesEventFilters(payload, [{ field: 'action', op: 'contains', value: 'open' }])).toBe(true)
  })

  it('contains does not match absent substring', () => {
    expect(matchesEventFilters(payload, [{ field: 'action', op: 'contains', value: 'xyz' }])).toBe(false)
  })

  it('startsWith matches prefix', () => {
    expect(matchesEventFilters(payload, [{ field: 'action', op: 'startsWith', value: 'open' }])).toBe(true)
  })

  it('startsWith does not match wrong prefix', () => {
    expect(matchesEventFilters(payload, [{ field: 'action', op: 'startsWith', value: 'ned' }])).toBe(false)
  })

  it('endsWith matches suffix', () => {
    expect(matchesEventFilters(payload, [{ field: 'action', op: 'endsWith', value: 'ned' }])).toBe(true)
  })

  it('endsWith does not match wrong suffix', () => {
    expect(matchesEventFilters(payload, [{ field: 'action', op: 'endsWith', value: 'open' }])).toBe(false)
  })

  it('navigates nested dot-notation paths', () => {
    expect(matchesEventFilters(payload, [{ field: 'nested.key', op: 'eq', value: 'value' }])).toBe(true)
  })

  it('returns false for unknown op', () => {
    expect(matchesEventFilters(payload, [{ field: 'action', op: 'regex', value: '.*' }])).toBe(false)
  })

  it('coerces numeric field to string for comparison', () => {
    expect(matchesEventFilters(payload, [{ field: 'status', op: 'eq', value: '200' }])).toBe(true)
  })

  it('returns false when payload is null', () => {
    expect(matchesEventFilters(null, [{ field: 'action', op: 'eq', value: 'opened' }])).toBe(false)
  })

  it('all filters must match (AND semantics)', () => {
    const filters = [
      { field: 'action', op: 'eq', value: 'opened' },
      { field: 'nested.key', op: 'eq', value: 'wrong' },
    ]
    expect(matchesEventFilters(payload, filters)).toBe(false)
  })
})
