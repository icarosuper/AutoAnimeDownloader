import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import { tick } from 'svelte'
import AnimeDetail from '../../src/routes/AnimeDetail.svelte'
import * as client from '../../src/lib/api/client.js'
import type { AnimeDetailResponse, AnimeInfo } from '../../src/lib/api/client.js'

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
  getLastCheck: vi.fn(),
}))

function animeInfo(overrides: Partial<AnimeInfo> = {}): AnimeInfo {
  return {
    anime_id: 42,
    name: 'Test Anime',
    episodes_downloaded: 0,
    episodes_released: 0,
    episodes_watched: 0,
    episodes_pending: 0,
    total_episodes: 0,
    latest_episode_number: 0,
    last_download_date: '',
    ...overrides,
  }
}

describe('AnimeDetail — last-check reason', () => {
  it('shows nothing when the anime has no problem in the report', async () => {
    const detail: AnimeDetailResponse = {
      anime_id: 42, total_episodes: 3, progress: 0, status: 'CURRENT',
      episodes: [{ episode_number: 1, airing_at: 0, time_until_airing: 0, is_aired: true, is_watched: false, is_downloaded: false }],
    }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])
    vi.mocked(client.getLastCheck).mockResolvedValue({
      finished_at: '2026-08-19T12:00:00Z', pass_error: '', limits: [],
      // Problem of ANOTHER anime: must not leak onto this screen.
      problems: [{ anime_id: 999, anime_name: 'Other', episodes: [1], code: 'no_torrent_found' }],
    })

    render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    expect(screen.queryByTestId('anime-last-check')).toBeNull()
  })

  it('shows the notice and marks only the affected episode', async () => {
    const detail: AnimeDetailResponse = {
      anime_id: 42, total_episodes: 3, progress: 0, status: 'CURRENT',
      episodes: [
        { episode_number: 1, airing_at: 0, time_until_airing: 0, is_aired: true, is_watched: false, is_downloaded: false },
        { episode_number: 2, airing_at: 0, time_until_airing: 0, is_aired: true, is_watched: false, is_downloaded: false },
      ],
    }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])
    vi.mocked(client.getLastCheck).mockResolvedValue({
      finished_at: '2026-08-19T12:00:00Z', pass_error: '', limits: [],
      problems: [{ anime_id: 42, anime_name: 'Test Anime', episodes: [2], code: 'all_above_size_limit', candidates: 8, limit_gb: 3 }],
    })

    const { container } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    const notice = await screen.findByTestId('anime-last-check')
    expect(notice.textContent).toContain('8')
    expect(notice.textContent).toContain('3')

    // The mark lives on episode 2's row and NOT on episode 1's. Desktop and mobile render the
    // same definition, so each row appears twice in the DOM.
    const marks = await screen.findAllByText(/not downloaded in the last check|não baixou na última verificação/i)
    expect(marks.length).toBeGreaterThan(0)
    // Two episodes × (desktop + mobile) = 4 rows, and only episode 2's are marked.
    const rows = Array.from(container.querySelectorAll('[data-episode-row]'))
    expect(rows.length).toBe(4)
    const marked = rows.filter((r) => /not downloaded in the last check|não baixou na última verificação/i.test(r.textContent ?? ''))
    expect(marked.length).toBe(2)
    // The mark is on episode 2's row, never on episode 1's: the number comes from the row
    // TITLE, not from a loose "2" anywhere in its text. (It used to be the "#" column's
    // <span>, which left the screen — it repeated in another typeface the number the title
    // already says.)
    for (const row of marked) {
      expect(row.querySelector('p.text-heading')?.textContent?.trim()).toMatch(/^(Episode|Episódio) 2$/)
    }
  })
})
