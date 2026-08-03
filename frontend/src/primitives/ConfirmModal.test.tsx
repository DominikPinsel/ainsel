import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfirmModal } from './ConfirmModal'

describe('ConfirmModal', () => {
  it('renders nothing when closed', () => {
    render(
      <ConfirmModal
        open={false}
        title="Delete"
        body="x"
        onConfirm={() => {}}
        onCancel={() => {}}
      />,
    )
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('renders title and body when open', () => {
    render(
      <ConfirmModal
        open
        title="Delete agent?"
        body="doc-writer will be permanently removed."
        onConfirm={() => {}}
        onCancel={() => {}}
      />,
    )
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Delete agent?')).toBeInTheDocument()
    expect(screen.getByText('doc-writer will be permanently removed.')).toBeInTheDocument()
  })

  it('calls onConfirm when confirm button clicked', async () => {
    const onConfirm = vi.fn()
    render(
      <ConfirmModal
        open
        title="t"
        body="b"
        confirmLabel="Delete"
        destructive
        onConfirm={onConfirm}
        onCancel={() => {}}
      />,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(onConfirm).toHaveBeenCalled()
  })

  it('calls onCancel when cancel button clicked', async () => {
    const onCancel = vi.fn()
    render(
      <ConfirmModal
        open
        title="t"
        body="b"
        onConfirm={() => {}}
        onCancel={onCancel}
      />,
    )
    await userEvent.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onCancel).toHaveBeenCalled()
  })

  it('calls onCancel on Escape', async () => {
    const onCancel = vi.fn()
    render(
      <ConfirmModal open title="t" body="b" onConfirm={() => {}} onCancel={onCancel} />,
    )
    await userEvent.keyboard('{Escape}')
    expect(onCancel).toHaveBeenCalled()
  })

  it('moves focus into the dialog when it opens', () => {
    render(
      <ConfirmModal
        open
        title="t"
        body="b"
        cancelLabel="Cancel"
        confirmLabel="OK"
        onConfirm={() => {}}
        onCancel={() => {}}
      />,
    )
    expect(document.activeElement?.textContent).toBe('Cancel')
  })

  it('wraps Tab forward from the last focusable back to the first', async () => {
    render(
      <ConfirmModal
        open
        title="t"
        body="b"
        cancelLabel="Cancel"
        confirmLabel="OK"
        onConfirm={() => {}}
        onCancel={() => {}}
      />,
    )
    const cancel = screen.getByRole('button', { name: 'Cancel' })
    const ok = screen.getByRole('button', { name: 'OK' })
    cancel.focus()
    await userEvent.tab()
    expect(document.activeElement).toBe(ok)
    await userEvent.tab()
    expect(document.activeElement).toBe(cancel)
  })
})
