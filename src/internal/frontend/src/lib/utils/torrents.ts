import type { TorrentInfo } from '../api/client.js'

const IDLE = '—'

/** Formats a byte-per-second rate. Zero renders as a dash: an idle torrent has no speed. */
export function formatSpeed(bytesPerSecond: number): string {
  if (!bytesPerSecond || bytesPerSecond <= 0) return IDLE
  if (bytesPerSecond < 1024) return `${bytesPerSecond} B/s`
  const units = ['KB/s', 'MB/s', 'GB/s']
  let value = bytesPerSecond / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(1)} ${units[unit]}`
}

/**
 * Formats a remaining time in seconds. null means infinite/unknown — rain only fills ETA
 * while the torrent is downloading at a non-zero speed, so null is the common case.
 */
export function formatEta(seconds: number | null): string {
  if (seconds === null || seconds === undefined || seconds < 0) return IDLE
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) {
    const minutes = Math.floor(seconds / 60)
    return `${minutes}m ${seconds % 60}s`
  }
  if (seconds < 86400) {
    const hours = Math.floor(seconds / 3600)
    return `${hours}h ${Math.floor((seconds % 3600) / 60)}m`
  }
  const days = Math.floor(seconds / 86400)
  return `${days}d ${Math.floor((seconds % 86400) / 3600)}h`
}

/** Formats a 0..1 progress fraction as a whole percentage. */
export function formatPercent(progress: number): string {
  return `${Math.round((progress || 0) * 100)}%`
}

/** Sums per-torrent speeds into the session-wide rates shown on the status screen. */
export function totalSpeeds(torrents: TorrentInfo[]): { download: number; upload: number } {
  return torrents.reduce(
    (acc, t) => ({ download: acc.download + t.download_speed, upload: acc.upload + t.upload_speed }),
    { download: 0, upload: 0 },
  )
}
