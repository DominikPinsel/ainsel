import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  getAgentRecents,
  RECENT_AGENT_LIMIT,
  recordAgentView,
  recentsScope,
} from './agentRecents'
import { PREF_CHANGE_EVENT } from './prefs'

const KEY = 'ainsel-agent-recents:sub-123'

beforeEach(() => {
  localStorage.clear()
})
afterEach(() => {
  vi.restoreAllMocks()
})

describe('recentsScope', () => {
  it('buckets by the stable sub, not the username', () => {
    const user = { sub: 'sub-123', username: 'kim' }
    expect(recentsScope(user)).toBe('sub-123')
  })

  it('falls back to anon with no user, so unauthenticated dev still works', () => {
    expect(recentsScope(undefined)).toBe('anon')
    expect(recentsScope(null)).toBe('anon')
    expect(recentsScope({})).toBe('anon')
    // An empty sub would otherwise produce a key ending in a bare colon.
    expect(recentsScope({ sub: '' })).toBe('anon')
  })
})

describe('recordAgentView', () => {
  it('keeps most-recently-viewed first and moves a revisit to the front', () => {
    recordAgentView('sub-123', 'a')
    recordAgentView('sub-123', 'b')
    recordAgentView('sub-123', 'c')
    expect(getAgentRecents('sub-123')).toEqual(['c', 'b', 'a'])
    recordAgentView('sub-123', 'a')
    expect(getAgentRecents('sub-123')).toEqual(['a', 'c', 'b'])
    // No duplicate left behind where 'a' used to sit.
    expect(getAgentRecents('sub-123')).toHaveLength(3)
  })

  it('caps the list so an old entry ages out', () => {
    for (const id of ['a', 'b', 'c', 'd', 'e', 'f']) {
      recordAgentView('sub-123', id)
    }
    const stored = getAgentRecents('sub-123')
    expect(stored).toHaveLength(RECENT_AGENT_LIMIT)
    expect(stored).toEqual(['f', 'e', 'd', 'c', 'b'])
    expect(stored).not.toContain('a')
    // Storage itself must not grow past the cap either.
    expect(JSON.parse(localStorage.getItem(KEY)!)).toHaveLength(RECENT_AGENT_LIMIT)
  })

  it('separates accounts sharing one browser', () => {
    recordAgentView('sub-123', 'mine')
    recordAgentView('sub-456', 'theirs')
    expect(getAgentRecents('sub-123')).toEqual(['mine'])
    expect(getAgentRecents('sub-456')).toEqual(['theirs'])
  })

  it('broadcasts the preference event so the nav re-reads without a remount', () => {
    const listener = vi.fn()
    window.addEventListener(PREF_CHANGE_EVENT, listener)
    recordAgentView('sub-123', 'a')
    window.removeEventListener(PREF_CHANGE_EVENT, listener)
    expect(listener).toHaveBeenCalledTimes(1)
  })

  it('stays quiet when the agent is already at the front', () => {
    recordAgentView('sub-123', 'a')
    const listener = vi.fn()
    window.addEventListener(PREF_CHANGE_EVENT, listener)
    recordAgentView('sub-123', 'a')
    const setItem = vi.spyOn(Storage.prototype, 'setItem')
    recordAgentView('sub-123', 'a')
    window.removeEventListener(PREF_CHANGE_EVENT, listener)
    expect(listener).not.toHaveBeenCalled()
    expect(setItem).not.toHaveBeenCalled()
  })
})

describe('getAgentRecents', () => {
  it('is empty when nothing was stored', () => {
    expect(getAgentRecents('nobody')).toEqual([])
  })

  it('survives a corrupted or hand-edited key', () => {
    localStorage.setItem(KEY, 'not json at all')
    expect(getAgentRecents('sub-123')).toEqual([])

    localStorage.setItem(KEY, '{"a":1}')
    expect(getAgentRecents('sub-123')).toEqual([])

    localStorage.setItem(KEY, '[1, null, "", "ok", {}]')
    expect(getAgentRecents('sub-123')).toEqual(['ok'])
  })
})
