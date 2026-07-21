import type { Page } from '@playwright/test'

export function makeStatusResponse(status = 'running') {
  return {
    success: true,
    data: { status, last_check: '2026-01-01T00:00:00Z', has_error: false, version: '1.0.0' },
  }
}

export const emptyAnimes = { success: true, data: [] }

export const baseConfig = {
  success: true,
  data: {
    anilist_usernames: ['testuser'],
    save_path: '/downloads',
    completed_anime_path: '',
    check_interval: 60,
    qbittorrent_url: 'http://localhost:8080',
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

export async function setupStatusPageMocks(page: Page, daemonStatus = 'running') {
  await page.route('**/api/v1/status', route => route.fulfill({ json: makeStatusResponse(daemonStatus) }))
  await page.route('**/api/v1/animes', route => route.fulfill({ json: emptyAnimes }))
  await page.route('**/api/v1/config', route => route.fulfill({ json: baseConfig }))
  await page.route('**/api/v1/ws', route => route.abort())
}
