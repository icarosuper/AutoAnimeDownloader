import { toasts } from '../stores/toasts.js'

// Usa o host atual da página quando não há variável de ambiente definida
// Isso permite que o frontend funcione tanto localmente quanto quando acessado remotamente
const getApiBaseUrl = (): string => {
  if (import.meta.env.VITE_API_BASE_URL) {
    return import.meta.env.VITE_API_BASE_URL
  }

  if (typeof window === 'undefined') {
    // Fallback for non-browser environments (build time, SSR, etc.)
    return 'http://localhost:8091/api/v1'
  }

  // Usa path relativo pois o frontend é servido pelo próprio backend
  // Isso funciona tanto localmente quanto via Cloudflare Tunnel/proxy reverso
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
 * Erro de API que preserva o `code` do envelope. A mensagem continua sendo a do servidor (nada
 * muda para quem só a exibe), mas quem precisa DECIDIR pelo motivo — o 409 do POST de avulso,
 * cujos cinco códigos viram cinco toasts diferentes — não precisa parsear texto para isso.
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
  /** Cancela a requisição. A busca da tela de adicionar aborta a anterior a cada tecla. */
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

    return data.data
  } catch (error) {
    console.error('API request failed:', error)
    if (!opts.silent) {
      const isAnilistEndpoint = /\/animes\/\d+\/episodes$/.test(endpoint)
      const message =
        isAnilistEndpoint && (responseStatus === 500 || responseStatus === 0)
          ? 'Falha na comunicação com o AniList'
          : error instanceof Error
            ? error.message
            : 'Erro desconhecido'
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
  /** Livre abaixo de min_free_disk_percent: o daemon parou de adicionar torrents. Calculado no
   *  servidor de proposito — um limiar duplicado no frontend discordaria do daemon. */
  disk_low: boolean
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
  completed_anime_path: string
  check_interval: number
  max_episodes_per_anime: number
  /** Acima deste numero de episodios um anime finalizado nao usa batch. 0 desliga. */
  max_batch_episodes: number
  /** Tetos de tamanho de torrent em GiB. 0 desliga. */
  max_batch_torrent_size_gb: number
  max_episode_torrent_size_gb: number
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
  /** Lancados, nao assistidos e nao baixados — a contagem de "atrasado". */
  episodes_pending: number
  total_episodes: number
  latest_episode_number: number
  last_download_date: string
  cover_image?: string
  is_blacklisted?: boolean
  /** Acompanhado pelo arquivo de avulsos, e não por uma lista do AniList. Origem, não estado. */
  is_standalone?: boolean
}

/** Motivos pelos quais um anime não pode ser adicionado como avulso — vêm prontos do servidor. */
export type BlockReason = '' | 'blacklist' | 'standalone' | 'tracked' | 'downloaded'

export interface AniListSearchResult {
  id: number
  title: string
  format: string
  status: string
  year: number
  /** 0 quando o AniList não sabe o total (comum em anime ainda no ar). */
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

export async function getStatus(): Promise<StatusResponse> {
  return apiRequest<StatusResponse>('GET', '/status')
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
  /** Identidade do episodio nas rotas de acao: (anime, numero). Nao existe mais episode_id. */
  episode_number: number
  airing_at: number
  time_until_airing: number
  is_aired: boolean
  is_watched: boolean
  is_downloaded: boolean
  download_date?: string
  episode_name?: string
  is_manually_managed?: boolean
  is_blocked?: boolean
  episode_hash?: string
}

export interface AnimeDetailResponse {
  // anime_id e o id da MIDIA na AniList: serve de chave interna e tambem monta o link
  // anilist.co/anime/{id}. Nao ha mais um anilist_id separado.
  anime_id: number
  total_episodes: number
  progress: number
  status: string
  cover_image?: string
  episodes: AnimeEpisodeInfo[]
  custom_search_query?: string
}

export interface AnimeSettings {
  custom_search_query: string
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
 * Busca no AniList. `silent` porque a tela busca a cada tecla (com debounce) — um erro de rede
 * transitório não pode virar um toast por letra digitada.
 */
export async function searchAniList(
  q: string,
  includeUnreleased = false,
  signal?: AbortSignal,
): Promise<AniListSearchResult[]> {
  // O parâmetro só viaja quando é true: o backend já assume false, e mandar `=false` em toda
  // busca só engorda a URL do log.
  const unreleased = includeUnreleased ? '&include_unreleased=true' : ''
  return apiRequest<AniListSearchResult[]>(
    'GET',
    `/anilist/search?q=${encodeURIComponent(q)}${unreleased}`,
    null,
    { silent: true, signal },
  )
}

/** Passa a acompanhar um anime avulso e já baixa o que estiver no ar. Devolve quantos entraram. */
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

