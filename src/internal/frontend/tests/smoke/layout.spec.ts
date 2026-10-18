import { test, expect } from '@playwright/test'
import {
  baseConfig,
  makeAnimesResponse,
  makeTorrentsResponse,
  setupStatusPageMocks,
} from './helpers.js'

/**
 * Layout regressions that only show up with real volume data — none of them breaks a content test,
 * because the element stays in the DOM and "visible", just off the screen.
 *
 * 1. Horizontal overflow. The hero card's "Active downloads" strip has `overflow-x-auto`, but that
 *    wasn't enough: the <section> that holds it is a grid item, and a grid item has
 *    `min-width: auto`, i.e. its floor is the content's intrinsic size. With many downloads the
 *    section grew to max-content (measured: 2494px in a 1280px grid), the strip's scroll never
 *    kicked in and the right column went off the screen. The assertion is about the PAGE
 *    (`documentElement.scrollWidth`), not the strip: it's the symptom the user sees, and it
 *    catches any other column/table that overflows for the same reason.
 *
 * 2. NavRail scrolling with the page. The rail is `h-screen`, but without positioning it was just
 *    a flex item in the normal flow — on a page taller than the viewport it went up with it and
 *    disappeared. The assertion compares the rail's position before and after scrolling to the end.
 */

// 24 active downloads: enough volume for the strip to exceed 4x the screen width.
const manyTorrents = Array.from({ length: 24 }, (_, i) => ({
  hash: `${i}`.padStart(40, 'a'),
  name: `[SubsPlease] Some Very Long Anime Title Here - ${i + 1} (1080p)`,
  anime_name: `Some Very Long Anime Title Here ${i + 1}`,
  status: 'downloading',
  completed: false,
  is_batch: false,
  episode_number: i + 1,
  bytes_completed: 524288000,
  bytes_total: 1073741824,
  bytes_uploaded: 104857600,
  progress: 0.11,
  download_speed: 1048576,
  upload_speed: 524288,
  peers_total: 14,
  eta_seconds: 3600,
  seeded_for_seconds: 0,
}))

// 30 animes with long names/counters: makes the page much taller than the viewport (for the rail
// test) and exercises the list columns' maximum width.
const manyAnimes = Array.from({ length: 30 }, (_, i) => ({
  anime_id: i + 1,
  name: `Smoking Behind the Supermarket with You ${i + 1}`,
  episodes_downloaded: 128,
  episodes_released: 128,
  episodes_watched: 128,
  episodes_pending: 0,
  total_episodes: 128,
  latest_last_download_date: '2026-01-03T00:00:00Z',
  is_blacklisted: false,
}))

async function setupBusyStatusPage(page: import('@playwright/test').Page) {
  // setupStatusPageMocks already covers status/animes/config/torrents/ws.
  await setupStatusPageMocks(page, 'running', makeAnimesResponse(manyAnimes))
  await page.route('**/api/v1/torrents', route =>
    route.fulfill({ json: makeTorrentsResponse(manyTorrents) }),
  )
}

// 1024 is the smallest viewport where the anime list table is rendered (below that Status.svelte
// switches to stacked cards); 1280 is the chromium project's default.
for (const width of [1024, 1280, 1600]) {
  test(`status page does not overflow horizontally at ${width}px with many active downloads`, async ({
    page,
  }) => {
    await setupBusyStatusPage(page)
    await page.setViewportSize({ width, height: 800 })
    await page.goto('/')
    await expect(page.getByRole('progressbar').first()).toBeVisible()

    const { scrollWidth, clientWidth } = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }))
    expect(scrollWidth, `page scrolls horizontally at ${width}px`).toBeLessThanOrEqual(clientWidth)
  })
}

test('the active downloads strip scrolls instead of pushing the card past the viewport', async ({
  page,
}) => {
  await setupBusyStatusPage(page)
  await page.goto('/')

  const strip = page.getByRole('progressbar').first().locator('xpath=ancestor::div[2]')
  const { clientWidth, scrollWidth } = await strip.evaluate((el: HTMLElement) => ({
    clientWidth: el.clientWidth,
    scrollWidth: el.scrollWidth,
  }))

  // The strip is narrower than its content — i.e. it became a real scroll container, instead of
  // having pushed the section to the content's size.
  expect(scrollWidth).toBeGreaterThan(clientWidth)
  expect(clientWidth).toBeLessThan(1280)
})

/**
 * The three screens with a table have FIXED-width tracks (Status LIST_GRID, Downloads ROW_GRID,
 * AnimeDetail EP_GRID). Each one adds up to more than the usable width at `md` (768px), so all of
 * them switch to cards/flex-wrap below `lg`. This test is what holds that cut in place: moving any
 * of them back down to `md`, or adding a fixed column, throws the page into horizontal scrolling
 * again — which is exactly the reported symptom, and no content test catches it, because the
 * elements stay in the DOM and "visible", just off the screen.
 */
test.describe('no screen scrolls horizontally', () => {
  const detail = {
    success: true,
    data: {
      anime_id: 1,
      anilist_id: 21,
      total_episodes: 12,
      progress: 1,
      status: 'CURRENT',
      cover_image: null,
      episodes: Array.from({ length: 8 }, (_, i) => ({
        episode_number: i + 1,
        airing_at: 0,
        time_until_airing: 0,
        is_aired: true,
        is_watched: i < 2,
        is_downloaded: i < 3,
        download_date: '2026-01-01T00:00:00Z',
        is_blacklisted: false,
      })),
    },
  }

  for (const route of ['/#/status', '/#/downloads', '/#/config', '/#/logs', '/#/status/1']) {
    for (const width of [768, 1024, 1280, 1440]) {
      test(`${route} at ${width}px`, async ({ page }) => {
        await setupBusyStatusPage(page)
        await page.route('**/api/v1/animes/1', r => r.fulfill({ json: detail }))
        await page.route('**/api/v1/logs**', r =>
          r.fulfill({
            json: {
              success: true,
              data: {
                lines: [
                  '2026-01-01T00:00:00Z INF daemon/checker.go:120 > checking anilist for user testuser',
                ],
              },
            },
          }),
        )
        await page.setViewportSize({ width, height: 900 })
        await page.goto(route)
        await page.waitForTimeout(400)

        const { scrollWidth, clientWidth } = await page.evaluate(() => ({
          scrollWidth: document.documentElement.scrollWidth,
          clientWidth: document.documentElement.clientWidth,
        }))
        expect(scrollWidth, `${route} scrolls horizontally at ${width}px`).toBeLessThanOrEqual(
          clientWidth,
        )
      })
    }
  }
})

/**
 * The MoreMenu panel is `absolute z-50`, but its z-index only counts INSIDE the rail: `position:
 * sticky` always creates a stacking context, regardless of z-index. Without a z-index on the <nav>,
 * the whole rail (panel included) paints in the root's z-auto layer, in tree order — and any
 * POSITIONED element that comes later in the DOM beats it. That's the case of daisyUI's `.card`
 * (position: relative), used on the Priorities screen: the cards showed up in front of the menu.
 *
 * The assertion uses elementFromPoint at the center of a menu item — it's the "who's in front" test
 * the user does with the click, without depending on the actionability timeout.
 */
const samplePriorities = {
  criteria_order: ['uncensored', 'source', 'resolution'],
  fansubs: ['subsplease'],
  resolutions: ['1080p', '720p'],
  sources: ['bd', 'web'],
  codecs: ['h265'],
  audio: ['jpn'],
  ignore_list: ['dub'],
}

test('the More menu paints above the page content', async ({ page }) => {
  await setupBusyStatusPage(page)
  // Priorities dies on load without these two: `config.priorities` (absent from baseConfig) and the
  // defaults GET. Without them there would be no card on the screen and the test would pass
  // vacuously.
  await page.route('**/api/v1/config', route =>
    route.fulfill({ json: { success: true, data: { ...baseConfig.data, priorities: samplePriorities } } }),
  )
  await page.route('**/api/v1/config/priorities/defaults', route =>
    route.fulfill({ json: { success: true, data: samplePriorities } }),
  )
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.goto('/#/priorities')

  // The cards really exist — they're what has to stay BEHIND the menu. `exact` because the screen
  // also has an sr-only label "Use uncensored", and without it the locator matches two elements.
  await expect(page.getByText('uncensored', { exact: true })).toBeVisible()

  const rail = page.getByRole('navigation', { name: 'Main' })
  await rail.getByRole('button', { name: /^more$/i }).click()

  const item = page.getByRole('menuitem', { name: /^logs$/i })
  await expect(item).toBeVisible()

  const coveredBy = await item.evaluate((el: HTMLElement) => {
    const rect = el.getBoundingClientRect()
    const top = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2)
    if (!top || el.contains(top) || top.contains(el)) return null
    return `${top.tagName}.${top.className}`
  })
  expect(coveredBy, 'page content paints over the More menu').toBeNull()

  // The backdrop (`fixed inset-0 z-40`) lives inside the rail, i.e. inside the z-30 context — a
  // click over a card still has to land on it and close the menu. `page.mouse` and not
  // `locator.click()`: the card's locator would be refused precisely for being covered by the
  // backdrop, which is the wanted behavior.
  const cardPoint = await page.getByText('uncensored', { exact: true }).evaluate((el: HTMLElement) => {
    const rect = el.getBoundingClientRect()
    return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 }
  })
  await page.mouse.click(cardPoint.x, cardPoint.y)
  await expect(item).toHaveCount(0)
})

/**
 * The counterpart of the rail's z-index: what has to stay ABOVE it still does. Modal/Toasts are
 * `fixed z-50` at the root — higher than the rail's 30 —, but that only holds while none of their
 * ancestors becomes a stacking context; the assertion is about who paints in front, not about the
 * number.
 */
test('a modal still paints above the nav rail', async ({ page }) => {
  await setupBusyStatusPage(page)
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.goto('/#/downloads')

  await page.getByRole('button', { name: /^delete — /i }).first().click()

  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()

  // The dialog is centered in the viewport, so its left edge falls over the rail's strip.
  const coveredBy = await dialog.evaluate((el: HTMLElement) => {
    const rect = el.getBoundingClientRect()
    const top = document.elementFromPoint(rect.x + 4, rect.y + rect.height / 2)
    if (!top || el.contains(top) || top.contains(el)) return null
    return `${top.tagName}.${top.className}`
  })
  expect(coveredBy, 'something paints over the delete dialog').toBeNull()
})

/**
 * Downloads header on a narrow screen. The title block is `flex-1`, i.e. flex-basis 0% — and a
 * `flex-wrap` container's line breaking uses each item's HYPOTHETICAL main size, which with basis 0
 * is ~0. Result: the speed box never went to the line below, the title block got the leftovers
 * (~30px in the user's screenshot) and the subtitle became one word per line. The floor comes from
 * `min-w-`, which GOES INTO that hypothetical size — that's why the fix is a minimum width and not
 * a breakpoint: an `sm:flex-row` would fix 375px and leave 640px still squeezed.
 *
 * The assertion counts real lines (one rect per line box via Range), not height in px.
 */
for (const width of [375, 414, 500, 640, 768, 900, 1280]) {
  test(`the downloads header subtitle is not squeezed to one word per line at ${width}px`, async ({
    page,
  }) => {
    await setupBusyStatusPage(page)
    await page.setViewportSize({ width, height: 800 })
    await page.goto('/#/downloads')

    const subtitle = page.getByText('Live progress of the embedded torrent client')
    await expect(subtitle).toBeVisible()

    const lines = await subtitle.evaluate((el: HTMLElement) => {
      const range = document.createRange()
      range.selectNodeContents(el)
      return range.getClientRects().length
    })
    expect(lines, `subtitle wraps into ${lines} lines at ${width}px`).toBeLessThanOrEqual(2)
  })
}

/**
 * The Downloads filter pills lived in an `overflow-x-auto` strip: on a narrow screen the last ones
 * ("Seeding", "Problems") stayed out of view behind a horizontal scroll, and the count they exist
 * to show — "is there something there?" — only showed up after dragging. Now the strip wraps to the
 * line below, so ALL of them are always visible.
 */
for (const width of [375, 414, 640]) {
  test(`every downloads filter pill is visible without horizontal scroll at ${width}px`, async ({
    page,
  }) => {
    await setupBusyStatusPage(page)
    await page.setViewportSize({ width, height: 800 })
    await page.goto('/#/downloads')

    const pills = ['all', 'downloading', 'seeding', 'problems'].map(name =>
      page.getByRole('button', { name: new RegExp(`^${name} \\d+$`, 'i') }),
    )

    for (const pill of pills) {
      await expect(pill).toBeVisible()
      // ratio 1: the WHOLE pill in the viewport, not an edge of it.
      await expect(pill).toBeInViewport({ ratio: 1 })
    }

    // And the strip isn't a scroll container: nothing hidden to the side.
    const scrolls = await pills[0].evaluate((el: HTMLElement) => {
      const row = el.parentElement as HTMLElement
      return row.scrollWidth > row.clientWidth
    })
    expect(scrolls, 'the filter pill row still scrolls horizontally').toBe(false)
  })
}

test('the nav rail stays pinned to the top of the viewport while the page scrolls', async ({
  page,
}) => {
  await setupBusyStatusPage(page)
  await page.goto('/')

  const rail = page.getByRole('navigation', { name: 'Main' })
  await expect(rail).toBeVisible()

  const before = await rail.boundingBox()
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))
  // Checks the page really scrolled; otherwise the test would pass vacuously.
  expect(await page.evaluate(() => window.scrollY)).toBeGreaterThan(200)
  const after = await rail.boundingBox()

  expect(before?.y).toBe(0)
  expect(after?.y).toBe(0)
  await expect(rail.getByRole('link', { name: /status/i })).toBeInViewport()
})

/**
 * Config index on mobile. The same regression as the Downloads filter pills, the same remedy: the
 * strip had 358px of space for 644px of content at 390px, so "Torrent search" and the two exit
 * links were behind a sideways drag. `flex-wrap` puts everything on screen.
 *
 * The assertion is double on purpose: "every item whole in the viewport" catches the hidden item,
 * and "the strip isn't a scroll container" catches `overflow-x-auto` coming back even if by luck
 * the items fit in a short language.
 */
for (const width of [375, 390, 414]) {
  test(`every config group and exit link is visible without horizontal scroll at ${width}px`, async ({
    page,
  }) => {
    await page.route('**/api/v1/config', route => route.fulfill({ json: baseConfig }))
    await page.route('**/api/v1/ws', route => route.abort())
    await page.setViewportSize({ width, height: 844 })
    await page.goto('/#/config')

    const index = page.getByRole('navigation', { name: /configuration sections/i })
    await expect(index).toBeVisible()

    const items = [
      index.getByRole('button', { name: /^library$/i }),
      index.getByRole('button', { name: /^lists$/i }),
      index.getByRole('button', { name: /^downloads$/i }),
      index.getByRole('button', { name: /^torrent search$/i }),
      index.getByRole('link', { name: /priorities/i }),
      index.getByRole('link', { name: /notifications/i }),
    ]

    for (const item of items) {
      // ratio 1: the WHOLE item in the viewport, not an edge of it.
      await expect(item).toBeInViewport({ ratio: 1 })
    }

    const scrolls = await index.evaluate((el: HTMLElement) => el.scrollWidth > el.clientWidth)
    expect(scrolls, 'the config index still scrolls horizontally').toBe(false)
  })
}

/**
 * Daemon actions in the Status header. The row was `flex shrink-0 gap-2`: it could neither shrink
 * NOR wrap, so its width was the sum of the three buttons forever — 358px in English, 438px in
 * pt-BR — and on a narrow screen "Stop Daemon" went off the right edge.
 *
 * Parametrized by LOCALE on purpose. The rest of the suite runs only in `en`, the default, and
 * that's exactly what let the bug through: in English the labels are ~80px shorter and the row only
 * overflowed below 375px, outside the tested widths. pt-BR is the longer of the two languages; a
 * width test that only runs in `en` measures the easy case.
 */
for (const locale of ['en', 'pt-BR'] as const) {
  for (const width of [320, 375, 390, 414] as const) {
    test(`status header daemon actions stay on screen at ${width}px in ${locale}`, async ({
      page,
    }) => {
      await page.addInitScript(l => localStorage.setItem('PARAGLIDE_LOCALE', l), locale)
      await setupStatusPageMocks(page, 'running')
      await page.setViewportSize({ width, height: 844 })
      await page.goto('/#/status')

      // Scoped to the header row: "+ Add anime" also exists in the list's empty state and in the
      // mobile NavTabBar, so the accessible name alone matches three elements. "Force check"
      // ("Forçar verificação") only exists here, and serves as the anchor to reach the row.
      const row = page
        .getByRole('button', { name: /forçar verificação|force check/i })
        .locator('..')

      const actions = [
        row.getByRole('link', { name: /adicionar anime|add anime/i }),
        row.getByRole('button', { name: /forçar verificação|force check/i }),
        row.getByRole('button', { name: /parar daemon|stop daemon/i }),
      ]
      for (const action of actions) {
        // ratio 1: the WHOLE button in the viewport, not an edge of it.
        await expect(action).toBeInViewport({ ratio: 1 })
      }

      // And the page didn't gain a horizontal scroll — it's the symptom the user sees.
      const over = await page.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
      )
      expect(over, 'the page scrolls horizontally').toBe(false)
    })
  }
}
