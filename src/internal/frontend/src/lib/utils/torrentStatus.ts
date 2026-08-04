import * as m from '../i18n/messages.js'

// Slugs emitidos pelo backend (src/internal/torrents/status.go, statusSlug()) mais 'queued',
// que quem escreve e a fila de downloads (queue.markQueued), nao o enum da rain. Mesma lista de
// src/lib/utils/torrentFilters.ts (KNOWN_STATUSES) — duplicada aqui só como array de valores
// porque aquele arquivo não a exporta; não invente slugs novos além destes.
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

/** Rótulo traduzido de um status de torrent. Usado por Downloads.svelte e DownloadsToolbar.svelte. */
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

/** Classe de badge (daisyUI) correspondente ao status. */
export function statusClass(status: string): string {
  switch (status) {
    case 'seeding':
      return 'badge-success'
    case 'downloading':
      return 'badge-info'
    case 'stopped':
    case 'queued':
      return 'badge-ghost'
    case 'stopping':
      return 'badge-warning'
    default:
      return 'badge-neutral'
  }
}
