import { describe, it, expect } from 'vitest'
import { ALL_EVENTS, EVENT_GROUPS, MESSAGE_PRESET_EVENTS, eventLabel } from '../../src/lib/domain/notificationEvents.js'

describe('notification events', () => {
  it('every event has a label of its own', () => {
    for (const ev of ALL_EVENTS) expect(eventLabel(ev)).not.toBe(ev)
  })

  it('no event is in two groups', () => {
    expect(new Set(ALL_EVENTS).size).toBe(ALL_EVENTS.length)
    expect(EVENT_GROUPS.map(g => g.group)).toEqual(['episodes', 'library', 'system'])
  })

  it('message presets start with known events only', () => {
    for (const ev of MESSAGE_PRESET_EVENTS) expect(ALL_EVENTS).toContain(ev)
  })
})
