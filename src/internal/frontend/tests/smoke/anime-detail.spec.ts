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
      episodes_pending: 1,
      total_episodes: 12,
      latest_last_download_date: '2026-01-01T00:00:00Z',
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

// Desktop and mobile render the same row definition; at the desktop viewport only the desktop
// block is displayed, so `:visible` is what keeps these counts from double-counting.
const rows = '[data-episode-row]:visible'

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/animes', route => route.fulfill({ json: mockAnimes }))
  await page.route('**/api/v1/animes/123/episodes', route => route.fulfill({ json: mockDetail }))
  await page.route('**/api/v1/ws', route => route.abort())
})

test('anime detail page loads episode list', async ({ page }) => {
  await page.goto('/#/status/123')
  await expect(page.locator(rows)).toHaveCount(2)
})

test('anime title links to its AniList page', async ({ page }) => {
  await page.goto('/#/status/123')
  const titleLink = page.getByRole('link', { name: 'Test Anime' })
  await expect(titleLink).toBeVisible()
  await expect(titleLink).toHaveAttribute('href', 'https://anilist.co/anime/21')
  await expect(titleLink).toHaveAttribute('target', '_blank')
})

test('anime detail shows Download button for undownloaded aired episode', async ({ page }) => {
  await page.goto('/#/status/123')
  // Episode 2 is aired but not downloaded -> episodeActions() gives it `download` as the
  // principal action, rendered with text in the fixed actions column.
  await expect(page.getByRole('button', { name: /^download$/i }).first()).toBeVisible()
})

test('clicking Download on undownloaded episode calls POST /.../download', async ({ page }) => {
  await page.route('**/api/v1/animes/123/episodes/1002/download', route =>
    route.fulfill({ json: { success: true, data: null } })
  )

  await page.goto('/#/status/123')

  const downloadRequest = page.waitForRequest(
    req => req.url().includes('/episodes/1002/download') && req.method() === 'POST'
  )
  await page.getByRole('button', { name: /^download$/i }).first().click()
  await downloadRequest
})

// The downloaded episode gets `redownload` as its principal action, not `download` — the two
// used to be adjacent icon-only buttons whose meaning was only in a `title` attribute.
test('downloaded episode offers Redownload as its principal action', async ({ page }) => {
  await page.goto('/#/status/123')
  await expect(page.getByRole('button', { name: /^redownload$/i }).first()).toBeVisible()
})

// Fase 4's core deliverable: one action definition (episodeActions) feeding a labelled menu,
// instead of the same five icon buttons written out twice.
test('the row overflow menu lists the remaining actions with text labels', async ({ page }) => {
  await page.goto('/#/status/123')

  await page.getByRole('button', { name: /more actions for episode 2/i }).first().click()

  const menu = page.getByRole('menu').first()
  await expect(menu.getByRole('menuitem', { name: /^replace$/i })).toBeVisible()

  // Only one menu open at a time, and Escape closes it (ActionMenu contract, spec §6).
  await page.keyboard.press('Escape')
  await expect(page.getByRole('menu')).toHaveCount(0)
})

// Regression: `title="Soltar episódio"` was hardcoded Portuguese in BOTH copies of the action
// list (AnimeDetail.svelte:785 and :956), outside i18n. It is now a message key, so an
// English session must read "Release episode".
test('the release action is translated, not hardcoded Portuguese', async ({ page }) => {
  await page.route('**/api/v1/animes/123/episodes', route =>
    route.fulfill({
      json: {
        ...mockDetail,
        data: {
          ...mockDetail.data,
          episodes: [{ ...mockDetail.data.episodes[1], is_blocked: true }],
        },
      },
    })
  )

  await page.goto('/#/status/123')
  await expect(page.getByRole('button', { name: /^release episode$/i }).first()).toBeVisible()
  await expect(page.getByText('Soltar episódio')).toHaveCount(0)
})

// The custom Nyaa search query (custom_search_query / updateAnimeSettings) is a real feature
// the design handoff forgot about; spec §9.2 keeps it in a collapsible block.
test('custom search query is present in a collapsible block and saves', async ({ page }) => {
  await page.route('**/api/v1/animes/123/settings', route =>
    route.fulfill({ json: { success: true, data: null } })
  )

  await page.goto('/#/status/123')

  const input = page.getByLabel(/custom nyaa search query/i)
  await expect(input).toBeHidden()

  await page.getByRole('button', { name: /custom nyaa search query/i }).click()
  await expect(input).toBeVisible()

  const saveRequest = page.waitForRequest(
    req => req.url().includes('/animes/123/settings') && req.method() === 'PUT'
  )
  await input.fill('custom query')
  await page.getByRole('button', { name: /^save$/i }).click()
  await saveRequest
})

test('episode row shows a progress bar when an active torrent is joined to it', async ({ page }) => {
  // Episode 2 carries episode_hash pointing at an in-flight torrent — indexTorrentsByEpisode
  // (src/lib/utils/torrentsByEpisode.ts) joins the two on that hash.
  //
  // is_downloaded is true here on purpose, and that is the realistic shape: the daemon writes
  // the saved-episode record (which is what produces both is_downloaded and episode_hash in
  // endpoint_anime_episodes.go) the moment the torrent is *added*, so an episode with a hash
  // always reports is_downloaded: true even while the download is still running. The old
  // fixture used is_downloaded: false, a combination the API can never emit, which is what let
  // the "!ep.is_downloaded" guard in AnimeDetail.svelte hide the bar in production unnoticed.
  const detailWithHash = {
    ...mockDetail,
    data: {
      ...mockDetail.data,
      episodes: mockDetail.data.episodes.map(ep =>
        ep.episode_number === 2
          ? { ...ep, is_downloaded: true, download_date: '2026-01-02T00:00:00Z', episode_hash: 'f'.repeat(40) }
          : ep
      ),
    },
  }
  await page.route('**/api/v1/animes/123/episodes', route => route.fulfill({ json: detailWithHash }))
  await page.route('**/api/v1/torrents', route =>
    route.fulfill({
      json: {
        success: true,
        data: [
          {
            hash: 'f'.repeat(40),
            name: 'Test Anime - 02',
            status: 'downloading',
            completed: false,
            is_batch: false,
            anime_name: 'Test Anime',
            episode_number: 2,
            bytes_completed: 100,
            bytes_total: 1000,
            bytes_uploaded: 0,
            progress: 0.4,
            download_speed: 2048,
            upload_speed: 0,
            peers_total: 3,
            eta_seconds: 120,
            seeded_for_seconds: 0,
          },
        ],
      },
    })
  )

  await page.goto('/#/status/123')

  await expect(page.getByRole('progressbar', { name: 'Download progress' }).first()).toBeVisible()
  await expect(page.locator(rows).filter({ hasText: '40%' })).toBeVisible()
})
