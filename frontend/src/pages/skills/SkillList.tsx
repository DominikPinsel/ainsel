import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useSkills, type SkillSummary } from '../../api/skills'
import { Button } from '../../primitives/Button'
import { Input } from '../../primitives/Input'
import { Pager } from '../../primitives/Pager'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column, type SortState } from '../../primitives/RegisterTable'
import { Tag } from '../../primitives/Tag'
import { Titleblock } from '../../layout/Titleblock'
import { formatRelative } from '../../utils/time'

const PAGE_SIZE_OPTIONS = [10, 20, 50] as const

// Maps column keys to the API orderBy values.
const SORTABLE_COLUMNS: Record<string, string> = {
  name: 'name',
  id: 'id',
  usedBy: 'usedBy',
  updated: 'updatedAt',
}

// Reverse lookup: API orderBy value → column key.
const API_TO_COLUMN_KEY: Record<string, string> = Object.fromEntries(
  Object.entries(SORTABLE_COLUMNS).map(([k, v]) => [v, k]),
)

// Default direction when a column is first clicked.
const DEFAULT_DIRECTIONS: Record<string, 'asc' | 'desc'> = {
  name: 'asc',
  id: 'asc',
  usedBy: 'desc',
  updated: 'desc',
}

export function SkillList() {
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const page = Math.max(1, Number(params.get('page')) || 1)
  const pageSizeParam = Number(params.get('pageSize'))
  const pageSize = (PAGE_SIZE_OPTIONS as readonly number[]).includes(pageSizeParam)
    ? pageSizeParam
    : 20
  const searchParam = params.get('search') ?? ''
  const tagsParam = params.get('tags') ?? ''
  const orderByParam = params.get('orderBy') ?? ''
  const orderDirParam = params.get('orderDir') ?? ''

  const [searchInput, setSearchInput] = useState(searchParam)

  // Sync local input when URL changes externally (e.g. back/forward)
  useEffect(() => {
    setSearchInput(searchParam)
  }, [searchParam])

  // Debounce search input → URL param
  useEffect(() => {
    const timer = setTimeout(() => {
      if (searchInput !== searchParam) {
        const next = new URLSearchParams(params)
        if (searchInput) {
          next.set('search', searchInput)
        } else {
          next.delete('search')
        }
        next.set('page', '1')
        setParams(next, { replace: true })
      }
    }, 300)
    return () => clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchInput])

  const activeTags = tagsParam ? tagsParam.split(',').filter(Boolean) : []

  const sortState: SortState | undefined =
    orderByParam && API_TO_COLUMN_KEY[orderByParam]
      ? { orderBy: API_TO_COLUMN_KEY[orderByParam], orderDir: orderDirParam === 'desc' ? 'desc' : 'asc' }
      : undefined

  const { data, isLoading, error } = useSkills({
    page,
    pageSize,
    search: searchParam || undefined,
    tags: tagsParam || undefined,
    orderBy: sortState ? SORTABLE_COLUMNS[sortState.orderBy] : undefined,
    orderDir: sortState?.orderDir,
  })

  // Collect all unique tags from current results for filter chips
  const availableTags = data
    ? [...new Set(data.items.flatMap((s) => s.tags ?? []))].sort()
    : []

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

  const toggleTag = (tag: string) => {
    const next = new URLSearchParams(params)
    const current = activeTags.includes(tag)
      ? activeTags.filter((t) => t !== tag)
      : [...activeTags, tag]
    if (current.length > 0) {
      next.set('tags', current.join(','))
    } else {
      next.delete('tags')
    }
    next.set('page', '1')
    setParams(next, { replace: true })
  }

  const handleSort = (columnKey: string) => {
    const next = new URLSearchParams(params)
    const apiColumn = SORTABLE_COLUMNS[columnKey]
    if (!apiColumn) return

    const currentOrderBy = next.get('orderBy')
    const currentOrderDir = next.get('orderDir')

    if (currentOrderBy === apiColumn) {
      // Toggle direction.
      const newDir = currentOrderDir === 'desc' ? 'asc' : 'desc'
      next.set('orderDir', newDir)
    } else {
      // New column: set orderBy and use default direction.
      next.set('orderBy', apiColumn)
      next.set('orderDir', DEFAULT_DIRECTIONS[columnKey] || 'asc')
    }
    next.set('page', '1')
    setParams(next, { replace: true })
  }

  const columns: readonly Column<SkillSummary>[] = [
    {
      key: 'name',
      header: 'Name',
      sortable: true,
      cell: (s) => (
        <b
          style={{ cursor: 'pointer', color: 'var(--accent)' }}
          onClick={() => navigate(`/skills/${encodeURIComponent(s.id)}`)}
        >
          {s.name}
        </b>
      ),
    },
    {
      key: 'id',
      header: 'ID',
      width: 180,
      sortable: true,
      cell: (s) => <span className="num">{s.id}</span>,
    },
    {
      key: 'description',
      header: 'Description',
      cell: (s) => <span style={{ color: 'var(--ink-3)' }}>{s.description || '—'}</span>,
    },
    {
      key: 'tags',
      header: 'Tags',
      width: 200,
      cell: (s) =>
        s.tags && s.tags.length > 0 ? (
          <span style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
            {s.tags.map((t) => (
              <Tag key={t}>{t}</Tag>
            ))}
          </span>
        ) : (
          <span style={{ color: 'var(--ink-3)' }}>—</span>
        ),
    },
    {
      key: 'usedBy',
      header: 'Used by',
      width: 90,
      align: 'right',
      sortable: true,
      cell: (s) =>
        s.usedBy ? (
          <span className="num">{s.usedBy}</span>
        ) : (
          <span className="num" style={{ color: 'var(--ink-3)' }}>—</span>
        ),
    },
    {
      key: 'updated',
      header: 'Updated',
      width: 130,
      align: 'right',
      sortable: true,
      cell: (s) => <span className="num">{formatRelative(s.updatedAt)}</span>,
    },
    {
      key: 'actions',
      header: 'Actions',
      width: 80,
      align: 'right',
      cell: (s) => (
        <Button size="sm" onClick={() => navigate(`/skills/${encodeURIComponent(s.id)}`)}>
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
            Fleet / <b>Skills</b>
          </>
        }
        title={
          <>
            Skill <em>Library</em>
          </>
        }
        actions={
          <Button variant="primary" onClick={() => navigate('/skills/new')}>
            ＋ New Skill
          </Button>
        }
      />
      <div style={{ padding: '28px 32px' }}>
        <div style={{ display: 'flex', gap: 12, marginBottom: 16, alignItems: 'center', flexWrap: 'wrap' }}>
          <Input
            placeholder="Search skills…"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            style={{ maxWidth: 320 }}
            aria-label="Search skills"
          />
          {availableTags.length > 0 && (
            <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', alignItems: 'center' }}>
              {availableTags.map((t) => (
                <Tag
                  key={t}
                  variant={activeTags.includes(t) ? 'ok' : 'default'}
                  solid={activeTags.includes(t)}
                  className="clickable"
                >
                  <span
                    style={{ cursor: 'pointer' }}
                    onClick={() => toggleTag(t)}
                    role="button"
                    tabIndex={0}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault()
                        toggleTag(t)
                      }
                    }}
                  >
                    {t}
                  </span>
                </Tag>
              ))}
            </div>
          )}
        </div>
        <Panel className="cropped">
          {isLoading ? (
            <div className="label" style={{ padding: 14 }}>
              Loading…
            </div>
          ) : error ? (
            <div className="label" style={{ padding: 14, color: 'var(--signal)' }}>
              Failed to load skills.
            </div>
          ) : data ? (
            <>
              <RegisterTable
                rows={data.items}
                columns={columns}
                rowKey={(s) => s.id}
                rowClassName={() => 'clickable'}
                sort={sortState}
                onSort={handleSort}
                emptyLabel={
                  searchParam || tagsParam
                    ? 'No skills match the current filters.'
                    : 'No skills yet. Create one to define reusable agent capabilities.'
                }
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
