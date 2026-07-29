// Deterministic across dev machines/CI regardless of the host's local timezone — formatDate()
// intentionally renders in the *runtime's* local zone (same as the pre-existing
// `new Date(x).toLocaleString()` call sites it's meant to replace), so the test process pins
// TZ instead of forcing UTC inside the implementation.
process.env.TZ = 'UTC'

import { describe, it, expect } from 'vitest'
import { formatSpeed, formatSpeedParts, formatBytes, formatPercent, formatEta, formatDate } from '../../src/lib/domain/format'

describe('formatSpeed', () => {
  it('formats zero as an idle dash regardless of locale', () => {
    expect(formatSpeed(0, 'pt-BR')).toBe('—')
    expect(formatSpeed(0, 'en')).toBe('—')
  })

  it('formats sub-1024 values as whole bytes', () => {
    expect(formatSpeed(512, 'pt-BR')).toBe('512 B/s')
  })

  it('uses a comma decimal separator in pt-BR', () => {
    expect(formatSpeed(1536, 'pt-BR')).toBe('1,5 KB/s')
  })

  it('uses a dot decimal separator in en', () => {
    expect(formatSpeed(1536, 'en')).toBe('1.5 KB/s')
  })

  it('groups thousands with a dot in pt-BR, a comma in en, for large-in-unit values', () => {
    // 1048470 B/s -> 1023.896... KB/s (stays in KB/s: dividing again would drop under 1024)
    expect(formatSpeed(1048470, 'pt-BR')).toBe('1.023,9 KB/s')
    expect(formatSpeed(1048470, 'en')).toBe('1,023.9 KB/s')
  })
})

describe('formatSpeedParts', () => {
  it('splits the number from its unit so the Status hero can size them separately', () => {
    expect(formatSpeedParts(1536, 'pt-BR')).toEqual({ value: '1,5', unit: 'KB/s' })
    expect(formatSpeedParts(1536, 'en')).toEqual({ value: '1.5', unit: 'KB/s' })
  })

  it('keeps B/s as the unit for sub-1024 values', () => {
    expect(formatSpeedParts(512, 'en')).toEqual({ value: '512', unit: 'B/s' })
  })

  it('returns the idle dash with no unit when there is no traffic', () => {
    expect(formatSpeedParts(0, 'en')).toEqual({ value: '—', unit: '' })
  })

  it('re-joins into exactly what formatSpeed returns', () => {
    for (const bytes of [0, 512, 1536, 1048470, 1073741824]) {
      const { value, unit } = formatSpeedParts(bytes, 'pt-BR')
      expect(unit ? `${value} ${unit}` : value).toBe(formatSpeed(bytes, 'pt-BR'))
    }
  })
})

describe('formatBytes', () => {
  it('formats sub-1024 values as whole bytes', () => {
    expect(formatBytes(512, 'pt-BR')).toBe('512 B')
  })

  it('auto-selects a unit and uses locale-aware separators', () => {
    expect(formatBytes(1048470, 'pt-BR')).toBe('1.023,9 KB')
    expect(formatBytes(1048470, 'en')).toBe('1,023.9 KB')
  })

  it('reaches GB for large values', () => {
    expect(formatBytes(1.5 * 1024 ** 3, 'pt-BR')).toBe('1,5 GB')
  })
})

describe('formatPercent', () => {
  it('rounds a 0..1 fraction to a whole percentage', () => {
    expect(formatPercent(0.4567, 'pt-BR')).toBe('46%')
    expect(formatPercent(0.4567, 'en')).toBe('46%')
  })

  it('clamps at 0% and 100%', () => {
    expect(formatPercent(0, 'pt-BR')).toBe('0%')
    expect(formatPercent(1, 'pt-BR')).toBe('100%')
  })
})

describe('formatEta', () => {
  it('renders null as a dash', () => {
    expect(formatEta(null, 'pt-BR')).toBe('—')
  })

  it('renders seconds under a minute', () => {
    expect(formatEta(45, 'pt-BR')).toBe('45s')
  })

  it('renders minutes and seconds', () => {
    expect(formatEta(150, 'pt-BR')).toBe('2m 30s')
  })

  it('renders hours and minutes', () => {
    expect(formatEta(7380, 'pt-BR')).toBe('2h 3m')
  })

  it('renders days for very long ETAs', () => {
    expect(formatEta(180000, 'pt-BR')).toBe('2d 2h')
  })
})

describe('formatDate', () => {
  it('renders a nullish date as a dash', () => {
    expect(formatDate(undefined, 'pt-BR')).toBe('—')
    expect(formatDate('', 'pt-BR')).toBe('—')
  })

  it('renders day/month/year order in pt-BR', () => {
    expect(formatDate('2026-03-04T10:30:00Z', 'pt-BR')).toBe('04/03/2026, 10:30')
  })

  it('renders month/day/year order in en', () => {
    expect(formatDate('2026-03-04T10:30:00Z', 'en')).toBe('3/4/26, 10:30 AM')
  })
})
