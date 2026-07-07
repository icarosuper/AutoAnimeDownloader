import type { AnimeInfo } from '../api/client.js'

export type SortKey = 'name' | 'episodes_watched' | 'last_download_date'
export type SortDir = 'asc' | 'desc'

export function filterAnimes(
  animes: AnimeInfo[],
  search: string,
  filterUnwatched: boolean,
): AnimeInfo[] {
  return animes.filter(a => {
    if (!a.name.toLowerCase().includes(search.toLowerCase())) return false
    if (filterUnwatched && a.episodes_watched >= a.episodes_released && a.episodes_released > 0) return false
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
// ponytail: 10% hardcoded — promote to a config field if users ever ask to tune it
const LOW_DISK_SPACE_RATIO = 0.1

export function formatBytes(bytes: number): string {
  return `${(bytes / GIB).toFixed(1)} GB`
}

export function isDiskSpaceLow(free: number, total: number): boolean {
  if (total <= 0) return false
  return free / total < LOW_DISK_SPACE_RATIO
}
