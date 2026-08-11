import { test, expect, type Page } from '@playwright/test'
import { baseConfig } from './helpers.js'

/**
 * A tela desenha UM grupo por vez, então um campo só existe no DOM enquanto o grupo dele está
 * aberto — daí `openGroup` antes de quase toda asserção. Os quatro grupos são Library, Anilist,
 * Downloads e Torrent search; o inicial é Library, porque é onde mora o único campo obrigatório
 * e é para lá que `?missingConfig=true` manda o usuário.
 *
 * Os testes rodam em `en` (o locale default), então os rótulos aqui são os de `messages/en.json`.
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
 * Rota o PUT para dentro do teste e devolve o corpo salvo.
 *
 * Espere pelo `whenSaved`, não por um `page.waitForRequest`: aquele resolve no EVENTO de request,
 * que pode chegar antes de o handler da rota rodar — sob carga (a suíte roda em workers
 * paralelos) dava `saved.body === undefined` e a asserção falhava de forma intermitente.
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
 * Um teste por grupo. É a única forma de um campo se perder nesta mudança: o Salvar manda
 * `config` inteiro por spread, então o PAYLOAD não corre risco — o risco é um campo ficar
 * invisível por não ter sido movido junto com o grupo dele.
 */
test('each group holds its own fields', async ({ page }) => {
  await page.goto('/#/config')

  await expect(page.getByLabel(/completed anime path/i)).toBeVisible()
  await expect(page.getByLabel(/rename files for jellyfin/i)).toBeVisible()
  await expect(page.getByLabel(/min free disk space/i)).toBeVisible()

  await openGroup(page, /^anilist$/i)
  await expect(page.getByLabel(/usernames/i)).toBeVisible()
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
  await expect(page.getByLabel(/max episodes for batch/i)).toBeVisible()
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
  await openGroup(page, /^anilist$/i)

  const usernames = page.getByLabel(/usernames/i)
  await usernames.fill('seconduser')
  await usernames.press('Enter')

  await expect(page.getByRole('button', { name: 'Remove seconduser' })).toBeVisible()
  await expect(usernames).toHaveValue('')

  await page.getByRole('button', { name: /^save$/i }).click()
  await saved.whenSaved

  expect(saved.body?.anilist_usernames).toEqual(['testuser', 'seconduser'])
})

test('status pills toggle and download/delete stay mutually exclusive', async ({ page }) => {
  const saved = await captureSave(page)
  await page.goto('/#/config')
  await openGroup(page, /^anilist$/i)

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
 * Com um grupo por vez, um toast dizendo "a pasta é obrigatória" enquanto o usuário olha para
 * outro grupo não teria como ser acionável — a validação que falha traz o grupo dela à tela.
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
 * Marcação de pendência. As duas marcas — o `*` no campo e o ponto no índice — derivam da MESMA
 * lista `requiredChecks` que barra o Salvar; é isso que impede o ponto de mentir quando uma regra
 * mudar. `check_interval: 0` reprova em Downloads, a pasta vazia reprova em Library, e os outros
 * dois grupos ficam limpos — o ponto é por grupo, não global.
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

  // Propriedade do DOM, não atributo: `required` é booleano, e afirmar a string vazia do
  // atributo depende de detalhe de serialização em vez do estado do campo.
  await expect(page.getByLabel(/completed anime path/i)).toHaveJSProperty('required', true)
  await expect(page.getByText(/required field/i)).toBeVisible()

  const index = sideIndex(page)
  await expect(index.getByRole('button', { name: /library.*missing/i })).toBeVisible()
  await expect(index.getByRole('button', { name: /downloads.*missing/i })).toBeVisible()
  await expect(index.getByRole('button', { name: /^anilist$/i })).toBeVisible()
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
 * Prioridades e Notificações gravam no mesmo PUT /config mas são telas próprias. O índice as
 * expõe como LINKS (não grupos): navegam em vez de trocar `activeGroup`, e por isso são <a>.
 */
test('the index links navigate to Priorities and Notifications', async ({ page }) => {
  await page.goto('/#/config')

  await sideIndex(page).getByRole('link', { name: /priorities/i }).click()
  await expect(page).toHaveURL(/#\/priorities$/)

  await page.goto('/#/config')
  await sideIndex(page).getByRole('link', { name: /notifications/i }).click()
  await expect(page).toHaveURL(/#\/notifications$/)
})
