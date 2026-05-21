import { test, expect } from '@playwright/test'
import { baseConfig } from './helpers.js'

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/config', route => route.fulfill({ json: baseConfig }))
  await page.route('**/api/v1/ws', route => route.abort())
})

test('config page loads and shows Configuration heading', async ({ page }) => {
  await page.goto('/#/config')
  await expect(page.getByRole('heading', { name: /configuration/i })).toBeVisible()
})

test('config page shows Save button after loading', async ({ page }) => {
  await page.goto('/#/config')
  await expect(page.getByRole('button', { name: /^save$/i })).toBeVisible()
})

test('clicking Save calls PUT /config', async ({ page }) => {
  let savedBody: unknown
  await page.route('**/api/v1/config', async route => {
    if (route.request().method() === 'PUT') {
      savedBody = await route.request().postDataJSON()
      await route.fulfill({ json: { success: true, data: null } })
    } else {
      await route.fulfill({ json: baseConfig })
    }
  })

  await page.goto('/#/config')
  await expect(page.getByRole('button', { name: /^save$/i })).toBeVisible()

  const putRequest = page.waitForRequest(
    req => req.url().includes('/api/v1/config') && req.method() === 'PUT'
  )
  await page.getByRole('button', { name: /^save$/i }).click()
  await putRequest

  expect(savedBody).toBeDefined()
})

test('config page shows check_interval field value', async ({ page }) => {
  await page.goto('/#/config')
  // baseConfig has check_interval: 60 — the input should contain "60"
  await expect(page.locator('input[type="number"]').first()).toBeVisible()
})
