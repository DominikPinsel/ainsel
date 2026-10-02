import { describe, expect, it } from 'vitest'

import {
  THEMES,
  composite,
  contrast,
  contrastOn,
  declarationsFor,
  paletteFor,
  readCss,
  resolveColor,
  tokenColor,
} from './cssThemeKit'

// The Tools tab and the MCP / Skills picker both mark the current row with a
// translucent signal tint over a themed pane. Before the Fjord migration the
// dual-list used `var(--surface-2, #eef)` — an undeclared token whose hardcoded
// lavender fallback painted a near-white block behind near-white text in every
// dark theme (measured 1.09:1). These assertions resolve the real cascade per
// theme and composite the tint over the pane it sits on, so a regression shows
// up as a number rather than as a screenshot nobody looks at.

const WCAG_AA = 4.5

type Surface = {
  /** What a reader would call it in a bug report. */
  name: string
  /** Rule carrying the tint. */
  tintSelector: string
  /** Rule carrying the foreground colour. */
  inkSelector: string
  /** Opaque pane the tint is painted over. */
  backdrop: string
}

const SURFACES: Surface[] = [
  {
    name: 'MCP / Skills picker — selected row label',
    tintSelector: '.dual-list-row.selected',
    inkSelector: '.dual-list-row.selected .dual-list-row-label',
    backdrop: '--paper',
  },
  {
    name: 'MCP / Skills picker — selected row description',
    tintSelector: '.dual-list-row.selected',
    inkSelector: '.dual-list-row-desc',
    backdrop: '--paper',
  },
  {
    name: 'Tool list — active row',
    tintSelector: '.tool-row.active',
    inkSelector: '.tool-row.active',
    backdrop: '--paper-2',
  },
  {
    name: 'Tool list — active row meta',
    tintSelector: '.tool-row.active',
    inkSelector: '.tool-row.active .tool-meta',
    backdrop: '--paper-2',
  },
  {
    name: 'Tool sources — active source',
    tintSelector: '.src-item.active',
    inkSelector: '.src-item.active',
    backdrop: '--paper-deep',
  },
  {
    name: 'Notice — warning banner',
    tintSelector: '.notice-warn',
    inkSelector: '.notice-warn',
    // Notices render both on the bare page and inside a Panel; --paper-2 is the
    // darker of the two in every theme, so it is the case worth pinning.
    backdrop: '--paper-2',
  },
  {
    name: 'Notice — error banner',
    tintSelector: '.notice-err',
    inkSelector: '.notice-err',
    backdrop: '--paper-2',
  },
]

const ALL_THEMES: (string | null)[] = [null, ...THEMES]

function themeName(theme: string | null): string {
  return theme ?? 'light'
}

/** Resolve one declaration of one rule against a theme's palette. */
function resolve(selector: string, prop: string, theme: string | null): string {
  const decls = declarationsFor(readCss('primitives.css'), selector)
  const raw = decls[prop]
  if (!raw) throw new Error(`${selector} declares no ${prop}`)
  return resolveColor(raw, paletteFor(theme))
}

describe('signal-tinted selection surfaces', () => {
  it.each(ALL_THEMES.map((t) => [themeName(t), t] as const))(
    'stays legible in the %s theme',
    (label, theme) => {
      for (const s of SURFACES) {
        const tint = resolve(s.tintSelector, 'background', theme)
        const ink = resolve(s.inkSelector, 'color', theme)
        const backdrop = tokenColor(s.backdrop, theme)
        const ratio = contrastOn(tint, backdrop, ink)
        expect(
          ratio,
          `${s.name} fails WCAG AA in the ${label} theme: ${ink} on ${tint} over ${backdrop}`,
        ).toBeGreaterThanOrEqual(WCAG_AA)
      }
    },
  )

  it('tints with the palette, never a hardcoded colour', () => {
    // The whole point of the migration: `.dual-list-row.selected` used to read
    // `var(--surface-2, #eef)`, and a literal hex is unreachable from any theme
    // block in tokens.css.
    const css = readCss('primitives.css')
    for (const s of SURFACES) {
      for (const [selector, prop] of [
        [s.tintSelector, 'background'],
        [s.inkSelector, 'color'],
      ] as const) {
        const value = declarationsFor(css, selector)[prop]
        expect(value, `${selector} declares no ${prop}`).toBeTruthy()
        expect(value, `${selector} ${prop} is not a token`).toMatch(/^var\(--[\w-]+\)$/)
      }
    }
  })

  it('routes active-state text through --signal-ink rather than per-theme patches', () => {
    // `html[data-theme="dark"] .foo.active { color: … }` was how the picker used
    // to cope with --signal-deep being unreadable on dark paper. One token per
    // theme replaces all of those, and a patch coming back is a regression.
    const css = readCss('primitives.css')
    const patched = [...css.matchAll(/html\[data-theme="[^\]]+"\]\s+([^{\n]+)/g)]
      .map((m) => m[1].trim())
      .filter((sel) => /dual-list|tool-row|src-item/.test(sel))
    expect(patched, 'per-theme override for a picker surface').toEqual([])

    for (const selector of [
      '.dual-list-row.selected .dual-list-row-label',
      '.tool-row.active',
      '.src-item.active',
    ]) {
      expect(resolve(selector, 'color', null)).toBe(tokenColor('--signal-ink', null))
    }
  })
})

/**
 * Text that is not a selection tint: section heads, row metadata, badges.
 * These are the rules that reached for `--ink-4`, the palette's faintest text
 * tier, and measured 2.3–4.0:1 — under WCAG AA in all four themes. `--ink-4`
 * stays correct for placeholders and log timestamps elsewhere in the console,
 * so the fix is per-rule, not a token change.
 */
type TextSurface = {
  name: string
  selector: string
  /** Opaque pane this text is painted on. */
  backdrop: string
}

const TEXT_SURFACES: TextSurface[] = [
  { name: 'Tool list — section head', selector: '.tool-section-head', backdrop: '--paper-deep' },
  {
    name: 'Tool sources — header label',
    selector: '.md-sources-header span',
    backdrop: '--paper-deep',
  },
  { name: 'Tool sources — rail item', selector: '.src-item', backdrop: '--paper-deep' },
  { name: 'Tool list — row index', selector: '.tool-row .idx', backdrop: '--paper-2' },
  { name: 'Tool list — row meta', selector: '.tool-row .tool-meta', backdrop: '--paper-2' },
  {
    name: 'Tool list — disabled row name',
    selector: '.tool-row-off .tool-name',
    backdrop: '--paper-2',
  },
  {
    name: 'Tool list — disabled row meta',
    selector: '.tool-row-off .tool-meta',
    backdrop: '--paper-2',
  },
  { name: 'Tool detail — hint text', selector: '.tool-hint', backdrop: '--paper-2' },
  { name: 'Tool detail — FQN', selector: '.tool-fqn', backdrop: '--paper-2' },
  { name: 'Tool detail — read-only note', selector: '.tool-readonly', backdrop: '--paper-2' },
  // Opaque color-mix chip: stays put whether it sits on the pane or on an
  // active row's tint, which a translucent --signal-haze badge did not.
  { name: 'Tool list — NEW badge', selector: '.tool-new-badge', backdrop: '--paper-2' },
  { name: 'Picker — pane header', selector: '.dual-list-pane-header', backdrop: '--paper' },
  { name: 'Picker — row label', selector: '.dual-list-row-label', backdrop: '--paper' },
  { name: 'Picker — row description', selector: '.dual-list-row-desc', backdrop: '--paper' },
  { name: 'Picker — missing-item marker', selector: '.dual-list-missing', backdrop: '--paper' },
  { name: 'Notice — default', selector: '.notice', backdrop: '--paper-2' },
]

/**
 * What the browser paints behind this text: the rule's own background (which
 * may be translucent, or a color-mix chip) flattened onto the pane beneath.
 */
function paintedBackground(selector: string, backdrop: string, theme: string | null): string {
  const decls = declarationsFor(readCss('primitives.css'), selector)
  const page = tokenColor(backdrop, theme)
  const own = decls.background ?? decls['background-color']
  if (!own) return page
  const resolved = resolveColor(own, paletteFor(theme))
  return resolved === 'transparent' ? page : composite(resolved, page)
}

describe('tools-tab and picker text', () => {
  it.each(ALL_THEMES.map((t) => [themeName(t), t] as const))(
    'clears WCAG AA in the %s theme',
    (label, theme) => {
      for (const s of TEXT_SURFACES) {
        const ink = resolve(s.selector, 'color', theme)
        const bg = paintedBackground(s.selector, s.backdrop, theme)
        const ratio = contrast(ink, bg)
        expect(
          ratio,
          `${s.name} fails WCAG AA in the ${label} theme: ${ink} on ${bg}`,
        ).toBeGreaterThanOrEqual(WCAG_AA)
      }
    },
  )
})
