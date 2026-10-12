import { describe, it, expect } from 'vitest'
import { onboardingSteps, allDone } from '../../src/lib/domain/onboarding'
import type { AnimeInfo, Config, StatusResponse } from '../../src/lib/api/client'

type OnboardingConfig = Pick<Config, 'completed_anime_path' | 'anilist_usernames' | 'mal_usernames'>

function makeConfig(overrides: Partial<OnboardingConfig> = {}): OnboardingConfig {
  return { completed_anime_path: '', anilist_usernames: [], ...overrides }
}

function makeAnime(overrides: Partial<AnimeInfo> = {}): AnimeInfo {
  return {
    anime_id: 1,
    name: 'Test Anime',
    episodes_downloaded: 0,
    episodes_released: 0,
    episodes_watched: 0,
    episodes_pending: 0,
    total_episodes: 12,
    latest_episode_number: 1,
    last_download_date: '',
    is_blacklisted: false,
    ...overrides,
  }
}

function makeStatus(lastCheck: string): StatusResponse {
  return {
    status: 'running',
    last_check: lastCheck,
    has_error: false,
    version: '1.0.0',
    disk_total: 0,
    disk_free: 0,
    disk_low: false,
  }
}

const VALID_CHECK = makeStatus('2026-08-19T10:00:00Z')

describe('onboardingSteps — item 1 (library folder)', () => {
  it('is green with a filled-in path', () => {
    const steps = onboardingSteps(makeConfig({ completed_anime_path: '/completed' }), [], VALID_CHECK)
    expect(steps.library).toBe(true)
  })

  it('is red with an empty path', () => {
    expect(onboardingSteps(makeConfig(), [], VALID_CHECK).library).toBe(false)
  })

  it('is red with a whitespace-only path', () => {
    const steps = onboardingSteps(makeConfig({ completed_anime_path: '   ' }), [], VALID_CHECK)
    expect(steps.library).toBe(false)
  })
})

// The "or" of item 2 is the rule these cases protect: the AniList account stopped being required
// when standalone animes came to exist, and a MyAnimeList account counts the same.
describe('onboardingSteps — item 2 (anime source)', () => {
  it('is green with an AniList username and ZERO animes', () => {
    const steps = onboardingSteps(makeConfig({ anilist_usernames: ['icaro'] }), [], VALID_CHECK)
    expect(steps.source).toBe(true)
  })

  it('is green with a MyAnimeList username and ZERO animes', () => {
    const steps = onboardingSteps(makeConfig({ mal_usernames: ['icaro'] }), [], VALID_CHECK)
    expect(steps.source).toBe(true)
  })

  it('is green with ZERO usernames and one anime', () => {
    const steps = onboardingSteps(makeConfig(), [makeAnime()], VALID_CHECK)
    expect(steps.source).toBe(true)
  })

  it('is red with everything empty', () => {
    expect(onboardingSteps(makeConfig(), [], VALID_CHECK).source).toBe(false)
  })
})

describe('onboardingSteps — item 3 (first check)', () => {
  it('is red with no status', () => {
    expect(onboardingSteps(makeConfig(), [], null).firstCheck).toBe(false)
  })

  it('is red with an empty last_check', () => {
    expect(onboardingSteps(makeConfig(), [], makeStatus('')).firstCheck).toBe(false)
  })

  // Go's time.Time zero value serializes as a perfectly parseable date — testing only for an
  // empty string would leave the item green on a daemon that never ran.
  it("is red with Go's zero value", () => {
    expect(onboardingSteps(makeConfig(), [], makeStatus('0001-01-01T00:00:00Z')).firstCheck).toBe(false)
  })

  it('is red with an unparseable date', () => {
    expect(onboardingSteps(makeConfig(), [], makeStatus('not a date')).firstCheck).toBe(false)
  })

  it('is green with a recent date', () => {
    expect(onboardingSteps(makeConfig(), [], VALID_CHECK).firstCheck).toBe(true)
  })
})

describe('allDone', () => {
  it('is true only with all three green', () => {
    expect(allDone({ library: true, source: true, firstCheck: true })).toBe(true)
  })

  it('is false if any of them is red', () => {
    expect(allDone({ library: false, source: true, firstCheck: true })).toBe(false)
    expect(allDone({ library: true, source: false, firstCheck: true })).toBe(false)
    expect(allDone({ library: true, source: true, firstCheck: false })).toBe(false)
  })
})
