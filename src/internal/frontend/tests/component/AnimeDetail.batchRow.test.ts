import { describe, it, expect, vi, beforeEach } from 'vitest'
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
  getLastCheck: vi.fn(),
}))

// O relatório da última verificação é buscado no mesmo tick do poll de torrents; um relatório
// vazio é o default de todos os testes que não são sobre ele.
beforeEach(() => {
  vi.mocked(client.getLastCheck).mockResolvedValue({
    finished_at: '0001-01-01T00:00:00Z', pass_error: '', problems: [], limits: [],
  })
})


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

describe('AnimeDetail — the pack row shows the pack\'s real range', () => {
  // O bug: com 5 episódios já assistidos, o daemon só registra 6..11 do pack 01-11, e a linha
  // era rotulada pelo min/max dos registros — "Episódios 6–11" para um pack que tem 1 a 11.
  it('labels the row with batch_start/batch_end, not the saved min/max', async () => {
    const episodes = [6, 7, 8, 9, 10, 11].map(n => ({
      episode_number: n, airing_at: 0, time_until_airing: 0,
      is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'pack',
      batch_start: 1, batch_end: 11,
    }))

    const detail: AnimeDetailResponse = { anime_id: 42, total_episodes: 11, progress: 5, status: 'REPEATING', episodes }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])

    const { getAllByText, queryAllByText } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    expect(getAllByText(/1\s*[–-]\s*11/).length).toBeGreaterThan(0)
    expect(queryAllByText(/6\s*[–-]\s*11/).length).toBe(0)
  })

  // Pack sem faixa conhecida (nome sem "01-11"): o backend manda os campos zerados/ausentes e o
  // min/max dos registros continua sendo o melhor palpite.
  it('falls back to the saved min/max when the range is unknown', async () => {
    const episodes = [3, 4, 5].map(n => ({
      episode_number: n, airing_at: 0, time_until_airing: 0,
      is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'pack',
    }))

    const detail: AnimeDetailResponse = { anime_id: 42, total_episodes: 5, progress: 2, status: 'CURRENT', episodes }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])

    const { getAllByText } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    expect(getAllByText(/3\s*[–-]\s*5/).length).toBeGreaterThan(0)
  })
})

describe('AnimeDetail — episode rows are labelled for the user, not for the daemon', () => {
  // A linha usa sempre o título localizado. Ela já mostrou o `episode_name` do registro salvo —
  // que é o rótulo de log do download ("Anime - Episode 7"), nunca um título de episódio —, e o
  // resultado era "Episódio 7" virar rótulo interno assim que o download começava. O campo saiu
  // da API; o que este teste tranca é o rótulo da linha.
  it('shows the localized episode title even for an episode already downloading', async () => {
    const episodes = [
      { episode_number: 1, airing_at: 0, time_until_airing: 0, is_aired: true, is_watched: false, is_downloaded: false },
      {
        episode_number: 2, airing_at: 0, time_until_airing: 0, is_aired: true, is_watched: false,
        is_downloaded: true, episode_hash: 'solo',
      },
    ]

    const detail: AnimeDetailResponse = { anime_id: 42, total_episodes: 2, progress: 0, status: 'CURRENT', episodes }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])

    const { getAllByText } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    // Locale dos testes é en: "Episode 2", e a asserção é de texto EXATO.
    expect(getAllByText('Episode 2').length).toBeGreaterThan(0)
  })
})

describe('AnimeDetail — a pack row swallows the episodes inside its range', () => {
  // Os episódios assistidos não têm registro salvo (o daemon nunca os baixa), mas estão DENTRO do
  // pack: o torrent traz os arquivos e o Organize hardlinka o diretório inteiro na biblioteca.
  // Mostrá-los como linhas soltas com "Baixar" oferecia um segundo torrent do que já está vindo.
  it('folds watched episodes inside the range into the pack row', async () => {
    const watched = [1, 2, 3, 4, 5].map(n => ({
      episode_number: n, airing_at: 0, time_until_airing: 0,
      is_aired: true, is_watched: true, is_downloaded: false,
    }))
    const packed = [6, 7, 8, 9, 10, 11].map(n => ({
      episode_number: n, airing_at: 0, time_until_airing: 0,
      is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'pack',
      batch_start: 1, batch_end: 11,
    }))

    const detail: AnimeDetailResponse = {
      anime_id: 42, total_episodes: 11, progress: 5, status: 'REPEATING', episodes: [...watched, ...packed],
    }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])

    const { container, getAllByText } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    // Desktop e mobile renderizam a mesma definição: 1 linha por vista, e só ela.
    expect(container.querySelectorAll('[data-episode-row]').length).toBe(1 * 2)
    expect(getAllByText(/1\s*[–-]\s*11/).length).toBeGreaterThan(0)
  })

  // Episódio ainda não lançado dentro da faixa NÃO é engolido: o pack que está no disco não pode
  // conter o que não foi ao ar, e a linha "Em breve" é o que diz quando ele chega.
  it('keeps an unaired episode inside the range as its own row', async () => {
    const packed = [1, 2].map(n => ({
      episode_number: n, airing_at: 0, time_until_airing: 0,
      is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'pack',
      batch_start: 1, batch_end: 4,
    }))
    const upcoming = [3, 4].map(n => ({
      episode_number: n, airing_at: 0, time_until_airing: 3600,
      is_aired: false, is_watched: false, is_downloaded: false,
    }))

    const detail: AnimeDetailResponse = {
      anime_id: 42, total_episodes: 4, progress: 0, status: 'CURRENT', episodes: [...packed, ...upcoming],
    }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])

    const { container } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    // 1 linha de pack + os 2 não lançados, nas duas vistas.
    expect(container.querySelectorAll('[data-episode-row]').length).toBe(3 * 2)
  })

  // Episódio dentro da faixa com torrent PRÓPRIO (baixado avulso antes do pack) mantém a linha
  // dele: ele é outro torrent, com progresso e ações próprias.
  it('keeps an episode that has its own torrent as its own row', async () => {
    const packed = [1, 2, 3].map(n => ({
      episode_number: n, airing_at: 0, time_until_airing: 0,
      is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'pack',
      batch_start: 1, batch_end: 4,
    }))
    const solo = {
      episode_number: 4, airing_at: 0, time_until_airing: 0,
      is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'solo',
    }

    const detail: AnimeDetailResponse = {
      anime_id: 42, total_episodes: 4, progress: 0, status: 'CURRENT', episodes: [...packed, solo],
    }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])

    const { container } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    expect(container.querySelectorAll('[data-episode-row]').length).toBe(2 * 2)
  })
})
