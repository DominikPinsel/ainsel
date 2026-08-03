import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  useAgent,
  useCreateAgent,
  useUpdateAgent,
  type AgentRequest,
  type AgentOllamaCloud,
  type AgentOpenCode,
  type AgentAlibabaCloud,
  type AgentCustomProvider,
} from '../../api/agents'
import { useAgentImages } from '../../api/agentImages'
import { usePersonas } from '../../api/personas'
import { ApiError } from '../../api/client'
import { Button } from '../../primitives/Button'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Select } from '../../primitives/Select'
import { Textarea } from '../../primitives/Textarea'
import { Titleblock } from '../../layout/Titleblock'
import { GroupField } from '../../components/GroupField'

const LLM_PROVIDERS = [
  { value: '', label: 'None' },
  { value: 'ollama-cloud', label: 'Ollama Cloud' },
  { value: 'opencode', label: 'OpenCode' },
  { value: 'alibaba-cloud', label: 'Alibaba Token Plan' },
  { value: 'custom', label: 'Custom' },
] as const

const API_KEY_LABELS: Record<string, string> = {
  'ollama-cloud': 'Ollama Cloud API Key',
  opencode: 'OpenCode API Key',
  'alibaba-cloud': 'Alibaba Token Plan API Key',
  custom: 'API Key',
}

const schema = z
  .object({
    name: z.string().min(1, 'Name is required'),
    description: z.string().optional(),
    imageRef: z.object({ name: z.string().min(1, 'Image is required') }),
    llm: z.object({
      model: z.string().min(1, 'Model is required'),
      provider: z.string().optional(),
      maxTurns: z.coerce.number().int().min(1).optional(),
      temperature: z.coerce.number().min(0).max(2).optional(),
    }),
    providerApiKey: z.string().optional(),
    customProviderUrl: z.string().optional(),
    persona: z.object({ id: z.string().min(1, 'Persona is required') }),
    replicas: z.coerce.number().int().min(0).optional(),
    groupId: z.string().optional(),
  })
  .superRefine((data, ctx) => {
    if (data.llm.provider === 'custom' && !data.customProviderUrl?.trim()) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['customProviderUrl'],
        message: 'URL is required for custom providers',
      })
    }
  })

type FormValues = z.infer<typeof schema>

export function AgentForm() {
  const { id } = useParams<{ id: string }>()
  const isEdit = id !== undefined
  const navigate = useNavigate()
  const [submitError, setSubmitError] = useState<string | null>(null)

  const existing = useAgent(isEdit ? id : undefined)
  const images = useAgentImages({ pageSize: 200 })
  const personas = usePersonas({ pageSize: 200 })
  const create = useCreateAgent()
  const update = useUpdateAgent()

  const {
    register,
    handleSubmit,
    reset,
    setValue,
    watch,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      description: '',
      imageRef: { name: '' },
      llm: { model: '', provider: 'ollama-cloud', maxTurns: 100, temperature: 1 },
      customProviderUrl: '',
      providerApiKey: '',
      persona: { id: '' },
      replicas: 1,
      groupId: '',
    },
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

    if (!isEdit && !values.groupId) {
      setError('groupId', { message: 'Group is required' })
      return
    }

    // Use `watch()` to explicitly read the current state of the provider,
    // URL, and API Key fields from the DOM. This ensures we get the
    // user's latest inputs exactly as they are in the UI.
    const activeProvider = watch('llm.provider')
    const activeUrl = watch('customProviderUrl')
    const activeApiKey = watch('providerApiKey')

    const ollamaCloud:
      | AgentOllamaCloud
      | undefined = activeProvider === 'ollama-cloud'
      ? activeApiKey
        ? { apiKey: activeApiKey }
        : undefined
      : undefined

    const openCode:
      | AgentOpenCode
      | undefined = activeProvider === 'opencode'
      ? activeApiKey
        ? { apiKey: activeApiKey }
        : undefined
      : undefined

    const alibabaCloud:
      | AgentAlibabaCloud
      | undefined = activeProvider === 'alibaba-cloud'
      ? activeApiKey
        ? { apiKey: activeApiKey }
        : undefined
      : undefined

    const customProvider:
      | AgentCustomProvider
      | undefined = activeProvider === 'custom'
      ? {
          url: activeUrl || '',
          ...(activeApiKey ? { apiKey: activeApiKey } : {}),
        }
      : undefined

    const body: AgentRequest = {
      name: values.name,
      description: values.description || undefined,
      imageRef: values.imageRef,
      llm: values.llm,
      ollamaCloud,
      openCode,
      alibabaCloud,
      customProvider,
      persona: { id: values.persona.id },
      replicas: values.replicas,
      groupId: isEdit ? undefined : values.groupId,
    }
    try {
      const saved = isEdit
        ? await update.mutateAsync({ id: id!, body })
        : await create.mutateAsync(body)
      navigate(`/agents/${encodeURIComponent(saved.id)}`, { replace: true })
    } catch (err) {
      if (err instanceof ApiError) setSubmitError(err.message)
      else setSubmitError('Save failed. Please try again.')
    }
  })

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Fleet / <Link to="/agents">Agents</Link> /{' '}
            <b>{isEdit ? 'Edit' : 'New'}</b>
          </>
        }
        title={
          <>
            {isEdit ? 'Edit' : 'New'} <em>Agent</em>
          </>
        }
        actions={
          <>
            <Button onClick={() => navigate(-1)}>Cancel</Button>
            <Button type="submit" variant="primary" form="agent-form" disabled={isSubmitting}>
              {isSubmitting ? 'Saving…' : isEdit ? 'Save' : 'Create'}
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
            {!isEdit ? (
              <GroupField
                value={watch('groupId') ?? ''}
                onChange={(v) => setValue('groupId', v, { shouldDirty: true })}
                error={errors.groupId?.message}
              />
            ) : null}
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
            Tool enablement uses every tool defined on the selected image.
            Per-agent overrides land in a follow-up.
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
                value={watch('llm.provider') ?? ''}
                onChange={(v) => setValue('llm.provider', v, { shouldDirty: true })}
                options={LLM_PROVIDERS}
              />
              <input type="hidden" {...register('llm.provider')} />
            </Field>
            {watch('llm.provider') ? (
              <>
                {watch('llm.provider') === 'custom' ? (
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
                  label={API_KEY_LABELS[watch('llm.provider') ?? ''] ?? 'API Key'}
                  htmlFor="providerApiKey"
                >
                  <Input
                    id="providerApiKey"
                    type="password"
                    autoComplete="off"
                    placeholder={isEdit ? 'Leave blank to keep existing key' : ''}
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