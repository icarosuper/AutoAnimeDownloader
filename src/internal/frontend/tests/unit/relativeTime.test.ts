import { describe, expect, it } from 'vitest'
import { formatRelative, nextAiringIn } from '../../src/lib/utils/relativeTime.js'

const NOW = 1_700_000_000_000

describe('formatRelative', () => {
  it('formats past and future with the same math', () => {
    expect(formatRelative(NOW - 3 * 3600_000, NOW, 'en')).toBe('3 hours ago')
    expect(formatRelative(NOW + 3 * 3600_000, NOW, 'en')).toBe('in 3 hours')
  })

  it('truncates towards zero, does not round (25h stays "1 day")', () => {
    expect(formatRelative(NOW - 25 * 3600_000, NOW, 'en')).toBe('yesterday')
    expect(formatRelative(NOW + 25 * 3600_000, NOW, 'en')).toBe('tomorrow')
  })

  it('picks the unit by magnitude', () => {
    expect(formatRelative(NOW + 30_000, NOW, 'en')).toBe('in 30 seconds')
    expect(formatRelative(NOW + 45 * 60_000, NOW, 'en')).toBe('in 45 minutes')
    expect(formatRelative(NOW + 5 * 86400_000, NOW, 'en')).toBe('in 5 days')
  })
})

describe('nextAiringIn', () => {
  it('converts the API unix in SECONDS', () => {
    expect(nextAiringIn(NOW / 1000 + 2 * 86400, NOW, 'en')).toBe('in 2 days')
  })

  it('vanishes when there is no scheduled episode', () => {
    expect(nextAiringIn(undefined, NOW, 'en')).toBe('')
    expect(nextAiringIn(0, NOW, 'en')).toBe('')
  })

  it('vanishes when the time has already passed — AniList delays publishing the next one', () => {
    expect(nextAiringIn(NOW / 1000 - 60, NOW, 'en')).toBe('')
  })
})
