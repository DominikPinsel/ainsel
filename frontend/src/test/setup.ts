import '@testing-library/jest-dom/vitest'
import { configure } from '@testing-library/react'

// Testing Library's async utils (`waitFor`, `findBy*`) default to a 1000 ms
// wall-clock budget, and that budget is spent waiting for a scheduler tick — so
// what consumes it is CPU contention, not the page. ChannelDetailPage's first
// render, the assertion that flakes in #245, settles in ~66 ms on an idle
// machine (cold first query in the file), 213 ms with the suite pinned to 2
// workers under moderate load, and 1031 ms under heavy load. Same page, same
// work; only the clock changed. The page is fine, the budget is what fails.
//
// Raising it here lifts every async assertion in the suite off the cliff instead
// of hand-tuning one `waitFor` at a time, and only costs time on runs that were
// going to fail anyway.
configure({ asyncUtilTimeout: 3000 })

// Node ≥ 22 ships an experimental `localStorage` global that is `undefined`
// unless `--localstorage-file` is passed.  This shadows the jsdom-provided
// implementation, so tests that touch `localStorage` blow up.  Provide a
// simple in-memory polyfill when the real one is missing.
if (typeof globalThis.localStorage === 'undefined' || globalThis.localStorage === null) {
  const store = new Map<string, string>()
  const storage: Storage = {
    get length() { return store.size },
    clear: () => store.clear(),
    getItem: (k: string) => store.get(k) ?? null,
    key: (i: number) => [...store.keys()][i] ?? null,
    removeItem: (k: string) => { store.delete(k) },
    setItem: (k: string, v: string) => { store.set(k, String(v)) },
  }
  Object.defineProperty(globalThis, 'localStorage', { value: storage, writable: true, configurable: true })
  Object.defineProperty(window, 'localStorage', { value: storage, writable: true, configurable: true })
}

// jsdom does not implement scrolling APIs. Components that auto-scroll on
// render (e.g. ChatView) call HTMLElement.scrollTo, which would otherwise throw
// "scrollRef.current?.scrollTo is not a function" during tests.
if (!HTMLElement.prototype.scrollTo) {
  HTMLElement.prototype.scrollTo = () => {}
}
if (!HTMLElement.prototype.scrollIntoView) {
  HTMLElement.prototype.scrollIntoView = () => {}
}
