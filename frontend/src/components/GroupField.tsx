import { useGroups } from '../api/groups'
import { Field } from '../primitives/Field'
import { Select } from '../primitives/Select'

type GroupFieldProps = {
  id?: string
  value: string
  onChange: (next: string) => void
  error?: string
  label?: string
  disabled?: boolean
}

/**
 * A form field that lets the user pick the group a resource belongs to.
 * Groups are fetched from the hub; the caller wires value/onChange into
 * react-hook-form (or plain state).
 */
export function GroupField({
  id = 'groupId',
  value,
  onChange,
  error,
  label = 'Group',
  disabled,
}: GroupFieldProps) {
  const groups = useGroups()
  const items = Array.isArray(groups.data) ? groups.data : []

  return (
    <Field label={label} htmlFor={id} error={error}>
      <Select
        id={id}
        value={value}
        onChange={onChange}
        disabled={disabled}
        options={items.map((g) => ({
          value: g.id,
          label: g.name,
        }))}
        emptyLabel={groups.isLoading ? 'Loading groups…' : 'Select a group…'}
      />
    </Field>
  )
}
