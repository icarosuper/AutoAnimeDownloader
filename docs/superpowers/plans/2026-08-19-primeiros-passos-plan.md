# Primeiros passos: guiar a primeira execução — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mostrar na tela de Status um card de três itens (pasta da biblioteca → fonte de animes → primeira verificação) que ensina o modelo mental do daemon e some sozinho quando os três ficam verdes, mais um deep-link `?group=` em Configurações e um caminho de volta para quem dispensou o card.

**Architecture:** É **tudo frontend**. Nenhum endpoint novo, nenhuma requisição nova (a tela de Status já busca `getStatus()`, `getAnimes()` e `getConfig()` em paralelo no `loadInitialData`), nenhum campo novo em `config.json`, nenhuma mudança no daemon. A decisão de quais itens estão verdes sai do componente para uma função pura em `lib/domain/onboarding.ts` (mesma fronteira de `animeState.ts` e `checkIssue.ts`); a dispensa mora num `writable` sobre `localStorage` em `lib/stores/onboarding.ts` (mesmo molde de `theme.ts`); `Config.svelte` unifica a leitura da querystring para poder ler `missingConfig` e `group` do mesmo `URLSearchParams`.

**Tech Stack:** Svelte 5 (sintaxe legacy — `export let` / `$:`, é o que o repo usa) + Vite + Tailwind/daisyUI, Paraglide i18n (`messages/{en,pt-BR}.json`), vitest (unit), Playwright (smoke), `@lucide/svelte` (ícones).

## Global Constraints

- Spec de origem: [`docs/superpowers/specs/2026-08-11-primeiros-passos-design.md`](../specs/2026-08-11-primeiros-passos-design.md).
- Repo root: `/home/icaro/Projetos/AAD/AutoAnimeDownloader`. **Todo caminho relativo neste plano é relativo a `src/internal/frontend/`**, exceto quando começa com `docs/`.
- **NUNCA commitar ou dar push sem permissão explícita do usuário** (CLAUDE.md). Os passos "Commit" abaixo significam: deixe o `git add` feito e **pergunte** antes do `git commit`. Se o usuário já autorizou commits nesta sessão, commite com a mensagem indicada.
- Comandos de teste (rodar de dentro de `src/internal/frontend/`): `bun run test:unit`, `bun run test:component`, `bun run test:smoke`. Os três já rodam `i18n:compile` antes — **não** edite nada em `src/lib/i18n/` (é gerado e gitignored); mexa só em `messages/en.json` e `messages/pt-BR.json`.
- `go test ./...` (na raiz do repo) tem de continuar passando. **Nada de Go muda neste plano** — se você editou um `.go`, está errado.
- **Sem `swag init`**: nenhum endpoint muda.
- Toda chave de mensagem nova entra nos **dois** arquivos, `messages/en.json` e `messages/pt-BR.json`, sempre. Os smoke tests rodam no locale default (`en`), então as asserções usam os textos de `en.json`.
- **Nenhuma requisição HTTP nova.** Se você adicionou um `fetch`/`getX()`, está errado — os dados já estão na tela.
- **Nada entra em `requiredChecks` nem no corpo do `PUT /config`.** A dispensa é preferência de UI, o payload de config não muda.
- Comentários novos em Svelte/TS seguem o estilo dos arquivos vizinhos: **português, com acentuação**, explicando *por quê*, não *o quê*. Não remova comentário existente que continue verdadeiro.
- Imports de módulos locais em `.svelte` e em `src/lib/` usam a extensão `.js` (`../lib/domain/onboarding.js`). Nos testes de `tests/unit/` a extensão é opcional — siga o arquivo vizinho.

## Não-objetivos (não invente escopo)

- Nenhuma rota nova, nenhum wizard, nenhum passo bloqueante.
- Nada sobre Prioridades/Notificações no card — estão atrás do menu "Mais" de propósito.
- Nada de diagnóstico de download: isso é o card de relatório da última verificação, **que já existe** em `routes/Status.svelte` (`data-testid="last-check-report"`).

## Desvios da spec (decididos aqui, com o motivo)

1. **O card de problemas do relatório não fica "logo acima da lista de animes".** A spec assume aquela posição, mas a implementação que existe hoje põe a `<section data-testid="last-check-report">` logo depois dos alertas, **antes** do grid do herói (`routes/Status.svelte:529`). O card de primeiros passos entra **entre o alerta de erro de passe e essa section** — que é ao mesmo tempo "depois dos alertas", "antes do herói" e "acima do card de problemas", ou seja, satisfaz as três regras da spec com a árvore real.
2. **`Status.svelte` passa a guardar `completed_anime_path` cru (`completedPath`), não só o booleano `libraryConfigured`.** A spec diz que a tela "passa a guardar `anilist_usernames` além do `completed_anime_path` que já vira `libraryConfigured`" — mas hoje o path cru é descartado na hora (`libraryConfigured = Boolean(configData.completed_anime_path)`). `onboardingSteps` recebe a config, não um booleano já cozido, então o campo cru precisa sobreviver. `libraryConfigured` **continua existindo e continua inicializando em `true`** — transformá-lo em derivado faria o botão "+ Adicionar anime" piscar desabilitado durante o load.
3. **A linha "Primeiros passos" em Configurações não é um `Input`.** É um bloco `label + hint + Button` escrito à mão, porque `Input`/`Toggle` fazem `bind:value` num campo de `config` e aqui **não há campo de config**. Os divisores saem de graça: o container dos campos é `divide-y divide-divider`.
4. **`onboarding_step_check_hint` não entra no objeto `T`.** `T` é `$: T = $locale && {...}` — só re-avalia quando o locale muda, e `{minutes}` interpola `checkInterval`, que chega depois, do `getConfig()`. A hint é chamada inline no markup (`{$locale && m.onboarding_step_check_hint({ minutes: checkInterval })}`), exatamente como `status_hero_summary` já faz.

---

### Task 1: `lib/domain/onboarding.ts` — a derivação pura dos três itens

A tela não decide nada, só pinta. Esta task é 100% testável sem DOM e sem locale.

**Files:**
- Create: `src/lib/domain/onboarding.ts`
- Test: `tests/unit/onboarding.test.ts`

**Interfaces:**
- Consumes: `AnimeInfo`, `Config`, `StatusResponse` de `src/lib/api/client.ts` (já existem; `Config.anilist_usernames: string[]`, `Config.completed_anime_path: string`, `StatusResponse.last_check: string`).
- Produces:
  - `export interface OnboardingSteps { library: boolean; source: boolean; firstCheck: boolean }`
  - `export function onboardingSteps(config: Pick<Config, 'completed_anime_path' | 'anilist_usernames'>, animes: AnimeInfo[], status: StatusResponse | null): OnboardingSteps`
  - `export function allDone(steps: OnboardingSteps): boolean`

- [ ] **Step 1: Write the failing test**

Crie `tests/unit/onboarding.test.ts` com este conteúdo exato:

```ts
import { describe, it, expect } from 'vitest'
import { onboardingSteps, allDone } from '../../src/lib/domain/onboarding'
import type { AnimeInfo, Config, StatusResponse } from '../../src/lib/api/client'

type OnboardingConfig = Pick<Config, 'completed_anime_path' | 'anilist_usernames'>

function makeConfig(overrides: Partial<OnboardingConfig> = {}): OnboardingConfig {
  return { completed_anime_path: '', anilist_usernames: [], ...overrides }
}

function makeAnime(overrides: Partial<AnimeInfo> = {}): AnimeInfo {
  return {
    anime_id: 1,
    name: 'Test Anime',
    episodes_downloaded: 0,
    episodes_released: 0,
    episodes_watched: 0,
    episodes_pending: 0,
    total_episodes: 12,
    latest_episode_number: 1,
    last_download_date: '',
    is_blacklisted: false,
    ...overrides,
  }
}

function makeStatus(lastCheck: string): StatusResponse {
  return {
    status: 'running',
    last_check: lastCheck,
    has_error: false,
    version: '1.0.0',
    disk_total: 0,
    disk_free: 0,
    disk_low: false,
  }
}

const VALID_CHECK = makeStatus('2026-08-19T10:00:00Z')

describe('onboardingSteps — item 1 (pasta da biblioteca)', () => {
  it('fica verde com um caminho preenchido', () => {
    const steps = onboardingSteps(makeConfig({ completed_anime_path: '/completed' }), [], VALID_CHECK)
    expect(steps.library).toBe(true)
  })

  it('fica vermelho com caminho vazio', () => {
    expect(onboardingSteps(makeConfig(), [], VALID_CHECK).library).toBe(false)
  })

  it('fica vermelho com caminho só de espaços', () => {
    const steps = onboardingSteps(makeConfig({ completed_anime_path: '   ' }), [], VALID_CHECK)
    expect(steps.library).toBe(false)
  })
})

// O "ou" do item 2 é a regra que estes três casos protegem: a conta AniList deixou de ser
// obrigatória quando animes avulsos passaram a existir.
describe('onboardingSteps — item 2 (fonte de animes)', () => {
  it('fica verde com username do AniList e ZERO animes', () => {
    const steps = onboardingSteps(makeConfig({ anilist_usernames: ['icaro'] }), [], VALID_CHECK)
    expect(steps.source).toBe(true)
  })

  it('fica verde com ZERO usernames e um anime', () => {
    const steps = onboardingSteps(makeConfig(), [makeAnime()], VALID_CHECK)
    expect(steps.source).toBe(true)
  })

  it('fica vermelho com os dois vazios', () => {
    expect(onboardingSteps(makeConfig(), [], VALID_CHECK).source).toBe(false)
  })
})

describe('onboardingSteps — item 3 (primeira verificação)', () => {
  it('fica vermelho sem status', () => {
    expect(onboardingSteps(makeConfig(), [], null).firstCheck).toBe(false)
  })

  it('fica vermelho com last_check vazio', () => {
    expect(onboardingSteps(makeConfig(), [], makeStatus('')).firstCheck).toBe(false)
  })

  // O zero-value de time.Time do Go serializa como uma data perfeitamente parseável — só
  // testar string vazia deixaria o item verde num daemon que nunca rodou.
  it('fica vermelho com o zero-value do Go', () => {
    expect(onboardingSteps(makeConfig(), [], makeStatus('0001-01-01T00:00:00Z')).firstCheck).toBe(false)
  })

  it('fica vermelho com uma data impossível de parsear', () => {
    expect(onboardingSteps(makeConfig(), [], makeStatus('não é data')).firstCheck).toBe(false)
  })

  it('fica verde com uma data recente', () => {
    expect(onboardingSteps(makeConfig(), [], VALID_CHECK).firstCheck).toBe(true)
  })
})

describe('allDone', () => {
  it('é verdadeiro só com os três verdes', () => {
    expect(allDone({ library: true, source: true, firstCheck: true })).toBe(true)
  })

  it('é falso se qualquer um estiver vermelho', () => {
    expect(allDone({ library: false, source: true, firstCheck: true })).toBe(false)
    expect(allDone({ library: true, source: false, firstCheck: true })).toBe(false)
    expect(allDone({ library: true, source: true, firstCheck: false })).toBe(false)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd src/internal/frontend && bun run test:unit -- tests/unit/onboarding.test.ts
```

Expected: FAIL — `Failed to resolve import "../../src/lib/domain/onboarding"`.

- [ ] **Step 3: Write minimal implementation**

Crie `src/lib/domain/onboarding.ts`:

```ts
import type { AnimeInfo, Config, StatusResponse } from '../api/client.js'

/**
 * Os três itens da linha de chegada do primeiro uso. A tela não decide nada, só pinta — é a
 * mesma fronteira que `animeState.ts` documenta: a lógica sai do componente para poder ser
 * testada sem DOM e sem locale.
 */
export interface OnboardingSteps {
  /** `completed_anime_path` preenchido. */
  library: boolean
  /** Pelo menos UMA fonte de animes: conta AniList **ou** anime avulso. */
  source: boolean
  /** O daemon já rodou um passe de verificação. */
  firstCheck: boolean
}

/**
 * Datas anteriores a 2010 são o zero-value de `time.Time` do Go (`0001-01-01T00:00:00Z`)
 * serializado, não um passe real — e ele é uma data perfeitamente parseável, então testar só
 * string vazia deixaria o item verde num daemon que nunca rodou. Mesmo corte que
 * `Status.svelte` já aplica em `formatDate`/`formatTimeAgo`.
 */
const MIN_REAL_YEAR = 2010

function isRealDate(value: string | undefined): boolean {
  if (!value) return false
  const date = new Date(value)
  return !isNaN(date.getTime()) && date.getFullYear() >= MIN_REAL_YEAR
}

export function onboardingSteps(
  config: Pick<Config, 'completed_anime_path' | 'anilist_usernames'>,
  animes: AnimeInfo[],
  status: StatusResponse | null,
): OnboardingSteps {
  return {
    library: !!config.completed_anime_path?.trim(),
    // Um "ou", não um "e": oferecer só a conta AniList mentiria sobre o modelo, porque um
    // anime avulso sozinho já dá ao daemon o que verificar.
    source: (config.anilist_usernames ?? []).length > 0 || animes.length > 0,
    firstCheck: isRealDate(status?.last_check),
  }
}

/** Derivado aqui para que a condição de render e os testes leiam a mesma regra. */
export function allDone(steps: OnboardingSteps): boolean {
  return steps.library && steps.source && steps.firstCheck
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd src/internal/frontend && bun run test:unit
```

Expected: PASS — os 13 casos novos verdes e nenhuma regressão nos arquivos existentes de `tests/unit/`.

- [ ] **Step 5: Commit**

```bash
git add src/internal/frontend/src/lib/domain/onboarding.ts src/internal/frontend/tests/unit/onboarding.test.ts
git commit -m "feat: derivação pura dos passos de primeiros passos"
```

---

### Task 2: Deep-link `?group=` em Configurações

O card do item ② vai apontar para `#/config?group=anilist`. Esta task entrega o alvo **antes** do link existir, para que o card nunca chegue apontando para um parâmetro ignorado.

`checkQueryParams()` hoje tem dois ramos (`window.location.search` e o trecho depois do `?` no hash, porque a app é um SPA de hash) e o primeiro faz `return`. Ela passa a resolver o `URLSearchParams` **uma vez** e ler os dois parâmetros dele — sem isso, `group` teria que ser lido em dois lugares e os dois poderiam divergir.

**Files:**
- Modify: `src/routes/Config.svelte:180-193` (a função `checkQueryParams`)
- Test: `tests/smoke/config.spec.ts` (acrescentar um teste no fim do arquivo)

**Interfaces:**
- Consumes: nada da Task 1.
- Produces: a URL `#/config?group=<library|anilist|downloads|search>` abre a tela naquele grupo. É o alvo do botão do item ② na Task 4.

- [ ] **Step 1: Write the failing test**

Acrescente ao **fim** de `tests/smoke/config.spec.ts`:

```ts
// O `?group=` é o alvo do botão "Connect AniList" do card de primeiros passos (tela de Status).
// A validação usa o array `groups` que a tela já monta, então um id desconhecido não pode
// deixar a tela num grupo que não renderiza nada — cai no default `library`.
test('?group=anilist abre a tela no grupo Anilist', async ({ page }) => {
  await page.goto('/#/config?group=anilist')

  await expect(page.getByLabel(/usernames/i)).toBeVisible()
  await expect(page.getByLabel(/completed anime path/i)).toHaveCount(0)
})

test('?group= com valor desconhecido cai no grupo Library', async ({ page }) => {
  await page.goto('/#/config?group=naoexiste')

  await expect(page.getByLabel(/completed anime path/i)).toBeVisible()
})

// Os dois parâmetros saem do MESMO URLSearchParams — antes cada ramo lia só `missingConfig` e
// o primeiro dava `return`, então ler os dois juntos é exatamente o que pode ter regredido.
test('?missingConfig=true&group=downloads mostra o banner E abre o grupo Downloads', async ({ page }) => {
  await page.goto('/#/config?missingConfig=true&group=downloads')

  await expect(page.getByRole('alert')).toBeVisible()
  await expect(page.getByLabel(/check interval/i)).toBeVisible()
})
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd src/internal/frontend && bun run test:smoke -- tests/smoke/config.spec.ts
```

Expected: FAIL — `?group=anilist abre a tela no grupo Anilist` falha porque `completed anime path` continua visível (a tela ignora `group` e fica no default `library`). Os outros dois passam desde já (são a rede de segurança da unificação).

- [ ] **Step 3: Write minimal implementation**

Em `src/routes/Config.svelte`, substitua a função `checkQueryParams` inteira (o bloco que começa em `function checkQueryParams() {`, logo abaixo de `let showMissingConfigBanner = false;`) por:

```ts
  /**
   * A app é um SPA de hash, então a query pode estar em `?a=b#/config` (raro) ou depois do `?`
   * DENTRO do hash (`#/config?a=b`, o caminho normal). Os params são resolvidos UMA vez e
   * todos os parâmetros saem dali: com um `return` por ramo, cada parâmetro novo teria que ser
   * lido nos dois lugares, e os dois poderiam divergir.
   */
  function resolveParams(): URLSearchParams | null {
    if (typeof window === "undefined") return null;
    if (window.location.search) return new URLSearchParams(window.location.search);
    const hashParts = window.location.hash.split("?");
    return hashParts.length > 1 ? new URLSearchParams(hashParts[1]) : null;
  }

  function checkQueryParams() {
    const params = resolveParams();
    if (!params) return;

    showMissingConfigBanner = params.has("missingConfig");

    // A validação usa o array `groups` que a tela já monta — uma segunda lista de ids sairia
    // de dia com ele. Valor desconhecido é ignorado e `activeGroup` fica no default
    // "library": sem a guarda, um link velho ou digitado errado deixaria a tela num grupo que
    // não renderiza nada.
    //
    // Sem conflito com a validação: `firstValidationError()` só mexe em `activeGroup` no
    // clique de Salvar, e isto roda no `onMount`.
    const group = params.get("group");
    if (group && groups.some((g) => g.id === group)) activeGroup = group as GroupId;
  }
```

Nada mais muda: `onMount` já chama `checkQueryParams()` antes de `loadConfig()`.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd src/internal/frontend && bun run test:smoke -- tests/smoke/config.spec.ts
```

Expected: PASS — os três testes novos e os 17 pré-existentes do arquivo.

- [ ] **Step 5: Commit**

```bash
git add src/internal/frontend/src/routes/Config.svelte src/internal/frontend/tests/smoke/config.spec.ts
git commit -m "feat: deep-link ?group= na tela de configurações"
```

---

### Task 3: A store da dispensa + as chaves de i18n

Duas peças pequenas que o card e o caminho de volta consomem. Ficam juntas porque nenhuma das duas tem deliverable testável sozinha, e as duas são pré-requisito das Tasks 4 e 5.

**Files:**
- Create: `src/lib/stores/onboarding.ts`
- Modify: `messages/en.json` (acrescentar 14 chaves no fim)
- Modify: `messages/pt-BR.json` (as mesmas 14 chaves, no fim)

**Interfaces:**
- Consumes: nada.
- Produces:
  - `export const onboardingDismissed` — store legível com `$onboardingDismissed` (boolean) e escrevível com `onboardingDismissed.set(true|false)`, persistido em `localStorage` sob a chave `onboarding-dismissed`.
  - As funções de mensagem do paraglide: `m.onboarding_title()`, `m.onboarding_step_library()`, `m.onboarding_step_library_hint()`, `m.onboarding_step_source()`, `m.onboarding_step_source_hint()`, `m.onboarding_step_check()`, `m.onboarding_step_check_hint({ minutes })`, `m.onboarding_dismiss()`, `m.onboarding_action_configure()`, `m.onboarding_action_connect_anilist()`, `m.onboarding_action_check_now()`, `m.onboarding_restore_label()`, `m.onboarding_restore_hint()`, `m.onboarding_restore_button()`.

- [ ] **Step 1: Criar a store**

Crie `src/lib/stores/onboarding.ts`:

```ts
import { writable } from 'svelte/store'

/**
 * A dispensa do card de primeiros passos. Mesmo molde de `theme.ts` e `locale.ts`:
 * `localStorage` com a guarda `isBrowser`.
 *
 * É por NAVEGADOR de propósito, e não um campo `tutorial_dismissed` no `config.json` — ver
 * decisions.md. Um campo de config seguiria o usuário, mas poria uma preferência de UI num
 * schema que o daemon lê e valeria para todos os dispositivos de uma vez.
 */
const DISMISSED_KEY = 'onboarding-dismissed'

const isBrowser = typeof window !== 'undefined'

function getStored(): boolean {
  if (!isBrowser) return false
  return localStorage.getItem(DISMISSED_KEY) === 'true'
}

function createOnboardingDismissed() {
  const { subscribe, set } = writable<boolean>(getStored())

  return {
    subscribe,
    set: (value: boolean) => {
      if (isBrowser) localStorage.setItem(DISMISSED_KEY, String(value))
      set(value)
    },
  }
}

export const onboardingDismissed = createOnboardingDismissed()
```

- [ ] **Step 2: Acrescentar as chaves em `messages/en.json`**

Acrescente ao fim do objeto (antes do `}` final, com vírgula na entrada anterior). É o locale base e o que os smoke tests leem:

```json
  "onboarding_title": "First steps",
  "onboarding_step_library": "Pick the library folder",
  "onboarding_step_library_hint": "That's where the episodes live. The daemon downloads and keeps seeding inside a .torrents subfolder of it, and links the episode into the anime's folder — without using twice the space.",
  "onboarding_step_source": "Tell it which animes to follow",
  "onboarding_step_source_hint": "Connect an AniList account to follow your list automatically, or add standalone animes one by one. Both work together.",
  "onboarding_step_check": "Wait for the first check",
  "onboarding_step_check_hint": "Every {minutes} minutes the daemon reads your list, searches Nyaa and downloads the missing episodes. From here on you don't have to do anything.",
  "onboarding_dismiss": "Don't show again",
  "onboarding_action_configure": "Set the folder",
  "onboarding_action_connect_anilist": "Connect AniList",
  "onboarding_action_check_now": "Check now",
  "onboarding_restore_label": "First steps",
  "onboarding_restore_hint": "The intro card on the Status screen.",
  "onboarding_restore_button": "Show again"
```

- [ ] **Step 3: Acrescentar as mesmas chaves em `messages/pt-BR.json`**

```json
  "onboarding_title": "Primeiros passos",
  "onboarding_step_library": "Escolha a pasta da biblioteca",
  "onboarding_step_library_hint": "É onde os episódios ficam. O daemon baixa e continua semeando numa subpasta .torrents dela, e cria um link do episódio na pasta do anime — sem duplicar espaço.",
  "onboarding_step_source": "Diga quais animes acompanhar",
  "onboarding_step_source_hint": "Ligue uma conta AniList para seguir sua lista automaticamente, ou adicione animes avulsos um a um. Os dois funcionam juntos.",
  "onboarding_step_check": "Aguarde a primeira verificação",
  "onboarding_step_check_hint": "A cada {minutes} minutos o daemon lê sua lista, procura no Nyaa e baixa os episódios que faltam. Daqui pra frente você não precisa fazer nada.",
  "onboarding_dismiss": "Não mostrar mais",
  "onboarding_action_configure": "Configurar pasta",
  "onboarding_action_connect_anilist": "Ligar conta AniList",
  "onboarding_action_check_now": "Verificar agora",
  "onboarding_restore_label": "Primeiros passos",
  "onboarding_restore_hint": "O card de introdução na tela de Status.",
  "onboarding_restore_button": "Mostrar novamente"
```

- [ ] **Step 4: Verificar que o paraglide compila as 14 chaves nos dois locales**

```bash
cd src/internal/frontend && bun run i18n:compile && ls src/lib/i18n/messages | grep '^onboarding_' | wc -l
```

Expected: `14` — e o comando de compile termina sem warning de chave faltando num locale. Se der um número diferente de 14, uma chave está só num dos dois JSONs, ou há vírgula/JSON inválido; conserte antes de seguir.

- [ ] **Step 5: Verificar que nada quebrou**

```bash
cd src/internal/frontend && bun run check && bun run test:unit
```

Expected: PASS, sem erro novo de `svelte-check`.

- [ ] **Step 6: Commit**

```bash
git add src/internal/frontend/src/lib/stores/onboarding.ts src/internal/frontend/messages/en.json src/internal/frontend/messages/pt-BR.json
git commit -m "feat: store da dispensa e textos de primeiros passos"
```

---

### Task 4: O card na tela de Status

Largura total, **depois** dos alertas (`disk_low`, erro de passe) e **antes** da section do relatório da última verificação e do grid do herói. Nenhuma requisição nova: `loadInitialData` já busca as três coisas.

**Files:**
- Modify: `src/routes/Status.svelte` — imports (`Check` do lucide, o domínio, a store), o objeto `T`, as variáveis de estado, `loadInitialData`, os reativos, e o markup entre o alerta de erro de passe e `{#if hasReport && lastCheck}`
- Test: `tests/smoke/status.spec.ts` (acrescentar um `test.describe` no fim)

**Interfaces:**
- Consumes: `onboardingSteps(config, animes, status)` e `allDone(steps)` de `../lib/domain/onboarding.js` (Task 1); `onboardingDismissed` de `../lib/stores/onboarding.js` e as 11 mensagens `onboarding_*` do card (Task 3); `#/config?group=anilist` (Task 2).
- Produces: a `<section data-testid="onboarding-card">` na tela de Status, e a chamada `onboardingDismissed.set(true)` que a Task 5 desfaz.

- [ ] **Step 1: Write the failing test**

Acrescente ao **fim** de `tests/smoke/status.spec.ts`. Note o import extra de `baseConfig` e do tipo `Page` na primeira linha do arquivo — ajuste os imports do topo para:

```ts
import { test, expect, type Page } from '@playwright/test'
import { baseConfig, makeAnimesResponse, setupStatusPageMocks } from './helpers.js'
```

E acrescente no fim:

```ts
/**
 * Primeiros passos. `setupStatusPageMocks` já entrega os três itens VERDES (pasta `/completed`,
 * `anilist_usernames: ['testuser']` e um `last_check` de 2026), então o card não aparece em
 * nenhum dos testes acima — e é isso que o segundo teste daqui trava.
 *
 * As rotas são re-registradas: o Playwright casa a rota registrada por ÚLTIMO, então estas
 * sobrescrevem as do `beforeEach` do topo do arquivo.
 */
test.describe('onboarding card', () => {
  async function setupFreshInstall(page: Page) {
    await page.route('**/api/v1/config', route =>
      route.fulfill({
        json: {
          success: true,
          data: { ...baseConfig.data, completed_anime_path: '', anilist_usernames: [] },
        },
      }),
    )
    // `0001-01-01T00:00:00Z` é o zero-value de time.Time do Go: daemon que nunca rodou.
    await page.route('**/api/v1/status', route =>
      route.fulfill({
        json: {
          success: true,
          data: {
            status: 'stopped',
            last_check: '0001-01-01T00:00:00Z',
            has_error: false,
            version: '1.0.0',
          },
        },
      }),
    )
  }

  test('instalação nova mostra o card com os três passos', async ({ page }) => {
    await setupFreshInstall(page)
    await page.goto('/')

    const card = page.getByTestId('onboarding-card')
    await expect(card).toBeVisible()
    await expect(card.getByRole('listitem')).toHaveCount(3)
  })

  test('some quando pasta, fonte e primeira verificação estão prontos', async ({ page }) => {
    await setupStatusPageMocks(page, 'running', makeAnimesResponse())
    await page.goto('/')

    await expect(page.getByRole('heading', { name: /status/i })).toBeVisible()
    await expect(page.getByTestId('onboarding-card')).toHaveCount(0)
  })

  test('"Don\'t show again" esconde o card, e a dispensa sobrevive ao reload', async ({ page }) => {
    await setupFreshInstall(page)
    await page.goto('/')

    await page.getByRole('button', { name: /don't show again/i }).click()
    await expect(page.getByTestId('onboarding-card')).toHaveCount(0)

    await page.reload()
    await expect(page.getByRole('heading', { name: /status/i })).toBeVisible()
    await expect(page.getByTestId('onboarding-card')).toHaveCount(0)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd src/internal/frontend && bun run test:smoke -- tests/smoke/status.spec.ts
```

Expected: FAIL — `instalação nova mostra o card com os três passos` estoura no `toBeVisible()` (nenhum elemento com `data-testid="onboarding-card"`). `some quando ...` já passa (o card ainda não existe); é a rede de segurança contra o card aparecer onde não deve.

- [ ] **Step 3a: Imports em `src/routes/Status.svelte`**

Troque a linha de import do lucide para incluir `Check`:

```ts
  import { Check, ChevronRight, Eye, Search, Unlink, X } from "@lucide/svelte";
```

E acrescente, logo abaixo do import de `checkIssue.js`:

```ts
  import { onboardingSteps, allDone } from "../lib/domain/onboarding.js";
  import { onboardingDismissed } from "../lib/stores/onboarding.js";
```

- [ ] **Step 3b: Chaves novas no objeto `T`**

Dentro do objeto `$: T = $locale && { ... }`, logo antes de `reportProblems:`, acrescente:

```ts
    onboardingTitle: m.onboarding_title(),
    onboardingDismiss: m.onboarding_dismiss(),
    onboardingStepLibrary: m.onboarding_step_library(),
    onboardingStepLibraryHint: m.onboarding_step_library_hint(),
    onboardingStepSource: m.onboarding_step_source(),
    onboardingStepSourceHint: m.onboarding_step_source_hint(),
    onboardingStepCheck: m.onboarding_step_check(),
    onboardingActionConfigure: m.onboarding_action_configure(),
    onboardingActionConnectAnilist: m.onboarding_action_connect_anilist(),
    onboardingActionCheckNow: m.onboarding_action_check_now(),
```

`onboarding_step_check_hint` **não** entra aqui de propósito: `T` só re-avalia quando `$locale` muda, e `{minutes}` interpola `checkInterval`, que chega depois do `getConfig()`. Ela é chamada inline no markup.

- [ ] **Step 3c: Estado novo**

Logo abaixo de `let libraryConfigured = true;` (mantenha essa linha e o comentário dela intactos — o `true` inicial é o que evita o botão "+ Adicionar anime" piscar desabilitado durante o load), acrescente:

```ts
  // O caminho cru e os usernames, além do booleano: `onboardingSteps` recebe a config, não um
  // booleano já cozido. Nenhuma requisição nova — os dois já vêm do mesmo `getConfig()`.
  let completedPath = "";
  let anilistUsernames: string[] = [];
```

E em `loadInitialData`, substitua as duas linhas de config por quatro:

```ts
      checkInterval = configData.check_interval;
      completedPath = configData.completed_anime_path ?? "";
      anilistUsernames = configData.anilist_usernames ?? [];
      libraryConfigured = Boolean(completedPath);
```

- [ ] **Step 3d: Reativos**

Logo abaixo de `$: passErrorText = ...`, acrescente:

```ts
  $: onboarding = onboardingSteps(
    { completed_anime_path: completedPath, anilist_usernames: anilistUsernames },
    animes,
    status,
  );
  // Verde nos três = some, sem clique. A dispensa é o segundo caminho, e é permanente.
  $: showOnboarding = !allDone(onboarding) && !$onboardingDismissed;
```

- [ ] **Step 3e: O markup**

Em `src/routes/Status.svelte`, insira o bloco abaixo **entre** o fechamento do alerta de erro de passe (`{/if}`, logo depois da `<div role="alert">` que renderiza `{passErrorText}`) e o comentário `<!-- Estado vazio é a regra: ... -->` que precede `{#if hasReport && lastCheck}`:

```svelte
    <!-- Primeiros passos. Só existe enquanto ALGUM item está vermelho — com os três verdes o
         card some sozinho, sem clique. Fica depois dos alertas (que são urgentes e do agora) e
         antes do relatório e do herói. Nenhuma requisição nova: os três itens são derivados do
         que `loadInitialData` já buscou. A dispensa é por navegador (localStorage), não campo
         do config.json — ver decisions.md. -->
    {#if showOnboarding}
      <section data-testid="onboarding-card" class="rounded-card border border-default bg-card p-4.5">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <p class="font-mono text-mono-label uppercase text-subtle">{T && T.onboardingTitle}</p>
          <!-- Botão de TEXTO, não um "×": num card de três itens o × lê como "fechar até
               recarregar", e o comportamento é permanente. O rótulo tem que dizer isso. -->
          <button
            type="button"
            class="text-caption text-subtle underline-offset-2 transition-colors hover:text-body hover:underline"
            on:click={() => onboardingDismissed.set(true)}
          >
            {T && T.onboardingDismiss}
          </button>
        </div>

        <ol class="mt-3.5 space-y-3.5">
          <!-- ① Pasta da biblioteca -->
          <li class="flex items-start gap-2.5">
            {#if onboarding.library}
              <Check size={16} strokeWidth={3} class="mt-0.5 shrink-0 text-ok" aria-hidden="true" />
            {:else}
              <span class="mt-1.5 inline-block h-2 w-2 shrink-0 rounded-full bg-warn" aria-hidden="true"></span>
            {/if}
            <div class="min-w-0 flex-1">
              <p class="text-copy font-semibold {onboarding.library ? 'text-subtle' : 'text-heading'}">
                {T && T.onboardingStepLibrary}
              </p>
              <p class="mt-0.5 text-caption text-subtle">{T && T.onboardingStepLibraryHint}</p>
              {#if !onboarding.library}
                <div class="mt-2 flex flex-wrap gap-2">
                  <!-- `library` já é o grupo inicial de Configurações, então sem `?group=`. -->
                  <Button href="#/config" variant="ghost">{T && T.onboardingActionConfigure}</Button>
                </div>
              {/if}
            </div>
          </li>

          <!-- ② Animes a acompanhar. DUAS ações porque há dois caminhos legítimos: a conta
               AniList deixou de ser obrigatória quando animes avulsos passaram a existir, e
               oferecer só um mentiria sobre o modelo. O botão de "+ Adicionar anime" herda o
               mesmo bloqueio do cabeçalho quando não há biblioteca, então a dependência de ②
               em ① se expressa sozinha, sem regra nova. -->
          <li class="flex items-start gap-2.5">
            {#if onboarding.source}
              <Check size={16} strokeWidth={3} class="mt-0.5 shrink-0 text-ok" aria-hidden="true" />
            {:else}
              <span class="mt-1.5 inline-block h-2 w-2 shrink-0 rounded-full bg-warn" aria-hidden="true"></span>
            {/if}
            <div class="min-w-0 flex-1">
              <p class="text-copy font-semibold {onboarding.source ? 'text-subtle' : 'text-heading'}">
                {T && T.onboardingStepSource}
              </p>
              <p class="mt-0.5 text-caption text-subtle">{T && T.onboardingStepSourceHint}</p>
              {#if !onboarding.source}
                <div class="mt-2 flex flex-wrap gap-2">
                  <Button href="#/config?group=anilist" variant="ghost">
                    {T && T.onboardingActionConnectAnilist}
                  </Button>
                  {#if libraryConfigured}
                    <Button href="#/add" variant="ghost">+ {$locale && m.nav_add_anime()}</Button>
                  {:else}
                    <!-- Tooltip no wrapper: um botão desabilitado não emite eventos de mouse
                         (mesmo padrão do cabeçalho desta tela e do NavRail). -->
                    <div class="tooltip tooltip-bottom" data-tip={$locale && m.add_library_required()}>
                      <Button variant="ghost" disabled>+ {$locale && m.nav_add_anime()}</Button>
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
          </li>

          <!-- ③ Primeira verificação -->
          <li class="flex items-start gap-2.5">
            {#if onboarding.firstCheck}
              <Check size={16} strokeWidth={3} class="mt-0.5 shrink-0 text-ok" aria-hidden="true" />
            {:else}
              <span class="mt-1.5 inline-block h-2 w-2 shrink-0 rounded-full bg-warn" aria-hidden="true"></span>
            {/if}
            <div class="min-w-0 flex-1">
              <p class="text-copy font-semibold {onboarding.firstCheck ? 'text-subtle' : 'text-heading'}">
                {T && T.onboardingStepCheck}
              </p>
              <p class="mt-0.5 text-caption text-subtle">
                {$locale && m.onboarding_step_check_hint({ minutes: checkInterval })}
              </p>
              {#if !onboarding.firstCheck}
                <div class="mt-2 flex flex-wrap gap-2">
                  <Button
                    variant="ghost"
                    on:click={handleCheck}
                    disabled={status.status === "checking" || actionLoading}
                  >
                    {T && T.onboardingActionCheckNow}
                  </Button>
                </div>
              {/if}
            </div>
          </li>
        </ol>
      </section>
    {/if}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd src/internal/frontend && bun run check && bun run test:smoke -- tests/smoke/status.spec.ts
```

Expected: PASS — os três testes novos e os pré-existentes de `status.spec.ts`. Se `some quando pasta, fonte e primeira verificação estão prontos` falhar, a condição de render está errada (o card não pode aparecer com os três verdes).

- [ ] **Step 5: Commit**

```bash
git add src/internal/frontend/src/routes/Status.svelte src/internal/frontend/tests/smoke/status.spec.ts
git commit -m "feat: card de primeiros passos na tela de status"
```

---

### Task 5: O caminho de volta em Configurações

Última linha do grupo **Biblioteca**: rótulo "First steps", dica "The intro card on the Status screen." e botão "Show again", desabilitado quando não há o que restaurar. É o grupo que hospeda a pasta para onde ① aponta, e Configurações já é a porta única de tudo que é configuração.

O desabilitado é sobre a **dispensa**, não sobre o card estar visível: com os três itens verdes o card não aparece de qualquer forma, e um botão habilitado ali seria um clique sem efeito visível.

**Files:**
- Modify: `src/routes/Config.svelte` — imports, objeto `T`, e o fim do bloco `{#if activeGroup === "library"}` (depois do campo `min_free_disk_percent`)
- Test: `tests/smoke/status.spec.ts` (mais um teste dentro do `test.describe('onboarding card')` da Task 4)

**Interfaces:**
- Consumes: `onboardingDismissed` de `../lib/stores/onboarding.js` e `m.onboarding_restore_label()` / `m.onboarding_restore_hint()` / `m.onboarding_restore_button()` (Task 3).
- Produces: nada para tasks posteriores.

- [ ] **Step 1: Write the failing test**

Acrescente dentro do `test.describe('onboarding card')` de `tests/smoke/status.spec.ts`, depois do teste do reload:

```ts
  // O caminho de volta. A rota de config já está mockada pelo `setupFreshInstall`, então a
  // tela de Configurações carrega igual.
  test('Configurações traz o card de volta com "Show again"', async ({ page }) => {
    await setupFreshInstall(page)
    await page.goto('/')

    await page.getByRole('button', { name: /don't show again/i }).click()
    await expect(page.getByTestId('onboarding-card')).toHaveCount(0)

    await page.goto('/#/config')
    const restore = page.getByRole('button', { name: /show again/i })
    await expect(restore).toBeEnabled()
    await restore.click()
    await expect(restore).toBeDisabled()

    await page.goto('/#/')
    await expect(page.getByTestId('onboarding-card')).toBeVisible()
  })

  test('"Show again" nasce desabilitado quando não há o que restaurar', async ({ page }) => {
    await setupFreshInstall(page)
    await page.goto('/#/config')

    await expect(page.getByRole('button', { name: /show again/i })).toBeDisabled()
  })
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd src/internal/frontend && bun run test:smoke -- tests/smoke/status.spec.ts
```

Expected: FAIL — os dois testes novos estouram procurando um botão `Show again` que não existe.

- [ ] **Step 3a: Imports e `T` em `src/routes/Config.svelte`**

Abaixo de `import { locale } from "../lib/stores/locale.js";` acrescente:

```ts
  import { onboardingDismissed } from "../lib/stores/onboarding.js";
```

E no objeto `$: T = $locale && { ... }`, logo antes de `btnRunCheck:`, acrescente:

```ts
    onboardingRestoreLabel: m.onboarding_restore_label(),
    onboardingRestoreHint: m.onboarding_restore_hint(),
    onboardingRestoreButton: m.onboarding_restore_button(),
```

- [ ] **Step 3b: A linha no grupo Biblioteca**

Dentro de `{#if activeGroup === "library"}`, **depois** do `<div class="p-4.5">` que contém o `Input` de `min_free_disk_percent` e antes do `{/if}` que fecha o grupo, acrescente:

```svelte
            <!-- Caminho de volta do card de primeiros passos. NÃO é campo de config: não entra
                 em `requiredChecks` nem no corpo do PUT — é preferência de UI, por navegador.
                 O divisor sai de graça do `divide-y` do container. `Input`/`Toggle` não servem
                 aqui porque os dois fazem `bind:` num campo de `config`, e não há campo.

                 O desabilitado é sobre a DISPENSA, não sobre o card estar visível: com os três
                 itens verdes o card não aparece de qualquer forma, e um botão habilitado aqui
                 seria um clique sem efeito visível. -->
            <div class="flex flex-wrap items-center justify-between gap-3 p-4.5">
              <div class="min-w-0">
                <p class="text-[14.5px] font-bold text-heading">{T && T.onboardingRestoreLabel}</p>
                <p class="text-caption text-subtle">{T && T.onboardingRestoreHint}</p>
              </div>
              <Button
                variant="ghost"
                disabled={!$onboardingDismissed}
                on:click={() => onboardingDismissed.set(false)}
              >
                {T && T.onboardingRestoreButton}
              </Button>
            </div>
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd src/internal/frontend && bun run check && bun run test:smoke
```

Expected: PASS — a suíte de smoke inteira, incluindo `config.spec.ts` (o teste `each group holds its own fields` não pode ter regredido: a linha nova é do grupo `library`, não dos outros).

- [ ] **Step 5: Commit**

```bash
git add src/internal/frontend/src/routes/Config.svelte src/internal/frontend/tests/smoke/status.spec.ts
git commit -m "feat: restaurar o card de primeiros passos em configurações"
```

---

### Task 6: Documentação e fechamento

**Files:**
- Modify: `docs/agents/architecture.md` — tabela de domínio puro, tabela de stores, e as células de `routes/Status.svelte` e `routes/Config.svelte`
- Modify: `docs/agents/decisions.md` — **uma** entrada nova no fim

**Interfaces:**
- Consumes: tudo das Tasks 1–5.
- Produces: nada.

- [ ] **Step 1: Linha nova na tabela "Pure domain logic" de `docs/agents/architecture.md`**

Na tabela que começa com `| File | Export | Purpose |` sob **Pure domain logic** (`src/lib/domain/`), acrescente depois da linha de `logLine.ts`:

```markdown
| `onboarding.ts` | `onboardingSteps(config, animes, status)`, `allDone(steps)` | The three first-run steps as booleans — library folder set, at least one anime source (AniList account **or** standalone anime), first check done. `firstCheck` rejects any year < 2010 because Go's `time.Time` zero value serializes to `0001-01-01T00:00:00Z`, a perfectly parseable date: testing only for the empty string would mark the step green on a daemon that never ran. `allDone` lives here too, so the render condition and the tests read the same rule |
```

- [ ] **Step 2: Linha nova na tabela "Stores" de `docs/agents/architecture.md`**

Na tabela sob **Stores** (`src/lib/stores/`), acrescente depois da linha de `locale.ts`:

```markdown
| `onboarding.ts` | `onboardingDismissed` | Whether the Status first-steps card was permanently dismissed. `localStorage` under `onboarding-dismissed`, same shape as `theme.ts`. Per browser on purpose, **not** a `config.json` field — see decisions.md |
```

- [ ] **Step 3: Atualizar a célula de `routes/Status.svelte` em `docs/agents/architecture.md`**

Na tabela de rotas, acrescente ao **fim** do texto da célula de `routes/Status.svelte` (antes do `|` que a fecha):

```markdown
. A full-width **first-steps card** (`data-testid="onboarding-card"`) sits after the alerts and before the last-check report and the hero: three items — library folder → anime source → first check — derived by `lib/domain/onboarding.ts` from data `loadInitialData` already fetched (**no new request**; the screen now also keeps `completed_anime_path` raw and `anilist_usernames`). It renders only while some item is still red and `onboardingDismissed` is false; item ② offers **both** `#/config?group=anilist` and `#/add`, the latter inheriting the same library-not-configured block as the header button. The dismiss control is a text button ("Don't show again"), not a `×` — the behaviour is permanent and the label has to say so
```

- [ ] **Step 4: Atualizar a célula de `routes/Config.svelte` em `docs/agents/architecture.md`**

Acrescente ao fim do texto da célula de `routes/Config.svelte`:

```markdown
. `checkQueryParams()` resolves the `URLSearchParams` **once** (`window.location.search` if present, otherwise the chunk after `?` inside the hash, since the app is a hash SPA) and reads both `missingConfig` and `group` from it — reading them in two branches would let the two diverge. `?group=<id>` opens the screen on that group, validated against the `groups` array the screen already builds; an unknown value is ignored and falls back to `library`. The Library group ends with a **First steps / Show again** row that flips `onboardingDismissed` back to false — a UI preference, so it is deliberately **not** in `requiredChecks` and not in the `PUT /config` body
```

- [ ] **Step 5: Entrada nova em `docs/agents/decisions.md`**

Acrescente ao **fim** do arquivo (a última entrada hoje é a 61; use o próximo número livre — confira com `grep -c '^### ' docs/agents/decisions.md` e com o número da última entrada):

```markdown

---

### 62. A dispensa do card de primeiros passos mora no `localStorage`, não no `config.json`

**Location:** `src/internal/frontend/src/lib/stores/onboarding.ts`; consumido por `routes/Status.svelte` (condição de render) e `routes/Config.svelte` (o botão "Mostrar novamente").

**What it looks like:** uma preferência do usuário que o app inteiro respeita, guardada no navegador em vez de no arquivo que o daemon já lê e escreve. Parece uma inconsistência — todo o resto da configuração está em `config.json`, e a "correção" óbvia é criar um campo `tutorial_dismissed` lá.

**Why it's right:** não é configuração do daemon; é preferência de UI, e o daemon não tem nada a fazer com ela. Um campo em `config.json` entraria no schema que o servidor valida, no corpo do `PUT /config` e — se alguém fosse coerente — no `requiredChecks` da tela de Configurações, três lugares para um booleano que só o navegador consome. E ele valeria para **todos** os dispositivos de uma vez: quem dispensou no desktop nunca mais veria o card no celular, onde ainda não conhece o app. `localStorage` é por navegador, que é exatamente o escopo certo, e é a mesma escolha que `theme` e `locale` já fazem neste frontend.

**Trade-off aceito:** trocar de navegador traz o card de volta, e limpar o storage do site também. Para um app self-hosted de um usuário só, ver de novo um card de três linhas é barato; o inverso — não conseguir vê-lo num dispositivo novo — é o caro.

Esconder o card com a biblioteca vazia deixa o app inoperante, e mesmo assim a dispensa é permanente: o passe de verificação continua abrindo `#/config?missingConfig=true` e o banner de lá continua aparecendo. É esse backstop que torna a dispensa permanente segura de oferecer.

**Don't "fix" by:**
- Criar `tutorial_dismissed` em `config.json` "para seguir o usuário" — põe preferência de UI num schema do daemon e uniformiza dispositivos que deveriam ser independentes.
- Trocar o botão "Não mostrar mais" por um `×` "porque é o padrão de card" — num card de três itens o `×` lê como "fechar até recarregar", e o comportamento é permanente. O rótulo é a documentação do comportamento.
- Fazer o card sumir sozinho depois de N dias/visitas — some quando os três itens ficam verdes, que é a condição que significa alguma coisa; tempo não é.
```

- [ ] **Step 6: Fechamento — rodar tudo**

```bash
cd /home/icaro/Projetos/AAD/AutoAnimeDownloader && go test ./...
cd /home/icaro/Projetos/AAD/AutoAnimeDownloader/src/internal/frontend && bun run check && bun run test:unit && bun run test:component && bun run test:smoke
```

Expected: PASS nos quatro. **Não** rode `swag init` — nenhum endpoint mudou. Se algum `.go` aparecer no `git status`, reverta: nada de Go muda neste plano.

- [ ] **Step 7: Commit**

```bash
git add docs/agents/architecture.md docs/agents/decisions.md
git commit -m "docs: card de primeiros passos e a decisão do localStorage"
```

---

## Cobertura da spec

| Seção da spec | Onde é implementada |
|---|---|
| O card: posição, "nenhuma requisição nova" | Task 4 (Steps 3c–3e) |
| Os três itens e suas ações | Task 4 (Step 3e); o alvo `?group=anilist` na Task 2 |
| Regra de "data válida" (ano ≥ 2010) | Task 1 (`isRealDate`) |
| Derivação pura em `lib/domain/onboarding.ts` + `allDone` | Task 1 |
| Textos e chaves de i18n | Task 3 (Steps 2–3) |
| Esconder (`localStorage`, botão de texto) | Task 3 (store) + Task 4 (Step 3e) |
| Caminho de volta em Configurações | Task 5 |
| Deep-link `?group=` e a unificação do `checkQueryParams` | Task 2 |
| Testes unitários | Task 1 (Step 1) |
| Testes de smoke | Task 2 (Step 1), Task 4 (Step 1), Task 5 (Step 1) |
| Fechamento (testes, docs, sem `swag`) | Task 6 |
