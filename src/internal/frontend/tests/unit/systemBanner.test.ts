import { describe, it, expect } from 'vitest'
import { get } from 'svelte/store'
import { pickBanner, secondsUntil } from '../../src/lib/domain/systemBanner.js'
import { backendHealth } from '../../src/lib/stores/backendHealth.js'
import type { AnilistHealth } from '../../src/lib/api/client.js'

const ok: AnilistHealth = { state: 'ok' }

describe('pickBanner — precedence', () => {
  it('unreachable backend beats AniList being down', () => {
    // With the daemon down, the AniList state is the last successful read, i.e. stale
    // information. Showing both would be contradictory noise (decisions.md #66).
    const banner = pickBanner('unreachable', { state: 'outage' })
    expect(banner?.kind).toBe('backend_unreachable')
  })

  it('backend error beats AniList rate limit', () => {
    expect(pickBanner('server_error', { state: 'rate_limited' })?.kind).toBe('backend_error')
  })

  it('shows nothing when everything is healthy', () => {
    expect(pickBanner('ok', ok)).toBeNull()
  })

  it('treats missing AniList as no information, not as a problem', () => {
    // The WebSocket status_update does not carry the field; absence must not become a banner.
    expect(pickBanner('ok', undefined)).toBeNull()
  })
})

describe('pickBanner — report button', () => {
  it('offers report on our own bugs (backend 5xx and AniList 400)', () => {
    expect(pickBanner('server_error', ok)?.reportable).toBe(true)
    expect(pickBanner('ok', { state: 'app_bug' })?.reportable).toBe(true)
  })

  it('does NOT offer report on rate limit or outage', () => {
    // Reporting normal behavior produces an issue that closes with "it's AniList", and trains
    // the user to ignore the button exactly when it matters.
    expect(pickBanner('ok', { state: 'rate_limited' })?.reportable).toBe(false)
    expect(pickBanner('ok', { state: 'outage' })?.reportable).toBe(false)
    expect(pickBanner('unreachable', ok)?.reportable).toBe(false)
  })
})

describe('pickBanner — raw AniList detail', () => {
  it('forwards the outage message (that is where a blocked-IP reason comes from)', () => {
    const banner = pickBanner('ok', { state: 'outage', message: 'IP blocked: too many requests' })
    expect(banner?.detail).toBe('IP blocked: too many requests')
  })
})

describe('secondsUntil', () => {
  it('rounds remaining seconds up', () => {
    const now = 1_000_000
    expect(secondsUntil(new Date(now + 29_400), now)).toBe(30)
  })

  it('never returns negative after the deadline has passed', () => {
    const now = 1_000_000
    expect(secondsUntil(new Date(now - 5000), now)).toBe(0)
  })

  it('returns 0 with no deadline — the banner falls back to the phrase without a countdown', () => {
    expect(secondsUntil(undefined, Date.now())).toBe(0)
  })
})

describe('backendHealth', () => {
  it('does not raise on the first failure — a single one is network noise', () => {
    backendHealth.reset()
    backendHealth.recordFailure('/status', 0)
    expect(get(backendHealth)).toBe('ok')
  })

  it('raises on the second consecutive failure', () => {
    backendHealth.reset()
    backendHealth.recordFailure('/status', 0)
    backendHealth.recordFailure('/status', 0)
    expect(get(backendHealth)).toBe('unreachable')
  })

  it('vanishes on the first success, never by timer', () => {
    backendHealth.reset()
    backendHealth.recordFailure('/status', 500)
    backendHealth.recordFailure('/status', 500)
    expect(get(backendHealth)).toBe('server_error')
    backendHealth.recordSuccess('/status')
    expect(get(backendHealth)).toBe('ok')
  })

  it('ignores 4xx — 404/405/409 are legitimate responses from a healthy backend', () => {
    backendHealth.reset()
    backendHealth.recordFailure('/animes', 409)
    backendHealth.recordFailure('/animes', 409)
    expect(get(backendHealth)).toBe('ok')
  })

  it('counts per endpoint: a broken /last-check is not erased by a healthy /status', () => {
    // This is the blind spot the banner closes: /last-check and /torrents use silent:true, so
    // a repeated 5xx on them used to be invisible.
    backendHealth.reset()
    backendHealth.recordFailure('/last-check', 500)
    backendHealth.recordFailure('/last-check', 500)
    backendHealth.recordSuccess('/status')
    expect(get(backendHealth)).toBe('server_error')
  })

  it('unreachable beats 5xx when both are lit', () => {
    backendHealth.reset()
    backendHealth.recordFailure('/last-check', 500)
    backendHealth.recordFailure('/last-check', 500)
    backendHealth.recordFailure('/status', 0)
    backendHealth.recordFailure('/status', 0)
    expect(get(backendHealth)).toBe('unreachable')
  })
})

describe('local cache', () => {
  // The snapshot date only counts when IT is what is on screen. With AniList up the file
  // exists the same way, and announcing "showing yesterday's cache" while showing fresh data
  // is saying the screen is wrong when it is right.
  it('does not expose the cache date with AniList up', () => {
    expect(pickBanner('ok', { state: 'ok', cache_saved_at: '2026-09-05T10:00:00Z' })).toBeNull()
  })

  it('exposes the cache date on an outage', () => {
    const banner = pickBanner('ok', { state: 'outage', cache_saved_at: '2026-09-05T10:00:00Z' })
    expect(banner?.kind).toBe('anilist_outage')
    expect(banner?.cachedAt).toEqual(new Date('2026-09-05T10:00:00Z'))
  })

  it('omits the date when there was never a snapshot', () => {
    expect(pickBanner('ok', { state: 'outage' })?.cachedAt).toBeUndefined()
  })
})
