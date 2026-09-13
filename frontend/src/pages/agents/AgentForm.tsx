import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useAgent, useUpdateAgent } from '../../api/agents'
import { useAgentImages } from '../../api/agentImages'
import { usePersonas } from '../../api/personas'
import { ApiError } from '../../api/client'
import { Button } from '../../primitives/Button'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Select } from '../../primitives/Select'
import { Textarea } from '../../primitives/Textarea'
import { Titleblock } from '../../layout/Titleblock'
import {
  API_KEY_LABELS,
  LLM_PROVIDERS,
  agentDefaults,
  agentSchema,
  buildAgentRequest,
  type AgentFormInput,
  type AgentFormValues,
} from './agentFormModel'

/**
 * The single-page edit form for an existing agent.
 *
 * Creation lives in the step-by-step {@link AgentWizard}; editing keeps every
 * field on one screen because jumping straight to a value is the point. The
 * two share their schema and request builder so they cannot drift.
 *
 * Anything agent-scoped and tab-shaped — persona content, runtime image,
 * tools, skills, MCP servers, environment — is edited on the agent's detail
 * tabs; this form covers identity, model, persona link and scaling.
 */
export function AgentForm() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [submitError, setSubmitError] = useState<string | null>(null)

  const existing = useAgent(id)
  const images = useAgentImages({ pageSize: 200 })
  const personas = usePersonas({ pageSize: 200 })
  const update = useUpdateAgent()

  const {
    register,
    handleSubmit,
    reset,
    setValue,
    watch,
    formState: { errors, isSubmitting },
  } = useForm<AgentFormInput, unknown, AgentFormValues>({
    resolver: zodResolver(agentSchema),
    defaultValues: agentDefaults,
  })

  useEffect(() => {
    if (existing.data) {
      reset({
        name: existing.data.name,
        description: existing.data.description ?? '',
        imageRef: { name: existing.data.imageRef?.name ?? '' },
        llm: {
          model: existing.data.llm?.model ?? '',
          provider: existing.data.llm?.provider ?? '',
          maxTurns: existing.data.llm?.maxTurns,
          temperature: existing.data.llm?.temperature,
        },
        customProviderUrl: existing.data.customProvider?.url ?? '',
        providerApiKey: '',
        persona: { id: existing.data.persona?.id ?? '' },
        replicas: existing.data.replicas ?? 1,
      })
    }
  }, [existing.data, reset])

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null)
    try {
      const saved = await update.mutateAsync({
        id: id!,
        body: buildAgentRequest(values, { isEdit: true }),
      })
      navigate(`/agents/${encodeURIComponent(saved.id)}`, { replace: true })
    } catch (err) {
      if (err instanceof ApiError) setSubmitError(err.message)
      else setSubmitError('Save failed. Please try again.')
    }
  })

  const provider = watch('llm.provider')

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Fleet / <Link to="/agents">Agents</Link> / <b>Edit</b>
          </>
        }
        title={
          <>
            Edit <em>Agent</em>
          </>
        }
        actions={
          <>
            <Button onClick={() => navigate(-1)}>Cancel</Button>
            <Button
              type="submit"
              variant="primary"
              form="agent-form"
              disabled={isSubmitting}
            >
              {isSubmitting ? 'Saving…' : 'Save'}
            </Button>
          </>
        }
      />
      <form
        id="agent-form"
        onSubmit={onSubmit}
        noValidate
        style={{ padding: '28px 32px', maxWidth: 920 }}
      >
        {submitError ? (
          <div
            role="alert"
            style={{
              padding: '10px 12px',
              border: '1.5px solid var(--signal)',
              background: 'var(--signal-haze)',
              color: 'var(--signal)',
              fontFamily: 'var(--mono)',
              fontSize: 11,
              marginBottom: 20,
            }}
          >
            {submitError}
          </div>
        ) : null}

        <section className="form-section">
          <header>
            <span className="idx">§01</span>
            <h3>General</h3>
          </header>
          <div className="form-grid">
            <Field label="Name" htmlFor="name" error={errors.name?.message}>
              <Input id="name" autoFocus {...register('name')} />
            </Field>
            <Field label="Description" htmlFor="description">
              <Textarea id="description" rows={2} {...register('description')} />
            </Field>
          </div>
        </section>

        <section className="form-section">
          <header>
            <span className="idx">§02</span>
            <h3>Image &amp; Tools</h3>
          </header>
          <div className="form-grid">
            <Field
              label="Image"
              htmlFor="imageRef.name"
              error={errors.imageRef?.name?.message}
            >
              <Select
                id="imageRef.name"
                value={watch('imageRef.name')}
                onChange={(v) =>
                  setValue('imageRef.name', v, { shouldValidate: true, shouldDirty: true })
                }
                options={(images.data?.items ?? []).map((i) => ({
                  value: i.id,
                  label: i.displayName ?? i.id,
                }))}
                emptyLabel="Select an image…"
              />
              {/* Hidden input keeps RHF in sync with the controlled Select */}
              <input type="hidden" {...register('imageRef.name')} />
            </Field>
          </div>
          <p className="label" style={{ marginTop: 12, color: 'var(--ink-3)' }}>
            Tools, skills, MCP servers and environment are edited per agent on
            the agent's Tools, Skills and Runtime tabs.
          </p>
        </section>

        <section className="form-section">
          <header>
            <span className="idx">§03</span>
            <h3>LLM</h3>
          </header>
          <div className="form-grid cols-3">
            <Field label="Model" htmlFor="llm.model" error={errors.llm?.model?.message}>
              <Input id="llm.model" {...register('llm.model')} />
            </Field>
            <Field label="Max Turns" htmlFor="llm.maxTurns">
              <Input
                id="llm.maxTurns"
                type="number"
                min={1}
                {...register('llm.maxTurns')}
              />
            </Field>
            <Field label="Temperature" htmlFor="llm.temperature">
              <Input
                id="llm.temperature"
                type="number"
                step={0.1}
                min={0}
                max={2}
                {...register('llm.temperature')}
              />
            </Field>
          </div>
          <div className="form-grid" style={{ marginTop: 16 }}>
            <Field label="Provider" htmlFor="llm.provider">
              <Select
                id="llm.provider"
                value={provider ?? ''}
                onChange={(v) => setValue('llm.provider', v, { shouldDirty: true })}
                options={LLM_PROVIDERS}
              />
              <input type="hidden" {...register('llm.provider')} />
            </Field>
            {provider ? (
              <>
                {provider === 'custom' ? (
                  <Field
                    label="Provider Base URL"
                    htmlFor="customProviderUrl"
                    error={errors.customProviderUrl?.message}
                  >
                    <Input
                      id="customProviderUrl"
                      {...register('customProviderUrl')}
                      placeholder="https://api.example.com/v1"
                    />
                  </Field>
                ) : null}
                <Field
                  label={API_KEY_LABELS[provider] ?? 'API Key'}
                  htmlFor="providerApiKey"
                >
                  <Input
                    id="providerApiKey"
                    type="password"
                    autoComplete="off"
                    placeholder="Leave blank to keep existing key"
                    {...register('providerApiKey')}
                  />
                </Field>
              </>
            ) : null}
          </div>
        </section>

        <section className="form-section">
          <header>
            <span className="idx">§04</span>
            <h3>Persona</h3>
          </header>
          <Field
            label="Persona"
            htmlFor="persona.id"
            error={errors.persona?.id?.message}
          >
            <Select
              id="persona.id"
              value={watch('persona.id') ?? ''}
              onChange={(v) =>
                setValue('persona.id', v, { shouldValidate: true, shouldDirty: true })
              }
              options={(personas.data?.items ?? []).map((p) => ({
                value: p.id,
                label: p.name,
              }))}
              emptyLabel="Select a persona…"
            />
            <input type="hidden" {...register('persona.id')} />
            <p className="label" style={{ marginTop: 8, color: 'var(--ink-3)' }}>
              No persona yet? <Link to="/personas/new">Create one</Link>.
            </p>
          </Field>
        </section>

        <section className="form-section">
          <header>
            <span className="idx">§05</span>
            <h3>Scaling</h3>
          </header>
          <div className="form-grid">
            <Field label="Replicas" htmlFor="replicas">
              <Input
                id="replicas"
                type="number"
                min={0}
                {...register('replicas')}
              />
            </Field>
          </div>
        </section>
      </form>
    </>
  )
}
