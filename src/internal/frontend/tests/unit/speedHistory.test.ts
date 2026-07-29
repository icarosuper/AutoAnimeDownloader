import { describe, it, expect, beforeEach } from 'vitest'
import { get } from 'svelte/store'
import { speedHistory } from '../../src/lib/stores/speedHistory'

describe('speedHistory', () => {
  beforeEach(() => {
    speedHistory.reset()
  })

  it('starts empty', () => {
    expect(get(speedHistory)).toEqual([])
  })

  it('appends samples in push order', () => {
    speedHistory.push(10)
    speedHistory.push(20)
    speedHistory.push(30)
    expect(get(speedHistory)).toEqual([10, 20, 30])
  })

  it('keeps only the most recent 20 samples', () => {
    for (let i = 1; i <= 25; i++) speedHistory.push(i)
    const history = get(speedHistory)
    expect(history).toHaveLength(20)
    expect(history[0]).toBe(6)
    expect(history[19]).toBe(25)
  })

  it('freezes on a failed poll: the caller simply does not push, and the history is untouched', () => {
    speedHistory.push(100)
    // A failed poll never calls push() — nothing to invoke here. The history must not
    // extrapolate/decay/insert a placeholder on its own.
    expect(get(speedHistory)).toEqual([100])
  })

  it('reset clears the history', () => {
    speedHistory.push(1)
    speedHistory.push(2)
    speedHistory.reset()
    expect(get(speedHistory)).toEqual([])
  })
})
