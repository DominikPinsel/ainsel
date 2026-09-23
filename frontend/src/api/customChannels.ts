import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

/**
 * Custom channels and channel bridges are a browser-local prototype of the
 * hub-side feature: the hub has no `channels` table yet, so anything a user
 * creates here lives in localStorage and can move events nowhere. It exists
 * to validate the model — grouping subscriptions under a named channel, and
 * walking the resulting channel graph — before the backend lands. Local
 * edges are labelled as such wherever they render.
 */

export interface CustomChannel {
  id: string // 'custom:<slug>'
  name: string
  description: string
  createdAt: string
}

export interface ChannelBridge {
  id: string
  from: string // channel id events leave
  to: string // channel id they arrive in
  name: string // label shown on the edge
}

interface Store {
  channels: CustomChannel[]
  bridges: ChannelBridge[]
}

const KEY = 'ainsel.customChannels.v1'
const EMPTY: Store = { channels: [], bridges: [] }

function read(): Store {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return EMPTY
    const parsed = JSON.parse(raw) as Partial<Store>
    return {
      channels: Array.isArray(parsed.channels) ? parsed.channels : [],
      bridges: Array.isArray(parsed.bridges) ? parsed.bridges : [],
    }
  } catch {
    return EMPTY
  }
}

function write(store: Store) {
  try {
    localStorage.setItem(KEY, JSON.stringify(store))
  } catch {
    // storage unavailable (private mode, quota) — mutations surface the
    // in-memory result; persistence is best-effort in the prototype
  }
}

function slug(name: string): string {
  return name.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
}

function uniqueId(base: string, taken: Set<string>): string {
  let id = `custom:${base}`
  let n = 2
  while (taken.has(id)) id = `custom:${base}-${n++}`
  return id
}

export function useCustomChannels() {
  return useQuery({
    queryKey: ['custom-channels'],
    queryFn: () => read(),
  })
}

function useInvalidate() {
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: ['custom-channels'] })
}

export function useCreateCustomChannel() {
  const invalidate = useInvalidate()
  return useMutation({
    mutationFn: async (input: { name: string; description?: string }) => {
      const store = read()
      const base = slug(input.name)
      if (!base) throw new Error('Name must contain at least one letter or digit')
      const taken = new Set([
        ...store.channels.map((c) => c.id),
        ...store.channels.map((c) => slug(c.name)),
      ])
      if (taken.has(`custom:${base}`)) throw new Error(`A channel named "${input.name}" already exists`)
      const channel: CustomChannel = {
        id: uniqueId(base, taken),
        name: input.name.trim(),
        description: input.description?.trim() || `Custom grouping channel for "${input.name.trim()}"`,
        createdAt: new Date().toISOString(),
      }
      write({ ...store, channels: [...store.channels, channel] })
      invalidate()
      return channel
    },
  })
}

export function useDeleteCustomChannel() {
  const invalidate = useInvalidate()
  return useMutation({
    mutationFn: async (id: string) => {
      const store = read()
      if (store.bridges.some((b) => b.from === id || b.to === id)) {
        throw new Error('Channels with bridges attached cannot be deleted')
      }
      write({ ...store, channels: store.channels.filter((c) => c.id !== id) })
      invalidate()
    },
  })
}

export function useAddBridge() {
  const invalidate = useInvalidate()
  return useMutation({
    mutationFn: async (input: { from: string; to: string; name: string }) => {
      const store = read()
      const bridge: ChannelBridge = {
        id: `b${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`,
        from: input.from,
        to: input.to,
        name: input.name.trim() || `${input.from.split(':').slice(1).join(':')} → ${input.to.split(':').slice(1).join(':')}`,
      }
      write({ ...store, bridges: [...store.bridges, bridge] })
      invalidate()
      return bridge
    },
  })
}

export function useRemoveBridge() {
  const invalidate = useInvalidate()
  return useMutation({
    mutationFn: async (id: string) => {
      const store = read()
      write({ ...store, bridges: store.bridges.filter((b) => b.id !== id) })
      invalidate()
    },
  })
}

/** Pure read for non-hook call sites (channel resolution in the registry). */
export function readCustomChannels(): Store {
  return read()
}
