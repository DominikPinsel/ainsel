import { useNavigate, useSearchParams } from 'react-router-dom'
import { useAgentImages } from '../../api/agentImages'
import type { AgentImageSummary } from '../../api/agentImages'
import { Button } from '../../primitives/Button'
import { Pager } from '../../primitives/Pager'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { Titleblock } from '../../layout/Titleblock'

const PAGE_SIZE_OPTIONS = [10, 20, 50] as const

export function ImageList() {
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const page = Math.max(1, Number(params.get('page')) || 1)
  const pageSizeParam = Number(params.get('pageSize'))
  const pageSize = (PAGE_SIZE_OPTIONS as readonly number[]).includes(pageSizeParam)
    ? pageSizeParam
    : 20

  const { data, isLoading, error } = useAgentImages({ page, pageSize })

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

  const columns: readonly Column<AgentImageSummary>[] = [
    {
      key: 'name',
      header: 'Name',
      cell: (i) => (
        <b
          style={{ cursor: 'pointer', color: 'var(--accent)' }}
          onClick={() => navigate(`/agent-images/${encodeURIComponent(i.id)}`)}
        >
          {i.displayName ?? i.id}
        </b>
      ),
    },
    {
      key: 'image',
      header: 'Image URL',
      cell: (i) => <span className="num">{i.imageURL}</span>,
    },
    {
      key: 'tools',
      header: 'Tools',
      width: 80,
      align: 'right',
      cell: (i) => <span className="num">{i.toolCount ?? 0}</span>,
    },
    {
      key: 'skills',
      header: 'Skills',
      width: 80,
      align: 'right',
      cell: (i) => <span className="num">{i.enabledSkills?.length ?? 0}</span>,
    },
  ]

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Fleet / <b>Agent Images</b>
          </>
        }
        title={
          <>
            Agent <em>Images</em>
          </>
        }
        actions={
          <Button variant="primary" onClick={() => navigate('/agent-images/new')}>
            ＋ New Agent Image
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
              Failed to load agent images.
            </div>
          ) : data ? (
            <>
              <RegisterTable
                rows={data.items}
                columns={columns}
                rowKey={(i) => i.id}
                emptyLabel="No agent images yet."
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
