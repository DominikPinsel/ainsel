import { useEffect, useState } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useConnectors } from '../../api/connectors'
import {
  useCreateTrigger,
  useUpdateTrigger,
  type TriggerRequest,
  type TriggerResponse,
} from '../../api/triggers'
import { ApiError } from '../../api/client'
import { Button } from '../../primitives/Button'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Select } from '../../primitives/Select'
import { Autocomplete } from '../../primitives/Autocomplete'
import { GroupField } from '../../components/GroupField'

const FILTER_OP_OPTIONS = [
  { value: 'eq', label: 'eq' },
  { value: 'neq', label: 'neq' },
  { value: 'prefix', label: 'prefix' },
  { value: 'suffix', label: 'suffix' },
  { value: 'contains', label: 'contains' },
  { value: 'not-contains', label: 'not-contains' },
  { value: 'in', label: 'in' },
  { value: 'not-in', label: 'not-in' },
  { value: 'regex', label: 'regex' },
] as const

/** Operators that use the `values` array (comma-separated in the UI). */
const MULTI_VALUE_OPS = new Set(['in', 'not-in'])

const filterSchema = z.object({
  field: z.string().min(1, 'Field is required'),
  op: z.string().min(1),
  value: z.string(),
})

const schema = z.object({
  name: z.string().min(1, 'Name is required'),
  connectorRef: z.string().min(1, 'Connector is required'),
  filters: z.array(filterSchema),
  groupId: z.string().optional(),
})

type FormValues = z.infer<typeof schema>

type TriggerFormProps = {
  agentId: string
  /** When provided, the form edits this trigger. Otherwise creates a new one. */
  trigger?: TriggerResponse
  onClose: () => void
  onSaved: () => void
}

export function TriggerForm({ agentId, trigger, onClose, onSaved }: TriggerFormProps) {
  const isEdit = trigger !== undefined
  const [submitError, setSubmitError] = useState<string | null>(null)

  const connectors = useConnectors({ pageSize: 200 })
  const create = useCreateTrigger()
  const update = useUpdateTrigger()

  const {
    register,
    handleSubmit,
    reset,
    setValue,
    watch,
    control,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      connectorRef: '',
      filters: [],
      groupId: '',
    },
  })

  const { fields, append, remove } = useFieldArray({ control, name: 'filters' })

  useEffect(() => {
    if (trigger) {
      reset({
        name: trigger.name,
        connectorRef: trigger.connectorRef ?? '',
        filters: (trigger.filters ?? []).map((f) => ({
          field: f.field,
          op: f.op,
          value: MULTI_VALUE_OPS.has(f.op) && f.values ? f.values.join(', ') : f.value,
        })),
      })
    }
  }, [trigger, reset])

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null)
    if (!isEdit && !values.groupId) {
      setError('groupId', { message: 'Group is required' })
      return
    }
    const mappedFilters = values.filters.map((f) => {
      if (MULTI_VALUE_OPS.has(f.op)) {
        return {
          field: f.field,
          op: f.op,
          value: '',
          values: f.value
            .split(',')
            .map((v) => v.trim())
            .filter(Boolean),
        }
      }
      return { field: f.field, op: f.op, value: f.value }
    })
    const body: TriggerRequest = {
      name: values.name,
      agentRef: agentId,
      connectorRef: values.connectorRef,
      filters: mappedFilters.length > 0 ? mappedFilters : undefined,
      groupId: isEdit ? undefined : values.groupId,
    }
    try {
      if (isEdit && trigger) {
        await update.mutateAsync({ id: trigger.id, body })
      } else {
        await create.mutateAsync(body)
      }
      onSaved()
    } catch (err) {
      if (err instanceof ApiError) setSubmitError(err.message)
      else setSubmitError('Save failed. Please try again.')
    }
  })

  const connectorRefValue = watch('connectorRef')

  return (
    <form
      onSubmit={onSubmit}
      noValidate
      style={{
        marginTop: 18,
        padding: '18px 20px',
        border: '1.5px solid var(--rule)',
        background: 'var(--paper-2)',
      }}
    >
      <header style={{ marginBottom: 14 }}>
        <h4
          style={{
            margin: 0,
            fontFamily: 'var(--mono)',
            fontSize: 11,
            letterSpacing: '0.12em',
            textTransform: 'uppercase',
            color: 'var(--ink-3)',
          }}
        >
          {isEdit ? 'Edit trigger' : 'New trigger'}
        </h4>
      </header>

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
            marginBottom: 16,
          }}
        >
          {submitError}
        </div>
      ) : null}

      <div className="form-grid cols-2">
        <Field label="Name" htmlFor="tf-name" error={errors.name?.message}>
          <Input id="tf-name" autoFocus {...register('name')} />
        </Field>
        <Field label="Agent" htmlFor="tf-agentRef">
          <Input id="tf-agentRef" value={agentId} disabled readOnly />
        </Field>
        <Field label="Connector" htmlFor="tf-connectorRef" error={errors.connectorRef?.message}>
          <Autocomplete
            id="tf-connectorRef"
            value={connectorRefValue}
            onChange={(v) => setValue('connectorRef', v, { shouldValidate: true })}
            options={(connectors.data?.items ?? []).map((c) => ({
              value: c.id,
              label: c.name,
            }))}
            placeholder="Select a connector…"
          />
          <input type="hidden" data-testid="hidden-connectorRef" {...register('connectorRef')} />
        </Field>
        {!isEdit ? (
          <GroupField
            id="tf-groupId"
            value={watch('groupId') ?? ''}
            onChange={(v) => setValue('groupId', v, { shouldDirty: true })}
            error={errors.groupId?.message}
          />
        ) : null}
      </div>

      <section style={{ marginTop: 18 }}>
        <header style={{ marginBottom: 10 }}>
          <h5
            style={{
              margin: 0,
              fontFamily: 'var(--mono)',
              fontSize: 11,
              letterSpacing: '0.12em',
              textTransform: 'uppercase',
              color: 'var(--ink-3)',
            }}
          >
            Filter chain
          </h5>
        </header>
        {fields.length === 0 ? (
          <div
            role="note"
            style={{
              padding: '10px 12px',
              border: '1.5px solid var(--warn)',
              background: 'rgba(192, 138, 20, 0.08)',
              color: 'var(--warn)',
              fontFamily: 'var(--mono)',
              fontSize: 11,
              marginBottom: 10,
            }}
          >
            No filters — this trigger will fire on <strong>every event</strong> from the selected
            connector. Add at least one filter (e.g. <code>type</code> eq{' '}
            <code>issues</code>) to limit which events reach the agent.
          </div>
        ) : (
          <div style={{ display: 'grid', gap: 6 }}>
            {fields.map((f, i) => (
              <div key={f.id} className="filter-row">
                <Input placeholder="field (e.g. labels)" {...register(`filters.${i}.field`)} />
                <Select
                  value={watch(`filters.${i}.op`)}
                  onChange={(v) => setValue(`filters.${i}.op`, v, { shouldValidate: true })}
                  options={FILTER_OP_OPTIONS}
                />
                <Input
                  placeholder={MULTI_VALUE_OPS.has(watch(`filters.${i}.op`)) ? 'value1, value2, …' : 'value'}
                  {...register(`filters.${i}.value`)}
                />
                <button
                  type="button"
                  aria-label={`Remove filter ${i + 1}`}
                  className="remove"
                  onClick={() => remove(i)}
                >
                  ×
                </button>
              </div>
            ))}
          </div>
        )}
        <button
          type="button"
          className="filter-add"
          onClick={() => append({ field: '', op: 'eq', value: '' })}
        >
          ＋ Add filter row
        </button>
        <p className="label" style={{ marginTop: 10 }}>
          Tip: To filter by event type, add a filter with field <code>type</code>
          (derived from the webhook source's event-type header). Example: field ={' '}
          <code>type</code>, op = <code>eq</code>, value = <code>issues</code>.
        </p>
      </section>

      <div
        style={{
          marginTop: 18,
          display: 'flex',
          gap: 10,
          justifyContent: 'flex-end',
        }}
      >
        <Button type="button" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" variant="primary" disabled={isSubmitting}>
          {isSubmitting ? 'Saving…' : isEdit ? 'Save' : 'Create'}
        </Button>
      </div>
    </form>
  )
}
