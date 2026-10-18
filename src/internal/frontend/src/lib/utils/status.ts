import type { AnimeInfo } from '../api/client.js'

export type SortKey = 'name' | 'episodes_watched' | 'last_download_date'
export type SortDir = 'asc' | 'desc'

/**
 * NFKD + stripping diacritics: makes "Kimi no Na wa" match "kimi no na wa" and full-width
 * katakana match the normal form. Kanji passes through intact.
 */
const normalizeName = (s: string) => s.normalize('NFKD').replace(/\p{Diacritic}/gu, '').toLowerCase()

export function filterAnimes(
  animes: AnimeInfo[],
  search: string,
  filterUnwatched: boolean,
  filterStandalone = false,
): AnimeInfo[] {
  const term = normalizeName(search.trim())
  return animes.filter(a => {
    // Search by ANY of the anime's titles — the user may only know the romaji, the kanji or a
    // synonym, while the card shows English.
    if (term && ![a.name, ...(a.alt_names ?? [])].some(n => normalizeName(n).includes(term))) return false
    if (filterUnwatched && a.episodes_watched >= a.episodes_released && a.episodes_released > 0) return false
    if (filterStandalone && !a.is_standalone) return false
    return true
  })
}

export function sortAnimes(
  animes: AnimeInfo[],
  sortKey: SortKey,
  sortDir: SortDir,
): AnimeInfo[] {
  return [...animes].sort((a, b) => {
    if (sortKey === 'name') {
      return sortDir === 'asc' ? a.name.localeCompare(b.name) : b.name.localeCompare(a.name)
    }
    const valA: number = sortKey === 'last_download_date'
      ? new Date(a.last_download_date || '1970-01-01').getTime()
      : a.episodes_watched
    const valB: number = sortKey === 'last_download_date'
      ? new Date(b.last_download_date || '1970-01-01').getTime()
      : b.episodes_watched
    if (valA < valB) return sortDir === 'asc' ? -1 : 1
    if (valA > valB) return sortDir === 'asc' ? 1 : -1
    return 0
  })
}

export function computeNextCheckIn(
  lastCheck: string,
  checkInterval: number,
  status: string,
  now: number,
): string | null {
  if (!lastCheck || !checkInterval || status === 'stopped') return null
  const last = new Date(lastCheck).getTime()
  if (isNaN(last) || last < new Date('2010-01-01').getTime()) return null
  const next = last + checkInterval * 60 * 1000
  const diff = next - now
  if (diff <= 0) return 'soon'
  const mins = Math.floor(diff / 60000)
  const secs = Math.floor((diff % 60000) / 1000)
  return mins > 0 ? `${mins}m ${secs}s` : `${secs}s`
}

const GIB = 1024 ** 3

export function formatBytes(bytes: number): string {
  return `${(bytes / GIB).toFixed(1)} GB`
}

// The "low disk" threshold lives on the server (StatusResponse.disk_low, computed with
// min_free_disk_percent): a duplicated calculation here would end up disagreeing with what
// the daemon is doing. Don't reintroduce the ratio to save a field on the response.

export type ProgressBreakdown = {
  watched: number
  downloaded: number
  released: number
  total: number
}

/**
 * Normalizes an anime's counters to what TripleProgressBar expects:
 * watched <= downloaded <= released <= total, all inside the same denominator.
 *
 * The cascade clamp is not cosmetic. `total_episodes` comes from AniList and is 0 while the
 * anime is airing with no announced count — in that case the denominator becomes
 * `episodes_released`. And the counters can cross in practice: the daemon marks an episode
 * as watched from AniList progress, which the user updates on their own, so
 * `episodes_watched` can pass `episodes_released` without anything being wrong. Without the
 * chained `Math.max` the bar would render a negative-width segment.
 *
 * `downloaded` does NOT come from `episodes_downloaded`: that field counts files on disk NOW
 * and drops when the daemon prunes a watched episode (`watched_episodes_to_keep`), which
 * pushed already-downloaded-and-watched episodes into the yellow segment ("still to
 * download"). Only `episodes_pending` survives pruning, because the backend walks it by
 * episode number — the same fix animeState.ts already applied to the "behind" chip.
 */
export function breakdown(a: AnimeInfo): ProgressBreakdown {
  const total = a.total_episodes > 0 ? a.total_episodes : a.episodes_released
  const watched = Math.min(Math.max(a.episodes_watched, 0), total)
  const released = Math.min(Math.max(a.episodes_released, watched), total)
  const downloaded = Math.max(released - Math.max(a.episodes_pending, 0), watched)
  return { watched, downloaded, released, total }
}
