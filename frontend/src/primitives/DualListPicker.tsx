import { useMemo, useState, type ReactNode } from 'react'

export type DualListPickerProps<T> = {
  items: T[]
  selectedIds: string[]
  onChange: (ids: string[]) => void
  getId: (item: T) => string
  getLabel: (item: T) => string
  getDescription?: (item: T) => ReactNode
  getSearchText?: (item: T) => string
  isLoading?: boolean
  emptyLabel?: string
  availableTitle?: string
  enabledTitle?: string
}

type IndexedRow<T> = { id: string; item: T | null; missing: boolean }

function defaultSearchText<T>(
  item: T,
  getLabel: (i: T) => string,
  getDescription?: (i: T) => ReactNode,
): string {
  const label = getLabel(item)
  const desc = getDescription?.(item)
  return `${label} ${typeof desc === 'string' ? desc : ''}`.toLowerCase()
}

export function DualListPicker<T>(props: DualListPickerProps<T>) {
  const {
    items,
    selectedIds,
    onChange,
    getId,
    getLabel,
    getDescription,
    getSearchText,
    isLoading = false,
    emptyLabel = 'No items available.',
    availableTitle = 'Available',
    enabledTitle = 'Enabled',
  } = props

  const [availableQuery, setAvailableQuery] = useState('')
  const [enabledQuery, setEnabledQuery] = useState('')
  const [availableSel, setAvailableSel] = useState<Set<string>>(new Set())
  const [enabledSel, setEnabledSel] = useState<Set<string>>(new Set())
  const [availableAnchor, setAvailableAnchor] = useState<string | null>(null)
  const [enabledAnchor, setEnabledAnchor] = useState<string | null>(null)

  const selectedSet = useMemo(() => new Set(selectedIds), [selectedIds])

  const { available, enabled } = useMemo(() => {
    const byId = new Map(items.map((it) => [getId(it), it]))
    const enabledRows: IndexedRow<T>[] = selectedIds.map((id) => {
      const item = byId.get(id) ?? null
      return { id, item, missing: item === null }
    })
    const availableRows: IndexedRow<T>[] = items
      .filter((it) => !selectedSet.has(getId(it)))
      .map((it) => ({ id: getId(it), item: it, missing: false }))
    return { available: availableRows, enabled: enabledRows }
  }, [items, selectedIds, selectedSet, getId])

  const matches = (row: IndexedRow<T>, q: string) => {
    if (!q.trim()) return true
    if (!row.item) return row.id.toLowerCase().includes(q.toLowerCase())
    const text = getSearchText
      ? getSearchText(row.item).toLowerCase()
      : defaultSearchText(row.item, getLabel, getDescription)
    return text.includes(q.toLowerCase())
  }

  const visibleAvailable = available.filter((r) => matches(r, availableQuery))
  const visibleEnabled = enabled.filter((r) => matches(r, enabledQuery))

  const toggleInSet = (set: Set<string>, id: string): Set<string> => {
    const next = new Set(set)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    return next
  }

  const moveToEnabled = () => {
    if (availableSel.size === 0) return
    const additions = [...availableSel]
    onChange([...selectedIds, ...additions])
    setAvailableSel(new Set())
  }

  const moveToAvailable = () => {
    if (enabledSel.size === 0) return
    onChange(selectedIds.filter((id) => !enabledSel.has(id)))
    setEnabledSel(new Set())
  }

  const addAllVisibleAvailable = () => {
    if (visibleAvailable.length === 0) return
    const ids = visibleAvailable.map((r) => r.id)
    onChange([...selectedIds, ...ids])
    setAvailableSel(new Set())
  }

  const removeAllVisibleEnabled = () => {
    if (visibleEnabled.length === 0) return
    const ids = new Set(visibleEnabled.map((r) => r.id))
    onChange(selectedIds.filter((id) => !ids.has(id)))
    setEnabledSel(new Set())
  }

  const renderPane = (
    title: string,
    rows: IndexedRow<T>[],
    visible: IndexedRow<T>[],
    isEmpty: boolean,
    sel: Set<string>,
    setSel: (s: Set<string>) => void,
    query: string,
    setQuery: (q: string) => void,
    footer: { label: string; onClick: () => void; disabled: boolean },
    anchor: string | null,
    setAnchor: (id: string | null) => void,
  ) => (
    <div className="dual-list-pane">
      <header className="dual-list-pane-header">
        <span>
          {title} ({rows.length})
        </span>
      </header>
      <div className="dual-list-search">
        <input
          type="search"
          role="searchbox"
          aria-label={`Search ${title}`}
          placeholder={`Filter ${title.toLowerCase()}…`}
          disabled={isLoading}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>
      <ul
        role="listbox"
        aria-multiselectable="true"
        aria-label={title}
        className="dual-list-rows"
      >
        {isLoading ? (
          <li className="dual-list-placeholder">Loading…</li>
        ) : isEmpty && title === availableTitle ? (
          <li className="dual-list-placeholder">{emptyLabel}</li>
        ) : (
          visible.map((row) => {
            const selected = sel.has(row.id)
            return (
              <li
                key={row.id}
                role="option"
                aria-selected={selected}
                className={selected ? 'dual-list-row selected' : 'dual-list-row'}
                tabIndex={0}
                onClick={(e) => {
                  if (e.shiftKey && anchor) {
                    const ids = visible.map((r) => r.id)
                    const a = ids.indexOf(anchor)
                    const b = ids.indexOf(row.id)
                    if (a >= 0 && b >= 0) {
                      const [lo, hi] = a < b ? [a, b] : [b, a]
                      const range = new Set(ids.slice(lo, hi + 1))
                      setSel(new Set([...sel, ...range]))
                      return
                    }
                  }
                  setSel(toggleInSet(sel, row.id))
                  setAnchor(row.id)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    setSel(toggleInSet(sel, row.id))
                  }
                }}
              >
                <div className="dual-list-row-label">
                  {row.item ? getLabel(row.item) : row.id}
                  {row.missing ? <span className="dual-list-missing"> (missing)</span> : null}
                </div>
                {row.item && getDescription ? (
                  <div className="dual-list-row-desc">{getDescription(row.item)}</div>
                ) : null}
              </li>
            )
          })
        )}
      </ul>
      <div className="dual-list-pane-footer">
        <button
          type="button"
          onClick={footer.onClick}
          disabled={footer.disabled || isLoading}
        >
          {footer.label}
        </button>
      </div>
    </div>
  )

  const availableEmpty = !isLoading && available.length === 0
  const enabledEmpty = !isLoading && enabled.length === 0

  return (
    <div className="dual-list">
      {renderPane(
        availableTitle, available, visibleAvailable, availableEmpty,
        availableSel, setAvailableSel,
        availableQuery, setAvailableQuery,
        {
          label: `Add all visible (${visibleAvailable.length})`,
          onClick: addAllVisibleAvailable,
          disabled: visibleAvailable.length === 0,
        },
        availableAnchor, setAvailableAnchor,
      )}
      <div className="dual-list-arrows">
        <button
          type="button"
          aria-label={`Add selected to ${enabledTitle}`}
          disabled={availableSel.size === 0 || isLoading}
          onClick={moveToEnabled}
        >
          →
        </button>
        <button
          type="button"
          aria-label={`Remove selected from ${enabledTitle}`}
          disabled={enabledSel.size === 0 || isLoading}
          onClick={moveToAvailable}
        >
          ←
        </button>
      </div>
      {renderPane(
        enabledTitle, enabled, visibleEnabled, enabledEmpty,
        enabledSel, setEnabledSel,
        enabledQuery, setEnabledQuery,
        {
          label: `Remove all visible (${visibleEnabled.length})`,
          onClick: removeAllVisibleEnabled,
          disabled: visibleEnabled.length === 0,
        },
        enabledAnchor, setEnabledAnchor,
      )}
    </div>
  )
}
