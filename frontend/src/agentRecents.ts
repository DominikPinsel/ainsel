import { PREF_CHANGE_EVENT } from './prefs'

/**
 * How many agents the nav remembers, and the ceiling on what gets stored.
 * Keeping the two equal means the list can never hold entries the nav will
 * not show, so nothing quietly pushes a real recent out of the window.
 */
export const RECENT_AGENT_LIMIT = 5

const KEY_PREFIX = 'ainsel-agent-recents'

function storageKey(scope: string): string {
  return `${KEY_PREFIX}:${scope}`
}

/**
 * Bucket recents per account so two operators sharing one browser do not
 * inherit each other's list. `sub` is the stable identifier rather than
 * `username`, so renaming an account keeps its history. Unauthenticated
 * local dev (`mode: 'none'`) has no user at all; `'anon'` keeps the key from
 * becoming `...:undefined`.
 */
export function recentsScope(user: { sub?: string } | null | undefined): string {
  return user?.sub || 'anon'
}

/** Most-recently-viewed first. Never throws, whatever is in storage. */
export function getAgentRecents(scope: string): string[] {
  try {
    const raw = localStorage.getItem(storageKey(scope))
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    // A hand-edited or half-written key must not take the nav down with it.
    if (!Array.isArray(parsed)) return []
    return (parsed as unknown[])
      .filter((id): id is string => typeof id === 'string' && id.length > 0)
      .slice(0, RECENT_AGENT_LIMIT)
  } catch {
    return []
  }
}

/**
 * Remember that an agent was opened, moving it to the front. Broadcasts the
 * existing preference-change event so already-mounted surfaces — the Spine's
 * recent list above all — re-read without a remount.
 */
export function recordAgentView(scope: string, id: string): void {
  const current = getAgentRecents(scope)
  if (current[0] === id) return // already front: no write, no event
  const next = [id, ...current.filter((x) => x !== id)].slice(0, RECENT_AGENT_LIMIT)
  try {
    localStorage.setItem(storageKey(scope), JSON.stringify(next))
  } catch {
    /* private mode / quota: recents are a convenience, not a failure */
  }
  window.dispatchEvent(new CustomEvent(PREF_CHANGE_EVENT))
}
