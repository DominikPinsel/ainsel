import type { Control, FieldErrors, UseFormRegister, UseFormWatch } from 'react-hook-form'
import { useFieldArray } from 'react-hook-form'
import type { ImageFormValues } from './ImageDetail.types'
import { Button } from '../../primitives/Button'
import { Input } from '../../primitives/Input'
import { Panel } from '../../primitives/Panel'

type EnvVarFieldArrayProps = {
  control: Control<ImageFormValues>
  register: UseFormRegister<ImageFormValues>
  watch: UseFormWatch<ImageFormValues>
  errors: FieldErrors<ImageFormValues>
}

export function EnvVarFieldArray({ control, register, watch, errors }: EnvVarFieldArrayProps) {
  const {
    fields: envFields,
    append: appendEnv,
    remove: removeEnv,
  } = useFieldArray({ control, name: 'env' })

  return (
    <div style={{ marginTop: 24 }}>
      <Panel title="Environment Variables">
        <div style={{ display: 'grid', gap: 8 }}>
          {envFields.map((field, idx) => (
            <div
              key={field.id}
              style={{
                display: 'grid',
                gridTemplateColumns: '1fr 1fr auto auto',
                gap: 8,
                alignItems: 'start',
              }}
            >
              <div>
                <Input placeholder="NAME" {...register(`env.${idx}.name`)} />
                {errors.env?.[idx]?.name?.message ? (
                  <div className="field-error">{errors.env[idx].name.message}</div>
                ) : null}
              </div>
              <Input
                placeholder={watch(`env.${idx}.secret`) ? '••••••••' : 'value'}
                type={watch(`env.${idx}.secret`) ? 'password' : 'text'}
                {...register(`env.${idx}.value`)}
              />
              <label
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 4,
                  fontSize: 12,
                  whiteSpace: 'nowrap',
                  cursor: 'pointer',
                }}
              >
                <input
                  type="checkbox"
                  {...register(`env.${idx}.secret`)}
                  checked={!!watch(`env.${idx}.secret`)}
                />
                Secret
              </label>
              <Button type="button" aria-label="Remove variable" onClick={() => removeEnv(idx)}>
                ×
              </Button>
            </div>
          ))}
          <div>
            <Button
              type="button"
              onClick={() => appendEnv({ name: '', value: '', secret: false })}
            >
              Add Variable
            </Button>
          </div>
        </div>
      </Panel>
    </div>
  )
}
