import { describe, it, expect, beforeEach } from 'vitest'
import { get } from 'svelte/store'
import { stallTracker } from '../../src/lib/stores/stallTracker'

describe('stallTracker', () => {
  beforeEach(() => {
    stallTracker.reset()
  })

  it('starts empty', () => {
    expect(get(stallTracker).size).toBe(0)
  })

  it('stamps a hash with `now` the first time it is seen at peers_total===0', () => {
    stallTracker.sync([{ hash: 'a', peers_total: 0 }], 1000)
    expect(get(stallTracker).get('a')).toBe(1000)
  })

  it('does not track a hash with peers_total > 0', () => {
    stallTracker.sync([{ hash: 'a', peers_total: 3 }], 1000)
    expect(get(stallTracker).has('a')).toBe(false)
  })

  it('keeps the ORIGINAL timestamp across repeated syncs, so duration keeps growing', () => {
    stallTracker.sync([{ hash: 'a', peers_total: 0 }], 1000)
    stallTracker.sync([{ hash: 'a', peers_total: 0 }], 5000)
    expect(get(stallTracker).get('a')).toBe(1000)
  })

  it('clears a hash once it gets peers again', () => {
    stallTracker.sync([{ hash: 'a', peers_total: 0 }], 1000)
    stallTracker.sync([{ hash: 'a', peers_total: 1 }], 2000)
    expect(get(stallTracker).has('a')).toBe(false)
  })

  it('clears a hash that disappears from the torrent list (removed/completed)', () => {
    stallTracker.sync([{ hash: 'a', peers_total: 0 }], 1000)
    stallTracker.sync([], 2000)
    expect(get(stallTracker).has('a')).toBe(false)
  })

  it('tracks multiple hashes independently', () => {
    stallTracker.sync(
      [
        { hash: 'a', peers_total: 0 },
        { hash: 'b', peers_total: 2 },
      ],
      1000,
    )
    const map = get(stallTracker)
    expect(map.get('a')).toBe(1000)
    expect(map.has('b')).toBe(false)
  })

  it('reset clears everything', () => {
    stallTracker.sync([{ hash: 'a', peers_total: 0 }], 1000)
    stallTracker.reset()
    expect(get(stallTracker).size).toBe(0)
  })
})
