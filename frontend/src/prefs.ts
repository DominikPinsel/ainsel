// Shared broadcast channel for localStorage-backed preferences: the write
// side dispatches PREF_CHANGE_EVENT so components reading the same keys
// re-render (see agentRecents / Spine).
export const PREF_CHANGE_EVENT = 'ainsel-pref-change'
