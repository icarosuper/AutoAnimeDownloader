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

// The last-check report is fetched on the same tick as the torrent poll; an empty report
// is the default of every test that is not about it.
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

    // Desktop and mobile render the same definition, so each row appears twice in the DOM.
    const rows = container.querySelectorAll('[data-episode-row]')
    expect(rows.length).toBe(2 * 2) // 1 pack row + 1 standalone episode, in both views
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

    // Scoped to episode rows, not the whole document: the standalone-episode "Redownload"
    // ConfirmDialog stays always mounted (closed, but present in the DOM for accessibility —
    // see the comment in ConfirmDialog.svelte), so its static "Redownload"/"Rebaixar" text
    // would show even on a screen with no redownload action offered.
    const { container } = render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    const rows = Array.from(container.querySelectorAll('[data-episode-row]'))
    expect(rows.length).toBeGreaterThan(0)
    for (const row of rows) {
      expect(within(row as HTMLElement).queryByText(/rebaixar|redownload/i)).toBeNull()
    }
  })

  it('the pack row does not offer "Replace" as its principal action', async () => {
    // Finding 1 (review, round 1): "Replace" on a pack row called replaceAnimeWithMagnet,
    // which deletes ALL of the anime's torrents (endpoint_episode_actions.go) — with successive
    // packs (001-100 + 101-200), clicking "Replace" on the first would silently destroy the
    // second. The pack row must not render any principal button (only "Delete", in the ⋯ menu).
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
      // The two buttons filtered out are not actions: `aria-haspopup` is the "..." trigger and
      // `aria-expanded` is the file-list toggle (read-only panel, see TorrentFiles.svelte).
      const buttons = within(row as HTMLElement).queryAllByRole('button').filter(
        (b) => !b.getAttribute('aria-haspopup') && !b.hasAttribute('aria-expanded'),
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

    // ConfirmDialog runs on `ui/Modal`, which only mounts content when it is open
    // (`{#if open}`), so there is at most ONE [role=dialog] in the tree. The assertion stays
    // scoped to it — not the document — so it doesn't match "stop tracking" (untrackOpen)
    // if that is ever also open.
    const openDialog = container.querySelector('[role="dialog"]') as HTMLElement
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
  // The bug: with 5 episodes already watched, the daemon only records 6..11 of pack 01-11,
  // and the row was labelled by the records' min/max — "Episodes 6–11" for a pack that has 1 to 11.
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

  // Pack with no known range (name without "01-11"): the backend sends the fields as zero/absent
  // and the records' min/max remains the best guess.
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
  // The row always uses the localized title. It used to show the saved record's `episode_name`
  // — which is the download log label ("Anime - Episode 7"), never an episode title — and the
  // result was "Episode 7" turning into an internal label as soon as the download started. The
  // field left the API; what this test locks is the row label.
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

    // Test locale is en: "Episode 2", and the assertion is EXACT text.
    expect(getAllByText('Episode 2').length).toBeGreaterThan(0)
  })
})

describe('AnimeDetail — a pack row swallows the episodes inside its range', () => {
  // Watched episodes have no saved record (the daemon never downloads them), but they ARE
  // INSIDE the pack: the torrent brings the files and Organize hardlinks the whole directory
  // into the library. Showing them as loose rows with "Download" offered a second torrent of
  // what is already coming.
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

    // Desktop and mobile render the same definition: 1 row per view, and only that.
    expect(container.querySelectorAll('[data-episode-row]').length).toBe(1 * 2)
    expect(getAllByText(/1\s*[–-]\s*11/).length).toBeGreaterThan(0)
  })

  // An episode not yet released inside the range is NOT swallowed: the pack on disk cannot
  // contain what hasn't aired, and the "Upcoming" row is what says when it arrives.
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

    // 1 pack row + the 2 unreleased, in both views.
    expect(container.querySelectorAll('[data-episode-row]').length).toBe(3 * 2)
  })

  // An episode inside the range with its OWN torrent (downloaded standalone before the pack)
  // keeps its row: it is another torrent, with its own progress and actions.
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
