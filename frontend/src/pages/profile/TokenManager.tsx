import { useEffect, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  useUserTokens,
  useCreateUserToken,
  useRevokeUserToken,
  type UserToken,
  type CreatedToken,
} from '../../api/userTokens'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { Input } from '../../primitives/Input'
import { Pager } from '../../primitives/Pager'
import { Panel } from '../../primitives/Panel'

function CreateTokenModal({
  open,
  onClose,
  onCreated,
}: {
  open: boolean
  onClose: () => void
  onCreated: (t: CreatedToken) => void
}) {
  const [name, setName] = useState('')
  const [days, setDays] = useState<30 | 60 | 90>(30)
  const [error, setError] = useState<string | null>(null)
  const { mutate: create, isPending } = useCreateUserToken()

  function handleCreate() {
    create(
      { name: name.trim(), expiresInDays: days },
      {
        onSuccess: (result) => {
          setError(null)
          onCreated(result)
          setName('')
          setDays(30)
        },
        onError: (err) => setError(err instanceof Error ? err.message : 'Failed to create token'),
      },
    )
  }

  function handleClose() {
    setName('')
    setDays(30)
    setError(null)
    onClose()
  }

  if (!open) return null
  return createPortal(
    <div className="modal-backdrop" role="dialog" aria-modal="true" aria-labelledby="create-token-dialog-title">
      <div className="modal-box">
        <h3 id="create-token-dialog-title">New MCP Token</h3>
        <div className="modal-body" style={{ display: 'grid', gap: 16 }}>
          <label>
            <div className="label" style={{ marginBottom: 6 }}>
              Token name
            </div>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. my-laptop"
              style={{ width: '100%' }}
            />
          </label>
          <div>
            <div className="label" style={{ marginBottom: 6 }}>
              Expires in
            </div>
            {([30, 60, 90] as const).map((d) => (
              <label
                key={d}
                style={{ display: 'inline-flex', alignItems: 'center', gap: 6, marginRight: 16 }}
              >
                <input
                  type="radio"
                  name="expires"
                  value={d}
                  checked={days === d}
                  onChange={() => setDays(d)}
                />
                {d} days
              </label>
            ))}
          </div>
        </div>
        {error && <p style={{ margin: 0, color: 'var(--danger)' }}>{error}</p>}
        <div className="modal-actions">
          <Button onClick={handleClose}>Cancel</Button>
          <Button variant="primary" disabled={!name.trim() || isPending} onClick={handleCreate}>
            {isPending ? 'Creating…' : 'Create'}
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  )
}

function NewTokenDisplay({
  token,
  onClose,
}: {
  token: CreatedToken | null
  onClose: () => void
}) {
  if (token === null) return null
  return createPortal(
    <div className="modal-backdrop" role="dialog" aria-modal="true" aria-labelledby="new-token-display-title">
      <div className="modal-box">
        <h3 id="new-token-display-title">Token Created</h3>
        <div className="modal-body" style={{ display: 'grid', gap: 12 }}>
          <p style={{ margin: 0 }}>Copy your token now. It will not be shown again.</p>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <code
              style={{
                flex: 1,
                wordBreak: 'break-all',
                background: 'var(--surface-2)',
                padding: '6px 10px',
                borderRadius: 4,
                fontSize: 13,
              }}
            >
              {token.token}
            </code>
            <Button size="sm" onClick={() => navigator.clipboard.writeText(token.token)}>
              Copy
            </Button>
          </div>
        </div>
        <div className="modal-actions">
          <Button variant="primary" onClick={onClose}>
            Done
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  )
}

export function TokenManager() {
  const [createOpen, setCreateOpen] = useState(false)
  const [newToken, setNewToken] = useState<CreatedToken | null>(null)
  const [revokeTarget, setRevokeTarget] = useState<UserToken | null>(null)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(5)
  const { data: tokens, isLoading, isError } = useUserTokens()
  const { mutate: revoke } = useRevokeUserToken()

  const sortedTokens = useMemo(() => {
    if (!tokens) return []
    return [...tokens].sort((a, b) => {
      const aRevoked = a.revokedAt ? 1 : 0
      const bRevoked = b.revokedAt ? 1 : 0
      if (aRevoked !== bRevoked) return aRevoked - bRevoked
      return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
    })
  }, [tokens])

  const totalPages = Math.ceil(sortedTokens.length / pageSize)
  useEffect(() => {
    if (totalPages > 0 && page > totalPages) {
      setPage(totalPages)
    }
  }, [page, totalPages])

  const clampedPage = Math.max(1, Math.min(page, totalPages || 1))
  const pagedTokens = sortedTokens.slice((clampedPage - 1) * pageSize, clampedPage * pageSize)

  function handleCreated(t: CreatedToken) {
    setCreateOpen(false)
    setNewToken(t)
  }

  function handleRevoke() {
    if (!revokeTarget) return
    revoke(revokeTarget.id, {
      onSuccess: () => {
        setRevokeTarget(null)
      },
    })
  }

  return (
    <>
      <Panel
        title="MCP Tokens"
        right={
          <Button size="sm" variant="primary" onClick={() => setCreateOpen(true)}>
            New Token
          </Button>
        }
      >
        {isLoading ? (
          <div className="label" style={{ padding: 14 }}>
            Loading…
          </div>
        ) : isError ? (
          <div className="label" style={{ padding: '14px 16px', color: 'var(--danger)' }}>
            Failed to load tokens.
          </div>
        ) : !tokens || tokens.length === 0 ? (
          <div className="label" style={{ padding: '14px 16px' }}>
            No tokens yet.
          </div>
        ) : (
          <>
            <table style={{ width: '100%', borderCollapse: 'collapse' }}>
              <thead>
                <tr>
                  <th style={{ textAlign: 'left', padding: '8px 16px' }}>Name</th>
                  <th style={{ textAlign: 'left', padding: '8px 16px' }}>Created</th>
                  <th style={{ textAlign: 'left', padding: '8px 16px' }}>Expires</th>
                  <th style={{ textAlign: 'left', padding: '8px 16px' }}>Last used</th>
                  <th style={{ textAlign: 'left', padding: '8px 16px' }}>Status</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {pagedTokens.map((tok) => (
                  <tr key={tok.id} style={{ borderTop: '1px solid var(--border)' }}>
                    <td style={{ padding: '8px 16px' }}>{tok.name}</td>
                    <td style={{ padding: '8px 16px' }}>
                      {new Date(tok.createdAt).toLocaleDateString()}
                    </td>
                    <td style={{ padding: '8px 16px' }}>
                      {new Date(tok.expiresAt).toLocaleDateString()}
                    </td>
                    <td style={{ padding: '8px 16px' }}>
                      {tok.lastUsedAt ? new Date(tok.lastUsedAt).toLocaleDateString() : '—'}
                    </td>
                    <td style={{ padding: '8px 16px' }}>
                      <span className={tok.revokedAt ? 'label' : 'tag tag-green'}>
                        {tok.revokedAt ? 'Revoked' : 'Active'}
                      </span>
                    </td>
                    <td style={{ padding: '8px 16px', textAlign: 'right' }}>
                      {!tok.revokedAt && (
                        <Button
                          size="sm"
                          variant="danger"
                          onClick={() => {
                            setRevokeTarget(tok)
                          }}
                        >
                          Revoke
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <Pager
              page={clampedPage}
              pageSize={pageSize}
              total={sortedTokens.length}
              pageSizeOptions={[5, 10, 20] as const}
              onPageChange={setPage}
              onPageSizeChange={(size) => {
                setPageSize(size)
                setPage(1)
              }}
            />
          </>
        )}
      </Panel>

      <CreateTokenModal open={createOpen} onClose={() => setCreateOpen(false)} onCreated={handleCreated} />

      <NewTokenDisplay token={newToken} onClose={() => setNewToken(null)} />

      <ConfirmModal
        open={revokeTarget !== null}
        title="Revoke Token"
        body={
          <>
            Revoke <b>{revokeTarget?.name}</b>? This cannot be undone.
          </>
        }
        confirmLabel="Revoke"
        destructive
        onConfirm={handleRevoke}
        onCancel={() => {
          setRevokeTarget(null)
        }}
      />
    </>
  )
}
