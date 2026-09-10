import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useUpdateAgent, type AgentResponse } from '../../api/agents'
import { usePersona, usePersonas, useUpdatePersona } from '../../api/personas'
import { ApiError } from '../../api/client'
import { Button } from '../../primitives/Button'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Panel } from '../../primitives/Panel'
import { Select } from '../../primitives/Select'
import { Textarea } from '../../primitives/Textarea'

const schema = z.object({
  name: z.string().min(1, 'Name is required').max(200),
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
 * The Persona tab of the agent detail page. Combines what used to be two
 * separate flows: choosing which persona the agent references (previously a
 * dropdown on the agent edit form) and editing that persona's content
 * (previously the standalone persona pages).
 *
 * Personas are still shared hub objects until the backend is streamlined —
 * saving here updates the persona for every agent that references it, which
 * the hints call out explicitly.
 */
export function AgentPersonaSection({ agent }: AgentPersonaSectionProps) {
  const personaId = agent.persona?.id
  const personas = usePersonas({ pageSize: 200 })
  const existing = usePersona(personaId)
  const updatePersona = useUpdatePersona()
  const updateAgent = useUpdateAgent()
  const [refError, setRefError] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<FormInput, unknown, FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', description: '', text: '' },
  })

  useEffect(() => {
    if (existing.data) {
      reset({
        name: existing.data.name,
        description: existing.data.description,
        text: existing.data.text,
      })
    }
  }, [existing.data, reset])

  const onSwitchPersona = (next: string) => {
    if (!next || next === personaId) return
    setRefError(null)
    updateAgent.mutate(
      { id: agent.id, body: { name: agent.name, persona: { id: next } } },
      {
        onError: (err) =>
          setRefError(err instanceof ApiError ? err.message : 'Failed to switch persona.'),
      },
    )
  }

  const onSubmit = handleSubmit(async (values) => {
    if (!personaId) return
    setSaveError(null)
    try {
      await updatePersona.mutateAsync({
        id: personaId,
        body: {
          name: values.name,
          description: values.description || '',
          text: values.text,
        },
      })
    } catch (err) {
      setSaveError(err instanceof ApiError ? err.message : 'Failed to save persona.')
    }
  })

  return (
    <div style={{ display: 'grid', gap: 24 }}>
      <Panel title="Linked Persona" className="cropped">
        <div style={{ display: 'grid', gap: 10 }}>
          <Select
            aria-label="Linked persona"
            value={personaId ?? ''}
            onChange={onSwitchPersona}
            options={(personas.data?.items ?? []).map((p) => ({
              value: p.id,
              label: p.name,
            }))}
            emptyLabel="Select a persona…"
            disabled={updateAgent.isPending}
          />
          <p className="label" style={{ margin: 0, color: 'var(--ink-3)' }}>
            Personas are shared objects. Switching only re-points this agent;
            the previous persona is kept for other agents that reference it.
          </p>
          {refError ? (
            <p className="label" style={{ margin: 0, color: 'var(--signal)' }}>
              {refError}
            </p>
          ) : null}
        </div>
      </Panel>

      {personaId ? (
        existing.isLoading ? (
          <Panel title="Persona Content" className="cropped">
            <p className="label">Loading persona…</p>
          </Panel>
        ) : existing.error instanceof ApiError &&
          existing.error.status === 404 ? (
          <Panel title="Persona Content" className="cropped">
            <p className="label" style={{ color: 'var(--signal)' }}>
              Persona not found (id: {personaId}).
            </p>
          </Panel>
        ) : existing.data ? (
          <Panel title="Persona Content" className="cropped">
            <form onSubmit={onSubmit} noValidate style={{ display: 'grid', gap: 14 }}>
              <Field label="Name" htmlFor="persona-name" error={errors.name?.message}>
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
                Saving updates the shared persona for every agent that references
                it.
              </p>
              {saveError ? (
                <p className="label" style={{ margin: 0, color: 'var(--signal)' }}>
                  {saveError}
                </p>
              ) : null}
              <div>
                <Button type="submit" variant="primary" disabled={isSubmitting}>
                  {isSubmitting ? 'Saving…' : 'Save persona'}
                </Button>
              </div>
            </form>
          </Panel>
        ) : null
      ) : null}
    </div>
  )
}
