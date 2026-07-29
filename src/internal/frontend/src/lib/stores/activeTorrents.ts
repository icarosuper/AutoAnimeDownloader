import { writable } from 'svelte/store'
import { getTorrents } from '../api/client.js'

/**
 * Count of "active" torrents, polled for the NavRail/NavTabBar Downloads badge (Fase 1 of the
 * UI redesign, spec §5). "Active" = downloading | downloading_metadata | verifying |
 * allocating — not seeding/stopped/stopping (see lib/utils/torrentStatus.ts for the full slug
 * list). This is an ambient indicator, not a precision counter: 10s interval, paused while the
 * tab is hidden. `Downloads.svelte` has its own 2s poll for the actual torrent list — this
 * store is intentionally separate; consolidating the two is out of scope for this phase.
 */

const ACTIVE_STATUSES = new Set(['downloading', 'downloading_metadata', 'verifying', 'allocating'])
const POLL_INTERVAL_MS = 10_000

function countActive(torrents: { status: string }[]): number {
  return torrents.filter((t) => ACTIVE_STATUSES.has(t.status)).length
}

export const activeTorrentCount = writable<number>(0, (set) => {
  let intervalId: ReturnType<typeof setInterval> | null = null

  async function poll() {
    try {
      const torrents = await getTorrents()
      set(countActive(torrents))
    } catch {
      // Ambient indicator only — a failed poll just clears the badge, no toast (getTorrents()
      // already calls the API with `silent: true`, so this catch is purely to reset the count).
      set(0)
    }
  }

  function start() {
    if (intervalId !== null) return
    poll()
    intervalId = setInterval(poll, POLL_INTERVAL_MS)
  }

  function stop() {
    if (intervalId !== null) {
      clearInterval(intervalId)
      intervalId = null
    }
  }

  function handleVisibilityChange() {
    if (document.visibilityState === 'hidden') {
      stop()
    } else {
      start()
    }
  }

  const isBrowser = typeof document !== 'undefined'

  if (isBrowser) {
    if (document.visibilityState !== 'hidden') start()
    document.addEventListener('visibilitychange', handleVisibilityChange)
  } else {
    start()
  }

  return () => {
    stop()
    if (isBrowser) {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  }
})
