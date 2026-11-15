// Options and toggling for the per-webhook source filter (WebhookPreset.anime_sources): which animes a
// webhook notifies about. Keys are the backend's: "anilist:<user>", "mal:<user>", "standalone".
// An empty list means every anime — the behavior before the filter existed — and matching is OR.

export const SOURCE_STANDALONE = 'standalone'

export type SourceOption =
  | { key: string; kind: 'account'; username: string; service?: 'AniList' | 'MAL' }
  | { key: string; kind: 'standalone' }
  | { key: string; kind: 'stale' }

/**
 * One option per configured account, then Standalone, then any key the webhook still holds that
 * no longer matches a configured account. That last group exists so a filter left pointing at a
 * removed account can be seen and unchecked — hidden, it would silence the webhook for good.
 * `service` is only set when the same username exists on AniList and MAL.
 */
export function sourceOptions(anilist: string[], mal: string[], current: string[] = []): SourceOption[] {
  const accounts = [
    ...anilist.map(username => ({ key: `anilist:${username}`, username, service: 'AniList' as const })),
    ...mal.map(username => ({ key: `mal:${username}`, username, service: 'MAL' as const })),
  ]
  const options: SourceOption[] = accounts.map(acc => ({
    key: acc.key,
    kind: 'account',
    username: acc.username,
    ...(accounts.filter(o => o.username === acc.username).length > 1 ? { service: acc.service } : {}),
  }))
  options.push({ key: SOURCE_STANDALONE, kind: 'standalone' })
  for (const key of current) {
    if (!options.some(o => o.key === key)) options.push({ key, kind: 'stale' })
  }
  return options
}

/** Checks or unchecks one source. Unchecking the last one leaves [] — back to "every anime". */
export function toggleSource(current: string[] = [], key: string, checked: boolean): string[] {
  const without = current.filter(k => k !== key)
  return checked ? [...without, key] : without
}
