import { useNavigate, Link } from 'react-router-dom'
import { useMCPServers, type MCPServerSummary } from '../../api/mcpServers'
import { Button } from '../../primitives/Button'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { Titleblock } from '../../layout/Titleblock'
import { formatRelative } from '../../utils/time'

export function MCPServerList() {
  const navigate = useNavigate()
  const { data, isLoading, error } = useMCPServers()

  const columns: readonly Column<MCPServerSummary>[] = [
    {
      key: 'name',
      header: 'Name',
      cell: (m) => (
        <b
          style={{ cursor: 'pointer', color: 'var(--accent)' }}
          onClick={() => navigate(`/settings/mcp-servers/${encodeURIComponent(m.name)}`)}
        >
          {m.displayName || m.name}
        </b>
      ),
    },
    {
      key: 'url',
      header: 'URL',
      cell: (m) => (
        <span style={{ fontFamily: 'var(--mono)', fontSize: 12, color: 'var(--ink-3)' }}>
          {m.url}
        </span>
      ),
    },
    {
      key: 'tokenFromEnv',
      header: 'Token env',
      width: 160,
      cell: (m) =>
        m.tokenFromEnv ? (
          <span style={{ fontFamily: 'var(--mono)', fontSize: 12 }}>{m.tokenFromEnv}</span>
        ) : (
          <span style={{ color: 'var(--ink-3)' }}>—</span>
        ),
    },
    {
      key: 'updated',
      header: 'Updated',
      width: 130,
      align: 'right',
      cell: (m) => <span className="num">{formatRelative(m.updatedAt)}</span>,
    },
  ]

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Setup / <Link to="/settings">Settings</Link> / <b>MCP Servers</b>
          </>
        }
        title={
          <>
            MCP <em>Servers</em>
          </>
        }
        actions={
          <Button variant="primary" onClick={() => navigate('/settings/mcp-servers/new')}>
            ＋ New Server
          </Button>
        }
      />
      <div style={{ padding: '28px 32px' }}>
        <Panel className="cropped">
          {isLoading ? (
            <div className="label" style={{ padding: 14 }}>
              Loading…
            </div>
          ) : error ? (
            <div className="label" style={{ padding: 14, color: 'var(--signal)' }}>
              Failed to load MCP servers.
            </div>
          ) : (
            <RegisterTable
              rows={data ?? []}
              columns={columns}
              rowKey={(m) => m.name}
              rowClassName={() => 'clickable'}
              emptyLabel="No MCP servers registered. Add one to make its tools available to agent images."
            />
          )}
        </Panel>
      </div>
    </>
  )
}
