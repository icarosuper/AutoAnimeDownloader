import { describe, it, expect } from 'vitest'
import { passErrorMessage } from '../../src/lib/domain/passError.js'

const KNOWN = ['anilist', 'mal', 'setup', 'library', 'torrent_backend', 'config', 'storage']

describe('passErrorMessage', () => {
  it('gives each known cause its own sentence', () => {
    const messages = KNOWN.map(passErrorMessage)
    expect(new Set(messages).size).toBe(KNOWN.length)
    for (const text of messages) expect(text.length).toBeGreaterThan(0)
  })

  it('leaks no jargon: no sentence mentions an HTTP status or JSON', () => {
    // The bug this fixes was the banner showing the raw `err.Error()` — in practice, AniList's
    // whole JSON response body.
    for (const code of KNOWN) {
      expect(passErrorMessage(code)).not.toMatch(/\{|\}|status code|40\d|50\d/)
    }
  })

  it('falls back to a readable sentence on an unknown code, never a blank', () => {
    // A backend newer than the frontend must not become an empty banner.
    expect(passErrorMessage('code_that_does_not_exist').length).toBeGreaterThan(0)
    expect(passErrorMessage('unknown').length).toBeGreaterThan(0)
  })
})
