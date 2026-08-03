import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  useGroup,
  useCreateGroup,
  useUpdateGroup,
} from '../../api/groups'
import { ApiError } from '../../api/client'
import { Button } from '../../primitives/Button'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Textarea } from '../../primitives/Textarea'
import { Titleblock } from '../../layout/Titleblock'

const schema = z.object({
  name: z.string().min(1, 'Name is required'),
  description: z.string().optional(),
})

type FormValues = z.infer<typeof schema>

export function GroupForm() {
  const { id } = useParams<{ id: string }>()
  const isEdit = id !== undefined
  const navigate = useNavigate()
  const [submitError, setSubmitError] = useState<string | null>(null)

  const existing = useGroup(isEdit ? id : undefined)
  const create = useCreateGroup()
  const update = useUpdateGroup()

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', description: '' },
  })

  useEffect(() => {
    if (existing.data) {
      const g = existing.data.group
      reset({
        name: g.name,
        description: g.description ?? '',
      })
    }
  }, [existing.data, reset])

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null)
    try {
      if (isEdit) {
        await update.mutateAsync({
          id: id!,
          body: {
            name: values.name,
            description: values.description ?? '',
          },
        })
        navigate(`/groups/${encodeURIComponent(id!)}`, { replace: true })
      } else {
        const saved = await create.mutateAsync({
          name: values.name,
          description: values.description || undefined,
        })
        navigate(`/groups/${encodeURIComponent(saved.id)}`, { replace: true })
      }
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
            Admin / <Link to="/groups">Groups</Link> / <b>{isEdit ? 'Edit' : 'New'}</b>
          </>
        }
        title={
          <>
            {isEdit ? 'Edit' : 'New'} <em>Group</em>
          </>
        }
        actions={
          <>
            <Button onClick={() => navigate(-1)}>Cancel</Button>
            <Button type="submit" variant="primary" form="group-form" disabled={isSubmitting}>
              {isSubmitting ? 'Saving…' : isEdit ? 'Save' : 'Create'}
            </Button>
          </>
        }
      />
      <form
        id="group-form"
        onSubmit={onSubmit}
        noValidate
        style={{ padding: '28px 32px', maxWidth: 640 }}
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
      </form>
    </>
  )
}
