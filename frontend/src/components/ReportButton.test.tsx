import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { toJpeg } from 'html-to-image'
import { ReportButton } from './ReportButton'
import { PREF_CHANGE_EVENT } from '../prefs'

vi.mock('../auth/AuthProvider', () => ({
  useAuth: vi.fn(() => ({
    token: 'test-token',
    user: { username: 'testuser', email: 'test@example.com' },
    ready: true,
    signinRedirect: vi.fn(),
    signoutRedirect: vi.fn(),
  })),
}))

vi.mock('html-to-image', () => ({
  toJpeg: vi.fn(),
}))

vi.mock('../utils/compressImage', () => ({
  compressImage: vi.fn((dataUrl: string) => Promise.resolve(dataUrl)),
}))

describe('ReportButton', () => {
  let windowOpen: ReturnType<typeof vi.fn>
  const jpegDataUrl = 'data:image/jpeg;base64,/9j/4AAAQSkZJRgABAQEASABIAAD'

  beforeEach(() => {
    windowOpen = vi.fn()
    // vitest 4's Mock type no longer structurally matches window.open
    window.open = windowOpen as unknown as typeof window.open

    window.__AINSEL_CONFIG__ = {
      oidcIssuer: 'https://auth.example.com',
      oidcClientId: 'test-client',
      oidcProjectId: 'test-project',
      forgejoApiBase: 'https://forgejo.example.com/api/v1',
      forgejoRepo: 'owner/repo',
    }

    localStorage.setItem('ainsel-report-btn', 'true')
    localStorage.setItem('ainsel-report-screenshot', 'true')

    window.dispatchEvent(new Event(PREF_CHANGE_EVENT))
  })

  afterEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
    delete (window as unknown as Record<string, unknown>).__AINSEL_CONFIG__
  })

  async function openModal() {
    render(
      <MemoryRouter initialEntries={['/dashboard']}>
        <ReportButton />
      </MemoryRouter>,
    )
    await userEvent.click(screen.getByRole('button', { name: /Report an error/i }))
    await waitFor(() => expect(screen.getByText('Report an Error')).toBeInTheDocument())
  }

  it('opens the issue form and downloads the screenshot', async () => {
    (toJpeg as ReturnType<typeof vi.fn>).mockResolvedValue(jpegDataUrl)
    await openModal()
    const textarea = screen.getByPlaceholderText(/What went wrong/i)
    await userEvent.type(textarea, 'something broke')

    await userEvent.click(screen.getByRole('button', { name: /Preview/i }))

    // window.open fires synchronously — popup blocker requires it in the
    // direct user-gesture call stack.
    expect(windowOpen).toHaveBeenCalledWith(
      expect.stringContaining('/owner/repo/issues/new'),
      '_blank',
      'noopener,noreferrer',
    )
    // Toast tells user to attach the downloaded screenshot.
    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument())
    expect(screen.getByRole('alert')).toHaveTextContent(/Screenshot downloaded/i)
  })

  it('does not download when there is no screenshot', async () => {
    (toJpeg as ReturnType<typeof vi.fn>).mockResolvedValue(null)
    await openModal()
    const textarea = screen.getByPlaceholderText(/What went wrong/i)
    await userEvent.type(textarea, 'something broke')

    await userEvent.click(screen.getByRole('button', { name: /Preview/i }))

    await waitFor(() => expect(windowOpen).toHaveBeenCalled())
    // No toast about screenshot download.
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('shows a toast when screenshot is downloaded', async () => {
    (toJpeg as ReturnType<typeof vi.fn>).mockResolvedValue(jpegDataUrl)
    await openModal()
    const textarea = screen.getByPlaceholderText(/What went wrong/i)
    await userEvent.type(textarea, 'something broke')

    await userEvent.click(screen.getByRole('button', { name: /Preview/i }))

    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument())
    expect(screen.getByRole('alert')).toHaveTextContent(/attach it to the issue/i)
    // window.open fires synchronously before the download.
    expect(windowOpen).toHaveBeenCalled()
  })
})
