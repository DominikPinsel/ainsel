import { useNavigate, useSearchParams } from 'react-router-dom'
import { useConnectors } from '../../api/connectors'
import type { ConnectorResponse } from '../../api/connectors'
import { Button } from '../../primitives/Button'
import { Dot } from '../../primitives/Dot'
import { Pager } from '../../primitives/Pager'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { Titleblock } from '../../layout/Titleblock'

const PAGE_SIZE_OPTIONS = [10, 20, 50] as const

export function ConnectorList() {
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const page = Math.max(1, Number(params.get('page')) || 1)
  const pageSizeParam = Number(params.get('pageSize'))
  const pageSize = (PAGE_SIZE_OPTIONS as readonly number[]).includes(pageSizeParam)
    ? pageSizeParam
    : 20

  const { data, isLoading, error } = useConnectors({ page, pageSize })

  const setPage = (p: number) => {
    const next = new URLSearchParams(params)
    next.set('page', String(p))
    setParams(next, { replace: true })
  }
  const setPageSize = (n: number) => {
    const next = new URLSearchParams(params)
    next.set('pageSize', String(n))
    next.set('page', '1')
    setParams(next, { replace: true })
  }

  const columns: readonly Column<ConnectorResponse>[] = [
    {
      key: 'name',
      header: 'Name',
      cell: (c) => (
        <b
          style={{ cursor: 'pointer', color: 'var(--accent)' }}
          onClick={() => navigate(`/connectors/${encodeURIComponent(c.id)}`)}
        >
          {c.name}
        </b>
      ),
    },
    {
      key: 'signatureHeader',
      header: 'Signature Header',
      cell: (c) => <span className="num">{c.signatureHeader}</span>,
    },
    {
      key: 'endpoint',
      header: 'Endpoint',
      cell: (c) => <span className="num">{c.webhookEndpoint ?? '—'}</span>,
    },
    {
      key: 'status',
      header: 'Status',
      width: 130,
      cell: (c) => {
        if (c.disabled) {
          return (
            <>
              <Dot state="stale" /> Disabled
            </>
          )
        }
        const ok = c.status?.ready === true
        return (
          <>
            <Dot state={ok ? 'ok' : 'err'} /> {ok ? 'Ready' : 'Failing'}
          </>
        )
      },
    },
    {
      key: 'actions',
      header: 'Actions',
      width: 80,
      align: 'right',
      cell: (c) => (
        <Button
          size="sm"
          onClick={() => navigate(`/connectors/${encodeURIComponent(c.id)}`)}
        >
          View
        </Button>
      ),
    },
  ]

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Fleet / <b>Connectors</b>
          </>
        }
        title={
          <>
            Connector <em>Catalogue</em>
          </>
        }
        actions={
          <Button variant="primary" onClick={() => navigate('/connectors/new')}>
            ＋ New Connector
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
              Failed to load connectors.
            </div>
          ) : data ? (
            <>
              <RegisterTable
                rows={data.items}
                columns={columns}
                rowKey={(c) => c.id}
                rowClassName={(c) =>
                  c.disabled ? 'row-stale' : c.status?.ready === false ? 'row-err' : undefined
                }
                emptyLabel="No connectors yet."
              />
              <Pager
                page={page}
                pageSize={pageSize}
                total={data.total}
                pageSizeOptions={PAGE_SIZE_OPTIONS}
                onPageChange={setPage}
                onPageSizeChange={setPageSize}
              />
            </>
          ) : null}
        </Panel>
      </div>
    </>
  )
}
