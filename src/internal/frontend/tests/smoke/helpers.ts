import type { Page } from '@playwright/test'

export function makeStatusResponse(status = 'running') {
  return {
    success: true,
    data: { status, last_check: '2026-01-01T00:00:00Z', has_error: false, version: '1.0.0' },
  }
}

export const emptyAnimes = { success: true, data: [] }

/**
 * Three animes that land on three different derived chip states (lib/domain/animeState.ts):
 * "up to date" (downloaded === released), "behind" (released > downloaded) and "blacklisted".
 * They also sort differently by name vs. watched vs. last download, which is what lets the
 * status spec assert that the sortable column headers actually reorder the list.
 */
export function makeAnimesResponse(animes: Array<Record<string, unknown>> = [
  {
    anime_id: 1,
    name: 'Alpha Chronicles',
    episodes_downloaded: 12,
    episodes_released: 12,
    episodes_watched: 12,
    episodes_pending: 0,
    total_episodes: 12,
    latest_episode_id: 12,
    last_download_date: '2026-01-03T00:00:00Z',
    is_blacklisted: false,
  },
  {
    anime_id: 2,
    name: 'Beta Sessions',
    episodes_downloaded: 3,
    episodes_released: 8,
    episodes_watched: 1,
    episodes_pending: 4,
    total_episodes: 12,
    latest_episode_id: 3,
    last_download_date: '2026-01-02T00:00:00Z',
    is_blacklisted: false,
  },
  {
    anime_id: 3,
    name: 'Gamma Protocol',
    episodes_downloaded: 5,
    episodes_released: 5,
    episodes_watched: 0,
    episodes_pending: 0,
    total_episodes: 24,
    latest_episode_id: 5,
    last_download_date: '2026-01-01T00:00:00Z',
    is_blacklisted: true,
  },
]) {
  return { success: true, data: animes }
}

// Status.svelte calls GET /api/v1/torrents on mount and every 5s to feed the speed card
// (totalSpeeds sums download_speed/upload_speed across torrents). Non-zero speeds here let
// tests assert the card actually renders from this data instead of sitting at its idle dash.
export function makeTorrentsResponse(torrents: Array<Record<string, unknown>> = [
  {
    hash: '0123456789abcdef0123456789abcdef01234567',
    name: 'fixture torrent',
    status: 'downloading',
    completed: false,
    is_batch: false,
    episode_number: null,
    bytes_completed: 524288000,
    bytes_total: 1073741824,
    bytes_uploaded: 104857600,
    progress: 0.48,
    download_speed: 1048576,
    upload_speed: 524288,
    peers_total: 14,
    eta_seconds: 240,
    seeded_for_seconds: 0,
  },
]) {
  return { success: true, data: torrents }
}

export const baseConfig = {
  success: true,
  data: {
    anilist_usernames: ['testuser'],
    completed_anime_path: '/completed',
    check_interval: 60,
    max_episodes_per_anime: 12,
    episode_retry_limit: 3,
    delete_watched_episodes: false,
    watched_episodes_to_keep: 0,
    excluded_lists: [],
    rename_files_for_jellyfin: false,
    download_statuses: ['CURRENT', 'REPEATING'],
    download_media_statuses: ['RELEASING', 'FINISHED'],
    delete_statuses: [],
    notifications: { webhooks: [] },
  },
}

export async function setupStatusPageMocks(
  page: Page,
  daemonStatus = 'running',
  // Defaults to no animes so the pre-existing callers keep exercising the empty state; the
  // status spec passes `makeAnimesResponse()` when it needs a list to search/filter/sort.
  animes: unknown = emptyAnimes,
) {
  await page.route('**/api/v1/status', route => route.fulfill({ json: makeStatusResponse(daemonStatus) }))
  await page.route('**/api/v1/animes', route => route.fulfill({ json: animes }))
  await page.route('**/api/v1/config', route => route.fulfill({ json: baseConfig }))
  await page.route('**/api/v1/torrents', route => route.fulfill({ json: makeTorrentsResponse() }))
  await page.route('**/api/v1/ws', route => route.abort())
}
