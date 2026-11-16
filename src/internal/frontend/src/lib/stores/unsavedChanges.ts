import { derived, writable } from 'svelte/store'

/**
 * "Unsaved changes" is a comparison against what was loaded, never a "something was touched" flag:
 * changing a field and changing it back is NOT dirty. Pages snapshot what they loaded (and again
 * after a successful save) and compare the current state's snapshot with it.
 *
 * The snapshot ignores differences that don't change what would be saved:
 * - key order (a preset or a reset rebuilds objects in another order);
 * - `undefined` vs an absent key vs an empty array (opening a webhook in the form turns a missing
 *   `anime_sources` into `[]`; Go reads both as an empty list). (decisions.md #129)
 */
export function snapshot(value: unknown): string {
  return JSON.stringify(normalize(value)) ?? ''
}

function normalize(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(normalize)
  if (value && typeof value === 'object') {
    const out: Record<string, unknown> = {}
    for (const key of Object.keys(value).sort()) {
      const v = (value as Record<string, unknown>)[key]
      if (v === undefined || (Array.isArray(v) && v.length === 0)) continue
      out[key] = normalize(v)
    }
    return out
  }
  return value
}

/** Which pages hold unsaved changes. Keyed so one page unmounting can't clear another's flag. */
const dirty = writable<Set<string>>(new Set())

export const hasUnsavedChanges = derived(dirty, (s) => s.size > 0)

export function setUnsaved(page: string, isDirty: boolean) {
  dirty.update((s) => {
    if (s.has(page) === isDirty) return s
    const next = new Set(s)
    if (isDirty) next.add(page)
    else next.delete(page)
    return next
  })
}

let current = false
hasUnsavedChanges.subscribe((v) => (current = v))

/** The path part of a hash route: a query-only change (`#/config?group=x`) is not leaving the page. */
const routePath = (hash: string) => hash.replace(/^#/, '').split('?')[0] || '/'

/**
 * Asks before leaving a page with unsaved changes, both ways out:
 * - reload / close tab: `beforeunload` (the browser shows its own text; a custom one is ignored);
 * - in-app route change: svelte-spa-router only listens to `hashchange` in the bubble phase, so a
 *   CAPTURE listener registered before the Router mounts runs first and can stop it with
 *   `stopImmediatePropagation`. The URL has already changed by then, so a cancel puts the old one
 *   back with `replaceState` (which fires no event). `push`, `replace`, links and Back all reach
 *   the router as `hashchange`.
 *
 * Must be installed before `<Router>` mounts (App.svelte's script). Returns the uninstaller.
 * (decisions.md #129)
 */
export function installUnsavedGuard(confirmLeave: () => boolean): () => void {
  let lastUrl = window.location.href

  const onHashChange = (e: Event) => {
    const prevUrl = lastUrl
    const prevPath = routePath(new URL(prevUrl).hash)
    if (current && routePath(window.location.hash) !== prevPath && !confirmLeave()) {
      e.stopImmediatePropagation()
      history.replaceState(history.state, '', prevUrl)
      return
    }
    lastUrl = window.location.href
  }

  const onBeforeUnload = (e: BeforeUnloadEvent) => {
    if (!current) return
    e.preventDefault()
    e.returnValue = ''
  }

  window.addEventListener('hashchange', onHashChange, true)
  window.addEventListener('beforeunload', onBeforeUnload)
  return () => {
    window.removeEventListener('hashchange', onHashChange, true)
    window.removeEventListener('beforeunload', onBeforeUnload)
  }
}
