/**
 * Client-side image compression utility.
 *
 * Takes a data URL (e.g. from html-to-image), draws it onto a canvas,
 * caps dimensions, and iteratively reduces JPEG quality until the result
 * is under a target byte budget. Returns a compressed data URL or null
 * on failure.
 */

const DEFAULT_MAX_WIDTH = 1200
const DEFAULT_MAX_HEIGHT = 1800
const DEFAULT_BUDGET_BYTES = 200 * 1024 // 200 KB
const MIN_QUALITY = 0.1
const QUALITY_STEP = 0.1

/**
 * Compute target dimensions that fit within maxW × maxH while preserving
 * the original aspect ratio.
 */
export function computeDimensions(
  srcW: number,
  srcH: number,
  maxW: number,
  maxH: number,
): { w: number; h: number } {
  let w = srcW
  let h = srcH
  if (w > maxW) {
    h = Math.round(h * (maxW / w))
    w = maxW
  }
  if (h > maxH) {
    w = Math.round(w * (maxH / h))
    h = maxH
  }
  return { w: Math.max(1, w), h: Math.max(1, h) }
}

/**
 * Compress a JPEG data URL. Returns a new (smaller) data URL, or null if
 * compression fails (e.g. canvas not available, image fails to load).
 */
export async function compressImage(
  dataUrl: string,
  options?: {
    maxWidth?: number
    maxHeight?: number
    budgetBytes?: number
  },
): Promise<string | null> {
  const maxW = options?.maxWidth ?? DEFAULT_MAX_WIDTH
  const maxH = options?.maxHeight ?? DEFAULT_MAX_HEIGHT
  const budget = options?.budgetBytes ?? DEFAULT_BUDGET_BYTES

  try {
    // Load the image
    const img = await loadImage(dataUrl)
    const { w, h } = computeDimensions(img.width, img.height, maxW, maxH)

    // Draw onto canvas at target size
    const canvas = document.createElement('canvas')
    canvas.width = w
    canvas.height = h
    const ctx = canvas.getContext('2d')
    if (!ctx) return null
    ctx.drawImage(img, 0, 0, w, h)

    // Iteratively reduce quality until under budget
    let quality = 0.9
    let result = canvas.toDataURL('image/jpeg', quality)

    while (dataUrlSize(result) > budget && quality > MIN_QUALITY) {
      quality = Math.max(MIN_QUALITY, quality - QUALITY_STEP)
      result = canvas.toDataURL('image/jpeg', quality)
    }

    return result
  } catch {
    return null
  }
}

/** Load an image from a data URL and wait for it to decode. */
function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error('Failed to load image'))
    img.src = src
  })
}

/** Return the byte size of a data URL's payload (base64 decoded size). */
function dataUrlSize(dataUrl: string): number {
  const base64 = dataUrl.split(',')[1]
  if (!base64) return 0
  return Math.ceil((base64.length * 3) / 4)
}
