import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render } from '@testing-library/svelte'
import { tick } from 'svelte'
import AnimeDetail from '../../src/routes/AnimeDetail.svelte'
import * as client from '../../src/lib/api/client.js'
import type { AnimeDetailResponse, TorrentInfo } from '../../src/lib/api/client.js'

// AnimeDetail.svelte only reads these named exports; stub every one it imports so the
// module mock doesn't leave any binding undefined.
vi.mock('../../src/lib/api/client.js', () => ({
  getAnimeDetail: vi.fn(),
  getAnimes: vi.fn(),
  getTorrents: vi.fn(),
  downloadEpisode: vi.fn(),
  deleteEpisode: vi.fn(),
  releaseEpisode: vi.fn(),
  redownloadEpisode: vi.fn(),
  replaceEpisodeWithMagnet: vi.fn(),
  replaceAnimeWithMagnet: vi.fn(),
  updateAnimeSettings: vi.fn(),
  removeStandaloneAnime: vi.fn(),
  deleteTorrent: vi.fn(),
}))

function detailWith(animeId: number, episodeHash: string | undefined): AnimeDetailResponse {
  return {
    anime_id: animeId,
    total_episodes: 1,
    progress: 0,
    status: 'CURRENT',
    episodes: [
      {
        episode_number: 1,
        airing_at: 0,
        time_until_airing: 0,
        is_aired: true,
        is_watched: false,
        // O daemon grava o registro salvo (origem de is_downloaded E de episode_hash) já ao
        // adicionar o torrent, então ter hash implica is_downloaded: true mesmo em voo.
        is_downloaded: episodeHash !== undefined,
        episode_hash: episodeHash,
      },
    ],
  }
}

function activeTorrent(hash: string): TorrentInfo {
  return {
    hash,
    name: 'Anime - 01',
    status: 'downloading',
    queue_position: 0,
    completed: false,
    episode_number: 1,
    is_batch: false,
    bytes_completed: 10,
    bytes_total: 100,
    bytes_uploaded: 0,
    progress: 0.1,
    download_speed: 1000,
    upload_speed: 0,
    peers_total: 3,
    eta_seconds: 90,
    seeded_for_seconds: 0,
  }
}

// vi.waitFor()'s polling relies on setTimeout internally, which is frozen once fake timers
// are enabled — so under vi.useFakeTimers() it never sees the resolved mocks. Flush the
// microtask queue directly instead: loadData()'s Promise.all -> reactive `detail` assignment
// -> the poll-start gate -> pollTorrents()'s own await is a handful of microtask hops, well
// within this many rounds of Promise.resolve() + tick().
async function flushMicrotasks(rounds = 10) {
  for (let i = 0; i < rounds; i++) {
    await Promise.resolve();
  }
  await tick();
}

describe('AnimeDetail — torrent poll cadence across anime switches', () => {
  // `vi.spyOn(global, 'setTimeout')`'s precise overload resolution doesn't collapse cleanly
  // into a single reusable type across the two assignment sites below (declaration vs.
  // beforeEach), so this is deliberately untyped rather than fighting the overload picker.
  let setTimeoutSpy: any

  beforeEach(() => {
    vi.useFakeTimers()
    setTimeoutSpy = vi.spyOn(global, 'setTimeout')
    vi.mocked(client.getAnimes).mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('re-evaluates cadence immediately on an animeId change instead of waiting out the previous anime\'s interval', async () => {
    // Anime A (id=1): no matching torrent -> settles on the slow 15s cadence.
    vi.mocked(client.getAnimeDetail).mockResolvedValueOnce(detailWith(1, undefined))
    vi.mocked(client.getTorrents).mockResolvedValueOnce([])

    const { rerender } = render(AnimeDetail, { props: { params: { id: '1' } } })

    await flushMicrotasks()
    expect(client.getTorrents).toHaveBeenCalledTimes(1)

    const delayAfterA = setTimeoutSpy.mock.calls.at(-1)?.[1]
    expect(delayAfterA).toBe(15000)

    // Switch to anime B (id=2), which has an active, non-completed torrent.
    vi.mocked(client.getAnimeDetail).mockResolvedValueOnce(detailWith(2, 'HASH-B'))
    vi.mocked(client.getTorrents).mockResolvedValueOnce([activeTorrent('HASH-B')])

    await rerender({ params: { id: '2' } })

    // Without advancing fake time at all, the animeId change must have already triggered a
    // fresh /torrents fetch — proving the switch re-polls immediately rather than waiting for
    // anime A's still-pending 15s timer to elapse.
    await flushMicrotasks()
    expect(client.getTorrents).toHaveBeenCalledTimes(2)

    const delayAfterB = setTimeoutSpy.mock.calls.at(-1)?.[1]
    expect(delayAfterB).toBe(2000)

    // The new short cadence must actually be the one governing subsequent polls: advancing
    // fake time by exactly one 2s tick (well short of anime A's stale 15s) should produce a
    // third fetch.
    vi.mocked(client.getTorrents).mockResolvedValueOnce([activeTorrent('HASH-B')])
    await vi.advanceTimersByTimeAsync(2000)
    expect(client.getTorrents).toHaveBeenCalledTimes(3)
  })
})
