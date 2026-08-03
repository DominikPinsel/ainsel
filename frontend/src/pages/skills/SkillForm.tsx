import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ApiError } from '../../api/client'
import { useSkill, useCreateSkill, useUpdateSkill, type SkillRequest } from '../../api/skills'
import { Button } from '../../primitives/Button'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Textarea } from '../../primitives/Textarea'
import { Titleblock } from '../../layout/Titleblock'
import { GroupField } from '../../components/GroupField'

const slugPattern = /^[a-z0-9]+(-[a-z0-9]+)*$/

const schema = z.object({
  id: z
    .string()
    .min(1, 'ID is required')
    .max(64, 'ID must be <= 64 chars')
    .regex(
      slugPattern,
      'Lowercase alphanumeric with hyphens, no leading/trailing/consecutive hyphens',
    ),
  name: z.string().min(1, 'Name is required').max(200),
  description: z.string().max(2000).optional().default(''),
  body: z.string().max(100_000).optional().default(''),
  groupId: z.string().optional(),
  tags: z
    .string()
    .max(600)
    .optional()
    .default('')
    // Mirror the backend rules (services/hub/internal/skills/service.go):
    // <= 10 tags and <= 50 chars each, counted after trim/lowercase/dedupe so
    // the form rejects input that would otherwise hit a confusing HTTP 400.
    .superRefine((val, ctx) => {
      const tags = new Set(
        val
          .split(',')
          .map((t) => t.trim().toLowerCase())
          .filter(Boolean),
      )
      if (tags.size > 10) {
        ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'Must have <= 10 tags' })
        return
      }
      for (const t of tags) {
        if (t.length > 50) {
          ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'Each tag must be <= 50 chars' })
          return
        }
      }
    }),
})

type FormValues = z.infer<typeof schema>

export function SkillForm() {
  const { id } = useParams<{ id: string }>()
  const isEdit = id !== undefined
  const navigate = useNavigate()
  const [submitError, setSubmitError] = useState<string | null>(null)
  const existing = useSkill(isEdit ? id : undefined)
  const create = useCreateSkill()
  const update = useUpdateSkill()

  const {
    register,
    handleSubmit,
    setError,
    reset,
    watch,
    setValue,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { id: '', name: '', description: '', body: '', tags: '', groupId: '' },
  })

  useEffect(() => {
    if (isEdit && existing.data) {
      reset({
        id: existing.data.id,
        name: existing.data.name,
        description: existing.data.description,
        body: existing.data.body,
        tags: (existing.data.tags ?? []).join(', '),
      })
    }
  }, [isEdit, existing.data, reset])

  const parseTags = (raw: string): string[] =>
    raw
      .split(',')
      .map((t) => t.trim().toLowerCase())
      .filter(Boolean)

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null)
    if (!isEdit && !values.groupId) {
      setError('groupId', { message: 'Group is required' })
      return
    }
    const tags = parseTags(values.tags)
    try {
      if (isEdit) {
        const body: SkillRequest = {
          name: values.name,
          description: values.description || '',
          body: values.body || '',
          tags,
        }
        await update.mutateAsync({ id: id!, body })
        navigate(`/skills/${encodeURIComponent(id!)}`, { replace: true })
      } else {
        const body: SkillRequest = {
          id: values.id,
          name: values.name,
          description: values.description || '',
          body: values.body || '',
          tags,
          groupId: values.groupId,
        }
        const saved = await create.mutateAsync(body)
        navigate(`/skills/${encodeURIComponent(saved.id)}`, { replace: true })
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setError('id', {
          type: 'server',
          message: `A skill with ID "${values.id}" already exists`,
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
            Fleet / <Link to="/skills">Skills</Link> / <b>{isEdit ? 'Edit' : 'New'}</b>
          </>
        }
        title={
          <>
            {isEdit ? 'Edit' : 'New'} <em>Skill</em>
          </>
        }
        actions={
          <>
            <Button onClick={() => navigate(-1)}>Cancel</Button>
            <Button type="submit" variant="primary" form="skill-form" disabled={isSubmitting}>
              {isSubmitting ? 'Saving…' : isEdit ? 'Save' : 'Create'}
            </Button>
          </>
        }
      />
      <form
        id="skill-form"
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
            <Field
              label="ID"
              htmlFor="id"
              error={errors.id?.message}
              hint={isEdit ? 'The ID cannot be changed after creation.' : undefined}
            >
              <Input
                id="id"
                autoFocus={!isEdit}
                disabled={isEdit}
                placeholder="code-review"
                {...register('id')}
              />
            </Field>
            <Field label="Name" htmlFor="name" error={errors.name?.message}>
              <Input id="name" autoFocus={isEdit} placeholder="Code Review" {...register('name')} />
            </Field>
            <Field label="Description" htmlFor="description" error={errors.description?.message}>
              <Input
                id="description"
                placeholder="What this skill does"
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
            <Field
              label="Tags"
              htmlFor="tags"
              error={errors.tags?.message}
              hint="Comma-separated tags for filtering, e.g. review, security, go"
            >
              <Input
                id="tags"
                placeholder="review, security, go"
                {...register('tags')}
              />
            </Field>
          </div>
        </section>

        <section className="form-section">
          <header>
            <span className="idx">§02</span>
            <h3>Skill Body</h3>
          </header>
          <Field
            label="Skill body (Markdown)"
            htmlFor="body"
            error={errors.body?.message}
            hint="Markdown instructions the agent loads as a slash command / appended skill."
          >
            <Textarea
              id="body"
              rows={20}
              placeholder={'# Code Review\n\nWhen invoked, review the PR for…'}
              {...register('body')}
            />
          </Field>
        </section>
      </form>
    </>
  )
}
