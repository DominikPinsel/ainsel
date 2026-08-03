import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  useMCPServer,
  useCreateMCPServer,
  useUpdateMCPServer,
  useDeleteMCPServer,
  type MCPServerRequest,
  type MCPServerUpdateRequest,
} from '../../api/mcpServers'
import { ApiError } from '../../api/client'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Panel } from '../../primitives/Panel'
import { Titleblock } from '../../layout/Titleblock'
import { GroupField } from '../../components/GroupField'

const schema = z.object({
  name: z.string().min(1, 'Name is required').regex(/^[a-z0-9-]+$/, 'Only lowercase letters, digits and hyphens'),
  displayName: z.string().min(1, 'Display name is required'),
  description: z.string(),
  url: z.string().min(1, 'URL is required'),
  groupId: z.string().optional(),
  tokenFromEnv: z
    .string()
    .max(64, 'At most 64 characters')
    .refine((v) => v === '' || /^[A-Z_][A-Z0-9_]*$/.test(v), 'Use uppercase letters, digits, and underscores (e.g. FORGEJO_PAT)'),
})

type FormValues = z.infer<typeof schema>

export function MCPServerForm() {
  const { name: paramName } = useParams<{ name?: string }>()
  const isEdit = paramName !== undefined
  const navigate = useNavigate()
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)

  const existing = useMCPServer(isEdit ? paramName : undefined)
  const create = useCreateMCPServer()
  const update = useUpdateMCPServer()
  const remove = useDeleteMCPServer()

  const {
    register,
    handleSubmit,
    reset,
    setError,
    watch,
    setValue,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', displayName: '', description: '', url: '', tokenFromEnv: '', groupId: '' },
  })

  useEffect(() => {
    if (isEdit && existing.data) {
      reset({
        name: existing.data.name,
        displayName: existing.data.displayName,
        description: existing.data.description ?? '',
        url: existing.data.url,
        tokenFromEnv: existing.data.tokenFromEnv ?? '',
      })
    }
  }, [isEdit, existing.data, reset])

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null)
    if (!isEdit && !values.groupId) {
      setError('groupId', { message: 'Group is required' })
      return
    }
    try {
      if (isEdit) {
        const body: MCPServerUpdateRequest = {
          displayName: values.displayName,
          description: values.description || undefined,
          url: values.url,
          tokenFromEnv: values.tokenFromEnv || undefined,
        }
        await update.mutateAsync({ name: paramName!, body })
      } else {
        const body: MCPServerRequest = {
          name: values.name,
          displayName: values.displayName,
          description: values.description || undefined,
          url: values.url,
          tokenFromEnv: values.tokenFromEnv || undefined,
          groupId: values.groupId,
        }
        await create.mutateAsync(body)
      }
      navigate('/settings/mcp-servers', { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setSubmitError(`An MCP server named "${values.name}" already exists.`)
        return
      }
      if (err instanceof ApiError) setSubmitError(err.message)
      else setSubmitError('Save failed. Please try again.')
    }
  })

  const onConfirmDelete = async () => {
    if (!paramName) return
    try {
      await remove.mutateAsync(paramName)
      setConfirmOpen(false)
      navigate('/settings/mcp-servers', { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setSubmitError('This MCP server is referenced by one or more agent images and cannot be deleted.')
      } else if (err instanceof ApiError) {
        setSubmitError(err.message)
      } else {
        setSubmitError('Delete failed. Please try again.')
      }
      setConfirmOpen(false)
    }
  }

  const server = existing.data

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Setup / <Link to="/settings">Settings</Link> /{' '}
            <Link to="/settings/mcp-servers">MCP Servers</Link> /{' '}
            <b>{isEdit ? server?.displayName ?? paramName : 'New'}</b>
          </>
        }
        title={
          <>
            {isEdit ? server?.displayName ?? <em>Server</em> : 'New '}
            {!isEdit ? <em>MCP Server</em> : null}
          </>
        }
        actions={
          <>
            <Button onClick={() => navigate(-1)}>Cancel</Button>
            <Button type="submit" variant="primary" form="mcp-server-form" disabled={isSubmitting}>
              {isSubmitting ? 'Saving…' : isEdit ? 'Save' : 'Create'}
            </Button>
            {isEdit ? (
              <Button variant="danger" onClick={() => setConfirmOpen(true)}>
                Delete
              </Button>
            ) : null}
          </>
        }
      />
      <form
        id="mcp-server-form"
        onSubmit={onSubmit}
        noValidate
        style={{ padding: '28px 32px', maxWidth: 700 }}
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

        <Panel title="Server">
          <div style={{ display: 'grid', gap: 14 }}>
            <Field label="Name" htmlFor="name" error={errors.name?.message}>
              <Input
                id="name"
                placeholder="my-mcp-server"
                disabled={isEdit}
                {...register('name')}
              />
            </Field>
            <Field label="Display Name" htmlFor="displayName" error={errors.displayName?.message}>
              <Input id="displayName" {...register('displayName')} />
            </Field>
            {!isEdit ? (
              <GroupField
                value={watch('groupId') ?? ''}
                onChange={(v) => setValue('groupId', v, { shouldDirty: true })}
                error={errors.groupId?.message}
              />
            ) : null}
            <Field label="Description" htmlFor="description">
              <Input id="description" {...register('description')} />
            </Field>
            <Field label="URL" htmlFor="url" error={errors.url?.message}>
              <Input
                id="url"
                placeholder="https://mcp.example.com/sse"
                {...register('url')}
              />
            </Field>
            <Field
              label="Token env variable"
              htmlFor="tokenFromEnv"
              hint="Name of the env var on the agent pod whose value is sent as the bearer token. The value itself is set on each AgentImage that uses this MCP server. Leave empty for an anonymous MCP server."
              error={errors.tokenFromEnv?.message}
            >
              <Input
                id="tokenFromEnv"
                placeholder="FORGEJO_PAT"
                {...register('tokenFromEnv')}
              />
            </Field>
          </div>
        </Panel>
      </form>

      <ConfirmModal
        open={confirmOpen}
        title="Delete MCP server?"
        body={
          <>
            <b>{server?.displayName ?? paramName}</b> will be permanently removed. Agent images
            referencing this server will need to be updated.
          </>
        }
        confirmLabel={remove.isPending ? 'Deleting…' : 'Delete'}
        destructive
        onConfirm={onConfirmDelete}
        onCancel={() => setConfirmOpen(false)}
      />
    </>
  )
}
