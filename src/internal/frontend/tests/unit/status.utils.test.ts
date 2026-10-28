import { describe, it, expect } from 'vitest'
import {
  filterAnimes,
  sortAnimes,
  computeNextCheckIn,
  formatBytes,
  breakdown,
  isAiringStatus,
  isFinishedStatus,
  encodeStatusView,
  decodeStatusView,
  DEFAULT_STATUS_VIEW,
} from '../../src/lib/utils/status.js'
import type { AnimeInfo } from '../../src/lib/api/client.js'

function makeAnime(overrides: Partial<AnimeInfo> = {}): AnimeInfo {
  return {
    anime_id: 1,
    name: 'Test Anime',
    episodes_downloaded: 0,
    episodes_released: 12,
    episodes_watched: 0,
    episodes_pending: 12,
    total_episodes: 24,
    latest_episode_number: 1,
    last_download_date: '2026-01-01T00:00:00Z',
    cover_image: undefined,
    is_blacklisted: false,
    ...overrides,
  }
}

describe('filterAnimes', () => {
  it('returns all animes when search is empty and no filters are on', () => {
    const animes = [makeAnime({ name: 'Naruto' }), makeAnime({ name: 'Bleach' })]
    expect(filterAnimes(animes, '')).toHaveLength(2)
  })

  it('filters by name (case-insensitive)', () => {
    const animes = [makeAnime({ name: 'Naruto' }), makeAnime({ name: 'Bleach' })]
    expect(filterAnimes(animes, 'naru')).toHaveLength(1)
    expect(filterAnimes(animes, 'BLEACH')).toHaveLength(1)
    expect(filterAnimes(animes, 'xyz')).toHaveLength(0)
  })

  it('filters out fully-watched animes when unwatched is true', () => {
    const animes = [
      makeAnime({ name: 'Watched', episodes_watched: 12, episodes_released: 12 }),
      makeAnime({ name: 'Unwatched', episodes_watched: 3, episodes_released: 12 }),
      makeAnime({ name: 'No episodes', episodes_watched: 0, episodes_released: 0 }),
    ]
    const result = filterAnimes(animes, '', { unwatched: true })
    expect(result.map(a => a.name)).toEqual(['Unwatched', 'No episodes'])
  })

  it('combines search + unwatched', () => {
    const animes = [
      makeAnime({ name: 'Naruto done', episodes_watched: 12, episodes_released: 12 }),
      makeAnime({ name: 'Naruto ongoing', episodes_watched: 5, episodes_released: 12 }),
      makeAnime({ name: 'Bleach', episodes_watched: 5, episodes_released: 12 }),
    ]
    const result = filterAnimes(animes, 'naruto', { unwatched: true })
    expect(result).toHaveLength(1)
    expect(result[0].name).toBe('Naruto ongoing')
  })

  it('finds the anime by any alternative title (kanji, romaji, synonym)', () => {
    const animes = [
      makeAnime({ name: 'Attack on Titan', alt_names: ['Shingeki no Kyojin', '進撃の巨人', 'AoT'] }),
      makeAnime({ name: 'Bleach' }),
    ]
    for (const term of ['shingeki', '進撃', 'aot', 'Attack']) {
      expect(filterAnimes(animes, term).map(a => a.name)).toEqual(['Attack on Titan'])
    }
    expect(filterAnimes(animes, 'xyz')).toHaveLength(0)
  })

  it('ignores accent and character width in the term and the title', () => {
    const animes = [makeAnime({ name: 'Kimi no Na wa.', alt_names: ['Ｙｏｕｒ Ｎａｍｅ．', 'Poké'] })]
    expect(filterAnimes(animes, 'your name')).toHaveLength(1)
    expect(filterAnimes(animes, 'poke')).toHaveLength(1)
    expect(filterAnimes(animes, 'pokè')).toHaveLength(1)
  })

  it('keeps only standalone animes when type is standalone', () => {
    const animes = [
      makeAnime({ name: 'Standalone', is_standalone: true }),
      makeAnime({ name: 'From the list' }),
    ]
    expect(filterAnimes(animes, '', { type: 'standalone' }).map(a => a.name)).toEqual(['Standalone'])
    expect(filterAnimes(animes, '', { type: 'all' })).toHaveLength(2)
  })

  const typed = [
    makeAnime({ name: 'Airing', media_status: 'RELEASING' }),
    makeAnime({ name: 'Hiatus', media_status: 'HIATUS' }),
    makeAnime({ name: 'Finished', media_status: 'FINISHED' }),
    makeAnime({ name: 'Cancelled', media_status: 'CANCELLED' }),
    makeAnime({ name: 'Upcoming', media_status: 'NOT_YET_RELEASED' }),
    makeAnime({ name: 'Unknown' }),
    makeAnime({ name: 'Standalone airing', media_status: 'RELEASING', is_standalone: true }),
    makeAnime({ name: 'Blacklisted finished', media_status: 'FINISHED', is_blacklisted: true }),
  ]

  it('keeps airing and hiatus rows for type airing', () => {
    expect(filterAnimes(typed, '', { type: 'airing' }).map(a => a.name)).toEqual([
      'Airing',
      'Hiatus',
      'Standalone airing',
    ])
  })

  it('keeps finished and cancelled rows for type finished', () => {
    expect(filterAnimes(typed, '', { type: 'finished' }).map(a => a.name)).toEqual([
      'Finished',
      'Cancelled',
      'Blacklisted finished',
    ])
  })

  it('keeps blacklisted rows for type blacklisted', () => {
    expect(filterAnimes(typed, '', { type: 'blacklisted' }).map(a => a.name)).toEqual([
      'Blacklisted finished',
    ])
  })

  it('leaves NOT_YET_RELEASED and missing status out of airing and finished', () => {
    expect(filterAnimes(typed, '', { type: 'airing' }).map(a => a.name)).not.toContain('Upcoming')
    expect(filterAnimes(typed, '', { type: 'airing' }).map(a => a.name)).not.toContain('Unknown')
    expect(filterAnimes(typed, '', { type: 'finished' }).map(a => a.name)).not.toContain('Upcoming')
    expect(filterAnimes(typed, '', { type: 'finished' }).map(a => a.name)).not.toContain('Unknown')
    expect(filterAnimes(typed, '', { type: 'all' })).toHaveLength(typed.length)
  })

  it('combines unwatched with each type', () => {
    const animes = [
      makeAnime({ name: 'Airing watched', media_status: 'RELEASING', episodes_watched: 12, episodes_released: 12 }),
      makeAnime({ name: 'Airing unwatched', media_status: 'RELEASING', episodes_watched: 3, episodes_released: 12 }),
      makeAnime({ name: 'Finished unwatched', media_status: 'FINISHED', episodes_watched: 1, episodes_released: 12 }),
    ]
    expect(filterAnimes(animes, '', { unwatched: true, type: 'airing' }).map(a => a.name)).toEqual([
      'Airing unwatched',
    ])
    expect(filterAnimes(animes, '', { unwatched: true, type: 'finished' }).map(a => a.name)).toEqual([
      'Finished unwatched',
    ])
  })

  it('combines search, type and unwatched', () => {
    const animes = [
      makeAnime({ name: 'Beta airing', media_status: 'RELEASING', episodes_watched: 1, episodes_released: 8 }),
      makeAnime({ name: 'Beta leftover', media_status: 'FINISHED', episodes_watched: 1, episodes_released: 8 }),
      makeAnime({ name: 'Alpha airing', media_status: 'RELEASING', episodes_watched: 0, episodes_released: 5 }),
    ]
    expect(filterAnimes(animes, 'beta', { unwatched: true, type: 'airing' }).map(a => a.name)).toEqual([
      'Beta airing',
    ])
  })

  it('counts a type bucket under the current search and unwatched', () => {
    const animes = [
      makeAnime({ name: 'Beta done', media_status: 'FINISHED', episodes_watched: 12, episodes_released: 12 }),
      makeAnime({ name: 'Beta leftover', media_status: 'FINISHED', episodes_watched: 1, episodes_released: 12 }),
      makeAnime({ name: 'Alpha leftover', media_status: 'FINISHED', episodes_watched: 0, episodes_released: 5 }),
    ]
    const opts = { unwatched: true, type: 'finished' as const }
    expect(filterAnimes(animes, 'beta', opts)).toHaveLength(1)
    expect(filterAnimes(animes, 'beta', opts)[0].name).toBe('Beta leftover')
  })
})

describe('isAiringStatus / isFinishedStatus', () => {
  it('treats RELEASING and HIATUS as airing', () => {
    expect(isAiringStatus('RELEASING')).toBe(true)
    expect(isAiringStatus('HIATUS')).toBe(true)
    expect(isAiringStatus('FINISHED')).toBe(false)
    expect(isAiringStatus(undefined)).toBe(false)
  })

  it('treats FINISHED and CANCELLED as finished', () => {
    expect(isFinishedStatus('FINISHED')).toBe(true)
    expect(isFinishedStatus('CANCELLED')).toBe(true)
    expect(isFinishedStatus('RELEASING')).toBe(false)
    expect(isFinishedStatus(undefined)).toBe(false)
  })
})

describe('encodeStatusView / decodeStatusView', () => {
  it('encodes a clean default view as an empty querystring', () => {
    expect(encodeStatusView(DEFAULT_STATUS_VIEW)).toBe('')
  })

  it('round-trips type, unwatched and search', () => {
    const view = { q: 'beta', unwatched: true, type: 'airing' as const }
    expect(encodeStatusView(view)).toBe('q=beta&unwatched=1&type=airing')
    expect(decodeStatusView(encodeStatusView(view))).toEqual(view)
  })

  it('treats an unknown type as all and ignores leftover params', () => {
    expect(decodeStatusView('type=foo&extra=1')).toEqual(DEFAULT_STATUS_VIEW)
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


describe('breakdown', () => {
  // The bug: episodes_downloaded counts files on disk NOW and drops when the daemon prunes
  // watched ones (watched_episodes_to_keep), so released - episodes_downloaded painted yellow
  // ("still to download") an episode that had already been downloaded and watched.
  it('watched and pruned from disk does not become a yellow segment', () => {
    const a = makeAnime({
      episodes_watched: 5,
      episodes_downloaded: 5, // 5 on disk, but they are eps 6-10; 1-5 were pruned
      episodes_released: 10,
      episodes_pending: 0,
      total_episodes: 13,
    })
    expect(breakdown(a)).toEqual({ watched: 5, downloaded: 10, released: 10, total: 13 })
  })

  it('an episode actually missing stays in the yellow segment', () => {
    const a = makeAnime({
      episodes_watched: 0,
      episodes_downloaded: 5,
      episodes_released: 8,
      episodes_pending: 3,
      total_episodes: 12,
    })
    expect(breakdown(a)).toEqual({ watched: 0, downloaded: 5, released: 8, total: 12 })
  })

  it('with no announced total the denominator becomes episodes_released', () => {
    const a = makeAnime({
      episodes_watched: 2,
      episodes_released: 4,
      episodes_pending: 1,
      total_episodes: 0,
    })
    expect(breakdown(a)).toEqual({ watched: 2, downloaded: 3, released: 4, total: 4 })
  })

  it('AniList progress above released does not produce a negative segment', () => {
    const a = makeAnime({
      episodes_watched: 9,
      episodes_downloaded: 0,
      episodes_released: 7,
      episodes_pending: 0,
      total_episodes: 12,
    })
    const b = breakdown(a)
    expect(b).toEqual({ watched: 9, downloaded: 9, released: 9, total: 12 })
    expect(b.downloaded - b.watched).toBeGreaterThanOrEqual(0)
    expect(b.released - b.downloaded).toBeGreaterThanOrEqual(0)
  })
})
