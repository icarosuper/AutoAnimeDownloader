import { describe, it, expect } from 'vitest'
import { onboardingSteps, allDone } from '../../src/lib/domain/onboarding'
import type { AnimeInfo, Config, StatusResponse } from '../../src/lib/api/client'

type OnboardingConfig = Pick<Config, 'completed_anime_path' | 'anilist_usernames'>

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

describe('onboardingSteps — item 1 (pasta da biblioteca)', () => {
  it('fica verde com um caminho preenchido', () => {
    const steps = onboardingSteps(makeConfig({ completed_anime_path: '/completed' }), [], VALID_CHECK)
    expect(steps.library).toBe(true)
  })

  it('fica vermelho com caminho vazio', () => {
    expect(onboardingSteps(makeConfig(), [], VALID_CHECK).library).toBe(false)
  })

  it('fica vermelho com caminho só de espaços', () => {
    const steps = onboardingSteps(makeConfig({ completed_anime_path: '   ' }), [], VALID_CHECK)
    expect(steps.library).toBe(false)
  })
})

// O "ou" do item 2 é a regra que estes três casos protegem: a conta AniList deixou de ser
// obrigatória quando animes avulsos passaram a existir.
describe('onboardingSteps — item 2 (fonte de animes)', () => {
  it('fica verde com username do AniList e ZERO animes', () => {
    const steps = onboardingSteps(makeConfig({ anilist_usernames: ['icaro'] }), [], VALID_CHECK)
    expect(steps.source).toBe(true)
  })

  it('fica verde com ZERO usernames e um anime', () => {
    const steps = onboardingSteps(makeConfig(), [makeAnime()], VALID_CHECK)
    expect(steps.source).toBe(true)
  })

  it('fica vermelho com os dois vazios', () => {
    expect(onboardingSteps(makeConfig(), [], VALID_CHECK).source).toBe(false)
  })
})

describe('onboardingSteps — item 3 (primeira verificação)', () => {
  it('fica vermelho sem status', () => {
    expect(onboardingSteps(makeConfig(), [], null).firstCheck).toBe(false)
  })

  it('fica vermelho com last_check vazio', () => {
    expect(onboardingSteps(makeConfig(), [], makeStatus('')).firstCheck).toBe(false)
  })

  // O zero-value de time.Time do Go serializa como uma data perfeitamente parseável — só
  // testar string vazia deixaria o item verde num daemon que nunca rodou.
  it('fica vermelho com o zero-value do Go', () => {
    expect(onboardingSteps(makeConfig(), [], makeStatus('0001-01-01T00:00:00Z')).firstCheck).toBe(false)
  })

  it('fica vermelho com uma data impossível de parsear', () => {
    expect(onboardingSteps(makeConfig(), [], makeStatus('não é data')).firstCheck).toBe(false)
  })

  it('fica verde com uma data recente', () => {
    expect(onboardingSteps(makeConfig(), [], VALID_CHECK).firstCheck).toBe(true)
  })
})

describe('allDone', () => {
  it('é verdadeiro só com os três verdes', () => {
    expect(allDone({ library: true, source: true, firstCheck: true })).toBe(true)
  })

  it('é falso se qualquer um estiver vermelho', () => {
    expect(allDone({ library: false, source: true, firstCheck: true })).toBe(false)
    expect(allDone({ library: true, source: false, firstCheck: true })).toBe(false)
    expect(allDone({ library: true, source: true, firstCheck: false })).toBe(false)
  })
})
