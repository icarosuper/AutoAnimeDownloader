import { test, expect } from '@playwright/test'
import { baseConfig } from './helpers.js'

/**
 * Fase 6 (spec §9.4). O que mudou na tela e por isso precisa de cobertura nova:
 *   - índice lateral com UM grupo visível por vez, então um campo só existe no DOM quando o
 *     grupo dele está aberto (era tudo empilhado antes);
 *   - os campos de lista viraram ChipsInput, sem botão "+" — o teclado é o único caminho;
 *   - a validação que falha traz o grupo dela à tela.
 * O que NÃO mudou (D5) e continua asseverado: Salvar é o único caminho de escrita, e ele
 * manda um PUT /config.
 */

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/config', route => route.fulfill({ json: baseConfig }))
  await page.route('**/api/v1/ws', route => route.abort())
})

/** Rota o PUT para dentro do teste e devolve o corpo salvo. */
async function captureSave(page: import('@playwright/test').Page) {
  const saved: { body?: Record<string, unknown> } = {}
  await page.route('**/api/v1/config', async route => {
    if (route.request().method() === 'PUT') {
      saved.body = route.request().postDataJSON()
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

  const putRequest = page.waitForRequest(
    req => req.url().includes('/api/v1/config') && req.method() === 'PUT'
  )
  await page.getByRole('button', { name: /^save$/i }).click()
  await putRequest

  expect(saved.body).toBeDefined()
})

test('the side index swaps which group is on screen', async ({ page }) => {
  await page.goto('/#/config')
  const index = page.getByRole('navigation', { name: /configuration sections/i })
  await expect(index).toBeVisible()

  // Anilist é o grupo inicial: seus campos existem, os de Automação não.
  await expect(page.getByLabel(/usernames/i)).toBeVisible()
  await expect(page.getByLabel(/check interval/i)).toHaveCount(0)

  await index.getByRole('button', { name: /^automation$/i }).click()

  await expect(page.getByLabel(/check interval/i)).toHaveValue('60')
  await expect(page.getByLabel(/usernames/i)).toHaveCount(0)
})

test('the chips input adds a value with Enter and it reaches the PUT body', async ({ page }) => {
  const saved = await captureSave(page)
  await page.goto('/#/config')

  const usernames = page.getByLabel(/usernames/i)
  await usernames.fill('seconduser')
  await usernames.press('Enter')

  // Virou chip: existe um × com nome acessível próprio, e o campo esvaziou.
  await expect(page.getByRole('button', { name: 'Remove seconduser' })).toBeVisible()
  await expect(usernames).toHaveValue('')

  const putRequest = page.waitForRequest(
    req => req.url().includes('/api/v1/config') && req.method() === 'PUT'
  )
  await page.getByRole('button', { name: /^save$/i }).click()
  await putRequest

  expect(saved.body?.anilist_usernames).toEqual(['testuser', 'seconduser'])
})

test('removing the last username blocks the save and shows the offending group', async ({ page }) => {
  let putCalls = 0
  await page.route('**/api/v1/config', async route => {
    if (route.request().method() === 'PUT') {
      putCalls++
      await route.fulfill({ json: { success: true, data: null } })
    } else {
      await route.fulfill({ json: baseConfig })
    }
  })

  await page.goto('/#/config')
  await page.getByRole('button', { name: 'Remove testuser' }).click()

  // Sai do grupo do campo inválido para provar que a validação traz ele de volta.
  await page
    .getByRole('navigation', { name: /configuration sections/i })
    .getByRole('button', { name: /^automation$/i })
    .click()
  await expect(page.getByLabel(/check interval/i)).toBeVisible()

  await page.getByRole('button', { name: /^save$/i }).click()

  await expect(page.getByText(/at least one anilist username/i)).toBeVisible()
  await expect(page.getByLabel(/usernames/i)).toBeVisible()
  expect(putCalls).toBe(0)
})

test('status pills toggle and download/delete stay mutually exclusive', async ({ page }) => {
  const saved = await captureSave(page)
  await page.goto('/#/config')

  const downloadGroup = page.getByRole('group', { name: /^download statuses$/i })
  const deleteGroup = page.getByRole('group', { name: /^delete statuses$/i })

  // "Watching" (CURRENT) vem ligado em baixar; ligá-lo em deletar tem de desligar o outro.
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

  const putRequest = page.waitForRequest(
    req => req.url().includes('/api/v1/config') && req.method() === 'PUT'
  )
  await page.getByRole('button', { name: /^save$/i }).click()
  await putRequest

  expect(saved.body?.delete_statuses).toEqual(['CURRENT'])
  expect(saved.body?.download_statuses).toEqual(['REPEATING'])
})
