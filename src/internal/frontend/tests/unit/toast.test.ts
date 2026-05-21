import { describe, it, expect, vi, afterEach } from 'vitest'
import { get } from 'svelte/store'
import { toast, toasts } from '../../src/lib/stores/toast.js'

afterEach(() => {
  vi.runAllTimers()
  vi.useRealTimers()
})

describe('toast helpers', () => {
  it('toast.success adds a success toast', () => {
    vi.useFakeTimers()
    const before = get(toasts).length
    toast.success('It worked')
    const all = get(toasts)
    expect(all.length).toBe(before + 1)
    expect(all.at(-1)!.type).toBe('success')
    expect(all.at(-1)!.message).toBe('It worked')
  })

  it('toast.error adds an error toast', () => {
    vi.useFakeTimers()
    toast.error('Something failed')
    const last = get(toasts).at(-1)!
    expect(last.type).toBe('error')
    expect(last.message).toBe('Something failed')
  })

  it('toast.warning adds a warning toast', () => {
    vi.useFakeTimers()
    toast.warning('Watch out')
    expect(get(toasts).at(-1)!.type).toBe('warning')
  })

  it('toast.info adds an info toast', () => {
    vi.useFakeTimers()
    toast.info('FYI')
    expect(get(toasts).at(-1)!.type).toBe('info')
  })

  it('toast auto-removes after default duration', () => {
    vi.useFakeTimers()
    toast.success('bye')
    const count = get(toasts).length
    vi.advanceTimersByTime(4000)
    expect(get(toasts).length).toBe(count - 1)
  })

  it('toast respects custom duration', () => {
    vi.useFakeTimers()
    toast.success('short', 100)
    const count = get(toasts).length
    vi.advanceTimersByTime(99)
    expect(get(toasts).length).toBe(count)
    vi.advanceTimersByTime(1)
    expect(get(toasts).length).toBe(count - 1)
  })
})
