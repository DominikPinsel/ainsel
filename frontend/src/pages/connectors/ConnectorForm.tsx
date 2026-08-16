import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../../api/client'
import {
  useConnector,
  useCreateConnector,
  useUpdateConnector,
  type ConnectorResponse,
} from '../../api/connectors'
import { Button } from '../../primitives/Button'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Select } from '../../primitives/Select'
import { Titleblock } from '../../layout/Titleblock'
import { copy } from '../../utils/clipboard'
import { Tag } from '../../primitives/Tag'
import { GroupField } from '../../components/GroupField'

const PRESET_OPTIONS = [
  { value: 'X-Hub-Signature-256', label: 'GitHub (X-Hub-Signature-256)' },
  { value: 'X-Forgejo-Signature', label: 'Forgejo (X-Forgejo-Signature)' },
  { value: 'custom', label: 'Custom…' },
] as const

const schema = z.object({
  name: z.string().min(1, 'Name is required'),
  signatureHeaderPreset: z.string().default('X-Hub-Signature-256'),
  signatureHeaderCustom: z.string().optional(),
  groupId: z.string().optional(),
})

// zod 4 tracks input/output types separately (defaults make inputs
// optional); useForm needs both so the resolver types line up.
type FormInput = z.input<typeof schema>
type FormValues = z.infer<typeof schema>

export function ConnectorForm() {
  const { id } = useParams<{ id: string }>()
  const isEdit = id !== undefined
  const navigate = useNavigate()
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [created, setCreated] = useState<ConnectorResponse | null>(null)

  const existing = useConnector(isEdit ? id : undefined)
  const create = useCreateConnector()
  const update = useUpdateConnector()

  const {
    register,
    handleSubmit,
    reset,
    watch,
    setValue,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<FormInput, unknown, FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      signatureHeaderPreset: 'X-Hub-Signature-256',
      signatureHeaderCustom: '',
      groupId: '',
    },
  })

  const signatureHeaderPreset = watch('signatureHeaderPreset') ?? 'X-Hub-Signature-256'

  useEffect(() => {
    if (existing.data) {
      const preset = PRESET_OPTIONS.find(
        (o) => o.value !== 'custom' && o.value === existing.data!.signatureHeader,
      )
      reset({
        name: existing.data.name,
        signatureHeaderPreset: preset ? preset.value : 'custom',
        signatureHeaderCustom: preset ? '' : (existing.data.signatureHeader ?? ''),
      })
    }
  }, [existing.data, reset])

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null)
    if (!isEdit && !values.groupId) {
      setError('groupId', { message: 'Group is required' })
      return
    }
    const signatureHeader =
      values.signatureHeaderPreset === 'custom'
        ? (values.signatureHeaderCustom ?? '')
        : values.signatureHeaderPreset

    try {
      if (isEdit) {
        await update.mutateAsync({
          id: id!,
          body: { name: values.name },
        })
        navigate(`/connectors/${encodeURIComponent(id!)}`, { replace: true })
      } else {
        const saved = await create.mutateAsync({
          name: values.name,
          signatureHeader,
          groupId: values.groupId,
        })
        setCreated(saved)
      }
    } catch (err) {
      if (err instanceof ApiError) setSubmitError(err.message)
      else setSubmitError('Save failed. Please try again.')
    }
  })

  if (created) {
    return (
      <>
        <Titleblock
          crumbs={
            <>
              Fleet / <Link to="/connectors">Connectors</Link> / <b>New</b>
            </>
          }
          title={
            <>
              New <em>Connector</em>
            </>
          }
          actions={
            <Button
              variant="primary"
              onClick={() =>
                navigate(`/connectors/${encodeURIComponent(created.id)}`, { replace: true })
              }
            >
              Done
            </Button>
          }
        />
        <div style={{ padding: '28px 32px', maxWidth: 760 }}>
          <div
            style={{
              padding: '20px 24px',
              border: '1.5px solid var(--rule)',
              background: 'var(--paper-2)',
              display: 'grid',
              gap: 16,
            }}
          >
            <div
              style={{
                fontFamily: 'var(--mono)',
                fontSize: 11,
                letterSpacing: '0.12em',
                textTransform: 'uppercase',
                color: 'var(--ink-3)',
                marginBottom: 4,
              }}
            >
              Connector created — save these values now
            </div>

            <div
              role="alert"
              style={{
                padding: '10px 12px',
                border: '1.5px solid var(--signal)',
                background: 'var(--signal-haze)',
                color: 'var(--signal)',
                fontFamily: 'var(--mono)',
                fontSize: 11,
              }}
            >
              This secret will not be shown again. Copy it before leaving this page.
            </div>

            {created.webhookEndpoint ? (
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <span className="label" style={{ width: 90 }}>
                  Endpoint
                </span>
                <span className="num" style={{ flex: 1 }}>
                  {created.webhookEndpoint}
                </span>
                <Button
                  size="sm"
                  onClick={() => created.webhookEndpoint && copy(created.webhookEndpoint)}
                >
                  ⧉ Copy
                </Button>
              </div>
            ) : null}

            {created.webhookSecretValue ? (
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <span className="label" style={{ width: 90 }}>
                  Secret
                </span>
                <span className="num" style={{ flex: 1 }}>
                  {created.webhookSecretValue}
                </span>
                <Button
                  size="sm"
                  onClick={() =>
                    created.webhookSecretValue && copy(created.webhookSecretValue)
                  }
                >
                  ⧉ Copy
                </Button>
                <Tag variant="err">ONE-TIME</Tag>
              </div>
            ) : null}

            <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <Button
                variant="primary"
                onClick={() =>
                  navigate(`/connectors/${encodeURIComponent(created.id)}`, {
                    replace: true,
                  })
                }
              >
                Dismiss
              </Button>
            </div>
          </div>
        </div>
      </>
    )
  }

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Fleet / <Link to="/connectors">Connectors</Link> /{' '}
            <b>{isEdit ? 'Edit' : 'New'}</b>
          </>
        }
        title={
          <>
            {isEdit ? 'Edit' : 'New'} <em>Connector</em>
          </>
        }
        actions={
          <>
            <Button onClick={() => navigate(-1)}>Cancel</Button>
            <Button
              type="submit"
              variant="primary"
              form="connector-form"
              disabled={isSubmitting}
            >
              {isSubmitting ? 'Saving…' : isEdit ? 'Save' : 'Create'}
            </Button>
          </>
        }
      />
      <form
        id="connector-form"
        onSubmit={onSubmit}
        noValidate
        style={{ padding: '28px 32px', maxWidth: 760 }}
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
            <h3>Signature Header</h3>
          </header>
          <div className="form-grid">
            <Field label="Signature Header" htmlFor="signatureHeaderPreset">
              <Select
                id="signatureHeaderPreset"
                value={signatureHeaderPreset}
                onChange={(v) =>
                  setValue('signatureHeaderPreset', v, { shouldValidate: true })
                }
                options={PRESET_OPTIONS}
              />
              <input type="hidden" {...register('signatureHeaderPreset')} />
            </Field>

            {signatureHeaderPreset === 'custom' ? (
              <Field
                label="Custom Header Name"
                htmlFor="signatureHeaderCustom"
                error={errors.signatureHeaderCustom?.message}
              >
                <Input
                  id="signatureHeaderCustom"
                  placeholder="e.g. X-My-Signature"
                  {...register('signatureHeaderCustom')}
                />
              </Field>
            ) : null}
          </div>
        </section>
      </form>
    </>
  )
}
