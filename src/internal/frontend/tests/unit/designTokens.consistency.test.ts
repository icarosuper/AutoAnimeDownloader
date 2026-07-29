import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import tailwindConfig from '../../tailwind.config.js'

/**
 * daisyUI v4 compiles its reserved theme color keys (primary, base-100, success, ...) to
 * OKLCH components at BUILD TIME via `culori.oklch()`, which cannot consume `var(--x)` — so
 * every color daisyUI's theme needs has to be duplicated as a hex literal in
 * `tailwind.config.js`, separate from the CSS custom properties in
 * `src/lib/design/tokens.css` that are the actual source of truth (see phase-0-report.md,
 * "Decisões e desvios" #2).
 *
 * This test closes the drift risk that duplication creates: it parses BOTH real files (no
 * hand-written hex literals here) and asserts every daisyUI key that is *documented* (in the
 * comments right next to it in tailwind.config.js) as mirroring a tokens.css variable still
 * has the same value in both places.
 */

const TOKENS_CSS_PATH = path.resolve(__dirname, '../../src/lib/design/tokens.css')

function stripComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?\*\//g, '')
}

function parseThemeBlock(css: string, selector: string): Record<string, string> {
  const blockMatch = css.match(
    new RegExp(`\\[data-theme=['"]${selector}['"]\\]\\s*\\{([\\s\\S]*?)\\n\\}`)
  )
  if (!blockMatch) {
    throw new Error(`tokens.css: could not find a [data-theme="${selector}"] block`)
  }
  const vars: Record<string, string> = {}
  const varRegex = /--([\w-]+):\s*([^;]+);/g
  let match: RegExpExecArray | null
  while ((match = varRegex.exec(blockMatch[1])) !== null) {
    vars[`--${match[1]}`] = match[2].trim()
  }
  return vars
}

function normalizeHex(value: string): string {
  return value.trim().toLowerCase()
}

function getDaisyuiTheme(themeName: string): Record<string, string> {
  const themes = tailwindConfig.daisyui.themes as Array<Record<string, Record<string, string>>>
  const entry = themes.find((t) => Object.hasOwn(t, themeName))
  if (!entry) {
    throw new Error(`tailwind.config.js: could not find a daisyui theme named "${themeName}"`)
  }
  return entry[themeName]
}

// The mapping itself (which daisyUI key mirrors which tokens.css variable) is a design
// decision documented as comments in tailwind.config.js — it has to be spelled out somewhere
// executable to be checkable at all. What must never be hand-written here are the *values*:
// those are always read live from the two real files below.
const SHARED_COLOR_MAPPING: Array<{ daisyuiKey: string; tokenVar: string }> = [
  { daisyuiKey: 'primary', tokenVar: '--accent' },
  { daisyuiKey: 'secondary', tokenVar: '--accent' },
  { daisyuiKey: 'accent', tokenVar: '--accent' },
  { daisyuiKey: 'primary-content', tokenVar: '--on-accent' },
  { daisyuiKey: 'secondary-content', tokenVar: '--on-accent' },
  { daisyuiKey: 'accent-content', tokenVar: '--on-accent' },
  { daisyuiKey: 'neutral', tokenVar: '--neutral' },
  { daisyuiKey: 'neutral-content', tokenVar: '--on-accent' },
  { daisyuiKey: 'base-100', tokenVar: '--bg-card' },
  { daisyuiKey: 'base-200', tokenVar: '--bg-sunken' },
  { daisyuiKey: 'base-content', tokenVar: '--text-primary' },
  { daisyuiKey: 'info', tokenVar: '--accent' },
  { daisyuiKey: 'info-content', tokenVar: '--on-accent' },
  { daisyuiKey: 'success', tokenVar: '--ok' },
  { daisyuiKey: 'success-content', tokenVar: '--on-ok' },
  { daisyuiKey: 'warning', tokenVar: '--warn' },
  { daisyuiKey: 'warning-content', tokenVar: '--on-warn' },
  { daisyuiKey: 'error', tokenVar: '--danger' },
  { daisyuiKey: 'error-content', tokenVar: '--on-danger' },
  // base-300 is intentionally NOT here: it has no tokens.css counterpart (it's a value
  // mathematically derived for the daisyUI integration, not a spec token) — see the comment
  // above `'base-300':` in tailwind.config.js for both themes.
]

describe('tokens.css <-> tailwind.config.js daisyui theme consistency', () => {
  const css = stripComments(readFileSync(TOKENS_CSS_PATH, 'utf-8'))
  const tokenThemes = {
    'aad-dark': parseThemeBlock(css, 'aad-dark'),
    'aad-light': parseThemeBlock(css, 'aad-light'),
  }

  it('tokens.css actually defines both theme blocks with tokens in them', () => {
    expect(Object.keys(tokenThemes['aad-dark']).length).toBeGreaterThan(10)
    expect(Object.keys(tokenThemes['aad-light']).length).toBeGreaterThan(10)
  })

  for (const themeName of ['aad-dark', 'aad-light'] as const) {
    it(`every shared color in ${themeName} matches between tokens.css and tailwind.config.js`, () => {
      const tokens = tokenThemes[themeName]
      const daisyuiTheme = getDaisyuiTheme(themeName)
      const mismatches: string[] = []

      for (const { daisyuiKey, tokenVar } of SHARED_COLOR_MAPPING) {
        const tokenValue = tokens[tokenVar]
        const daisyuiValue = daisyuiTheme[daisyuiKey]

        if (tokenValue === undefined) {
          mismatches.push(`${tokenVar}: missing from tokens.css [data-theme="${themeName}"]`)
          continue
        }
        if (daisyuiValue === undefined) {
          mismatches.push(
            `${daisyuiKey}: missing from tailwind.config.js daisyui theme "${themeName}"`
          )
          continue
        }
        if (normalizeHex(tokenValue) !== normalizeHex(daisyuiValue)) {
          mismatches.push(
            `${tokenVar} (tokens.css) = "${tokenValue}" but ${daisyuiKey} (tailwind.config.js, "${themeName}") = "${daisyuiValue}"`
          )
        }
      }

      expect(mismatches, mismatches.join('\n')).toEqual([])
    })
  }
})
