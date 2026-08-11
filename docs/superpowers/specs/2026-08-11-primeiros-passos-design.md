# Primeiros passos: guiar a primeira execução

Data: 2026-08-11

**Dependência:** `2026-08-11-relatorio-ultima-verificacao-design.md` precisa estar implementado
antes. Os dois desenham cards na tela de Status e o relatório é quem responde "por que nada
baixou"; esta spec assume aquele card existindo e não repete a pergunta.

## Problema

Instalação nova: `main.go:279` detecta `!fileManager.ConfigExists()`, espera a porta subir e abre o
navegador em `#/status`. O usuário cai numa tela vazia com um daemon parado e nenhuma indicação do
que fazer. Tudo o que o app sabe sobre a primeira execução morre naquela variável local — o frontend
nunca fica sabendo.

O que existe hoje de orientação, e por que não basta:

| Mecanismo | Quando aparece | Limite |
|---|---|---|
| `#/config?missingConfig=true` | passe de verificação sem `completed_anime_path` | só depois de um passe rodar, e só sobre a pasta |
| Estado vazio da lista de animes | zero animes na tela de Status | dois botões sem texto que explique a ordem ou o porquê |
| Botão "+ Adicionar anime" desabilitado | sem biblioteca configurada | diz que não pode, não diz o que fazer |

Nenhum deles diz o que o daemon **é**: que ele varre uma lista a cada N minutos, procura no Nyaa e
baixa o que falta. Esse modelo mental não está escrito em lugar nenhum da UI.

## Objetivo e linha de chegada

A linha de chegada do primeiro uso é **pasta configurada + pelo menos uma fonte de animes** (conta
AniList *ou* anime avulso). Sem fonte, o daemon roda em loop sobre uma lista vazia e mostra
"Rodando" com zero downloads — o sintoma mais parecido com um bug que o app produz.

Não é linha de chegada "ver um download progredindo": isso depende de o Nyaa ter resultado, e um
tutorial que falha por motivo alheio ao usuário é pior que nenhum.

## Não-objetivos

- **Descoberta de features.** Prioridades e Notificações estão atrás do menu "Mais" de propósito;
  quem nunca mexer nelas não perdeu nada. Um item de notificações na checklist nunca ficaria verde
  sozinho e manteria o card na tela para sempre.
- **Diagnóstico de download.** É o spec do relatório da última verificação.
- **Mudança de comportamento do daemon.** Nada aqui muda o que é baixado, quando, ou como. É tudo
  frontend, mais um parâmetro de query.
- **Wizard.** Nenhuma rota nova, nenhum passo bloqueante, nenhum estado "meio caminho andado".

## O card

`routes/Status.svelte`, largura total, **depois** dos alertas (`disk_low`, erro de passe) e **antes**
do card herói de velocidade.

Fica acima do card de problemas do spec do relatório (que mora logo acima da lista de animes) e os
dois quase nunca coexistem: o card de problemas só tem conteúdo depois de um passe com animes, e
nesse ponto os itens ① e ② já estão verdes.

**Nenhuma requisição nova.** `loadInitialData` já busca `getStatus()`, `getAnimes()` e `getConfig()`
em paralelo. A tela passa a guardar `anilist_usernames` além do `completed_anime_path` que já vira
`libraryConfigured`.

### Os três itens

| # | Verde quando | Ações |
|---|---|---|
| ① Pasta da biblioteca | `config.completed_anime_path !== ""` | → `#/config` (o grupo `library` já é o inicial) |
| ② Animes a acompanhar | `anilist_usernames.length > 0 \|\| animes.length > 0` | → `#/config?group=anilist` **e** → `#/add` |
| ③ Primeira verificação | `status.last_check` é data válida | → "Verificar agora" (`handleCheck`, já existe) |

② tem duas ações porque tem dois caminhos legítimos: a conta AniList deixou de ser obrigatória
quando animes avulsos passaram a existir, e oferecer só um caminho mentiria sobre o modelo. O botão
`#/add` herda o desabilitado que a tela já aplica sem biblioteca (`libraryConfigured` +
`add_library_required`), então a dependência de ② em ① se expressa sozinha, sem regra nova.

"Data válida" em ③ é a mesma regra que `Status.svelte` já aplica em `formatDate`/`formatTimeAgo`:
string não vazia, `Date` parseável e **ano ≥ 2010**. O corte do ano existe porque o zero-value de
`time.Time` do Go serializa como `0001-01-01T00:00:00Z`, que é uma data perfeitamente parseável — só
testar string vazia deixaria ③ verde num daemon que nunca rodou.

O card só renderiza quando **algum** item está vermelho. Verde nos três = some, sem clique.

### Derivação

Função pura em `lib/domain/onboarding.ts`, no molde de `animeState.ts` e `utils/status.ts`:

```ts
export interface OnboardingSteps {
  library: boolean
  source: boolean
  firstCheck: boolean
}

export function onboardingSteps(
  config: Pick<Config, 'completed_anime_path' | 'anilist_usernames'>,
  animes: AnimeInfo[],
  status: StatusResponse | null,
): OnboardingSteps
```

A tela não decide nada, só pinta. É a mesma fronteira que `lib/domain/animeState.ts` documenta: a
lógica sai do componente para poder ser testada sem DOM.

`allDone(steps)` — os três verdes — é derivado aqui também, para que a condição de render e os testes
leiam a mesma regra.

### Textos

Uma linha por item. É onde o modelo mental do app cabe, e é o único lugar da UI que o diz.

**① Escolha a pasta da biblioteca**
> É onde os episódios ficam. O daemon baixa e continua semeando numa subpasta `.torrents` dela, e
> cria um link do episódio na pasta do anime — sem duplicar espaço.

**② Diga quais animes acompanhar**
> Ligue uma conta AniList para seguir sua lista automaticamente, ou adicione animes avulsos um a um.
> Os dois funcionam juntos.

**③ Aguarde a primeira verificação**
> A cada {minutes} minutos o daemon lê sua lista, procura no Nyaa e baixa os episódios que faltam.
> Daqui pra frente você não precisa fazer nada.

`{minutes}` interpola `check_interval`, que a tela já carrega em `checkInterval`.

Chaves novas em `messages/{pt-BR,en}.json`, compiladas pelo paraglide (`bun run i18n:compile`, já
embutido nos scripts de build e teste; o diretório compilado é gitignored):

| Chave | Uso |
|---|---|
| `onboarding_title` | título do card |
| `onboarding_step_library` / `..._library_hint` | ① |
| `onboarding_step_source` / `..._source_hint` | ② |
| `onboarding_step_check` / `..._check_hint` | ③, com `{minutes}` |
| `onboarding_dismiss` | "Não mostrar mais" |
| `onboarding_action_configure` / `..._connect_anilist` / `..._check_now` | botões |
| `onboarding_restore_label` / `..._restore_hint` / `..._restore_button` | a linha em Configurações |

As ações de ② reaproveitam `nav_add_anime`, que já existe.

## Esconder

`lib/stores/onboarding.ts`, no molde de `lib/stores/theme.ts`: `writable<boolean>` inicializado de
`localStorage` sob a chave `onboarding-dismissed`, gravando no `set`, com a guarda `isBrowser` que
aquele arquivo já usa.

Condição de render do card: `!allDone(steps) && !$onboardingDismissed`.

O controle é um botão de texto **"Não mostrar mais"** no cabeçalho do card, não um `×`. Um `×` num
card de três itens lê como "fechar até recarregar", e o comportamento é permanente — o rótulo tem
que dizer isso.

### Por que localStorage e não config

`localStorage` é por navegador: quem abre o daemon do celular depois de dispensar no desktop vê o
card de novo. A alternativa — um campo `tutorial_dismissed` no `config.json` — seguiria o usuário,
mas colocaria uma preferência de UI num schema que o daemon lê e a faria valer para todos os
dispositivos de uma vez. Para um app self-hosted de um usuário, a troca é essa, e é a mesma que
`theme` e `locale` já fazem.

Esconder com a biblioteca vazia deixa o app inoperante, mas **não** mudo: o passe de verificação
continua abrindo `#/config?missingConfig=true` e o banner de lá continua aparecendo. Esse backstop é
o que torna a dispensa permanente segura de oferecer.

## Caminho de volta

Última linha do grupo **Biblioteca** em `routes/Config.svelte`, separada por divisor: rótulo
"Primeiros passos", dica "O card de introdução na tela de Status.", botão "Mostrar novamente",
desabilitado quando `!$onboardingDismissed` — não há o que restaurar. A condição é sobre a dispensa,
não sobre o card estar visível: com os três itens verdes o card não aparece de qualquer forma, e um
botão habilitado ali seria um clique sem efeito visível.

É o grupo que hospeda a pasta para onde ① aponta, e a spec de reorganização daquela tela já declarou
Configurações como a porta única de tudo que é configuração.

As alternativas custam mais. `NavRail` e `MoreMenu` hospedam o seletor de tema **duplicado de
propósito** — dois blocos no DOM, pelo motivo registrado em `NavRail.svelte:5` (ids duplicados) —
então colocar lá é escrever a mesma marcação duas vezes para um undo raro. O estado vazio da lista
de animes é mais barato ainda, mas só aparece com zero animes, deixando de fora quem tem animes e um
① ou ③ vermelho.

**Não entra em `requiredChecks` nem no corpo do `PUT /config`.** Não é campo de config; é preferência
de UI, e o payload não muda.

## Deep-link `?group=`

`checkQueryParams()` (`Config.svelte:184`) já lê a query de `window.location.search` **ou** do trecho
depois do `?` no hash, porque a app é um SPA de hash. Hoje cada ramo lê `missingConfig` e o primeiro
faz `return`; a função passa a resolver o `URLSearchParams` **uma vez** (`search` se houver, senão o
trecho do hash) e ler os dois parâmetros dele. Sem essa unificação, `group` teria que ser lido em
dois lugares e os dois poderiam divergir:

```ts
const params = resolveParams()          // search, senão hash
if (!params) return
showMissingConfigBanner = params.has("missingConfig")
const group = params.get("group")
if (group && groups.some((g) => g.id === group)) activeGroup = group as GroupId
```

A validação usa o array `groups` que a tela já monta — não uma segunda lista de ids que precisaria
ser mantida em dia com ele. Valor desconhecido é ignorado e `activeGroup` fica no default
`"library"`: sem essa guarda, um link velho ou digitado errado deixaria a tela num grupo que não
renderiza nada.

Ordem em relação à validação: `firstValidationError()` só mexe em `activeGroup` no clique de Salvar,
então não há conflito com a leitura da query no `onMount`.

## Testes

### `tests/unit/onboarding.test.ts`

- `library`: verde com caminho, vermelho com `""`.
- `source`: verde com `anilist_usernames` preenchido e **zero** animes; verde com zero usernames e um
  anime; vermelho com os dois vazios. É o "ou" que estes três casos protegem.
- `firstCheck`: vermelho com `status === null`, com `last_check` vazio e com `last_check` de ano
  < 2010 (o zero-value do Go); verde com data recente.
- `allDone`: verdadeiro só com os três verdes.

### `tests/smoke/status.spec.ts` (arquivo existente)

- Config vazia → card na tela, três itens.
- Config completa + um anime + `last_check` válido → card ausente.
- "Não mostrar mais" → card some, e continua ausente depois de `page.reload()`.
- `#/config` → "Mostrar novamente" → card de volta no Status.
- `#/config?group=anilist` abre no grupo Anilist.

Sem teste do texto dos itens: é i18n, e afirmar a frase seria testar a tradução em vez do
comportamento.

## Fechamento

- `go test ./...` (nada de Go muda, mas é o checklist do repositório)
- `bun run test:unit` e `bun run test:smoke`
- **Sem `swag init`** — nenhum endpoint muda.
- `docs/agents/architecture.md`: o card em `routes/Status.svelte`, os arquivos novos
  `lib/domain/onboarding.ts` e `lib/stores/onboarding.ts`, e o parâmetro `?group=` em
  `Config.svelte`.
- `docs/agents/decisions.md`: **uma** entrada — a dispensa mora em `localStorage`, não no
  `config.json`, porque é preferência de UI por navegador. É a decisão que alguém tentaria
  "consertar" depois transformando em campo de config.
