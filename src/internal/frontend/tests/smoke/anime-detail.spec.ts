import { test, expect } from '@playwright/test'

const mockAnimes = {
  success: true,
  data: [
    {
      anime_id: 123,
      name: 'Test Anime',
      episodes_downloaded: 1,
      episodes_released: 3,
      episodes_watched: 1,
      total_episodes: 12,
      latest_episode_id: 1003,
      last_download_date: '2026-01-01T00:00:00Z',
      cover_image: null,
      is_blacklisted: false,
    },
  ],
}

const mockDetail = {
  success: true,
  data: {
    anime_id: 123,
    anilist_id: 21,
    total_episodes: 12,
    progress: 1,
    status: 'CURRENT',
    cover_image: null,
    custom_search_query: null,
    episodes: [
      {
        episode_id: 1001,
        episode_number: 1,
        airing_at: 0,
        time_until_airing: 0,
        is_aired: true,
        is_watched: true,
        is_downloaded: true,
        download_date: '2026-01-01T00:00:00Z',
        episode_name: 'Episode 1',
        is_manually_managed: false,
        is_blocked: false,
      },
      {
        episode_id: 1002,
        episode_number: 2,
        airing_at: 0,
        time_until_airing: 0,
        is_aired: true,
        is_watched: false,
        is_downloaded: false,
        download_date: null,
        episode_name: 'Episode 2',
        is_manually_managed: false,
        is_blocked: false,
      },
    ],
  },
}

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/animes', route => route.fulfill({ json: mockAnimes }))
  await page.route('**/api/v1/animes/123/episodes', route => route.fulfill({ json: mockDetail }))
  await page.route('**/api/v1/ws', route => route.abort())
})

test('anime detail page loads episode list', async ({ page }) => {
  await page.goto('/#/status/123')
  // 2 episodes in mock data → 2 rows in the desktop table
  await expect(page.locator('table tbody tr')).toHaveCount(2)
})

test('anime title links to its AniList page', async ({ page }) => {
  await page.goto('/#/status/123')
  const titleLink = page.getByRole('link', { name: 'Test Anime' })
  await expect(titleLink).toBeVisible()
  await expect(titleLink).toHaveAttribute('href', 'https://anilist.co/anime/21')
  await expect(titleLink).toHaveAttribute('target', '_blank')
})

test('anime detail shows Download button for undownloaded aired episode', async ({ page }) => {
  await page.route('**/api/v1/animes/123/episodes/1002/download', route =>
    route.fulfill({ json: { success: true, data: null } })
  )

  await page.goto('/#/status/123')
  // Episode 2 is aired but not downloaded — Download button should be visible
  await expect(page.getByRole('button', { name: /download/i }).first()).toBeVisible()
})

test('clicking Download on undownloaded episode calls POST /.../download', async ({ page }) => {
  await page.route('**/api/v1/animes/123/episodes/1002/download', route =>
    route.fulfill({ json: { success: true, data: null } })
  )

  await page.goto('/#/status/123')

  const downloadRequest = page.waitForRequest(
    req => req.url().includes('/episodes/1002/download') && req.method() === 'POST'
  )
  // Click the Download button for Episode 2 (first visible Download button)
  await page.getByRole('button', { name: /^download$/i }).first().click()
  await downloadRequest
})
