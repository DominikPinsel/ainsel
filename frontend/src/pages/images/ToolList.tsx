import { useState, useEffect } from 'react'
import type { ToolFormValue } from './ImageDetail.types'
import type { Source } from './ToolSourceSidebar'

const BULK_THRESHOLD = 15

type Props = {
  tools: ToolFormValue[]
  activeSource: Source
  selectedIndex: number | null
  onSelect: (index: number) => void
  onToggle: (index: number, enabled: boolean) => void
  onToggleAll: (indices: number[], enabled: boolean) => void
  onAdd: () => void
}

function shortName(tool: ToolFormValue): string {
  if (tool.kind !== 'mcp' || !tool.mcpSource) return tool.name
  const prefix = `mcp__${tool.mcpSource}__`
  return tool.name.startsWith(prefix) ? tool.name.slice(prefix.length) : tool.name
}

type IndexedTool = { t: ToolFormValue; i: number }

function filterTools(tools: ToolFormValue[], activeSource: Source, query: string): IndexedTool[] {
  const q = query.trim().toLowerCase()
  return tools
    .map((t, i) => ({ t, i }))
    .filter(({ t }) => {
      if (activeSource === 'all') return true
      if (activeSource === 'native') return t.kind !== 'mcp'
      return t.kind === 'mcp' && t.mcpSource === activeSource
    })
    .filter(({ t }) => q === '' || shortName(t).toLowerCase().includes(q))
}

function SectionHeader({ label, on, total }: { label: string; on: number; total: number }) {
  return (
    <div className="tool-section-head">
      <span>{label}</span>
      <span style={{ color: on < total ? 'var(--signal)' : undefined }}>
        {on}/{total}
      </span>
    </div>
  )
}

export function ToolList({ tools, activeSource, selectedIndex, onSelect, onToggle, onToggleAll, onAdd }: Props) {
  const [query, setQuery] = useState('')

  useEffect(() => {
    setQuery('')
  }, [activeSource])

  const displayed = filterTools(tools, activeSource, query)

  const sourceTools = tools
    .map((t, i) => ({ t, i }))
    .filter(({ t }) => {
      if (activeSource === 'all') return true
      if (activeSource === 'native') return t.kind !== 'mcp'
      return t.kind === 'mcp' && t.mcpSource === activeSource
    })

  const showBulk = activeSource !== 'all' && sourceTools.length > BULK_THRESHOLD

  const groups: { label: string; items: IndexedTool[] }[] = []
  if (activeSource === 'all') {
    const native = displayed.filter(({ t }) => t.kind !== 'mcp')
    if (native.length > 0) groups.push({ label: 'Native', items: native })
    const servers = new Map<string, IndexedTool[]>()
    for (const item of displayed) {
      if (item.t.kind === 'mcp' && item.t.mcpSource) {
        if (!servers.has(item.t.mcpSource)) servers.set(item.t.mcpSource, [])
        servers.get(item.t.mcpSource)!.push(item)
      }
    }
    for (const [server, items] of servers) {
      groups.push({ label: server, items })
    }
  } else {
    groups.push({ label: activeSource, items: displayed })
  }

  const enabledInSource = sourceTools.filter(({ t }) => t.enabled).length
  const headerLabel =
    activeSource === 'all'
      ? 'All Tools'
      : activeSource === 'native'
        ? 'Native'
        : activeSource

  return (
    <div className="md-left">
      <header className="md-bar">
        <h4>
          {headerLabel}
          <span style={{ fontWeight: 400, color: 'var(--ink-3)', marginLeft: 6 }}>
            · {enabledInSource}/{sourceTools.length} on
          </span>
        </h4>
        <div className="md-bar-grow" />
        {showBulk ? (
          <>
            <button
              type="button"
              className="tool-bulk-btn"
              onClick={() => onToggleAll(sourceTools.map(({ i }) => i), true)}
              aria-label="all on"
            >
              All on
            </button>
            <button
              type="button"
              className="tool-bulk-btn off"
              onClick={() => onToggleAll(sourceTools.map(({ i }) => i), false)}
              aria-label="all off"
            >
              All off
            </button>
          </>
        ) : null}
        {(activeSource === 'all' || activeSource === 'native') ? (
          <button type="button" className="btn btn-sm btn-primary" onClick={onAdd}>
            ＋ Add
          </button>
        ) : null}
      </header>

      <div className="tool-search">
        <input
          type="text"
          className="input"
          placeholder={`Filter ${headerLabel.toLowerCase()}…`}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      <div className="tool-list">
        {displayed.length === 0 ? (
          <div className="label" style={{ padding: 16, color: 'var(--ink-3)' }}>
            {query ? 'No tools match.' : 'No tools yet.'}
          </div>
        ) : (
          groups.map((group) => (
            <div key={group.label}>
              {activeSource === 'all' ? (
                <SectionHeader
                  label={group.label}
                  on={group.items.filter(({ t }) => t.enabled).length}
                  total={group.items.length}
                />
              ) : null}
              {group.items.map(({ t, i }) => {
                const name = shortName(t)
                return (
                  <div
                    key={i}
                    role="button"
                    tabIndex={0}
                    className={i === selectedIndex ? 'tool-row active' : `tool-row${!t.enabled ? ' tool-row-off' : ''}`}
                    onClick={() => onSelect(i)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        onSelect(i)
                      }
                    }}
                    aria-pressed={i === selectedIndex}
                    aria-label={`Select tool ${name}`}
                  >
                    <input
                      type="checkbox"
                      checked={t.enabled}
                      aria-label={`toggle ${name}`}
                      onChange={(e) => {
                        e.stopPropagation()
                        onToggle(i, e.target.checked)
                      }}
                      onClick={(e) => e.stopPropagation()}
                    />
                    <div style={{ minWidth: 0 }}>
                      <div className="tool-name">
                        {name}
                        {t.isNew ? <span className="tool-new-badge">new</span> : null}
                      </div>
                      <div className="tool-meta">
                        {t.kind}{!t.enabled ? ' · off' : ''}
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          ))
        )}
      </div>
    </div>
  )
}
