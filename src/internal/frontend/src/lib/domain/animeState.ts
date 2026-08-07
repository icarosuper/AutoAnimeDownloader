import type { AnimeInfo, TorrentInfo } from '../api/client.js'

/**
 * `deriveAnimeChip` (spec §7, Fase 2) — pure cascade that turns an anime + its related
 * torrents into ONE chip state. No Svelte, no i18n: the return value is a KEY plus the raw
 * numbers a label needs (episode number, percent, gap count), never a rendered string. This
 * is deliberate — see the module-level note below for why the boundary sits here.
 *
 * i18n boundary: the cascade below decides WHICH state applies (a business rule, testable
 * without a locale). Turning `AnimeChipState` into visible text ("Baixando ep. 5 · 42%",
 * "Atrasado 3 ep") is a Paraglide concern that belongs to whichever screen renders the chip
 * (Fase 3+) — it switches on `key` and interpolates `episodeNumber`/`percent`/`behindCount`
 * into the matching message function. Baking a string in here would force this module to
 * import the message tree and make every test locale-sensitive for no reason.
 */

export type AnimeChipKey =
  | 'blacklisted'
  | 'downloading'
  | 'noSeeds'
  | 'paused'
  | 'awaitingPremiere'
  | 'upToDate'
  | 'behind'

export type AnimeChipVariant = 'accent' | 'ok' | 'warn' | 'danger' | 'neutral'

export interface AnimeChipState {
  key: AnimeChipKey
  variant: AnimeChipVariant
  /** Only set for `blacklisted` — spec §7 calls for an dimmed/esmaecida row. */
  dimmed?: boolean
  /** Only set for `downloading`. */
  episodeNumber?: number
  /** Only set for `downloading`, 0-100. */
  percent?: number
  /** Only set for `behind` — how many released episodes aren't downloaded yet. */
  behindCount?: number
}

// spec §8 (stallTracker): the "sem seeds" branch fires once a torrent has sat at
// peers_total===0 for at least this long. The store only does the bookkeeping (since when is
// a hash at zero peers); the threshold itself is a chip business rule, so it lives here.
const NO_SEEDS_THRESHOLD_MS = 10 * 60 * 1000

/**
 * `stalledSince` maps torrent hash -> the timestamp (ms) at which its peers_total was first
 * observed at 0. It comes from `lib/stores/stallTracker.ts`, which is the piece allowed to
 * touch Svelte; this function stays pure by taking the map as data instead of reading the
 * store itself. Screens read `$stallTracker` and pass it straight through.
 */
export function deriveAnimeChip(
  anime: AnimeInfo,
  torrents: TorrentInfo[],
  now: number,
  stalledSince: Map<string, number> = new Map(),
): AnimeChipState {
  // Cascade order is the spec (§7): first branch that matches wins.
  if (anime.is_blacklisted) {
    return { key: 'blacklisted', variant: 'neutral', dimmed: true }
  }

  // Join anime<->torrent via anime_id (TorrentInfo.anime_id), per spec — there is no
  // anilist_status branch, that field doesn't exist on this endpoint.
  const related = torrents.filter((t) => t.anime_id === anime.anime_id)

  const downloading = related.find((t) => t.status === 'downloading')
  if (downloading) {
    return {
      key: 'downloading',
      variant: 'accent',
      episodeNumber: downloading.episode_number ?? undefined,
      percent: Math.round((downloading.progress || 0) * 100),
    }
  }

  const stalled = related.find((t) => {
    if (t.peers_total !== 0) return false
    const since = stalledSince.get(t.hash)
    if (since === undefined) return false
    return now - since >= NO_SEEDS_THRESHOLD_MS
  })
  if (stalled) {
    return { key: 'noSeeds', variant: 'danger' }
  }

  const stopped = related.find((t) => t.status === 'stopped')
  if (stopped) {
    return { key: 'paused', variant: 'neutral' }
  }

  if (anime.episodes_released === 0) {
    return { key: 'awaitingPremiere', variant: 'neutral' }
  }

  // `episodes_pending` ja desconta o que foi assistido (o daemon nunca baixa episodio abaixo do
  // progresso da AniList, entao ele nao e atraso) — ver countPendingEpisodes no backend.
  if (anime.episodes_pending === 0) {
    return { key: 'upToDate', variant: 'ok' }
  }

  return {
    key: 'behind',
    variant: 'warn',
    behindCount: anime.episodes_pending,
  }
}
