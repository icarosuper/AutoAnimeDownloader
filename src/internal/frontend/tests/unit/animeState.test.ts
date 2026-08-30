import { describe, it, expect } from 'vitest'
import { deriveAnimeChip } from '../../src/lib/domain/animeState'
import type { AnimeInfo, TorrentInfo } from '../../src/lib/api/client'

function makeAnime(overrides: Partial<AnimeInfo> = {}): AnimeInfo {
  return {
    anime_id: 1,
    name: 'Test Anime',
    episodes_downloaded: 0,
    episodes_released: 0,
    episodes_watched: 0,
    episodes_pending: 0,
    total_episodes: 12,
    latest_episode_number: 1,
    last_download_date: '',
    is_blacklisted: false,
    ...overrides,
  }
}

function makeTorrent(overrides: Partial<TorrentInfo> = {}): TorrentInfo {
  return {
    hash: 'h1',
    name: 't',
    status: 'downloading',
    queue_position: 0,
    completed: false,
    anime_id: 1,
    episode_number: 5,
    is_batch: false,
    bytes_completed: 0,
    bytes_total: 0,
    bytes_uploaded: 0,
    progress: 0.5,
    download_speed: 0,
    upload_speed: 0,
    peers_total: 3,
    eta_seconds: null,
    seeded_for_seconds: 0,
    ...overrides,
  }
}

const NOW = new Date('2026-01-01T00:00:00Z').getTime()

describe('deriveAnimeChip', () => {
  it('is_blacklisted wins over every other branch (even an active download)', () => {
    const anime = makeAnime({ is_blacklisted: true, episodes_released: 10, episodes_downloaded: 3 })
    const torrents = [makeTorrent({ status: 'downloading' })]
    const chip = deriveAnimeChip(anime, torrents, NOW)
    expect(chip).toEqual({ key: 'blacklisted', variant: 'neutral', dimmed: true })
  })

  it('a downloading torrent for this anime produces the downloading chip with episode + percent', () => {
    const anime = makeAnime({ episodes_released: 10, episodes_downloaded: 4 })
    const torrents = [makeTorrent({ status: 'downloading', episode_number: 5, progress: 0.42 })]
    const chip = deriveAnimeChip(anime, torrents, NOW)
    expect(chip).toEqual({ key: 'downloading', variant: 'accent', episodeNumber: 5, percent: 42 })
  })

  it('ignores torrents belonging to a different anime_id', () => {
    const anime = makeAnime({ anime_id: 1, episodes_released: 5, episodes_downloaded: 5 })
    const torrents = [makeTorrent({ anime_id: 999, status: 'downloading' })]
    const chip = deriveAnimeChip(anime, torrents, NOW)
    expect(chip.key).toBe('upToDate')
  })

  it('peers_total=0 for at least 10 minutes produces the noSeeds chip', () => {
    const anime = makeAnime({ episodes_released: 10, episodes_downloaded: 4 })
    const torrents = [makeTorrent({ status: 'downloading_metadata', peers_total: 0, hash: 'stalled' })]
    const stalledSince = new Map([['stalled', NOW - 11 * 60 * 1000]])
    const chip = deriveAnimeChip(anime, torrents, NOW, stalledSince)
    expect(chip).toEqual({ key: 'noSeeds', variant: 'danger' })
  })

  it('peers_total=0 for under 10 minutes does NOT produce the noSeeds chip', () => {
    const anime = makeAnime({ episodes_released: 10, episodes_downloaded: 10 })
    const torrents = [makeTorrent({ status: 'downloading_metadata', peers_total: 0, hash: 'stalled' })]
    const stalledSince = new Map([['stalled', NOW - 5 * 60 * 1000]])
    const chip = deriveAnimeChip(anime, torrents, NOW, stalledSince)
    expect(chip.key).not.toBe('noSeeds')
  })

  it('a completed torrent with no peers is an idle seed, not a stall', () => {
    const anime = makeAnime({ episodes_released: 10, episodes_downloaded: 10 })
    const torrents = [
      makeTorrent({ status: 'seeding', completed: true, peers_total: 0, hash: 'seed' }),
    ]
    const stalledSince = new Map([['seed', NOW - 60 * 60 * 1000]])
    const chip = deriveAnimeChip(anime, torrents, NOW, stalledSince)
    expect(chip.key).toBe('upToDate')
  })

  it('a downloading torrent takes priority over a stalled one for the same anime', () => {
    const anime = makeAnime({ episodes_released: 10, episodes_downloaded: 4 })
    const torrents = [
      makeTorrent({ hash: 'a', status: 'downloading', episode_number: 6, progress: 0.1 }),
      makeTorrent({ hash: 'b', status: 'verifying', peers_total: 0 }),
    ]
    const stalledSince = new Map([['b', NOW - 20 * 60 * 1000]])
    const chip = deriveAnimeChip(anime, torrents, NOW, stalledSince)
    expect(chip.key).toBe('downloading')
  })

  it('a stopped torrent (no active download, no stall) produces the paused chip', () => {
    const anime = makeAnime({ episodes_released: 10, episodes_downloaded: 4 })
    const torrents = [makeTorrent({ status: 'stopped', peers_total: 2 })]
    const chip = deriveAnimeChip(anime, torrents, NOW)
    expect(chip).toEqual({ key: 'paused', variant: 'neutral' })
  })

  it('episodes_released === 0 produces the awaitingPremiere chip', () => {
    const anime = makeAnime({ episodes_released: 0, episodes_downloaded: 0 })
    const chip = deriveAnimeChip(anime, [], NOW)
    expect(chip).toEqual({ key: 'awaitingPremiere', variant: 'neutral' })
  })

  it('episodes_pending === 0 produces the upToDate chip', () => {
    const anime = makeAnime({ episodes_released: 8, episodes_downloaded: 8, episodes_pending: 0 })
    const chip = deriveAnimeChip(anime, [], NOW)
    expect(chip).toEqual({ key: 'upToDate', variant: 'ok' })
  })

  it('episodes_pending > 0 produces the behind chip with the pending count', () => {
    const anime = makeAnime({ episodes_released: 8, episodes_downloaded: 5, episodes_pending: 3 })
    const chip = deriveAnimeChip(anime, [], NOW)
    expect(chip).toEqual({ key: 'behind', variant: 'warn', behindCount: 3 })
  })

  it('nada baixado mas tudo assistido nao e atraso (episodes_pending === 0)', () => {
    const anime = makeAnime({ episodes_released: 8, episodes_downloaded: 0, episodes_watched: 8, episodes_pending: 0 })
    const chip = deriveAnimeChip(anime, [], NOW)
    expect(chip).toEqual({ key: 'upToDate', variant: 'ok' })
  })
})
