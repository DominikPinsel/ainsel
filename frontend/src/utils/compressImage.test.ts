import { describe, expect, it, vi } from 'vitest'
import { compressImage, computeDimensions } from './compressImage'

describe('computeDimensions', () => {
  it('returns original size when within limits', () => {
    expect(computeDimensions(800, 600, 1200, 1800)).toEqual({ w: 800, h: 600 })
  })

  it('scales down width when exceeding max width', () => {
    const result = computeDimensions(2400, 1200, 1200, 1800)
    expect(result.w).toBe(1200)
    expect(result.h).toBe(600)
  })

  it('scales down height when exceeding max height', () => {
    const result = computeDimensions(800, 3600, 1200, 1800)
    expect(result.w).toBe(400)
    expect(result.h).toBe(1800)
  })

  it('scales down both when exceeding both limits', () => {
    const result = computeDimensions(4000, 6000, 1200, 1800)
    // Width-limited: 1200 x 1800 → height still exceeds → height-limited: 360 x 1800
    // Actually: first width check: w=1200, h=6000*(1200/4000)=1800. h=1800 ≤ 1800, ok.
    expect(result.w).toBe(1200)
    expect(result.h).toBe(1800)
  })

  it('never returns zero dimensions', () => {
    const result = computeDimensions(1, 1, 1200, 1800)
    expect(result.w).toBeGreaterThanOrEqual(1)
    expect(result.h).toBeGreaterThanOrEqual(1)
  })
})

describe('compressImage', () => {
  it('returns null when image fails to load', async () => {
    // Mock Image to always error
    const origImage = globalThis.Image
    globalThis.Image = vi.fn(function () {
      const img = { onerror: null as (() => void) | null, onload: null as (() => void) | null, src: '' }
      Object.defineProperty(img, 'src', {
        set() {
          queueMicrotask(() => img.onerror?.())
        },
      })
      return img as unknown as HTMLImageElement
    }) as unknown as typeof Image

    const result = await compressImage('data:image/jpeg;base64,abc')
    expect(result).toBeNull()

    globalThis.Image = origImage
  })

  it('returns a compressed data URL on success', async () => {
    const compressedUrl = 'data:image/jpeg;base64,compressed'

    // Mock Image to load successfully
    const origImage = globalThis.Image
    globalThis.Image = vi.fn(function () {
      const img = {
        onerror: null as (() => void) | null,
        onload: null as (() => void) | null,
        src: '',
        width: 800,
        height: 600,
      }
      Object.defineProperty(img, 'src', {
        set() {
          queueMicrotask(() => img.onload?.())
        },
      })
      return img as unknown as HTMLImageElement
    }) as unknown as typeof Image

    // Mock canvas
    const mockCtx = {
      drawImage: vi.fn(),
    }
    const mockCanvas = {
      width: 0,
      height: 0,
      getContext: vi.fn(() => mockCtx),
      toDataURL: vi.fn(() => compressedUrl),
    }
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      if (tag === 'canvas') return mockCanvas as unknown as HTMLCanvasElement
      return document.createElement(tag)
    })

    const result = await compressImage('data:image/jpeg;base64,abc')
    expect(result).toBe(compressedUrl)
    expect(mockCtx.drawImage).toHaveBeenCalled()

    globalThis.Image = origImage
    vi.restoreAllMocks()
  })
})
