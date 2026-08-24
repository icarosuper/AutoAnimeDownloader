import { describe, it, expect } from 'vitest'
import { get } from 'svelte/store'
import { pickBanner, secondsUntil } from '../../src/lib/domain/systemBanner.js'
import { backendHealth } from '../../src/lib/stores/backendHealth.js'
import type { AnilistHealth } from '../../src/lib/api/client.js'

const ok: AnilistHealth = { state: 'ok' }

describe('pickBanner — precedência', () => {
  it('backend inalcançável ganha da AniList fora do ar', () => {
    // Com o daemon fora, o estado da AniList é a última leitura bem-sucedida, ou seja,
    // informação datada. Mostrar os dois seria ruído contraditório (decisions.md #66).
    const banner = pickBanner('unreachable', { state: 'outage' })
    expect(banner?.kind).toBe('backend_unreachable')
  })

  it('erro do backend ganha do rate limit da AniList', () => {
    expect(pickBanner('server_error', { state: 'rate_limited' })?.kind).toBe('backend_error')
  })

  it('não mostra nada quando está tudo são', () => {
    expect(pickBanner('ok', ok)).toBeNull()
  })

  it('trata AniList ausente como sem informação, não como problema', () => {
    // O status_update do WebSocket não carrega o campo; ausência não pode virar banner.
    expect(pickBanner('ok', undefined)).toBeNull()
  })
})

describe('pickBanner — botão de reportar', () => {
  it('oferece reportar em bug nosso (5xx do backend e 400 da AniList)', () => {
    expect(pickBanner('server_error', ok)?.reportable).toBe(true)
    expect(pickBanner('ok', { state: 'app_bug' })?.reportable).toBe(true)
  })

  it('NÃO oferece reportar em rate limit nem em outage', () => {
    // Reportar comportamento normal gera issue que se fecha com "é a AniList", e treina o
    // usuário a ignorar o botão justamente quando ele importa.
    expect(pickBanner('ok', { state: 'rate_limited' })?.reportable).toBe(false)
    expect(pickBanner('ok', { state: 'outage' })?.reportable).toBe(false)
    expect(pickBanner('unreachable', ok)?.reportable).toBe(false)
  })
})

describe('pickBanner — detalhe cru da AniList', () => {
  it('repassa a mensagem do outage (é onde vem o motivo de um IP bloqueado)', () => {
    const banner = pickBanner('ok', { state: 'outage', message: 'IP blocked: too many requests' })
    expect(banner?.detail).toBe('IP blocked: too many requests')
  })
})

describe('secondsUntil', () => {
  it('arredonda para cima os segundos restantes', () => {
    const now = 1_000_000
    expect(secondsUntil(new Date(now + 29_400), now)).toBe(30)
  })

  it('nunca devolve negativo depois de o prazo passar', () => {
    const now = 1_000_000
    expect(secondsUntil(new Date(now - 5000), now)).toBe(0)
  })

  it('devolve 0 sem prazo — o banner cai na frase sem contagem', () => {
    expect(secondsUntil(undefined, Date.now())).toBe(0)
  })
})

describe('backendHealth', () => {
  it('não acusa na primeira falha — uma só é ruído de rede', () => {
    backendHealth.reset()
    backendHealth.recordFailure('/status', 0)
    expect(get(backendHealth)).toBe('ok')
  })

  it('acusa na segunda falha consecutiva', () => {
    backendHealth.reset()
    backendHealth.recordFailure('/status', 0)
    backendHealth.recordFailure('/status', 0)
    expect(get(backendHealth)).toBe('unreachable')
  })

  it('some no primeiro sucesso, nunca por timer', () => {
    backendHealth.reset()
    backendHealth.recordFailure('/status', 500)
    backendHealth.recordFailure('/status', 500)
    expect(get(backendHealth)).toBe('server_error')
    backendHealth.recordSuccess('/status')
    expect(get(backendHealth)).toBe('ok')
  })

  it('ignora 4xx — 404/405/409 são respostas legítimas de um backend são', () => {
    backendHealth.reset()
    backendHealth.recordFailure('/animes', 409)
    backendHealth.recordFailure('/animes', 409)
    expect(get(backendHealth)).toBe('ok')
  })

  it('conta por endpoint: um /last-check quebrado não é apagado pelo /status são', () => {
    // Este é o ponto cego que o banner fecha: /last-check e /torrents usam silent:true, então
    // um 5xx repetido neles era invisível.
    backendHealth.reset()
    backendHealth.recordFailure('/last-check', 500)
    backendHealth.recordFailure('/last-check', 500)
    backendHealth.recordSuccess('/status')
    expect(get(backendHealth)).toBe('server_error')
  })

  it('inalcançável ganha de 5xx quando os dois estão acesos', () => {
    backendHealth.reset()
    backendHealth.recordFailure('/last-check', 500)
    backendHealth.recordFailure('/last-check', 500)
    backendHealth.recordFailure('/status', 0)
    backendHealth.recordFailure('/status', 0)
    expect(get(backendHealth)).toBe('unreachable')
  })
})
