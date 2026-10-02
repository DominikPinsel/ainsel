import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import { describe, expect, it } from 'vitest'

/**
 * Design-token guard.
 *
 * The Fjord overhaul (af55f3d) renamed the palette but left a residue of
 * `var(--old-token, #hardcoded)` references behind. Those tokens no longer
 * exist, so the fallback renders instead — off-palette colours that no theme
 * can reach, and in the dual-list picker a near-white selected row that is
 * unreadable in every dark theme.
 *
 * This spec makes that class of bug impossible to reintroduce: any
 * `var(--x)` whose `--x` is not declared in `src/design/` fails the build.
 *
 * Files that predate the guard and are not part of this change are listed in
 * BASELINE. It is a ratchet — shrink it, never grow it. When you migrate a
 * file, delete its entry; adding one back is a review-time red flag.
 */

const SRC = join(__dirname, '..')
const DESIGN = join(SRC, 'design')

/** Files still carrying pre-Fjord phantom tokens. Remove entries as you migrate. */
const BASELINE: Record<string, string[]> = {
  'layout/Layout.css': ['--chrome-top'],
  'pages/agents/AgentList.tsx': ['--accent'],
  'pages/agents/AgentWizard.tsx': ['--rule'],
  'pages/agents/ScheduleForm.tsx': ['--rule'],
  'pages/agents/TriggerForm.tsx': ['--rule'],
  'pages/chat/ChatView.css': ['--chrome-top'],
  'pages/chat/ChatView.tsx': ['--accent', '--border', '--font-body', '--ink-1', '--surface-2'],
  'pages/chat/chatBusy.css': ['--accent'],
  'pages/connectors/ConnectorForm.tsx': ['--rule'],
  'pages/connectors/ConnectorList.tsx': ['--accent'],
  'pages/groups/GroupDetail.tsx': ['--chrome-1'],
  'pages/groups/GroupList.tsx': ['--accent'],
  'pages/images/ImageList.tsx': ['--accent'],
  'pages/mcp-servers/MCPServerList.tsx': ['--accent'],
  'pages/observability/events/EventView.tsx': ['--border'],
  'pages/personas/PersonaList.tsx': ['--accent'],
  'pages/profile/TokenManager.tsx': ['--border', '--danger', '--surface-2'],
  'pages/skills/SkillList.tsx': ['--accent'],
  'pages/users/UserList.tsx': ['--accent', '--muted'],
}

const SCAN_EXT = new Set(['.css', '.ts', '.tsx'])

/**
 * Specs are skipped: they quote `var(--…)` as fixtures and assertions, never
 * as styling, so a token name in a test file renders nothing.
 */
function isSpec(file: string): boolean {
  return /\.test\.tsx?$/.test(file)
}

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      if (entry === 'node_modules' || entry === 'dist') continue
      walk(full, out)
    } else if (SCAN_EXT.has(entry.slice(entry.lastIndexOf('.'))) && !isSpec(entry)) {
      out.push(full)
    }
  }
  return out
}

/** Every custom property declared anywhere under src/design/. */
function declaredTokens(): Set<string> {
  const declared = new Set<string>()
  for (const file of readdirSync(DESIGN)) {
    if (!file.endsWith('.css')) continue
    const css = readFileSync(join(DESIGN, file), 'utf8')
    for (const m of css.matchAll(/(--[a-zA-Z0-9-]+)\s*:/g)) declared.add(m[1])
  }
  return declared
}

/** `var(--x)` references per file, excluding declarations. */
function referencedTokens(file: string): Set<string> {
  const text = readFileSync(file, 'utf8')
  const refs = new Set<string>()
  for (const m of text.matchAll(/var\(\s*(--[a-zA-Z0-9-]+)/g)) refs.add(m[1])
  return refs
}

function phantomTokens(): Map<string, string[]> {
  const declared = declaredTokens()
  const found = new Map<string, string[]>()
  for (const file of walk(SRC)) {
    const rel = relative(SRC, file)
    const phantoms = [...referencedTokens(file)].filter((t) => !declared.has(t)).sort()
    if (phantoms.length > 0) found.set(rel, phantoms)
  }
  return found
}

describe('design tokens', () => {
  it('declares a non-trivial palette (sanity check on the scanner)', () => {
    expect(declaredTokens().size).toBeGreaterThan(30)
    expect(declaredTokens()).toContain('--signal')
    expect(declaredTokens()).toContain('--paper')
  })

  it('has no var(--x) references to undeclared tokens beyond the baseline', () => {
    const actual = phantomTokens()

    // New offenders: a file that is not in the baseline, or a token that the
    // baseline does not excuse for that file.
    const introduced: string[] = []
    for (const [file, tokens] of actual) {
      const excused = new Set(BASELINE[file] ?? [])
      const fresh = tokens.filter((t) => !excused.has(t))
      if (fresh.length > 0) introduced.push(`${file}: ${fresh.join(', ')}`)
    }

    expect(
      introduced,
      'Undeclared design tokens resolve to their hardcoded fallback, which no ' +
        'theme can override. Declare the token in src/design/tokens.css or use ' +
        'an existing one.',
    ).toEqual([])

    // Ratchet: a baseline entry that no longer matches reality means the file
    // was migrated (good) or renamed (stale). Either way, update the baseline.
    const stale: string[] = []
    for (const [file, tokens] of Object.entries(BASELINE)) {
      const now = new Set(actual.get(file) ?? [])
      const gone = tokens.filter((t) => !now.has(t))
      if (gone.length > 0) stale.push(`${file}: ${gone.join(', ')}`)
    }
    expect(
      stale,
      'These baseline entries no longer match the source. Delete them so the ' +
        'ratchet stays tight.',
    ).toEqual([])
  })
})
