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
     * Reconciles a torrent poll tick, touching ONLY the hashes that came in this list: at
     * 0 peers the hash keeps its old stamp (or gets `now` if it's new); with peers it is
     * deleted. A hash absent from the list stays as-is — `AnimeDetail` syncs only one
     * anime's torrents, and pruning by absence zeroed every other stall clock (the Status
     * chip reset to zero on every visit to a detail).
     *
     * The cost is an orphaned entry when a torrent disappears for good; it only matters
     * again if the same hash is re-added still with no peers, and it goes away on reload.
     */
    sync(torrents: { hash: string; peers_total: number }[], now: number) {
      update((prev) => {
        const next = new Map(prev)
        for (const t of torrents) {
          if (t.peers_total === 0) {
            next.set(t.hash, prev.get(t.hash) ?? now)
          } else {
            next.delete(t.hash)
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
