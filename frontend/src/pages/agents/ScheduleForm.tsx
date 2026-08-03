import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import {
  useCreateCronTrigger,
  useUpdateCronTrigger,
  type CronTriggerRequest,
  type CronTriggerResponse,
} from '../../api/cronTriggers'
import { ApiError } from '../../api/client'
import { Button } from '../../primitives/Button'
import { Check } from '../../primitives/Check'
import { Field } from '../../primitives/Field'
import { Input } from '../../primitives/Input'
import { Textarea } from '../../primitives/Textarea'
import { GroupField } from '../../components/GroupField'

const schema = z.object({
  name: z.string().min(1, 'Name is required'),
  schedule: z
    .string()
    .min(1, 'Schedule is required')
    .refine(
      (v) => {
        const parts = v.trim().split(/\s+/)
        return parts.length === 5
      },
      'Must be a 5-field cron expression: minute hour day-of-month month day-of-week',
    ),
  prompt: z.string().min(1, 'Prompt is required'),
  enabled: z.boolean(),
  groupId: z.string().optional(),
})

type FormValues = z.infer<typeof schema>

type ScheduleFormProps = {
  agentId: string
  /** When provided, the form edits this cron trigger. Otherwise creates a new one. */
  trigger?: CronTriggerResponse
  onClose: () => void
  onSaved: () => void
}

const SCHEDULE_PRESETS = [
  { label: 'Every weekday 9am', value: '0 9 * * 1-5' },
  { label: 'Daily midnight', value: '0 0 * * *' },
  { label: 'Every hour', value: '0 * * * *' },
  { label: 'Every 6 hours', value: '0 */6 * * *' },
  { label: 'Sundays 2am', value: '0 2 * * 0' },
] as const

export function ScheduleForm({ agentId, trigger, onClose, onSaved }: ScheduleFormProps) {
  const isEdit = trigger !== undefined
  const [submitError, setSubmitError] = useState<string | null>(null)

  const create = useCreateCronTrigger()
  const update = useUpdateCronTrigger()

  const {
    register,
    handleSubmit,
    reset,
    setValue,
    watch,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      schedule: '',
      prompt: '',
      enabled: true,
      groupId: '',
    },
  })

  useEffect(() => {
    if (trigger) {
      reset({
        name: trigger.name,
        schedule: trigger.schedule,
        prompt: trigger.prompt,
        enabled: trigger.enabled,
      })
    }
  }, [trigger, reset])

  const onSubmit = handleSubmit(async (values) => {
    setSubmitError(null)
    if (!isEdit && !values.groupId) {
      setError('groupId', { message: 'Group is required' })
      return
    }
    try {
      if (isEdit && trigger) {
        await update.mutateAsync({
          id: trigger.id,
          body: {
            name: values.name,
            schedule: values.schedule,
            prompt: values.prompt,
            enabled: values.enabled,
          },
        })
      } else {
        const body: CronTriggerRequest = {
          name: values.name,
          agentRef: agentId,
          schedule: values.schedule,
          prompt: values.prompt,
          enabled: values.enabled,
          groupId: values.groupId,
        }
        await create.mutateAsync(body)
      }
      onSaved()
    } catch (err) {
      if (err instanceof ApiError) setSubmitError(err.message)
      else setSubmitError('Save failed. Please try again.')
    }
  })

  const enabled = watch('enabled')
  const scheduleValue = watch('schedule')

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
          {isEdit ? 'Edit schedule' : 'New schedule'}
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
        <Field label="Name" htmlFor="sf-name" error={errors.name?.message}>
          <Input id="sf-name" autoFocus {...register('name')} placeholder="e.g. Nightly stale issue sweep" />
        </Field>
        <Field label="Agent" htmlFor="sf-agentRef">
          <Input id="sf-agentRef" value={agentId} disabled readOnly />
        </Field>
        {!isEdit ? (
          <GroupField
            id="sf-groupId"
            value={watch('groupId') ?? ''}
            onChange={(v) => setValue('groupId', v, { shouldDirty: true })}
            error={errors.groupId?.message}
          />
        ) : null}
        <Field
          label="Schedule"
          htmlFor="sf-schedule"
          error={errors.schedule?.message}
          hint="5-field cron (minute hour dom month dow). Numbers only — named days not supported."
        >
          <Input
            id="sf-schedule"
            {...register('schedule')}
            placeholder="0 9 * * 1-5"
            style={{ fontFamily: 'var(--mono)', fontSize: 12 }}
          />
          <div style={{ display: 'flex', gap: 6, marginTop: 6, flexWrap: 'wrap' }}>
            {SCHEDULE_PRESETS.map((p) => (
              <button
                key={p.value}
                type="button"
                className="filter-add"
                style={{
                  fontSize: 10,
                  padding: '3px 8px',
                  opacity: scheduleValue === p.value ? 1 : 0.6,
                }}
                onClick={() => setValue('schedule', p.value, { shouldValidate: true })}
              >
                {p.label}
              </button>
            ))}
          </div>
        </Field>
        <Field label="Enabled" htmlFor="sf-enabled">
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, height: '100%', paddingTop: 6 }}>
            <Check
              checked={enabled}
              onChange={(v) => setValue('enabled', v)}
              aria-label="Enabled"
            />
            <span style={{ fontFamily: 'var(--mono)', fontSize: 11, color: 'var(--ink-3)' }}>
              {enabled ? 'Schedule is active' : 'Paused (no fires)'}
            </span>
          </div>
        </Field>
      </div>

      <div style={{ marginTop: 18 }}>
        <Field label="Prompt" htmlFor="sf-prompt" error={errors.prompt?.message} hint="Sent verbatim to the agent as the user message on each fire.">
          <Textarea
            id="sf-prompt"
            rows={5}
            {...register('prompt')}
            placeholder="e.g. Review all issues labeled 'stale' in the last 7 days and close any that have no activity."
            style={{ fontFamily: 'var(--mono)', fontSize: 12 }}
          />
        </Field>
      </div>

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