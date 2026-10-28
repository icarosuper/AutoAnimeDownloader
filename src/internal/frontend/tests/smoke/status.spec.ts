import { test, expect, type Page } from '@playwright/test'
import { baseConfig, makeAnimesResponse, setupStatusPageMocks } from './helpers.js'

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

  // "4 ep behind", not 5: the segment comes from `episodes_pending` (4 in the fixture), not
  // from the subtraction `episodes_released - episodes_downloaded` (8-3=5) — decisions.md #88.
  test('renders a derived state chip per anime', async ({ page }) => {
    await expect(page.getByText('Up to date').first()).toBeVisible()
    await expect(page.getByText('4 ep behind').first()).toBeVisible()
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

  test('airing type filter keeps releasing titles and All restores the table', async ({ page }) => {
    await page.getByRole('radio', { name: /airing/i }).click()
    await expect(page.getByText('Beta Sessions').first()).toBeVisible()
    await expect(page.getByText('Alpha Chronicles')).toHaveCount(0)
    await expect(page.getByText('Gamma Protocol')).toHaveCount(0)

    await page.getByRole('radio', { name: /^all /i }).click()
    await expect(page.getByText('Alpha Chronicles').first()).toBeVisible()
    await expect(page.getByText('Gamma Protocol').first()).toBeVisible()
  })

  test('hash restores type, unwatched and search', async ({ page }) => {
    await page.goto('/#/status?type=airing&unwatched=1&q=Beta')
    await expect(page.getByPlaceholder(/search animes/i)).toHaveValue('Beta')
    await expect(page.getByRole('button', { name: /unwatched/i })).toHaveAttribute('aria-pressed', 'true')
    await expect(page.getByRole('radio', { name: /airing/i })).toHaveAttribute('aria-checked', 'true')
    await expect(page.getByText('Beta Sessions').first()).toBeVisible()
    await expect(page.getByText('Alpha Chronicles')).toHaveCount(0)
  })

  test('standalone is a type option and the old chip is gone', async ({ page }) => {
    await expect(page.getByRole('radio', { name: /standalone/i })).toBeVisible()
    await expect(page.getByRole('button', { name: /standalone only/i })).toHaveCount(0)
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

test.describe('type filter hybrid control', () => {
  test.describe('desktop', () => {
    test.use({ viewport: { width: 1280, height: 720 } })

    test('shows the radiogroup and hides the type menu trigger', async ({ page }) => {
      await setupStatusPageMocks(page, 'running', makeAnimesResponse())
      await page.goto('/')
      await expect(page.getByRole('radiogroup', { name: /filter by type/i })).toBeVisible()
      await expect(page.getByRole('button', { name: /^(all|airing) /i })).toHaveCount(0)
    })

    test('keeps every type radio inside the animes card', async ({ page }) => {
      await page.addInitScript(() => localStorage.setItem('PARAGLIDE_LOCALE', 'pt-BR'))
      await setupStatusPageMocks(page, 'running', makeAnimesResponse())
      await page.goto('/')
      const card = page.locator('section').filter({ has: page.getByRole('heading', { name: /^animes$/i }) })
      const cardBox = await card.boundingBox()
      expect(cardBox).toBeTruthy()
      const radios = page.getByRole('radio')
      await expect(radios).toHaveCount(5)
      for (let i = 0; i < 5; i++) {
        const box = await radios.nth(i).boundingBox()
        expect(box, `radio ${i} has a box`).toBeTruthy()
        expect(box!.x, `radio ${i} left`).toBeGreaterThanOrEqual(cardBox!.x - 1)
        expect(box!.x + box!.width, `radio ${i} right`).toBeLessThanOrEqual(cardBox!.x + cardBox!.width + 1)
      }
    })
  })

  // Tightest width where the table (and the segmented type control) is on: Portuguese
  // labels are the long ones, and this is the screenshot that was clipping Blacklist.
  test.describe('desktop tight', () => {
    test.use({ viewport: { width: 1024, height: 720 } })

    test('keeps every type radio inside the animes card in pt-BR', async ({ page }) => {
      await page.addInitScript(() => localStorage.setItem('PARAGLIDE_LOCALE', 'pt-BR'))
      await setupStatusPageMocks(page, 'running', makeAnimesResponse())
      await page.goto('/')
      const card = page.locator('section').filter({ has: page.getByRole('heading', { name: /^animes$/i }) })
      const cardBox = await card.boundingBox()
      expect(cardBox).toBeTruthy()
      const radios = page.getByRole('radio')
      await expect(radios).toHaveCount(5)
      for (let i = 0; i < 5; i++) {
        const box = await radios.nth(i).boundingBox()
        expect(box, `radio ${i} has a box`).toBeTruthy()
        expect(box!.x, `radio ${i} left`).toBeGreaterThanOrEqual(cardBox!.x - 1)
        expect(box!.x + box!.width, `radio ${i} right`).toBeLessThanOrEqual(cardBox!.x + cardBox!.width + 1)
      }
    })
  })

  // 800px is past `md` (768) but still in the card list (`lg` is 1024). The type
  // control follows that cut: a five-segment radio does not fit beside search
  // once the rail is on, so this width uses the ActionMenu like the phone layout.
  test.describe('tablet', () => {
    test.use({ viewport: { width: 800, height: 720 } })

    test('shows the type menu trigger and hides the radiogroup', async ({ page }) => {
      await setupStatusPageMocks(page, 'running', makeAnimesResponse())
      await page.goto('/')
      await expect(page.getByRole('radiogroup', { name: /filter by type/i })).toBeHidden()
      await expect(page.getByRole('button', { name: /^all /i })).toBeVisible()
    })
  })

  test.describe('narrow', () => {
    test.use({ viewport: { width: 700, height: 720 } })

    test('shows the type menu trigger and hides the radiogroup', async ({ page }) => {
      await setupStatusPageMocks(page, 'running', makeAnimesResponse())
      await page.goto('/')
      await expect(page.getByRole('radiogroup', { name: /filter by type/i })).toBeHidden()
      await expect(page.getByRole('button', { name: /^all /i })).toBeVisible()
    })

    test('keeps the clear X on the same row as N of M', async ({ page }) => {
      await setupStatusPageMocks(page, 'running', makeAnimesResponse())
      await page.goto('/')
      await page.getByRole('button', { name: /unwatched/i }).click()
      const count = page.getByText(/of 3/)
      const clear = page.getByRole('button', { name: /clear filters/i })
      const countBox = await count.boundingBox()
      const clearBox = await clear.boundingBox()
      expect(countBox, 'count has a box').toBeTruthy()
      expect(clearBox, 'clear X has a box').toBeTruthy()
      const countMid = countBox!.y + countBox!.height / 2
      const clearMid = clearBox!.y + clearBox!.height / 2
      expect(Math.abs(countMid - clearMid)).toBeLessThan(countBox!.height)
      expect(clearBox!.x).toBeGreaterThan(countBox!.x)
    })
  })
})

/**
 * First steps. `setupStatusPageMocks` already delivers the three GREEN items (folder
 * `/completed`, `anilist_usernames: ['testuser']` and a 2026 `last_check`), so the card
 * does not appear in any of the tests above — and that is what the second test here locks.
 *
 * Routes are re-registered: Playwright matches the LAST registered route, so these override
 * the ones from the file-top `beforeEach`.
 */
test.describe('onboarding card', () => {
  async function setupFreshInstall(page: Page) {
    await page.route('**/api/v1/config', route =>
      route.fulfill({
        json: {
          success: true,
          data: { ...baseConfig.data, completed_anime_path: '', anilist_usernames: [] },
        },
      }),
    )
    // `0001-01-01T00:00:00Z` is Go's time.Time zero-value: a daemon that never ran.
    await page.route('**/api/v1/status', route =>
      route.fulfill({
        json: {
          success: true,
          data: {
            status: 'stopped',
            last_check: '0001-01-01T00:00:00Z',
            has_error: false,
            version: '1.0.0',
          },
        },
      }),
    )
  }

  test('a fresh install shows the card with the three steps', async ({ page }) => {
    await setupFreshInstall(page)
    await page.goto('/')

    const card = page.getByTestId('onboarding-card')
    await expect(card).toBeVisible()
    await expect(card.getByRole('listitem')).toHaveCount(3)
  })

  // The complaint that motivated marking-by-hand: on a fresh install steps 1 and 3 started
  // green (the folder has a default and the pass runs on its own), and the tutorial looked
  // half-done before the user had read anything.
  test('the three steps start unchecked and checking all three hides the card', async ({ page }) => {
    await setupFreshInstall(page)
    await page.goto('/')

    const boxes = page.getByTestId('onboarding-card').getByRole('checkbox')
    await expect(boxes).toHaveCount(3)
    for (let i = 0; i < 3; i++) await expect(boxes.nth(i)).not.toBeChecked()

    // `click()` and not `check()`: checking the third makes the card vanish, and `check()`
    // sits waiting to confirm the state of an element that already left the DOM.
    for (let i = 0; i < 3; i++) await boxes.nth(i).click()
    await expect(page.getByTestId('onboarding-card')).toHaveCount(0)

    await page.reload()
    await expect(page.getByRole('heading', { name: /status/i })).toBeVisible()
    await expect(page.getByTestId('onboarding-card')).toHaveCount(0)
  })

  test('vanishes when folder, source and first check are ready', async ({ page }) => {
    await setupStatusPageMocks(page, 'running', makeAnimesResponse())
    await page.goto('/')

    await expect(page.getByRole('heading', { name: /status/i })).toBeVisible()
    await expect(page.getByTestId('onboarding-card')).toHaveCount(0)
  })

  test('"Don\'t show again" hides the card, and the dismissal survives a reload', async ({ page }) => {
    await setupFreshInstall(page)
    await page.goto('/')

    await page.getByRole('button', { name: /don't show again/i }).click()
    await expect(page.getByTestId('onboarding-card')).toHaveCount(0)

    await page.reload()
    await expect(page.getByRole('heading', { name: /status/i })).toBeVisible()
    await expect(page.getByTestId('onboarding-card')).toHaveCount(0)
  })

  // The way back. The config route is already mocked by `setupFreshInstall`, so the
  // Settings screen loads the same.
  test('Settings brings the card back with "Show again"', async ({ page }) => {
    await setupFreshInstall(page)
    await page.goto('/')

    await page.getByRole('button', { name: /don't show again/i }).click()
    await expect(page.getByTestId('onboarding-card')).toHaveCount(0)

    await page.goto('/#/config')
    const restore = page.getByRole('button', { name: /show again/i })
    await expect(restore).toBeEnabled()
    await restore.click()
    await expect(restore).toBeDisabled()

    await page.goto('/#/')
    await expect(page.getByTestId('onboarding-card')).toBeVisible()
  })

  test('"Show again" starts disabled when there is nothing to restore', async ({ page }) => {
    await setupFreshInstall(page)
    await page.goto('/#/config')

    await expect(page.getByRole('button', { name: /show again/i })).toBeDisabled()
  })
})
