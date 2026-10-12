import { toasts } from '../stores/toasts.js'
import { backendHealth } from '../stores/backendHealth.js'

// Uses the page's current host when no environment variable is set
// This lets the frontend work both locally and when accessed remotely
const getApiBaseUrl = (): string => {
  if (import.meta.env.VITE_API_BASE_URL) {
    return import.meta.env.VITE_API_BASE_URL
  }

  if (typeof window === 'undefined') {
    // Fallback for non-browser environments (build time, SSR, etc.)
    return 'http://localhost:8091/api/v1'
  }

  // Uses a relative path since the frontend is served by the backend itself
  // This works both locally and through Cloudflare Tunnel/a reverse proxy
  return `${window.location.origin}/api/v1`
}

const API_BASE_URL = getApiBaseUrl()

interface ApiResponse<T> {
  success: boolean
  data: T
  error?: {
    code: string
    message: string
  }
}

/**
 * API error that keeps the envelope's `code`. The message is still the server's (nothing changes
 * for whoever only displays it), but whoever needs to DECIDE by the reason — the 409 of the
 * standalone POST, whose five codes become five different toasts — doesn't need to parse text for
 * it.
 */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly code: string,
    readonly status: number,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

interface ApiRequestOptions {
  /** Cancels the request. The add screen's search aborts the previous one on every submit. */
  signal?: AbortSignal
  /**
   * Suppresses the automatic error toast on failure. For endpoints polled on a short interval
   * (e.g. getTorrents(), hit every 2-15s by Downloads/Status/AnimeDetail) a transient failure
   * must not spam a toast on every tick — the caller degrades silently and shows nothing instead.
   * The error is still logged and still rethrown, so callers that DO want to surface it (their
   * own try/catch + toast.error) keep working exactly as before.
   */
  silent?: boolean
}

async function apiRequest<T>(
  method: string,
  endpoint: string,
  body: unknown = null,
  opts: ApiRequestOptions = {},
): Promise<T> {
  const url = `${API_BASE_URL}${endpoint}`
  const options: RequestInit = {
    method,
    headers: {
      'Content-Type': 'application/json',
    },
    signal: opts.signal,
  }

  if (body) {
    options.body = JSON.stringify(body)
  }

  let responseStatus = 0
  try {
    const response = await fetch(url, options)
    responseStatus = response.status
    const data: ApiResponse<T> = await response.json()

    if (!response.ok) {
      throw new ApiError(
        data.error?.message || `HTTP error! status: ${response.status}`,
        data.error?.code ?? 'UNKNOWN',
        response.status,
      )
    }

    backendHealth.recordSuccess(endpoint)
    return data.data
  } catch (error) {
    console.error('API request failed:', error)
    // Before any toast: a repeated 5xx on an endpoint with `silent: true` was INVISIBLE — the app
    // degraded silently and the user only saw frozen numbers. Backend health is counted here
    // precisely because this is the only point every request goes through.
    backendHealth.recordFailure(endpoint, responseStatus)
    if (!opts.silent) {
      const isAnilistEndpoint = /\/animes\/\d+\/episodes$/.test(endpoint)
      const message =
        isAnilistEndpoint && (responseStatus === 500 || responseStatus === 0)
          ? 'Failed to reach AniList'
          : error instanceof Error
            ? error.message
            : 'Unknown error'
      toasts.add(message)
    }
    throw error
  }
}

export interface StatusResponse {
  status: string
  last_check: string
  has_error: boolean
  version: string
  disk_total: number
  disk_free: number
  /**
   * Free space below min_free_disk_percent: the daemon stopped adding torrents. Computed on the
   * server on purpose — a threshold duplicated in the frontend would disagree with the daemon.
   */
  disk_low: boolean
  /**
   * Optional on purpose: the WebSocket's `status_update` carries only three fields, and Status
   * synthesizes a StatusResponse from it. Also absent on a daemon older than the frontend.
   * Consumers treat absence as "no information", never as "all is well".
   */
  anilist?: AnilistHealth
}

/**
 * Last known AniList state, written by any call (the daemon's pass or the frontend's poll). A code,
 * never a ready sentence: the daemon doesn't know the browser's locale.
 */
export interface AnilistHealth {
  state: 'ok' | 'rate_limited' | 'outage' | 'app_bug'
  /** AniList's raw message — a blocked-IP 403 carries the reason written out to be read. */
  message?: string
  /** End of the 429 timeout, derived from Retry-After. Absent when it doesn't apply. */
  retry_at?: string
  since?: string
  /**
   * Date of the disk cache snapshot. Absent when there never was one. It's not a fact about
   * AniList: it's what answers "how old is what I'm seeing?" while it's down.
   */
  cache_saved_at?: string
}

export interface WebhookPreset {
  name: string
  url: string
  method: string
  headers: Record<string, string>
  body: string
  events: string[]
}

export interface Priorities {
  criteria_order: string[]
  fansubs: string[]
  resolutions: string[]
  sources: string[]
  codecs: string[]
  audio: string[]
  ignore_list: string[]
}

export interface Config {
  anilist_username?: string
  anilist_usernames: string[]
  mal_usernames?: string[]
  completed_anime_path: string
  check_interval: number
  max_episodes_per_anime: number
  /** Tetos de tamanho de torrent em GiB. 0 desliga. */
  max_batch_torrent_size_gb: number
  max_episode_torrent_size_gb: number
  /** Seeders floor: a torrent below this is dropped from the search. 0 disables. */
  min_seeders: number
  /** Ceiling of Nyaa pages per search. The search only goes deeper if it has few candidates. */
  max_search_pages: number
  /** Abaixo desta porcentagem de espaco livre o download e pausado. 0 desliga. */
  min_free_disk_percent: number
  episode_retry_limit: number
  max_concurrent_downloads: number
  delete_watched_episodes: boolean
  watched_episodes_to_keep: number
  excluded_list?: string
  excluded_lists: string[]
  rename_files_for_jellyfin: boolean
  download_statuses: string[]
  download_media_statuses: string[]
  delete_statuses: string[]
  notifications: {
    webhooks: WebhookPreset[]
    batch_window_seconds: number
  }
  priorities: Priorities
}

export interface AnimeInfo {
  anime_id: number
  name: string
  episodes_downloaded: number
  episodes_released: number
  episodes_watched: number
  /** Released, not watched and not downloaded — the "behind" count. */
  episodes_pending: number
  total_episodes: number
  latest_episode_number: number
  last_download_date: string
  cover_image?: string
  is_blacklisted?: boolean
  /** Tracked by the standalone file, not by an AniList or MyAnimeList list. Origin, not state. */
  is_standalone?: boolean
  /**
   * Unix timestamp (seconds) of the next episode's premiere; absent when no episode is scheduled.
   * It's a timestamp, not a ready countdown, so the countdown doesn't freeze with a cached response
   * — whoever renders counts down on the client.
   */
  next_airing_at?: number
  /**
   * Other titles of the same anime (romaji, English, kanji, synonyms). Only the search uses them.
   */
  alt_names?: string[]
}

/** Reasons an anime can't be added as standalone — they come ready from the server. */
export type BlockReason = '' | 'blacklist' | 'standalone' | 'tracked' | 'downloaded'

export interface AniListSearchResult {
  id: number
  title: string
  format: string
  status: string
  year: number
  /** 0 when AniList doesn't know the total (common for an anime still airing). */
  episodes: number
  cover?: string
  block_reason: BlockReason
}

export interface LogsResponse {
  lines: string[]
}

export interface TorrentInfo {
  hash: string
  name: string
  status: string
  /** 1-based place in the download queue's waiting line; 0 = not waiting. */
  queue_position: number
  completed: boolean
  anime_name?: string
  anime_id?: number
  episode_number: number | null
  is_batch: boolean
  bytes_completed: number
  bytes_total: number
  bytes_uploaded: number
  progress: number
  download_speed: number
  upload_speed: number
  peers_total: number
  eta_seconds: number | null
  seeded_for_seconds: number
}

/** A file inside a torrent. */
export interface TorrentFile {
  /** RAW relative path, as in the metadata — fansub, resolution and codec come from here. */
  path: string
  size: number
  /** null = unknown (stopped torrent: rain releases the pieces). It's not "nothing downloaded". */
  bytes_completed: number | null
  /** Number extracted from the FILE NAME by the backend; null when nothing matches. */
  episode: number | null
  /**
   * Video codec read from the file HEADER (Matroska/EBML), not from the name — most releases don't
   * write the codec in the name. Absent when it can't be answered: not a `.mkv`, or not finished
   * downloading yet (rain doesn't download sequentially, so the header may not be on disk).
   */
  codec?: string
}

/** One line of the last check's report: an (anime, code) pair. */
export interface Issue {
  anime_id: number
  anime_name: string
  /** Only comes with a problem. A limit has no "affected episodes" — it has downloaded/pending. */
  episodes?: number[]
  code: string
  candidates?: number
  limit_gb?: number
  min_seeders?: number
  downloaded?: number
  pending?: number
  batch_skipped?: string
}

/** The report of the LAST pass, and only of it. It's not history. */
export interface CheckReport {
  finished_at: string
  pass_error: string
  /**
   * Classified cause of the abort. The frontend builds the sentence from HERE; `pass_error` is the
   * raw text, shown collapsed for whoever opens an issue.
   */
  pass_error_code?: string
  problems: Issue[]
  limits: Issue[]
}

export async function getStatus(opts: { silent?: boolean } = {}): Promise<StatusResponse> {
  // `silent` for pollers: the AppShell queries every 30s only to feed the banner, and a daemon
  // that's down can't become a toast every half minute.
  return apiRequest<StatusResponse>('GET', '/status', null, opts)
}

export async function getLastCheck(): Promise<CheckReport> {
  // Polled together with the torrents poll of Status and AnimeDetail: a transient failure should
  // degrade silently instead of toasting on every tick.
  return apiRequest<CheckReport>('GET', '/last-check', null, { silent: true })
}

export async function getConfig(): Promise<Config> {
  return apiRequest<Config>('GET', '/config')
}

export async function updateConfig(config: Config): Promise<void> {
  return apiRequest<void>('PUT', '/config', config)
}

export async function getPriorityDefaults(): Promise<Priorities> {
  return apiRequest<Priorities>('GET', '/config/priorities/defaults')
}

export async function getAnimes(): Promise<AnimeInfo[]> {
  return apiRequest<AnimeInfo[]>('GET', '/animes')
}

export interface AnimeEpisodeInfo {
  /**
   * The episode's identity in the action routes: (anime, number). There's no episode_id anymore.
   */
  episode_number: number
  airing_at: number
  time_until_airing: number
  is_aired: boolean
  is_watched: boolean
  is_downloaded: boolean
  download_date?: string
  is_manually_managed?: boolean
  is_blocked?: boolean
  episode_hash?: string
  /** The pack's real range (from the torrent name). Absent/0 = unknown — falls back to min/max. */
  batch_start?: number
  batch_end?: number
}

export interface AnimeDetailResponse {
  // anime_id is the AniList MEDIA id: it's the internal key and also builds the
  // anilist.co/anime/{id} link. There's no separate anilist_id anymore.
  anime_id: number
  total_episodes: number
  progress: number
  status: string
  cover_image?: string
  episodes: AnimeEpisodeInfo[]
}

export interface AnimeSettings {
  /** Manual progress — only read for a standalone anime (a list anime's comes from its list). */
  progress?: number
}

export async function getAnimeDetail(animeId: number): Promise<AnimeDetailResponse> {
  return apiRequest<AnimeDetailResponse>('GET', `/animes/${animeId}/episodes`)
}

export async function downloadEpisode(animeId: number, episodeNumber: number): Promise<void> {
  return apiRequest<void>('POST', `/animes/${animeId}/episodes/${episodeNumber}/download`)
}

export async function deleteEpisode(animeId: number, episodeNumber: number): Promise<void> {
  return apiRequest<void>('DELETE', `/animes/${animeId}/episodes/${episodeNumber}`)
}

export async function releaseEpisode(animeId: number, episodeNumber: number): Promise<void> {
  return apiRequest<void>('POST', `/animes/${animeId}/episodes/${episodeNumber}/release`)
}

export async function redownloadEpisode(animeId: number, episodeNumber: number): Promise<void> {
  return apiRequest<void>('POST', `/animes/${animeId}/episodes/${episodeNumber}/redownload`)
}

export async function replaceEpisodeWithMagnet(animeId: number, episodeNumber: number, magnet: string): Promise<void> {
  return apiRequest<void>('POST', `/animes/${animeId}/episodes/${episodeNumber}/replace`, { magnet })
}

export async function replaceAnimeWithMagnet(animeId: number, magnet: string): Promise<void> {
  return apiRequest<void>('POST', `/animes/${animeId}/replace`, { magnet })
}

export async function updateAnimeSettings(animeId: number, settings: AnimeSettings): Promise<void> {
  return apiRequest<void>('PUT', `/animes/${animeId}/settings`, settings)
}

export async function triggerCheck(): Promise<void> {
  return apiRequest<void>('POST', '/check')
}

export async function startDaemon(): Promise<void> {
  return apiRequest<void>('POST', '/daemon/start')
}

export async function stopDaemon(): Promise<void> {
  return apiRequest<void>('POST', '/daemon/stop')
}

export async function getLogs(lines?: number): Promise<LogsResponse> {
  const endpoint = lines ? `/logs?lines=${lines}` : '/logs'
  return apiRequest<LogsResponse>('GET', endpoint)
}

export async function getTorrents(): Promise<TorrentInfo[]> {
  // Polled on a short interval by Downloads.svelte (2s), Status.svelte (5s), and
  // AnimeDetail.svelte (2s/15s adaptive) — a transient failure must degrade silently rather
  // than toast on every tick.
  return apiRequest<TorrentInfo[]>('GET', '/torrents', null, { silent: true })
}

/**
 * Files inside a torrent. Fetched ON DEMAND (when the row expands), never embedded in `GET
 * /torrents`: that one is a 2s poll, and putting the list in it would multiply the payload by N for
 * data almost nobody is looking at.
 *
 * `silent` because the open panel refreshes on the screen's same poll tick.
 */
export async function getTorrentFiles(hash: string): Promise<TorrentFile[]> {
  return apiRequest<TorrentFile[]>('GET', `/torrents/${hash}/files`, null, { silent: true })
}

export async function pauseTorrent(hash: string): Promise<void> {
  return apiRequest<void>('POST', `/torrents/${hash}/pause`)
}

export async function resumeTorrent(hash: string): Promise<void> {
  return apiRequest<void>('POST', `/torrents/${hash}/resume`)
}

export async function prioritizeTorrent(hash: string): Promise<void> {
  return apiRequest<void>('POST', `/torrents/${hash}/prioritize`)
}

/**
 * Batch prioritize: ONE request, applied in the order given — N calls to `prioritizeTorrent`
 * would front-push past each other and reverse the batch. Unknown or already-completed hashes
 * are ignored by the backend, so the whole call cannot fail because one episode finished
 * between the render and the click.
 */
export async function prioritizeTorrents(hashes: string[]): Promise<void> {
  return apiRequest<void>('POST', '/torrents/prioritize', { hashes })
}

export async function announceTorrent(hash: string): Promise<void> {
  return apiRequest<void>('POST', `/torrents/${hash}/announce`)
}

export async function deleteTorrent(
  hash: string,
  opts: { keepData: boolean; block: boolean },
): Promise<void> {
  const params = new URLSearchParams({
    keep_data: String(opts.keepData),
    block: String(opts.block),
  })
  return apiRequest<void>('DELETE', `/torrents/${hash}?${params.toString()}`)
}

/**
 * Search on AniList. `silent` because the screen draws its own failure notice next to the results —
 * a toast on top of that would say the same thing twice.
 */
export async function searchAniList(
  q: string,
  includeUnreleased = false,
  signal?: AbortSignal,
): Promise<AniListSearchResult[]> {
  // The parameter only travels when it's true: the backend already assumes false, and sending
  // `=false` on every search only bloats the log URL.
  const unreleased = includeUnreleased ? '&include_unreleased=true' : ''
  return apiRequest<AniListSearchResult[]>(
    'GET',
    `/anilist/search?q=${encodeURIComponent(q)}${unreleased}`,
    null,
    { silent: true, signal },
  )
}

/**
 * Starts tracking a standalone anime and downloads whatever is already aired. Returns how many went
 * in.
 */
export async function addStandaloneAnime(mediaId: number): Promise<{ added: number }> {
  return apiRequest<{ added: number }>('POST', '/standalone-animes', { media_id: mediaId }, { silent: true })
}

export async function removeStandaloneAnime(mediaId: number, deleteEpisodes: boolean): Promise<void> {
  const params = new URLSearchParams({ delete_episodes: String(deleteEpisodes) })
  return apiRequest<void>('DELETE', `/standalone-animes/${mediaId}?${params.toString()}`)
}

export async function testWebhook(name: string): Promise<void> {
  const res = await fetch(
    `/api/v1/notifications/webhooks/${encodeURIComponent(name)}/test`,
    { method: 'POST' }
  )
  if (!res.ok) {
    const data = await res.json()
    throw new Error(data.error?.message ?? 'Failed to test webhook')
  }
}

