import { Outlet } from 'react-router-dom'
import { Titleblock } from '../../layout/Titleblock'

export function SettingsLayout() {
  return (
    <>
      <Titleblock
        crumbs={
          <>
            Library / <b>MCP Servers</b>
          </>
        }
        title={
          <>
            Settings <em>Console</em>
          </>
        }
      />
      <div style={{ padding: '28px 32px' }}>
        <main>
          <Outlet />
        </main>
      </div>
    </>
  )
}
