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

/**
 * Expand every `var(--x)` occurrence in a value, not just a value that is
 * wholly one. Needed for `color-mix(in srgb, var(--signal) 12%, var(--paper))`,
 * which resolveVar() passes through untouched.
 */
export function expandVars(value: string, palette: Map<string, string>, depth = 0): string {
  if (depth > 8) throw new Error(`var() chain too deep at "${value}"`)
  const out = value.replace(/var\(\s*(--[\w-]+)\s*\)/g, (_all, name: string) => {
    const v = palette.get(name)
    if (!v) throw new Error(`unknown token ${name} in "${value}"`)
    return v
  })
  return out === value ? out.trim() : expandVars(out, palette, depth + 1)
}

/**
 * Resolve a colour value to a literal `#rrggbb` (opaque) or `rgba(...)`.
 * Evaluates `color-mix(in srgb, …)` the way the CSS spec does — premultiplied
 * alpha, weights normalised — so a mixed surface can be measured rather than
 * guessed at.
 */
export function resolveColor(value: string, palette: Map<string, string>): string {
  let s = expandVars(value, palette)
  for (let guard = 0; s.includes('color-mix('); guard++) {
    if (guard > 8) throw new Error(`color-mix() nested too deep in "${value}"`)
    const m =
      /color-mix\(\s*in\s+srgb\s*,\s*(.+?)\s+(\d*\.?\d+)%\s*,\s*(.+?)\s*(?:(\d*\.?\d+)%)?\s*\)/i.exec(
        s,
      )
    if (!m) throw new Error(`unsupported color-mix() form in "${value}"`)
    const a = parseColor(m[1].trim())
    const b = parseColor(m[3].trim())
    // An omitted second weight is `100% - first`, per spec.
    const w1 = Number(m[2]) / 100
    const w2 = (m[4] === undefined ? 100 - Number(m[2]) : Number(m[4])) / 100
    const total = w1 + w2
    const p1 = w1 / total
    const p2 = w2 / total
    const alpha = p1 * a.a + p2 * b.a
    const chan = (x: number, y: number) => (alpha === 0 ? 0 : (p1 * x * a.a + p2 * y * b.a) / alpha)
    const rgb = [chan(a.r, b.r), chan(a.g, b.g), chan(a.b, b.b)].map((n) => Math.round(n))
    const to2 = (n: number) => n.toString(16).padStart(2, '0')
    const mixed =
      alpha >= 1
        ? `#${rgb.map(to2).join('')}`
        : `rgba(${rgb[0]}, ${rgb[1]}, ${rgb[2]}, ${Number(alpha.toFixed(4))})`
    s = s.slice(0, m.index) + mixed + s.slice(m.index! + m[0].length)
  }
  return s
}

/**
 * Declarations of one top-level rule in primitives.css.
 *
 * The selector must match a whole entry of some rule's selector list, not a
 * substring: `indexOf('.tool-meta {')` also hits `.tool-row-off .tool-meta {`,
 * which quietly measures a different rule than the one asked for.
 */
export function declarationsFor(css: string, selector: string): Record<string, string> {
  for (const rule of parseRules(css)) {
    if (rule.selectors.includes(selector)) return rule.declarations
  }
  throw new Error(`no rule for "${selector}" in primitives.css`)
}

export type CssRule = { selectors: string[]; declarations: Record<string, string> }

/**
 * At-rules whose bodies are ordinary style rules. Flattening one would apply its
 * declarations unconditionally, and skipping it would hide them, so parseRules
 * refuses rather than guessing.
 */
const CONDITIONAL_AT_RULES = new Set([
  'media',
  'supports',
  'container',
  'layer',
  'scope',
  'document',
])

/**
 * Every top-level rule in a stylesheet, in source order.
 *
 * Non-conditional at-rules (`@keyframes`, `@font-face`) are dropped: their
 * bodies are not selector rules, and `@keyframes` percentage stops would
 * otherwise be mistaken for selectors.
 */
export function parseRules(css: string): CssRule[] {
  const source = withoutAtRuleBlocks(stripComments(css))
  const out: CssRule[] = []
  for (const m of source.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const declarations: Record<string, string> = {}
    for (const decl of m[2].split(';')) {
      const [prop, ...value] = decl.split(':')
      if (prop && value.length) declarations[prop.trim()] = value.join(':').trim()
    }
    out.push({
      selectors: m[1]
        .trim()
        .split(',')
        .map((s) => s.trim()),
      declarations,
    })
  }
  return out
}

/** Remove at-rule blocks, throwing on the conditional ones. */
function withoutAtRuleBlocks(source: string): string {
  let out = ''
  let i = 0
  while (i < source.length) {
    const c = source[i]
    // A quoted value may mention an at-rule (`content: "@media"`); copy it
    // through verbatim so it is not mistaken for one.
    if (c === '"' || c === "'") {
      const close = source.indexOf(c, i + 1)
      const end = close === -1 ? source.length : close + 1
      out += source.slice(i, end)
      i = end
      continue
    }
    const at = /^@([\w-]+)/.exec(source.slice(i))
    if (!at) {
      out += c
      i++
      continue
    }
    const name = at[1].toLowerCase()
    if (CONDITIONAL_AT_RULES.has(name)) {
      throw new Error(`parseRules does not handle @${name}; its body is real style rules`)
    }
    i = endOfAtRule(source, i) // drop the whole block
  }
  return out
}

/** Index just past an at-rule: its balanced `{…}` body, or its terminating `;`. */
function endOfAtRule(source: string, start: number): number {
  let i = start
  let depth = 0
  while (i < source.length) {
    const c = source[i]
    if (c === '{') depth++
    else if (c === '}') {
      depth--
      if (depth === 0) return i + 1
    } else if (c === ';' && depth === 0) return i + 1
    i++
  }
  return source.length
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
