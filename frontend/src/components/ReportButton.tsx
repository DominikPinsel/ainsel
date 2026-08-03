import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useLocation } from 'react-router-dom'
import { toJpeg } from 'html-to-image'
import { useAuth } from '../auth/AuthProvider'
import { compressImage } from '../utils/compressImage'
import { runtimeConfig } from '../runtime-config'
import { getReportBtnEnabled, getReportScreenshotEnabled, PREF_CHANGE_EVENT } from '../prefs'
import { Button } from '../primitives/Button'
import { Textarea } from '../primitives/Textarea'
import './ReportButton.css'

type Step = 'compose' | null

function buildIssueTitle(pathname: string): string {
  const date = new Date().toISOString().split('T')[0]
  return `[Bug Report] ${pathname} — ${date}`
}

function buildIssueBody(opts: {
  description: string
  username: string
  email: string
  pathname: string
  href: string
  hasScreenshot: boolean
}): string {
  const { description, username, email, pathname, href, hasScreenshot } = opts
  const lines: string[] = [
    '## Description',
    '',
    description,
    '',
    '## Context',
    '',
    `- **Page:** \`${pathname}\``,
    `- **URL:** ${href}`,
    `- **User:** ${username} (${email})`,
    `- **Reported at:** ${new Date().toISOString()}`,
  ]
  if (hasScreenshot) {
    lines.push('', '## Screenshot', '', '_Paste screenshot here._')
  }
  return lines.join('\n')
}

function buildForgejoNewIssueUrl(title: string, body: string): string | null {
  const { forgejoApiBase, forgejoRepo } = runtimeConfig()
  if (!forgejoApiBase || !forgejoRepo) return null
  // forgejoApiBase ends with /api/v1 — strip it to get the web root
  const base = forgejoApiBase.replace(/\/api\/v1\/?$/, '')
  const params = new URLSearchParams({ title, body })
  return `${base}/${forgejoRepo}/issues/new?${params.toString()}`
}

const REPORT_UI_CLASSES = ['report-fab', 'modal-backdrop', 'report-toast']

async function captureScreenshot(): Promise<string | null> {
  try {
    return await toJpeg(document.body, {
      cacheBust: true,
      quality: 0.7,
      pixelRatio: 0.75,
      filter: (node) => {
        if (node instanceof HTMLElement) {
          return !REPORT_UI_CLASSES.some((cls) => node.classList.contains(cls))
        }
        return true
      },
    })
  } catch {
    return null
  }
}

/**
 * Trigger a browser download of a data-URL image. The file lands in the
 * user's default downloads folder so they can drag-and-drop it onto the
 * Forgejo issue form.
 */
function downloadScreenshot(dataUrl: string, filename: string): void {
  const a = document.createElement('a')
  a.href = dataUrl
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

const FOCUSABLE =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

function focusableWithin(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE))
}

function Modal({
  children,
  onClose,
  labelId,
}: {
  children: React.ReactNode
  onClose: () => void
  labelId: string
}) {
  const ref = useRef<HTMLDivElement | null>(null)
  const prevFocusRef = useRef<HTMLElement | null>(null)
  // Stable ref so the effect never needs to re-run when onClose identity changes.
  const onCloseRef = useRef(onClose)
  useEffect(() => { onCloseRef.current = onClose })

  useEffect(() => {
    prevFocusRef.current = document.activeElement as HTMLElement | null
    const root = ref.current
    if (root) {
      const els = focusableWithin(root)
      ;(els[0] ?? root).focus()
    }

    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        onCloseRef.current()
        return
      }
      if (e.key === 'Tab') {
        const r = ref.current
        if (!r) return
        const els = focusableWithin(r)
        if (els.length === 0) { e.preventDefault(); return }
        const first = els[0]
        const last = els[els.length - 1]
        const active = document.activeElement as HTMLElement | null
        if (e.shiftKey) {
          if (active === first || !r.contains(active)) { e.preventDefault(); last.focus() }
        } else if (active === last || !r.contains(active)) {
          e.preventDefault(); first.focus()
        }
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
      const prev = prevFocusRef.current
      if (prev && document.body.contains(prev)) prev.focus()
    }
  }, []) // stable — reads onClose through ref

  return createPortal(
    <div className="modal-backdrop" role="dialog" aria-modal="true" aria-labelledby={labelId} ref={ref}>
      <div className="modal-box">{children}</div>
    </div>,
    document.body,
  )
}

function Toast({ message, onDismiss }: { message: string; onDismiss: () => void }) {
  useEffect(() => {
    const timer = setTimeout(onDismiss, 5000)
    return () => clearTimeout(timer)
  }, [onDismiss])

  return createPortal(
    <div className="report-toast" role="alert">
      <span>{message}</span>
      <button className="report-toast-dismiss" onClick={onDismiss} aria-label="Dismiss notification">
        ×
      </button>
    </div>,
    document.body,
  )
}

export function ReportButton() {
  const [enabled, setEnabled] = useState(() => getReportBtnEnabled())
  const [screenshotEnabled, setScreenshotEnabled] = useState(() => getReportScreenshotEnabled())

  useEffect(() => {
    const handler = () => {
      setEnabled(getReportBtnEnabled())
      setScreenshotEnabled(getReportScreenshotEnabled())
    }
    window.addEventListener(PREF_CHANGE_EVENT, handler)
    return () => window.removeEventListener(PREF_CHANGE_EVENT, handler)
  }, [])
  const { user } = useAuth()
  const { pathname } = useLocation()

  const [step, setStep] = useState<Step>(null)
  const [description, setDescription] = useState('')
  const [screenshotDataUrl, setScreenshotDataUrl] = useState<string | null>(null)
  const [toast, setToast] = useState<string | null>(null)

  // Generation counter: incremented on every open/close to discard stale screenshot results.
  const screenshotGenRef = useRef(0)

  const dismissToast = useCallback(() => setToast(null), [])

  const handleClose = useCallback(() => {
    screenshotGenRef.current++
    setStep(null)
  }, [])

  if (!enabled) return null

  async function handleOpen() {
    const gen = ++screenshotGenRef.current
    setDescription('')
    const raw = screenshotEnabled ? await captureScreenshot() : null
    if (screenshotGenRef.current !== gen) return
    // Compress the screenshot so it is small enough to upload reliably.
    const compressed = raw ? await compressImage(raw) : null
    if (screenshotGenRef.current !== gen) return
    setScreenshotDataUrl(compressed ?? raw)
    setStep('compose')
  }

  function handlePreview() {
    const title = buildIssueTitle(pathname)
    const body = buildIssueBody({
      description,
      username: user?.username ?? 'unknown',
      email: user?.email ?? '',
      pathname,
      href: window.location.href,
      hasScreenshot: screenshotEnabled && screenshotDataUrl !== null,
    })
    const url = buildForgejoNewIssueUrl(title, body)
    if (!url) {
      alert('Forgejo is not configured — cannot open issue form.')
      return
    }
    // Open the window synchronously — must stay in the direct user-gesture
    // call stack, otherwise the browser popup blocker silently swallows it.
    window.open(url, '_blank', 'noopener,noreferrer')
    // Download the compressed screenshot so the user can attach it to the
    // newly opened issue. Download is 100% reliable (no clipboard permission
    // needed) and the file is small enough to drag-and-drop onto the form.
    if (screenshotDataUrl) {
      const date = new Date().toISOString().split('T')[0]
      downloadScreenshot(screenshotDataUrl, `screenshot-${date}.jpg`)
      setToast('Screenshot downloaded — please attach it to the issue.')
    }
    handleClose()
  }

  return (
    <>
      <button className="report-fab" onClick={handleOpen} aria-label="Report an error" title="Report an error">
        !
      </button>

      {toast && <Toast message={toast} onDismiss={dismissToast} />}

      {step === 'compose' && (
        <Modal onClose={handleClose} labelId="report-compose-title">
          <h3 id="report-compose-title">Report an Error</h3>
          <div className="modal-body" style={{ display: 'grid', gap: 16 }}>
            <label>
              <div className="label" style={{ marginBottom: 6 }}>Describe the error</div>
              <Textarea
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="What went wrong? What did you expect to happen?"
                rows={5}
                style={{ width: '100%' }}
                autoFocus
              />
            </label>
            {screenshotEnabled && screenshotDataUrl && (
              <div>
                <div className="label" style={{ marginBottom: 6 }}>Screenshot</div>
                <img src={screenshotDataUrl} alt="Page screenshot" className="report-screenshot-preview" />
              </div>
            )}
          </div>
          <div className="modal-actions">
            <Button onClick={handleClose}>Cancel</Button>
            <Button variant="primary" disabled={!description.trim()} onClick={handlePreview}>
              Preview
            </Button>
          </div>
        </Modal>
      )}

    </>
  )
}