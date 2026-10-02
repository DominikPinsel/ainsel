/// <reference types="node" />
// Shared helpers for specs that assert on the *rendered* result of the
// stylesheets rather than on component markup. Vitest stubs CSS imports (a
// `?raw` import of a .css file is an empty string), so the only way to see
// what a theme actually produces is to read the files off disk and resolve the
// cascade by hand.
//
// Needs node types for exactly this module rather than the app-wide type space.
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

/** Every theme the app ships, i.e. every override block in tokens.css. */
export const THEMES = ['dark', 'peat', 'tallow'] as const

export function stripComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?\*\//g, '')
}

/**
 * Read a stylesheet from disk. A wrong working directory must fail loudly
 * rather than quietly hand back nothing and pass every check.
 */
export function readCss(file: string): string {
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

/** Token map for a theme: `:root` with the theme's block layered over it. */
export function paletteFor(theme: string | null): Map<string, string> {
  const css = readCss('tokens.css')
  const blocks = (header: string) => {
    const start = css.indexOf(header)
    if (start === -1) throw new Error(`tokens.css has no "${header}" block`)
    return css.slice(start + header.length, css.indexOf('}', start))
  }
  const map = new Map<string, string>()
  const sources = [blocks(':root {')]
  if (theme) sources.push(blocks(`html[data-theme="${theme}"] {`))
  for (const block of sources) {
    for (const line of block.split('\n')) {
      const m = /^\s*(--[\w-]+)\s*:\s*(.+?)\s*;?\s*$/.exec(line)
      if (m) map.set(m[1], m[2])
    }
  }
  return map
}

/**
 * Resolve a declaration value down to a literal, following var() chains.
 * A `var()` fallback is used only when the token really is undeclared, which
 * mirrors the browser — and is exactly how off-palette colours sneak in, so
 * `tokens.guard.test.ts` fails the build on any token that reaches that path.
 */
export function resolveVar(value: string, palette: Map<string, string>, depth = 0): string {
  if (depth > 8) throw new Error(`var() chain too deep at "${value}"`)
  const m = /^var\(\s*(--[\w-]+)\s*(?:,\s*(.+))?\s*\)$/.exec(value.trim())
  if (!m) return value.trim()
  const next = palette.get(m[1]) ?? m[2]
  if (!next) throw new Error(`unknown token ${m[1]} in "${value}"`)
  return resolveVar(next, palette, depth + 1)
}

/** Declarations of one top-level rule in primitives.css. */
export function declarationsFor(css: string, selector: string): Record<string, string> {
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

/** Foreground/background of one rule with its tokens resolved for a theme. */
export function surfaceColors(selector: string, theme: string | null) {
  const css = readCss('primitives.css')
  const palette = paletteFor(theme)
  const decls = declarationsFor(css, selector)
  return {
    bg: resolveVar(decls.background ?? '', palette),
    ink: resolveVar(decls.color ?? '', palette),
  }
}

// WCAG relative luminance and contrast ratio, on resolved hex values.
export function luminance(hex: string): number {
  const [r, g, b] = hex
    .slice(1)
    .match(/../g)!
    .map((h) => parseInt(h, 16) / 255)
    .map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

export function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

type RGB = { r: number; g: number; b: number; a: number }

/** Parse `#rgb` / `#rrggbb` / `rgb()` / `rgba()` into 0-255 channels + alpha. */
export function parseColor(value: string): RGB {
  const hex = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(value.trim())
  if (hex) {
    let h = hex[1]
    if (h.length === 3)
      h = h
        .split('')
        .map((c) => c + c)
        .join('')
    return {
      r: parseInt(h.slice(0, 2), 16),
      g: parseInt(h.slice(2, 4), 16),
      b: parseInt(h.slice(4, 6), 16),
      a: 1,
    }
  }
  const fn = /^rgba?\(([^)]+)\)$/i.exec(value.trim())
  if (fn) {
    const parts = fn[1]
      .split(/[\s,/]+/)
      .filter(Boolean)
      .map(Number)
    if (parts.length >= 3 && parts.slice(0, 3).every((n) => !Number.isNaN(n))) {
      return { r: parts[0], g: parts[1], b: parts[2], a: parts.length > 3 ? parts[3] : 1 }
    }
  }
  throw new Error(`cannot parse colour "${value}"`)
}

/**
 * Flatten a possibly translucent colour onto an opaque backdrop — what the
 * browser actually paints. Tinted surfaces like `--signal-haze` are rgba, so
 * their contrast only means something once composited over the pane beneath.
 */
export function composite(fg: string, backdrop: string): string {
  const f = parseColor(fg)
  const b = parseColor(backdrop)
  const mix = (c: number, bc: number) => Math.round(c * f.a + bc * (1 - f.a))
  const to2 = (n: number) => n.toString(16).padStart(2, '0')
  return `#${to2(mix(f.r, b.r))}${to2(mix(f.g, b.g))}${to2(mix(f.b, b.b))}`
}

/** Contrast of `fg` (possibly translucent) painted over `backdrop`. */
export function contrastOn(fg: string, backdrop: string, ink: string): number {
  return contrast(composite(fg, backdrop), ink)
}

/** Resolve a token to a literal colour for a theme. */
export function tokenColor(token: string, theme: string | null): string {
  return resolveVar(`var(${token})`, paletteFor(theme))
}
