import { describe, expect, it } from 'vitest'
import { applyPreset, PRESETS } from '../../src/lib/domain/priorityPresets'

describe('applyPreset', () => {
  it('promotes only what is already in the list', () => {
    expect(applyPreset(['hevc', 'av1', 'h.264', 'xvid'], ['h.264'])).toEqual([
      'h.264',
      'hevc',
      'av1',
      'xvid',
    ])
  })

  it('keeps a hand-added token, pushing it down', () => {
    expect(applyPreset(['hevc', 'vp9', 'h.264'], ['h.264'])).toEqual(['h.264', 'hevc', 'vp9'])
  })

  it('is a no-op for a token not in the list', () => {
    const list = ['hevc', 'av1']
    expect(applyPreset(list, ['h.264'])).toEqual(list)
  })

  it("promotes in the preset's order", () => {
    expect(applyPreset(['h.264', 'hevc', 'av1'], ['av1', 'hevc'])).toEqual(['av1', 'hevc', 'h.264'])
  })

  it('every preset token exists in the default codecs list', () => {
    // Mirrors the backend default (nyaa.DefaultPriorities), guaranteed there by
    // TestDefaultPriorities_TokensAreReachable.
    const defaults = ['h.264', 'hevc', 'av1', 'xvid']
    for (const p of PRESETS.codecs ?? []) {
      for (const token of p.first) expect(defaults).toContain(token)
    }
  })
})
