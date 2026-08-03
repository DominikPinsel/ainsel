import { useCallback, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../../api/client'
import { useAgentImages } from '../../api/agentImages'
import {
  useAssignSkill,
  useDeleteSkill,
  useSkill,
  useSkillAssignments,
  useUnassignSkill,
} from '../../api/skills'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { DualListPicker } from '../../primitives/DualListPicker'
import { Markdown } from '../../primitives/Markdown'
import { Panel } from '../../primitives/Panel'
import { Tag } from '../../primitives/Tag'
import { Titleblock } from '../../layout/Titleblock'
import { formatISO } from '../../utils/time'

export function SkillDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [referrers, setReferrers] = useState<string[]>([])
  const { data, isLoading, error } = useSkill(id)
  const del = useDeleteSkill()

  const onDelete = async () => {
    if (!id) return
    try {
      await del.mutateAsync(id)
      setConfirmOpen(false)
      navigate('/skills', { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        const body = err.body as { referrers?: { agentImageName: string }[] } | undefined
        setReferrers(body?.referrers?.map((r) => r.agentImageName) ?? [])
      } else if (err instanceof ApiError) {
        setReferrers([])
        setConfirmOpen(true)
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
            Fleet / <Link to="/skills">Skills</Link> / <b>{data?.name ?? id ?? '—'}</b>
          </>
        }
        title={<>{data?.name ?? <em>Skill</em>}</>}
        actions={
          <>
            <Button onClick={() => id && navigate(`/skills/${encodeURIComponent(id)}/edit`)}>
              Edit
            </Button>
            <Button variant="danger" onClick={() => setConfirmOpen(true)}>
              Delete
            </Button>
          </>
        }
      />
      <div style={{ padding: '28px 32px' }}>
        {isLoading ? <p className="label">Loading…</p> : null}
        {error ? (
          <p className="label" style={{ color: 'var(--signal)' }}>
            Failed to load skill.
          </p>
        ) : null}

        {data ? (
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
                  <div className="k">Tags</div>
                  <div className="v">
                    {data.tags && data.tags.length > 0 ? (
                      <span style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                        {data.tags.map((t) => (
                          <Tag key={t}>{t}</Tag>
                        ))}
                      </span>
                    ) : (
                      '—'
                    )}
                  </div>
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

            <Panel title="Skill body" className="cropped">
              {data.body ? (
                <Markdown source={data.body} />
              ) : (
                <p className="label">This skill has no body yet.</p>
              )}
            </Panel>

            <AgentImagesPanel skillId={data.id} />
          </div>
        ) : null}
      </div>

      <ConfirmModal
        open={confirmOpen}
        title={`Delete skill "${data?.name ?? ''}"?`}
        destructive
        confirmLabel={
          referrers.length > 0 ? 'Cannot delete' : del.isPending ? 'Deleting…' : 'Delete'
        }
        body={
          referrers.length > 0 ? (
            <div>
              <p>
                This skill is enabled on {referrers.length} agent image
                {referrers.length === 1 ? '' : 's'}. Remove it from
                {referrers.length === 1 ? ' it' : ' them'} before deleting:
              </p>
              <ul>
                {referrers.map((name) => (
                  <li key={name}>{name}</li>
                ))}
              </ul>
            </div>
          ) : (
            <p>This will permanently delete the skill and remove it from the shared ConfigMap.</p>
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

function AgentImagesPanel({ skillId }: { skillId: string }) {
  const { data: assignmentsData, isLoading: assignmentsLoading } = useSkillAssignments(skillId)
  const { data: imagesData, isLoading: imagesLoading } = useAgentImages({ pageSize: 200 })

  const assign = useAssignSkill()
  const unassign = useUnassignSkill()

  const images = imagesData?.items ?? []
  const assignedNames = useMemo(
    () => assignmentsData?.items?.map((a) => a.agentImageName) ?? [],
    [assignmentsData],
  )

  const handleChange = useCallback(
    async (nextSelected: string[]) => {
      const prevSet = new Set(assignedNames)
      const nextSet = new Set(nextSelected)

      // Images to assign (in next but not in prev)
      const toAdd = nextSelected.filter((n) => !prevSet.has(n))
      // Images to unassign (in prev but not in next)
      const toRemove = assignedNames.filter((n) => !nextSet.has(n))

      // Run mutations sequentially so we stop on first failure and
      // can surface the error to the user.
      try {
        for (const name of toAdd) {
          await assign.mutateAsync({ skillId, agentImageName: name })
        }
        for (const name of toRemove) {
          await unassign.mutateAsync({ skillId, agentImageName: name })
        }
      } catch {
        // Revert the picker to the server state on failure.
        assign.reset()
        unassign.reset()
      }
    },
    [assignedNames, assign, unassign, skillId],
  )

  return (
    <Panel title="Agent images">
      <DualListPicker
        items={images}
        selectedIds={assignedNames}
        onChange={handleChange}
        getId={(img) => img.id}
        getLabel={(img) => img.displayName || img.id}
        getDescription={(img) => img.description}
        isLoading={assignmentsLoading || imagesLoading}
        emptyLabel="No agent images available. Create agent images first to assign this skill."
        availableTitle="Available images"
        enabledTitle="Enabled on image"
      />
    </Panel>
  )
}
