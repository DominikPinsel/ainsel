import type { Control, UseFormRegister, UseFormSetValue, UseFormWatch } from 'react-hook-form'
import { useFieldArray } from 'react-hook-form'
import type { ImageFormValues } from './ImageDetail.types'
import { Cropped } from '../../primitives/Cropped'
import { ToolEditor } from './ToolEditor'
import { ToolList } from './ToolList'
import { ToolSourceSidebar, type Source } from './ToolSourceSidebar'

type ToolFieldArrayProps = {
  control: Control<ImageFormValues>
  register: UseFormRegister<ImageFormValues>
  setValue: UseFormSetValue<ImageFormValues>
  watch: UseFormWatch<ImageFormValues>
  selectedIndex: number | null
  setSelectedIndex: (index: number | null) => void
  activeSource: Source
  setActiveSource: (source: Source) => void
  onRefreshMCP: () => void
  isRefreshing: boolean
  canRefresh: boolean
}

export function ToolFieldArray({
  control,
  register,
  setValue,
  watch,
  selectedIndex,
  setSelectedIndex,
  activeSource,
  setActiveSource,
  onRefreshMCP,
  isRefreshing,
  canRefresh,
}: ToolFieldArrayProps) {
  const {
    fields,
    append,
    remove: removeTool,
  } = useFieldArray({ control, name: 'tools' })

  const tools = watch('tools')

  const onAddTool = () => {
    append({ name: '', kind: 'shell', description: '', enabled: true, isNew: false, examples: [] })
    setSelectedIndex(fields.length)
  }

  const onRemoveSelected = () => {
    if (selectedIndex === null) return
    removeTool(selectedIndex)
    setSelectedIndex(null)
  }

  const onToggleTool = (index: number, enabled: boolean) => {
    setValue(`tools.${index}.enabled`, enabled, { shouldDirty: true })
  }

  const onToggleAllTools = (indices: number[], enabled: boolean) => {
    indices.forEach((i) => setValue(`tools.${i}.enabled`, enabled, { shouldDirty: true }))
  }

  return (
    <div style={{ marginTop: 24 }}>
      <Cropped>
        <div
          className="md-grid"
          style={{
            gridTemplateColumns: '140px 240px 1fr',
            gridTemplateRows: 'minmax(0, 1fr)',
            height: '640px',
            maxHeight: '70vh',
            minHeight: '480px',
          }}
        >
          <ToolSourceSidebar
            tools={tools}
            activeSource={activeSource}
            onSourceChange={setActiveSource}
            onRefresh={onRefreshMCP}
            isRefreshing={isRefreshing}
            canRefresh={canRefresh}
          />
          <ToolList
            tools={tools}
            activeSource={activeSource}
            selectedIndex={selectedIndex}
            onSelect={setSelectedIndex}
            onToggle={onToggleTool}
            onToggleAll={onToggleAllTools}
            onAdd={onAddTool}
          />
          <ToolEditor
            key={selectedIndex ?? 'none'}
            toolIndex={selectedIndex}
            tools={tools}
            control={control}
            register={register}
            setValue={setValue}
            watch={watch}
            onRemove={onRemoveSelected}
          />
        </div>
      </Cropped>
    </div>
  )
}
