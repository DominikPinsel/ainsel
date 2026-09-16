import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useUpdateAgent, type AgentResponse } from '../../api/agents'
import { useAgentPersona, usePersonas, useSaveAgentPersona } from '../../api/personas'
import { ApiError } from '../../api/client'
import { Button } from '../../primitives/Button'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Panel } from '../../primitives/Panel'
import { Select } from '../../primitives/Select'
import { Textarea } from '../../primitives/Textarea'

const schema = z.object({
  name: z.string().max(200).optional().default(''),
  description: z.string().max(2000).optional().default(''),
  text: z.string().min(1, 'Persona text is required').max(100_000),
})

// zod 4 tracks input/output types separately (defaults make inputs wider);
// useForm needs both so the resolver types line up.
type FormInput = z.input<typeof schema>
type FormValues = z.infer<typeof schema>

type AgentPersonaSectionProps = {
  agent: AgentResponse
}

/**
 * The Persona tab of the agent detail page: which persona the agent uses, and
 * its content.
 *
 * The content is agent-owned. Saving writes to the agent's own persona — the
 * hub forks a private copy the first time an agent that still references a
 * shared template is edited here, so a template used by other agents is never
 * rewritten from this page. The hints say which case applies, and a template
 * can still be selected as the starting point.
 */
export function AgentPersonaSection({ agent }: AgentPersonaSectionProps) {
  const personas = usePersonas({ pageSize: 200 })
  const view = useAgentPersona(agent.id)
  const savePersona = useSaveAgentPersona()
  const updateAgent = useUpdateAgent()
  const [refError, setRefError] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)

  const persona = view.data?.persona
  const ref = view.data?.ref ?? ''
  const owned = Boolean(view.data?.owned)
  // A reference that points at a persona which no longer exists.
  const dangling = Boolean(ref && !persona)

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<FormInput, unknown, FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', description: '', text: '' },
  })

  // Prefill from the linked persona; an owned persona that was just created
  // arrives through the same query, so the form tracks the saved state.
  useEffect(() => {
    if (!view.data) return
    reset({
      name: persona?.name ?? '',
      description: persona?.description ?? '',
      text: persona?.text ?? '',
    })
  }, [view.data, persona, reset])

  const onSwitchPersona = (next: string) => {
    if (!next || next === ref) return
    setRefError(null)
    updateAgent.mutate(
      { id: agent.id, body: { name: agent.name, persona: { id: next } } },
      {
        onError: (err) =>
          setRefError(err instanceof ApiError ? err.message : 'Failed to switch persona.'),
        onSuccess: () => view.refetch(),
      },
    )
  }

  const onSubmit = handleSubmit(async (values) => {
    setSaveError(null)
    try {
      await savePersona.mutateAsync({
        agentId: agent.id,
        body: {
          name: values.name || undefined,
          description: values.description || '',
          text: values.text,
        },
      })
    } catch (err) {
      setSaveError(err instanceof ApiError ? err.message : 'Failed to save persona.')
    }
  })

  // The owned persona is hidden from the library list, so it needs its own
  // option to stay selectable while it is linked.
  const options = (personas.data?.items ?? [])
    .filter((p) => p.id !== ref)
    .map((p) => ({ value: p.id, label: p.name }))
  if (persona) {
    options.unshift({
      value: ref,
      label: owned ? `${persona.name} — this agent's own` : persona.name,
    })
  }

  const contentHint = !ref
    ? 'This agent has no persona yet. Saving creates one that belongs to this agent alone.'
    : dangling
      ? `The linked persona (${ref}) no longer exists. Saving creates a new one for this agent.`
      : owned
        ? "This persona belongs to this agent — changes stay private to it."
        : 'This persona is a shared template. Saving creates a private copy for this agent and leaves the template untouched.'

  return (
    <div style={{ display: 'grid', gap: 24 }}>
      <Panel title="Linked Persona" className="cropped">
        <div style={{ display: 'grid', gap: 10 }}>
          <Select
            aria-label="Linked persona"
            value={ref}
            onChange={onSwitchPersona}
            options={options}
            emptyLabel="No persona linked"
            disabled={updateAgent.isPending || view.isLoading}
          />
          <p className="label" style={{ margin: 0, color: 'var(--ink-3)' }}>
            Pick a shared template as the starting point, or edit below to give
            this agent its own persona.
          </p>
          {refError ? (
            <p className="label" style={{ margin: 0, color: 'var(--signal)' }}>
              {refError}
            </p>
          ) : null}
        </div>
      </Panel>

      <Panel title="Persona Content" className="cropped">
        {view.isLoading ? (
          <p className="label">Loading persona…</p>
        ) : (
          <form onSubmit={onSubmit} noValidate style={{ display: 'grid', gap: 14 }}>
            <Field
              label="Name"
              htmlFor="persona-name"
              error={errors.name?.message}
              hint="Optional — defaults to the agent's name."
            >
              <Input id="persona-name" {...register('name')} />
            </Field>
            <Field
              label="Description"
              htmlFor="persona-description"
              error={errors.description?.message}
            >
              <Input id="persona-description" {...register('description')} />
            </Field>
            <Field label="Persona Text" htmlFor="persona-text" error={errors.text?.message}>
              <Textarea
                id="persona-text"
                rows={20}
                className="mono"
                spellCheck={false}
                {...register('text')}
              />
            </Field>
            <p className="label" style={{ margin: 0, color: 'var(--ink-3)' }}>
              {contentHint}
            </p>
            {saveError ? (
              <p className="label" style={{ margin: 0, color: 'var(--signal)' }}>
                {saveError}
              </p>
            ) : null}
            <div style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
              <Button type="submit" variant="primary" disabled={isSubmitting}>
                {isSubmitting ? 'Saving…' : !ref || !owned ? 'Save as own persona' : 'Save persona'}
              </Button>
              {!owned && ref && !dangling ? (
                <span className="label" style={{ color: 'var(--ink-3)' }}>
                  First save forks a private copy.
                </span>
              ) : null}
            </div>
          </form>
        )}
      </Panel>
    </div>
  )
}
