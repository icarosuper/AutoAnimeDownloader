import { test, expect } from '@playwright/test'

/**
 * Phase 7 (spec §9.5). The Logs screen had no smoke; it got one here because the phase's
 * deliverable is precisely "filter, search and follow preserved" — and all three changed
 * shape: the level filter became pills with counts (it was a <select> with no count) and
 * the source became a column derived from zerolog's `caller`.
 */

const LINES = [
  '{"level":"info","time":"2026-07-29T11:04:02Z","caller":"loop.go:90","message":"check started"}',
  '{"level":"warn","time":"2026-07-29T11:04:03Z","caller":"nyaa.go:41","message":"no torrent for Alpha","query":"Alpha 12"}',
  '{"level":"error","time":"2026-07-29T11:04:05Z","caller":"anilist.go:210","message":"failed to query AniList"}',
  '{"level":"debug","time":"2026-07-29T11:04:06Z","caller":"session.go:77","message":"torrent added"}',
]

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/logs**', route =>
    route.fulfill({ json: { success: true, data: { lines: LINES, total: LINES.length } } })
  )
  await page.route('**/api/v1/ws', route => route.abort())
})

/**
 * One row per log entry. Anchors on the `listitem` role, not a CSS class: the 4-column grid
 * only exists from `md`, so any class selector would start depending on the test viewport.
 */
function rows(page: import('@playwright/test').Page) {
  return page.getByRole('listitem')
}

test('logs page renders one row per line with the derived source column', async ({ page }) => {
  await page.goto('/#/logs')

  await expect(rows(page)).toHaveCount(4)
  // caller loop.go -> daemon package -> "scheduler"; nyaa.go -> "rss"; anilist.go -> "anilist".
  await expect(page.getByText('scheduler', { exact: true })).toBeVisible()
  await expect(page.getByText('rss', { exact: true })).toBeVisible()
  await expect(page.getByText('anilist', { exact: true })).toBeVisible()
})

test('level pills carry counts and filter the body', async ({ page }) => {
  await page.goto('/#/logs')

  const filters = page.getByRole('group', { name: /filter by level/i })
  await expect(filters.getByRole('button', { name: /^all 4$/i })).toBeVisible()
  await expect(filters.getByRole('button', { name: /^error 1$/i })).toBeVisible()

  await filters.getByRole('button', { name: /^error 1$/i }).click()

  await expect(rows(page)).toHaveCount(1)
  await expect(page.getByText('failed to query AniList')).toBeVisible()
  await expect(page.getByText('check started')).toHaveCount(0)
})

test('search filters the body and highlights the match', async ({ page }) => {
  await page.goto('/#/logs')

  await page.getByPlaceholder(/search/i).fill('torrent')

  // "no torrent for Alpha" and "torrent added".
  await expect(rows(page)).toHaveCount(2)
  await expect(page.locator('mark').first()).toHaveText('torrent')
})

test('search and level filter are kept in the querystring', async ({ page }) => {
  await page.goto('/#/logs')

  await page.getByPlaceholder(/search/i).fill('alpha')
  await page.getByRole('group', { name: /filter by level/i }).getByRole('button', { name: /^warn/i }).click()

  await expect(page).toHaveURL(/level=warn/)
  await expect(page).toHaveURL(/q=alpha/)
})

test('deep link restores the level filter from the URL', async ({ page }) => {
  await page.goto('/#/logs?level=error')

  await expect(rows(page)).toHaveCount(1)
  await expect(
    page.getByRole('group', { name: /filter by level/i }).getByRole('button', { name: /^error/i })
  ).toHaveAttribute('aria-pressed', 'true')
})

test('the follow toggle stays on by default and can be turned off', async ({ page }) => {
  await page.goto('/#/logs')

  const follow = page.getByRole('checkbox', { name: /auto-scroll/i })
  await expect(follow).toBeChecked()

  await follow.uncheck()
  await expect(follow).not.toBeChecked()
})
