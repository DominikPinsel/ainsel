import type { Control, FieldErrors, UseFormRegister, UseFormSetValue, UseFormWatch } from 'react-hook-form'
import { Link, useNavigate } from 'react-router-dom'
import type { ImageFormValues } from './ImageDetail.types'
import type { Source } from './ToolSourceSidebar'
import { Button } from '../../primitives/Button'
import { ConfirmModal } from '../../primitives/ConfirmModal'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Panel } from '../../primitives/Panel'
import { Titleblock } from '../../layout/Titleblock'
import { EnvVarFieldArray } from './EnvVarFieldArray'
import { ToolFieldArray } from './ToolFieldArray'
import { McpServerSelector } from './McpServerSelector'
import { SkillsSelector } from './SkillsSelector'
import { GroupField } from '../../components/GroupField'

export type ImageFormViewProps = {
  isEdit: boolean
  id: string | undefined
  image: { displayName?: string; imageURL?: string } | undefined
  register: UseFormRegister<ImageFormValues>
  control: Control<ImageFormValues>
  setValue: UseFormSetValue<ImageFormValues>
  watch: UseFormWatch<ImageFormValues>
  errors: FieldErrors<ImageFormValues>
  isSubmitting: boolean
  onSubmit: (e: React.FormEvent) => void
  onRefreshMCP: () => Promise<void>
  onConfirmDelete: () => void
  submitError: string | null
  mcpWarnings: string[]
  mcpRefreshResult: string | null
  selectedIndex: number | null
  setSelectedIndex: (index: number | null) => void
  activeSource: Source
  setActiveSource: (source: Source) => void
  confirmOpen: boolean
  setConfirmOpen: (open: boolean) => void
  isRefreshing: boolean
  isSaving: boolean
  isDeleting: boolean
}

export function ImageFormView({
  isEdit,
  id,
  image,
  register,
  control,
  setValue,
  watch,
  errors,
  isSubmitting,
  onSubmit,
  onRefreshMCP,
  onConfirmDelete,
  submitError,
  mcpWarnings,
  mcpRefreshResult,
  selectedIndex,
  setSelectedIndex,
  activeSource,
  setActiveSource,
  confirmOpen,
  setConfirmOpen,
  isRefreshing,
  isSaving,
  isDeleting,
}: ImageFormViewProps) {
  const navigate = useNavigate()
  const mcpServers = watch('mcpServers') ?? []
  const envVars = watch('env') ?? []

  return (
    <>
      <Titleblock
        crumbs={
          <>
            Fleet / <Link to="/agent-images">Agent Images</Link> /{' '}
            <b>{isEdit ? (image?.displayName ?? id) : 'New'}</b>
          </>
        }
        title={
          <>
            {isEdit ? (image?.displayName ?? <em>Image</em>) : 'New '}
            {!isEdit ? <em>Image</em> : null}
          </>
        }
        actions={
          <>
            <Button onClick={() => navigate(-1)}>Cancel</Button>
            <Button type="submit" variant="primary" form="image-form" disabled={isSubmitting}>
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
        id="image-form"
        onSubmit={onSubmit}
        noValidate
        style={{ padding: '28px 32px', maxWidth: 1100 }}
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
        {mcpWarnings.length > 0 ? (
          <div
            role="alert"
            style={{
              padding: '10px 12px',
              border: '1.5px solid var(--warning, #b45309)',
              background: 'var(--warning-haze, #fef3c7)',
              color: 'var(--warning, #b45309)',
              fontFamily: 'var(--mono)',
              fontSize: 11,
              marginBottom: 20,
            }}
          >
            <strong>MCP refresh warnings (some servers may not have been reached):</strong>
            <ul style={{ margin: '6px 0 0', paddingLeft: 18 }}>
              {mcpWarnings.map((w, i) => (
                <li key={i}>{w}</li>
              ))}
            </ul>
          </div>
        ) : null}
        {mcpRefreshResult ? (
          <div
            role="status"
            style={{
              padding: '10px 12px',
              border: '1.5px solid var(--success, #15803d)',
              background: 'var(--success-haze, #dcfce7)',
              color: 'var(--success, #15803d)',
              fontFamily: 'var(--mono)',
              fontSize: 11,
              marginBottom: 20,
            }}
          >
            {mcpRefreshResult}
          </div>
        ) : null}

        <Panel title="Image" className="cropped">
          <div style={{ display: 'grid', gap: 14 }}>
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
            <Field label="Image URL" htmlFor="imageURL" error={errors.imageURL?.message}>
              <Input id="imageURL" placeholder="ghcr.io/org/image:tag" {...register('imageURL')} />
            </Field>
            <Field label="Description" htmlFor="description">
              <Input id="description" {...register('description')} />
            </Field>
          </div>
        </Panel>

        <EnvVarFieldArray
          control={control}
          register={register}
          watch={watch}
          errors={errors}
        />

        <McpServerSelector
          mcpServers={mcpServers}
          envNames={envVars.map((e) => e.name).filter((n) => !!n)}
          onChange={(names) => setValue('mcpServers', names)}
          refresh={
            isEdit && id
              ? {
                  id,
                  onClick: onRefreshMCP,
                  isPending: isRefreshing,
                  isSaving,
                }
              : undefined
          }
        />

        <SkillsSelector
          enabledSkills={watch('enabledSkills') ?? []}
          onChange={(ids) => setValue('enabledSkills', ids, { shouldDirty: true })}
        />

        <ToolFieldArray
          control={control}
          register={register}
          setValue={setValue}
          watch={watch}
          selectedIndex={selectedIndex}
          setSelectedIndex={setSelectedIndex}
          activeSource={activeSource}
          setActiveSource={setActiveSource}
          onRefreshMCP={onRefreshMCP}
          isRefreshing={isRefreshing}
          canRefresh={isEdit && !!id}
        />
      </form>

      <ConfirmModal
        open={confirmOpen}
        title="Delete agent image?"
        body={
          <>
            <b>{image?.displayName ?? id}</b> will be permanently removed. Agents referencing this
            image will fail to start until you point them at a replacement.
          </>
        }
        confirmLabel={isDeleting ? 'Deleting…' : 'Delete'}
        destructive
        onConfirm={onConfirmDelete}
        onCancel={() => setConfirmOpen(false)}
      />
    </>
  )
}
