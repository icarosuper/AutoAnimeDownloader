import { describe, it, expect } from 'vitest'
import { countByLevel, formatLogTime, parseLogLine } from '../../src/lib/domain/logLine.js'

describe('parseLogLine', () => {
  it('reads zerolog JSON format and splits caller from the rest of the fields', () => {
    const line =
      '{"level":"warn","time":"2026-07-29T11:04:03Z","caller":"nyaa.go:41","message":"no torrent","query":"Alpha 12"}'

    const parsed = parseLogLine(line)

    expect(parsed.level).toBe('warn')
    expect(parsed.message).toBe('no torrent')
    expect(parsed.caller).toBe('nyaa.go:41')
    // caller becomes its own column: it must not reappear at the end of the message.
    expect(parsed.extras).toBe('"query"="Alpha 12"')
    expect(parsed.raw).toBe(line)
  })

  it('does not invent extras when there are only the standard fields', () => {
    const parsed = parseLogLine('{"level":"info","time":"2026-07-29T11:04:02Z","message":"ok"}')
    expect(parsed.extras).toBeUndefined()
    expect(parsed.caller).toBeUndefined()
  })

  it('serializes nested objects inside extras', () => {
    const parsed = parseLogLine('{"level":"info","message":"m","meta":{"a":1}}')
    expect(parsed.extras).toBe('"meta"="{"a":1}"')
  })

  it('treats fatal and panic as error', () => {
    expect(parseLogLine('{"level":"fatal","message":"x"}').level).toBe('error')
    expect(parseLogLine('{"level":"panic","message":"x"}').level).toBe('error')
  })

  it('falls back to info when the JSON level is unknown or missing', () => {
    expect(parseLogLine('{"message":"x"}').level).toBe('info')
    expect(parseLogLine('{"level":"trace","message":"x"}').level).toBe('info')
  })

  it('reads zerolog console format by the level abbreviations', () => {
    const parsed = parseLogLine('11:04AM WRN no seeds')
    expect(parsed.level).toBe('warn')
    expect(parsed.message).toBe('11:04AM WRN no seeds')
    expect(parsed.time).toBe('')
  })

  it('maps console FAT to error', () => {
    expect(parseLogLine('11:04AM FAT died').level).toBe('error')
  })

  it('uses a keyword heuristic on a line with no known format', () => {
    expect(parseLogLine('something failed with error 500').level).toBe('error')
    expect(parseLogLine('warning: disk almost full').level).toBe('warn')
    expect(parseLogLine('just some line').level).toBe('info')
  })

  it('does not break on malformed JSON — falls through to the heuristics', () => {
    const parsed = parseLogLine('{"level":"error", this is not json')
    expect(parsed.level).toBe('error')
    expect(parsed.raw).toBe('{"level":"error", this is not json')
  })
})

describe('formatLogTime', () => {
  it('formats an ISO timestamp as HH:MM:SS', () => {
    // Built from a local Date so the test does not depend on the machine's timezone.
    const iso = new Date(2026, 6, 29, 9, 5, 3).toISOString()
    expect(formatLogTime(iso)).toBe('09:05:03')
  })

  it('returns an empty string when there is no timestamp', () => {
    expect(formatLogTime(undefined)).toBe('')
    expect(formatLogTime('')).toBe('')
  })

  it('returns the raw value when it is not a valid date, instead of "Invalid Date"', () => {
    expect(formatLogTime('not-a-date')).toBe('not-a-date')
  })
})

describe('countByLevel', () => {
  it('counts by level and keeps the total in all', () => {
    const parsed = [
      '{"level":"info","message":"a"}',
      '{"level":"error","message":"b"}',
      '{"level":"error","message":"c"}',
      '{"level":"debug","message":"d"}',
    ].map(parseLogLine)

    expect(countByLevel(parsed)).toEqual({ all: 4, debug: 1, info: 1, warn: 0, error: 2 })
  })

  it('zeros everything on an empty list', () => {
    expect(countByLevel([])).toEqual({ all: 0, debug: 0, info: 0, warn: 0, error: 0 })
  })
})
