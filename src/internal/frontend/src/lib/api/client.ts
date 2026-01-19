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

  // Sempre usa o hostname atual e inclui a porta 8091 explicitamente
  // Isso garante que funcione tanto localmente quanto remotamente
  // O daemon sempre roda na porta 8091, então sempre usamos essa porta
  const protocol = window.location.protocol
  const hostname = window.location.hostname

  return `${protocol}//${hostname}:8091/api/v1`
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

  try {
    const response = await fetch(url, options)
    const data: ApiResponse<T> = await response.json()

    if (!response.ok) {
      throw new Error(data.error?.message || `HTTP error! status: ${response.status}`)
    }

    return data.data
  } catch (error) {
    console.error('API request failed:', error)
    throw error
  }
}

export interface StatusResponse {
  status: string
  last_check: string
  has_error: boolean
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
  excluded_list: string
}

export interface AnimeInfo {
  name: string
  episodes_count: number
  latest_episode_id: number
}

export interface Episode {
  episode_id: number
  episode_name: string
  episode_hash: string
  download_date: string
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

export async function getEpisodes(): Promise<Episode[]> {
  return apiRequest<Episode[]>('GET', '/episodes')
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

