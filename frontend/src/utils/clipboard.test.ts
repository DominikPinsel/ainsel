import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clipboardHasImage, copy, copyImage } from './clipboard'

describe('clipboard.copy', () => {
  let writeText: ReturnType<typeof vi.fn>

  beforeEach(() => {
    writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('writes the given text', async () => {
    const ok = await copy('hello')
    expect(writeText).toHaveBeenCalledWith('hello')
    expect(ok).toBe(true)
  })

  it('returns false when the API throws', async () => {
    writeText.mockRejectedValueOnce(new Error('denied'))
    const ok = await copy('x')
    expect(ok).toBe(false)
  })
})

describe('clipboard.copyImage', () => {
  let write: ReturnType<typeof vi.fn>
  const jpegDataUrl = 'data:image/jpeg;base64,/9j/4AAQSkZJRgABAQEASABIAAD'

  beforeEach(() => {
    write = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { write },
    })
    // NOTE: use a function() here, not an arrow function — tinyspy (vitest 4)
    // mirrors native semantics and rejects `new` on arrow functions.
    globalThis.ClipboardItem = vi.fn(function (items: Record<string, Blob>) {
      return items
    }) as unknown as typeof ClipboardItem
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('writes a jpeg blob to the clipboard', async () => {
    const ok = await copyImage(jpegDataUrl)
    expect(write).toHaveBeenCalled()
    expect(ClipboardItem).toHaveBeenCalled()
    expect(ok).toBe(true)
  })

  it('returns false when the API throws', async () => {
    write.mockRejectedValueOnce(new Error('denied'))
    const ok = await copyImage(jpegDataUrl)
    expect(ok).toBe(false)
  })

  it('returns false when fetch throws', async () => {
    const originalFetch = globalThis.fetch
    globalThis.fetch = vi.fn().mockRejectedValueOnce(new Error('network'))
    const ok = await copyImage(jpegDataUrl)
    expect(ok).toBe(false)
    globalThis.fetch = originalFetch
  })
})

describe('clipboard.clipboardHasImage', () => {
  let read: ReturnType<typeof vi.fn>

  beforeEach(() => {
    read = vi.fn().mockResolvedValue([])
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { read },
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns true when clipboard contains an image item', async () => {
    read.mockResolvedValueOnce([
      { types: ['image/png'] },
    ])
    const ok = await clipboardHasImage()
    expect(ok).toBe(true)
  })

  it('returns false when clipboard contains only text', async () => {
    read.mockResolvedValueOnce([
      { types: ['text/plain'] },
    ])
    const ok = await clipboardHasImage()
    expect(ok).toBe(false)
  })

  it('returns false when clipboard is empty', async () => {
    read.mockResolvedValueOnce([])
    const ok = await clipboardHasImage()
    expect(ok).toBe(false)
  })

  it('returns false when the API throws', async () => {
    read.mockRejectedValueOnce(new Error('denied'))
    const ok = await clipboardHasImage()
    expect(ok).toBe(false)
  })
})