import { NavLink } from 'react-router-dom'
import { useConnectors } from '../../api/connectors'
import type { ConnectorResponse } from '../../api/connectors'
import { Dot } from '../../primitives/Dot'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'

const columns: readonly Column<ConnectorResponse>[] = [
  {
    key: 'name',
    header: 'Connector',
    cell: (c) => (
      <>
        <NavLink to={`/connectors/${c.id}`} style={{ color: 'var(--ink)', fontWeight: 600 }}>
          {c.name}
        </NavLink>
        <div className="label" style={{ marginTop: 2 }}>
          {c.signatureHeader}
        </div>
      </>
    ),
  },
  {
    key: 'endpoint',
    header: 'Endpoint',
    cell: (c) => (
      <span className="num" style={{ fontSize: 11 }}>
        {c.webhookEndpoint ?? '—'}
      </span>
    ),
  },
  {
    key: 'ready',
    header: 'Ready',
    width: 130,
    cell: (c) => {
      const ok = c.status?.ready === true
      return (
        <>
          <Dot state={ok ? 'ok' : 'err'} />{' '}
          <span style={{ color: ok ? undefined : 'var(--signal)', fontWeight: 600 }}>
            {ok ? 'Ready' : 'Failing'}
          </span>
        </>
      )
    },
  },
]

export function ConnectorRegister() {
  const { data, isLoading, error } = useConnectors({ pageSize: 200 })

  return (
    <Panel
      title={`Connector Register · ${String(data?.total ?? 0).padStart(2, '0')} entries`}
      className="cropped"
    >
      {isLoading ? <div className="label" style={{ padding: 14 }}>Loading…</div> : null}
      {error ? (
        <div className="label" style={{ padding: 14, color: 'var(--signal)' }}>
          Failed to load connectors.
        </div>
      ) : null}
      {!isLoading && !error && data ? (
        <RegisterTable
          rows={data.items}
          columns={columns}
          rowKey={(c) => c.id}
          rowClassName={(c) => (c.status?.ready === false ? 'row-err' : undefined)}
          emptyLabel="No connectors configured."
        />
      ) : null}
    </Panel>
  )
}
