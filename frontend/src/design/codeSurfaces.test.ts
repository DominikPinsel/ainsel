import { describe, expect, it } from 'vitest'

import {
  THEMES,
  contrast,
  luminance,
  declarationsFor,
  paletteFor,
  readCss,
  resolveVar,
  surfaceColors,
} from './cssThemeKit'

// The payload / markdown well is an always-dark element, like the sidebar chrome:
// a JSON block that flips to white in a dark theme does not read as code, and
// `background: var(--ink)` is exactly that flip. These assertions check the
// rendered colours rather than the source text, so they still bite if someone
// reintroduces the pair under a different token name.

const ALWAYS_DARK_SURFACES = ['pre.code', '.md-body pre', '.example-block textarea'] as const

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
