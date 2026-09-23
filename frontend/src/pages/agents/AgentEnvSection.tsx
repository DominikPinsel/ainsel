import { useState } from 'react'
import {
  useUpdateAgent,
  type AgentEnvVar,
  type AgentResponse,
} from '../../api/agents'
import { useAgentImage } from '../../api/agentImages'
import { ApiError } from '../../api/client'
import { Button } from '../../primitives/Button'
import { Input } from '../../primitives/Input'
import { Panel } from '../../primitives/Panel'
import { Tag } from '../../primitives/Tag'

type AgentEnvSectionProps = {
  agent: AgentResponse
}

type DraftRow = {
  key: number
  name: string
  value: string
  secret: boolean
}

let rowKey = 0
const nextKey = () => ++rowKey

function toRows(env: AgentEnvVar[]): DraftRow[] {
  return env.map((e) => ({
    key: nextKey(),
    name: e.name,
    value: e.value,
    secret: !!e.secret,
  }))
}

function fromRows(rows: DraftRow[]): AgentEnvVar[] {
  return rows.map((r) => ({
    name: r.name.trim(),
    value: r.value,
    secret: r.secret,
  }))
}

/**
 * The Runtime tab's agent-scoped environment section: this agent's own
 * variables, layered on top of the runtime image's env. Image entries the
 * agent does not override are listed read-only for context; the first override
 * pins an env list to this agent, and clearing every row hands control back to
 * the image.
 *
 * Secret values are masked by the hub, so an existing secret row shows an
 * empty field: submitting it unchanged keeps the stored value.
 */
export function AgentEnvSection({ agent }: AgentEnvSectionProps) {
  const imageName = agent.imageRef?.name
  const image = useAgentImage(imageName)
  const updateAgent = useUpdateAgent()

  const stored = agent.env ?? []
  const serialized = JSON.stringify(stored)
  const [rows, setRows] = useState<DraftRow[]>(() => toRows(stored))
  const [syncedFrom, setSyncedFrom] = useState(serialized)
  const [saveError, setSaveError] = useState<string | null>(null)

  // Re-seed the draft when the stored list changes underneath us (a save
  // landing, or another tab re-pointing the image), never while typing.
  if (serialized !== syncedFrom) {
    setSyncedFrom(serialized)
    setRows(toRows(stored))
    setSaveError(null)
  }

  const dirty = JSON.stringify(fromRows(rows)) !== serialized

  const imageEnv = image.data?.env ?? []
  const overridden = new Set(rows.map((r) => r.name.trim()).filter(Boolean))

  const nameErrors = rows.map((row, idx) => {
    const name = row.name.trim()
    if (name === '') return 'Name is required.'
    if (rows.some((other, i) => i !== idx && other.name.trim() === name)) {
      return 'Duplicate name.'
    }
    return null
  })
  const hasErrors = nameErrors.some((e) => e !== null)

  const setRow = (key: number, patch: Partial<DraftRow>) => {
    setSaveError(null)
    setRows((prev) => prev.map((r) => (r.key === key ? { ...r, ...patch } : r)))
  }

  const onSave = () => {
    if (hasErrors) return
    setSaveError(null)
    updateAgent.mutate(
      { id: agent.id, body: { name: agent.name, env: fromRows(rows) } },
      {
        onError: (err) =>
          setSaveError(
            err instanceof ApiError ? err.message : 'Failed to save environment.',
          ),
      },
    )
  }

  if (!imageName) {
    return (
      <Panel title="Environment">
        <p className="label" style={{ margin: 0, color: 'var(--ink-3)' }}>
          No image linked yet — pick one on the Runtime tab to configure
          environment variables.
        </p>
      </Panel>
    )
  }

  return (
    <div style={{ display: 'grid', gap: 8 }}>
      <div style={{ marginTop: 24 }}>
        <Panel title="Environment">
          <div style={{ display: 'grid', gap: 8 }}>
            {imageEnv.length > 0 ? (
              <div style={{ display: 'grid', gap: 4 }}>
                <span className="label" style={{ color: 'var(--ink-3)' }}>
                  From the runtime image
                </span>
                {imageEnv.map((e) => (
                  <div
                    key={e.name}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                      fontSize: 13,
                    }}
                  >
                    <code>{e.name}</code>
                    {e.secret ? (
                      <span style={{ color: 'var(--ink-3)' }}>••••••••</span>
                    ) : (
                      <span style={{ color: 'var(--ink-3)' }}>
                        {e.value || '(empty)'}
                      </span>
                    )}
                    {overridden.has(e.name) ? <Tag>overridden here</Tag> : null}
                  </div>
                ))}
              </div>
            ) : null}

            {rows.length > 0 ? (
              <span className="label" style={{ color: 'var(--ink-3)' }}>
                This agent's overrides
              </span>
            ) : null}

            {rows.map((row, idx) => (
              <div
                key={row.key}
                style={{
                  display: 'grid',
                  gridTemplateColumns: '1fr 1fr auto auto',
                  gap: 8,
                  alignItems: 'start',
                }}
              >
                <div>
                  <Input
                    placeholder="NAME"
                    aria-label="Variable name"
                    value={row.name}
                    onChange={(e) => setRow(row.key, { name: e.target.value })}
                  />
                  {nameErrors[idx] ? (
                    <div className="field-error">{nameErrors[idx]}</div>
                  ) : null}
                </div>
                <Input
                  placeholder={row.secret ? 'unchanged' : 'value'}
                  aria-label="Variable value"
                  type={row.secret ? 'password' : 'text'}
                  value={row.value}
                  onChange={(e) => setRow(row.key, { value: e.target.value })}
                />
                <label
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 4,
                    fontSize: 12,
                    whiteSpace: 'nowrap',
                    cursor: 'pointer',
                  }}
                >
                  <input
                    type="checkbox"
                    checked={row.secret}
                    onChange={(e) => setRow(row.key, { secret: e.target.checked })}
                  />
                  Secret
                </label>
                <Button
                  type="button"
                  aria-label="Remove variable"
                  onClick={() => {
                    setSaveError(null)
                    setRows((prev) => prev.filter((r) => r.key !== row.key))
                  }}
                >
                  ×
                </Button>
              </div>
            ))}

            <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <Button
                type="button"
                onClick={() =>
                  setRows((prev) => [
                    ...prev,
                    { key: nextKey(), name: '', value: '', secret: false },
                  ])
                }
              >
                Add Variable
              </Button>
              <Button
                type="button"
                onClick={onSave}
                disabled={!dirty || hasErrors || updateAgent.isPending}
              >
                {updateAgent.isPending ? 'Saving…' : 'Save overrides'}
              </Button>
            </div>
          </div>
        </Panel>
      </div>

      {agent.env ? null : (
        <p className="label" style={{ margin: '0 0 4px', color: 'var(--ink-3)' }}>
          Currently inherited from the runtime image — the first change pins the
          environment to this agent.
        </p>
      )}
      {saveError ? (
        <p className="label" style={{ margin: 0, color: 'var(--signal)' }}>
          {saveError}
        </p>
      ) : null}
    </div>
  )
}
