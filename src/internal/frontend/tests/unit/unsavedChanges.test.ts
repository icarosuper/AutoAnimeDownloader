import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { get } from 'svelte/store'
import { snapshot, setUnsaved, hasUnsavedChanges, installUnsavedGuard } from '../../src/lib/stores/unsavedChanges.js'

describe('snapshot', () => {
  it('ignores key order', () => {
    expect(snapshot({ a: 1, b: { c: 2, d: 3 } })).toBe(snapshot({ b: { d: 3, c: 2 }, a: 1 }))
  })

  it('treats undefined, absent and empty-array keys as the same', () => {
    expect(snapshot({ a: 1, list: [], x: undefined })).toBe(snapshot({ a: 1 }))
  })

  it('keeps array order and real differences', () => {
    expect(snapshot({ l: ['a', 'b'] })).not.toBe(snapshot({ l: ['b', 'a'] }))
    expect(snapshot({ n: 5 })).not.toBe(snapshot({ n: 6 }))
    expect(snapshot({ l: ['a'] })).not.toBe(snapshot({ l: [] }))
  })
})

describe('setUnsaved', () => {
  afterEach(() => { setUnsaved('a', false); setUnsaved('b', false) })

  it('one page clearing its flag does not clear another page', () => {
    setUnsaved('a', true)
    setUnsaved('b', true)
    setUnsaved('a', false)
    expect(get(hasUnsavedChanges)).toBe(true)
    setUnsaved('b', false)
    expect(get(hasUnsavedChanges)).toBe(false)
  })
})

describe('installUnsavedGuard', () => {
  let uninstall: () => void
  let confirmLeave: ReturnType<typeof vi.fn>
  // Stands in for svelte-spa-router: a bubble-phase hashchange listener registered after the guard.
  let routerSaw: string[]
  const router = () => routerSaw.push(window.location.hash)

  beforeEach(() => {
    history.replaceState(null, '', '#/config')
    confirmLeave = vi.fn(() => false)
    uninstall = installUnsavedGuard(confirmLeave as () => boolean)
    routerSaw = []
    window.addEventListener('hashchange', router)
  })

  afterEach(() => {
    uninstall()
    window.removeEventListener('hashchange', router)
    setUnsaved('config', false)
  })

  const navigate = (hash: string) => {
    history.replaceState(null, '', hash)
    window.dispatchEvent(new HashChangeEvent('hashchange'))
  }

  it('lets the route change through with nothing unsaved, without asking', () => {
    navigate('#/logs')
    expect(confirmLeave).not.toHaveBeenCalled()
    expect(routerSaw).toEqual(['#/logs'])
  })

  it('cancel keeps the page: the router never sees the change and the URL goes back', () => {
    setUnsaved('config', true)
    navigate('#/logs')
    expect(confirmLeave).toHaveBeenCalledOnce()
    expect(routerSaw).toEqual([])
    expect(window.location.hash).toBe('#/config')
  })

  it('confirm lets the route change through', () => {
    setUnsaved('config', true)
    confirmLeave.mockReturnValue(true)
    navigate('#/logs')
    expect(routerSaw).toEqual(['#/logs'])
  })

  it('a query-only change on the same page is not leaving it', () => {
    setUnsaved('config', true)
    navigate('#/config?group=search')
    expect(confirmLeave).not.toHaveBeenCalled()
    expect(routerSaw).toEqual(['#/config?group=search'])
  })

  it('blocks unload only with unsaved changes', () => {
    const clean = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(clean)
    expect(clean.defaultPrevented).toBe(false)

    setUnsaved('config', true)
    const dirty = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(dirty)
    expect(dirty.defaultPrevented).toBe(true)
  })
})
