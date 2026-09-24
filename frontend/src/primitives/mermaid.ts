// Shared lazy Mermaid loader. Diagrams are rare enough that the bundle should
// only be fetched when one is actually rendered — markdown bodies and the
// channel relation graph both go through this module.
let mermaidPromise: Promise<(typeof import('mermaid'))['default']> | null = null

export function loadMermaid(): Promise<(typeof import('mermaid'))['default']> {
  if (!mermaidPromise) {
    mermaidPromise = import('mermaid').then((mod) => {
      const mermaid = mod.default
      mermaid.initialize({ startOnLoad: false, theme: 'dark', securityLevel: 'strict' })
      return mermaid
    })
  }
  return mermaidPromise
}
