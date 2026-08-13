import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/svelte'
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

function detailWith(animeId: number): AnimeDetailResponse {
  return {
    anime_id: animeId,
    total_episodes: 1,
    progress: 0,
    status: 'CURRENT',
    episodes: [],
  }
}

describe('AnimeDetail — watched count and standalone progress input', () => {
  it('standalone anime shows the watched count and a progress input', async () => {
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({ is_standalone: true, episodes_watched: 48 })])
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detailWith(42))
    vi.mocked(client.getTorrents).mockResolvedValue([])

    const { getByLabelText, container } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    const input = getByLabelText(/assistidos|watched/i) as HTMLInputElement
    expect(input.value).toBe('48')
    expect(container.textContent).toMatch(/48/)
  })

  it('anime from a list shows the watched count without an input', async () => {
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({ is_standalone: false, episodes_watched: 7 })])
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detailWith(42))
    vi.mocked(client.getTorrents).mockResolvedValue([])

    const { queryByLabelText, container } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    expect(queryByLabelText(/assistidos|watched/i)).toBeNull()
    expect(container.textContent).toMatch(/7/)
  })
})
