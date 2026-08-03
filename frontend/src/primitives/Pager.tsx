import { Select } from './Select'

type PagerProps = {
  page: number
  pageSize: number
  total: number
  pageSizeOptions: readonly number[]
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
}

export function Pager({
  page,
  pageSize,
  total,
  pageSizeOptions,
  onPageChange,
  onPageSizeChange,
}: PagerProps) {
  const totalPages = total === 0 ? 0 : Math.ceil(total / pageSize)
  const firstRow = total === 0 ? 0 : (page - 1) * pageSize + 1
  const lastRow = total === 0 ? 0 : Math.min(page * pageSize, total)
  const pad2 = (n: number) => String(n).padStart(2, '0')

  return (
    <div className="pager">
      <span className="pager-info">
        <b>{pad2(firstRow)}</b>–<b>{pad2(lastRow)}</b> of <b>{total}</b> · rows per page
      </span>
      <Select
        size="sm"
        value={String(pageSize)}
        onChange={(v) => onPageSizeChange(Number(v))}
        options={pageSizeOptions.map((n) => ({ value: String(n), label: String(n) }))}
        aria-label="Rows per page"
      />
      <span style={{ width: 12 }} />
      <button
        className="pager-step"
        aria-label="Previous page"
        disabled={page <= 1}
        onClick={() => onPageChange(page - 1)}
      >
        ◀
      </button>
      {Array.from({ length: Math.max(totalPages, 1) }, (_, i) => i + 1).map((p) => (
        <button
          key={p}
          className={p === page ? 'pager-step active' : 'pager-step'}
          aria-label={`Page ${p}`}
          onClick={() => onPageChange(p)}
        >
          {p}
        </button>
      ))}
      <button
        className="pager-step"
        aria-label="Next page"
        disabled={page >= totalPages || totalPages === 0}
        onClick={() => onPageChange(page + 1)}
      >
        ▶
      </button>
    </div>
  )
}
