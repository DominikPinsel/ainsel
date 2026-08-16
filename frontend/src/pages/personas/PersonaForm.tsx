import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  usePersona,
  useCreatePersona,
  useUpdatePersona,
  type PersonaRequest,
} from '../../api/personas'
import { ApiError } from '../../api/client'
import { Button } from '../../primitives/Button'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Textarea } from '../../primitives/Textarea'
import { Titleblock } from '../../layout/Titleblock'
import { GroupField } from '../../components/GroupField'

const schema = z.object({
  name: z.string().min(1, 'Name is required').max(200),
  description: z.string().max(2000).optional().default(''),
  text: z.string().min(1, 'Persona text is required').max(100_000),
  groupId: z.string().optional(),
})

// zod 4 tracks input/output types separately (defaults & coercions make
// inputs optional); useForm needs both so the resolver types line up.
type FormInput = z.input<typeof schema>
type FormValues = z.infer<typeof schema>

export function PersonaForm() {
  const { id } = useParams<{ id: string }>()
  const isEdit = id !== undefined
  const navigate = useNavigate()
  const [submitError, setSubmitError] = useState<string | null>(null)
  const existing = usePersona(isEdit ? id : undefined)
  const create = useCreatePersona()
  const update = useUpdatePersona()

  const {
    register,
    handleSubmit,
    setError,
    reset,
    watch,
    setValue,
    formState: { errors, isSubmitting },
  } = useForm<FormInput, unknown, FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', description: '', text: '', groupId: '' },
  })

  useEffect(() => {
    if (isEdit && existing.data) {
      reset({
        name: existing.data.name,
        description: existing.data.description,
        text: existing.data.text,
      })
    }
  }, [isEdit, existing.data, reset])

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null)
    if (!isEdit && !values.groupId) {
      setError('groupId', { message: 'Group is required' })
      return
    }
    const body: PersonaRequest = {
      name: values.name,
      description: values.description || '',
      text: values.text,
      groupId: isEdit ? undefined : values.groupId,
    }
    try {
      const saved = isEdit
        ? await update.mutateAsync({ id: id!, body })
        : await create.mutateAsync(body)
      navigate(`/personas/${encodeURIComponent(saved.id)}`, { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setError('name', {
          type: 'server',
          message: `A persona named "${values.name}" already exists`,
        })
        return
      }
      if (err instanceof ApiError) setSubmitError(err.message)
      else setSubmitError('Save failed. Please try again.')
    }
  })

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Fleet / <Link to="/personas">Personas</Link> /{' '}
            <b>{isEdit ? 'Edit' : 'New'}</b>
          </>
        }
        title={
          <>
            {isEdit ? 'Edit' : 'New'} <em>Persona</em>
          </>
        }
        actions={
          <>
            <Button onClick={() => navigate(-1)}>Cancel</Button>
            <Button
              type="submit"
              variant="primary"
              form="persona-form"
              disabled={isSubmitting}
            >
              {isSubmitting ? 'Saving…' : isEdit ? 'Save' : 'Create'}
            </Button>
          </>
        }
      />
      <form
        id="persona-form"
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
            <h3>Identity</h3>
          </header>
          <div className="form-grid">
            <Field label="Name" htmlFor="name" error={errors.name?.message}>
              <Input
                id="name"
                autoFocus
                placeholder="code-reviewer"
                {...register('name')}
              />
            </Field>
            <Field
              label="Description"
              htmlFor="description"
              error={errors.description?.message}
            >
              <Input
                id="description"
                placeholder="What this persona is for"
                {...register('description')}
              />
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
            <h3>Persona Text</h3>
          </header>
          <Field
            label="Persona text"
            htmlFor="text"
            error={errors.text?.message}
          >
            <Textarea
              id="text"
              rows={20}
              placeholder="# Persona&#10;&#10;You are…"
              {...register('text')}
            />
          </Field>
        </section>
      </form>
    </>
  )
}
