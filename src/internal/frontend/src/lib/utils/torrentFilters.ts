import type { TorrentInfo } from '../api/client.js'

export type SortKey = 'default' | 'name' | 'progress' | 'download_speed' | 'eta' | 'size' | 'peers'
export type SortDir = 'asc' | 'desc'
/**
 * `problems` and `closed` are Fase 5 additions (spec §9.3):
 *
 * - `problems` backs the "Problemas" filter pill. It is NOT a status slug set — see
 *   `isProblemTorrent` for the definition — so it needs its own dimension instead of riding on
 *   `statuses`. Keeping `statuses` untouched also means old deep links (`?status=downloading`)
 *   decode exactly as they always did.
 * - `closed` holds the accordion groups the user COLLAPSED, not the ones they expanded. The
 *   spec asks for the open set to survive in the URL; encoding the closed set is the same
 *   information (it's the complement) but makes "everything expanded" — the useful default —
 *   the state that serializes to an empty querystring, so a clean view still round-trips to a
 *   clean URL.
 */
export type ViewState = {
  query: string
  statuses: string[]
  problems: boolean
  sort: SortKey
  dir: SortDir
  closed: string[]
}

const SORT_KEYS: SortKey[] = ['default', 'name', 'progress', 'download_speed', 'eta', 'size', 'peers']
const SORT_DIRS: SortDir[] = ['asc', 'desc']

// Slugs emitted by src/internal/torrents/status.go (statusSlug), plus 'queued', the one
// the download queue writes itself (queue.markQueued). Kept in sync by hand — see
// status.go's comment for why it isn't derived from the rain enum.
const KNOWN_STATUSES = [
  'stopped',
  'queued',
  'downloading_metadata',
  'allocating',
  'verifying',
  'downloading',
  'seeding',
  'stopping',
]

export const DEFAULT_VIEW_STATE: ViewState = {
  query: '',
  statuses: [],
  problems: false,
  sort: 'default',
  dir: 'asc',
  closed: [],
}

/**
 * A torrent "in trouble": actively trying to transfer but with nobody on the other end.
 *
 * Deliberately NOT "status is stopped/stopping" — a paused torrent is a user decision, not a
 * problem, and lumping it in here would make the pill fire constantly for people who pause
 * things on purpose. Completed torrents are excluded too: a finished download sitting at zero
 * peers is just a seed nobody wants right now, which is normal.
 */
export function isProblemTorrent(t: TorrentInfo): boolean {
  if (t.completed) return false
  // 'queued' entra aqui junto com os parados: um torrent que a fila de downloads segurou
  // tem 0 peers POR DEFINICAO, e sinalizar isso como problema encheria a pill de vermelho
  // toda vez que o limite de downloads simultaneos fizesse o trabalho dele.
  if (t.status === 'stopped' || t.status === 'stopping' || t.status === 'queued') return false
  return t.peers_total === 0
}

/** Strips diacritics and lowercases, so "Frieren" matches "frieren" and "Attaque" matches "ataque". */
function foldText(s: string): string {
  return s
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
}

/**
 * Filters by free-text query (matched against anime_name and name), by status multi-select,
 * and by the problems flag. `problems` is optional so existing callers that only pass
 * query/statuses keep compiling and behaving identically.
 */
export function filterTorrents(
  list: TorrentInfo[],
  v: Pick<ViewState, 'query' | 'statuses'> & Partial<Pick<ViewState, 'problems'>>,
): TorrentInfo[] {
  const query = foldText(v.query.trim())
  const statuses = v.statuses

  return list.filter((t) => {
    if (query) {
      const haystack = foldText(`${t.anime_name ?? ''} ${t.name ?? ''}`)
      if (!haystack.includes(query)) return false
    }
    if (statuses.length > 0 && !statuses.includes(t.status)) return false
    if (v.problems && !isProblemTorrent(t)) return false
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
  if (v.problems) params.set('problems', '1')
  if (v.sort !== 'default') {
    params.set('sort', v.sort)
    params.set('dir', v.dir)
  }
  if (v.closed.length > 0) params.set('closed', v.closed.join(','))
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

  const problems = params.get('problems') === '1'

  // Group keys are opaque strings produced by `groupKey()`; there is no list of valid ones to
  // validate against (they depend on whatever torrents exist right now), so this only drops
  // empties. A stale key for a group that no longer exists is harmless — it matches nothing.
  const rawClosed = params.get('closed')
  const closed = rawClosed
    ? rawClosed
        .split(',')
        .map((s) => s.trim())
        .filter((s) => s.length > 0)
    : DEFAULT_VIEW_STATE.closed

  return { query, statuses, problems, sort, dir, closed }
}

/**
 * The four filter pills of spec §9.3, as presets over the underlying view state rather than a
 * fifth state field. Three of them are just status-slug sets; "problems" is the one that isn't
 * expressible as slugs (see `isProblemTorrent`) and so rides on its own flag.
 *
 * Presets are mutually exclusive in the UI, but the state they write to is the SAME `statuses`
 * array the old status multi-select used — which is why a pre-existing deep link like
 * `?status=verifying` still filters exactly as it always did. It simply lights up no pill,
 * because it matches none of the four presets, and `activeFilterPreset` returns null.
 */
export type FilterPreset = 'all' | 'downloading' | 'seeding' | 'problems'

/** Every slug that means "this torrent is working towards being complete". */
export const DOWNLOADING_SLUGS = ['downloading', 'downloading_metadata', 'allocating', 'verifying']

export function applyFilterPreset(v: ViewState, preset: FilterPreset): ViewState {
  const statuses = preset === 'downloading' ? DOWNLOADING_SLUGS : preset === 'seeding' ? ['seeding'] : []
  return { ...v, statuses, problems: preset === 'problems' }
}

function sameSet(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((x) => b.includes(x))
}

/** Which pill (if any) the current state corresponds to. null = a filter no pill represents. */
export function activeFilterPreset(v: Pick<ViewState, 'statuses' | 'problems'>): FilterPreset | null {
  if (v.problems) return 'problems'
  if (v.statuses.length === 0) return 'all'
  if (sameSet(v.statuses, DOWNLOADING_SLUGS)) return 'downloading'
  if (sameSet(v.statuses, ['seeding'])) return 'seeding'
  return null
}

/**
 * Stable identity for an accordion group. `anime_id` is the real key; `anime_name` is the
 * fallback for torrents the backend couldn't link to an anime (manual magnets), and everything
 * with neither collapses into one "other" bucket rather than becoming a group per torrent.
 *
 * Prefixed (`a:` / `n:`) so a numeric id and an anime literally named "123" can't collide.
 */
export function groupKey(t: TorrentInfo): string {
  if (t.anime_id !== undefined && t.anime_id !== null) return `a:${t.anime_id}`
  if (t.anime_name) return `n:${t.anime_name}`
  return 'other'
}

export interface TorrentGroup {
  key: string
  name: string
  animeId?: number
  torrents: TorrentInfo[]
  /** 0..1 aggregate — see `groupTorrents` for why this isn't bytes_completed/bytes_total. */
  progress: number
  downloadSpeed: number
  uploadSpeed: number
  bytesTotal: number
  problemCount: number
  downloadingCount: number
}

/**
 * Buckets torrents by anime and orders the buckets the way spec §9.3 asks: problems first,
 * then anything downloading, then the rest (seeding/paused), alphabetical within each rank.
 * Row order INSIDE a group is left to the caller's sort key — group order is a fixed severity
 * rule, so a user sorting by "peers" still gets the broken groups at the top where they can
 * see them.
 *
 * Aggregate progress is a bytes_total-weighted mean of each torrent's own `progress`, NOT
 * sum(bytes_completed)/sum(bytes_total): rain frees piece data when a torrent is stopped, so
 * `bytes_completed` drops to 0 for paused entries while `progress` keeps reporting the true
 * fraction from the piece count. Summing bytes would make a group's bar collapse the moment
 * one of its torrents was paused.
 */
export function groupTorrents(list: TorrentInfo[]): TorrentGroup[] {
  const byKey = new Map<string, TorrentGroup>()

  for (const t of list) {
    const key = groupKey(t)
    let group = byKey.get(key)
    if (!group) {
      group = {
        key,
        name: t.anime_name || t.name || key,
        animeId: t.anime_id ?? undefined,
        torrents: [],
        progress: 0,
        downloadSpeed: 0,
        uploadSpeed: 0,
        bytesTotal: 0,
        problemCount: 0,
        downloadingCount: 0,
      }
      byKey.set(key, group)
    }
    group.torrents.push(t)
    group.downloadSpeed += t.download_speed
    group.uploadSpeed += t.upload_speed
    group.bytesTotal += t.bytes_total
    if (isProblemTorrent(t)) group.problemCount++
    if (t.status === 'downloading') group.downloadingCount++
  }

  const groups = Array.from(byKey.values())

  for (const group of groups) {
    const weighted = group.torrents.reduce((sum, t) => sum + t.progress * (t.bytes_total || 0), 0)
    group.progress =
      group.bytesTotal > 0
        ? weighted / group.bytesTotal
        : // No torrent in this group has known metadata yet — fall back to a plain mean so the
          // bar still reflects whatever progress rain does report.
          group.torrents.reduce((sum, t) => sum + t.progress, 0) / (group.torrents.length || 1)
  }

  function rank(g: TorrentGroup): number {
    if (g.problemCount > 0) return 0
    if (g.downloadingCount > 0) return 1
    return 2
  }

  return groups.sort((a, b) => rank(a) - rank(b) || a.name.localeCompare(b.name))
}

/**
 * Ordem em que os episódios de um grupo devem ser priorizados: crescente por
 * `episode_number`, com batch e episódio sem número no fim, preservando a ordem de exibição
 * entre eles. O backend aplica a lista NA ORDEM RECEBIDA, então quem decide é aqui.
 *
 * Nada a ver com `sortTorrents`: aquilo ordena o que a tela mostra, isto ordena o que a
 * requisição envia — o usuário pode estar ordenando por peers e ainda assim querer o
 * episódio 1 na frente da fila.
 */
export function prioritizeOrder(list: TorrentInfo[]): string[] {
  const num = (t: TorrentInfo) => (t.episode_number === null || t.episode_number === undefined ? null : t.episode_number)
  return list
    .map((t, i) => ({ t, i }))
    .sort((a, b) => {
      const an = num(a.t)
      const bn = num(b.t)
      if (an === null && bn === null) return a.i - b.i
      if (an === null) return 1
      if (bn === null) return -1
      return an - bn || a.i - b.i
    })
    .map((x) => x.t.hash)
}

/**
 * Alvos do priorizar em lote a partir da SELEÇÃO, na ordem em que o usuário marcou as linhas —
 * o `Set` preserva ordem de inserção, e é ela que vale aqui, não a ordem de exibição (que é o
 * que `bulkTargets` daria). Hashes que já saíram da lista ou que não aceitam priorizar caem
 * fora: o backend os ignoraria de qualquer jeito, e contá-los inflaria a mensagem de resultado.
 */
export function selectionPrioritizeOrder(
  selected: Set<string>,
  visible: TorrentInfo[],
  canPrioritize: (t: TorrentInfo) => boolean,
): string[] {
  const byHash = new Map(visible.map((t) => [t.hash, t]))
  return Array.from(selected).filter((h) => {
    const t = byHash.get(h)
    return t !== undefined && canPrioritize(t)
  })
}
