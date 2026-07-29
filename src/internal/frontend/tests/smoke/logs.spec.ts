import { test, expect } from '@playwright/test'

/**
 * Fase 7 (spec §9.5). A tela de Logs não tinha smoke; ganhou um aqui porque o entregável da
 * fase é justamente "filtro, busca e follow preservados" — e os três mudaram de forma:
 * o filtro de nível virou pills com contagem (era um <select> sem contagem) e a origem passou
 * a ser uma coluna derivada do `caller` do zerolog.
 */

const LINES = [
  '{"level":"info","time":"2026-07-29T11:04:02Z","caller":"loop.go:90","message":"verificacao iniciada"}',
  '{"level":"warn","time":"2026-07-29T11:04:03Z","caller":"nyaa.go:41","message":"nenhum torrent para Alpha","query":"Alpha 12"}',
  '{"level":"error","time":"2026-07-29T11:04:05Z","caller":"anilist.go:210","message":"falha ao consultar a AniList"}',
  '{"level":"debug","time":"2026-07-29T11:04:06Z","caller":"session.go:77","message":"torrent adicionado"}',
]

test.beforeEach(async ({ page }) => {
  await page.route('**/api/v1/logs**', route =>
    route.fulfill({ json: { success: true, data: { lines: LINES, total: LINES.length } } })
  )
  await page.route('**/api/v1/ws', route => route.abort())
})

/**
 * Uma linha por entrada de log. Ancora no papel `listitem`, não numa classe CSS: o grid de 4
 * colunas só existe a partir de `md`, então qualquer seletor de classe passaria a depender do
 * viewport do teste.
 */
function rows(page: import('@playwright/test').Page) {
  return page.getByRole('listitem')
}

test('logs page renders one row per line with the derived source column', async ({ page }) => {
  await page.goto('/#/logs')

  await expect(rows(page)).toHaveCount(4)
  // caller loop.go -> pacote daemon -> "scheduler"; nyaa.go -> "rss"; anilist.go -> "anilist".
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
  await expect(page.getByText('falha ao consultar a AniList')).toBeVisible()
  await expect(page.getByText('verificacao iniciada')).toHaveCount(0)
})

test('search filters the body and highlights the match', async ({ page }) => {
  await page.goto('/#/logs')

  await page.getByPlaceholder(/search/i).fill('torrent')

  // "nenhum torrent para Alpha" e "torrent adicionado".
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
