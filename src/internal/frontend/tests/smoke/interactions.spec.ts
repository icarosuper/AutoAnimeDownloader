import { test, expect } from '@playwright/test'
import { setupStatusPageMocks } from './helpers.js'

test('clicking Stop Daemon calls POST /daemon/stop', async ({ page }) => {
  await setupStatusPageMocks(page, 'running')
  await page.route('**/api/v1/daemon/stop', route =>
    route.fulfill({ json: { success: true, data: null } })
  )

  await page.goto('/')
  const stopRequest = page.waitForRequest(
    req => req.url().includes('/daemon/stop') && req.method() === 'POST'
  )
  await page.getByRole('button', { name: /stop daemon/i }).click()
  await stopRequest
})

test('clicking Start Daemon calls POST /daemon/start', async ({ page }) => {
  await setupStatusPageMocks(page, 'stopped')
  await page.route('**/api/v1/daemon/start', route =>
    route.fulfill({ json: { success: true, data: null } })
  )

  await page.goto('/')
  await expect(page.getByRole('button', { name: /start daemon/i })).toBeVisible()

  const startRequest = page.waitForRequest(
    req => req.url().includes('/daemon/start') && req.method() === 'POST'
  )
  await page.getByRole('button', { name: /start daemon/i }).click()
  await startRequest
})

test('clicking Force Check calls POST /check', async ({ page }) => {
  await setupStatusPageMocks(page, 'running')
  await page.route('**/api/v1/check', route =>
    route.fulfill({ json: { success: true, data: null } })
  )

  await page.goto('/')
  const checkRequest = page.waitForRequest(
    req => req.url().includes('/api/v1/check') && req.method() === 'POST'
  )
  await page.getByRole('button', { name: /force check/i }).click()
  await checkRequest
})

test('Force Check button is disabled while checking', async ({ page }) => {
  await setupStatusPageMocks(page, 'checking')

  await page.goto('/')
  const btn = page.getByRole('button', { name: /checking/i })
  await expect(btn).toBeDisabled()
})
