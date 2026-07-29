import type { AnimeEpisodeInfo, TorrentInfo } from '../api/client.js'

/**
 * `episodeActions` (spec §7, Fase 2) — the action model for one episode row, as DATA. Desktop
 * (icon buttons) and mobile (bottom sheet / ActionMenu) both render from the same
 * `{ principal, menu }` shape instead of each screen re-deriving "which of the five endpoints
 * apply here" on its own, which is how the pre-redesign AnimeDetail.svelte ended up with the
 * same `is_aired && !is_downloaded` / `is_downloaded` / `is_manually_managed || is_blocked`
 * conditions written out twice (desktop table + mobile card list).
 *
 * `labelKey` is an identifier, not a resolved string or a Paraglide function reference — same
 * i18n boundary as `animeState.ts`. The five ids below (`download`, `redownload`, `delete`,
 * `release`, `replace`) map 1:1 to the five actions in `lib/api/client.ts`
 * (downloadEpisode / redownloadEpisode / deleteEpisode / releaseEpisode /
 * replaceEpisodeWithMagnet); `labelKey` mirrors `id` today, but is a separate field so a
 * future context (e.g. bulk vs. per-row) can pick a different label for the same action
 * without changing its identity.
 *
 * `destructive: true` marks `delete` and `redownload`: both replace a file on disk and both
 * already run through a confirmation dialog in the current screen (`ConfirmDialog` /
 * `TorrentDeleteDialog`) — see spec §7. Screens must keep gating those two behind a confirm
 * step; this module only flags that requirement, it doesn't enforce it.
 */

export type EpisodeActionId = 'download' | 'redownload' | 'delete' | 'release' | 'replace'

export interface Action {
  id: EpisodeActionId
  labelKey: string
  variant: 'solid' | 'ghost' | 'warn'
  destructive?: true
}

export interface EpisodeActionSet {
  /** The single prominent action for this row, if any (rendered as a `Button`). */
  principal?: Action
  /** Secondary actions, rendered inside `ActionMenu`. Empty when there is nothing to offer. */
  menu: Action[]
}

const DOWNLOAD: Action = { id: 'download', labelKey: 'download', variant: 'solid' }
const REDOWNLOAD: Action = { id: 'redownload', labelKey: 'redownload', variant: 'ghost', destructive: true }
const DELETE: Action = { id: 'delete', labelKey: 'delete', variant: 'warn', destructive: true }
const RELEASE: Action = { id: 'release', labelKey: 'release', variant: 'ghost' }
const REPLACE: Action = { id: 'replace', labelKey: 'replace', variant: 'ghost' }

/**
 * Classifies one episode (+ its live torrent, if any — joined by the caller via
 * `torrentsByEpisode.ts`, same as the pre-redesign screen did) into the five states of
 * spec §7's table. The cascade order below IS that table's row order: blocked/manually-managed
 * is checked before "downloaded" and before "has an active torrent" because a user who
 * manually took over an episode should keep seeing "Soltar" even while a torrent they added by
 * hand happens to be mid-download — the whole point of that state is that automation defers to
 * them until they release it back.
 */
export function episodeActions(ep: AnimeEpisodeInfo, torrent: TorrentInfo | undefined): EpisodeActionSet {
  if (ep.is_manually_managed || ep.is_blocked) {
    // Row: "Bloqueado ou gerenciado manualmente"
    return { principal: RELEASE, menu: [DOWNLOAD, DELETE] }
  }

  if (torrent && !torrent.completed && torrent.status === 'downloading') {
    // Row: "Torrent ativo (baixando)"
    return { principal: undefined, menu: [REPLACE, DELETE] }
  }

  if (ep.is_downloaded) {
    // Row: "Baixado"
    return { principal: REDOWNLOAD, menu: [REPLACE, DELETE] }
  }

  if (ep.is_aired) {
    // Row: "No ar e não baixado"
    return { principal: DOWNLOAD, menu: [REPLACE] }
  }

  // Row: "Não lançou"
  return { principal: undefined, menu: [] }
}
