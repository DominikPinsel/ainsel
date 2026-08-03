import type { CSSProperties, ReactNode } from 'react'

export type Column<T> = {
  key: string
  header: ReactNode
  cell: (row: T) => ReactNode
  width?: string | number
  align?: 'left' | 'right' | 'center'
  sortable?: boolean
}

export type SortState = {
  orderBy: string
  orderDir: 'asc' | 'desc'
}

type RegisterTableProps<T> = {
  rows: T[]
  columns: readonly Column<T>[]
  rowKey: (row: T) => string
  emptyLabel?: ReactNode
  rowClassName?: (row: T) => string | undefined
  withRowNumbers?: boolean
  sort?: SortState
  onSort?: (columnKey: string) => void
}

const sortIndicatorStyle: CSSProperties = {
  marginLeft: 4,
  fontSize: '0.75em',
  opacity: 0.7,
}

const sortableHeaderStyle: CSSProperties = {
  cursor: 'pointer',
  userSelect: 'none',
}

export function RegisterTable<T>({
  rows,
  columns,
  rowKey,
  emptyLabel,
  rowClassName,
  withRowNumbers = true,
  sort,
  onSort,
}: RegisterTableProps<T>) {
  if (rows.length === 0 && emptyLabel) {
    return (
      <div className="label" style={{ padding: '14px' }}>
        {emptyLabel}
      </div>
    )
  }

  const renderHeader = (col: Column<T>) => {
    const isSortable = col.sortable === true && onSort != null
    const isActive = sort?.orderBy === col.key

    if (!isSortable) {
      return col.header
    }

    let indicator = ''
    if (isActive) {
      indicator = sort?.orderDir === 'desc' ? '▼' : '▲'
    }

    return (
      <span
        style={sortableHeaderStyle}
        onClick={() => onSort(col.key)}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onSort(col.key)
          }
        }}
      >
        {col.header}
        {indicator && <span style={sortIndicatorStyle}>{indicator}</span>}
      </span>
    )
  }

  return (
    <div className="reg-wrap">
      <table className="reg">
        <thead>
          <tr>
            {withRowNumbers ? <th className="rowno">#</th> : null}
            {columns.map((col) => (
              <th
                key={col.key}
                style={{
                  width: col.width,
                  textAlign: col.align,
                }}
              >
                {renderHeader(col)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, idx) => (
            <tr key={rowKey(row)} className={rowClassName?.(row)}>
              {withRowNumbers ? (
                <td className="rowno num">{String(idx + 1).padStart(2, '0')}</td>
              ) : null}
              {columns.map((col) => (
                <td key={col.key} style={{ textAlign: col.align }}>
                  {col.cell(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
