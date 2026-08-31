import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import path from 'node:path'

/**
 * `src/lib/design/tokens.css` é a fonte da verdade ÚNICA das cores, desde que o daisyUI saiu.
 *
 * Este arquivo já teve mais dois testes, que comparavam cada cor daqui com a cópia em hex que
 * o tema do daisyUI mantinha em `tailwind.config.js` — ele compilava as chaves de tema para
 * OKLCH em build time e não conseguia consumir `var(--x)`, então cada cor existia duas vezes e
 * podia divergir. Com o daisyUI removido não há segunda cópia, e os dois testes foram apagados
 * junto com o risco que vigiavam.
 *
 * O que sobra é o invariante que continua valendo: os dois blocos de tema precisam existir e
 * estar preenchidos. Sem um deles, um tema inteiro cai silenciosamente para os valores do outro.
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

  it('define os dois blocos de tema, com tokens dentro', () => {
    expect(Object.keys(tokenThemes['aad-dark']).length).toBeGreaterThan(10)
    expect(Object.keys(tokenThemes['aad-light']).length).toBeGreaterThan(10)
  })

  // Um token que existe num tema e falta no outro não quebra o build: a tela só herda o valor
  // do :root e destoa em silêncio no tema incompleto.
  it('define exatamente as mesmas chaves nos dois temas', () => {
    const dark = Object.keys(tokenThemes['aad-dark']).sort()
    const light = Object.keys(tokenThemes['aad-light']).sort()
    expect(light).toEqual(dark)
  })
})
