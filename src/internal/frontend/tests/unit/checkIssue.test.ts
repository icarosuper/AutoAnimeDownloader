import { describe, it, expect } from 'vitest'
import { issueMessage, batchNote } from '../../src/lib/domain/checkIssue.js'
import type { Issue } from '../../src/lib/api/client.js'

function issue(overrides: Partial<Issue> = {}): Issue {
  return { anime_id: 1, anime_name: 'Bleach', code: 'no_torrent_found', ...overrides }
}

describe('issueMessage', () => {
  it('interpola candidatos e teto no problema de tamanho', () => {
    const text = issueMessage(issue({ code: 'all_above_size_limit', candidates: 8, limit_gb: 3 }))
    expect(text).toContain('8')
    expect(text).toContain('3')
  })

  it('interpola candidatos e piso de seeders', () => {
    const text = issueMessage(issue({ code: 'no_seeders', candidates: 4, min_seeders: 5 }))
    expect(text).toContain('4')
    expect(text).toContain('5')
  })

  it('interpola baixados e pendentes no limite por anime', () => {
    const text = issueMessage(issue({ code: 'max_episodes_per_anime', downloaded: 12, pending: 35 }))
    expect(text).toContain('12')
    expect(text).toContain('35')
  })

  it('tem frase para cada código conhecido', () => {
    const codes = [
      'all_above_size_limit',
      'no_seeders',
      'no_torrent_found',
      'disk_full',
      'torrent_rejected',
      'max_episodes_per_anime',
    ]
    for (const code of codes) {
      expect(issueMessage(issue({ code })), code).not.toBe('')
    }
  })

  // Um codigo novo no backend nao pode virar linha em branco na tela: o fallback e o proprio
  // codigo, feio e legivel, em vez de silencio.
  it('cai no código cru quando não conhece a frase', () => {
    expect(issueMessage(issue({ code: 'codigo_do_futuro' }))).toContain('codigo_do_futuro')
  })
})

describe('batchNote', () => {
  it('explica cada motivo de batch desligado', () => {
    for (const value of ['no_result', 'above_size_limit', 'no_coverage']) {
      expect(batchNote(issue({ code: 'max_episodes_per_anime', batch_skipped: value })), value).not.toBe('')
    }
  })

  it('é vazio quando o anime nunca foi elegível a batch', () => {
    expect(batchNote(issue({ code: 'max_episodes_per_anime' }))).toBe('')
  })
})
