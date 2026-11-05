import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, within, fireEvent, waitFor } from '@testing-library/svelte'
import { tick } from 'svelte'
import AnimeDetail from '../../src/routes/AnimeDetail.svelte'
import * as client from '../../src/lib/api/client.js'
import type { AnimeDetailResponse } from '../../src/lib/api/client.js'

// AnimeDetail.svelte only reads these named exports; stub every one it imports so the
// module mock doesn't leave any binding undefined.
vi.mock('../../src/lib/api/client.js', () => ({
  getAnimeDetail: vi.fn(),
  getAnimes: vi.fn(),
  getTorrents: vi.fn(),
  downloadEpisode: vi.fn(),
  deleteEpisode: vi.fn(),
  batchDeleteEpisodes: vi.fn(),
  batchRedownloadEpisodes: vi.fn(),
  releaseEpisode: vi.fn(),
  redownloadEpisode: vi.fn(),
  replaceEpisodeWithMagnet: vi.fn(),
  replaceAnimeWithMagnet: vi.fn(),
  updateAnimeSettings: vi.fn(),
  removeStandaloneAnime: vi.fn(),
  deleteTorrent: vi.fn(),
  getLastCheck: vi.fn(),
}))

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(client.getLastCheck).mockResolvedValue({
    finished_at: '0001-01-01T00:00:00Z', pass_error: '', problems: [], limits: [],
  })
  const episodes = [1, 2, 3].map(n => ({
    episode_number: n, airing_at: 0, time_until_airing: 0,
    is_aired: true, is_watched: false, is_downloaded: n < 3, episode_hash: `h${n}`,
  }))
  const detail: AnimeDetailResponse = { anime_id: 42, total_episodes: 3, progress: 0, status: 'CURRENT', episodes }
  vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
  vi.mocked(client.getAnimes).mockResolvedValue([])
  vi.mocked(client.getTorrents).mockResolvedValue([])
})

async function selectAllAndConfirm(action: RegExp) {
  const { container, getAllByRole, getByRole } = render(AnimeDetail, { props: { params: { id: '42' } } })
  await tick(); await tick()
  await fireEvent.click(getAllByRole('checkbox', { name: /selecionar todos|select all/i })[0])
  await fireEvent.click(getByRole('button', { name: action }))
  await tick()
  const dialog = container.querySelector('[role="dialog"]') as HTMLElement
  const buttons = within(dialog).getAllByRole('button')
  await fireEvent.click(buttons[buttons.length - 1])
}

// The selection is ONE request for the downloaded episodes: parallel single deletes could each
// still see the other episodes of a shared pack on file and leave the pack on disk.
describe('AnimeDetail — bulk actions go through the batch endpoints', () => {
  it('deletes the downloaded episodes of the selection in one request', async () => {
    vi.mocked(client.batchDeleteEpisodes).mockResolvedValue()
    await selectAllAndConfirm(/^(deletar selecionados|delete selected)$/i)

    await waitFor(() => expect(client.batchDeleteEpisodes).toHaveBeenCalledWith(42, [1, 2]))
    expect(client.deleteEpisode).not.toHaveBeenCalled()
  })

  it('redownloads the downloaded episodes of the selection in one request', async () => {
    vi.mocked(client.batchRedownloadEpisodes).mockResolvedValue({ redownloaded: [1], failed: [{ episode: 2, error: 'x' }] })
    await selectAllAndConfirm(/^(rebaixar selecionados|redownload selected)$/i)

    await waitFor(() => expect(client.batchRedownloadEpisodes).toHaveBeenCalledWith(42, [1, 2]))
    expect(client.redownloadEpisode).not.toHaveBeenCalled()
  })
})
