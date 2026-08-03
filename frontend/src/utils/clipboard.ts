export async function copy(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

export async function copyImage(dataUrl: string): Promise<boolean> {
  try {
    const response = await fetch(dataUrl)
    const blob = await response.blob()
    await navigator.clipboard.write([new ClipboardItem({ [blob.type]: blob })])
    return true
  } catch {
    return false
  }
}

/**
 * Reads the clipboard and returns true if it contains at least one image item.
 * Returns false if the clipboard is empty, contains only text, or the API
 * is unavailable.
 */
export async function clipboardHasImage(): Promise<boolean> {
  try {
    const items = await navigator.clipboard.read()
    return items.some((item) =>
      item.types.some((type) => type.startsWith('image/')),
    )
  } catch {
    return false
  }
}