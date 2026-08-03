import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useConnector, useDeleteConnector, useUpdateConnector, useRotateConnectorSecret } from '../../api/connectors'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { Dot } from '../../primitives/Dot'
import { Panel } from '../../primitives/Panel'
import { Tag } from '../../primitives/Tag'
import { Titleblock } from '../../layout/Titleblock'
import { copy } from '../../utils/clipboard'

export function ConnectorDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [generatedSecret, setGeneratedSecret] = useState<string | null>(null)
  const { data, isLoading, error } = useConnector(id)
  const remove = useDeleteConnector()
  const toggle = useUpdateConnector()
  const rotate = useRotateConnectorSecret()

  const onConfirmDelete = async () => {
    if (!id) return
    await remove.mutateAsync(id)
    setConfirmOpen(false)
    navigate('/connectors', { replace: true })
  }

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Fleet / <Link to="/connectors">Connectors</Link> /{' '}
            <b>{data?.name ?? id ?? '—'}</b>
          </>
        }
        title={<>{data?.name ?? <em>Connector</em>}</>}
        actions={
          <>
            {data ? (
              <Button
                variant={data.disabled ? 'primary' : undefined}
                onClick={() => {
                  if (!id) return
                  toggle.mutate({
                    id,
                    body: { name: data.name, disabled: !data.disabled },
                  })
                }}
                disabled={toggle.isPending}
              >
                {data.disabled ? 'Enable' : 'Disable'}
              </Button>
            ) : null}
            <Button
              onClick={() => id && navigate(`/connectors/${encodeURIComponent(id)}/edit`)}
            >
              Edit
            </Button>
            <Button variant="danger" onClick={() => setConfirmOpen(true)}>
              Delete
            </Button>
          </>
        }
      />
      <div style={{ padding: '28px 32px', display: 'grid', gap: 24 }}>
        {isLoading ? <p className="label">Loading…</p> : null}
        {error ? (
          <p className="label" style={{ color: 'var(--signal)' }}>
            Failed to load connector.
          </p>
        ) : null}

        {data ? (
          <>
            <Panel title="Connection" className="cropped">
              <div className="info-grid">
                <div>
                  <div className="k">Name</div>
                  <div className="v">{data.name}</div>
                </div>
                <div>
                  <div className="k">Signature Header</div>
                  <div className="v">
                    <span className="num">{data.signatureHeader}</span>
                  </div>
                </div>
              </div>
            </Panel>

            <Panel title="Status" className="cropped">
              <div className="info-grid">
                <div>
                  <div className="k">Ready</div>
                  <div className="v">
                    {data.disabled ? (
                      <>
                        <Dot state="stale" /> Disabled
                      </>
                    ) : (
                      <>
                        <Dot state={data.status?.ready ? 'ok' : 'err'} />{' '}
                        {data.status?.ready ? 'Ready' : 'Failing'}
                      </>
                    )}
                  </div>
                </div>
              </div>
              {data.status?.conditions && data.status.conditions.length > 0 ? (
                <div style={{ marginTop: 12, display: 'grid', gap: 6 }}>
                  {data.status.conditions.map((c, i) => (
                    <div
                      key={i}
                      style={{
                        fontFamily: 'var(--mono)',
                        fontSize: 11,
                        color: c.status === 'False' ? 'var(--signal)' : 'var(--ink-2)',
                      }}
                    >
                      <b>{c.type}</b>: {c.status} — {c.reason ?? '—'}
                    </div>
                  ))}
                </div>
              ) : null}
            </Panel>

            {data.webhookEndpoint ? (
              <Panel title="Webhook Setup" className="cropped">
                <div style={{ display: 'grid', gap: 12 }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <span className="label" style={{ width: 90 }}>
                      Endpoint
                    </span>
                    <span className="num" style={{ flex: 1 }}>
                      {data.webhookEndpoint}
                    </span>
                    <Button
                      size="sm"
                      onClick={() => data.webhookEndpoint && copy(data.webhookEndpoint)}
                    >
                      ⧉ Copy
                    </Button>
                  </div>
                  {(() => {
                    const visibleSecret = generatedSecret ?? data.webhookSecretValue ?? null
                    return (
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                        <span className="label" style={{ width: 90 }}>
                          Secret
                        </span>
                        {visibleSecret ? (
                          <>
                            <span className="num" style={{ flex: 1 }}>
                              {visibleSecret}
                            </span>
                            <Button size="sm" onClick={() => copy(visibleSecret)}>
                              ⧉ Copy
                            </Button>
                            <Tag variant="err">ONE-TIME</Tag>
                          </>
                        ) : (
                          <span className="label" style={{ flex: 1, color: 'var(--ink-3)' }}>
                            ●●●●●●●●
                          </span>
                        )}
                        <Button
                          size="sm"
                          disabled={rotate.isPending}
                          onClick={() =>
                            id &&
                            rotate.mutateAsync(id).then((r) =>
                              setGeneratedSecret(r.webhookSecretValue ?? null),
                            )
                          }
                        >
                          {rotate.isPending ? 'Generating…' : 'Generate new'}
                        </Button>
                      </div>
                    )
                  })()}
                </div>
              </Panel>
            ) : null}
          </>
        ) : null}
      </div>

      <ConfirmModal
        open={confirmOpen}
        title="Delete connector?"
        body={
          <>
            <b>{data?.name ?? id}</b> will be permanently removed. Triggers
            referencing this connector will become invalid.
          </>
        }
        confirmLabel={remove.isPending ? 'Deleting…' : 'Delete'}
        destructive
        onConfirm={onConfirmDelete}
        onCancel={() => setConfirmOpen(false)}
      />
    </>
  )
}
