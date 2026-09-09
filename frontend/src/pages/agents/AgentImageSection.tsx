import { useState } from 'react'
import { useUpdateAgent, type AgentResponse } from '../../api/agents'
import { useAgentImages } from '../../api/agentImages'
import { ApiError } from '../../api/client'
import { Panel } from '../../primitives/Panel'
import { Select } from '../../primitives/Select'
import { ImageFormContainer } from '../images/ImageFormContainer'

type AgentImageSectionProps = {
  agent: AgentResponse
}

/**
 * Shared image-reference picker shown on the Image tab of the agent detail
 * page: a dropdown that re-points the agent at another image. The referenced
 * image is what carries the runtime (container image, env, tools, skills,
 * MCP servers) — until the backend is streamlined, every tab edits the same
 * image object, just different sections of it.
 */
function ImageRefPicker({ agent }: AgentImageSectionProps) {
  const images = useAgentImages({ pageSize: 200 })
  const updateAgent = useUpdateAgent()
  const [refError, setRefError] = useState<string | null>(null)
  const imageName = agent.imageRef?.name

  const onSwitchImage = (next: string) => {
    if (!next || next === imageName) return
    setRefError(null)
    updateAgent.mutate(
      { id: agent.id, body: { name: agent.name, imageRef: { name: next } } },
      {
        onError: (err) =>
          setRefError(err instanceof ApiError ? err.message : 'Failed to switch image.'),
      },
    )
  }

  return (
    <Panel title="Image" className="cropped">
      <div style={{ display: 'grid', gap: 10 }}>
        <Select
          aria-label="Agent image"
          value={imageName ?? ''}
          onChange={onSwitchImage}
          options={(images.data?.items ?? []).map((i) => ({
            value: i.id,
            label: i.displayName ?? i.id,
          }))}
          emptyLabel="Select an image…"
          disabled={updateAgent.isPending}
        />
        <p className="label" style={{ margin: 0, color: 'var(--ink-3)' }}>
          The image bundles the runtime: container image URL, environment,
          tools, skills, and MCP servers. Switching re-points this agent;
          saving on any tab updates the shared image for every agent that
          references it.
        </p>
        {refError ? (
          <p className="label" style={{ margin: 0, color: 'var(--signal)' }}>
            {refError}
          </p>
        ) : null}
      </div>
    </Panel>
  )
}

function NoImagePanel({ what, hint }: { what: string; hint: string }) {
  return (
    <Panel title={what} className="cropped">
      <p className="label">{hint}</p>
    </Panel>
  )
}

/**
 * The Image tab of the agent detail page: image picker plus the image side
 * of the referenced image (identity: display name, container URL,
 * description — and environment variables).
 */
export function AgentImageSection({ agent }: AgentImageSectionProps) {
  const imageName = agent.imageRef?.name
  return (
    <div style={{ display: 'grid', gap: 24 }}>
      <ImageRefPicker agent={agent} />
      {imageName ? (
        <ImageFormContainer key={imageName} id={imageName} embedded sections="image" />
      ) : (
        <NoImagePanel
          what="Image"
          hint="No image linked yet — pick one above to configure it."
        />
      )}
    </div>
  )
}

/**
 * The Tools tab of the agent detail page: the tools side of the referenced
 * image (MCP servers and the tool list). The embedded header shows which
 * image is being edited; switching images happens on the Image tab.
 */
export function AgentToolsSection({ agent }: AgentImageSectionProps) {
  const imageName = agent.imageRef?.name
  return imageName ? (
    <ImageFormContainer key={imageName} id={imageName} embedded sections="tools" />
  ) : (
    <NoImagePanel
      what="Tools"
      hint="No image linked yet — pick one on the Image tab to configure its tools."
    />
  )
}

/**
 * The Skills tab of the agent detail page: the skills side of the referenced
 * image (which skills are enabled on it).
 */
export function AgentSkillsSection({ agent }: AgentImageSectionProps) {
  const imageName = agent.imageRef?.name
  return imageName ? (
    <ImageFormContainer key={imageName} id={imageName} embedded sections="skills" />
  ) : (
    <NoImagePanel
      what="Skills"
      hint="No image linked yet — pick one on the Image tab to configure its skills."
    />
  )
}
