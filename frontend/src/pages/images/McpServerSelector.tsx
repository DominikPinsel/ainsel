import { useMemo } from 'react'
import { useMCPServers } from '../../api/mcpServers'
import { DualListPicker } from '../../primitives/DualListPicker'
import { Button } from '../../primitives/Button'
import { Panel } from '../../primitives/Panel'

type McpServerSelectorProps = {
  mcpServers: string[]
  envNames: string[]
  onChange: (names: string[]) => void
  refresh?: {
    id: string
    onClick: () => Promise<void>
    isPending: boolean
    isSaving?: boolean
  }
}

export function McpServerSelector({ mcpServers, envNames, onChange, refresh }: McpServerSelectorProps) {
  const { data: available, isLoading } = useMCPServers()
  const envNameSet = useMemo(() => new Set(envNames), [envNames])
  const servers = available ?? []

  const describe = (s: (typeof servers)[number]) => {
    const tokenEnv = s.tokenFromEnv ?? ''
    if (tokenEnv === '') return null
    const missing = !envNameSet.has(tokenEnv)
    return (
      <span style={{ color: missing ? 'var(--signal)' : undefined }}>
        {missing ? `⚠ $${tokenEnv} not in env` : `reads token from $${tokenEnv}`}
      </span>
    )
  }

  return (
    <div style={{ marginTop: 24 }}>
      <Panel title="MCP Servers">
        <DualListPicker
          items={servers}
          selectedIds={mcpServers}
          onChange={onChange}
          getId={(s) => s.name}
          getLabel={(s) => s.displayName || s.name}
          getDescription={describe}
          getSearchText={(s) => `${s.displayName ?? ''} ${s.name} ${s.tokenFromEnv ?? ''}`}
          isLoading={isLoading}
          emptyLabel="No MCP servers configured."
          enabledTitle="Referenced by this image"
        />
        {refresh ? (
          <div style={{ marginTop: 12, display: 'flex', justifyContent: 'flex-end' }}>
            <Button
              type="button"
              variant="primary"
              disabled={refresh.isPending || refresh.isSaving}
              onClick={refresh.onClick}
            >
              {refresh.isSaving
                ? 'Saving…'
                : refresh.isPending
                  ? 'Refreshing…'
                  : 'Refresh MCP Tools'}
            </Button>
          </div>
        ) : null}
      </Panel>
    </div>
  )
}
