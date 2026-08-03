import { useState } from 'react'

type InlineEditCellProps = {
  value: string
  onCommit: (next: string) => Promise<void> | void
  ariaLabel?: string
  recentError?: boolean
}

export function InlineEditCell({
  value,
  onCommit,
  ariaLabel,
  recentError,
}: InlineEditCellProps) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(value)

  const startEditing = () => {
    setDraft(value)
    setEditing(true)
  }

  const commit = async () => {
    setEditing(false)
    if (draft !== value && draft.trim() !== '') {
      await onCommit(draft)
    }
  }

  const cancel = () => {
    setDraft(value)
    setEditing(false)
  }

  if (!editing) {
    return (
      <span
        className={recentError ? 'inline-edit error' : 'inline-edit'}
        role="button"
        tabIndex={0}
        aria-label={ariaLabel ?? `Edit ${value}`}
        onClick={startEditing}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            startEditing()
          }
        }}
      >
        {value}
      </span>
    )
  }

  return (
    <span className="inline-edit-form">
      <input
        autoFocus
        aria-label={ariaLabel ?? `Edit ${value}`}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            commit()
          } else if (e.key === 'Escape') {
            e.preventDefault()
            cancel()
          }
        }}
      />
    </span>
  )
}
