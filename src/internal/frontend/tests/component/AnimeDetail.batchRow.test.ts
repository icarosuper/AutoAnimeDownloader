import { describe, it, expect, vi } from 'vitest'
import { render, within, fireEvent } from '@testing-library/svelte'
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

describe('AnimeDetail — a pack is one row', () => {
  it('renders 100 episodes sharing one hash as a single row', async () => {
    const episodes = Array.from({ length: 100 }, (_, i) => ({
      episode_number: i + 1, airing_at: 0, time_until_airing: 0,
      is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'pack',
    }))
    episodes.push({
      episode_number: 101, airing_at: 0, time_until_airing: 0,
      is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'solo',
    })

    const detail: AnimeDetailResponse = { anime_id: 42, total_episodes: 101, progress: 0, status: 'CURRENT', episodes }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])

    const { container, getAllByText } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    // Desktop e mobile renderizam a mesma definição, então cada linha aparece duas vezes no DOM.
    const rows = container.querySelectorAll('[data-episode-row]')
    expect(rows.length).toBe(2 * 2) // 1 linha de pack + 1 episódio solto, nas duas vistas
    expect(getAllByText(/1\s*[–-]\s*100/).length).toBeGreaterThan(0)
  })

  it('the pack row does not offer redownload', async () => {
    const episodes = [1, 2].map(n => ({
      episode_number: n, airing_at: 0, time_until_airing: 0,
      is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'pack',
    }))
    const detail: AnimeDetailResponse = { anime_id: 42, total_episodes: 2, progress: 0, status: 'CURRENT', episodes }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])

    // Escopado às linhas de episódio, não ao documento inteiro: o ConfirmDialog de "Rebaixar"
    // de episódio avulso fica sempre montado (fechado, mas presente no DOM por acessibilidade —
    // ver o comentário em ConfirmDialog.svelte), então seu texto estático "Redownload"/"Rebaixar"
    // apareceria mesmo numa tela sem nenhuma ação de rebaixar oferecida.
    const { container } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    const rows = Array.from(container.querySelectorAll('[data-episode-row]'))
    expect(rows.length).toBeGreaterThan(0)
    for (const row of rows) {
      expect(within(row as HTMLElement).queryByText(/rebaixar|redownload/i)).toBeNull()
    }
  })

  it('the pack row does not offer "Substituir" as its principal action', async () => {
    // Finding 1 (review, round 1): "Substituir" na linha de pack chamava replaceAnimeWithMagnet,
    // que apaga TODOS os torrents do anime (endpoint_episode_actions.go) — com packs sucessivos
    // (001-100 + 101-200), clicar "Substituir" no primeiro destruiria o segundo em silêncio. A
    // linha de pack não deve renderizar nenhum botão principal (só "Excluir", no menu ⋯).
    const episodes = [1, 2].map(n => ({
      episode_number: n, airing_at: 0, time_until_airing: 0,
      is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'pack',
    }))
    const detail: AnimeDetailResponse = { anime_id: 42, total_episodes: 2, progress: 0, status: 'CURRENT', episodes }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])

    const { container } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    const rows = Array.from(container.querySelectorAll('[data-episode-row]'))
    expect(rows.length).toBeGreaterThan(0)
    for (const row of rows) {
      // No principal Button at all in a pack row — "Delete" only exists inside the "..." menu.
      const buttons = within(row as HTMLElement).queryAllByRole('button').filter(
        (b) => !b.getAttribute('aria-haspopup'),
      )
      expect(buttons).toHaveLength(0)
      expect(within(row as HTMLElement).queryByText(/substituir|replace/i)).toBeNull()
    }
  })

  it('deleting a pack goes through DELETE /torrents/{hash}, never replaceAnimeWithMagnet, and the dialog does not promise untracking', async () => {
    // Finding 2 (review, round 1): TorrentDeleteDialog with scope="anime" + standalone promised
    // "stop tracking this anime" without ever calling removeStandaloneAnime, and exaggerated the
    // blast radius (RemoveTorrentWithEpisodes only touches the episodes sharing THIS hash).
    // scope="episode" is the correct, honest scope for a per-hash delete.
    const episodes = [1, 2].map(n => ({
      episode_number: n, airing_at: 0, time_until_airing: 0,
      is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'pack-hash',
    }))
    const detail: AnimeDetailResponse = { anime_id: 42, total_episodes: 2, progress: 0, status: 'CURRENT', episodes }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({ is_standalone: true })])
    vi.mocked(client.getTorrents).mockResolvedValue([])
    vi.mocked(client.deleteTorrent).mockResolvedValue(undefined)

    const { getAllByRole, getByRole, container } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    const menuTrigger = getAllByRole('button', { name: /mais ações do pack|more actions for the pack/i })[0]
    await fireEvent.click(menuTrigger)

    const deleteItem = getByRole('menuitem', { name: /excluir|delete/i })
    await fireEvent.click(deleteItem)
    await tick()

    // Every other ConfirmDialog/TorrentDeleteDialog on the page stays mounted-but-closed (for
    // a11y — see ConfirmDialog.svelte), so scope the assertion to the one dialog that actually
    // opened (native `open` attribute is only present on it, per ConfirmDialog's `{open}` spread)
    // instead of the whole document — otherwise the always-mounted "stop tracking" ConfirmDialog
    // (untrackOpen, unrelated to this flow) would produce a false failure either way.
    const openDialog = container.querySelector('dialog[open]') as HTMLElement
    expect(openDialog).toBeTruthy()
    expect(within(openDialog).queryByText(/deixar de rastrear|stop tracking/i)).toBeNull()

    const confirmButton = within(openDialog).getAllByRole('button').find((b) => /^(delete|excluir)$/i.test(b.textContent?.trim() ?? ''))
    expect(confirmButton).toBeTruthy()
    await fireEvent.click(confirmButton as HTMLElement)

    expect(client.deleteTorrent).toHaveBeenCalledWith('pack-hash', expect.any(Object))
    expect(client.replaceAnimeWithMagnet).not.toHaveBeenCalled()
  })
})
