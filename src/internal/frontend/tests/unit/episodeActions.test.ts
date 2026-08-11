import { describe, it, expect } from 'vitest'
import { episodeActions } from '../../src/lib/domain/episodeActions'
import type { AnimeEpisodeInfo, TorrentInfo } from '../../src/lib/api/client'

function makeEpisode(overrides: Partial<AnimeEpisodeInfo> = {}): AnimeEpisodeInfo {
  return {
    episode_number: 1,
    airing_at: 0,
    time_until_airing: 0,
    is_aired: true,
    is_watched: false,
    is_downloaded: false,
    is_manually_managed: false,
    is_blocked: false,
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
    episode_number: 1,
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

describe('episodeActions', () => {
  // Row 1: "Torrent ativo (baixando)" -> sem principal, menu: replace + delete(destructive)
  it('an actively-downloading torrent has no principal action and offers replace/delete in the menu', () => {
    const ep = makeEpisode({ is_aired: true, is_downloaded: false })
    const torrent = makeTorrent({ status: 'downloading' })
    const result = episodeActions(ep, torrent)
    expect(result.principal).toBeUndefined()
    expect(result.menu.map((a) => a.id)).toEqual(['replace', 'delete'])
    expect(result.menu.find((a) => a.id === 'delete')?.destructive).toBe(true)
  })

  // Row 2: "No ar e não baixado" -> principal: Baixar (solid), menu: replace
  it('an aired, not-yet-downloaded episode has Baixar as the principal action (solid) and replace in the menu', () => {
    const ep = makeEpisode({ is_aired: true, is_downloaded: false })
    const result = episodeActions(ep, undefined)
    expect(result.principal).toEqual({ id: 'download', labelKey: 'download', variant: 'solid' })
    expect(result.menu.map((a) => a.id)).toEqual(['replace'])
  })

  // Row 3: "Baixado" -> principal: Rebaixar/redownload (ghost, destructive), menu: replace + delete(destructive)
  it('a downloaded episode has redownload as the principal action (ghost, destructive) and replace/delete in the menu', () => {
    const ep = makeEpisode({ is_aired: true, is_downloaded: true })
    const result = episodeActions(ep, undefined)
    expect(result.principal).toEqual({ id: 'redownload', labelKey: 'redownload', variant: 'ghost', destructive: true })
    expect(result.menu.map((a) => a.id)).toEqual(['replace', 'delete'])
    expect(result.menu.find((a) => a.id === 'delete')?.destructive).toBe(true)
  })

  // Row 4: "Bloqueado ou gerenciado manualmente" -> principal: Soltar (ghost), menu: download + delete(destructive)
  it('a blocked episode has release as the principal action (ghost) and download/delete in the menu', () => {
    const ep = makeEpisode({ is_aired: true, is_downloaded: false, is_blocked: true })
    const result = episodeActions(ep, undefined)
    expect(result.principal).toEqual({ id: 'release', labelKey: 'release', variant: 'ghost' })
    expect(result.menu.map((a) => a.id)).toEqual(['download', 'delete'])
    expect(result.menu.find((a) => a.id === 'delete')?.destructive).toBe(true)
  })

  it('a manually-managed episode is treated the same as a blocked one', () => {
    const ep = makeEpisode({ is_aired: true, is_downloaded: false, is_manually_managed: true })
    const result = episodeActions(ep, undefined)
    expect(result.principal?.id).toBe('release')
    expect(result.menu.map((a) => a.id)).toEqual(['download', 'delete'])
  })

  // Row 5: "Não lançou" -> nada
  it('an episode that has not aired yet has no principal action and an empty menu', () => {
    const ep = makeEpisode({ is_aired: false, is_downloaded: false })
    const result = episodeActions(ep, undefined)
    expect(result.principal).toBeUndefined()
    expect(result.menu).toEqual([])
  })

  it('a blocked/manually-managed episode takes priority over an active torrent', () => {
    const ep = makeEpisode({ is_aired: true, is_downloaded: false, is_blocked: true })
    const torrent = makeTorrent({ status: 'downloading' })
    const result = episodeActions(ep, torrent)
    expect(result.principal?.id).toBe('release')
  })
})
