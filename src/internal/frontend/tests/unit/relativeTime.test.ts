import { describe, expect, it } from 'vitest'
import { formatRelative, nextAiringIn } from '../../src/lib/utils/relativeTime.js'

const NOW = 1_700_000_000_000

describe('formatRelative', () => {
  it('formata passado e futuro com a mesma conta', () => {
    expect(formatRelative(NOW - 3 * 3600_000, NOW, 'en')).toBe('3 hours ago')
    expect(formatRelative(NOW + 3 * 3600_000, NOW, 'en')).toBe('in 3 hours')
  })

  it('trunca para zero, nao arredonda (25h continua "1 dia")', () => {
    expect(formatRelative(NOW - 25 * 3600_000, NOW, 'en')).toBe('yesterday')
    expect(formatRelative(NOW + 25 * 3600_000, NOW, 'en')).toBe('tomorrow')
  })

  it('escolhe a unidade pela magnitude', () => {
    expect(formatRelative(NOW + 30_000, NOW, 'en')).toBe('in 30 seconds')
    expect(formatRelative(NOW + 45 * 60_000, NOW, 'en')).toBe('in 45 minutes')
    expect(formatRelative(NOW + 5 * 86400_000, NOW, 'en')).toBe('in 5 days')
  })
})

describe('nextAiringIn', () => {
  it('converte o unix em SEGUNDOS da API', () => {
    expect(nextAiringIn(NOW / 1000 + 2 * 86400, NOW, 'en')).toBe('in 2 days')
  })

  it('some quando nao ha episodio agendado', () => {
    expect(nextAiringIn(undefined, NOW, 'en')).toBe('')
    expect(nextAiringIn(0, NOW, 'en')).toBe('')
  })

  it('some quando o horario ja passou — a AniList atrasa a publicacao do proximo', () => {
    expect(nextAiringIn(NOW / 1000 - 60, NOW, 'en')).toBe('')
  })
})
