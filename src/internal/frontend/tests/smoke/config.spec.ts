import { test, expect, type Page } from '@playwright/test'
import { baseConfig } from './helpers.js'

/**
 * The screen draws ONE group at a time, so a field only exists in the DOM while its group is open —
 * hence `openGroup` before almost every assertion. The four groups are Library, Lists, Downloads
 * and Torrent search; the initial one is Library, because it's where the only required field lives
 * and where `?missingConfig=true` sends the user.
 *
 * The tests run in `en` (the default locale), so the labels here are those of `messages/en.json`.
 */

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/config', route => route.fulfill({ json: baseConfig }))
  await page.route('**/api/v1/ws', route => route.abort())
})

function sideIndex(page: Page) {
  return page.getByRole('navigation', { name: /configuration sections/i })
}

async function openGroup(page: Page, name: RegExp) {
  await sideIndex(page).getByRole('button', { name }).click()
}

/**
 * Routes the PUT into the test and returns the saved body.
 *
 * Wait for `whenSaved`, not for a `page.waitForRequest`: that one resolves on the request EVENT,
 * which can arrive before the route handler runs — under load (the suite runs in parallel workers)
 * it gave `saved.body === undefined` and the assertion failed intermittently.
 */
async function captureSave(page: Page) {
  const saved: { body?: Record<string, unknown>; whenSaved: Promise<void> } = {
    whenSaved: Promise.resolve(),
  }
  let markSaved: () => void = () => {}
  saved.whenSaved = new Promise<void>(resolve => {
    markSaved = resolve
  })
  await page.route('**/api/v1/config', async route => {
    if (route.request().method() === 'PUT') {
      saved.body = route.request().postDataJSON()
      markSaved()
      await route.fulfill({ json: { success: true, data: null } })
    } else {
      await route.fulfill({ json: baseConfig })
    }
  })
  return saved
}

test('config page loads and shows Configuration heading', async ({ page }) => {
  await page.goto('/#/config')
  await expect(page.getByRole('heading', { name: /configuration/i })).toBeVisible()
})

test('config page shows Save button after loading', async ({ page }) => {
  await page.goto('/#/config')
  await expect(page.getByRole('button', { name: /^save$/i })).toBeVisible()
})

test('clicking Save calls PUT /config', async ({ page }) => {
  const saved = await captureSave(page)

  await page.goto('/#/config')
  await expect(page.getByRole('button', { name: /^save$/i })).toBeVisible()

  await page.getByRole('button', { name: /^save$/i }).click()
  await saved.whenSaved

  expect(saved.body).toBeDefined()
})

test('Library is the initial group and holds the required path', async ({ page }) => {
  await page.goto('/#/config')

  await expect(page.getByLabel(/completed anime path/i)).toHaveValue('/completed')
  await expect(page.getByLabel(/check interval/i)).toHaveCount(0)
})

/**
 * One test per group. It's the only way a field could get lost in this change: Save sends the whole
 * `config` by spread, so the PAYLOAD is not at risk — the risk is a field becoming invisible for
 * not having been moved along with its group.
 */
test('each group holds its own fields', async ({ page }) => {
  await page.goto('/#/config')

  await expect(page.getByLabel(/completed anime path/i)).toBeVisible()
  await expect(page.getByLabel(/rename files to a standard format/i)).toBeVisible()
  await expect(page.getByLabel(/min free disk space/i)).toBeVisible()

  await openGroup(page, /^lists$/i)
  await expect(page.getByLabel(/anilist usernames/i)).toBeVisible()
  await expect(page.getByLabel(/excluded list/i)).toBeVisible()
  await expect(page.getByRole('group', { name: /^download statuses$/i })).toBeVisible()
  await expect(page.getByRole('group', { name: /airing statuses to download/i })).toBeVisible()
  await expect(page.getByRole('group', { name: /^delete statuses$/i })).toBeVisible()

  await openGroup(page, /^downloads$/i)
  await expect(page.getByLabel(/check interval/i)).toBeVisible()
  await expect(page.getByLabel(/max concurrent downloads/i)).toBeVisible()
  await expect(page.getByLabel(/max episodes per anime/i)).toBeVisible()
  await expect(page.getByLabel(/delete watched episodes/i)).toBeVisible()

  await openGroup(page, /torrent search/i)
  await expect(page.getByLabel(/minimum seeders/i)).toBeVisible()
  await expect(page.getByLabel(/max search pages/i)).toBeVisible()
  await expect(page.getByLabel(/episode retry limit/i)).toBeVisible()
  await expect(page.getByLabel(/max batch torrent size/i)).toBeVisible()
  await expect(page.getByLabel(/max episode torrent size/i)).toBeVisible()
})

test('the side index swaps which group is on screen', async ({ page }) => {
  await page.goto('/#/config')
  await expect(sideIndex(page)).toBeVisible()

  await expect(page.getByLabel(/completed anime path/i)).toBeVisible()
  await expect(page.getByLabel(/check interval/i)).toHaveCount(0)

  await openGroup(page, /^downloads$/i)

  await expect(page.getByLabel(/check interval/i)).toHaveValue('60')
  await expect(page.getByLabel(/completed anime path/i)).toHaveCount(0)
})

test('the chips input adds a value with Enter and it reaches the PUT body', async ({ page }) => {
  const saved = await captureSave(page)
  await page.goto('/#/config')
  await openGroup(page, /^lists$/i)

  const usernames = page.getByLabel(/anilist usernames/i)
  await usernames.fill('seconduser')
  await usernames.press('Enter')

  await expect(page.getByRole('button', { name: 'Remove seconduser' })).toBeVisible()
  await expect(usernames).toHaveValue('')

  await page.getByRole('button', { name: /^save$/i }).click()
  await saved.whenSaved

  expect(saved.body?.anilist_usernames).toEqual(['testuser', 'seconduser'])
})

test('MyAnimeList usernames reach the PUT body in their own field', async ({ page }) => {
  const saved = await captureSave(page)
  await page.goto('/#/config')
  await openGroup(page, /^lists$/i)

  const malUsernames = page.getByLabel(/myanimelist usernames/i)
  await malUsernames.fill('maluser')
  await malUsernames.press('Enter')

  await page.getByRole('button', { name: /^save$/i }).click()
  await saved.whenSaved

  expect(saved.body?.mal_usernames).toEqual(['maluser'])
  expect(saved.body?.anilist_usernames).toEqual(['testuser'])
})

test('status pills toggle and download/delete stay mutually exclusive', async ({ page }) => {
  const saved = await captureSave(page)
  await page.goto('/#/config')
  await openGroup(page, /^lists$/i)

  const downloadGroup = page.getByRole('group', { name: /^download statuses$/i })
  const deleteGroup = page.getByRole('group', { name: /^delete statuses$/i })

  await expect(downloadGroup.getByRole('button', { name: 'Watching', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  await deleteGroup.getByRole('button', { name: 'Watching', exact: true }).click()

  await expect(deleteGroup.getByRole('button', { name: 'Watching', exact: true })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  await expect(downloadGroup.getByRole('button', { name: 'Watching', exact: true })).toHaveAttribute(
    'aria-pressed',
    'false'
  )

  await page.getByRole('button', { name: /^save$/i }).click()
  await saved.whenSaved

  expect(saved.body?.delete_statuses).toEqual(['CURRENT'])
  expect(saved.body?.download_statuses).toEqual(['REPEATING'])
})

/**
 * With one group at a time, a toast saying "the folder is required" while the user looks at another
 * group couldn't be acted on — the failing validation brings its group to the screen.
 */
test('a failing validation brings its group to screen and blocks the PUT', async ({ page }) => {
  let putCalls = 0
  await page.route('**/api/v1/config', async route => {
    if (route.request().method() === 'PUT') {
      putCalls++
      await route.fulfill({ json: { success: true, data: null } })
    } else {
      await route.fulfill({ json: { success: true, data: { ...baseConfig.data, completed_anime_path: '' } } })
    }
  })

  await page.goto('/#/config')
  await openGroup(page, /torrent search/i)
  await expect(page.getByLabel(/minimum seeders/i)).toBeVisible()

  await page.getByRole('button', { name: /^save$/i }).click()

  await expect(page.getByText(/completed anime path is required/i)).toBeVisible()
  await expect(page.getByLabel(/completed anime path/i)).toBeVisible()
  expect(putCalls).toBe(0)
})

/**
 * Pending marks. Both marks — the `*` on the field and the dot in the index — derive from the SAME
 * `requiredChecks` list that blocks Save; that's what keeps the dot from lying when a rule changes.
 * `check_interval: 0` fails in Downloads, the empty folder fails in Library, and the other two
 * groups stay clean — the dot is per group, not global.
 */
const incompleteConfig = {
  success: true,
  data: { ...baseConfig.data, completed_anime_path: '', check_interval: 0 },
}

test('required fields are marked and the side index flags the groups still missing data', async ({
  page,
}) => {
  await page.route('**/api/v1/config', route => route.fulfill({ json: incompleteConfig }))
  await page.goto('/#/config')

  // A DOM property, not an attribute: `required` is a boolean, and asserting the attribute's empty
  // string depends on a serialization detail instead of the field's state.
  await expect(page.getByLabel(/completed anime path/i)).toHaveJSProperty('required', true)
  await expect(page.getByText(/required field/i)).toBeVisible()

  const index = sideIndex(page)
  await expect(index.getByRole('button', { name: /library.*missing/i })).toBeVisible()
  await expect(index.getByRole('button', { name: /downloads.*missing/i })).toBeVisible()
  await expect(index.getByRole('button', { name: /^lists$/i })).toBeVisible()
  await expect(index.getByRole('button', { name: /^torrent search$/i })).toBeVisible()
})

test('the missing mark clears as soon as the field is filled', async ({ page }) => {
  await page.route('**/api/v1/config', route => route.fulfill({ json: incompleteConfig }))
  await page.goto('/#/config')

  const index = sideIndex(page)
  await expect(index.getByRole('button', { name: /library.*missing/i })).toBeVisible()

  await page.getByLabel(/completed anime path/i).fill('/completed')

  await expect(index.getByRole('button', { name: /library.*missing/i })).toHaveCount(0)
  await expect(index.getByRole('button', { name: /downloads.*missing/i })).toBeVisible()
})

/**
 * Priorities and Notifications write to the same PUT /config but are screens of their own. The
 * index exposes them as LINKS (not groups): they navigate instead of switching `activeGroup`, and
 * that's why they are <a>.
 */
test('the index links navigate to Priorities and Notifications', async ({ page }) => {
  await page.goto('/#/config')

  await sideIndex(page).getByRole('link', { name: /priorities/i }).click()
  await expect(page).toHaveURL(/#\/priorities$/)

  await page.goto('/#/config')
  await sideIndex(page).getByRole('link', { name: /notifications/i }).click()
  await expect(page).toHaveURL(/#\/notifications$/)
})

// `?group=` is the target of the "Connect your list" button of the first-steps card (Status
// screen). Validation uses the `groups` array the screen already builds, so an unknown id can't
// leave the screen on a group that renders nothing — it falls back to the default `library`.
test('?group=anilist opens the screen on the Lists group', async ({ page }) => {
  await page.goto('/#/config?group=anilist')

  await expect(page.getByLabel(/anilist usernames/i)).toBeVisible()
  await expect(page.getByLabel(/completed anime path/i)).toHaveCount(0)
})

test('?group= with an unknown value falls back to the Library group', async ({ page }) => {
  await page.goto('/#/config?group=naoexiste')

  await expect(page.getByLabel(/completed anime path/i)).toBeVisible()
})

// Both parameters come from the SAME URLSearchParams — each branch used to read only
// `missingConfig` and the first one did `return`, so reading both together is exactly what could
// have regressed.
test('?missingConfig=true&group=downloads shows the banner AND opens the Downloads group', async ({ page }) => {
  await page.goto('/#/config?missingConfig=true&group=downloads')

  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByLabel(/check interval/i)).toBeVisible()
})
