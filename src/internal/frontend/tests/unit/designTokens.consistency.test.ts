import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import path from 'node:path'

/**
 * `src/lib/design/tokens.css` is the SINGLE source of truth for colors, since daisyUI left.
 *
 * This file used to have two more tests, which compared each color here with the hex copy that
 * daisyUI's theme kept in `tailwind.config.js` — it compiled theme keys to OKLCH at build time
 * and couldn't consume `var(--x)`, so each color existed twice and could diverge. With daisyUI
 * removed there is no second copy, and the two tests were deleted along with the risk they
 * watched.
 *
 * What remains is the invariant that still holds: both theme blocks need to exist and be
 * filled. Without one of them, a whole theme silently falls back to the other's values.
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

describe('tokens.css', () => {
  const css = stripComments(readFileSync(TOKENS_CSS_PATH, 'utf-8'))
  const tokenThemes = {
    'aad-dark': parseThemeBlock(css, 'aad-dark'),
    'aad-light': parseThemeBlock(css, 'aad-light'),
  }

  it('defines both theme blocks, with tokens inside', () => {
    expect(Object.keys(tokenThemes['aad-dark']).length).toBeGreaterThan(10)
    expect(Object.keys(tokenThemes['aad-light']).length).toBeGreaterThan(10)
  })

  // A token that exists in one theme and is missing from the other doesn't break the build:
  // the screen just inherits the :root value and silently mismatches on the incomplete theme.
  it('defines exactly the same keys in both themes', () => {
    const dark = Object.keys(tokenThemes['aad-dark']).sort()
    const light = Object.keys(tokenThemes['aad-light']).sort()
    expect(light).toEqual(dark)
  })
})
