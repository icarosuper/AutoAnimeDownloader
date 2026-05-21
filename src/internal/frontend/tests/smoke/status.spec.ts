import { test, expect } from '@playwright/test'
import { setupStatusPageMocks } from './helpers.js'

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
