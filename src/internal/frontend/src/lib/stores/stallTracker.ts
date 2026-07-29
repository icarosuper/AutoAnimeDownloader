import { writable } from 'svelte/store'

/**
 * `stallTracker` (spec §8, Fase 2) — per torrent hash, the timestamp since which
 * `peers_total === 0`. In-memory only; resets on page reload, which the spec explicitly
 * accepts (a fresh page load re-observes stalls from scratch within the next poll or two —
 * cheap enough that persisting this across reloads isn't worth the complexity).
 *
 * This store is bookkeeping ONLY: it does not decide what counts as "stalled long enough to
 * show a chip" — that 10-minute threshold is a chip-rendering business rule and lives in
 * `lib/domain/animeState.ts` (`deriveAnimeChip`), which stays pure by taking this store's
 * snapshot as a plain `Map` argument instead of importing a Svelte store. The screen wires
 * them together: call `sync()` once per torrent poll tick, then pass `get(stallTracker)`
 * straight into `deriveAnimeChip`.
 *
 * Like `speedHistory.ts`, this does not poll on its own — torrent data is already polled by
 * whichever screen needs it.
 */

function createStallTracker() {
  const { subscribe, update, set } = writable<Map<string, number>>(new Map())

  return {
    subscribe,
    /**
     * Reconciles the tracker against one torrent poll tick. A hash at peers_total===0 keeps
     * its original timestamp if already tracked, or gets stamped with `now` if newly seen.
     * Any hash NOT in this tick's list at peers_total===0 (peers came back, or the torrent was
     * removed/completed and dropped out of the list entirely) is cleared.
     */
    sync(torrents: { hash: string; peers_total: number }[], now: number) {
      update((prev) => {
        const next = new Map<string, number>()
        for (const t of torrents) {
          if (t.peers_total === 0) {
            next.set(t.hash, prev.get(t.hash) ?? now)
          }
        }
        return next
      })
    },
    reset() {
      set(new Map())
    },
  }
}

export const stallTracker = createStallTracker()
