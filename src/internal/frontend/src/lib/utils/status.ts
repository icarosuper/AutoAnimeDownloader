import type { AccountSource, AnimeInfo } from '../api/client.js'

export type SortKey = 'name' | 'episodes_watched' | 'last_download_date'
export type SortDir = 'asc' | 'desc'

export type AnimeTypeFilter = 'all' | 'airing' | 'finished' | 'standalone' | 'blacklisted'

export type StatusFilterOptions = {
  unwatched?: boolean
  type?: AnimeTypeFilter
}

export type StatusViewState = {
  q: string
  unwatched: boolean
  type: AnimeTypeFilter
  /** Selected account keys (`accountKey`); empty = every account, i.e. no user filter. */
  accounts: string[]
}

export const DEFAULT_STATUS_VIEW: StatusViewState = {
  q: '',
  unwatched: false,
  type: 'all',
  accounts: [],
}

/**
 * The identity of a list account in the user filter and in the URL. The source is part of it
 * because the same username can exist on AniList and on MyAnimeList.
 */
export function accountKey(source: AccountSource, username: string): string {
  return `${source}:${username}`
}

/** A configured list account, as the user filter and the webhook source filter both list it. */
export type ListAccount = {
  key: string
  source: AccountSource
  username: string
  /** Only set when the same username exists on AniList and MAL — the label needs it then. */
  service?: 'AniList' | 'MAL'
}

/** Every configured account, AniList first, in config order. */
export function listAccounts(anilist: string[], mal: string[]): ListAccount[] {
  const all = [
    ...anilist.map(username => ({ source: 'anilist' as const, username })),
    ...mal.map(username => ({ source: 'mal' as const, username })),
  ]
  return all.map(acc => ({
    key: accountKey(acc.source, acc.username),
    source: acc.source,
    username: acc.username,
    ...(all.filter(o => o.username === acc.username).length > 1
      ? { service: acc.source === 'anilist' ? ('AniList' as const) : ('MAL' as const) }
      : {}),
  }))
}

/** The username alone; the service is spelled out only when the name is ambiguous. */
export function accountLabel(acc: { username: string; service?: string }): string {
  return acc.service ? `${acc.username} (${acc.service})` : acc.username
}

const ACCOUNT_KEY = /^(anilist|mal):.+$/

/** Splits an account key back into source and username; null for a malformed key. */
export function parseAccountKey(key: string): { source: AccountSource; username: string } | null {
  if (!ACCOUNT_KEY.test(key)) return null
  const i = key.indexOf(':')
  return { source: key.slice(0, i) as AccountSource, username: key.slice(i + 1) }
}

/**
 * Maps each selected key onto the configured key it names and drops the ones the config doesn't
 * have. The username matches case-insensitively — AniList and MAL usernames are — and the result
 * carries the CONFIGURED spelling, which is what the daemon stamps on `accounts` (a URL with
 * `anilist:alice` against a configured `Alice` would otherwise match nothing). The source must be
 * exact.
 */
export function canonicalAccounts(selected: string[], configured: string[]): string[] {
  const out: string[] = []
  for (const key of selected) {
    const sel = parseAccountKey(key)
    if (!sel) continue
    const match = configured.find(c => {
      const conf = parseAccountKey(c)
      return !!conf && conf.source === sel.source && conf.username.toLowerCase() === sel.username.toLowerCase()
    })
    if (match && !out.includes(match)) out.push(match)
  }
  return out
}

/**
 * Applies the user filter: keeps the animes at least one selected account tracks (OR), and
 * re-projects `episodes_watched` / `episodes_pending` onto THOSE accounts — the lowest progress
 * among them, same rule the backend uses across all accounts. Without the projection "Unwatched",
 * the progress bar, the "behind" chip and the sort would keep answering for the account furthest
 * behind, even an unselected one. Runs BEFORE filterAnimes, so every count downstream agrees.
 * An empty selection returns the list untouched (decisions.md #125).
 */
export function scopeToAccounts(animes: AnimeInfo[], selected: string[]): AnimeInfo[] {
  if (selected.length === 0) return animes
  const wanted = new Set(selected)
  const scoped: AnimeInfo[] = []
  for (const a of animes) {
    const mine = (a.accounts ?? []).filter(acc => wanted.has(accountKey(acc.source, acc.username)))
    if (mine.length === 0) continue
    scoped.push({
      ...a,
      episodes_watched: Math.min(...mine.map(acc => acc.episodes_watched)),
      episodes_pending: Math.max(...mine.map(acc => acc.episodes_pending)),
    })
  }
  return scoped
}

const TYPE_FILTERS: AnimeTypeFilter[] = ['all', 'airing', 'finished', 'standalone', 'blacklisted']

/**
 * NFKD + stripping diacritics: makes "Kimi no Na wa" match "kimi no na wa" and full-width
 * katakana match the normal form. Kanji passes through intact.
 */
const normalizeName = (s: string) => s.normalize('NFKD').replace(/\p{Diacritic}/gu, '').toLowerCase()

export function isAiringStatus(status?: string): boolean {
  return status === 'RELEASING' || status === 'HIATUS'
}

export function isFinishedStatus(status?: string): boolean {
  return status === 'FINISHED' || status === 'CANCELLED'
}

function matchesType(a: AnimeInfo, type: AnimeTypeFilter): boolean {
  switch (type) {
    case 'airing':
      return isAiringStatus(a.media_status)
    case 'finished':
      return isFinishedStatus(a.media_status)
    case 'standalone':
      return !!a.is_standalone
    case 'blacklisted':
      return !!a.is_blacklisted
    default:
      return true
  }
}

export function filterAnimes(
  animes: AnimeInfo[],
  search: string,
  options: StatusFilterOptions = {},
): AnimeInfo[] {
  return animes.filter(animeFilter(search, options))
}

function animeFilter(search: string, options: StatusFilterOptions): (a: AnimeInfo) => boolean {
  const term = normalizeName(search.trim())
  const unwatched = options.unwatched ?? false
  const type = options.type ?? 'all'
  return a => {
    // Search by ANY of the anime's titles — the user may only know the romaji, the kanji or a
    // synonym, while the card shows English.
    if (term && ![a.name, ...(a.alt_names ?? [])].some(n => normalizeName(n).includes(term))) return false
    if (unwatched && a.episodes_watched >= a.episodes_released && a.episodes_released > 0) return false
    if (!matchesType(a, type)) return false
    return true
  }
}

/**
 * For each key, what `filterAnimes(scopeToAccounts(animes, [key]), search, options).length` would
 * be — the count the user filter shows next to each account — in one pass over the list instead
 * of one pass per account. Each anime is tested with its watched/pending projected onto that one
 * account, like scopeToAccounts does.
 */
export function accountCounts(
  animes: AnimeInfo[],
  keys: string[],
  search: string,
  options: StatusFilterOptions = {},
): Record<string, number> {
  const counts: Record<string, number> = Object.fromEntries(keys.map(k => [k, 0]))
  const matches = animeFilter(search, options)
  for (const a of animes) {
    const seen = new Set<string>()
    for (const acc of a.accounts ?? []) {
      const key = accountKey(acc.source, acc.username)
      if (!(key in counts) || seen.has(key)) continue
      seen.add(key)
      if (matches({ ...a, episodes_watched: acc.episodes_watched, episodes_pending: acc.episodes_pending })) counts[key]++
    }
  }
  return counts
}

export function encodeStatusView(v: StatusViewState): string {
  const params = new URLSearchParams()
  if (v.q.trim()) params.set('q', v.q)
  if (v.unwatched) params.set('unwatched', '1')
  if (v.type !== 'all') params.set('type', v.type)
  for (const key of v.accounts) params.append('account', key)
  return params.toString()
}

export function decodeStatusView(qs: string): StatusViewState {
  const params = new URLSearchParams(qs)
  const typeRaw = params.get('type')
  const type = TYPE_FILTERS.includes(typeRaw as AnimeTypeFilter) ? (typeRaw as AnimeTypeFilter) : 'all'
  return {
    q: params.get('q') ?? '',
    unwatched: params.get('unwatched') === '1',
    type,
    // Only the shape is checked here; whether the account is still configured is the screen's call.
    accounts: [...new Set(params.getAll('account').filter(k => ACCOUNT_KEY.test(k)))],
  }
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
// min_free_disk_gb including reserved): a duplicated calculation here would end up disagreeing
// with what the daemon is doing. Don't reintroduce the ratio to save a field on the response.

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
