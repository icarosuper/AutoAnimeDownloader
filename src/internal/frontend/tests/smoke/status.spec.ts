import { test, expect } from '@playwright/test'
import { makeAnimesResponse, setupStatusPageMocks } from './helpers.js'

test.beforeEach(async ({ page }) => {
  await setupStatusPageMocks(page, 'running')
})

test('status page shows Running badge', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText('Running')).toBeVisible()
})

test('status page shows Stop Daemon button when daemon is running', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('button', { name: /stop daemon/i })).toBeVisible()
})

test('status page shows Force Check button', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('button', { name: /force check/i })).toBeVisible()
})

test('status page shows Start Daemon button when daemon is stopped', async ({ page }) => {
  await setupStatusPageMocks(page, 'stopped')
  await page.goto('/')
  await expect(page.getByText('Stopped')).toBeVisible()
  await expect(page.getByRole('button', { name: /start daemon/i })).toBeVisible()
})

test('status page keeps the go-to-config empty state when there are no animes', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('link', { name: /go to config/i })).toBeVisible()
})

// Status.svelte polls GET /api/v1/torrents to feed the hero speed card (totalSpeeds). Without
// this assertion the card was rendering off an unmocked request that Playwright silently
// failed — the suite passed only because the error was swallowed by loadTorrents' try/catch.
// The hero splits number from unit into two elements (formatSpeedParts), so this asserts the
// two halves rather than the joined "1.0 MB/s" string the pre-redesign card rendered.
test('status page hero renders download speed and the upload summary from torrents data', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText('1.0', { exact: true })).toBeVisible()
  await expect(page.getByText('MB/s', { exact: true })).toBeVisible()
  await expect(page.getByText(/512\.0 KB\/s up/)).toBeVisible()
})

// One ProgressRing per active download, with the episode/ETA meta line beside it.
test('status page hero lists one ring per active download', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('progressbar', { name: 'fixture torrent' })).toBeVisible()
})

test.describe('anime list', () => {
  test.beforeEach(async ({ page }) => {
    await setupStatusPageMocks(page, 'running', makeAnimesResponse())
    await page.goto('/')
  })

  test('renders a derived state chip per anime', async ({ page }) => {
    await expect(page.getByText('Up to date').first()).toBeVisible()
    await expect(page.getByText('5 ep behind').first()).toBeVisible()
    await expect(page.getByText('Blacklisted').first()).toBeVisible()
  })

  test('search filters the list down to matching animes', async ({ page }) => {
    await page.getByPlaceholder(/search animes/i).fill('Beta')
    await expect(page.getByText('Beta Sessions').first()).toBeVisible()
    await expect(page.getByText('Alpha Chronicles')).toHaveCount(0)
    await expect(page.getByText('1 of 3')).toBeVisible()
  })

  test('unwatched filter hides animes that are fully watched', async ({ page }) => {
    await page.getByRole('button', { name: /unwatched/i }).click()
    // Alpha is watched 12/12 released -> filtered out; Beta (1/8) and Gamma (0/5) stay.
    await expect(page.getByText('Alpha Chronicles')).toHaveCount(0)
    await expect(page.getByText('Beta Sessions').first()).toBeVisible()
    await expect(page.getByText('Gamma Protocol').first()).toBeVisible()
  })

  test('name column header sorts the list and toggles direction', async ({ page }) => {
    // The desktop row's name cell is the only `span[title]` inside a row link (the mobile
    // stack uses a <p>), so this selects exactly one element per visible row, in DOM order.
    const names = page.locator('a[href^="#/status/"] span[title]')

    await page.getByRole('button', { name: /^name/i }).click()
    await expect(names).toHaveText(['Alpha Chronicles', 'Beta Sessions', 'Gamma Protocol'])

    await page.getByRole('button', { name: /^name/i }).click()
    await expect(names).toHaveText(['Gamma Protocol', 'Beta Sessions', 'Alpha Chronicles'])
  })

  test('rows link through to the anime detail route', async ({ page }) => {
    await page.getByText('Beta Sessions').first().click()
    await expect(page).toHaveURL(/#\/status\/2$/)
  })
})
