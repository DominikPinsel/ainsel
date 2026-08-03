import type { ToolFormValue } from './ImageDetail.types'

export type Source = 'all' | 'native' | string

type Props = {
  tools: ToolFormValue[]
  activeSource: Source
  onSourceChange: (source: Source) => void
  onRefresh: () => void
  isRefreshing: boolean
  canRefresh: boolean
}

type SourceEntry = {
  key: Source
  label: string
  total: number
  enabled: number
  hasNew: boolean
}

function computeSources(tools: ToolFormValue[]): SourceEntry[] {
  const nativeTools = tools.filter((t) => t.kind !== 'mcp')
  const mcpServers = new Map<string, ToolFormValue[]>()
  for (const t of tools) {
    if (t.kind === 'mcp' && t.mcpSource) {
      if (!mcpServers.has(t.mcpSource)) mcpServers.set(t.mcpSource, [])
      mcpServers.get(t.mcpSource)!.push(t)
    }
  }

  const entries: SourceEntry[] = [
    {
      key: 'all',
      label: 'All',
      total: tools.length,
      enabled: tools.filter((t) => t.enabled).length,
      hasNew: tools.some((t) => t.isNew),
    },
  ]

  if (nativeTools.length > 0) {
    entries.push({
      key: 'native',
      label: 'Native',
      total: nativeTools.length,
      enabled: nativeTools.filter((t) => t.enabled).length,
      hasNew: false,
    })
  }

  for (const [server, serverTools] of mcpServers) {
    entries.push({
      key: server,
      label: server,
      total: serverTools.length,
      enabled: serverTools.filter((t) => t.enabled).length,
      hasNew: serverTools.some((t) => t.isNew),
    })
  }

  return entries
}

export function ToolSourceSidebar({
  tools,
  activeSource,
  onSourceChange,
  onRefresh,
  isRefreshing,
  canRefresh,
}: Props) {
  const sources = computeSources(tools)

  return (
    <div className="md-sources">
      <header className="md-sources-header">
        <span>Sources</span>
      </header>
      <div className="md-sources-list">
        {sources.map((s, i) => (
          <div key={s.key}>
            {i > 0 && s.key !== 'all' && s.key !== 'native' && (sources[i - 1]?.key === 'native' || sources[i - 1]?.key === 'all') && (
              <div className="md-sources-sep" />
            )}
            <button
              type="button"
              className={s.key === activeSource ? 'src-item active' : 'src-item'}
              onClick={() => onSourceChange(s.key)}
              aria-label={`${s.label}${s.hasNew ? ', has new tools' : ''}, ${s.enabled} of ${s.total} enabled`}
              aria-pressed={s.key === activeSource}
            >
              <span className="src-label">{s.label}</span>
              <span className="src-count">
                {s.hasNew ? '★' : null}
                {s.enabled}/{s.total}
              </span>
            </button>
          </div>
        ))}
      </div>
      {canRefresh ? (
        <div className="md-sources-footer">
          <button
            type="button"
            className="btn btn-sm btn-primary"
            disabled={isRefreshing}
            onClick={onRefresh}
            aria-label="Refresh MCP"
            style={{ width: '100%' }}
          >
            {isRefreshing ? 'Refreshing…' : '↻ Refresh MCP'}
          </button>
        </div>
      ) : null}
    </div>
  )
}
