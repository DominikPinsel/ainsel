import { useEffect, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Button } from './Button'

type ConfirmModalProps = {
  open: boolean
  title: string
  body: ReactNode
  confirmLabel?: string
  cancelLabel?: string
  destructive?: boolean
  error?: string | null
  onConfirm: () => void
  onCancel: () => void
}

const FOCUSABLE_SELECTOR =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

function focusableWithin(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR))
}

export function ConfirmModal({
  open,
  title,
  body,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  destructive,
  error,
  onConfirm,
  onCancel,
}: ConfirmModalProps) {
  const dialogRef = useRef<HTMLDivElement | null>(null)
  const previouslyFocusedRef = useRef<HTMLElement | null>(null)

  useEffect(() => {
    if (!open) return
    // Remember what was focused before the modal opened.
    previouslyFocusedRef.current = document.activeElement as HTMLElement | null

    // Move focus into the dialog (first focusable, or the dialog itself).
    const root = dialogRef.current
    if (root) {
      const els = focusableWithin(root)
      const target = els[0] ?? root
      target.focus()
    }

    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        onCancel()
        return
      }
      if (e.key === 'Tab') {
        const r = dialogRef.current
        if (!r) return
        const els = focusableWithin(r)
        if (els.length === 0) {
          e.preventDefault()
          return
        }
        const first = els[0]
        const last = els[els.length - 1]
        const active = document.activeElement as HTMLElement | null
        if (e.shiftKey) {
          if (active === first || !r.contains(active)) {
            e.preventDefault()
            last.focus()
          }
        } else if (active === last || !r.contains(active)) {
          e.preventDefault()
          first.focus()
        }
      }
    }
    window.addEventListener('keydown', onKey)

    return () => {
      window.removeEventListener('keydown', onKey)
      // Restore focus to the trigger element when the modal closes.
      const prev = previouslyFocusedRef.current
      if (prev && document.body.contains(prev)) {
        prev.focus()
      }
    }
  }, [open, onCancel])

  if (!open) return null
  return createPortal(
    <div className="modal-backdrop" role="dialog" aria-modal="true" ref={dialogRef}>
      <div className="modal-box">
        <h3>{title}</h3>
        <div className="modal-body">{body}</div>
        {error ? (
          <p className="label" style={{ color: 'var(--signal)', margin: '0 0 8px' }}>
            {error}
          </p>
        ) : null}
        <div className="modal-actions">
          <Button onClick={onCancel}>{cancelLabel}</Button>
          <Button
            variant={destructive ? 'danger' : 'primary'}
            onClick={onConfirm}
          >
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  )
}
