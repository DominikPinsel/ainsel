import { useState, type FormEvent } from 'react'
import {
  useUsers,
  useUpdateUser,
  useSyncUser,
  useSyncMe,
  useCreateUser,
  useDeleteUser,
  useResetUserPassword,
  userDisplayName,
  isUnsyncedUser,
} from '../../api/users'
import type { HubUser } from '../../api/users'
import { useCurrentUser } from '../../hooks/useCurrentUser'
import { useAuth } from '../../auth/AuthProvider'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { Titleblock } from '../../layout/Titleblock'

const LOCAL_PREFIX = 'local:'

function isLocalUser(u: HubUser): boolean {
  return u.id.startsWith(LOCAL_PREFIX)
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : 'Request failed'
}

export function UserList() {
  const { data, isLoading, error } = useUsers()
  const { data: currentUser } = useCurrentUser()
  const { mode } = useAuth()
  const update = useUpdateUser()
  const sync = useSyncUser()
  const syncMe = useSyncMe()
  const create = useCreateUser()
  const remove = useDeleteUser()
  const resetPw = useResetUserPassword()

  const isAdmin = currentUser?.isAdmin === true

  // --- create form state ---
  const [showCreate, setShowCreate] = useState(false)
  const [newUsername, setNewUsername] = useState('')
  const [newEmail, setNewEmail] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [newIsAdmin, setNewIsAdmin] = useState(false)

  // --- per-row action state ---
  const [deleteTarget, setDeleteTarget] = useState<HubUser | null>(null)
  const [resetTarget, setResetTarget] = useState<HubUser | null>(null)
  const [resetPassword, setResetPassword] = useState('')

  const toggleAdmin = (user: HubUser) => {
    update.mutate({ id: user.id, body: { isAdmin: !user.isAdmin } })
  }

  const submitCreate = (e: FormEvent) => {
    e.preventDefault()
    create.mutate(
      {
        username: newUsername.trim(),
        password: newPassword,
        email: newEmail.trim() || undefined,
        isAdmin: newIsAdmin,
      },
      {
        onSuccess: () => {
          setShowCreate(false)
          setNewUsername('')
          setNewEmail('')
          setNewPassword('')
          setNewIsAdmin(false)
        },
      },
    )
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
          <>
            <b>{userDisplayName(u)}</b>{' '}
            {isLocalUser(u) && (
              <span
                className="label"
                title="Local account (username/password)"
                style={{ fontSize: '0.7em', opacity: 0.7 }}
              >
                local
              </span>
            )}
          </>
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
          onChange={() => isAdmin && toggleAdmin(u)}
          disabled={!isAdmin}
          aria-label={`Toggle admin for ${userDisplayName(u)}`}
          style={{ cursor: isAdmin ? 'pointer' : 'default', accentColor: 'var(--accent)' }}
        />
      ),
    },
    ...(mode === 'oidc'
      ? [
          {
            key: 'sync',
            header: '',
            width: 100,
            align: 'center' as const,
            cell: (u: HubUser) => {
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
      : []),
    ...(isAdmin
      ? [
          {
            key: 'actions',
            header: '',
            width: 190,
            align: 'center' as const,
            cell: (u: HubUser) => {
              const isSelf = currentUser != null && u.id === currentUser.id
              return (
                <span style={{ display: 'inline-flex', gap: 6 }}>
                  {isLocalUser(u) && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => {
                        setResetTarget(u)
                        setResetPassword('')
                      }}
                      aria-label={`Reset password for ${userDisplayName(u)}`}
                    >
                      Reset password
                    </Button>
                  )}
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={isSelf}
                    title={isSelf ? 'You cannot delete your own account' : undefined}
                    onClick={() => setDeleteTarget(u)}
                    aria-label={`Delete ${userDisplayName(u)}`}
                  >
                    Delete
                  </Button>
                </span>
              )
            },
          },
        ]
      : []),
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
      <div style={{ padding: '28px 32px', display: 'grid', gap: 16 }}>
        {isAdmin && (
          <div>
            {!showCreate ? (
              <Button variant="primary" onClick={() => setShowCreate(true)}>
                New user
              </Button>
            ) : (
              <Panel title="New local user">
                <form
                  onSubmit={submitCreate}
                  style={{ padding: 16, display: 'grid', gap: 14, maxWidth: 480 }}
                >
                  <Field label="Username" htmlFor="new-username">
                    <Input
                      id="new-username"
                      value={newUsername}
                      onChange={(e) => setNewUsername(e.target.value)}
                      autoComplete="off"
                      required
                    />
                  </Field>
                  <Field label="Email (optional)" htmlFor="new-email">
                    <Input
                      id="new-email"
                      type="email"
                      value={newEmail}
                      onChange={(e) => setNewEmail(e.target.value)}
                      autoComplete="off"
                    />
                  </Field>
                  <Field label="Password" htmlFor="new-password" hint="At least 8 characters.">
                    <Input
                      id="new-password"
                      type="password"
                      value={newPassword}
                      onChange={(e) => setNewPassword(e.target.value)}
                      autoComplete="new-password"
                      required
                    />
                  </Field>
                  <label style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                    <input
                      type="checkbox"
                      checked={newIsAdmin}
                      onChange={(e) => setNewIsAdmin(e.target.checked)}
                      style={{ accentColor: 'var(--accent)' }}
                    />
                    Administrator
                  </label>
                  {create.isError && (
                    <div style={{ color: 'var(--signal)' }}>{errorMessage(create.error)}</div>
                  )}
                  <div style={{ display: 'flex', gap: 8 }}>
                    <Button type="submit" variant="primary" disabled={create.isPending}>
                      {create.isPending ? 'Creating…' : 'Create user'}
                    </Button>
                    <Button variant="ghost" onClick={() => setShowCreate(false)}>
                      Cancel
                    </Button>
                  </div>
                </form>
              </Panel>
            )}
          </div>
        )}

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

      <ConfirmModal
        open={deleteTarget != null}
        title="Delete user"
        destructive
        confirmLabel="Delete"
        body={
          deleteTarget ? (
            <p>
              Delete <b>{userDisplayName(deleteTarget)}</b>? Group memberships and API tokens of
              this user are removed as well. OIDC users reappear on their next login; local users
              are permanently revoked.
            </p>
          ) : null
        }
        error={remove.isError ? errorMessage(remove.error) : null}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) {
            remove.mutate(deleteTarget.id, {
              onSuccess: () => setDeleteTarget(null),
              onError: () => undefined, // keep modal open, show error
            })
          }
        }}
      />

      <ConfirmModal
        open={resetTarget != null}
        title="Reset password"
        confirmLabel="Reset"
        body={
          resetTarget ? (
            <Field label="New password" htmlFor="reset-password" hint="At least 8 characters.">
              <Input
                id="reset-password"
                type="password"
                value={resetPassword}
                onChange={(e) => setResetPassword(e.target.value)}
                autoComplete="new-password"
              />
            </Field>
          ) : null
        }
        error={resetPw.isError ? errorMessage(resetPw.error) : null}
        onCancel={() => setResetTarget(null)}
        onConfirm={() => {
          if (resetTarget && resetPassword) {
            resetPw.mutate(
              { id: resetTarget.id, password: resetPassword },
              {
                onSuccess: () => setResetTarget(null),
                onError: () => undefined,
              },
            )
          }
        }}
      />
    </>
  )
}
