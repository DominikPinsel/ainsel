import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'

export type StringFilters = Record<string, string | undefined>

export function useUrlFilters<T extends StringFilters>(
  keys: readonly (keyof T & string)[],
) {
  const [searchParams, setSearchParams] = useSearchParams()

  const filters = useMemo(() => {
    const out: StringFilters = {}
    for (const k of keys) {
      const v = searchParams.get(k)
      if (v !== null && v !== '') out[k] = v
    }
    return out as Partial<T>
  }, [searchParams, keys])

  const setFilters = useCallback(
    (patch: Partial<T>) => {
      const next = new URLSearchParams(searchParams)
      for (const [k, v] of Object.entries(patch)) {
        if (v === undefined || v === null || v === '') {
          next.delete(k)
        } else {
          next.set(k, String(v))
        }
      }
      setSearchParams(next, { replace: true })
    },
    [searchParams, setSearchParams],
  )

  const reset = useCallback(() => {
    const next = new URLSearchParams(searchParams)
    for (const k of keys) next.delete(k)
    setSearchParams(next, { replace: true })
  }, [keys, searchParams, setSearchParams])

  return { filters, setFilters, reset }
}
