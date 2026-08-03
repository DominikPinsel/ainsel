import { useNavigate, useSearchParams } from 'react-router-dom'
import { useAgents } from '../../api/agents'
import { usePersonas, type PersonaSummary } from '../../api/personas'
import { Button } from '../../primitives/Button'
import { Pager } from '../../primitives/Pager'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { Tag } from '../../primitives/Tag'
import { Titleblock } from '../../layout/Titleblock'
import { formatRelative } from '../../utils/time'

const PAGE_SIZE_OPTIONS = [10, 20, 50] as const

export function PersonaList() {
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const page = Math.max(1, Number(params.get('page')) || 1)
  const pageSizeParam = Number(params.get('pageSize'))
  const pageSize = (PAGE_SIZE_OPTIONS as readonly number[]).includes(pageSizeParam)
    ? pageSizeParam
    : 20

  const { data, isLoading, error } = usePersonas({ page, pageSize })
  // Used-by computed from agents — N+M scan; acceptable at scale (<= 100 agents).
  const { data: agents } = useAgents({ pageSize: 100 })
  const usedByCount = (personaId: string) =>
    (agents?.items ?? []).filter((a) => a.persona?.id === personaId).length

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

  const columns: readonly Column<PersonaSummary>[] = [
    {
      key: 'name',
      header: 'Name',
      cell: (p) => (
        <b
          style={{ cursor: 'pointer', color: 'var(--accent)' }}
          onClick={() => navigate(`/personas/${encodeURIComponent(p.id)}`)}
        >
          {p.name}
        </b>
      ),
    },
    {
      key: 'description',
      header: 'Description',
      cell: (p) => (
        <span style={{ color: 'var(--ink-3)' }}>{p.description || '—'}</span>
      ),
    },
    {
      key: 'version',
      header: 'Version',
      width: 90,
      cell: (p) => <Tag>{`v${p.currentVersion}`}</Tag>,
    },
    {
      key: 'updated',
      header: 'Updated',
      width: 130,
      align: 'right',
      cell: (p) => <span className="num">{formatRelative(p.updatedAt)}</span>,
    },
    {
      key: 'usedBy',
      header: 'Used by',
      width: 110,
      align: 'right',
      cell: (p) => <span className="num">{usedByCount(p.id)} agents</span>,
    },
  ]

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Fleet / <b>Personas</b>
          </>
        }
        title={
          <>
            Persona <em>Library</em>
          </>
        }
        actions={
          <Button variant="primary" onClick={() => navigate('/personas/new')}>
            ＋ New Persona
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
              Failed to load personas.
            </div>
          ) : data ? (
            <>
              <RegisterTable
                rows={data.items}
                columns={columns}
                rowKey={(p) => p.id}
                rowClassName={() => 'clickable'}
                emptyLabel="No personas yet. Create one to define agent behavior."
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
