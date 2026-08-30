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
     * Reconciles a torrent poll tick, mexendo SÓ nos hashes que vieram nesta lista: a 0 peers
     * o hash mantém o carimbo antigo (ou ganha `now` se é novo), com peers ele é apagado.
     * Hash ausente da lista fica como está — `AnimeDetail` sincroniza só os torrents de um
     * anime, e podar por ausência zerava o relógio de stall de todos os outros (o chip do
     * Status voltava ao zero a cada visita a um detalhe).
     *
     * O custo é uma entrada órfã quando um torrent some de vez; ela só volta a pesar se o
     * mesmo hash for readicionado ainda sem peers, e some no reload.
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
