import { Select } from './Select'

// pageItems computes the page buttons to render. For short page ranges all
// pages are shown; for long ranges the list collapses to first, a window
// around the current page, and last, separated by gap markers.
function pageItems(page: number, totalPages: number): (number | 'gap-left' | 'gap-right')[] {
  if (totalPages <= 9) {
    return Array.from({ length: totalPages }, (_, i) => i + 1)
  }
  const items: (number | 'gap-left' | 'gap-right')[] = [1]
  const start = Math.max(2, page - 1)
  const end = Math.min(totalPages - 1, page + 1)
  if (start > 2) items.push('gap-left')
  for (let p = start; p <= end; p++) items.push(p)
  if (end < totalPages - 1) items.push('gap-right')
  items.push(totalPages)
  return items
}

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
      {pageItems(page, totalPages).map((item) =>
        item === 'gap-left' || item === 'gap-right' ? (
          <span key={item} className="pager-gap" aria-hidden="true">
            …
          </span>
        ) : (
          <button
            key={item}
            className={item === page ? 'pager-step active' : 'pager-step'}
            aria-label={`Page ${item}`}
            onClick={() => onPageChange(item)}
          >
            {item}
          </button>
        ),
      )}
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
