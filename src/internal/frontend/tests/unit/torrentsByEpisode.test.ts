import { describe, it, expect } from 'vitest'
import { indexTorrentsByEpisode } from '../../src/lib/utils/torrentsByEpisode'
import type { TorrentInfo, AnimeEpisodeInfo } from '../../src/lib/api/client'

function makeTorrent(overrides: Partial<TorrentInfo> = {}): TorrentInfo {
  return {
    hash: 'h1',
    name: 'Some Torrent',
    status: 'downloading',
    queue_position: 0,
    completed: false,
    episode_number: null,
    is_batch: false,
    bytes_completed: 0,
    bytes_total: 0,
    bytes_uploaded: 0,
    progress: 0,
    download_speed: 0,
    upload_speed: 0,
    peers_total: 0,
    eta_seconds: null,
    seeded_for_seconds: 0,
    ...overrides,
  }
}

function makeEpisode(overrides: Partial<AnimeEpisodeInfo> = {}): AnimeEpisodeInfo {
  return {
    episode_number: 1,
    airing_at: 0,
    time_until_airing: 0,
    is_aired: true,
    is_watched: false,
    is_downloaded: false,
    ...overrides,
  }
}

describe('indexTorrentsByEpisode', () => {
  it('matches a single episode to its torrent', () => {
    const torrent = makeTorrent({ hash: 'abc' })
    const episode = makeEpisode({ episode_number: 3, episode_hash: 'abc' })

    const result = indexTorrentsByEpisode([torrent], [episode])

    expect(result.get(3)).toBe(torrent)
    expect(result.size).toBe(1)
  })

  it('maps a batch torrent to every episode it covers', () => {
    const torrent = makeTorrent({ hash: 'batch-hash', is_batch: true })
    const episodes = [
      makeEpisode({ episode_number: 1, episode_hash: 'batch-hash' }),
      makeEpisode({ episode_number: 2, episode_hash: 'batch-hash' }),
      makeEpisode({ episode_number: 3, episode_hash: 'batch-hash' }),
    ]

    const result = indexTorrentsByEpisode([torrent], episodes)

    expect(result.size).toBe(3)
    expect(result.get(1)).toBe(torrent)
    expect(result.get(2)).toBe(torrent)
    expect(result.get(3)).toBe(torrent)
  })

  it('does not include a torrent with no matching episode', () => {
    const torrent = makeTorrent({ hash: 'orphan' })
    const episode = makeEpisode({ episode_number: 1, episode_hash: 'other-hash' })

    const result = indexTorrentsByEpisode([torrent], [episode])

    expect(result.size).toBe(0)
  })

  it('does not leak a torrent belonging to another anime', () => {
    const otherAnimeTorrent = makeTorrent({ hash: 'other-anime-hash' })
    const episodes = [
      makeEpisode({ episode_number: 1, episode_hash: 'this-anime-hash-1' }),
      makeEpisode({ episode_number: 2, episode_hash: 'this-anime-hash-2' }),
    ]

    const result = indexTorrentsByEpisode([otherAnimeTorrent], episodes)

    expect(result.size).toBe(0)
  })

  it('excludes episodes without an episode_hash', () => {
    const torrent = makeTorrent({ hash: 'abc' })
    const episode = makeEpisode({ episode_number: 1, episode_hash: undefined })

    const result = indexTorrentsByEpisode([torrent], [episode])

    expect(result.size).toBe(0)
  })
})
