import type { TorrentInfo } from '../api/client.js'

export type SortKey = 'default' | 'name' | 'progress' | 'download_speed' | 'eta' | 'size' | 'peers'
export type SortDir = 'asc' | 'desc'
export type ViewState = { query: string; statuses: string[]; sort: SortKey; dir: SortDir }

const SORT_KEYS: SortKey[] = ['default', 'name', 'progress', 'download_speed', 'eta', 'size', 'peers']
const SORT_DIRS: SortDir[] = ['asc', 'desc']

// Slugs emitted by src/internal/torrents/status.go (statusSlug). Kept in sync by hand —
// see that file's comment for why it isn't derived from the rain enum.
const KNOWN_STATUSES = [
  'stopped',
  'downloading_metadata',
  'allocating',
  'verifying',
  'downloading',
  'seeding',
  'stopping',
]

export const DEFAULT_VIEW_STATE: ViewState = { query: '', statuses: [], sort: 'default', dir: 'asc' }

/** Strips diacritics and lowercases, so "Frieren" matches "frieren" and "Attaque" matches "ataque". */
function foldText(s: string): string {
  return s
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
}

/** Filters by free-text query (matched against anime_name and name) and by status multi-select. */
export function filterTorrents(
  list: TorrentInfo[],
  v: Pick<ViewState, 'query' | 'statuses'>,
): TorrentInfo[] {
  const query = foldText(v.query.trim())
  const statuses = v.statuses

  return list.filter((t) => {
    if (query) {
      const haystack = foldText(`${t.anime_name ?? ''} ${t.name ?? ''}`)
      if (!haystack.includes(query)) return false
    }
    if (statuses.length > 0 && !statuses.includes(t.status)) return false
    return true
  })
}

function compareEta(a: TorrentInfo, b: TorrentInfo, dir: SortDir): number {
  const aNull = a.eta_seconds === null || a.eta_seconds === undefined
  const bNull = b.eta_seconds === null || b.eta_seconds === undefined
  // null ETA always sorts last, regardless of direction.
  if (aNull && bNull) return 0
  if (aNull) return 1
  if (bNull) return -1
  const diff = (a.eta_seconds as number) - (b.eta_seconds as number)
  return dir === 'asc' ? diff : -diff
}

function compareBy(key: SortKey, a: TorrentInfo, b: TorrentInfo, dir: SortDir): number {
  if (key === 'eta') return compareEta(a, b, dir)

  let diff: number
  switch (key) {
    case 'name':
      diff = (a.name ?? '').localeCompare(b.name ?? '')
      break
    case 'progress':
      diff = a.progress - b.progress
      break
    case 'download_speed':
      diff = a.download_speed - b.download_speed
      break
    case 'size':
      diff = a.bytes_total - b.bytes_total
      break
    case 'peers':
      diff = a.peers_total - b.peers_total
      break
    default:
      diff = 0
  }
  return dir === 'asc' ? diff : -diff
}

/**
 * Sorts a copy of the list (input is never mutated) by the given key/direction.
 * 'default' returns the list as received — the backend already orders it (incomplete
 * torrents first, then alphabetical) — and `dir` is ignored in that case.
 */
export function sortTorrents(list: TorrentInfo[], sort: SortKey, dir: SortDir): TorrentInfo[] {
  if (sort === 'default') return list.slice()
  return list.slice().sort((a, b) => compareBy(sort, a, b, dir))
}

/**
 * Serializes view state into a querystring. Fields at their default value are omitted so a
 * "clean" view (no filters/sort) round-trips to an empty querystring. `dir` is only ever
 * emitted when `sort !== 'default'`, since direction is meaningless for the default order.
 */
export function encodeViewState(v: ViewState): string {
  const params = new URLSearchParams()
  if (v.query.trim()) params.set('q', v.query)
  if (v.statuses.length > 0) params.set('status', v.statuses.join(','))
  if (v.sort !== 'default') {
    params.set('sort', v.sort)
    params.set('dir', v.dir)
  }
  return params.toString()
}

/** Parses a querystring into view state. Tolerant of garbage: unknown values fall back to defaults. */
export function decodeViewState(qs: string): ViewState {
  const params = new URLSearchParams(qs)

  const query = params.get('q') ?? DEFAULT_VIEW_STATE.query

  const rawStatus = params.get('status')
  const statuses = rawStatus
    ? rawStatus
        .split(',')
        .map((s) => s.trim())
        .filter((s) => KNOWN_STATUSES.includes(s))
    : DEFAULT_VIEW_STATE.statuses

  const rawSort = params.get('sort')
  const sort: SortKey = SORT_KEYS.includes(rawSort as SortKey) ? (rawSort as SortKey) : DEFAULT_VIEW_STATE.sort

  const rawDir = params.get('dir')
  const dir: SortDir =
    sort === 'default'
      ? DEFAULT_VIEW_STATE.dir
      : SORT_DIRS.includes(rawDir as SortDir)
        ? (rawDir as SortDir)
        : DEFAULT_VIEW_STATE.dir

  return { query, statuses, sort, dir }
}
