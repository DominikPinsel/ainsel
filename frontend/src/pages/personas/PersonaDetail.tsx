import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAgents } from '../../api/agents'
import { ApiError } from '../../api/client'
import { useDeletePersona, usePersona } from '../../api/personas'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { Markdown } from '../../primitives/Markdown'
import { Panel } from '../../primitives/Panel'
import { SectionStatus } from '../../primitives/SectionStatus'
import { Tabs } from '../../primitives/Tabs'
import { Titleblock } from '../../layout/Titleblock'
import { formatISO } from '../../utils/time'

const TABS = [
  { value: 'overview', label: 'Overview' },
  { value: 'status', label: 'Status' },
] as const

export function PersonaDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [tab, setTab] = useState<string>('overview')
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [referrers, setReferrers] = useState<string[]>([])
  const { data, isLoading, error } = usePersona(id)
  const { data: agents } = useAgents({ pageSize: 100 })
  const del = useDeletePersona()

  const referringAgents = (agents?.items ?? []).filter(
    (a) => a.persona?.id === id,
  )

  const onDelete = async () => {
    if (!id) return
    try {
      await del.mutateAsync(id)
      setConfirmOpen(false)
      navigate('/personas', { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        const body = err.body as { referrers?: { agentName: string }[] } | undefined
        setReferrers(body?.referrers?.map((r) => r.agentName) ?? [])
      } else {
        throw err
      }
    }
  }

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Fleet / <Link to="/personas">Personas</Link> /{' '}
            <b>{data?.name ?? id ?? '—'}</b>
          </>
        }
        title={<>{data?.name ?? <em>Persona</em>}</>}
        actions={
          <>
            <Button
              onClick={() =>
                id && navigate(`/personas/${encodeURIComponent(id)}/edit`)
              }
            >
              Edit
            </Button>
            <Button variant="danger" onClick={() => setConfirmOpen(true)}>
              Delete
            </Button>
          </>
        }
      />
      <div style={{ padding: '28px 32px' }}>
        <Tabs value={tab} onChange={setTab} tabs={TABS} aria-label="Persona sections" />
        <div style={{ marginTop: 24 }}>
          {isLoading ? <p className="label">Loading…</p> : null}
          {error ? (
            <p className="label" style={{ color: 'var(--signal)' }}>
              Failed to load persona.
            </p>
          ) : null}

          {data && tab === 'overview' ? (
            <div style={{ display: 'grid', gap: 24 }}>
              <Panel title="Identity" className="cropped">
                <div className="info-grid">
                  <div>
                    <div className="k">ID</div>
                    <div className="v num">{data.id}</div>
                  </div>
                  <div>
                    <div className="k">Name</div>
                    <div className="v">{data.name}</div>
                  </div>
                  <div>
                    <div className="k">Description</div>
                    <div className="v">{data.description || '—'}</div>
                  </div>
                  <div>
                    <div className="k">Version</div>
                    <div className="v">v{data.currentVersion}</div>
                  </div>
                  <div>
                    <div className="k">Created</div>
                    <div className="v num">{formatISO(data.createdAt)}</div>
                  </div>
                  <div>
                    <div className="k">Updated</div>
                    <div className="v num">{formatISO(data.updatedAt)}</div>
                  </div>
                </div>
              </Panel>

              <Panel title="Used by" className="cropped">
                {referringAgents.length === 0 ? (
                  <p className="label">No agents reference this persona yet.</p>
                ) : (
                  <ul style={{ listStyle: 'none', padding: 0, margin: 0 }}>
                    {referringAgents.map((a) => (
                      <li key={a.id} style={{ padding: '4px 0' }}>
                        <Link to={`/agents/${encodeURIComponent(a.id)}`}>
                          {a.name}
                        </Link>
                      </li>
                    ))}
                  </ul>
                )}
              </Panel>

              <Panel title="Persona text" className="cropped">
                <Markdown source={data.text} />
              </Panel>
            </div>
          ) : null}

          {data && tab === 'status' ? (
            <Panel title="Version history" className="cropped">
              <SectionStatus
                state="unavailable"
                title="Version history coming soon"
                detail={`Current version: v${data.currentVersion}`}
              />
            </Panel>
          ) : null}
        </div>
      </div>

      <ConfirmModal
        open={confirmOpen}
        title={`Delete persona "${data?.name ?? ''}"?`}
        destructive
        confirmLabel={
          referrers.length > 0
            ? 'Cannot delete'
            : del.isPending
            ? 'Deleting…'
            : 'Delete'
        }
        body={
          referrers.length > 0 ? (
            <div>
              <p>
                This persona is referenced by {referrers.length} agent
                {referrers.length === 1 ? '' : 's'}. Unreference
                {referrers.length === 1 ? ' it' : ' them'} before deleting:
              </p>
              <ul>
                {referrers.map((name) => (
                  <li key={name}>{name}</li>
                ))}
              </ul>
            </div>
          ) : (
            <p>This will permanently delete the persona and its version history.</p>
          )
        }
        onConfirm={() => {
          if (referrers.length === 0) onDelete()
        }}
        onCancel={() => {
          setConfirmOpen(false)
          setReferrers([])
        }}
      />
    </>
  )
}
