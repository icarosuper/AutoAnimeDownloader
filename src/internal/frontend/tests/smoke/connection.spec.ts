import { test, expect } from '@playwright/test'
import { setupStatusPageMocks } from './helpers.js'

// The connection dot sits in the rail on every screen. It used to read a socket that only
// Status and Downloads opened (and closed on leave), so any other screen showed it red —
// "Disconnected" — with the daemon perfectly fine.

const CONNECTED = '[data-tip="Real-time updates active"]'

test.beforeEach(async ({ page }) => {
  await setupStatusPageMocks(page)
  // An accepted socket that never says anything: the app only needs it to open.
  await page.routeWebSocket('**/api/v1/ws', () => {})
})

test('landing straight on a screen without its own socket shows the dot connected', async ({ page }) => {
  await page.goto('/#/config')
  await expect(page.locator(CONNECTED)).toBeVisible()
})

test('leaving Status keeps the dot connected', async ({ page }) => {
  await page.goto('/#/status')
  await expect(page.locator(CONNECTED)).toBeVisible()
  await page.getByRole('link', { name: 'Configuration' }).click()
  await expect(page).toHaveURL(/#\/config/)
  await expect(page.locator(CONNECTED)).toBeVisible()
})
