import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate } from 'react-router-dom'
import {
  useAgentImage,
  useCreateAgentImage,
  useDeleteAgentImage,
  useUpdateAgentImage,
  useRefreshMCPTools,
} from '../../api/agentImages'
import { ApiError } from '../../api/client'
import { imageSchema } from '../../schemas/images'
import type { ImageFormValues } from './ImageDetail.types'
import type { Source } from './ToolSourceSidebar'
import {
  DEFAULT_SHELL_TOOLS,
  buildRequestBody,
  envVarToFormValue,
  toFormTool,
} from './imageTransforms'
import { ImageFormView } from './ImageFormView'

type ImageFormContainerProps = {
  id: string | undefined
}

export function ImageFormContainer({ id }: ImageFormContainerProps) {
  const isEdit = id !== undefined
  const navigate = useNavigate()
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [mcpWarnings, setMcpWarnings] = useState<string[]>([])
  const [mcpRefreshResult, setMcpRefreshResult] = useState<string | null>(null)
  const [selectedIndex, setSelectedIndex] = useState<number | null>(0)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [activeSource, setActiveSource] = useState<Source>('all')
  const initialized = useRef(false)

  const existing = useAgentImage(isEdit ? id : undefined)
  const create = useCreateAgentImage()
  const update = useUpdateAgentImage()
  const remove = useDeleteAgentImage()
  const refreshMcp = useRefreshMCPTools()

  const {
    register,
    handleSubmit,
    reset,
    control,
    setValue,
    watch,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<ImageFormValues>({
    resolver: zodResolver(imageSchema),
    defaultValues: {
      displayName: '',
      description: '',
      imageURL: '',
      groupId: '',
      tools: DEFAULT_SHELL_TOOLS,
      env: [],
      mcpServers: [],
      enabledSkills: [],
    },
  })

  useEffect(() => {
    initialized.current = false
  }, [id])

  useEffect(() => {
    if (existing.data && !initialized.current) {
      reset({
        displayName: existing.data.displayName ?? '',
        description: existing.data.description ?? '',
        imageURL: existing.data.imageURL,
        tools: (existing.data.tools ?? []).map(toFormTool),
        env: (existing.data.env ?? []).map(envVarToFormValue),
        mcpServers: (existing.data.mcpServers ?? []).map((m) => m.name),
        enabledSkills: existing.data.enabledSkills ?? [],
      })
      setSelectedIndex((existing.data.tools ?? []).length > 0 ? 0 : null)
      initialized.current = true
    }
  }, [existing.data, reset])

  const onRefreshMCP = handleSubmit(async (values) => {
    if (!id) return
    setSubmitError(null)
    setMcpWarnings([])
    setMcpRefreshResult(null)
    try {
      await update.mutateAsync({ id, body: buildRequestBody(values) })
      const prevMcpTools = values.tools.filter((t) => t.kind === 'mcp')
      const result = await refreshMcp.mutateAsync(id)
      if (result.warnings?.length) setMcpWarnings(result.warnings)

      const nextMcpTools = (result.image.tools ?? []).filter((t) => t.kind === 'mcp')
      const prevKeys = new Set(prevMcpTools.map((t) => `${t.mcpSource}::${t.name}`))
      const nextKeys = new Set(nextMcpTools.map((t) => `${t.mcpSource}::${t.name}`))
      const added = nextMcpTools.filter((t) => !prevKeys.has(`${t.mcpSource}::${t.name}`))
      const removed = prevMcpTools.filter((t) => !nextKeys.has(`${t.mcpSource}::${t.name}`))

      if (added.length === 0 && removed.length === 0) {
        setMcpRefreshResult('MCP tools refreshed — no changes detected.')
      } else {
        const parts: string[] = []
        const countByServer = (tools: typeof added) => {
          const m = new Map<string, number>()
          tools.forEach((t) => {
            const s = t.mcpSource ?? 'unknown'
            m.set(s, (m.get(s) ?? 0) + 1)
          })
          return m
        }
        countByServer(added).forEach((n, s) => parts.push(`+${n} from ${s}`))
        countByServer(removed).forEach((n, s) => parts.push(`−${n} from ${s}`))
        setMcpRefreshResult(`MCP tools refreshed: ${parts.join(', ')}`)
      }

      const nextTools = (result.image.tools ?? []).map(toFormTool)
      reset({
        displayName: result.image.displayName ?? '',
        description: result.image.description ?? '',
        imageURL: result.image.imageURL,
        tools: nextTools,
        env: (result.image.env ?? []).map(envVarToFormValue),
        mcpServers: (result.image.mcpServers ?? []).map((m) => m.name),
        enabledSkills: result.image.enabledSkills ?? [],
      })
      setSelectedIndex(nextTools.length > 0 ? 0 : null)
    } catch (err) {
      if (err instanceof ApiError) setSubmitError(err.message)
      else setSubmitError('Refresh failed. Please try again.')
    }
  })

  const onConfirmDelete = async () => {
    if (!id) return
    await remove.mutateAsync(id)
    setConfirmOpen(false)
    navigate('/agent-images', { replace: true })
  }

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null)
    setMcpWarnings([])
    if (!isEdit && !values.groupId) {
      setError('groupId', { message: 'Group is required' })
      return
    }
    try {
      const saved = isEdit
        ? await update.mutateAsync({ id: id!, body: buildRequestBody(values) })
        : await create.mutateAsync({ ...buildRequestBody(values), groupId: values.groupId })
      const nextTools = (saved.tools ?? []).map(toFormTool)
      reset({
        displayName: saved.displayName ?? '',
        description: saved.description ?? '',
        imageURL: saved.imageURL,
        tools: nextTools,
        env: (saved.env ?? []).map(envVarToFormValue),
        mcpServers: (saved.mcpServers ?? []).map((m) => m.name),
        enabledSkills: saved.enabledSkills ?? [],
      })
      setSelectedIndex(nextTools.length > 0 ? 0 : null)
      navigate(`/agent-images/${encodeURIComponent(saved.id)}`, { replace: true })
    } catch (err) {
      if (err instanceof ApiError) setSubmitError(err.message)
      else setSubmitError('Save failed. Please try again.')
    }
  })

  const image = existing.data

  return (
    <ImageFormView
      isEdit={isEdit}
      id={id}
      image={image}
      register={register}
      control={control}
      setValue={setValue}
      watch={watch}
      errors={errors}
      isSubmitting={isSubmitting}
      onSubmit={onSubmit}
      onRefreshMCP={onRefreshMCP}
      onConfirmDelete={onConfirmDelete}
      submitError={submitError}
      mcpWarnings={mcpWarnings}
      mcpRefreshResult={mcpRefreshResult}
      selectedIndex={selectedIndex}
      setSelectedIndex={setSelectedIndex}
      activeSource={activeSource}
      setActiveSource={setActiveSource}
      confirmOpen={confirmOpen}
      setConfirmOpen={setConfirmOpen}
      isRefreshing={refreshMcp.isPending}
      isSaving={update.isPending}
      isDeleting={remove.isPending}
    />
  )
}
