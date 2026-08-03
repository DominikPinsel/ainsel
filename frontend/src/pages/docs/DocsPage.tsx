import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { marked } from 'marked'
import { Markdown } from '../../primitives/Markdown'
import { Titleblock } from '../../layout/Titleblock'
import './DocsPage.css'

// The in-app Docs page browses the repository's own `docs/` directory at
// runtime (docsify-style). The navigation is driven by docs/_sidebar.md —
// add a new `- [Title](slug)` line plus a `slug.md` file and it appears with
// zero code changes. Markdown files are served as static assets under
// <base>/docs/ (see the repoDocsPlugin in vite.config.ts).
const DOCS_BASE = `${import.meta.env.BASE_URL}docs/`

type DocNavItem = { title: string; slug: string }
type DocSection = { title: string; items: DocNavItem[] }
type TocEntry = { level: number; text: string; id: string }

// Parse a docsify-style _sidebar.md into grouped sections.
function parseSidebar(md: string): DocSection[] {
  const sections: DocSection[] = []
  let current: DocSection = { title: '', items: [] }

  for (const line of md.split('\n')) {
    const heading = line.match(/^##\s+(.+?)\s*$/)
    if (heading) {
      if (current.items.length > 0 || current.title) {
        sections.push(current)
      }
      current = { title: heading[1], items: [] }
      continue
    }
    const m = line.match(/^\s*[-*]\s*\[([^\]]+)\]\(([^)]+)\)/)
    if (!m) continue
    const title = m[1]
    const slug = m[2]
      .replace(/^\//, '')
      .replace(/^docs\//, '')
      .replace(/^\.\//, '')
      .replace(/\.md$/, '')
    if (!slug) continue
    current.items.push({ title, slug })
  }
  if (current.items.length > 0 || current.title) {
    sections.push(current)
  }
  return sections
}

function allItems(sections: DocSection[]): DocNavItem[] {
  return sections.flatMap((s) => s.items)
}

// Generate a GitHub-style slug from heading text.
function slugify(text: string): string {
  return text
    .toLowerCase()
    .replace(/[^\w\s-]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
}

// Extract ## and ### headings from markdown source for the in-page TOC.
// Uses marked.lexer() so that headings inside fenced code blocks are
// correctly excluded (marked's lexer understands code fences natively).
function parseHeadings(md: string): TocEntry[] {
  const entries: TocEntry[] = []
  const seen = new Set<string>()
  const tokens = marked.lexer(md)
  for (const token of tokens) {
    if (token.type !== 'heading') continue
    if (token.depth < 2 || token.depth > 3) continue
    const text = token.text.replace(/[`*_~]/g, '').trim()
    let id = slugify(text)
    // De-duplicate slugs.
    if (seen.has(id)) {
      let n = 2
      while (seen.has(`${id}-${n}`)) n++
      id = `${id}-${n}`
    }
    seen.add(id)
    entries.push({ level: token.depth, text, id })
  }
  return entries
}

// Rewrite relative .md links in markdown source to in-app /docs/<slug> routes.
function rewriteDocLinks(md: string): string {
  return md.replace(/\]\(([^)]+)\)/g, (match, href: string) => {
    // Leave external, mailto, and anchor-only links untouched
    if (/^(https?:|mailto:|#)/.test(href)) return match
    // Leave links pointing outside the docs/ directory (../…) untouched —
    // they reference repo files that have no in-app route.
    if (/^\.\.\//.test(href)) return match
    // Split off any in-page anchor (#fragment) so it survives the rewrite.
    const [path, anchor] = href.split('#')
    const stripped = path
      .replace(/^\.\//, '')
      .replace(/^docs\//, '')
      .replace(/^\//, '')
      .replace(/\.md$/, '')
    const route = `/docs/${stripped}`
    return `](${route}${anchor ? `#${anchor}` : ''})`
  })
}

export function DocsPage() {
  const splat = useParams()['*']
  const navigate = useNavigate()
  const [sections, setSections] = useState<DocSection[]>([])
  const [source, setSource] = useState('')
  const [loading, setLoading] = useState(true)
  const [notFound, setNotFound] = useState(false)
  const contentRef = useRef<HTMLDivElement>(null)

  // --- Search ---
  const [query, setQuery] = useState('')
  const [searchResults, setSearchResults] = useState<DocNavItem[]>([])
  const docCache = useRef<Map<string, string>>(new Map())
  const itemsRef = useRef<DocNavItem[]>([])

  const items = useMemo(() => allItems(sections), [sections])
  itemsRef.current = items

  // Default to the first sidebar entry when no topic is given (/docs).
  const slug = splat ?? items[0]?.slug

  useEffect(() => {
    let cancelled = false
    fetch(`${DOCS_BASE}_sidebar.md`)
      .then((r) => (r.ok ? r.text() : ''))
      .then((md) => {
        if (!cancelled) setSections(parseSidebar(md))
      })
      .catch(() => {
        if (!cancelled) setSections([])
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (!slug) return
    let cancelled = false
    setLoading(true)
    setNotFound(false)
    fetch(`${DOCS_BASE}${slug}.md`)
      .then((r) => {
        if (!r.ok) throw new Error(`docs: ${r.status}`)
        return r.text()
      })
      .then((md) => {
        if (cancelled) return
        setSource(md)
        setLoading(false)
      })
      .catch(() => {
        if (cancelled) return
        setNotFound(true)
        setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [slug])

  const renderedSource = useMemo(
    () => (loading || notFound ? '' : rewriteDocLinks(source)),
    [source, loading, notFound],
  )

  // Lazy-load all doc contents on first search, then filter.
  useEffect(() => {
    const q = query.trim().toLowerCase()
    if (!q) {
      setSearchResults([])
      return
    }
    let cancelled = false
    const cache = docCache.current
    const allItems = itemsRef.current
    const missing = allItems.filter((item) => !cache.has(item.slug))
    Promise.all(
      missing.map((item) =>
        fetch(`${DOCS_BASE}${item.slug}.md`)
          .then((r) => (r.ok ? r.text() : ''))
          .then((text) => {
            cache.set(item.slug, text)
            return { item, text }
          })
          .catch(() => ({ item, text: '' })),
      ),
    ).then(() => {
      if (cancelled) return
      const results = allItems.filter((item) => {
        const text = cache.get(item.slug) ?? ''
        return (
          item.title.toLowerCase().includes(q) ||
          text.toLowerCase().includes(q)
        )
      })
      setSearchResults(results)
    })
    return () => {
      cancelled = true
    }
  }, [query])

  const toc = useMemo(
    () => (loading || notFound ? [] : parseHeadings(renderedSource)),
    [renderedSource, loading, notFound],
  )

  const showToc = toc.length > 3

  // Add id attributes to rendered heading elements so TOC anchor links work.
  useEffect(() => {
    if (!contentRef.current || toc.length === 0) return
    const headings = contentRef.current.querySelectorAll('h2, h3')
    headings.forEach((h, i) => {
      if (toc[i]) h.id = toc[i].id
    })
  }, [toc, renderedSource])

  if (notFound) {
    return <Navigate to="/docs" replace />
  }

  const title = items.find((n) => n.slug === slug)?.title ?? slug

  // Prev/next navigation follows the sidebar order.
  const currentIndex = items.findIndex((n) => n.slug === slug)
  const prevItem = currentIndex > 0 ? items[currentIndex - 1] : undefined
  const nextItem = currentIndex >= 0 && currentIndex < items.length - 1
    ? items[currentIndex + 1]
    : undefined

  // Intercept clicks on internal doc links rendered inside the markdown body
  // so they navigate via React Router instead of triggering a full page load.
  const onContentClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const anchor = (e.target as HTMLElement).closest('a')
    if (!anchor) return
    const href = anchor.getAttribute('href') ?? ''
    // Internal doc links → SPA navigation
    if (href.startsWith('/docs/')) {
      e.preventDefault()
      navigate(href)
      return
    }
    // External http links → let the browser handle (opens in same tab)
    if (/^(https?:|mailto:)/.test(href)) return
    // Anything else (relative ../ paths pointing outside docs/) → block
    // navigation so the user doesn't leave the SPA on a dead URL.
    e.preventDefault()
  }

  const onTocClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const anchor = (e.target as HTMLElement).closest('a')
    if (!anchor) return
    const id = anchor.getAttribute('href')?.slice(1)
    if (!id) return
    e.preventDefault()
    const el = contentRef.current?.querySelector(`#${CSS.escape(id)}`)
    el?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  return (
    <div className="docs-shell">
      <Titleblock
        crumbs={
          <>
            Docs / <b>{title}</b>
          </>
        }
        title={
          <>
            Docs <em>{title}</em>
          </>
        }
      />
      <div className={`docs-page${showToc ? ' docs-page--with-toc' : ''}`}>
        <aside className="docs-sidebar">
          <input
            className="docs-search"
            type="search"
            placeholder="Search docs…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          {query.trim() ? (
            <nav className="docs-nav">
              {searchResults.length === 0 ? (
                <div className="docs-search-empty">No matches</div>
              ) : (
                searchResults.map((item) => (
                  <Link
                    key={item.slug}
                    to={`/docs/${item.slug}`}
                    className={item.slug === slug ? 'active' : ''}
                    onClick={() => setQuery('')}
                  >
                    {item.title}
                  </Link>
                ))
              )}
            </nav>
          ) : (
            <nav className="docs-nav">
              {sections.map((section) => (
                <div key={section.title || '_'} className="docs-nav-section">
                  {section.title && <div className="docs-nav-heading">{section.title}</div>}
                  {section.items.map((item) => (
                    <Link
                      key={item.slug}
                      to={`/docs/${item.slug}`}
                      className={[
                        item.slug === slug ? 'active' : '',
                        item.slug.includes('/') ? 'docs-nav-sub' : '',
                      ].filter(Boolean).join(' ')}
                    >
                      {item.title}
                    </Link>
                  ))}
                </div>
              ))}
            </nav>
          )}
        </aside>
        <main className="docs-main">
          {loading ? (
            <div className="md-body">Loading…</div>
          ) : (
            <div ref={contentRef} onClick={onContentClick} className="docs-content-col">
              <Markdown source={renderedSource} />
              {(prevItem || nextItem) && (
                <nav className="docs-prev-next">
                  {prevItem ? (
                    <Link to={`/docs/${prevItem.slug}`} className="docs-prev-next-link docs-prev">
                      <span className="docs-prev-next-label">← Previous</span>
                      <span className="docs-prev-next-title">{prevItem.title}</span>
                    </Link>
                  ) : <span />}
                  {nextItem ? (
                    <Link to={`/docs/${nextItem.slug}`} className="docs-prev-next-link docs-next">
                      <span className="docs-prev-next-label">Next →</span>
                      <span className="docs-prev-next-title">{nextItem.title}</span>
                    </Link>
                  ) : <span />}
                </nav>
              )}
            </div>
          )}
        </main>
        {showToc && (
          <aside className="docs-toc" onClick={onTocClick}>
            <div className="docs-toc-heading">Contents</div>
            {toc.map((entry) => (
              <a
                key={entry.id}
                href={`#${entry.id}`}
                className={`docs-toc-link docs-toc-l${entry.level}`}
              >
                {entry.text}
              </a>
            ))}
          </aside>
        )}
      </div>
    </div>
  )
}
