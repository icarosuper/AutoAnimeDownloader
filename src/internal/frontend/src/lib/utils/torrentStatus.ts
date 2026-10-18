import * as m from '../i18n/messages.js'

// Slugs emitted by the backend (src/internal/torrents/status.go, statusSlug()) plus 'queued',
// which the download queue writes itself (queue.markQueued), not the rain enum. Same list as
// src/lib/utils/torrentFilters.ts (KNOWN_STATUSES) — duplicated here only as an array of
// values because that file doesn't export it; don't invent slugs beyond these.
export const STATUS_SLUGS = [
  'stopped',
  'queued',
  'downloading_metadata',
  'allocating',
  'verifying',
  'downloading',
  'seeding',
  'stopping',
] as const

/** Translated label for a torrent status. Used by Downloads.svelte and DownloadsToolbar.svelte. */
export function statusLabel(status: string): string {
  switch (status) {
    case 'stopped':
      return m.downloads_status_stopped()
    case 'queued':
      return m.downloads_status_queued()
    case 'downloading_metadata':
      return m.downloads_status_downloading_metadata()
    case 'allocating':
      return m.downloads_status_allocating()
    case 'verifying':
      return m.downloads_status_verifying()
    case 'downloading':
      return m.downloads_status_downloading()
    case 'seeding':
      return m.downloads_status_seeding()
    case 'stopping':
      return m.downloads_status_stopping()
    default:
      return m.downloads_status_unknown()
  }
}

