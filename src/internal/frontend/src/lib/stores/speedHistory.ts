import { writable } from 'svelte/store'

/**
 * `speedHistory` (spec §8, Fase 2) — the last 20 aggregated download-speed samples, feeding
 * `ProgressBar`/`Sparkline` on the Status screen's speed card.
 *
 * This store does NOT poll on its own (unlike `activeTorrents.ts`): torrent data is already
 * polled by whichever screen needs it (Status.svelte at 5s, per the existing polling table).
 * A second independent `getTorrents()` poll here would just be redundant traffic. Instead the
 * screen calls `speedHistory.push(sample)` itself, once per successful poll tick, with the
 * aggregated speed for that tick (e.g. `totalSpeeds(torrents).download`).
 *
 * CRITICAL RULE (spec §8): when a poll fails, the caller must simply not call `push()` for
 * that tick. There is no `fail()`/`freeze()` method on purpose — freezing is the *absence* of
 * a call, not a state transition this store performs. Extrapolating a value for the failed
 * tick (repeating the last sample, interpolating, etc.) would fake a continuous stream the
 * daemon never actually reported, which is exactly what the design handoff prohibits.
 */

const MAX_SAMPLES = 20

function createSpeedHistory() {
  const { subscribe, update, set } = writable<number[]>([])

  return {
    subscribe,
    /** Appends one fresh aggregated-speed sample from a successful poll tick. */
    push(sample: number) {
      update((history) => {
        const next = [...history, sample]
        return next.length > MAX_SAMPLES ? next.slice(next.length - MAX_SAMPLES) : next
      })
    },
    /** Clears the history (e.g. on navigating away from the screen that owns the poll). */
    reset() {
      set([])
    },
  }
}

export const speedHistory = createSpeedHistory()
