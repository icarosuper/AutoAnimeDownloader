import type { AnimeInfo, Config, StatusResponse } from '../api/client.js'

/**
 * Os três itens da linha de chegada do primeiro uso. A tela não decide nada, só pinta — é a
 * mesma fronteira que `animeState.ts` documenta: a lógica sai do componente para poder ser
 * testada sem DOM e sem locale.
 */
export interface OnboardingSteps {
  /** `completed_anime_path` preenchido. */
  library: boolean
  /** Pelo menos UMA fonte de animes: conta AniList **ou** anime avulso. */
  source: boolean
  /** O daemon já rodou um passe de verificação. */
  firstCheck: boolean
}

/**
 * Datas anteriores a 2010 são o zero-value de `time.Time` do Go (`0001-01-01T00:00:00Z`)
 * serializado, não um passe real — e ele é uma data perfeitamente parseável, então testar só
 * string vazia deixaria o item verde num daemon que nunca rodou. Mesmo corte que
 * `Status.svelte` já aplica em `formatDate`/`formatTimeAgo`.
 */
const MIN_REAL_YEAR = 2010

function isRealDate(value: string | undefined): boolean {
  if (!value) return false
  const date = new Date(value)
  return !isNaN(date.getTime()) && date.getFullYear() >= MIN_REAL_YEAR
}

export function onboardingSteps(
  config: Pick<Config, 'completed_anime_path' | 'anilist_usernames'>,
  animes: AnimeInfo[],
  status: StatusResponse | null,
): OnboardingSteps {
  return {
    library: !!config.completed_anime_path?.trim(),
    // Um "ou", não um "e": oferecer só a conta AniList mentiria sobre o modelo, porque um
    // anime avulso sozinho já dá ao daemon o que verificar.
    source: (config.anilist_usernames ?? []).length > 0 || animes.length > 0,
    firstCheck: isRealDate(status?.last_check),
  }
}

/** Derivado aqui para que a condição de render e os testes leiam a mesma regra. */
export function allDone(steps: OnboardingSteps): boolean {
  return steps.library && steps.source && steps.firstCheck
}
