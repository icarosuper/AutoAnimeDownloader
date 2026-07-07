import { describe, it, expect } from 'vitest'
import { filterAnimes, sortAnimes, computeNextCheckIn, formatBytes, isDiskSpaceLow } from '../../src/lib/utils/status.js'
import type { AnimeInfo } from '../../src/lib/api/client.js'

function makeAnime(overrides: Partial<AnimeInfo> = {}): AnimeInfo {
  return {
    anime_id: 1,
    name: 'Test Anime',
    episodes_downloaded: 0,
    episodes_released: 12,
    episodes_watched: 0,
    total_episodes: 24,
    latest_episode_id: 0,
    last_download_date: '2026-01-01T00:00:00Z',
    cover_image: undefined,
    is_blacklisted: false,
    ...overrides,
  }
}

describe('filterAnimes', () => {
  it('returns all animes when search is empty and filterUnwatched is false', () => {
    const animes = [makeAnime({ name: 'Naruto' }), makeAnime({ name: 'Bleach' })]
    expect(filterAnimes(animes, '', false)).toHaveLength(2)
  })

  it('filters by name (case-insensitive)', () => {
    const animes = [makeAnime({ name: 'Naruto' }), makeAnime({ name: 'Bleach' })]
    expect(filterAnimes(animes, 'naru', false)).toHaveLength(1)
    expect(filterAnimes(animes, 'BLEACH', false)).toHaveLength(1)
    expect(filterAnimes(animes, 'xyz', false)).toHaveLength(0)
  })

  it('filters out fully-watched animes when filterUnwatched is true', () => {
    const animes = [
      makeAnime({ name: 'Watched', episodes_watched: 12, episodes_released: 12 }),
      makeAnime({ name: 'Unwatched', episodes_watched: 3, episodes_released: 12 }),
      makeAnime({ name: 'No episodes', episodes_watched: 0, episodes_released: 0 }),
    ]
    const result = filterAnimes(animes, '', true)
    expect(result.map(a => a.name)).toEqual(['Unwatched', 'No episodes'])
  })

  it('combines search + filterUnwatched', () => {
    const animes = [
      makeAnime({ name: 'Naruto done', episodes_watched: 12, episodes_released: 12 }),
      makeAnime({ name: 'Naruto ongoing', episodes_watched: 5, episodes_released: 12 }),
      makeAnime({ name: 'Bleach', episodes_watched: 5, episodes_released: 12 }),
    ]
    const result = filterAnimes(animes, 'naruto', true)
    expect(result).toHaveLength(1)
    expect(result[0].name).toBe('Naruto ongoing')
  })
})

describe('sortAnimes', () => {
  const a = makeAnime({ name: 'Aaa', episodes_watched: 1, last_download_date: '2026-01-03T00:00:00Z' })
  const b = makeAnime({ name: 'Bbb', episodes_watched: 3, last_download_date: '2026-01-01T00:00:00Z' })
  const c = makeAnime({ name: 'Ccc', episodes_watched: 2, last_download_date: '2026-01-02T00:00:00Z' })

  it('sorts by name asc', () => {
    const result = sortAnimes([c, a, b], 'name', 'asc')
    expect(result.map(x => x.name)).toEqual(['Aaa', 'Bbb', 'Ccc'])
  })

  it('sorts by name desc', () => {
    const result = sortAnimes([a, c, b], 'name', 'desc')
    expect(result.map(x => x.name)).toEqual(['Ccc', 'Bbb', 'Aaa'])
  })

  it('sorts by episodes_watched asc', () => {
    const result = sortAnimes([b, c, a], 'episodes_watched', 'asc')
    expect(result.map(x => x.episodes_watched)).toEqual([1, 2, 3])
  })

  it('sorts by last_download_date desc (newest first)', () => {
    const result = sortAnimes([b, c, a], 'last_download_date', 'desc')
    expect(result[0].name).toBe('Aaa') // 2026-01-03 is newest
    expect(result[2].name).toBe('Bbb') // 2026-01-01 is oldest
  })

  it('does not mutate the input array', () => {
    const input = [b, a]
    sortAnimes(input, 'name', 'asc')
    expect(input[0].name).toBe('Bbb')
  })
})

describe('computeNextCheckIn', () => {
  const now = new Date('2026-01-01T01:00:00Z').getTime()

  it('returns null when status is stopped', () => {
    expect(computeNextCheckIn('2026-01-01T00:00:00Z', 60, 'stopped', now)).toBeNull()
  })

  it('returns null when lastCheck is empty', () => {
    expect(computeNextCheckIn('', 60, 'running', now)).toBeNull()
  })

  it('returns null when lastCheck is before 2010', () => {
    expect(computeNextCheckIn('2009-01-01T00:00:00Z', 60, 'running', now)).toBeNull()
  })

  it('returns "soon" when next check is overdue', () => {
    // last check was 1h ago, interval is 30min → overdue
    expect(computeNextCheckIn('2026-01-01T00:00:00Z', 30, 'running', now)).toBe('soon')
  })

  it('returns remaining time as "Xm Ys"', () => {
    // last check 10 minutes ago, interval 60 → 50 minutes left
    const lastCheck = new Date('2026-01-01T00:50:00Z').toISOString()
    const result = computeNextCheckIn(lastCheck, 60, 'running', now)
    expect(result).toBe('50m 0s')
  })

  it('returns remaining time as "Xs" when under 1 minute', () => {
    // last check 59m 30s ago, interval 60 → 30s left
    const lastCheck = new Date(now - (59 * 60 + 30) * 1000).toISOString()
    const result = computeNextCheckIn(lastCheck, 60, 'running', now)
    expect(result).toBe('30s')
  })
})

describe('formatBytes', () => {
  it('formats bytes as GB with one decimal', () => {
    expect(formatBytes(500_107_862_016)).toBe('465.8 GB')
  })

  it('returns 0.0 GB for zero', () => {
    expect(formatBytes(0)).toBe('0.0 GB')
  })
})

describe('isDiskSpaceLow', () => {
  it('is true when free/total is below 10%', () => {
    expect(isDiskSpaceLow(5, 100)).toBe(true)
  })

  it('is false when free/total is at or above 10%', () => {
    expect(isDiskSpaceLow(10, 100)).toBe(false)
  })

  it('is false when total is 0 (no data)', () => {
    expect(isDiskSpaceLow(0, 0)).toBe(false)
  })
})
