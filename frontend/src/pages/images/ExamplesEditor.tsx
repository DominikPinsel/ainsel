import { useFieldArray, type Control, type UseFormRegister } from 'react-hook-form'
import type { ImageFormValues } from './ImageDetail.types'

type ExamplesEditorProps = {
  control: Control<ImageFormValues>
  register: UseFormRegister<ImageFormValues>
  toolIndex: number
}

export function ExamplesEditor({ control, register, toolIndex }: ExamplesEditorProps) {
  const { fields, append, remove } = useFieldArray({
    control,
    name: `tools.${toolIndex}.examples` as const,
  })

  return (
    <div>
      {fields.length === 0 ? (
        <p className="label" style={{ color: 'var(--ink-3)', marginBottom: 8 }}>
          No examples yet.
        </p>
      ) : (
        fields.map((f, i) => (
          <div key={f.id} className="example-block">
            <div className="example-block-header">
              <input
                aria-label={`Example ${i + 1} title`}
                placeholder={`Example ${i + 1} title`}
                {...register(`tools.${toolIndex}.examples.${i}.title`)}
              />
              <button
                type="button"
                className="remove"
                aria-label={`Remove example ${i + 1}`}
                onClick={() => remove(i)}
              >
                ×
              </button>
            </div>
            <textarea
              rows={4}
              aria-label={`Example ${i + 1} snippet`}
              placeholder="# example snippet"
              {...register(`tools.${toolIndex}.examples.${i}.snippet`)}
            />
          </div>
        ))
      )}
      <button
        type="button"
        className="filter-add"
        onClick={() => append({ title: '', snippet: '' })}
      >
        ＋ Add example
      </button>
    </div>
  )
}
