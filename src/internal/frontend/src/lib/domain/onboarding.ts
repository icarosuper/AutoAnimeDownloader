import type { AnimeInfo, Config, StatusResponse } from '../api/client.js'

/**
 * The three items of the first-use finish line. The screen decides nothing, it only paints — the
 * same boundary `animeState.ts` documents: the logic leaves the component so it can be tested
 * without a DOM and without a locale.
 */
export interface OnboardingSteps {
  /** `completed_anime_path` filled in. */
  library: boolean
  /** At least ONE anime source: an AniList **or** MyAnimeList account, **or** a standalone anime. */
  source: boolean
  /** The daemon has already run a check pass. */
  firstCheck: boolean
}

/**
 * Dates before 2010 are Go's serialized `time.Time` zero value (`0001-01-01T00:00:00Z`), not a
 * real pass — and it's a perfectly parseable date, so testing only for an empty string would
 * leave the item green on a daemon that never ran. The same cutoff `Status.svelte` already
 * applies in `formatDate`/`formatTimeAgo`.
 */
const MIN_REAL_YEAR = 2010

function isRealDate(value: string | undefined): boolean {
  if (!value) return false
  const date = new Date(value)
  return !isNaN(date.getTime()) && date.getFullYear() >= MIN_REAL_YEAR
}

export function onboardingSteps(
  config: Pick<Config, 'completed_anime_path' | 'anilist_usernames' | 'mal_usernames'>,
  animes: AnimeInfo[],
  status: StatusResponse | null,
): OnboardingSteps {
  return {
    library: !!config.completed_anime_path?.trim(),
    // An "or", not an "and": offering only a list account would lie about the model, because a
    // standalone anime alone already gives the daemon something to check.
    source:
      (config.anilist_usernames ?? []).length > 0 ||
      (config.mal_usernames ?? []).length > 0 ||
      animes.length > 0,
    firstCheck: isRealDate(status?.last_check),
  }
}

/** Derived here so the render condition and the tests read the same rule. */
export function allDone(steps: OnboardingSteps): boolean {
  return steps.library && steps.source && steps.firstCheck
}
