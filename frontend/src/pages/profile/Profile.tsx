import { useCallback, useState } from 'react'
import { useAuth } from '../../auth/AuthProvider'
import { useCurrentUser } from '../../hooks/useCurrentUser'
import { userDisplayName, useSyncMe } from '../../api/users'
import { Titleblock } from '../../layout/Titleblock'
import { Button } from '../../primitives/Button'
import { Check } from '../../primitives/Check'
import { Panel } from '../../primitives/Panel'
import { Select } from '../../primitives/Select'
import { getStoredTheme, setStoredTheme, type Theme } from '../../theme'
import {
  getReportBtnEnabled,
  setReportBtnEnabled,
  getReportScreenshotEnabled,
  setReportScreenshotEnabled,
} from '../../prefs'
import { TokenManager } from './TokenManager'

const THEME_OPTIONS: { value: Theme; label: string }[] = [
  { value: 'light', label: 'Light' },
  { value: 'dark', label: 'Dark' },
  { value: 'peat', label: 'Peat' },
  { value: 'tallow', label: 'Tallow' },
]

export function Profile() {
  const { signoutRedirect } = useAuth()
  const { data: user, isLoading } = useCurrentUser()
  const [theme, setTheme] = useState<Theme>(() => getStoredTheme() ?? 'light')
  const [reportBtn, setReportBtn] = useState(() => getReportBtnEnabled())
  const [reportScreenshot, setReportScreenshot] = useState(() => getReportScreenshotEnabled())
  const syncMe = useSyncMe()

  const handleThemeChange = useCallback((next: string) => {
    const t = next as Theme
    setTheme(t)
    setStoredTheme(t)
  }, [])

  const handleReportBtnChange = useCallback((next: boolean) => {
    setReportBtn(next)
    setReportBtnEnabled(next)
  }, [])

  const handleReportScreenshotChange = useCallback((next: boolean) => {
    setReportScreenshot(next)
    setReportScreenshotEnabled(next)
  }, [])

  return (
    <>
      <Titleblock
        crumbs={<b>Profile</b>}
        title={
          <>
            Your <em>Profile</em>
          </>
        }
      />
      <div style={{ padding: '28px 32px', display: 'grid', gap: 24, maxWidth: 700 }}>
        <Panel title="Account">
          {isLoading ? (
            <div className="label" style={{ padding: 14 }}>
              Loading…
            </div>
          ) : user ? (
            <div style={{ padding: '14px 16px', display: 'grid', gap: 12 }}>
              <Row
                label="Username"
                value={userDisplayName(user)}
                action={
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => syncMe.mutate()}
                    disabled={syncMe.isPending}
                    aria-label="Sync username from Zitadel"
                  >
                    {syncMe.isPending ? '…' : 'Sync'}
                  </Button>
                }
              />
              {syncMe.isError && (
                <div style={{ color: 'var(--signal)', fontSize: '0.85em', textAlign: 'right' }}>
                  Sync failed. Please try again.
                </div>
              )}
              <Row label="Email" value={user.email || '—'} />
              <Row label="Role" value={user.isAdmin ? 'Admin' : 'Member'} />
              <Row label="Member since" value={new Date(user.createdAt).toLocaleDateString()} />
            </div>
          ) : null}
        </Panel>

        <Panel title="Appearance">
          <div style={{ padding: '14px 16px' }}>
            <div style={{ display: 'flex', alignItems: 'baseline', gap: 12 }}>
              <span className="label">Theme</span>
              <Select
                id="theme-select"
                value={theme}
                onChange={handleThemeChange}
                options={THEME_OPTIONS}
                aria-label="Theme"
              />
            </div>
          </div>
        </Panel>

        <TokenManager />

        <Panel title="Developer">
          <div style={{ padding: '14px 16px', display: 'grid', gap: 14 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
              <Check
                checked={reportBtn}
                onChange={handleReportBtnChange}
                aria-label="Enable report error button"
              />
              <span className="label">Show report error button</span>
            </div>
            {reportBtn && (
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <Check
                  checked={reportScreenshot}
                  onChange={handleReportScreenshotChange}
                  aria-label="Include screenshot in error reports"
                />
                <span className="label">Include screenshot in reports</span>
              </div>
            )}
          </div>
        </Panel>

        <Panel title="Session">
          <div style={{ padding: '14px 16px' }}>
            <Button variant="danger" onClick={() => signoutRedirect()}>
              Log out
            </Button>
          </div>
        </Panel>
      </div>
    </>
  )
}

function Row({
  label,
  value,
  action,
}: {
  label: string
  value: string
  action?: React.ReactNode
}) {
  return (
    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
      <span className="label">{label}</span>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
        <b>{value}</b>
        {action}
      </div>
    </div>
  )
}
