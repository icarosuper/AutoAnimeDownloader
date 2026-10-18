import type { BackendState } from '../stores/backendHealth.js'
import type { AnilistHealth } from '../api/client.js'

/**
 * The banner is UNIQUE and has precedence — never a stack. With the daemon down, the AniList
 * state on screen is the last successful read, i.e. stale information; showing it next to
 * "daemon not responding" is contradictory noise. See decisions.md #66.
 *
 * Order goes from most upstream to most downstream: nothing below the backend is observable
 * without the backend.
 */
export type BannerKind =
  | 'backend_unreachable'
  | 'backend_error'
  | 'anilist_outage'
  | 'anilist_rate_limited'
  | 'anilist_app_bug'

export interface SystemBanner {
  kind: BannerKind
  /** Raw AniList message, when present. A 403 for a blocked IP explains the reason in
   *  writing, and it's the only information the frontend cannot reconstruct on its own. */
  detail?: string
  /** Instant at which the rate limit expires, for the countdown. */
  retryAt?: Date
  /** Date of the local cache being served in place of AniList. Only present in degraded
   *  states: with AniList up the snapshot exists but it is not what is on screen. */
  cachedAt?: Date
  /** Report button ONLY for our own bugs: backend 5xx, and AniList 400 (the schema changed
   *  and no amount of waiting will fix it). Reporting a rate limit or outage produces an
   *  issue that closes with "it's AniList", and trains the user to ignore the button
   *  exactly when it matters. */
  reportable: boolean
}

export function pickBanner(
  backend: BackendState,
  anilist: AnilistHealth | null | undefined,
): SystemBanner | null {
  if (backend === 'unreachable') return { kind: 'backend_unreachable', reportable: false }
  if (backend === 'server_error') return { kind: 'backend_error', reportable: true }

  const cachedAt = anilist?.cache_saved_at ? new Date(anilist.cache_saved_at) : undefined

  switch (anilist?.state) {
    case 'rate_limited':
      return {
        kind: 'anilist_rate_limited',
        retryAt: anilist.retry_at ? new Date(anilist.retry_at) : undefined,
        reportable: false,
      }
    case 'outage':
      return { kind: 'anilist_outage', detail: anilist.message, cachedAt, reportable: false }
    case 'app_bug':
      return { kind: 'anilist_app_bug', detail: anilist.message, cachedAt, reportable: true }
    default:
      return null
  }
}

/** Seconds remaining until `retryAt`, never negative. Outside the component so it is testable. */
export function secondsUntil(retryAt: Date | undefined, now: number): number {
  if (!retryAt) return 0
  return Math.max(0, Math.ceil((retryAt.getTime() - now) / 1000))
}
