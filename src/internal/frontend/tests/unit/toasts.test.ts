import { describe, it, expect, vi, afterEach } from 'vitest'
import { get } from 'svelte/store'
import { toasts } from '../../src/lib/stores/toasts.js'

afterEach(() => {
  vi.runAllTimers()
  vi.useRealTimers()
})

describe('toasts store', () => {
  it('add appends toast with correct message and type', () => {
    vi.useFakeTimers()
    const before = get(toasts).length
    toasts.add('hello', 'success')
    const all = get(toasts)
    expect(all.length).toBe(before + 1)
    const added = all.at(-1)!
    expect(added.message).toBe('hello')
    expect(added.type).toBe('success')
  })

  it('toast auto-removes after duration', () => {
    vi.useFakeTimers()
    toasts.add('bye', 'error', 50)
    const count = get(toasts).length
    vi.advanceTimersByTime(50)
    expect(get(toasts).length).toBe(count - 1)
  })

  it('remove() deletes specific toast by id', () => {
    vi.useFakeTimers()
    toasts.add('specific', 'info', 9999)
    const id = get(toasts).at(-1)!.id
    toasts.remove(id)
    expect(get(toasts).find(t => t.id === id)).toBeUndefined()
  })
})
