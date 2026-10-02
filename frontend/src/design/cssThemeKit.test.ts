import { describe, expect, it } from 'vitest'

import {
  composite,
  contrast,
  declarationsFor,
  expandVars,
  parseColor,
  parseRules,
  resolveColor,
} from './cssThemeKit'

// Ground truth for color-mix() read out of headless Chrome:
// getComputedStyle(el).backgroundColor for each expression below, which Chrome
// reports as `color(srgb r g b / a)` with 0-1 components. Pinned here so the
// evaluator in cssThemeKit is checked against the engine rather than against
// its own assumptions.
const CHROME_COLOR_MIX: [expr: string, r: number, g: number, b: number, a: number][] = [
  ['color-mix(in srgb, #0c8a8f 12%, #ffffff)', 0.885647, 0.944941, 0.947294, 1],
  ['color-mix(in srgb, #e08a34 12%, #3a3327)', 0.305569, 0.240941, 0.159059, 1],
  // A translucent operand exercises premultiplied-alpha mixing, where a naive
  // weighted average of the channels gives a different answer.
  ['color-mix(in srgb, rgba(12,138,143,0.5) 40%, #ffffff)', 0.761065, 0.884957, 0.889873, 0.800784],
  ['color-mix(in srgb, #d8912f 14%, #1e1810)', 0.219765, 0.160549, 0.0797647, 1],
  ['color-mix(in srgb, #3ec7c9 13%, #141b23)', 0.0998431, 0.193569, 0.221882, 1],
]

describe('resolveColor', () => {
  it.each(CHROME_COLOR_MIX)('matches Chrome for %s', (expr, r, g, b, a) => {
    const c = parseColor(resolveColor(expr, new Map()))
    expect(c.r).toBeCloseTo(r * 255, 0)
    expect(c.g).toBeCloseTo(g * 255, 0)
    expect(c.b).toBeCloseTo(b * 255, 0)
    expect(c.a).toBeCloseTo(a, 2)
  })

  it('expands var() nested inside color-mix()', () => {
    const pal = new Map([
      ['--signal', '#0c8a8f'],
      ['--paper-2', '#ffffff'],
    ])
    expect(
      parseColor(resolveColor('color-mix(in srgb, var(--signal) 12%, var(--paper-2))', pal)),
    ).toEqual(parseColor(resolveColor('color-mix(in srgb, #0c8a8f 12%, #ffffff)', new Map())))
  })

  it('treats an omitted second weight as 100% minus the first', () => {
    const pal = new Map([
      ['--a', '#000000'],
      ['--b', '#ffffff'],
    ])
    expect(resolveColor('color-mix(in srgb, var(--a) 30%, var(--b))', pal)).toBe(
      resolveColor('color-mix(in srgb, var(--a) 30%, var(--b) 70%)', pal),
    )
  })

  it('passes plain tokens and literals through', () => {
    const pal = new Map([['--signal-haze', 'rgba(12, 138, 143, 0.1)']])
    expect(resolveColor('var(--signal-haze)', pal)).toBe('rgba(12, 138, 143, 0.1)')
    expect(resolveColor('#abc', new Map())).toBe('#abc')
  })

  it('rejects a form it cannot evaluate rather than guessing', () => {
    expect(() => resolveColor('color-mix(in lab, red, blue)', new Map())).toThrow(/unsupported/)
  })
})

describe('expandVars', () => {
  it('expands every occurrence, not just a wholly-var value', () => {
    const pal = new Map([
      ['--a', '#111111'],
      ['--b', '#222222'],
    ])
    expect(expandVars('1px solid var(--a) var(--b)', pal)).toBe('1px solid #111111 #222222')
  })

  it('names the unknown token it could not resolve', () => {
    expect(() => expandVars('var(--nope)', new Map())).toThrow(/--nope/)
  })
})

describe('declarationsFor', () => {
  const css = `
    .tool-row-off .tool-meta { color: var(--ink-4); }
    .tool-row .tool-meta { color: var(--ink-3); font-size: 10px; }
    a:hover, b:hover { color: red; }
  `

  it('matches a whole selector, not a substring', () => {
    // indexOf('.tool-meta {') would hit the `.tool-row-off` rule first and
    // silently report the wrong colour.
    expect(declarationsFor(css, '.tool-row .tool-meta').color).toBe('var(--ink-3)')
  })

  it('finds a selector that is one entry of a selector list', () => {
    expect(declarationsFor(css, 'b:hover').color).toBe('red')
  })

  it('reports a selector that genuinely has no rule', () => {
    expect(() => declarationsFor(css, '.tool-meta')).toThrow(/no rule for "\.tool-meta"/)
  })
})

describe('parseRules', () => {
  it('refuses conditional group rules rather than flattening them', () => {
    // A flattened @media would apply its declarations unconditionally; a
    // skipped one would hide them. Neither is acceptable, so it throws.
    expect(() => parseRules('@media (min-width: 1px) { .a { color: red } }')).toThrow(/@media/)
    expect(() =>
      parseRules('.a { color: red }\n@supports (display: grid) { .b { color: blue } }'),
    ).toThrow(/@supports/)
  })

  it('drops at-rules whose body is not selector rules', () => {
    // @keyframes stops would otherwise be read as selectors.
    const rules = parseRules(
      '@keyframes spin { from { opacity: 0 } to { opacity: 1 } }\n.a { color: red }',
    )
    expect(rules.map((r) => r.selectors)).toEqual([['.a']])
    expect(parseRules('@import url("x.css");\n.a { color: red }')).toHaveLength(1)
  })

  it('does not mistake an at-rule mentioned in a value for one', () => {
    const rules = parseRules('.a { content: "@media"; color: red }')
    expect(rules).toHaveLength(1)
    expect(rules[0].declarations.color).toBe('red')
  })

  it('parses flat rules in source order', () => {
    const rules = parseRules('/* c */ .a { color: red } .b, .c { color: blue }')
    expect(rules.map((r) => r.selectors)).toEqual([['.a'], ['.b', '.c']])
  })
})

describe('composite and contrast', () => {
  it('flattens a translucent tint onto an opaque backdrop', () => {
    // 12*0.1 + 255*0.9 = 231 = 0xe7, and likewise for the other channels.
    expect(composite('rgba(12, 138, 143, 0.1)', '#ffffff')).toBe('#e7f3f4')
  })

  it('leaves an opaque colour alone', () => {
    expect(composite('#123456', '#ffffff')).toBe('#123456')
  })

  it('is symmetric and peaks at black on white', () => {
    expect(contrast('#123456', '#ffffff')).toBeCloseTo(contrast('#ffffff', '#123456'), 10)
    expect(contrast('#000000', '#ffffff')).toBeCloseTo(21, 5)
  })
})
