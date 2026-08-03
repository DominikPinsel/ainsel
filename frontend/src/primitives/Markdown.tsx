import { useEffect, useMemo, useRef } from 'react'
import { marked } from 'marked'
import DOMPurify from 'dompurify'

type MarkdownProps = {
  source: string
  className?: string
}

// Lazy-load mermaid only when a doc actually contains a diagram.
let mermaidPromise: Promise<typeof import('mermaid')['default']> | null = null

function loadMermaid() {
  if (!mermaidPromise) {
    mermaidPromise = import('mermaid').then((mod) => {
      const mermaid = mod.default
      mermaid.initialize({ startOnLoad: false, theme: 'dark' })
      return mermaid
    })
  }
  return mermaidPromise
}

export function Markdown({ source, className }: MarkdownProps) {
  const ref = useRef<HTMLDivElement>(null)
  const html = useMemo(() => {
    const raw = marked.parse(source ?? '', { async: false }) as string
    return DOMPurify.sanitize(raw)
  }, [source])

  // Open external links in a new tab so users don't navigate away from the
  // app when following http/https links in markdown content.
  useEffect(() => {
    if (!ref.current) return
    ref.current.querySelectorAll('a[href^="http"]').forEach((a) => {
      a.setAttribute('target', '_blank')
      a.setAttribute('rel', 'noopener noreferrer')
    })
  }, [html])

  // Render mermaid code blocks (```mermaid) as SVG diagrams.
  useEffect(() => {
    const container = ref.current
    if (!container) return
    const blocks = container.querySelectorAll('code.language-mermaid')
    if (blocks.length === 0) return
    let cancelled = false
    loadMermaid().then(async (mermaid) => {
      if (cancelled || !ref.current) return
      for (let i = 0; i < blocks.length; i++) {
        const code = blocks[i]
        const pre = code.parentElement
        if (!pre) continue
        const graphDef = code.textContent || ''
        try {
          const { svg } = await mermaid.render(`mermaid-svg-${i}`, graphDef)
          const div = document.createElement('div')
          div.className = 'mermaid-diagram'
          div.innerHTML = svg
          pre.replaceWith(div)
        } catch {
          // leave as a code block on parse error
        }
      }
    })
    return () => {
      cancelled = true
    }
  }, [html])

  const cls = ['md-body', className].filter(Boolean).join(' ')
  return <div ref={ref} className={cls} dangerouslySetInnerHTML={{ __html: html }} />
}