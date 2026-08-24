import type { AnimeInfo } from '../api/client.js'

export type SortKey = 'name' | 'episodes_watched' | 'last_download_date'
export type SortDir = 'asc' | 'desc'

/**
 * NFKD + remocao de diacriticos: faz "Kimi no Na wa" casar com "kimi no na wa" e katakana de
 * largura completa casar com o normal. Kanji passa intacto.
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
    // Busca por QUALQUER titulo do anime — o usuario pode conhecer so o romaji, o kanji ou um
    // sinonimo, enquanto o card mostra o ingles.
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

// O limiar de "disco baixo" vive no servidor (StatusResponse.disk_low, calculado com
// min_free_disk_percent): um cálculo duplicado aqui acabaria discordando do que o daemon está
// fazendo. Não reintroduzir o ratio para economizar um campo na resposta.
