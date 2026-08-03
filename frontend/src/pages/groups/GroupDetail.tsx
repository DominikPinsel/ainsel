import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  useGroup,
  useAddGroupMembers,
  useRemoveGroupMember,
  type GroupMember,
  type GroupRole,
} from '../../api/groups'
import { useUsers, userDisplayName } from '../../api/users'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { Panel } from '../../primitives/Panel'
import { RegisterTable, type Column } from '../../primitives/RegisterTable'
import { Autocomplete } from '../../primitives/Autocomplete'
import { Titleblock } from '../../layout/Titleblock'

const ROLE_LABEL: Record<GroupRole, string> = {
  reader: 'Reader',
  writer: 'Writer',
  owner: 'Owner',
}

export function GroupDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data, isLoading, error } = useGroup(id)
  const users = useUsers()
  const addMembers = useAddGroupMembers()
  const removeMember = useRemoveGroupMember()
  const [confirmUserId, setConfirmUserId] = useState<string | null>(null)
  const [selectedUserId, setSelectedUserId] = useState('')
  const [selectedRole, setSelectedRole] = useState<GroupRole>('reader')

  const memberIds = new Set((data?.members ?? []).map((m) => m.user.id))
  const nonMembers = (users.data ?? []).filter((u) => !memberIds.has(u.id))
  const userById = useMemo(
    () => new Map((users.data ?? []).map((u) => [u.id, u])),
    [users.data],
  )

  const onAddMember = async () => {
    if (!id || !selectedUserId) return
    await addMembers.mutateAsync({ groupId: id, userIds: [selectedUserId], role: selectedRole })
    setSelectedUserId('')
  }

  const onConfirmRemove = async () => {
    if (!id || !confirmUserId) return
    await removeMember.mutateAsync({ groupId: id, userId: confirmUserId })
    setConfirmUserId(null)
  }

  const confirmMember = (data?.members ?? []).find((m) => m.user.id === confirmUserId)

  const memberColumns: readonly Column<GroupMember>[] = [
    {
      key: 'username',
      header: 'Username',
      cell: (m) => <b>{userDisplayName(m.user)}</b>,
    },
    {
      key: 'email',
      header: 'Email',
      cell: (m) => <span className="num">{m.user.email}</span>,
    },
    {
      key: 'role',
      header: 'Role',
      width: 100,
      cell: (m) => <b>{ROLE_LABEL[m.role] ?? m.role}</b>,
    },
    {
      key: 'admin',
      header: 'Admin',
      width: 80,
      align: 'center',
      cell: (m) => (m.user.isAdmin ? 'Yes' : '—'),
    },
    {
      key: 'actions',
      header: '',
      width: 80,
      align: 'right',
      cell: (m) => (
        <Button
          variant="danger"
          size="sm"
          onClick={() => setConfirmUserId(m.user.id)}
        >
          Remove
        </Button>
      ),
    },
  ]

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Admin / <Link to="/groups">Groups</Link> /{' '}
            <b>{data?.group.name ?? id ?? '—'}</b>
          </>
        }
        title={<>{data?.group.name ?? <em>Group</em>}</>}
        actions={
          <Button onClick={() => id && navigate(`/groups/${encodeURIComponent(id)}/edit`)}>
            Edit
          </Button>
        }
      />
      <div style={{ padding: '28px 32px', display: 'grid', gap: 24 }}>
        {isLoading ? <p className="label">Loading…</p> : null}
        {error ? (
          <p className="label" style={{ color: 'var(--signal)' }}>
            Failed to load group.
          </p>
        ) : null}

        {data ? (
          <>
            <Panel title="General" className="cropped">
              <div className="info-grid">
                <div>
                  <div className="k">Name</div>
                  <div className="v">{data.group.name}</div>
                </div>
                <div>
                  <div className="k">Description</div>
                  <div className="v">{data.group.description || '—'}</div>
                </div>
              </div>
            </Panel>

            <Panel title={`Members · ${data.members.length}`} className="cropped">
              <RegisterTable
                rows={data.members}
                columns={memberColumns}
                rowKey={(m) => m.user.id}
                emptyLabel="No members yet."
              />
              <div
                style={{
                  display: 'flex',
                  gap: 8,
                  padding: '12px 14px',
                  borderTop: '1px solid var(--chrome-1)',
                }}
              >
                <Autocomplete
                  value={selectedUserId}
                  onChange={setSelectedUserId}
                  options={nonMembers.map((u) => ({
                    value: u.id,
                    label: userDisplayName(u),
                  }))}
                  placeholder="Add a member…"
                  filter={(option, query) => {
                    const u = userById.get(option.value)
                    if (!u) return false
                    const q = query.toLowerCase()
                    return (
                      userDisplayName(u).toLowerCase().includes(q) ||
                      u.email.toLowerCase().includes(q)
                    )
                  }}
                  aria-label="Select user to add"
                />
                <select
                  value={selectedRole}
                  onChange={(e) => setSelectedRole(e.target.value as GroupRole)}
                  aria-label="Role for new member"
                  style={{ padding: '6px 8px' }}
                >
                  <option value="reader">Reader</option>
                  <option value="writer">Writer</option>
                  <option value="owner">Owner</option>
                </select>
                <Button
                  variant="primary"
                  size="sm"
                  onClick={onAddMember}
                  disabled={!selectedUserId || addMembers.isPending}
                >
                  {addMembers.isPending ? 'Adding…' : 'Add'}
                </Button>
              </div>
            </Panel>
          </>
        ) : null}
      </div>

      <ConfirmModal
        open={confirmUserId !== null}
        title="Remove member?"
        body={
          <>
            <b>{confirmMember ? userDisplayName(confirmMember.user) : confirmUserId}</b> will be removed from this group.
          </>
        }
        confirmLabel={removeMember.isPending ? 'Removing…' : 'Remove'}
        destructive
        onConfirm={onConfirmRemove}
        onCancel={() => setConfirmUserId(null)}
      />
    </>
  )
}
