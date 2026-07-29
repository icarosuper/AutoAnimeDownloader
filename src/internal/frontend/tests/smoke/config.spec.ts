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

/**
 * Rota o PUT para dentro do teste e devolve o corpo salvo.
 *
 * Espere pelo `whenSaved`, não por um `page.waitForRequest`: aquele resolve no EVENTO de request,
 * que pode chegar antes de o handler da rota rodar — sob carga (a suíte roda em workers
 * paralelos) dava `saved.body === undefined` e a asserção falhava de forma intermitente.
 * `whenSaved` resolve depois de o corpo ter sido registrado.
 */
async function captureSave(page: import('@playwright/test').Page) {
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

  await page.getByRole('button', { name: /^save$/i }).click()
  await saved.whenSaved

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

  await page.getByRole('button', { name: /^save$/i }).click()
  await saved.whenSaved

  expect(saved.body?.delete_statuses).toEqual(['CURRENT'])
  expect(saved.body?.download_statuses).toEqual(['REPEATING'])
})

/**
 * Marcação de obrigatoriedade. Com UM grupo visível por vez, o asterisco no campo não basta: o
 * usuário não vê os outros três grupos, então "o que falta preencher" precisa aparecer no índice
 * lateral. As duas marcas derivam da MESMA lista de validações que barra o Salvar — é isso que
 * impede o ponto de mentir quando uma regra mudar.
 */
const incompleteConfig = {
  success: true,
  data: { ...baseConfig.data, anilist_usernames: [], completed_anime_path: '' },
}

test('required fields are marked and the side index flags the groups still missing data', async ({
  page,
}) => {
  await page.route('**/api/v1/config', route => route.fulfill({ json: incompleteConfig }))
  await page.goto('/#/config')

  // O campo de usuários é obrigatório (é a primeira validação do Salvar) e agora diz isso.
  await expect(page.getByLabel(/usernames/i)).toHaveAttribute('aria-required', 'true')
  // A legenda que explica o asterisco.
  await expect(page.getByText(/required field/i)).toBeVisible()

  const index = page.getByRole('navigation', { name: /configuration sections/i })
  // Anilist (usuários vazios) e Downloads (pasta vazia) estão pendentes; Automação e Filtros não.
  await expect(index.getByRole('button', { name: /anilist.*missing/i })).toBeVisible()
  await expect(index.getByRole('button', { name: /downloads.*missing/i })).toBeVisible()
  await expect(index.getByRole('button', { name: /^automation$/i })).toBeVisible()
  await expect(index.getByRole('button', { name: /^filters$/i })).toBeVisible()
})

test('the missing mark clears as soon as the field is filled', async ({ page }) => {
  await page.route('**/api/v1/config', route => route.fulfill({ json: incompleteConfig }))
  await page.goto('/#/config')

  const index = page.getByRole('navigation', { name: /configuration sections/i })
  await expect(index.getByRole('button', { name: /anilist.*missing/i })).toBeVisible()

  const usernames = page.getByLabel(/usernames/i)
  await usernames.fill('testuser')
  await usernames.press('Enter')

  await expect(index.getByRole('button', { name: /anilist.*missing/i })).toHaveCount(0)
  // Downloads continua pendente — a marca é por grupo, não global.
  await expect(index.getByRole('button', { name: /downloads.*missing/i })).toBeVisible()
})

// O mesmo, mas pelo OUTRO caminho de binding: `Input.svelte` (`bind:value` do componente, que
// por sua vez faz `bind:value` no <input> interno), e não o ChipsInput do teste acima. Se essa
// cadeia não invalidasse `config`, `pendingGroups` congelaria e o ponto mentiria — é o grupo que
// o usuário de fato abre para preencher a pasta.
test('filling the completed path clears the Downloads mark', async ({ page }) => {
  await page.route('**/api/v1/config', route => route.fulfill({ json: incompleteConfig }))
  await page.goto('/#/config')

  const index = page.getByRole('navigation', { name: /configuration sections/i })
  await index.getByRole('button', { name: /downloads.*missing/i }).click()

  await page.getByLabel(/completed anime path/i).fill('/completed')

  await expect(index.getByRole('button', { name: /downloads.*missing/i })).toHaveCount(0)
  await expect(index.getByRole('button', { name: /anilist.*missing/i })).toBeVisible()
})
