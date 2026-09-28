/// <reference types="node" />
// This test reads the stylesheets off disk, so it needs node types for exactly
// this file rather than the app-wide type space.
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

// The payload / markdown well is an always-dark element, like the sidebar chrome:
// a JSON block that flips to white in a dark theme does not read as code, and
// `background: var(--ink)` is exactly that flip. These assertions check the
// rendered colours rather than the source text, so they still bite if someone
// reintroduces the pair under a different token name.

const ALWAYS_DARK_SURFACES = ['pre.code', '.md-body pre', '.example-block textarea'] as const
// Every theme the app ships. If a fifth appears in tokens.css, this list fails
// and has to be extended deliberately.
const THEMES = ['dark', 'peat', 'tallow'] as const

function stripComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?\*\//g, '')
}

/**
 * Read a stylesheet from disk. Vitest stubs CSS imports (a `?raw` import of a
 * .css file is an empty string), so the only way to assert on the real bytes is
 * to read the file — and a wrong working directory must fail loudly rather than
 * quietly hand back nothing and pass everything.
 */
function readCss(file: string): string {
  const path = resolve(process.cwd(), 'src/design', file)
  let text: string
  try {
    text = readFileSync(path, 'utf8')
  } catch {
    throw new Error(`cannot read ${file} from ${path} — run vitest from frontend/`)
  }
  if (!text.includes('--')) throw new Error(`${path} does not look like the token stylesheet`)
  return stripComments(text)
}

/** Token map for a theme: :root with the theme's block layered over it. */
function paletteFor(theme: string | null): Map<string, string> {
  const css = readCss('tokens.css')
  const blocks = (header: string) => {
    const start = css.indexOf(header)
    if (start === -1) throw new Error(`tokens.css has no "${header}" block`)
    return css.slice(start + header.length, css.indexOf('}', start))
  }
  const map = new Map<string, string>()
  const blocks_ = [blocks(':root {')]
  if (theme) blocks_.push(blocks(`html[data-theme="${theme}"] {`))
  for (const block of blocks_) {
    for (const line of block.split('\n')) {
      const m = /^\s*(--[\w-]+)\s*:\s*(.+?)\s*;?\s*$/.exec(line)
      if (m) map.set(m[1], m[2])
    }
  }
  return map
}

/** Resolve a declaration value down to a literal, following var() chains. */
function resolveVar(value: string, palette: Map<string, string>, depth = 0): string {
  if (depth > 8) throw new Error(`var() chain too deep at "${value}"`)
  const m = /^var\(\s*(--[\w-]+)\s*(?:,\s*(.+))?\s*\)$/.exec(value.trim())
  if (!m) return value.trim()
  const next = palette.get(m[1]) ?? m[2]
  if (!next) throw new Error(`unknown token ${m[1]} in "${value}"`)
  return resolveVar(next, palette, depth + 1)
}

/** Declarations of one top-level rule in primitives.css. */
function declarationsFor(css: string, selector: string): Record<string, string> {
  const start = css.indexOf(`${selector} {`)
  if (start === -1) throw new Error(`no rule for "${selector}" in primitives.css`)
  const body = css.slice(start + selector.length + 2, css.indexOf('}', start))
  const out: Record<string, string> = {}
  for (const decl of body.split(';')) {
    const [prop, ...value] = decl.split(':')
    if (prop && value.length) out[prop.trim()] = value.join(':').trim()
  }
  return out
}

// WCAG relative luminance and contrast ratio, on resolved hex values.
function luminance(hex: string): number {
  const [r, g, b] = hex
    .slice(1)
    .match(/../g)!
    .map((h) => parseInt(h, 16) / 255)
    .map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

function surfaceColors(selector: string, theme: string | null) {
  const css = readCss('primitives.css')
  const palette = paletteFor(theme)
  const decls = declarationsFor(css, selector)
  return {
    bg: resolveVar(decls.background ?? '', palette),
    ink: resolveVar(decls.color ?? '', palette),
  }
}

describe('always-dark code surfaces', () => {
  it.each(ALWAYS_DARK_SURFACES)('%s declares a background and a colour', (selector) => {
    const decls = declarationsFor(readCss('primitives.css'), selector)
    expect(decls.background, `${selector} has no background`).toBeTruthy()
    expect(decls.color, `${selector} has no color`).toBeTruthy()
  })

  it('keeps the light theme well dark', () => {
    // :root is the light theme, so it is covered separately from the overrides.
    for (const selector of ALWAYS_DARK_SURFACES) {
      const { bg, ink } = surfaceColors(selector, null)
      expect(luminance(bg), `${selector} bg is light in light theme`).toBeLessThan(0.2)
      expect(contrast(bg, ink), `${selector} fails AA in light theme`).toBeGreaterThanOrEqual(4.5)
    }
  })

  it.each(THEMES)('the well stays dark and legible in the %s theme', (theme) => {
    for (const selector of ALWAYS_DARK_SURFACES) {
      const { bg, ink } = surfaceColors(selector, theme)
      expect(bg, `${selector} background did not resolve to a hex color`).toMatch(
        /^#[0-9a-f]{6}$/i,
      )
      expect(ink, `${selector} text did not resolve to a hex color`).toMatch(/^#[0-9a-f]{6}$/i)
      expect(
        luminance(bg),
        `${selector} renders as a light box in ${theme} (${bg}, luminance ${luminance(bg).toFixed(3)})`,
      ).toBeLessThan(0.2)
      expect(
        contrast(bg, ink),
        `${selector} text fails WCAG AA in ${theme} (${ink} on ${bg})`,
      ).toBeGreaterThanOrEqual(4.5)
    }
  })

  it('resolves var() through a chain, not just one hop', () => {
    // Guards the harness itself: a surface may point at a token that points at
    // a colour, and silently returning "var(--x)" would pass every check above.
    const palette = paletteFor('dark')
    expect(resolveVar('var(--code-bg)', palette)).toMatch(/^#[0-9a-f]{6}$/i)
    expect(surfaceColors('pre.code', 'dark').bg).not.toContain('var(')
  })
})
