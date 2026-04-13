import { toasts } from '../stores/toasts.js'

// Usa o host atual da página quando não há variável de ambiente definida
// Isso permite que o frontend funcione tanto localmente quanto quando acessado remotamente
const getApiBaseUrl = (): string => {
  if (import.meta.env.VITE_API_BASE_URL) {
    return import.meta.env.VITE_API_BASE_URL
  }

  // Check if we're in browser environment before accessing window
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

async function apiRequest<T>(method: string, endpoint: string, body: unknown = null): Promise<T> {
  const url = `${API_BASE_URL}${endpoint}`
  const options: RequestInit = {
    method,
    headers: {
      'Content-Type': 'application/json',
    },
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
      throw new Error(data.error?.message || `HTTP error! status: ${response.status}`)
    }

    return data.data
  } catch (error) {
    console.error('API request failed:', error)
    const isAnilistEndpoint = /\/animes\/\d+\/episodes$/.test(endpoint)
    const message =
      isAnilistEndpoint && (responseStatus === 500 || responseStatus === 0)
        ? 'Falha na comunicação com o AniList'
        : error instanceof Error
          ? error.message
          : 'Erro desconhecido'
    toasts.add(message)
    throw error
  }
}

export interface StatusResponse {
  status: string
  last_check: string
  has_error: boolean
  version: string
}

export interface Config {
  anilist_username: string
  save_path: string
  completed_anime_path: string
  check_interval: number
  qbittorrent_url: string
  max_episodes_per_anime: number
  episode_retry_limit: number
  delete_watched_episodes: boolean
  watched_episodes_to_keep: number
  excluded_list: string
  rename_files_for_jellyfin: boolean
}

export interface AnimeInfo {
  anime_id: number
  name: string
  episodes_count: number
  total_episodes: number
  latest_episode_id: number
  last_download_date: string
}

export interface LogsResponse {
  lines: string[]
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

export async function getAnimes(): Promise<AnimeInfo[]> {
  return apiRequest<AnimeInfo[]>('GET', '/animes')
}

export interface AnimeEpisodeInfo {
  episode_id: number
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
}

export interface AnimeDetailResponse {
  anime_id: number
  total_episodes: number
  progress: number
  status: string
  episodes: AnimeEpisodeInfo[]
}

export async function getAnimeDetail(animeId: number): Promise<AnimeDetailResponse> {
  return apiRequest<AnimeDetailResponse>('GET', `/animes/${animeId}/episodes`)
}

export async function downloadEpisode(animeId: number, episodeId: number): Promise<void> {
  return apiRequest<void>('POST', `/animes/${animeId}/episodes/${episodeId}/download`)
}

export async function deleteEpisode(animeId: number, episodeId: number): Promise<void> {
  return apiRequest<void>('DELETE', `/animes/${animeId}/episodes/${episodeId}`)
}

export async function releaseEpisode(animeId: number, episodeId: number): Promise<void> {
  return apiRequest<void>('POST', `/animes/${animeId}/episodes/${episodeId}/release`)
}

export async function redownloadEpisode(animeId: number, episodeId: number): Promise<void> {
  return apiRequest<void>('POST', `/animes/${animeId}/episodes/${episodeId}/redownload`)
}

export async function replaceEpisodeWithMagnet(animeId: number, episodeId: number, magnet: string): Promise<void> {
  return apiRequest<void>('POST', `/animes/${animeId}/episodes/${episodeId}/replace`, { magnet })
}

export async function replaceAnimeWithMagnet(animeId: number, magnet: string): Promise<void> {
  return apiRequest<void>('POST', `/animes/${animeId}/replace`, { magnet })
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

