/**
 * `logSourceFromCaller` (spec §7, Fase 2) — maps zerolog's `caller` field (populated by
 * `.Caller()` in `src/internal/logger/logger.go`, formatted as `<basename>:<line>` via
 * `CallerMarshalFunc`, e.g. `"endpoint_torrents.go:229"`) to the origin shown in the Logs
 * screen: scheduler / anilist / rss / torrent / api.
 *
 * DELIBERATE APPROXIMATION — do not "fix" this later thinking it's a bug. The backend has no
 * structured `component` field (that's backlog, tracked separately, and explicitly out of
 * scope for this phase — see the Fase 2 brief). Lacking that, this module guesses the origin
 * from which Go source FILE logged the line, using the file's package as a stand-in. That
 * mapping is necessarily hand-maintained: a new file added to one of these packages needs an
 * entry here, and there is no way for a frontend-only test to catch that it's missing.
 *
 * "rss" = the `nyaa` package (Nyaa.si is scraped as an RSS-shaped source, per the project's
 * README/architecture docs). "scheduler" = the `daemon` package (loop/check orchestration).
 * Files outside all five mapped packages (`files`, `notifications`, `tray`, or any future
 * package) fall back to "other" — the spec only names five origins, so "other" is this
 * module's own addition to keep the function total.
 */

export type LogSource = 'scheduler' | 'anilist' | 'rss' | 'torrent' | 'api' | 'other'

// src/internal/<package>/*.go basenames, snapshotted at Fase 2 time. Keep in sync by hand if a
// file is added/renamed/moved between these packages — see module doc above.
const FILE_TO_SOURCE: Record<string, LogSource> = {
  // src/internal/daemon
  'debug.go': 'scheduler',
  'episodes.go': 'scheduler',
  'helpers.go': 'scheduler',
  'jobs.go': 'scheduler',
  'loop.go': 'scheduler',
  'manual_download.go': 'scheduler',
  'migration.go': 'scheduler',
  'search.go': 'scheduler',
  'state.go': 'scheduler',
  'verification.go': 'scheduler',

  // src/internal/anilist
  'anilist.go': 'anilist',

  // src/internal/nyaa
  'nyaa.go': 'rss',
  'nyaa_match.go': 'rss',
  'nyaa_regex.go': 'rss',
  'priorities.go': 'rss',

  // src/internal/torrents
  'backend.go': 'torrent',
  'fakebackend.go': 'torrent',
  'rootmarker.go': 'torrent',
  'session.go': 'torrent',
  'sessionmanager.go': 'torrent',
  'status.go': 'torrent',

  // src/internal/api
  'client.go': 'api',
  'endpoint_anime_episodes.go': 'api',
  'endpoint_anime_settings.go': 'api',
  'endpoint_animes.go': 'api',
  'endpoint_check.go': 'api',
  'endpoint_config.go': 'api',
  'endpoint_daemon_start.go': 'api',
  'endpoint_daemon_stop.go': 'api',
  'endpoint_episode_actions.go': 'api',
  'endpoint_logs.go': 'api',
  'endpoint_notifications.go': 'api',
  'endpoint_priorities.go': 'api',
  'endpoint_status.go': 'api',
  'endpoint_torrents.go': 'api',
  'middleware.go': 'api',
  'responses.go': 'api',
  'server.go': 'api',
  'websocket.go': 'api',
}

/** Any `endpoint_*.go` file is API even if a new one is added without updating the table above. */
function isApiEndpointFile(basename: string): boolean {
  return basename.startsWith('endpoint_')
}

/** `caller` is `"<basename>:<line>"`, occasionally with a leading path — basename is always last. */
function basenameOf(caller: string): string {
  const withoutLine = caller.split(':')[0]
  const segments = withoutLine.split('/')
  return segments[segments.length - 1]
}

export function logSourceFromCaller(caller: string | undefined): LogSource {
  if (!caller) return 'other'
  const basename = basenameOf(caller)
  if (!basename) return 'other'
  return FILE_TO_SOURCE[basename] ?? (isApiEndpointFile(basename) ? 'api' : 'other')
}
