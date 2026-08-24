import { describe, it, expect } from 'vitest'
import { passErrorMessage } from '../../src/lib/domain/passError.js'

describe('passErrorMessage', () => {
  it('dá uma frase própria a cada causa conhecida', () => {
    const codes = ['anilist', 'setup', 'library', 'torrent_backend', 'config', 'storage']
    const messages = codes.map(passErrorMessage)
    expect(new Set(messages).size).toBe(codes.length)
    for (const text of messages) expect(text.length).toBeGreaterThan(0)
  })

  it('não vaza jargão: nenhuma frase cita status HTTP nem JSON', () => {
    // O bug que isto conserta era o banner exibindo `err.Error()` cru — na prática, o corpo
    // JSON da resposta da AniList inteiro.
    for (const code of ['anilist', 'setup', 'library', 'torrent_backend', 'config', 'storage']) {
      expect(passErrorMessage(code)).not.toMatch(/\{|\}|status code|40\d|50\d/)
    }
  })

  it('cai numa frase legível em código desconhecido, nunca em branco', () => {
    // Um backend mais novo que o frontend não pode virar banner vazio.
    expect(passErrorMessage('codigo_que_nao_existe').length).toBeGreaterThan(0)
    expect(passErrorMessage('unknown').length).toBeGreaterThan(0)
  })
})
