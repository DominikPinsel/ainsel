import { useUsers, useUpdateUser, useSyncUser, useSyncMe, userDisplayName, isUnsyncedUser } from '../../api/users'
import type { HubUser } from '../../api/users'
import { useCurrentUser } from '../../hooks/useCurrentUser'
import { Button } from '../../primitives/Button'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { Titleblock } from '../../layout/Titleblock'

export function UserList() {
  const { data, isLoading, error } = useUsers()
  const { data: currentUser } = useCurrentUser()
  const update = useUpdateUser()
  const sync = useSyncUser()
  const syncMe = useSyncMe()

  const toggleAdmin = (user: HubUser) => {
    update.mutate({ id: user.id, body: { isAdmin: !user.isAdmin } })
  }

  const columns: readonly Column<HubUser>[] = [
    {
      key: 'username',
      header: 'Username',
      cell: (u) =>
        isUnsyncedUser(u) ? (
          <span style={{ color: 'var(--muted)', fontStyle: 'italic' }} title={u.id}>
            Not synced yet
          </span>
        ) : (
          <b>{userDisplayName(u)}</b>
        ),
    },
    {
      key: 'email',
      header: 'Email',
      cell: (u) => <span className="num">{u.email}</span>,
    },
    {
      key: 'admin',
      header: 'Admin',
      width: 90,
      align: 'center',
      cell: (u) => (
        <input
          type="checkbox"
          checked={u.isAdmin}
          onChange={() => toggleAdmin(u)}
          aria-label={`Toggle admin for ${userDisplayName(u)}`}
          style={{ cursor: 'pointer', accentColor: 'var(--accent)' }}
        />
      ),
    },
    {
      key: 'sync',
      header: '',
      width: 100,
      align: 'center',
      cell: (u) => {
        const isOwnRow = currentUser != null && u.id === currentUser.id

        if (isOwnRow) {
          const isSyncing = syncMe.isPending
          return (
            <>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => syncMe.mutate()}
                disabled={isSyncing}
                aria-label={`Sync your profile`}
                title="Fetches your latest identity from the identity provider"
              >
                {isSyncing ? '…' : 'Sync'}
              </Button>
              {syncMe.isError && (
                <span style={{ color: 'var(--signal)', fontSize: '0.75em', display: 'block' }}>
                  Failed
                </span>
              )}
            </>
          )
        }

        const isSyncing = sync.isPending && sync.variables === u.id
        return (
          <>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => sync.mutate(u.id)}
              disabled={isSyncing}
              aria-label={`Clear cache for ${userDisplayName(u)}`}
              title="Clears cached data; name updates on the user's next login"
            >
              {isSyncing ? '…' : 'Clear cache'}
            </Button>
            {sync.isError && sync.variables === u.id && (
              <span style={{ color: 'var(--signal)', fontSize: '0.75em', display: 'block' }}>
                Failed
              </span>
            )}
          </>
        )
      },
    },
  ]

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Admin / <b>Users</b>
          </>
        }
        title={
          <>
            User <em>Registry</em>
          </>
        }
      />
      <div style={{ padding: '28px 32px' }}>
        <Panel className="cropped">
          {isLoading ? (
            <div className="label" style={{ padding: 14 }}>
              Loading…
            </div>
          ) : error ? (
            <div className="label" style={{ padding: 14, color: 'var(--signal)' }}>
              Failed to load users.
            </div>
          ) : data ? (
            <RegisterTable
              rows={data}
              columns={columns}
              rowKey={(u) => u.id}
              emptyLabel="No users found."
            />
          ) : null}
        </Panel>
      </div>
    </>
  )
}
