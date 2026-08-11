import { test, expect } from '@playwright/test'
import {
  baseConfig,
  makeAnimesResponse,
  makeTorrentsResponse,
  setupStatusPageMocks,
} from './helpers.js'

/**
 * Regressões de layout que só aparecem com dados reais de volume — nenhuma delas quebra um
 * teste de conteúdo, porque o elemento continua no DOM e "visível", só que fora da tela.
 *
 * 1. Estouro horizontal. A faixa "Downloads ativos" do card herói tem `overflow-x-auto`, mas
 *    isso não bastava: a <section> que a contém é item de um grid e item de grid tem
 *    `min-width: auto`, ou seja, seu piso é o tamanho intrínseco do conteúdo. Com muitos
 *    downloads a section crescia até o max-content (medido: 2494px num grid de 1280px), o
 *    scroll da faixa nunca ativava e a coluna da direita saía da tela. A asserção é sobre a
 *    PÁGINA (`documentElement.scrollWidth`), não sobre a faixa: é o sintoma que o usuário vê,
 *    e pega qualquer outra coluna/tabela que estoure pelo mesmo motivo.
 *
 * 2. NavRail rolando junto com a página. O rail é `h-screen`, mas sem posicionamento era só um
 *    item de flex no fluxo normal — numa página mais alta que a viewport ele subia junto e
 *    sumia. A asserção compara a posição do rail antes e depois de um scroll até o fim.
 */

// 24 downloads ativos: volume suficiente para a faixa passar de 4x a largura da tela.
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

// 30 animes com nomes/contadores longos: deixa a página bem mais alta que a viewport (para o
// teste do rail) e exercita a largura máxima das colunas da lista.
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
  // setupStatusPageMocks já cobre status/animes/config/torrents/ws.
  await setupStatusPageMocks(page, 'running', makeAnimesResponse(manyAnimes))
  await page.route('**/api/v1/torrents', route =>
    route.fulfill({ json: makeTorrentsResponse(manyTorrents) }),
  )
}

// 1024 é o menor viewport em que a tabela da lista de animes é renderizada (abaixo disso
// Status.svelte troca para cards empilhados); 1280 é o padrão do projeto chromium.
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

  // A faixa é mais estreita que seu conteúdo — ou seja, virou container de scroll de verdade,
  // em vez de ter empurrado a section para o tamanho do conteúdo.
  expect(scrollWidth).toBeGreaterThan(clientWidth)
  expect(clientWidth).toBeLessThan(1280)
})

/**
 * As três telas com tabela têm trilhas de largura FIXA (Status LIST_GRID, Downloads ROW_GRID,
 * AnimeDetail EP_GRID). Cada uma soma mais que a largura útil em `md` (768px), então todas
 * trocam para cards/flex-wrap abaixo de `lg`. Este teste é o que segura esse corte no lugar:
 * baixar qualquer um deles de volta para `md`, ou acrescentar uma coluna fixa, volta a jogar a
 * página em rolagem horizontal — que é exatamente o sintoma reclamado, e nenhum teste de
 * conteúdo pega, porque os elementos continuam no DOM e "visíveis", só que fora da tela.
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
      custom_search_query: null,
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
 * O painel do MoreMenu é `absolute z-50`, mas o z-index dele só vale DENTRO do rail: `position:
 * sticky` cria contexto de empilhamento sempre, independente de z-index. Sem z-index no <nav>, o
 * rail inteiro (painel incluído) pinta na camada z-auto da raiz, em ordem de árvore — e qualquer
 * elemento POSICIONADO que venha depois no DOM ganha dele. É o caso do `.card` do daisyUI
 * (position: relative), usado na tela de Prioridades: os cards apareciam na frente do menu.
 *
 * A asserção usa elementFromPoint no centro de um item do menu — é o teste de "quem está na
 * frente" que o usuário faz com o clique, sem depender do timeout de actionability.
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
  // Prioridades morre no load sem estes dois: `config.priorities` (ausente do baseConfig) e o
  // GET dos defaults. Sem eles não haveria card nenhum na tela e o teste passaria a vazio.
  await page.route('**/api/v1/config', route =>
    route.fulfill({ json: { success: true, data: { ...baseConfig.data, priorities: samplePriorities } } }),
  )
  await page.route('**/api/v1/config/priorities/defaults', route =>
    route.fulfill({ json: { success: true, data: samplePriorities } }),
  )
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.goto('/#/priorities')

  // Os cards existem de verdade — é o que precisa ficar ATRÁS do menu.
  await expect(page.getByText('uncensored')).toBeVisible()

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

  // O backdrop (`fixed inset-0 z-40`) vive dentro do rail, ou seja, dentro do contexto z-30 —
  // um clique sobre um card ainda tem de cair nele e fechar o menu. `page.mouse` e não
  // `locator.click()`: o locator do card seria recusado justamente por estar coberto pelo
  // backdrop, que é o comportamento que se quer.
  const cardPoint = await page.getByText('uncensored').evaluate((el: HTMLElement) => {
    const rect = el.getBoundingClientRect()
    return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 }
  })
  await page.mouse.click(cardPoint.x, cardPoint.y)
  await expect(item).toHaveCount(0)
})

/**
 * Contrapartida do z-index do rail: o que precisa ficar ACIMA dele continua acima. Modal/Toasts
 * são `fixed z-50` na raiz — maior que os 30 do rail —, mas isso só vale enquanto nenhum
 * ancestral deles virar contexto de empilhamento; a asserção é sobre quem pinta na frente, não
 * sobre o número.
 */
test('a modal still paints above the nav rail', async ({ page }) => {
  await setupBusyStatusPage(page)
  await page.setViewportSize({ width: 1280, height: 800 })
  await page.goto('/#/downloads')

  await page.getByRole('button', { name: /^delete — /i }).first().click()

  const dialog = page.getByRole('dialog')
  await expect(dialog).toBeVisible()

  // O diálogo é centralizado na viewport, então sua borda esquerda cai sobre a faixa do rail.
  const coveredBy = await dialog.evaluate((el: HTMLElement) => {
    const rect = el.getBoundingClientRect()
    const top = document.elementFromPoint(rect.x + 4, rect.y + rect.height / 2)
    if (!top || el.contains(top) || top.contains(el)) return null
    return `${top.tagName}.${top.className}`
  })
  expect(coveredBy, 'something paints over the delete dialog').toBeNull()
})

/**
 * Cabeçalho de Downloads em tela estreita. O bloco do título é `flex-1`, ou seja, flex-basis 0%
 * — e a quebra de linha de um container `flex-wrap` usa o tamanho principal HIPOTÉTICO de cada
 * item, que com basis 0 é ~0. Resultado: a caixa de velocidade nunca ia para a linha de baixo, o
 * bloco do título ficava com as sobras (~30px no print do usuário) e o subtítulo virava uma
 * palavra por linha. O piso vem de `min-w-`, que ENTRA nesse tamanho hipotético — é por isso que
 * a correção é uma largura mínima e não um breakpoint: um `sm:flex-row` consertaria 375px e
 * deixaria 640px ainda espremido.
 *
 * A asserção conta linhas de verdade (uma retângulo por line box via Range), não altura em px.
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
 * As pills de filtro de Downloads viviam numa faixa `overflow-x-auto`: em tela estreita as
 * últimas ("Seeding", "Problems") ficavam fora da vista atrás de um scroll horizontal, e a
 * contagem que elas existem para mostrar — "tem algo ali?" — só aparecia depois de arrastar.
 * Agora a faixa quebra para a linha de baixo, então TODAS ficam sempre visíveis.
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
      // ratio 1: a pill INTEIRA na viewport, não uma borda dela.
      await expect(pill).toBeInViewport({ ratio: 1 })
    }

    // E a faixa não é um container de scroll: nada escondido para o lado.
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
  // Confere que a página realmente rolou; senão o teste passaria por vacuidade.
  expect(await page.evaluate(() => window.scrollY)).toBeGreaterThan(200)
  const after = await rail.boundingBox()

  expect(before?.y).toBe(0)
  expect(after?.y).toBe(0)
  await expect(rail.getByRole('link', { name: /status/i })).toBeInViewport()
})

/**
 * Índice do Config no mobile. Mesma regressão das pills de filtro de Downloads, mesmo remédio:
 * a faixa tinha 358px de espaço para 644px de conteúdo em 390px, então "Torrent search" e os
 * dois links de saída ficavam atrás de um arrasto lateral. `flex-wrap` põe tudo na tela.
 *
 * A asserção é dupla de propósito: "todo item inteiro na viewport" pega o item escondido, e
 * "a faixa não é container de scroll" pega o retorno de `overflow-x-auto` mesmo que por sorte
 * os itens caibam num idioma curto.
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
      index.getByRole('button', { name: /^anilist$/i }),
      index.getByRole('button', { name: /^downloads$/i }),
      index.getByRole('button', { name: /^torrent search$/i }),
      index.getByRole('link', { name: /priorities/i }),
      index.getByRole('link', { name: /notifications/i }),
    ]

    for (const item of items) {
      // ratio 1: o item INTEIRO na viewport, não uma borda dele.
      await expect(item).toBeInViewport({ ratio: 1 })
    }

    const scrolls = await index.evaluate((el: HTMLElement) => el.scrollWidth > el.clientWidth)
    expect(scrolls, 'the config index still scrolls horizontally').toBe(false)
  })
}

/**
 * Ações do daemon no cabeçalho do Status. A fileira era `flex shrink-0 gap-2`: não podia
 * encolher NEM quebrar, então sua largura era a soma dos três botões para sempre — 358px em
 * inglês, 438px em pt-BR — e em tela estreita "Parar Daemon" saía pela direita.
 *
 * Parametrizado por LOCALE de propósito. O resto da suíte roda só em `en`, o default, e foi
 * exatamente isso que deixou o bug passar: em inglês os rótulos são ~80px mais curtos e a
 * fileira só estourava abaixo de 375px, fora das larguras testadas. pt-BR é o idioma mais
 * longo dos dois; um teste de largura que só roda em `en` mede o caso fácil.
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

      // Escopado à fileira do cabeçalho: "+ Add anime" também existe no empty state da lista e
      // na NavTabBar do mobile, então o nome acessível sozinho casa três elementos. "Forçar
      // verificação" só existe aqui, e serve de âncora para chegar na fileira.
      const row = page
        .getByRole('button', { name: /forçar verificação|force check/i })
        .locator('..')

      const actions = [
        row.getByRole('link', { name: /adicionar anime|add anime/i }),
        row.getByRole('button', { name: /forçar verificação|force check/i }),
        row.getByRole('button', { name: /parar daemon|stop daemon/i }),
      ]
      for (const action of actions) {
        // ratio 1: o botão INTEIRO na viewport, não uma borda dele.
        await expect(action).toBeInViewport({ ratio: 1 })
      }

      // E a página não ganhou scroll horizontal — é o sintoma que o usuário vê.
      const over = await page.evaluate(
        () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
      )
      expect(over, 'the page scrolls horizontally').toBe(false)
    })
  }
}
