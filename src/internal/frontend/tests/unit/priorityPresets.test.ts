import { describe, expect, it } from 'vitest'
import { applyPreset, PRESETS } from '../../src/lib/domain/priorityPresets'

describe('applyPreset', () => {
  it('promove só o que já está na lista', () => {
    expect(applyPreset(['hevc', 'av1', 'h.264', 'xvid'], ['h.264'])).toEqual([
      'h.264',
      'hevc',
      'av1',
      'xvid',
    ])
  })

  it('preserva token adicionado à mão, empurrando para baixo', () => {
    expect(applyPreset(['hevc', 'vp9', 'h.264'], ['h.264'])).toEqual(['h.264', 'hevc', 'vp9'])
  })

  it('é no-op para token que não existe na lista', () => {
    const list = ['hevc', 'av1']
    expect(applyPreset(list, ['h.264'])).toEqual(list)
  })

  it('promove na ordem do preset', () => {
    expect(applyPreset(['h.264', 'hevc', 'av1'], ['av1', 'hevc'])).toEqual(['av1', 'hevc', 'h.264'])
  })

  it('todo token de preset existe na lista default de codecs', () => {
    // Espelha o default do backend (nyaa.DefaultPriorities), garantido lá pelo teste
    // TestDefaultPriorities_TokensAreReachable.
    const defaults = ['h.264', 'hevc', 'av1', 'xvid']
    for (const p of PRESETS.codecs ?? []) {
      for (const token of p.first) expect(defaults).toContain(token)
    }
  })
})
