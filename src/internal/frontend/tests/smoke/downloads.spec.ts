import { test, expect } from '@playwright/test'
import type { Page } from '@playwright/test'
import { makeStatusResponse } from './helpers.js'

function torrent(overrides: Record<string, unknown>) {
  return {
    hash: '0'.repeat(40),
    name: 'fixture torrent',
    status: 'downloading',
    completed: false,
    is_batch: false,
    anime_name: null,
    episode_number: null,
    bytes_completed: 524288000,
    bytes_total: 1073741824,
    bytes_uploaded: 104857600,
    progress: 0.3,
    download_speed: 1048576,
    upload_speed: 524288,
    peers_total: 14,
    eta_seconds: 240,
    seeded_for_seconds: 0,
    ...overrides,
  }
}

// 4 torrents: 2 "Frieren" downloading (different progress, for sort assertions), 1 "Frieren"
// stopped (for status-filter pruning) and 1 "Bleach" seeding (for the search test).
const mockTorrents = [
  torrent({
    hash: 'a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1',
    name: 'Frieren - 01',
    anime_name: 'Frieren',
    status: 'downloading',
    progress: 0.3,
  }),
  torrent({
    hash: 'b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2',
    name: 'Frieren - 02',
    anime_name: 'Frieren',
    status: 'downloading',
    progress: 0.7,
  }),
  torrent({
    hash: 'c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3',
    name: 'Frieren - 03',
    anime_name: 'Frieren',
    status: 'stopped',
    progress: 0.1,
  }),
  torrent({
    hash: 'd4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4',
    name: 'Bleach batch',
    anime_name: 'Bleach',
    status: 'seeding',
    progress: 0.9,
  }),
]

// Torrent rows inside an expanded accordion group.
const ROWS = '[data-torrent-row]'

async function mockCommon(page: Page) {
  await page.route('**/api/v1/status', route => route.fulfill({ json: makeStatusResponse('running') }))
  await page.route('**/api/v1/animes', route => route.fulfill({ json: { success: true, data: [] } }))
  await page.route('**/api/v1/ws', route => route.abort())
}

async function mockTorrentsList(page: Page, torrents = mockTorrents) {
  // Exact-path match so it doesn't also swallow `/torrents/<hash>?...` action requests.
  await page.route('**/api/v1/torrents', route => route.fulfill({ json: { success: true, data: torrents } }))
}

test.beforeEach(async ({ page }) => {
  await mockCommon(page)
})

test('typing in search reduces the visible rows', async ({ page }) => {
  await mockTorrentsList(page)
  await page.goto('/#/downloads')
  await expect(page.locator(ROWS)).toHaveCount(4)

  await page.getByPlaceholder('Search by name…').fill('bleach')
  await expect(page.locator(ROWS)).toHaveCount(1)
  await expect(page.locator('[data-group]')).toContainText('Bleach')
})

test('select-all marks only the visible rows and shows the bulk action bar', async ({ page }) => {
  await mockTorrentsList(page)
  await page.goto('/#/downloads')

  // Narrow to the 3 "Frieren" rows first, so we can assert select-all does NOT also pick up
  // the filtered-out "Bleach" row.
  await page.getByPlaceholder('Search by name…').fill('frieren')
  await expect(page.locator(ROWS)).toHaveCount(3)

  await page.getByRole('checkbox', { name: 'Select all visible torrents' }).check()
  await expect(page.getByText('3 selected')).toBeVisible()
})

test('switching the status filter prunes the selection to what remains visible', async ({ page }) => {
  await mockTorrentsList(page)
  await page.goto('/#/downloads')

  // Select all 4 rows.
  await page.getByRole('checkbox', { name: 'Select all visible torrents' }).check()
  await expect(page.getByText('4 selected')).toBeVisible()

  // Filter down to only "seeding" (the Bleach row) — the other 3 selected hashes drop out of
  // view and must be pruned from the selection.
  await page.getByRole('button', { name: /^Seeding/ }).click()

  await expect(page.locator(ROWS)).toHaveCount(1)
  await expect(page.getByText('1 selected')).toBeVisible()
})

test('delete dialog opens and fires DELETE with keep_data=false&block=true', async ({ page }) => {
  await mockTorrentsList(page)

  let deleteUrl: string | null = null
  await page.route('**/api/v1/torrents/a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1*', async route => {
    if (route.request().method() === 'DELETE') {
      deleteUrl = route.request().url()
      await route.fulfill({ json: { success: true, data: null } })
    } else {
      await route.continue()
    }
  })

  await page.goto('/#/downloads')

  const firstRow = page.locator(ROWS).first()
  await firstRow.getByRole('button', { name: /^Delete — / }).click()

  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()

  const deleteRequest = page.waitForRequest(
    req => req.url().includes('/api/v1/torrents/a1a1') && req.method() === 'DELETE',
  )
  // Both checkboxes default to checked (delete files + block redownload) — confirm without
  // touching them, matching the "default is delete + block" decision.
  await dialog.getByRole('button', { name: 'Delete' }).click()
  await deleteRequest

  expect(deleteUrl).toContain('keep_data=false')
  expect(deleteUrl).toContain('block=true')
})

test('deep link arrives with search, filter and sort already applied', async ({ page }) => {
  await mockTorrentsList(page)

  await page.goto('/#/downloads?q=frieren&status=downloading&sort=progress&dir=desc')

  // q=frieren + status=downloading matches exactly the two "downloading" Frieren rows
  // (progress 0.3 and 0.7); the stopped Frieren row and the Bleach row are excluded.
  await expect(page.locator(ROWS)).toHaveCount(2)

  await expect(page.getByPlaceholder('Search by name…')).toHaveValue('frieren')

  // dir=desc on progress: the 70% row (Frieren - 02) sorts before the 30% row (Frieren - 01).
  // (Both rows display as "Frieren" — anime_name wins over the raw torrent name in the Name
  // column — so the percentage is what distinguishes them here.)
  const rows = page.locator(ROWS)
  await expect(rows.nth(0)).toContainText('70%')
  await expect(rows.nth(1)).toContainText('30%')
})

// Spec §9.3 keeps the whole view state in the URL — and Fase 5 adds the accordion to it, so a
// collapsed group survives a reload/share the same way search, filter and sort already did.
test('collapsing a group is reflected in the querystring and survives a reload', async ({ page }) => {
  await mockTorrentsList(page)
  await page.goto('/#/downloads')

  // Frieren has 3 torrents, Bleach 1.
  await expect(page.locator(ROWS)).toHaveCount(4)

  await page.getByRole('button', { name: /expand or collapse frieren/i }).click()
  await expect(page.locator(ROWS)).toHaveCount(1)
  await expect(page).toHaveURL(/closed=n%3AFrieren/)

  await page.reload()
  await expect(page.locator(ROWS)).toHaveCount(1)
})

test('the problems pill filters down to torrents that are stuck at zero peers', async ({ page }) => {
  await mockTorrentsList(page, [
    torrent({ hash: 'a'.repeat(40), name: 'Healthy', anime_name: 'Healthy', peers_total: 8 }),
    torrent({ hash: 'b'.repeat(40), name: 'Stuck', anime_name: 'Stuck', peers_total: 0 }),
    // Paused at zero peers is a user decision, not a problem — it must stay out.
    torrent({ hash: 'c'.repeat(40), name: 'Paused', anime_name: 'Paused', status: 'stopped', peers_total: 0 }),
  ])
  await page.goto('/#/downloads')
  await expect(page.locator(ROWS)).toHaveCount(3)

  await page.getByRole('button', { name: /^problems/i }).click()

  await expect(page.locator(ROWS)).toHaveCount(1)
  await expect(page.locator('[data-group]')).toContainText('Stuck')
  await expect(page).toHaveURL(/problems=1/)
})

// The bulk group is always present now (dimmed + disabled with an empty selection) instead of
// appearing and disappearing, which used to shove the list up and down on every click.
test('bulk actions are visible but disabled until something is selected', async ({ page }) => {
  await mockTorrentsList(page)
  await page.goto('/#/downloads')

  const bulkPause = page.getByRole('button', { name: /^pause$/i })
  await expect(bulkPause).toBeVisible()
  await expect(bulkPause).toBeDisabled()

  await page.getByRole('checkbox', { name: 'Select all visible torrents' }).check()
  await expect(bulkPause).toBeEnabled()
})

// A group's aggregate bar must read its own state: an all-seeded group is not "still working".
// Asserted on the computed color because the first implementation painted every non-problem
// group accent-purple, which looked like an in-flight download next to green Seeding chips.
test('the group bar is colored by the group state, not always accent', async ({ page }) => {
  await mockTorrentsList(page, [
    torrent({ hash: 'a'.repeat(40), name: 'DL - 01', anime_name: 'Downloading show', anime_id: 1, status: 'downloading', peers_total: 5 }),
    torrent({ hash: 'b'.repeat(40), name: 'SEED - 01', anime_name: 'Seeded show', anime_id: 2, status: 'seeding', completed: true, progress: 1, peers_total: 2 }),
    torrent({ hash: 'c'.repeat(40), name: 'STUCK - 01', anime_name: 'Stalled show', anime_id: 3, status: 'downloading', peers_total: 0 }),
  ])
  await page.goto('/#/downloads')

  async function groupBarColor(name: string) {
    const bar = page.getByRole('progressbar', { name: `Overall progress of ${name}` }).locator('div').first()
    return bar.evaluate(el => getComputedStyle(el).backgroundColor)
  }

  const downloading = await groupBarColor('Downloading show')
  const seeded = await groupBarColor('Seeded show')
  const stalled = await groupBarColor('Stalled show')

  expect(seeded).not.toBe(downloading)
  expect(stalled).not.toBe(downloading)
  expect(stalled).not.toBe(seeded)
})

// Progress here comes from the 2s HTTP poll, not the WebSocket — so when the socket drops the
// screen has to say the numbers below may be stale rather than let them read as live.
test('a dropped daemon connection shows the stale-progress banner', async ({ page }) => {
  await mockTorrentsList(page)
  await page.goto('/#/downloads')

  await expect(page.getByRole('alert')).toContainText('progress below may be out of date')
  await expect(page.getByRole('button', { name: /reconnect/i })).toBeVisible()
})
