const STORAGE_KEY = 'ainsel-theme'

export type Theme = 'light' | 'dark' | 'peat' | 'tallow'

export function getStoredTheme(): Theme | null {
  try {
    return localStorage.getItem(STORAGE_KEY) as Theme | null
  } catch {
    return null
  }
}

export function setStoredTheme(theme: Theme) {
  try {
    localStorage.setItem(STORAGE_KEY, theme)
  } catch {
    // ignore
  }
  applyTheme(theme)
}

export function applyTheme(theme: Theme | null) {
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  const resolved = theme ?? (prefersDark ? 'dark' : 'light')
  document.documentElement.dataset.theme = resolved
  document.documentElement.style.colorScheme = resolved === 'light' ? 'light' : 'dark'
}

export function toggleTheme() {
  const current = document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light'
  const next = current === 'dark' ? 'light' : 'dark'
  setStoredTheme(next)
}

export function initTheme() {
  applyTheme(getStoredTheme())

  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (getStoredTheme() === null) {
      applyTheme(null)
    }
  })
}
