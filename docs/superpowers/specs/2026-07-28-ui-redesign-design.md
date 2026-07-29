# Redesign de UI — Spec

Data: 2026-07-28
Branch alvo: `feature/ui-redesign`
Handoff de origem: `design_handoff_ui_redesign/`

## 1. Enquadramento

O handoff em `design_handoff_ui_redesign/` é um pacote de artboards HTML gerado sem acesso ao
comportamento real do app. Ele acerta na linguagem visual e em alguns padrões de interação, mas
**erra nas duas direções** quanto a funcionalidade: inventa recursos que não existem e omite
recursos que existem e funcionam hoje.

**Este redesign trata o handoff como referência visual e de interação, não como spec funcional.**
A regra de decisão, aplicada em cada tela:

- Ideia de UI (layout, hierarquia, cor, tipografia, padrão de interação) → **adota**.
- Funcionalidade que o handoff presume mas o app não tem → **descarta**.
- Funcionalidade que existe hoje e o handoff não previu → **preserva**, achando lugar para ela
  no layout novo.

Nenhuma mudança de comportamento do daemon entra neste trabalho. As telas passam a mostrar o que
já fazem, melhor.

Correções factuais sobre o handoff: ele afirma que o app é React — é **Svelte 5** (Vite, Tailwind 3,
daisyUI 4, `svelte-spa-router`, i18n via Paraglide/inlang). Ele também descreve uma tela
"Biblioteca" separada, que não será criada (ver decisão D4).

## 2. Decisões

| # | Decisão | Motivo |
|---|---|---|
| D1 | Manter os dois temas (claro e escuro) | O app tem seletor Light/Dark/Sistema hoje; o handoff é dark-only. O tema claro será derivado e validado visualmente na Fase 0. |
| D2 | daisyUI vira tema custom (`aad-dark` / `aad-light`) | Resolve os dois temas sem reescrever componentes existentes e preserva o seletor atual sem mudança de código. |
| D3 | Backend só onde é barato — e, com o reframe da seção 1, **nenhuma mudança de backend entra** | Todos os itens que exigiam Go viraram funcionalidade inventada pelo handoff e foram descartados. |
| D4 | Status continua sendo uma tela só (daemon + lista de animes) | A separação em "Biblioteca" era mudança estrutural, não ideia de UI. Rotas atuais preservadas, incluindo `/status/:id`. |
| D5 | Config mantém o botão Salvar; sem autosave e sem barra de restart | `PUT /config` valida tudo de uma vez e faz I/O de filesystem (`Librarian.ProbePath`); autosave com debounce dispararia 400 no meio da digitação. |
| D6 | Branch única `feature/ui-redesign`, uma fase por commit, merge no fim | Durante a transição o app fica visualmente misto; isso fica contido na branch. |

## 3. Fora de escopo

Não entram neste trabalho e **não** viram backlog:

- Redesenho da tela de Notificações. Ela herda os tokens da Fase 0 (inevitável, já que o tema
  daisyUI muda global) e não recebe nenhum trabalho dedicado.

Não entram neste trabalho e **viram backlog** (`docs/agents/` ou issue, a critério do autor):

- `anilist_status` em `AnimeInfo` (o handoff pedia badge e filtro por status cru da AniList na
  lista de animes; o campo não existe no endpoint `/animes`).
- Flags `keep_data` / `block` / "remover torrent" na exclusão de episódio. Hoje
  `handleDeleteEpisode` (`src/internal/api/endpoint_episode_actions.go:143-151`) **sempre** remove
  os arquivos e **sempre** bloqueia o episódio. Os três toggles do artboard 4 mudariam esse
  comportamento, não só a UI.
- Campo `component` estruturado no log (origem precisa: scheduler / anilist / rss / torrent).
  A Fase 2 entrega uma aproximação derivando do `caller` do zerolog, sem custo de backend.
- **Bug: `LoopControl.UpdateInterval` nunca é chamado.** Definido em
  `src/internal/daemon/loop.go:90`, sem nenhum call site fora de testes. O intervalo fica capturado
  no closure no boot, então alterar `check_interval` na configuração salva o valor, a UI confirma o
  save, e o daemon segue no intervalo antigo até reiniciar. Todos os outros campos são relidos a
  cada passada (`verification.go:25,54`). Independente deste redesign.

## 4. Fundações

### 4.1 Tokens

Arquivo novo `src/internal/frontend/src/lib/design/tokens.css`, com as variáveis CSS consumidas
pelos dois temas daisyUI. Componentes referenciam **nomes semânticos** via Tailwind
(`bg-card`, `border-default`, `text-subtle`, `text-accent`, `bg-ok/12`); **nenhum hex literal em
componente**.

Escala escura (do handoff, autoritativa):

| Token | Valor |
|---|---|
| `--bg-window` | `#07080b` |
| `--bg-surface` | `#0c0e13` |
| `--bg-card` | `#101218` |
| `--bg-sunken` | `#0a0c11` |
| `--bg-control` | `#171a22` |
| `--bg-menu` | `#161922` |
| `--border-default` | `rgba(255,255,255,.08)` |
| `--border-divider` | `rgba(255,255,255,.045)` |
| `--text-primary` | `#e8eaf0` |
| `--text-secondary` | `#c3c9d6` |
| `--text-tertiary` | `#9aa1b2` |
| `--text-subtle` | `#7c8494` |
| `--accent` | `#8272ee` |
| `--ok` | `#4bd4a2` |
| `--warn` | `#eeb14b` |
| `--danger` | `#f27575` |
| `--neutral` | `#8d93a3` |

Texto **sobre** acento sólido, no escuro: `--on-accent` `#12111a`, `--on-ok` `#0d1b16`,
`--on-warn` `#1e1a10`, `--on-danger` `#1e1414`. No escuro o acento é claro o bastante para pedir
texto escuro por cima.

Escala clara (derivada — **valores iniciais, a validar na Fase 0**). As superfícies invertem, mas
os acentos **não podem ser reaproveitados**: `#8272ee` sobre fundo claro rende ~3.9:1, reprovando em
texto pequeno. Cada acento ganha um par escurecido para texto/borda; o hex original segue sendo
usado nos fundos tingidos, que continuam legíveis.

| Token | Valor |
|---|---|
| `--bg-window` | `#f5f6f9` |
| `--bg-surface` | `#ffffff` |
| `--bg-card` | `#ffffff` |
| `--bg-sunken` | `#eef0f5` |
| `--bg-control` | `#eef0f5` |
| `--bg-menu` | `#ffffff` |
| `--border-default` | `rgba(16,18,24,.10)` |
| `--border-divider` | `rgba(16,18,24,.06)` |
| `--text-primary` | `#14161d` |
| `--text-secondary` | `#3d4351` |
| `--text-tertiary` | `#5c6273` |
| `--text-subtle` | `#7c8494` |
| `--accent` (texto/borda/fill sólido) | `#6a55d6` |
| `--ok` (texto/borda) | `#0e7a58` |
| `--warn` (texto/borda) | `#8a5d0b` |
| `--danger` (texto/borda) | `#c53b3b` |
| `--neutral` | `#6f7788` |

Texto sobre acento sólido, no claro, **inverte**: os quatro `--on-*` viram `#ffffff`, porque no claro
o acento sólido é a variante escurecida. Componente nenhum decide isso — todos usam `--on-accent`,
`--on-ok`, `--on-warn`, `--on-danger`, e o tema resolve.

Critério de aceite dos dois temas: todo par texto/fundo usado em texto abaixo de 18px atinge
**4.5:1**; texto grande e bordas, **3:1**. Fundos tingidos ficam em 7–16% de opacidade da cor;
bordas tingidas, 22–32%.

Regra de composição herdada do handoff, a respeitar em ambos os temas: **no máximo um acento sólido
por tela**. O roxo sólido é da ação primária; o verde sólido é exclusivo de "Iniciar daemon".

### 4.2 Escalas

Entram como extensão do `tailwind.config.js`:

- Espaçamento: 4 · 6 · 8 · 10 · 14 · 18 · 22 · 26px.
- Raios: 6 (badge) · 9 (botão/ícone) · 12 (campo/bloco) · 16 (card) · 18 (modal/herói) · 999 (pill).
- Barras de progresso: 4px (dentro de linha) · 5–6px (card) · 7px (grupo) · 9–10px (barra tripla),
  sempre `border-radius:99px` com trilho `rgba(255,255,255,.07)` (escuro) / `rgba(16,18,24,.08)` (claro).
- Elevação (modal, menu): `0 24px 50px -20px rgba(0,0,0,.95)` no escuro;
  `0 16px 40px -18px rgba(16,18,24,.28)` no claro.

### 4.3 Tipografia

- **Manrope** (400/500/600/700/800) para interface — `@fontsource/manrope`.
- **JetBrains Mono** (400/500/700) para números, horários, hashes e rótulos de coluna —
  `@fontsource/jetbrains-mono`.
- Inter (`@fontsource/inter`) sai; o import em `src/app.css` e o `fontFamily.sans` do
  `tailwind.config.js` são atualizados.

Fontes **auto-hospedadas via `@fontsource`, nunca Google Fonts CDN**: o `dist` é embedado no binário
Go (`//go:embed dist`) e o daemon roda em LAN e offline. O handoff cita Google Fonts; ignorar.

Escala: título de tela 20–22px/800 `letter-spacing:-.01em` · número herói 46px/600 `-.03em` ·
número de card 22–30px/650 · título de card 14–15px/700 · corpo 13–13.5px/600 ·
secundário 12–12.5px/500 · rótulo mono 10.5–11px/600 `letter-spacing:.10–.12em` maiúsculo.

### 4.4 Ícones

`@lucide/svelte`, tree-shakeable — só os ícones importados entram no bundle, o que importa por causa
do embed no binário. Tamanho 16–20px, traço ~2px. Hoje não há biblioteca: são SVGs inline no
`Layout.svelte` e em `AnimeDetail.svelte`.

## 5. Shell de navegação

`src/internal/frontend/src/components/Layout.svelte` (230 linhas) tem os seis links de navegação
escritos **duas vezes** — bloco desktop e bloco de menu mobile — com as classes de estado ativo
repetidas em cada um. Ele é quebrado em:

| Arquivo | Responsabilidade |
|---|---|
| `components/shell/AppShell.svelte` | Grid rail + conteúdo; decide rail vs tab bar no breakpoint 768px; hospeda `Toasts` |
| `components/shell/NavRail.svelte` | Rail vertical de 74px (desktop) |
| `components/shell/NavTabBar.svelte` | Tab bar inferior de 4 colunas (mobile), com safe area |
| `components/shell/MoreMenu.svelte` | Item "Mais" → Notificações, Prioridades, Logs |
| `lib/navItems.ts` | Fonte única dos itens (rota, ícone, chave i18n, grupo) |

Estrutura do rail, dado D4 (sem tela Biblioteca):

```
Status
Downloads
──────────  (divisor 1px, 24px de largura)
Configurações
Mais  →  Notificações · Prioridades · Logs
```

Especificação visual: rail 74px, fundo `--bg-sunken`, borda direita 1px. Cada item = ícone 18px +
rótulo 9.5px/600 embaixo, `gap:5px`, alvo de 56×46px, raio 12px. Ativo: fundo do acento a 16%, texto
`#c5bcfa` (escuro) / `--accent` (claro), rótulo peso 700. Inativo: `--text-subtle`.

Badge numérico em Downloads quando há torrents ativos: círculo de 15px, fundo `--ok`, texto mono
9px/800, canto superior direito do ícone.

Mobile (<768px): tab bar de 4 colunas (Status, Downloads, Config, Mais), `padding:10px 8px 22px`
(o 22px é safe area), ícone 20px + rótulo 10.5px, alvos ≥44px.

O seletor de tema, o toggle de idioma, o ponto de conexão do WebSocket e a versão do app continuam
existindo; migram do header horizontal para o rodapé do rail (desktop) e para o menu "Mais" (mobile).

## 6. Primitivos

Novos, em `src/internal/frontend/src/components/ui/`:

| Componente | Notas |
|---|---|
| `Chip.svelte` | Variantes accent / ok / warn / danger / neutral; fundo tingido + borda tingida |
| `Button.svelte` | Variantes `solid` (acento, ação que faz avançar), `ghost`, `warn` (destrutiva/atenção) |
| `ProgressBar.svelte` | `transition: width .9s linear`; **congela** quando o dado está velho (ver §8) |
| `TripleProgressBar.svelte` | Segmentos adjacentes e somados (não sobrepostos): assistidos `--ok`, baixados `--accent`, lançados `--warn`, sobre o trilho = total. Sempre acompanhada de legenda textual |
| `ProgressRing.svelte` | 46px, `conic-gradient(<cor> <pct>%, trilho 0)` + disco interno de 34px com o percentual em mono |
| `Sparkline.svelte` | 20 barras de 7px, `gap:3px`, altura máx. 64px |
| `PulseDot.svelte` | `@keyframes` 2.4s ease-in-out infinito, `opacity 1→.35` e `scale 1→.82`. **Só** em indicadores de "vivo" |
| `Modal.svelte` | Base para os diálogos; foco preso, fecha com Esc e clique fora |
| `ActionMenu.svelte` | Menu `⋯` no desktop (min-width 236px, raio 12px, fundo `--bg-menu`, flip para cima nas últimas linhas) e action sheet no mobile (raio `20px 20px 0 0`, alça 38×4px). **Um componente, dois layouts, uma definição de itens** |
| `Cover.svelte` | Capa da AniList (`cover_image`) com fallback hachurado via `repeating-linear-gradient` |
| `Toggle.svelte`, `Checkbox.svelte` | Checkbox 16px, raio 4px; marcado = fundo acento com "✓" |

Os componentes existentes (`Loading`, `ErrorMessage`, `StatusBadge`, `Toasts`, `ConfirmDialog`,
`Input`, `DownloadsToolbar`, `TorrentDeleteDialog`) são mantidos e reestilizados, não substituídos.

## 7. Lógica pura

Novo diretório `src/internal/frontend/src/lib/domain/`, sem dependência de Svelte, coberto por
vitest:

**`animeState.ts`** — `deriveAnimeChip(anime, torrents, now)`, cascata na ordem:

```
is_blacklisted                                  → "Na blacklist"        (neutro, linha esmaecida)
torrent com status=downloading                  → "Baixando ep. N · P%" (acento)
torrent com peers_total=0 há mais de 10 min     → "Sem seeds"           (danger)
torrent com status=stopped                      → "Pausado"             (neutro)
episodes_released == 0                          → "Aguardando estreia"  (neutro)
episodes_downloaded == episodes_released        → "Em dia"              (ok)
senão                                           → "Atrasado N ep"       (warn)
```

O ramo `anilist_status` do handoff é omitido (§3). O join anime↔torrent usa `anime_id` de
`TorrentInfo`.

**`episodeActions.ts`** — o modelo de ações do artboard 4, **como dado**. Uma função
`episodeActions(ep, torrent) → { principal?: Action, menu: Action[] }`, onde `Action` é
`{ id, labelKey, variant, destructive? }`. É esta estrutura que permite ao desktop e ao mobile
renderizarem a mesma definição sem duplicá-la.

Mapeamento para as **cinco ações que existem** (`downloadEpisode`, `redownloadEpisode`,
`deleteEpisode`, `releaseEpisode`, `replaceEpisodeWithMagnet`):

| Estado do episódio | Ação principal | No menu `⋯` |
|---|---|---|
| Torrent ativo (baixando) | — | Substituir torrent · Excluir episódio… |
| No ar e não baixado | **Baixar** (solid) | Substituir torrent |
| Baixado | Rebaixar (ghost) | Substituir torrent · Excluir episódio… |
| Bloqueado ou gerenciado manualmente | **Soltar** (ghost) | Baixar · Excluir episódio… |
| Não lançou | — | — |

`Excluir` e `Rebaixar` continuam passando pelos diálogos de confirmação que já existem hoje —
exclusão **nunca** é um clique só.

**`format.ts`** — formatação sensível ao locale. pt-BR usa vírgula decimal e ponto de milhar
(`4,2 MB/s`, `1.284 ep`). Cobre velocidade, bytes, percentual, ETA e data. Substitui os helpers
espalhados em `lib/utils/torrents.ts` e `lib/utils/status.ts` onde houver sobreposição; o resto
permanece.

**`logSource.ts`** — mapeia o `caller` do zerolog (`verification.go:120`) para a origem exibida
(scheduler / anilist / rss / torrent / api). Aproximação deliberada, sem custo de backend; o campo
estruturado está no backlog (§3).

**`torrentStatusLabel`** — os estados crus aparecem sempre **traduzidos**, nunca como identificador:
`downloading`→"Baixando", `downloading_metadata`→"Buscando metadata", `verifying`→"Verificando",
`allocating`→"Alocando", `stopped`→"Pausado", `seeding`→"Seeding". Isso já existe em
`lib/utils/torrentStatus.ts` e é preservado.

## 8. Stores

- **`speedHistory.ts`** — últimas 20 amostras de velocidade agregada, alimentando o sparkline.
  Regra crítica: quando um poll falha, o histórico **congela** e a barra para de avançar. Não
  extrapolar. O handoff é explícito: *não fingir stream contínuo*.
- **`stallTracker.ts`** — por hash, desde quando `peers_total == 0`. Alimenta o chip "Sem seeds"
  (limiar de 10 min). Vive em memória; zera no reload da página, o que é aceitável.

O WebSocket **não transmite progresso de torrent** — só estado do daemon (`status`, `lastCheck`,
`hasError`). O progresso vem do polling HTTP de `GET /api/v1/torrents`. A UI reflete isso:
indicador de conexão discreto (ponto de 6–8px), banner visível **apenas quando desconectado**, com
texto avisando que o progresso pode estar defasado, e a nota "polling 2s" em mono pequeno perto do
resumo de banda.

## 9. Telas

### 9.1 Status (artboards 1b + 1d fundidos) — `routes/Status.svelte`

Continua sendo daemon **e** lista de animes (D4). Layout novo, de cima para baixo:

1. **Header**: título "Status"; pílula do daemon (`border-radius:999px`, ponto com `pulseDot`,
   rótulo, divisor vertical, "há N min" em mono); botões **Verificar agora** e **Parar daemon**.
   Com o daemon parado, a pílula troca para o esquema neutro/vermelho e o botão vira **Iniciar
   daemon** em `--ok` sólido — a única ação verde sólida do app.
2. **Card herói** (esquerda, `1.15fr`): "VELOCIDADE TOTAL" em rótulo mono, número 46px + "MB/s",
   subtítulo "N baixando · N seeding · X MB/s ↑", `Sparkline` à direita, divisor, e uma linha de
   `ProgressRing` (um por download ativo) com nome curto do anime e "ep. N · N min".
3. **Coluna direita** (`1fr`): card de resumo da biblioteca (`TripleProgressBar` + legenda) e card
   duplo Espaço em disco / Próxima verificação. O aviso de disco baixo (`isDiskSpaceLow`) é mantido.
4. **Lista de animes** (largura total): capa 32×44 + título, `Chip` derivado, `TripleProgressBar`
   com legenda textual, última baixa, e link para `/status/:id`. Linhas na blacklist com
   `opacity:.45` na capa e no título.

**Preservar** (o handoff não previu): busca por texto; filtro "não assistidos"; ordenação por
**nome**, **assistidos** e **última baixa**, com cabeçalho clicável e indicador de direção
(`handleSort`, `sortAnimes`, `SortKey`, `SortDir` em `lib/utils/status.ts`); estados vazios atuais,
incluindo o "vá para configurações" quando não há animes.

**Descartar** do artboard 1b: a seção "Fila e problemas" — é redundante com a tela de Downloads, que
existe justamente para isso.

Mobile: os filtros viram faixa rolável horizontal (`overflow-x:auto`; o container precisa de
`flex:0 0 auto` para não colapsar quando a lista cresce) e a tabela vira cards empilhados.

### 9.2 Anime (artboard 4) — `routes/AnimeDetail.svelte`

A tela de maior ganho. Hoje tem 971 linhas, em boa parte porque a lista de ações está escrita duas
vezes: tabela desktop (~linhas 720–790) e cards mobile (~linhas 895–960), com os mesmos cinco botões
**só-ícone** (`title` como única pista textual), que aparecem e somem conforme o estado e por isso
mudam de posição entre linhas.

- **Cabeçalho**: breadcrumb "Status / <anime>", capa 78×110, título 22px/700, `Chip` do estado
  derivado, contagem "N / N · N assistidos", e **Colar magnet** (secundária, chama
  `replaceAnimeWithMagnet`) à direita.
- **Busca customizada do anime** (`custom_search_query` / `updateAnimeSettings`): o handoff esqueceu
  dela. Fica num bloco recolhível logo abaixo do cabeçalho, com o rótulo e o texto de ajuda atuais.
- **Barra de seleção múltipla**, quando há ≥1 selecionado: fundo do acento a 9%, borda inferior do
  acento, contagem à esquerda e as ações em lote **que existem hoje** — Baixar, Soltar, Excluir —
  à direita. Bulk "Rebaixar" **não** é adicionado (seria funcionalidade nova).
- **Tabela**: checkbox · número do ep em mono 14px/700 · título + nota secundária · `Chip` de estado
  com `ProgressBar` fina de 4px embaixo quando há torrent em voo · metadado (data de exibição,
  data de download, tamanho, velocidade) · coluna de ações.
- **Coluna de ações**: uma ação principal **com texto, sempre na mesma coluna**, mais um `⋯`
  (32px, raio 9px) com o resto **também rotulado**. Alimentada por `episodeActions.ts` (§7),
  renderizada por `ActionMenu` — desktop e mobile lendo a mesma definição.

Regra do inline de progresso, a **preservar exatamente**, com o comentário que a explica
(`AnimeDetail.svelte:68-74`): um episódio só ganha `episode_hash` quando o daemon grava o registro
salvo, no momento em que o torrent é *adicionado*, não quando termina. Como
`endpoint_anime_episodes.go` preenche `is_downloaded` e `episode_hash` no mesmo `if`, vale
`episode_hash != "" ⟺ is_downloaded`. A condição do inline **não pode exigir `!ep.is_downloaded`** —
essa combinação nunca ocorre e a barra nunca apareceria. Quem decide é só `!torrent.completed`.

Preservar também o polling adaptativo (2s com torrent ativo, 15s sem), reagendado por `setTimeout`
em vez de `setInterval` para nunca rodar dois timers ao mesmo tempo.

Corrigir: `title="Soltar episódio"` está hardcoded em português, fora do i18n, nas duas cópias
(`AnimeDetail.svelte:785` e `:956`). Vira chave de mensagem.

### 9.3 Downloads (artboard 1c) — `routes/Downloads.svelte`

- **Barra de resumo** no header: ↓ em `--ok` e ↑ em neutro, mono 14px/700, num bloco com raio 10px,
  divisores de 1×14px, e "N baixando · N seeding" à direita. Indicador discreto "polling 2s" com
  ponto de 6px.
- **Banner de WebSocket caído**, só quando desconectado: faixa âmbar, "Sem conexão com o daemon — o
  progresso abaixo pode estar defasado", com link "Reconectar".
- **Toolbar de seleção**: "Selecionar todos" e as quatro ações em lote que existem — Pausar,
  Retomar, Reanunciar, Remover… Com nada selecionado, o grupo fica em `opacity:.45` e desabilitado;
  o rótulo troca para "N selecionados".
- **Filtros** em pills com contagem: Todos / Baixando / Seeding / Problemas.
- **Accordion por anime** (agrupamento derivado de `anime_id`/`anime_name`, já presentes em
  `TorrentInfo`). Cabeçalho do grupo clicável inteiro: checkbox, capa 34×46, título 15px/800, chip
  de contagem, `ProgressBar` agregada de 7px, percentual agregado em mono, metadado, ações em lote
  do grupo em quadrados de 32px, caret. Linhas de torrent com recuo de 66px: checkbox, episódio em
  mono, chip do estado traduzido, hash truncado (`a3f9c1e8…7b2d`) em mono, barra de 5px, tamanho /
  peers / percentual, ações individuais em quadrados de 28px.
- **Ordem**: problemas primeiro, depois baixando, depois seeding. Estado do accordion é por grupo,
  não global.

**Preservar** (o handoff não previu): busca por texto; ordenação por coluna (nome, progresso,
velocidade, ETA, peers); e — importante — o **estado da view codificado na URL**
(`encodeViewState`/`decodeViewState` em `lib/utils/torrentFilters.ts`), decisão deliberada e
documentada em comentário. Filtro, busca, ordenação **e** o conjunto de grupos abertos continuam
refletidos na querystring. Preservar também a poda da seleção ao que está visível pós-filtro
(`pruneSelectionToVisible`) e o `busy` por hash, que desabilita a linha em ação sem congelar a
tabela.

`TorrentDeleteDialog` (com `keep_data` e `block`) é mantido e reestilizado no formato do artboard
1e: bloco de resumo do que sai e uma frase de consequência real por toggle.

### 9.4 Config (artboard 3a) — `routes/Config.svelte`

Re-skin, sem mudança de comportamento (D5). Índice lateral de 196px com os grupos, um grupo visível
por vez, item ativo com o mesmo tratamento do rail. Dentro do grupo, campos separados por divisores
de 1px, cada um com rótulo 13.5px/700, o controle e uma linha de ajuda 12px em `--text-subtle`.
Multi-seleção de status da AniList como pills toggle com "✓". O botão **Salvar** permanece.

Os campos de lista (`anilist_usernames`, `excluded_lists`) adotam o **input multi-valor** do artboard
1e: chips dentro do campo, cursor na mesma caixa, placeholder "Digite e aperte Enter", **sem botão
"+"**. Enter e vírgula confirmam; Backspace com campo vazio remove o último; clicar no × remove;
colar lista separada por vírgulas cria vários. Texto de ajuda embaixo do campo.

### 9.5 Logs (artboard 3c) — `routes/Logs.svelte`

Grid de 4 colunas `82px 60px 90px 1fr`: horário em mono, badge de nível, origem (via `logSource.ts`)
em mono 11.5px, mensagem em mono 12.5px. Fundo do corpo em `--bg-sunken`, mais escuro que os cards,
como um terminal. Preservar o que já existe: filtros por nível com contagem, busca com destaque do
trecho encontrado, seletor de número de linhas, recarga automática e o toggle de acompanhar o fim
(que ganha o `PulseDot`).

### 9.6 Prioridades e Notificações

Sem trabalho dedicado. Herdam os tokens da Fase 0 e continuam funcionando como hoje. Prioridades
segue acessível pelo menu "Mais".

## 10. Fases

Uma fase por commit, na branch `feature/ui-redesign`, merge no fim.

| Fase | Escopo | Entregável verificável |
|---|---|---|
| 0 | `tokens.css`, temas `aad-dark`/`aad-light`, `theme.ts` apontando para eles, fontes, ícones | App inteiro repintado com layout antigo; **validação visual dos dois temas, com o claro conferido contra o critério de contraste da §4.1** |
| 1 | `AppShell`, `NavRail`, `NavTabBar`, `MoreMenu`, `navItems.ts`; `Layout.svelte` desmontado | Navegação nova em desktop e mobile; nenhuma rota quebrada |
| 2 | Primitivos de `components/ui/` + `lib/domain/` + `lib/stores/` novos | Testes vitest da lógica pura passando; nenhuma tela alterada |
| 3 | Status (§9.1) | Busca, ordenação e filtro preservados e cobertos por smoke test |
| 4 | Anime (§9.2) | Duas cópias de ação colapsadas em uma; busca customizada presente; `title` hardcoded corrigido |
| 5 | Downloads (§9.3) | Estado da view ainda na URL; seleção ainda podada ao visível |
| 6 | Config (§9.4) | Salvar funciona; inputs multi-valor com as cinco regras de teclado |
| 7 | Logs (§9.5) | Filtro, busca e follow preservados |

## 11. Testes

- **Lógica pura** (Fase 2): vitest para `animeState`, `episodeActions`, `format`, `logSource`,
  `speedHistory`, `stallTracker`. `episodeActions` recebe um caso por linha da tabela da §7.
- **Componente**: `@testing-library/svelte` para `ActionMenu` (abre um por vez, fecha com Esc,
  clique fora e ao escolher item; flip para cima) e `ChipsInput` (Enter, vírgula, Backspace em campo
  vazio, ×, colar lista).
- **Smoke (Playwright)**: os specs existentes em `tests/smoke/` (`status`, `downloads`,
  `anime-detail`, `config`, `interactions`) quebram por seletor e texto. Cada fase atualiza o spec
  da sua tela **na mesma fase**, não no fim.
- `go test ./...` continua verde: nenhuma mudança de backend entra neste trabalho.

## 12. Riscos

| Risco | Mitigação |
|---|---|
| Tema claro sem referência de design | Fase 0 entrega os dois temas antes de qualquer mudança de layout; validação visual e critério de contraste explícito antes de seguir |
| Regressão silenciosa de funcionalidade que o handoff omitiu | As listas de "preservar" em §9 são o checklist; cada fase atualiza o smoke test da sua tela |
| Bundle maior no binário Go (duas famílias de fonte) | `@fontsource` com os pesos exatos listados na §4.3; `@lucide/svelte` tree-shakeable; conferir o tamanho do `dist` ao fim da Fase 0 |
| Quebra do inline de progresso em AnimeDetail | A regra `episode_hash != "" ⟺ is_downloaded` está transcrita na §9.2 e o comentário original é mantido no código |
