/**
 * `formatRelative` serves past and future with the same math: `Intl.RelativeTimeFormat` only
 * looks at the sign, so "3 hours ago" and "in 3 hours" both come from here.
 *
 * `Math.trunc` (not `round`) is the towards-zero cut that the status screen's "X ago" already
 * had: 25h stays "1 day ago", not "2 days ago".
 *
 * Lives outside components because it is pure and testable without Svelte — `locale` is an
 * argument instead of being read from the store for the same reason.
 */
export function formatRelative(targetMs: number, nowMs: number, locale: string): string {
  const diffSeconds = Math.trunc((targetMs - nowMs) / 1000)
  const abs = Math.abs(diffSeconds)
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
  if (abs < 60) return rtf.format(diffSeconds, 'second')
  if (abs < 3600) return rtf.format(Math.trunc(diffSeconds / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.trunc(diffSeconds / 3600), 'hour')
  return rtf.format(Math.trunc(diffSeconds / 86400), 'day')
}

/**
 * `nextAiringIn` turns the API's `next_airing_at` (unix in SECONDS) into relative text, or an
 * empty string when there is nothing to show. Two reasons for empty:
 *
 * - `next_airing_at` missing/0 — finished anime or a hiatus between seasons; AniList returns
 *   `nextAiringEpisode: null` in both cases.
 * - time already in the past — AniList sometimes takes a while to publish the next episode,
 *   and a "next episode 2 days ago" would be a lie on screen.
 *
 * Returns the TIME text, not the whole phrase: the renderer interpolates into `m.next_airing`
 * (same i18n boundary as `domain/animeState.ts`).
 */
export function nextAiringIn(
  nextAiringAt: number | undefined,
  nowMs: number,
  locale: string,
): string {
  if (!nextAiringAt || nextAiringAt * 1000 <= nowMs) return ''
  return formatRelative(nextAiringAt * 1000, nowMs, locale)
}
