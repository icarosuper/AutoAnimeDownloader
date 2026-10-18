import { writable, derived } from 'svelte/store'

/**
 * Health of OUR backend, derived only from the responses the app already makes. There is no
 * health-check endpoint: a daemon that returns 200 on `/status` is healthy by definition, and
 * one that doesn't respond wouldn't respond to a new endpoint either.
 *
 * `unreachable` = the request never arrived (daemon down, network out). `server_error` = it
 * arrived and came back 5xx, which is our bug. 4xx does NOT count: 404, 405 and the 409 from
 * the standalone POST are legitimate responses from a working backend, and treating them as
 * degradation would put a banner on screen every time the user tried to add a duplicate anime.
 */
export type BackendState = 'ok' | 'unreachable' | 'server_error'

/**
 * CONSECUTIVE failures required before raising the alarm. An isolated failure on a 5s poll is
 * network noise; the banner exists for state that persists (see decisions.md #66). It's also
 * what lets us live with `silent: true` without reintroducing the toast spam that option exists
 * to prevent.
 */
const FAILURES_BEFORE_ALARM = 2

/**
 * Counters per endpoint, not a global counter: a broken `/last-check` while `/status` still
 * answers is our bug and has to show. With a single counter, one endpoint's success would
 * erase the other's failure.
 */
const failures = new Map<string, { count: number; state: Exclude<BackendState, 'ok'> }>()

const store = writable<BackendState>('ok')

function recompute() {
  let next: BackendState = 'ok'
  for (const f of failures.values()) {
    if (f.count < FAILURES_BEFORE_ALARM) continue
    // Unreachable beats 5xx: with the daemon down, the 5xx left in the counter is stale
    // information from the last time it was still answering.
    if (f.state === 'unreachable') {
      next = 'unreachable'
      break
    }
    next = 'server_error'
  }
  store.set(next)
}

export const backendHealth = {
  subscribe: store.subscribe,

  /** A useful response zeros the endpoint. The banner disappears on the first success, never on a timer. */
  recordSuccess(endpoint: string) {
    if (failures.delete(endpoint)) recompute()
  },

  /** `status` 0 means the fetch never even received a response. */
  recordFailure(endpoint: string, status: number) {
    if (status !== 0 && status < 500) {
      // Legitimate response from a healthy backend: not degradation, and must not keep a
      // previous alarm for this endpoint lit.
      this.recordSuccess(endpoint)
      return
    }
    const state = status === 0 ? 'unreachable' : 'server_error'
    const prev = failures.get(endpoint)
    failures.set(endpoint, { count: prev && prev.state === state ? prev.count + 1 : 1, state })
    recompute()
  },

  /** Tests only: reset the module state between cases. */
  reset() {
    failures.clear()
    store.set('ok')
  },
}

/** Convenience for render conditions. */
export const backendDegraded = derived(store, ($s) => $s !== 'ok')
