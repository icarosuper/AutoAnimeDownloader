# Architectural Decisions

Patterns that look wrong but are intentional. Read before "fixing" anything.

## Índice

As entradas **não estão em ordem numérica** no arquivo — use este índice, ou `grep -n "^### 59\."`.
Cada entrada é autocontida: leia só a que a referência aponta, não o arquivo inteiro.

- [**#1** — Var-swap mocking for anilist and nyaa HTTP clients](#1-var-swap-mocking-for-anilist-and-nyaa-http-clients)
- [**#2** — FileManagerInterface declared twice](#2-filemanagerinterface-declared-twice)
- [**#3** — JSONL format for episodes.json with full-file rewrite on save](#3-jsonl-format-for-episodesjson-with-full-file-rewrite-on-save)
- [**#4** — cancelPtr / donePtr pointer mutation for runtime interval updates](#4-cancelptr--doneptr-pointer-mutation-for-runtime-interval-updates)
- [**#5** — State notifier called outside the mutex lock](#5-state-notifier-called-outside-the-mutex-lock)
- [**#6** — Non-blocking send on WebSocket broadcast channel](#6-non-blocking-send-on-websocket-broadcast-channel)
- [**#7** — Never-closing channel as headless tray fallback](#7-never-closing-channel-as-headless-tray-fallback)
- [**#8** — Hard part filter — nil-part torrents rejected when requestedPart is set](#8-hard-part-filter--nil-part-torrents-rejected-when-requestedpart-is-set)
- [**#9** — PREQUEL offset gate requires part ≥ 2](#9-prequel-offset-gate-requires-part--2)
- [**#10** — "Cour N" treated as Part N, not as a distinct concept](#10-cour-n-treated-as-part-n-not-as-a-distinct-concept)
- [**#11** — GetCustomListsMap — separate lightweight query + cache for customLists](#11-getcustomlistsmap--separate-lightweight-query--cache-for-customlists)
- [**#12** — Build logic lives in scripts/build.sh, not in Makefile targets](#12-build-logic-lives-in-scriptsbuildsh-not-in-makefile-targets)
- [**#13** — GetFrontendAnimeList — separate lighter Anilist query for the API endpoint](#13-getfrontendanimelist--separate-lighter-anilist-query-for-the-api-endpoint)
- [**#14** — nyaaSearcher — dependency injection for Nyaa search in processAnimeEpisodes](#14-nyaasearcher--dependency-injection-for-nyaa-search-in-processanimeepisodes)
- [**#15** — atomic.Pointer[Priorities] package-level global in nyaa](#15-atomicpointerpriorities-package-level-global-in-nyaa)
- [**#16** — anime_id é o AniList **media** ID (foi o MediaList entry ID até a migração)](#16-anime_id-é-o-anilist-media-id-foi-o-medialist-entry-id-até-a-migração)
- [**#17** — --debug-anime early-exit branch in cmd/daemon/main.go](#17---debug-anime-early-exit-branch-in-cmddaemonmaingo)
- [**#18** — extractTitleTokens truncates the torrent name at the first episode/season marker before tokenizing](#18-extracttitletokens-truncates-the-torrent-name-at-the-first-episodeseason-marker-before-tokenizing)
- [**#19** — Disk space is read via OS stat on CompletedAnimePath](#19-disk-space-is-read-via-os-stat-on-completedanimepath)
- [**#20** — extractSeason has a roman-numeral fallback kept out of reSeasonPatterns](#20-extractseason-has-a-roman-numeral-fallback-kept-out-of-reseasonpatterns)
- [**#21** — Embedded torrent client + hardlink-into-library model (replaces qBittorrent)](#21-embedded-torrent-client--hardlink-into-library-model-replaces-qbittorrent)
- [**#22** — Organize everything to completed_anime_path, and the batch-hygiene deletion limitation](#22-organize-everything-to-completed_anime_path-and-the-batch-hygiene-deletion-limitation)
- [**#23** — Integration tests skip unless DAEMON_URL is set explicitly](#23-integration-tests-skip-unless-daemon_url-is-set-explicitly)
- [**#24** — A failed torrent is dropped from the session and re-added by the next pass — no blacklist](#24-a-failed-torrent-is-dropped-from-the-session-and-re-added-by-the-next-pass--no-blacklist)
- [**#25** — Seeding is created at startup and is independent of the daemon loop](#25-seeding-is-created-at-startup-and-is-independent-of-the-daemon-loop)
- [**#26** — The hardlink probe runs on every verification pass, not just on config save](#26-the-hardlink-probe-runs-on-every-verification-pass-not-just-on-config-save)
- [**#27** — saveEpisodesToFile merges by hand — it uses neither SaveEpisodesToFile nor a bare UpsertEpisodes](#27-saveepisodestofile-merges-by-hand--it-uses-neither-saveepisodestofile-nor-a-bare-upsertepisodes)
- [**#28** — Organize replaces a library file that has the same name but different bytes](#28-organize-replaces-a-library-file-that-has-the-same-name-but-different-bytes)
- [**#29** — Startup reconciliation keys off empty LibraryPaths, not off the hardlink missing from disk](#29-startup-reconciliation-keys-off-empty-librarypaths-not-off-the-hardlink-missing-from-disk)
- [**#30** — Progress data comes from one Stats() per torrent, pulled only while a screen is open](#30-progress-data-comes-from-one-stats-per-torrent-pulled-only-while-a-screen-is-open)
- [**#31** — Diretório de download derivado da biblioteca](#31-diretório-de-download-derivado-da-biblioteca)
- [**#32** — DELETE /torrents/{hash}: default é apagar + bloquear, keep_data é binário, não há endpoint de lote](#32-delete-torrentshash-default-é-apagar--bloquear-keep_data-é-binário-não-há-endpoint-de-lote)
- [**#33** — daisyUI fica travado na v4 enquanto o Tailwind for v3 (e o inline de progresso não pode olhar is_downloaded)](#33-daisyui-fica-travado-na-v4-enquanto-o-tailwind-for-v3-e-o-inline-de-progresso-não-pode-olhar-is_downloaded)
- [**#34** — Troca da pasta de download é detectada por marcador duplo, e derruba a sessão da rain](#34-troca-da-pasta-de-download-é-detectada-por-marcador-duplo-e-derruba-a-sessão-da-rain)
- [**#35** — os.SameFile no Windows resolve o arquivo tarde, então FileInfo não é snapshot em teste](#35-ossamefile-no-windows-resolve-o-arquivo-tarde-então-fileinfo-não-é-snapshot-em-teste)
- [**#36** — AppShell escolhe rail vs tab bar em JS (matchMedia), não com hidden md:flex/md:hidden](#36-appshell-escolhe-rail-vs-tab-bar-em-js-matchmedia-não-com-hidden-mdflexmdhidden)
- [**#37** — Tabelas de largura fixa só a partir de lg, e min-w-0 obrigatório em item de grid que contém faixa rolável](#37-tabelas-de-largura-fixa-só-a-partir-de-lg-e-min-w-0-obrigatório-em-item-de-grid-que-contém-faixa-rolável)
- [**#38** — Em cabeçalho flex-wrap, o item flex-1 leva min-w-[240px] — e não min-w-0](#38-em-cabeçalho-flex-wrap-o-item-flex-1-leva-min-w-240px--e-não-min-w-0)
- [**#39** — Faixa de escolhas FINITA quebra linha (flex-wrap); faixa de conteúdo ILIMITADO rola](#39-faixa-de-escolhas-finita-quebra-linha-flex-wrap-faixa-de-conteúdo-ilimitado-rola)
- [**#40** — O z-30 do NavRail não é redundante com o z-50 do painel do MoreMenu](#40-o-z-30-do-navrail-não-é-redundante-com-o-z-50-do-painel-do-moremenu)
- [**#41** — A fila de downloads contém TODOS os incompletos, é persistida, e queued é o único slug que a rain não produz](#41-a-fila-de-downloads-contém-todos-os-incompletos-é-persistida-e-queued-é-o-único-slug-que-a-rain-não-produz)
- [**#42** — Todo estado persistido do FileManager grava com temp+rename sob m.mu, e fetchAniListEntries devolve nil de propósito](#42-todo-estado-persistido-do-filemanager-grava-com-temprename-sob-mmu-e-fetchanilistentries-devolve-nil-de-propósito)
- [**#43** — A identidade de um anime é Media.Id; status é uma pergunta POR CONTA (download = OR, deleção = AND)](#43-a-identidade-de-um-anime-é-mediaid-status-é-uma-pergunta-por-conta-download--or-deleção--and)
- [**#44** — O tvshow.nfo é escrito DEPOIS dos hardlinks, nunca sobrescreve, e falhar nele não falha o organize](#44-o-tvshownfo-é-escrito-depois-dos-hardlinks-nunca-sobrescreve-e-falhar-nele-não-falha-o-organize)
- [**#45** — Uma pasta de biblioteca por ENTRADA da AniList — o marcador de season fica no nome](#45-uma-pasta-de-biblioteca-por-entrada-da-anilist--o-marcador-de-season-fica-no-nome)
- [**#46** — GetFrontendAnimeList é cacheado por 60s — o poll de /api/v1/animes é o que estoura a AniList](#46-getfrontendanimelist-é-cacheado-por-60s--o-poll-de-apiv1animes-é-o-que-estoura-a-anilist)
- [**#47** — Notificações são agrupadas por janela, e o body é escapado só quando o preset é JSON](#47-notificações-são-agrupadas-por-janela-e-o-body-é-escapado-só-quando-o-preset-é-json)
- [**#48** — max_episodes_per_anime nunca limitou batch — a regra agora vive na janela de packs, não num teto de contagem](#48-max_episodes_per_anime-nunca-limitou-batch--a-regra-agora-vive-na-janela-de-packs-não-num-teto-de-contagem)
- [**#49** — Anime avulso: "acompanhado pela lista" é o snapshot que o daemon PROCESSA, e DownloadStandaloneAnime nunca chama handleSavedEpisodes](#49-anime-avulso-acompanhado-pela-lista-é-o-snapshot-que-o-daemon-processa-e-downloadstandaloneanime-nunca-chama-handlesavedepisodes)
- [**#50** — Busca de avulso: o filtro de não lançados é server-side e por concatenação, e o card bloqueado vira link em vez de beco sem saída](#50-busca-de-avulso-o-filtro-de-não-lançados-é-server-side-e-por-concatenação-e-o-card-bloqueado-vira-link-em-vez-de-beco-sem-saída)
- [**#51** — Episódio aceita 4 dígitos, exceto entre colchetes](#51-episódio-aceita-4-dígitos-exceto-entre-colchetes)
- [**#52** — A lista de episódios é sintetizada, e a chave de um episódio é (anime, número)](#52-a-lista-de-episódios-é-sintetizada-e-a-chave-de-um-episódio-é-anime-número)
- [**#53** — Busca de episódio descarta batch e filme, e usa hasMovieMarker — não isMovie](#53-busca-de-episódio-descarta-batch-e-filme-e-usa-hasmoviemarker--não-ismovie)
- [**#54** — Piso de seeders: seeders ilegível é descartado, tamanho ilegível passa](#54-piso-de-seeders-seeders-ilegível-é-descartado-tamanho-ilegível-passa)
- [**#55** — health compara FAIXA de seeders, e vem antes de fansub](#55-health-compara-faixa-de-seeders-e-vem-antes-de-fansub)
- [**#56** — Série longa busca o episódio também com zero-padding, e o gate é LastAiredEpisode](#56-série-longa-busca-o-episódio-também-com-zero-padding-e-o-gate-é-lastairedepisode)
- [**#57** — Paginação adaptativa: sequencial, com piso de candidatos, sem orçamento por anime](#57-paginação-adaptativa-sequencial-com-piso-de-candidatos-sem-orçamento-por-anime)
- [**#58** — RunAnimeDebug espelha processAnimeEpisodes: enumeração, resolução de pack e fallback single](#58-runanimedebug-espelha-processanimeepisodes-enumeração-resolução-de-pack-e-fallback-single)
- [**#59** — Elegibilidade a batch deixou de ser metadado e virou filtro de resultado](#59-elegibilidade-a-batch-deixou-de-ser-metadado-e-virou-filtro-de-resultado)
- [**#60** — A janela de packs cobre a partir do primeiro pendente, e o corte é sempre de prefixo](#60-a-janela-de-packs-cobre-a-partir-do-primeiro-pendente-e-o-corte-é-sempre-de-prefixo)
- [**#61** — Episódio de pack é imune à poda por limite E ao keep-set de assistidos](#61-episódio-de-pack-é-imune-à-poda-por-limite-e-ao-keep-set-de-assistidos)
- [**#62** — Progresso de avulso mora em AnimeSettings e é injetado no MediaList sintético, não vira caminho paralelo](#62-progresso-de-avulso-mora-em-animesettings-e-é-injetado-no-medialist-sintético-não-vira-caminho-paralelo)
- [**#63** — A dispensa do card de primeiros passos mora no localStorage, não no config.json](#63-a-dispensa-do-card-de-primeiros-passos-mora-no-localstorage-não-no-configjson)
- [**#64** — Os passos do card de primeiros passos são marcados à mão, não derivados do estado do daemon](#64-os-passos-do-card-de-primeiros-passos-são-marcados-à-mão-não-derivados-do-estado-do-daemon)
- [**#65** — O orçamento da AniList é gasto pelo refresh de órfãos, não pelo passe do daemon — e o campo errors de um 200 não é lido](#65-o-orçamento-da-anilist-é-gasto-pelo-refresh-de-órfãos-não-pelo-passe-do-daemon--e-o-campo-errors-de-um-200-não-é-lido)
- [**#66** — Banner é estado degradado; toast é falha de ação — e o banner tem três fontes com precedência, não três banners](#66-banner-é-estado-degradado-toast-é-falha-de-ação--e-o-banner-tem-três-fontes-com-precedência-não-três-banners)
- [**#67** — Passe de verificação é um por vez, e o concorrente é descartado — não enfileirado](#67-passe-de-verificação-é-um-por-vez-e-o-concorrente-é-descartado--não-enfileirado)
- [**#68** — Numeração do pack: o offset é adivinhado dos próprios arquivos, e a colisão de basename usa o caminho relativo](#68-numeração-do-pack-o-offset-é-adivinhado-dos-próprios-arquivos-e-a-colisão-de-basename-usa-o-caminho-relativo)
- [**#69** — codec passou a valer para episódio, e nome sem tag de codec empata em vez de perder](#69-codec-passou-a-valer-para-episódio-e-nome-sem-tag-de-codec-empata-em-vez-de-perder)
- [**#70** — Resolução canonicaliza no extrator, e os presets de codec são reordenação no frontend](#70-resolução-canonicaliza-no-extrator-e-os-presets-de-codec-são-reordenação-no-frontend)
- [**#71** — A AniList não tem id de franquia: a cadeia de PREQUEL é a única fonte, e busca por nome é armadilha](#71-a-anilist-não-tem-id-de-franquia-a-cadeia-de-prequel-é-a-única-fonte-e-busca-por-nome-é-armadilha)
- [**#72** — O orçamento da AniList se mede pelos headers da resposta, não por um contador nosso](#72-o-orçamento-da-anilist-se-mede-pelos-headers-da-resposta-não-por-um-contador-nosso)
- [**#73** — O frontend não busca direto na AniList, mesmo podendo](#73-o-frontend-não-busca-direto-na-anilist-mesmo-podendo)
- [**#74** — A unidade do guard de exclusão de pack é o conteúdo do torrent, não a lista de registros](#74-a-unidade-do-guard-de-exclusão-de-pack-é-o-conteúdo-do-torrent-não-a-lista-de-registros)
- [**#75** — O corte no marcador e a primeira tentativa do match de titulo, nao a unica](#75-o-corte-no-marcador-e-a-primeira-tentativa-do-match-de-titulo-nao-a-unica)
- [**#76** — searchNyaaWithVariants para na primeira variante que devolve qualquer resultado — medido, e fica assim](#76-searchnyaawithvariants-para-na-primeira-variante-que-devolve-qualquer-resultado--medido-e-fica-assim)
- [**#77** — O eixo absoluto por série é um BFS de duas em duas gerações, e o nível cortado volta para a fila](#77-o-eixo-absoluto-por-série-é-um-bfs-de-duas-em-duas-gerações-e-o-nível-cortado-volta-para-a-fila)
- [**#78** — A unidade de posse de um torrent é a cobertura no eixo absoluto, não a chave (anime_id, episódio)](#78-a-unidade-de-posse-de-um-torrent-é-a-cobertura-no-eixo-absoluto-não-a-chave-anime_id-episódio)
- [**#79** — A escolha de pack pergunta "cobre a janela?", não "é da part N?" — e a numeração do pack é palpite entre três hipóteses](#79-a-escolha-de-pack-pergunta-cobre-a-janela-não-é-da-part-n--e-a-numeração-do-pack-é-palpite-entre-três-hipóteses)
- [**#80** — O teto de tamanho de pack é aplicado DUAS vezes: na busca do Nyaa e no filtro do daemon](#80-o-teto-de-tamanho-de-pack-é-aplicado-duas-vezes-na-busca-do-nyaa-e-no-filtro-do-daemon)
- [**#81** — A precedência dos códigos do relatório é cascata, não conjunto](#81-a-precedência-dos-códigos-do-relatório-é-cascata-não-conjunto)
- [**#82** — SetLastCheckError limpa o relatório da última verificação](#82-setlastcheckerror-limpa-o-relatório-da-última-verificação)
- [**#83** — A faixa do nome do pack é o primeiro casamento plausível, não o primeiro casamento](#83-a-faixa-do-nome-do-pack-é-o-primeiro-casamento-plausível-não-o-primeiro-casamento)
- [**#84** — A cobertura de um pack sem faixa no nome vem da lista de arquivos, e não da suposição de que ele cobre tudo](#84-a-cobertura-de-um-pack-sem-faixa-no-nome-vem-da-lista-de-arquivos-e-não-da-suposição-de-que-ele-cobre-tudo)

---

### 1. Var-swap mocking for anilist and nyaa HTTP clients

**What it looks like:** Package-level `var httpDo` / `var httpGet` function variables instead of an `HTTPClient` interface.

**Why it's right:** `anilist` and `nyaa` are thin HTTP wrappers with no branching behavior — there's nothing to mock except the HTTP call itself. The var-swap pattern avoids creating a whole interface for one method, keeps zero test boilerplate, and the `MockXxx(fn) restore` helper makes tests self-cleaning.

**Don't "fix" by:** introducing `HTTPClient` interfaces in these packages. Interface injection is reserved for components that have real behavioral variation (see `FileManagerInterface`, torrent client).

---

### 2. `FileManagerInterface` declared twice

**What it looks like:** The same interface exists in both `internal/daemon/helpers.go` and `internal/api/server.go` instead of a shared `interfaces` package.

**Why it's right:** `api` already imports `daemon` (for `*State`, `LoopControl`, etc.). If `daemon` imported `api` (or a shared package that `api` also imports transitively), the import graph would cycle. Duplicating the interface keeps each package self-contained and Go's type system satisfies both interfaces structurally — no explicit coupling needed.

**Don't "fix" by:** extracting to a shared `interfaces` or `types` package without first tracing the full import graph. A seemingly neutral extraction will break the build.

---

### 3. JSONL format for `episodes.json` with full-file rewrite on save

**What it looks like:** The file is JSONL (one JSON object per line) but `saveEpisodesToFile` rewrites the entire file, not just appends new lines.

**Why it's right:** JSONL allows line-by-line parsing and provides backward compatibility with the old plain-text episode format (fallback parser in `parser.go`). Full rewrite on save is intentional: it avoids partial-write corruption — if a true append fails mid-write, the file is left in a mixed state. Read-modify-write with `WriteFile` is atomic at the OS level on the platforms we target.

**Don't "fix" by:** switching to true file-append (`os.O_APPEND`) — that breaks deduplication logic and leaves no room for deletions. Don't switch to a binary format — JSONL is human-readable for debugging.

---

### 4. `cancelPtr` / `donePtr` pointer mutation for runtime interval updates

**Location:** `internal/daemon/loop.go` — `StartLoop` / `UpdateInterval`

**What it looks like:** `cancelPtr := &cancel` — storing a pointer to a `context.CancelFunc` local variable, then reassigning the pointee in `UpdateInterval`. Looks like an unnecessary level of indirection.

**Why it's right:** `UpdateInterval` needs to cancel the running goroutine and start a new one with a different interval, all under the same mutex. Storing pointers to the cancel function and done channel lets `UpdateInterval` swap them atomically without exposing internal state or replacing the entire `LoopControl` struct returned to callers.

**Don't "fix" by:** removing the indirection or flattening into a channel-based command pattern. The current structure keeps `LoopControl` stable (callers hold the same pointer) while the internals are replaced underneath.

---

### 5. State notifier called outside the mutex lock

**Location:** `internal/daemon/state.go` — `SetStatus`, `SetLastCheck`, `SetLastCheckError`

**What it looks like:** The code snapshots `notifier` and state values while holding `s.mu`, releases the lock, then calls `notifier.NotifyStateChange(...)`. Releasing before calling looks like a race.

**Why it's right:** `NotifyStateChange` triggers WebSocket broadcasts, which may acquire their own locks. Calling it while holding `s.mu` risks deadlock if any downstream code tries to read State. Snapshotting values under lock and calling the notifier after releasing is the standard Go pattern for callbacks that must not be called under a lock.

**Don't "fix" by:** moving `NotifyStateChange` inside the `s.mu.Lock()` block. That is the deadlock.

---

### 6. Non-blocking send on WebSocket broadcast channel

**Location:** `internal/api/websocket.go` — `NotifyStateChange`

**What it looks like:** `select { case wsm.broadcast <- message: default: /* drop */ }` — silently drops messages when the channel is full.

**Why it's right:** `NotifyStateChange` is called from the daemon loop (inside `SetStatus`). If the WebSocket consumers are slow, blocking here would stall the entire verification loop. WebSocket clients are UI-only; they get eventual consistency, not strict delivery. The channel has a 256-message buffer, so drops only happen under extreme backpressure.

**Don't "fix" by:** making the send blocking or adding a retry loop. That couples daemon throughput to UI client speed.

---

### 7. Never-closing channel as headless tray fallback

**Location:** `cmd/daemon/main.go` — shutdown select block

**What it looks like:** When no tray manager exists, a channel is created and never closed or signaled: `c := make(chan struct{}); trayShutdownChan = c`. Looks like a leak or forgotten close.

**Why it's right:** The main goroutine selects on both `sigChan` (OS signals) and `trayShutdownChan` (tray quit). A nil channel would panic in a select. A never-closed channel simply never fires, leaving OS signals as the only exit path — which is the correct behavior for headless/server deployments where there is no tray.

**Don't "fix" by:** using a nil check before the select or replacing with a boolean flag. The channel idiom keeps the select uniform and is idiomatic Go for "this path never triggers."

---

### 8. Hard part filter — nil-part torrents rejected when requestedPart is set

**Location:** `internal/nyaa/nyaa.go` — `ScrapNyaa`, `ScrapNyaaForAnime`

**What it looks like:** When `requestedPart != nil`, torrents whose name has no part marker are rejected, even though they might be the right episode. Looks overly strict.

**Why it's right:** Split-season animes (e.g. Mushoku Tensei II Part 1 / Part 2) have the same episode numbers in both halves. Without a hard part filter, Part 1 torrents would be downloaded for Part 2 entries. A torrent with no part marker in its name is ambiguous and must be treated as the wrong release when the caller knows it wants a specific part. The false-negative cost (missing a valid torrent) is lower than the false-positive cost (wrong episode downloaded silently).

**Don't "fix" by:** accepting nil-part torrents as "probably correct." The offset fallback (see decision 9) handles the rare case where no part-marked torrent exists.

---

### 9. PREQUEL offset gate requires part ≥ 2

**Location:** `internal/daemon/helpers.go` — `ComputeEpisodeOffset`

**What it looks like:** `ComputeEpisodeOffset` always returns 0 unless `part >= 2`, even when a PREQUEL relation exists. Looks like a missing case.

**Why it's right:** The offset (PREQUEL episode count) is only needed for split-season Part 2 releases where the torrent site numbers episodes globally (e.g. episode 14 on Nyaa when Anilist calls it episode 1 of Part 2). Kaguya-sama Season 2, for example, also has a PREQUEL relation with 12 episodes — applying a +12 offset there would search for episode 13 when the correct episode is 1. The gate `part >= 2` restricts the offset to the specific case it was designed for, preventing all single-season animes with prior seasons from receiving a wrong offset.

O mesmo raciocínio vale para o **formato** da PREQUEL: só `TV`/`TV_SHORT` entram na conta. Um filme de recap ou uma OVA também chegam como `PREQUEL` e com `Episodes` preenchido, mas não fazem parte da numeração que os fansubs seguem — somá-los mandaria a busca para um episódio inexistente. Quando não há PREQUEL de TV, o offset é 0 e a busca cai na numeração relativa, que é o que boa parte dos grupos usa. Errar para 0 é barato; errar para um número é uma busca que não acha nada.

**Don't "fix" by:** removing the gate or applying the offset whenever a PREQUEL exists. That breaks every multi-season anime that does not use split-part numbering. Nor by accepting a PREQUEL of any format.

---

### 11. `GetCustomListsMap` — separate lightweight query + cache for `customLists`

**Location:** `internal/anilist/anilist.go` — `GetCustomListsMap`; called at the top of `searchAnilist` (`verification.go`) and `fetchAniListEntries` (`api/endpoint_animes.go`) before `GetAllCurrentAnime`.

**What it looks like:** We call Anilist twice per verification cycle: once with a minimal `id + customLists` query, and once with the full `GetAllCurrentAnime` query. Then we overwrite `ml.CustomLists` from the first result when the second comes back with `null`. Looks like redundant work and a band-aid.

**Why it's right:** Anilist's GraphQL engine enforces a query-complexity budget. `GetAllCurrentAnime` includes `airingSchedule.nodes` (which is large for currently-releasing anime — it contains all future episode schedules) plus `relations`, `synonyms`, and `coverImage`. For users watching many currently-airing animes, this query exceeds Anilist's complexity threshold, causing the API to return `null` for the `customLists` scalar on every entry in the response — silently, with HTTP 200 and no error field. COMPLETED/DROPPED animes are unaffected because their `airingSchedule` is empty.

**Root-cause confirmed by:** back-to-back curl tests: Run 1 returned 0/20 null, Runs 2–5 returned 20/20 null (rate-limit exhausted). The user also ran a manual minimal GraphQL query (only `id` + `customLists`) which returned correct data including `{"AutoDownloader": true}` — confirming the API works; only the complex query suppresses the field.

**Effect when broken:** `customLists` is always `null` → `animeIsInExcludedList` never matches → blacklisted animes (e.g. in the "AutoDownloader" custom list used to block titles) are downloaded anyway and the frontend block icon is never shown.

**Gate de orçamento:** quando o chamador passa `PriorityDisposable` e o gate recusa, a função serve a leitura **vencida** do cache em vez de devolver `nil` — ver #72, "O par do poll degrada junto ou não degrada".

**Cache:** `GetCustomListsMap` caches results keyed by `username + statuses` (`ttlCache`, ver decisions.md #46). Uma resposta com pelo menos um `CustomLists` não-nulo fica 5 minutos (`customListsTTL`); uma resposta vazia fica só 30 segundos (`customListsEmptyTTL`), porque "vazio" tanto pode ser uma conta sem custom lists quanto um campo que a AniList degradou. Antes o vazio não era cacheado de jeito nenhum, e aí uma conta sem custom lists gerava um request por poll do frontend — exatamente o amplificador que estourava o limite. A leitura do cache acontece antes da busca, então nenhuma resposta degradada chega a evictar uma entrada ainda válida.

**Overlay guard:** `if cl, ok := clMap[ml.Id]; ok && len(cl) > 0 { ml.CustomLists = cl }` — the `len(cl) > 0` guard ensures that a rate-limited nil response from `GetCustomListsMap` (which would produce an empty map entry) never silently clears data that `GetAllCurrentAnime` might have returned correctly on a lucky call.

**Don't "fix" by:**
- Removing `GetCustomListsMap` and relying solely on `GetAllCurrentAnime` — `customLists` will be null whenever the complex query hits the complexity limit, breaking blacklist exclusions entirely.
- Removing the cache — rapid successive calls (frontend polling, concurrent goroutines) will exhaust rate limits, causing all calls to return null.
- Removing the `len(cl) > 0` guard — an empty map overwrites any valid data the complex query returned, flipping all animes to "not blacklisted."

---

### 13. `GetFrontendAnimeList` — separate lighter Anilist query for the API endpoint

**Location:** `internal/anilist/anilist.go` — `GetFrontendAnimeList`; called from `fetchAniListEntries` (`api/endpoint_animes.go`).

**What it looks like:** There are now two functions that both return `*AniListResponse` and differ only in which GraphQL fields they request. Looks like duplication.

**Why it's right:** `GetAllCurrentAnime` (used by the daemon verification loop via `searchAnilist`) needs `synonyms`, `relations`, and `format` to match torrents and compute offsets, but does not need `coverImage`. `GetFrontendAnimeList` (frontend `/animes` endpoint) needs `coverImage` for display but never touches `synonyms`, `relations`, or the `id` field on airingSchedule nodes. Keeping them separate lets each query stay within Anilist's complexity budget — `GetAllCurrentAnime` avoids the cost of fetching images, and `GetFrontendAnimeList` avoids the cost of fetching relations/synonyms. The return type is the same (`*AniListResponse`); unused fields simply remain at their zero values.

**Don't "fix" by:** merging back into one query. Requesting all fields from both call sites is what caused complexity-budget exhaustion (see decision 11).

---

### 12. Build logic lives in `scripts/build.sh`, not in Makefile targets

**What it looks like:** `build-linuxamd64` / `build-linuxarm64` / `build-windows` just delegate to `bash scripts/build.sh <platform> <version>` with no logic in the Makefile itself.

**Why it's right:** Make's autocomplete exposes every named target, including internal ones (`docker-build`, `docker-buildx-build`, `docker-build-classic`, `checksums`, `check-docker`) and file/directory targets (`build/linux-amd64`, `build/linux-amd64/autoanimedownloader-daemon`, etc.). Moving the implementation to a script eliminates all internal targets from the completion list without requiring per-machine shell configuration.

**Don't "fix" by:** inlining Docker logic back into Makefile targets or re-introducing named intermediate targets. That re-pollutes autocomplete on every machine.

---

### 14. `nyaaSearcher` — dependency injection for Nyaa search in `processAnimeEpisodes`

**Location:** `internal/daemon/search.go` (`nyaaSearcher` struct + `defaultNyaaSearcher()`); `internal/daemon/episodes.go` (`processAnimeEpisodes`).

**What it looks like:** `processAnimeEpisodes` receives a `nyaaSearcher` struct with function-valued fields (`searchAnime`, `searchSingleEpisode`, `searchMovie`) instead of calling `searchNyaaForAnime`, `searchNyaaForMovie`, etc. directly. Looks like unnecessary indirection — these are pure functions with no state.

**Why it's right:** The `episodeInTorrents` hash-check fix prevents batch torrents from being re-downloaded in a loop. Without injection, it's impossible to write a test that proves the loop can't recur: a test that calls the real Nyaa makes a live HTTP request, is flaky, and can't observe whether `searchAnime` was invoked. Injection lets the regression test (`TestProcessAnimeEpisodes_BatchNoRedownload`) confirm both that `searchAnime` is never called and that `POST /add` is never sent when all episode hashes already match.

**Don't "fix" by:** removing the `nyaaSearcher` parameter and going back to direct package calls. That makes the regression test impossible to write, and the loop bug would be undetectable until it reappears in production.

---

### 10. "Cour N" treated as Part N, not as a distinct concept

**Location:** `internal/nyaa/nyaa_regex.go` — `rePartPatterns`; `internal/daemon/helpers.go` — `ExtractAnimeSeasonPart`

**What it looks like:** `Cour 2` is extracted by `ExtractPart`, not by a separate cour field. Looks like it conflates two different concepts.

**Why it's right:** Anilist uses "Cour" and "Part" interchangeably for split seasons (e.g. NieR:Automata Ver1.1a Cour 2). On Nyaa, release groups also use both terms for the same split. A separate cour field would require all search/filter logic to handle two parallel fields with identical semantics. Mapping cour→part keeps the filter logic and tests uniform and matches how the data actually appears in the wild.

**Don't "fix" by:** adding a separate `Cour *int` field to `TorrentResult` or a separate extraction path.

---

### 15. `atomic.Pointer[Priorities]` package-level global in `nyaa`

**Location:** `internal/nyaa/priorities.go` — `active atomic.Pointer[Priorities]`, `ActivePriorities()`, `SetPriorities()`.

**What it looks like:** A mutable global swapped at runtime instead of threading a `Priorities` value/config through every sort/filter function call (`SortTorrentResults`, `SortMovieResults`, `fansubPriority`, `ShouldIgnore`, etc.). Looks like hidden state that could cause data races or surprising cross-test pollution.

**Why it's right:** The sort/filter call sites (`ScrapNyaa*`, `SortTorrentResults`, `SortMovieResults`, and a dozen small `xPriority` helpers) are deep, numerous, and have no `*Config` or context parameter today — threading one through would touch most of `nyaa.go` for a value that changes rarely (only on config save/load). `atomic.Pointer` makes reads lock-free and the swap atomic, so the daemon's verification loop (running concurrently with API requests) never observes a torn read. `SetPriorities` returns a `restore func()`, mirroring the existing `MockNyaaHttpGet` convention (decision 1) — tests `defer restore()` instead of mutating shared state permanently. Package `init()` seeds the pointer with `DefaultPriorities()` so any code that calls the sort functions without ever calling `SetPriorities` (most unit tests) still gets correct, non-nil behavior.

**Don't "fix" by:** threading a `Priorities` parameter through every nyaa function, or replacing the atomic pointer with a mutex-guarded struct. The former is a large, low-value refactor; the latter adds lock contention to a hot path (every torrent comparison during sort) for no correctness benefit over `atomic.Pointer`.

---

### 16. `anime_id` é o AniList **media** ID (foi o MediaList entry ID até a migração)

**Location:** `internal/anilist/anilist.go` (`Media.Id`, `GetAnimeInfo(mediaId, usernames)`); `internal/api/endpoint_animes.go` e `endpoint_anime_episodes.go` (campos `AnimeID`); `daemon/migration.go` (`MigrateAnimeIDsToMedia`).

**What it looks like:** `anime_id` é *o* identificador de anime em todo lugar (arquivo de settings, registros de episódio, parâmetros de rota) e também serve de componente da URL `anilist.co/anime/{id}` — não existe mais um `anilist_id` separado na API.

**Why it's right:** até a migração, o valor vinha do campo `id` do tipo `MediaList` da AniList, que é o id da **entrada de lista** e portanto **por conta**. Com mais de uma conta configurada, o mesmo anime tinha dois `anime_id`, e o app tratava como dois animes. Ver a decisão #43 para o bug concreto e o desenho atual. Instalações antigas são convertidas uma única vez por `MigrateAnimeIDsToMedia`, marcada pelo campo `anime_ids_are_media_ids` do config.

**Don't "fix" by:** voltar a chavear qualquer coisa por `MediaList.Id` (é por conta — ver #43); rodar a migração de novo depois da marca ligada (interpretaria ids de mídia como ids de entrada).

---

### 17. `--debug-anime` early-exit branch in `cmd/daemon/main.go`

**Location:** `cmd/daemon/main.go` — `runDebugAnime`, the `flag.Int("debug-anime", ...)` check at the top of `main()`.

**What it looks like:** `main()` parses a flag and, if set, runs a completely different code path (`runDebugAnime`) and returns — skipping the PID file, API server, tray, and daemon loop entirely. Looks like a debug hack that snuck into production entry point.

**Why it's right:** It's a deliberate one-shot diagnostic mode (`make debug-anime ID=<anilistId>` / `go run ./src/cmd/daemon --debug-anime <id>`) for the recurring "why didn't this anime download" problem. It reuses real production functions (`daemon.RunAnimeDebug` → `checkEpisode`, `partitionSearchResults`/`pickBatches`) so the debug output can't drift from actual verification-loop behavior, and it deliberately avoids touching the torrent client so it can run without the daemon up (the torrent client is embedded, so there is nothing external to start either way). See `docs/agents/troubleshooting-downloads.md` Step 0 and `daemon/debug.go`.

**Don't "fix" by:** moving this behind the HTTP API (it exists specifically to work without a running daemon) or deleting it as dead code (it's the primary entry point for the fast-path troubleshooting flow).

---

### 18. `extractTitleTokens` truncates the torrent name at the first episode/season marker before tokenizing

**Location:** `internal/nyaa/nyaa_match.go` — `truncateAtFirstMarker`, `reLeadingBracket`, called at the top of `extractTitleTokens`.

**What it looks like:** Tokenizing only the substring before the first `S01E05`/`- 05`/`Episode 3`-style marker, and separately stripping a leading `[Group]`/`(Group)` tag, instead of just tokenizing the whole torrent name. Looks like it would drop legitimate title words.

**Why it's right:** Real torrent names often carry the episode's plot title, streaming-service tag, or a duplicate alt-title in parentheses *after* the episode marker (e.g. `KAIJU GIRL CARAMELISE S01E01 The Kaiju Girl Appears in Tokyo 1080p CR WEB-DL ... (Otome Kaijuu Caramelise, Multi-Subs)`). None of that belongs to the anime's core title, but it isn't covered by the fixed `titleTechnicalTokens` allowlist (unknown fansub tags like `varyg`/`ironclad`, stray split tokens like `h` from `H 264`, episode-title prose). Tokenizing it anyway inflates the Jaccard union enough that `jaccardThreshold` (0.8 for ≤3-token queries) rejects genuine matches — this caused two real currently-airing anime to never download (see debug session that produced this decision). Truncating at the marker (reusing `reEpisodePatterns`/`reSeasonPatterns`, already used by `extractEpisodeNumber`/`extractSeason`) removes exactly the noise while preserving genuine extra title words that appear *before* the marker (e.g. "Alternative Gun Gale Online" in a Sword Art Online spinoff), so the existing spin-off-rejection tests still pass.

**Don't "fix" by:** lowering `jaccardThreshold` instead. The two failure modes overlap: the real Kaiju Girl Caramelise match scores ~0.33 Jaccard, but the SAO-spinoff torrent that must stay rejected scores ~0.5 — no single threshold accepts one without accepting the other. The marker-truncation approach fixes the union inflation at its source instead.

**Amendment (see #75):** o corte continua sendo a PRIMEIRA tentativa e vale exatamente como descrito acima, mas deixou de ser a única: quando ele falha, `titleMatchesQuery` tenta os títulos alternativos que aparecem depois do marcador. A frase "none of that belongs to the anime's core title" era falsa para os grupos que repetem o romaji no rodapé.

---

### 19. Disk space is read via OS stat on `CompletedAnimePath`

**Location:** `internal/files/diskspace_unix.go`, `internal/files/diskspace_windows.go`; `internal/api/endpoint_status.go` (`handleStatus`).

**What it looks like:** Reading disk space with a raw platform-specific filesystem syscall on a local path, when a portable Go library might seem cleaner.

**Why it's right:** The dashboard needs both total capacity **and** free space ("tamanho total, tamanho disponível" per `docs/TODO.md`); the syscall pair (`Statfs` / `GetDiskFreeSpaceEx`) is the direct way to get both. `handleStatus` swallows stat errors (empty/unreadable `CompletedAnimePath`) rather than surfacing them, so a bad path just hides the disk card instead of breaking `/api/v1/status`.

> **Revision (embedded-client refactor):** The original rationale compared this against qBittorrent's `free_space_on_disk` API (which only reported free space, not total). That comparison is now moot — the torrent client is embedded and there is no qBittorrent API to read from. The old cross-host mount-mismatch caveat no longer applies either. The OS-stat approach stands as the only and correct source.

> **Amendment (see #31):** disk space is now measured on `CompletedAnimePath`, not the legacy `save_path` field. Since the download directory is derived from `CompletedAnimePath` (`Config.DownloadPath`), the two are always on the same filesystem by construction, so measuring the library path also correctly reports free space for downloads-in-progress.

**Don't "fix" by:** trying to route disk stats through the torrent library — it has no such API, and `CompletedAnimePath` is on the same filesystem as the download directory the daemon writes to.

---

### 20. `extractSeason` has a roman-numeral fallback kept out of `reSeasonPatterns`

**Location:** `internal/nyaa/nyaa_regex.go` — `reRomanSeason`, `romanSeasonValues`; `internal/nyaa/nyaa.go` — `extractSeason`.

**What it looks like:** A second, separate regex (`\b(II|III|IV|V|VI|VII|VIII|IX|X)\b`) tried only after the whole `reSeasonPatterns` loop has failed, instead of just adding it as one more entry in that slice.

**Why it's right:** Some sequels are titled with only a roman numeral (e.g. Anilist id 194829, "Katainaka no Ossan, Kensei ni Naru II") — no "Season 2"/"S2" appears anywhere in the AniList title, and fansub groups (Erai-raws, Ironclad) release episodes using that exact title verbatim, with no separate season marker either. Before this fix, `extractSeason("...Naru II - 01 [1080p]...")` returned `nil` while `ExtractAnimeSeasonPart` (via the `"...2nd Season"` synonym) correctly resolved `requestedSeason=2`, so every real torrent got rejected by the hard season filter in `ScrapNyaaForMultipleEpisodes` (`season == nil` vs `requestedSeason=2`) even though `titleMatchesQuery` already matched the full title including "II" — the anime failed to download every cycle. `reSeasonPatterns` is also used by `truncateAtFirstMarker` (decision 18) to decide where to cut a torrent name before tokenizing; if the roman-numeral pattern were merged into that slice, it would truncate `"...Naru II"` right before "II", dropping it from the Jaccard title tokens and silently changing match behavior for every title ending in a numeral. Keeping it as a separate, lower-priority fallback used only by `extractSeason` avoids that cross-effect.

**Don't "fix" by:** merging `reRomanSeason` into `reSeasonPatterns`, or matching lowercase roman numerals — lowercase risks false positives from unrelated fansub/codec tokens, and uppercase-only matches how anime titles actually format them.

---

### 21. Embedded torrent client + hardlink-into-library model (replaces qBittorrent)

**Location:** `internal/torrents/` (`TorrentBackend`, `Session`/`SessionManager`, `FakeBackend`); `internal/files/librarian.go`; `internal/daemon/jobs.go` (`JobOrganize`), `episodes.go`, `verification.go`.

**What it looks like:** The app embeds a full BitTorrent client (`github.com/cenkalti/rain/v2`) and hardlinks completed files into a second directory, instead of the more familiar "talk to a running qBittorrent over its WebUI and let it manage files."

**Why it's right:** Embedding makes the daemon a single self-contained binary — no external qBittorrent to install, run, secure, or keep reachable, and no `qbittorrent_url`/`QBITTORRENT_URL` to configure. Torrents download to `save_path` and keep **seeding** there; on completion the video files are **hardlinked** into `completed_anime_path` (the Jellyfin library). Hardlinking (not copy/move) means:
- No wasted space — the library name and the seeded file share the same bytes.
- Seeding is never interrupted — the seeded file is never moved or renamed (renaming would break the torrent). The Jellyfin name (`"Anime Name - E05.mkv"`, for single episodes and for batch files whose number is readable) lives only on the library hardlink.
- The hard constraint: `save_path` and `completed_anime_path` **must be on the same filesystem/volume** (hardlinks can't cross devices). `completed_anime_path` is therefore now **required**, and the config-save endpoint validates the pair with a real hardlink probe (`Librarian.ProbePaths`), rejecting cross-device paths with HTTP 400.

Torrent logic sits behind the `TorrentBackend` interface (`Add`/`List`/`Get`/`Remove`/`Ensure`/`SetCallbacks`/`Close`) so the daemon injects one uniform backend — rain-backed `SessionManager` in production, in-memory `FakeBackend` in tests (the qBittorrent mock server is gone). Resume data lives in a bbolt DB at `~/.autoAnimeDownloader/session.db`, deliberately **outside** `save_path` so it survives a `save_path` change. rain listens on a default port range (20000–30000) with no UPnP/NAT-PMP; inbound peers may need manual forwarding, but DHT+PEX work without it.

**Don't "fix" by:** re-introducing an external torrent client or `qbittorrent_url`; renaming the seeded file to the Jellyfin name (breaks seeding); copying/moving into the library instead of hardlinking (wastes space, stops seeding); or moving `session.db` into `save_path` (loses resume data on a path change). Note the webhook event key string is still `download_completed` — the Go constant was renamed `QBittorrentDownloadCompleted → DownloadCompleted`, but that is not user-visible; don't "fix" a webhook that isn't broken.

**Amendment (see #31):** `save_path` is no longer a user-configured field — the download directory is now derived from `completed_anime_path` (`Config.DownloadPath`). What was previously a constraint on the user's configuration ("these two fields must be on the same filesystem, and the endpoint validates it") is now an architectural invariant that holds by construction: there is only one path, so there is nothing to be on a different filesystem from. `Librarian.ProbePaths` (two arguments) is gone; the surviving probe is `Librarian.ProbePath` (one argument, #26).

---

### 22. Organize everything to `completed_anime_path`, and the batch-hygiene deletion limitation

**Location:** `internal/daemon/jobs.go` (`organizeTorrent`), `internal/daemon/episodes.go` (`removeEpisodesAndLinks`, `canRemoveTorrent`). Pinned with real files on disk by `TestRemoveEpisodesAndLinks_RealHardlinks` (`internal/daemon/orchestration_test.go`), whose subtests separate the batch asymmetry (torrent *and* library files kept while a sibling survives) from the non-batch shared-hash case (library link removed, torrent kept).

**What it looks like:** (a) *Every* completed torrent — ongoing or finished — is hardlinked into `completed_anime_path`, not just finished animes. (b) When deleting a watched episode that belongs to a **batch** torrent, its library hardlink is **kept** until the entire batch is deleted, which looks like a leak.

**Why it's right (behavior change):** Previously only FINISHED animes were moved to `completed_anime_path`, while ongoing episodes were renamed in place inside `save_path`. Now `save_path` is purely the download/seeding working directory and `completed_anime_path` is the Jellyfin library — so *every* completed torrent gets organized into the library, uniformly, via the single idempotent `organize` job. This removes the ongoing-vs-finished special case and gives Jellyfin one consistent library path to watch.

**Why it's right (batch guard):** Deletion frees space by removing both the library hardlink and the seeding torrent (`TorrentBackend.Remove` with `keepData=false`). For a **single-episode** torrent that maps cleanly to one library file, both links are removed. For a **batch** torrent shared by many episodes, the raw batch filenames can't be safely mapped back to one specific episode — removing "the file for episode 5" risks deleting the wrong file. So per-episode library removal is **skipped for batches**; the batch's library files (and the seeding torrent) are only removed once **all** of that torrent's episodes are in the delete set (`canRemoveTorrent` — que desde #74 também exige que o pack não cubra episódios sem registro). While any sibling episode survives, the batch torrent and its library links stay. The small space cost of keeping a shared batch around a bit longer is preferred over the correctness risk of deleting the wrong episode's file.

**Don't "fix" by:** trying to delete individual episode files out of a batch torrent's library folder (raw filenames aren't reliably episode-addressable), or removing a batch torrent while siblings still reference it (breaks the survivors' library links and stops seeding for episodes still wanted).

**Amendment (see #31):** `save_path` no longer exists as a config field; "everything organizes to `completed_anime_path`" now also covers the download/seeding directory itself, which lives at `<completed_anime_path>/.torrents` (`Config.DownloadPath`).

---

### 23. Integration tests skip unless `DAEMON_URL` is set explicitly

**Location:** `src/tests/integration/integration_test.go` (`requireDaemon`, `defaultTestPath`, `testSavePath`/`testCompletedPath`); `docker/docker-compose.test.yml` (`test` service env).

**What it looks like:** `go test ./...` reports the entire integration suite as SKIP on a developer machine, even when a daemon *is* running on `localhost:8091`. The obvious-looking "fix" is to drop the `DAEMON_URL` check and let `probeDaemon()` decide, since a reachable daemon seems like exactly the condition the tests want.

**Why it's right:** These tests are not read-only. `TestAPIEndpoints` and `TestFullDownloadFlow` both `PUT /api/v1/config`, overwriting `save_path`, `completed_anime_path`, `anilist_username`, `check_interval` and `excluded_lists` on whatever daemon answers. Gating on reachability alone meant that following `CLAUDE.md`'s "run `go test ./...` after any change" silently reconfigured the developer's own daemon: the save path became `/tmp/test`, and on distros where `/tmp` is a tmpfs the daemon then downloaded real torrents into RAM until the filesystem filled. A live daemon is not consent to reconfigure it, so the opt-in is an explicit env var. `docker-compose.test.yml` sets `DAEMON_URL=http://daemon:8091`, so Docker and CI runs are unaffected.

The paths written into the config are likewise no longer hardcoded: they come from `TEST_SAVE_PATH`/`TEST_COMPLETED_PATH`, defaulting to `~/aad-test/downloads` and `~/aad-test/library`. Two constraints shape that default — it must not be under `/tmp` (tmpfs), and both paths must share one filesystem, because the config endpoint runs a real hardlink probe (`Librarian.ProbePaths`) and rejects cross-volume pairs with HTTP 400. Docker overrides both to `/app/data/aad-test/*`, inside the daemon container's mounted volume; note the strings are interpreted by the **daemon** container, not the test container.

**Don't "fix" by:** removing the `DAEMON_URL` gate so the tests "work" during `go test ./...`; hardcoding the config paths again; putting the default paths back under `/tmp`; or splitting save and completed paths across different volumes (the hardlink probe rejects it).

**Amendment (see #31):** `testSavePath`/`TEST_SAVE_PATH` were removed — the integration test no longer sends `save_path` at all, since the field doesn't exist anymore. `Librarian.ProbePaths` (the two-argument save/completed variant) was replaced by `Librarian.ProbePath(completedPath string)`.

---

### 24. A failed torrent is dropped from the session and re-added by the next pass — no blacklist

**Location:** `internal/daemon/helpers.go` — `HandleTorrentFailure`; wired as the `onFailed` callback in `cmd/daemon/main.go`.

**What it looks like:** When the embedded client stops a torrent with an error, we fire the `download_failed` webhook and then **remove the torrent from the session** (`Remove(hash, false)`, discarding partial data). Since nothing records that the hash failed, the very next verification pass finds the episode missing from the session and re-adds the same magnet. For a torrent that is genuinely dead (no seeds at all) this is an unbounded re-add loop, one attempt per `check_interval`. That looks like a bug we forgot to guard.

**Why it's right:** The alternative is worse. rain leaves a failed torrent in state Stopped **inside** the session and never restarts it, and the per-torrent listener goroutine exits after `NotifyStop` and is not re-armed (`torrents/session.go`). If we only logged, `episodeInTorrents` (`daemon/helpers.go`) would keep seeing the hash and the daemon would believe the episode was downloaded — forever. No retry, no organize, no notification, and the episode silently never reaches the library. Dropping the torrent converts a permanent silent failure into an automatic retry that reuses the machinery already in place (the loop re-searches Nyaa and may pick a *different*, healthier release the second time). `EpisodeRetryLimit` does not bound this, because it resets each pass.

**Cost of the accepted risk:** a dead torrent costs one Nyaa search plus one magnet add per `check_interval` — cheap, visible in the logs, and each retry fires a `download_failed` webhook so the user is not left guessing.

**Don't "fix" by:** adding a speculative per-hash blacklist with a TTL. That is the correct fix *if* the re-add churn is ever observed to be a real problem, but it introduces new state to persist, expire, and expose in the UI (otherwise a permanently blacklisted episode becomes the new silent failure). Build it against a real report, not preemptively. Also don't go back to log-only in `onFailed`, and don't pass `keepData=true` — partial data from a torrent that errored out is not reusable and only occupies the save path.

---

### 25. Seeding is created at startup and is independent of the daemon loop

**Location:** `cmd/daemon/main.go` — `ensureStartupSession`, called right after `jobQueue.Start()`; `internal/daemon/verification.go` — the `backend.Ensure(configs.DownloadPath())` call inside the verification pass.

**What it looks like:** `Ensure` is called from **two** places — once at startup and once on every verification pass — which reads like a redundant call that could be dropped from one side.

**Why it's right:** The two calls serve different purposes and both are needed.

- **Startup call:** torrents keep **seeding** from the derived download directory (`Config.DownloadPath()`, nested under `completed_anime_path`) after they complete. If the session were created only by the verification pass, stopping the daemon loop from the WebUI (or booting with the loop stopped) would mean no session at all, i.e. **seeding stops** — a behavior regression against the external-qBittorrent setup, which kept seeding regardless of what the daemon was doing. That matters for private trackers with ratio requirements. Creating the session at startup makes seeding a property of the *process*, not of the loop.
- **Verification-pass call:** the daemon must be able to boot with an incomplete config (no `completed_anime_path`, and therefore no derivable download path). In that case the startup call deliberately does nothing and the session stays lazy; the pass's `Ensure` is what creates it once the user saves a config. It also handles `completed_anime_path` changing at runtime (recreate, since the derived download path moves with it) and is where the `created == true` return feeds startup reconciliation.

Ordering detail: `ensureStartupSession` runs **after** `jobQueue.Start()`. Creating the session arms the resume listeners, which can fire a completion immediately, and `Start()` loads the persisted job list *over* whatever is in memory — enqueueing before it would silently discard the job. The shutdown defers keep their LIFO order (`jobQueue.Stop()` drains organize jobs that still need the session, then `torrentManager.Close()` flushes bbolt); do not reorder them.

**Don't "fix" by:** removing the `Ensure` from the verification pass (breaks the incomplete-config boot and reacting to `completed_anime_path` changes), removing the startup call (stops seeding whenever the loop is stopped), moving startup reconciliation out of the verification pass, or calling `ensureStartupSession` before `jobQueue.Start()`.

**Amendment (see #31):** `daemon.MigrateSavePath` used to run in `main.go` between `jobQueue.Start()` and `ensureStartupSession`, and `ensureStartupSession` was skipped for the whole boot when it failed. Both are **gone** (2026-08-24, see the amendment on #31): `ensureStartupSession` now runs unconditionally right after `jobQueue.Start()`.

---

### 26. The hardlink probe runs on every verification pass, not just on config save

**Location:** `internal/daemon/verification.go` — the `librarian.ProbePath` gate right after `isConfigComplete`; also `internal/api/endpoint_config.go` (`PUT /config`).

**What it looks like:** Every pass writes a probe file to `save_path`, hardlinks it into `completed_anime_path` and deletes both — real disk I/O on a hot loop, duplicating a validation the config endpoint already performs.

**Why it's right:** `isConfigComplete` only checks that the fields are non-empty, and the endpoint probe only covers configs saved **through the API after** the embedded-client upgrade. Two populations bypass it entirely: users who configured a `completed_anime_path` on a different volume back when the app *moved* files (perfectly legal then — `rename` crosses devices, `link` does not), and any deployment where `docker/entrypoint.sh` writes `config.json` straight from env vars. For them the daemon would download happily while every `JobOrganize` failed with `EXDEV`, retried 20 times over ~2.5h, and was dropped — with `LastCheckError` never set, so the WebUI showed a healthy daemon and an empty library.

The probe aborts the pass instead of merely warning: downloading episodes that provably cannot be organized only fills the disk. It reuses `Librarian.ProbePaths`, so the message the user sees in the UI is identical to the one `PUT /config` returns. Cost is one small file write, one link and two unlinks per `check_interval` (default 10 min) — negligible next to the Anilist and Nyaa requests in the same pass.

**Don't "fix" by:** removing the gate because "the endpoint already validates it" (it does not, for pre-upgrade and entrypoint-written configs), or downgrading it to a warning that lets the pass continue. Caching the result per path pair is a legitimate optimization if the I/O ever shows up in a profile — but it must be invalidated on config change, and the current cost does not justify the extra state.

**Amendment (see #31):** the probe is now single-path. `Librarian.ProbePaths(save, completed)` was replaced by `Librarian.ProbePath(completedPath)`, which writes a probe file directly under `<completedPath>/.torrents` and hardlinks it in place — there is no second user-supplied path to compare against, since the download directory is derived, not configured. The check still exists for the same reason: exFAT/FAT32 and some SMB shares don't support hardlinks at all, so the invariant "the download dir and the library share a filesystem" is guaranteed by construction, but "the filesystem supports hardlinks" is not.

---

### 27. `saveEpisodesToFile` merges by hand — it uses neither `SaveEpisodesToFile` nor a bare `UpsertEpisodes`

**Location:** `internal/daemon/episodes.go` — `saveEpisodesToFile` and `mergeSavedEpisode`; the two `FileManager` primitives in `internal/files/filemanager.go`.

**What it looks like:** The daemon loads every saved episode, merges field by field, and only then calls `UpsertEpisodes` — when the `FileManager` already exposes two perfectly good one-liners. Either `SaveEpisodesToFile(eps)` or `UpsertEpisodes(eps)` would compile and look cleaner.

**Why it's right:** The three functions have genuinely different semantics and each is wrong on its own here.

- (Histórico: a chave era `EpisodeID`, o id do nó de agenda da AniList; hoje é `EpisodeKey{AnimeID, Episode}` — ver #52.) `SaveEpisodesToFile` is **append-only with dedupe by key**: for an ID that already exists it drops the incoming record *entirely*. That was the original bug. On the upgrade path to the embedded client, the rain session boots empty, so `episodeInTorrents` is false for every saved episode and the loop re-downloads all of them — and every one of those updates was silently discarded. The stale record kept `EpisodeNumber: 0` (a field that did not exist before the upgrade, so it deserialises as zero for all pre-existing records) and the stale `EpisodeHash`. The first caused every episode of an anime to be organized as `Anime - E00.mkv`; the second broke `JobOrganize`'s saved-episode ↔ torrent join by hash, so the job retried 20 times over ~2.5h and gave up, forever.
- `UpsertEpisodes` **replaces the record wholesale**. That is exactly what `organizeTorrent` needs when writing `LibraryPaths` back, and it is what makes it wrong here: it would clobber `ManuallyManaged`, a user flag the automatic loop must never clear (clearing it lets the loop delete an episode the user pinned).

The merge is the only place that knows which fields belong to *this* download (hash, number, `IsBatch`, names, totals, date — all taken from the new record), which belongs to the *user* (`ManuallyManaged`, OR-ed so a manual download can still set it), and which is *derived* (`LibraryPaths`, reset to nil because the old hardlinks point at the previous release — leaving it set would make `organizeTorrent` think the episode was already organized and never create the new link).

The API handlers in `endpoint_episode_actions.go` reach the same requirement by a shorter route: they call `UpsertEpisodes` directly, never `SaveEpisodesToFile`. Wholesale replacement is safe there — and only there — because every `daemon.ManualDownload*` constructor returns a record with `ManuallyManaged: true` and `LibraryPaths` unset, so the record already carries what `mergeSavedEpisode` would have computed. `handleDownloadEpisode` is the call site that actually needed this: unlike redownload and replace, it does **not** delete the old record first, so append-only dedupe silently dropped the whole update whenever a record already existed. Pinned by `TestHandleDownloadEpisode_UpdatesExistingRecord`, which uses a real `FileManager` over a temp dir — the api package's `mockFileManager` implements both primitives identically and cannot see the difference.

**Don't "fix" by:** collapsing the daemon's merge into either primitive; "simplifying" `mergeSavedEpisode` to `merged.ManuallyManaged = existing.ManuallyManaged` (breaks manual download setting the flag on a fresh record); or switching the API handlers to `SaveEpisodesToFile` "for consistency with the daemon". If a future `ManualDownload*` variant stops setting `ManuallyManaged`, the handlers must move to the daemon's merge rather than keep the bare upsert.

---

### 28. `Organize` replaces a library file that has the same name but different bytes

**Location:** `internal/files/librarian.go` — the `os.SameFile` branch in `Organize`; the `*req.EpisodeNumber > 0` term in the `useJellyfin` condition.

**What it looks like:** When the destination path already exists, `Organize` deletes it and creates a fresh hardlink. Destroying a file that is already in the user's library looks reckless — the safe-looking options are to skip it (as the code originally did, on a bare `Stat(dest) == nil`) or to fail loudly.

**Why it's right:** A bare existence check cannot tell "our hardlink, already created" from "a different file that happens to collide on name", and treating the second as the first is silent data loss: the caller is told the episode was organized, `LibraryPaths` is written pointing at someone else's bytes, and the `download_completed` webhook fires. `os.SameFile` (inode on Unix, file index on Windows — both `Stat` calls go through the injectable `FileSystem`, so fakes still work) splits the two cases. Same file is a true no-op, which is what reconciliation and job retries need. Different file only arises from redownload and replace-episode, where the user explicitly asked for the swap, so the new release wins; the replacement is logged at `Info` with source and destination so it stays auditable. Failing instead would deadlock those flows in `JobOrganize` retries. The seeded file is never touched — only the library name is recycled — so seeding is unaffected.

The related `*req.EpisodeNumber > 0` guard is defence in depth for the same failure: AniList has no episode 0, so a zero unambiguously means *missing data* rather than a real episode. Falling back to the raw filename (unique per release) is strictly better than emitting `Anime - E00.mkv`, which collides across every episode of the anime and, before the `SameFile` fix, made them all resolve to the same destination. Decision 27 fixes the cause; this guard makes sure no future path reintroduces it.

**Don't "fix" by:** restoring the bare `Stat`-and-skip, turning the different-inode case into an error, or dropping the `> 0` term because "the persistence layer guarantees a real episode number now" — the guard exists precisely because that guarantee is not enforceable at this layer.

---

### 29. Startup reconciliation keys off empty `LibraryPaths`, not off the hardlink missing from disk

**Location:** `internal/daemon/verification.go` — `reconcileLibrary`, called at the top of every verification pass. Pinned by `TestReconcileLibrary_Marker` in `internal/daemon/orchestration_test.go`.

**What it looks like:** Reconciliation decides whether an episode still needs organizing by looking only at the **record** (`LibraryPaths` empty → enqueue `JobOrganize`). It never stats the paths, so a record whose `LibraryPaths` point at files the user has since deleted is treated as already organized and skipped. The embedded-client design (§2) actually specified the opposite — check whether the hardlink exists on disk — so this reads like an unfinished implementation.

**Why it's right:** Statting the paths would turn every pass into a repair loop that **resurrects files the user deleted on purpose**. Deleting from the library is a legitimate user action (freeing the library while leaving the torrent seeding, or pruning something they no longer want), and the seeded file is still there for the hardlink to be recreated from — so a disk-based check would recreate it on the next pass, every pass, forever, with no way to opt out short of deleting the episode record too. The record is the daemon's own statement of "I have organized this"; the disk belongs to the user. Keeping them separate means reconciliation only ever fixes work the daemon failed to finish (a crash between torrent completion and organize, or a session restarted before `JobOrganize` ran), which is exactly its purpose.

The deviation is intentional and better than the design. It is also what makes reconciliation idempotent and cheap: `EnqueueOrganize` dedupes by hash, so three consecutive passes over the same unorganized torrent produce one job.

**Don't "fix" by:** adding a `Stat` on `LibraryPaths` to "detect" missing library files, or clearing `LibraryPaths` when the paths no longer resolve — both reintroduce the resurrection loop. A user who wants the link back should redownload or replace the episode, which resets `LibraryPaths` through the merge in decision 27.

**One exception, see decision 34:** when the *entire download root* is detected as swapped (moved/trashed/replaced folder), `clearLibraryPathsAfterRootSwap` wipes the records once. That is a different event from a per-file deletion and is edge-triggered, so it cannot become the loop this decision guards against.

---

### 30. Progress data comes from one `Stats()` per torrent, pulled only while a screen is open

**Location:** `internal/torrents/session.go` (`toInfo`); `internal/api/endpoint_torrents.go`; `frontend/src/routes/Downloads.svelte`; `frontend/src/routes/Status.svelte`.

**What it looks like:** `TorrentInfo` carries a dozen progress fields filled from a single
`t.Stats()` call, and the WebUI polls `GET /api/v1/torrents` on a plain client-side timer
while a screen that needs it is mounted — instead of the more familiar "push updates over
the existing WebSocket". Two screens poll it independently, at different rates: `Downloads.svelte`
every 2s (`setInterval(load, 2000)`) while the downloads screen is open, and `Status.svelte`
every 5s (`torrentsPollInterval = setInterval(loadTorrents, 5000)`) for the global speed
card, since Status is the default `#/` landing route. Both intervals are cleared on
unmount, so the endpoint is only hit while at least one of those two screens is on screen.

**Why it's right:** rain's `Stats()` is not a getter — it is a blocking round-trip into that
torrent's goroutine (`torrent/torrent_commands.go:141`), and so are `Peers()`, `Trackers()`
and `Webseeds()`. Reading fields one at a time, or on a server-side ticker, would hammer
every torrent's goroutine forever, including when nobody is looking. Pulling on demand costs
exactly zero when no screen is open. The same reasoning keeps `Peers()`/`Trackers()` out of
the list route entirely.

**Don't "fix" by:** adding a parallel `Stats()`-per-field accessor; moving the poll to a
server-side ticker or the WebSocket without gating it on `len(wsm.clients) > 0`; or calling
`Peers()`/`Trackers()`/`FileStats()` from the list endpoint.

**Related:** the API exposes a hand-written status slug (`statusSlug`) rather than rain's
`Status.String()`, which returns display text with a space (`"Downloading Metadata"`) and
can be reworded by any library upgrade. And `Session.Resume` re-arms the per-torrent listener,
because pausing makes the one-shot `NotifyStop` fire and kills the goroutine — without the
re-arm a resumed torrent would complete without ever enqueuing `JobOrganize`. Finally,
`Completed` is computed by `completedFromStats` from piece counts (`st.Pieces.Have >=
st.Pieces.Total`), not from `Status`: pausing a fully-downloaded torrent takes it out of
`Seeding`, and both `jobs.go`'s retry gate and `verification.go`'s reconciliation gate on
`Completed`, so deriving it from `Status` would make a paused-but-finished torrent invisible
to organization — the plan originally specified `Completed: st.Status == torrent.Seeding`,
and this was corrected during review specifically because `Pause` made the bug reachable.
The same lesson applies one layer up, to bytes rather than pieces: `Torrent.Stop()` frees
rain's piece data (`closeData` nils `t.pieces`), which zeroes `Bytes.Completed` while
`Bytes.Total` survives, so `buildTorrentResponse` falls back to the piece ratio
(`PiecesHave`/`PiecesTotal`) whenever `BytesCompleted` reads 0 — otherwise a torrent paused
mid-download renders an empty progress bar for its whole paused lifetime.

---

### 31. Diretório de download derivado da biblioteca

**Location:** `internal/files/filemanager.go` (`Config.DownloadPath`), `internal/files/librarian.go` (`ProbePath`), `internal/daemon/migration.go`.

**What it looks like:** `save_path` sumiu da configuração; o diretório de download é `<completed_anime_path>/.torrents`, calculado a cada uso.

**Why it's right:** a restrição de mesmo-filesystem do `#21` era uma armadilha que só aparecia como erro no save; derivando o caminho ela vira invariante. Dois campos obrigatórios sem diferença clara confundiam o usuário, para quem só a biblioteca importa. `ProbePath` continua existindo porque exFAT/FAT32/alguns SMB não têm hardlink nenhum. O `.ignore` + o prefixo com ponto mantêm o Jellyfin fora da pasta de download.

**Don't "fix" by:** reintroduzir `save_path` como campo de config ou variável de ambiente; tirar a guarda que faz `DeleteEmptyFolders` pular `.torrents`.

**Amendment (2026-08-24): a migração foi REMOVIDA.** `daemon.MigrateSavePath`, o campo
`Config.SavePath`, a zeragem dele no `PUT /config` e os seis testes de migração não existem mais.
O que sobrou da decisão é só o caminho derivado (`Config.DownloadPath`).

Consequência que precisa ficar explícita: uma instalação que **ainda não migrou** (um
`config.json` com `save_path` preenchido, de uma versão anterior) perde o vínculo com os dados.
O campo agora é desconhecido, o JSON o ignora, a rain abre o caminho derivado, acha vazio e
**rebaixa a biblioteca inteira**. Mover `<save_path>/*` para `<completed_anime_path>/.torrents`
antes de subir a versão nova passou a ser um passo manual, documentado no README.

As três coisas que a migração garantia e agora ninguém garante: o rename preserva inode (seed e
hardlinks sobrevivem), o marcador de raiz (`torrents.RootMarkerName`) viaja junto com os dados —
sem isso o `Ensure` seguinte enxerga troca de pasta e zera os `LibraryPaths` (ver #34) — e a
operação era idempotente com retry a cada passe. Quem reintroduzir a migração precisa dos três,
não só do rename.

---

### 32. `DELETE /torrents/{hash}`: default é apagar + bloquear, `keep_data` é binário, não há endpoint de lote

**Location:** `internal/api/endpoint_torrents.go` (`handleTorrentDelete`, `parseBoolQueryParam`), `internal/daemon/episodes.go` (`RemoveTorrentWithEpisodes`, `RemoveTorrentOptions`, `removeEpisodesAndLinks`), `frontend/src/components/TorrentDeleteDialog.svelte`.

**What it looks like:** a UI de exclusão da tela de Downloads (linha única ou seleção em massa) abre um diálogo com dois checkboxes — "apagar arquivos" e "não baixar de novo" — **ambos marcados por padrão**. Desmarcar o primeiro vira `keep_data=true` (mantém a cópia semeada **e** o hardlink da biblioteca — nunca só um dos dois). Desmarcar o segundo vira `block=false` e a UI mostra um aviso em texto amarelo em vez de impedir a combinação. Não existe `DELETE /torrents/bulk`; o cliente dispara N requisições ao endpoint por hash já existente.

**Why it's right:**

- **Delete + block por padrão:** é a ação que corresponde à intenção mais comum de quem clica em "excluir" — livrar-se do torrent de vez. Deixar os dois desmarcados por padrão faria o caso comum exigir dois cliques extras; deixar só "apagar" marcado por padrão recriaria silenciosamente o torrent no próximo passo do loop (ver próximo ponto).
- **"Excluir sem bloquear" é um rebaixar-do-zero, não um cancelar:** `handleAlreadySavedEpisode` (`daemon/daemon.go`) rebaixa (redownloada) qualquer episódio cujo registro em `episodes.json` exista mas cujo hash não bata com nenhum torrent vivo na sessão — é assim que o daemon se recupera de um torrent removido por fora (crash, limpeza manual do rain). `DELETE /torrents/{hash}` com `block=false` produz exatamente esse estado: registro sobrevive (a menos que `keep_data` também tire os hardlinks), torrent não. Não há como o backend distinguir "usuário quer parar de baixar isso" de "torrent morreu e precisa recomeçar" só olhando o registro — por isso a UI avisa por texto ("sem isso, o daemon pode baixar de novo") em vez do backend proibir ou forçar a combinação. Bloquear é a única forma de expressar "não, mesmo" para esse fluxo.
- **`keep_data` não separa biblioteca de cópia semeada:** o hardlink da biblioteca e o arquivo semeado pelo torrent são o mesmo inode (decisão 21). Apagar um e manter o outro não libera espaço nenhum — só quebra a exibição no Jellyfin ou para de semear sem ganhar disco. `removeEpisodesAndLinks` trata isso como um parâmetro binário único (`keepData`): ambos ficam ou ambos vão.
- **Sem endpoint de lote:** o pior caso realista é dezenas de torrents selecionados, não milhares — N requisições HTTP em paralelo (`Promise.all` no frontend) resolve isso sem exigir semântica de erro parcial no backend (o que falhou? o quê retry?) nem um novo formato de payload. O custo de implementar e manter um body `{hashes: []string}` com resposta por-item não compensa para essa escala.
- **404 do DELETE decidido só pela sessão:** igual a `pause`/`resume`/`announce` (`torrentAction`), a existência do torrent é checada só via `server.Torrents.Get(hash)`. Se o hash não está na sessão mas ainda existem registros de episódio com aquele hash (órfãos — torrent já removido por fora), a rota responde 404 e **não mexe** nesses registros; limpar um registro órfão é responsabilidade de `DELETE /animes/{id}/episodes/{episodeId}`, não desta rota. Misturar as duas responsabilidades tornaria o contrato de erro ambíguo (404 significa "torrent não existe" ou "nada para fazer"?).

**Fora de escopo (investigado, não esquecido):**

- **Limite de downloads simultâneos:** exigiria um campo de config novo, uma fila real no daemon, e a persistência do *motivo* de um torrent estar pausado — a rain persiste o estado pausado (decisão 25), mas não o motivo, então sem esse campo extra o scheduler retomaria por engano um torrent que o usuário pausou de propósito. Vira spec própria.
- **Prioridade por torrent:** `github.com/cenkalti/rain/v2` não tem nenhum conceito de prioridade, nem por torrent nem por arquivo — só `SpeedLimitDownload`/`SpeedLimitUpload` globais. Implementar throttling próprio em cima disso mexeria no caminho quente da lib por um ganho que não compensa. Se a fila de simultâneos acima vier a existir, a ordem da fila já é a prioridade, e um botão "baixar primeiro" que move o item para o topo da fila entrega o valor sem exigir scheduler nenhum.

**Don't "fix" by:** fazer o backend recusar `block=false` sozinho (a combinação é válida e intencional — só não é o padrão); tentar separar biblioteca de cópia semeada adicionando um terceiro parâmetro (mesmo inode, ver decisão 21); adicionar um endpoint de lote "porque é óbvio" sem antes confirmar que a escala realista (dezenas, não milhares) justifica a complexidade extra de erro parcial.

---

### 33. daisyUI fica travado na v4 enquanto o Tailwind for v3 (e o inline de progresso não pode olhar `is_downloaded`)

**Location:** `frontend/package.json` (`daisyui: ^4`), `frontend/tailwind.config.js`, `frontend/src/components/Layout.svelte` (tooltip do WebSocket), `frontend/src/routes/AnimeDetail.svelte` (inline de progresso do torrent), `internal/api/endpoint_anime_episodes.go`.

**What it looks like:** o projeto usa Tailwind 3.4 + daisyUI **4**, não a 5. E a barra de progresso inline na tela do anime aparece com base só em `!torrent.completed` — nunca em `!ep.is_downloaded`.

**Why it's right:**

- **daisyUI 5 é CSS-first e só suporta Tailwind 4.** Rodando pelo plugin JS no Tailwind 3, ela emite todo o CSS de componente dentro de `@layer daisyui.l1.l2.l3` (cascade layers reais), enquanto o preflight do Tailwind 3 sai **sem layer nenhum**. CSS sem layer sempre ganha de CSS em layer, independente de especificidade — então `button { background-color: transparent; padding: 0 }` e `*,::before,::after { border-width: 0 }` do preflight atropelavam `.btn` e `.checkbox`. Resultado visível: botões daisyUI viravam texto puro sem fundo/borda/padding, e checkboxes ficavam literalmente invisíveis (só o ✓ aparecia quando marcado). Nada disso é ajustável por config: não dá para pôr o preflight do Tailwind 3 numa layer (`@layer tw-base { @tailwind base }` faz o Tailwind 3 estourar "no matching @tailwind directive"), nem para tirar a daisyUI 5 das layers.
- **A v4 é a versão feita para o Tailwind 3** e emite CSS sem layer, então convive com o preflight pela cascata normal. Todas as classes em uso aqui (`btn*`, `checkbox*`, `badge*`, `progress*`, `card*`, `menu`, `dropdown*`, `modal*`, `join`, `alert*`, `input-bordered`, `select-bordered`, `table*`, `base-100/200/300`, `base-content/NN`) existem na v4 — inclusive `input-bordered`/`select-bordered`/`btn-outline`, que são idioma v4 e denunciam que o código foi escrito para ela antes de alguém bumpar a major.
- **O histórico do repo é feito de remendos deste mesmo bug**: `--btn-shadow: 0` global, "substituir btn DaisyUI por classes Tailwind diretas" (é daí que vêm os `bg-blue-600`/`bg-red-600` crus em `Status.svelte` e `AnimeDetail.svelte`), e a troca de `data-tip` por `.tooltip-content`. Esse último é API só da v5 e, na v4, renderiza o texto do tooltip **sempre visível** ao lado do ponto de conexão na navbar — por isso o `Layout.svelte` voltou para `data-tip`.
- **`episode_hash != "" ⟺ is_downloaded`.** `handleAnimeEpisodes` preenche `IsDownloaded` e `EpisodeHash` no mesmo `if` (existe registro salvo para aquele nó), e o daemon grava o registro salvo no instante em que o torrent é **adicionado**, não quando termina (`daemon/episodes.go`, `daemon/manual_download.go`). Ou seja, um episódio baixando **já** vem com `is_downloaded: true`. A condição antiga do inline (`torrent && !torrent.completed && !ep.is_downloaded`) exigia uma combinação que a API nunca emite, então a barra jamais aparecia em produção — e o smoke test não pegou porque a fixture montava justamente esse estado impossível.

**Don't "fix" by:** subir a daisyUI para 5 "porque é a mais nova" sem migrar o Tailwind para 4 junto (volta o bug de layer inteiro, e de forma silenciosa: só componentes cujo preflight conflita quebram); usar `.tooltip-content` no lugar de `data-tip`; reintroduzir classes Tailwind cruas no lugar de `btn` "porque o btn não pinta" — isso era sintoma do problema de layer, não limitação da daisyUI; voltar a filtrar o inline de progresso por `!ep.is_downloaded`; escrever fixture de teste com `is_downloaded: false` + `episode_hash` preenchido.

---

### 34. Troca da pasta de download é detectada por marcador duplo, e derruba a sessão da rain

**Location:** `internal/torrents/rootmarker.go`, `internal/torrents/sessionmanager.go` (`Ensure`, `checkRoot`, `writeRoot`), `internal/daemon/verification.go` (`clearLibraryPathsAfterRootSwap`), `internal/daemon/migration.go` (o marcador viaja junto no rename). Fixado por `TestSessionManagerDetectsRootSwap`, `TestSessionManagerRootSwapReportedOnce`, `TestSessionManagerMovedAndRepointedIsNotASwap`, `TestSessionManagerFirstRunAdoptsExistingFolder` e `TestClearLibraryPathsAfterRootSwap`.

**What it looks like:** existem dois arquivos com o mesmo id aleatório — `.aad_root` **dentro** da pasta de download e `download_root.id` na pasta de config. `Ensure` compara os dois a cada passe e, quando divergem, fecha e recria a sessão da rain.

**Why it's right:** a rain segura *file descriptors* abertos. Quando o usuário move a pasta de download (ou manda pra lixeira) com o daemon rodando, os bytes continuam caindo no inode que foi embora enquanto o caminho configurado fica vazio — a UI mostra tudo semeando e **todo** `JobOrganize` falha com `no such file or directory`, queimando as 20 tentativas em silêncio. Nada no processo percebe, porque o caminho em string não mudou: só o inode por trás dele mudou.

O par de marcadores é o que torna isso detectável sem depender de inode (que não existe no Windows):

- o marcador **de dentro** viaja com a pasta, então some do caminho configurado exatamente quando a pasta é movida;
- o id **de fora** fica onde o usuário não mexe, então sobrevive para servir de referência — inclusive quando a pasta é movida com o daemon **parado**.

Recriar a sessão é o conserto completo porque a rain já faz a parte difícil: em `torrent_allocation.go`, se os arquivos não existem (`al.HasMissing`), ela **descarta o bitfield do resume** e rebaixa. Não é preciso mexer no resume db nem remover torrent nenhum.

**Por que o sinal é latch e não retorno do `Ensure`:** os endpoints de download manual também chamam `Ensure`. Se a flag viesse no retorno, uma chamada dessas engoliria a troca — a sessão seria recriada, mas os registros de biblioteca órfãos nunca seriam limpos, e a recuperação ficaria pela metade sem nenhum erro visível. `Ensure` marca `pendingSwap`; só `ConsumeRootSwap`, chamado no passe de verificação, limpa. Fixado por `TestSessionManagerRootSwapSurvivesAnotherEnsure`.

Três propriedades que caíram de graça e são intencionais:

- **Mover a pasta E repontar o config** (`completed_anime_path` para o novo lugar) **não** é troca: o marcador viajou junto, os ids batem, nada é rebaixado. É o caminho "certo" de mover a biblioteca.
- **Primeiro boot / upgrade de uma versão sem marcador** não é troca: sem id de referência não há o que comparar, e adotar a pasta como está é a única leitura segura — a alternativa apaga os registros de uma biblioteca saudável.
- **É edge-triggered:** `Ensure` reporta a troca uma vez e já regrava o marcador, então o passe seguinte volta ao normal.

**Emenda à decisão 29:** `clearLibraryPathsAfterRootSwap` zera `LibraryPaths` — justamente o que a #29 proíbe. A proibição continua valendo para arquivo faltando: o que ela protege é a exclusão deliberada de **um** episódio da biblioteca, que um cheque por `Stat` a cada passe ressuscitaria para sempre. Uma troca de raiz é outro evento: sumiu a pasta inteira para onde os registros apontam, o daemon já está rebaixando o conteúdo dela, e a detecção dispara **uma vez por troca** — nunca vira o laço de ressurreição da #29. Sem essa limpeza a recuperação fica pela metade: os torrents rebaixam, mas os episódios organizados antes da troca ficam com `LibraryPaths` órfão e nunca voltam para a biblioteca.

**Don't "fix" by:** devolver a flag no retorno do `Ensure` em vez do latch (ver acima); trocar os marcadores por comparação de inode (`syscall.Stat_t.Ino` não existe no Windows); tratar erro de leitura do marcador como "sumiu" (uma falha de permissão passaria a apagar os registros da biblioteca — por isso `readRootID` só engole `IsNotExist`); fazer a limpeza de `LibraryPaths` a cada passe em vez de só na troca (aí sim vira a violação da #29); ~~tirar o rename do marcador em `MigrateSavePath`~~ (a migração foi removida em 2026-08-24 — ver #31; se ela voltar, o rename do marcador volta a ser obrigatório pelo mesmo motivo).

---

### 35. `os.SameFile` no Windows resolve o arquivo tarde, então FileInfo não é snapshot em teste

**Location:** `internal/files/librarian_test.go` (`TestOrganizeReplacesDifferentFileAtDestination`, o `staleAlias`). O uso em produção fica em `internal/files/librarian.go:156`.

**What it looks like:** o teste cria um hardlink extra (`staleAlias`) para o arquivo velho **antes** de chamar `Organize`, e depois compara `os.Stat(staleAlias)` contra o destino — em vez de guardar um `os.Stat(dest)` antes da troca, que seria o jeito óbvio.

**Why it's right:** no Linux `os.Stat` grava dev+inode na hora, então um `FileInfo` é um retrato do arquivo daquele instante. No Windows não: `saveInfoFromPath` (`os/types_windows.go`) guarda **só o caminho**, e `SameFile` chama `loadFileId`, que abre esse caminho de novo no momento da comparação. Um `FileInfo` tirado de `dest` antes da substituição passa a apontar para o arquivo **novo** depois dela, e `os.SameFile(staleInfo, destInfo)` vira sempre `true` — o teste falhava só no job `test-backend-windows`, com o código de produção correto. Manter um segundo nome para o arquivo velho fixa a identidade dele em qualquer plataforma, porque `Organize` substitui com `Remove(dest)` + `link(src, dest)` e o hardlink alternativo sobrevive ao `Remove`.

Em `librarian.go:156` o mesmo `os.SameFile` está correto: os dois `Stat` são feitos na hora da comparação, com os dois caminhos existindo.

**Don't "fix" by:** trocar o alias por um `os.Stat(dest)` guardado antes da troca ("é a mesma coisa e lê melhor" — não é, e só quebra no Windows); marcar o teste como `t.Skip` no Windows (era exatamente o cross-device/hardlink que esse job existe para cobrir); apagar a asserção do arquivo velho por parecer redundante com a de conteúdo (uma mutação que pula a substituição faz as duas falharem, mas a de identidade é a que distingue "relinkou" de "sobrescreveu por cima").

---

### 36. `AppShell` escolhe rail vs tab bar em JS (`matchMedia`), não com `hidden md:flex`/`md:hidden`

**Location:** `frontend/src/components/shell/AppShell.svelte`, `NavRail.svelte`, `NavTabBar.svelte`, `MoreMenu.svelte` (Fase 1 do redesign de UI, spec §5).

**What it looks like:** `AppShell` guarda um booleano `isDesktop` (inicializado sincronamente de `window.matchMedia('(min-width: 768px)')`, atualizado por um listener de `change`) e monta `{#if isDesktop}<NavRail>{:else}<NavTabBar>{/if}` — só um dos dois existe no DOM a qualquer momento. O padrão Tailwind mais comum para "rail no desktop, tab bar no mobile" seria montar os dois sempre e escondê-los por classe (`hidden md:flex` / `md:hidden`), como o `Layout.svelte` antigo fazia para seu bloco desktop/mobile.

**Why it's right:** `NavRail` e `NavTabBar` cada um monta seu próprio `MoreMenu`, que no mobile hospeda um `<select id="theme-select-mobile">` (o desktop tem o seu próprio `<select id="theme-select">`, sempre visível no rodapé do rail). Com os dois blocos sempre no DOM e só escondidos por CSS, ambos existiriam ao mesmo tempo — dois elementos com o mesmo propósito de controle, potencialmente dois ids duplicados se algum dia convergirem, e dois menus "Mais" interativos simultâneos para ferramentas que não respeitam `display:none` puramente por visibilidade (ex. `getByLabelText`/`getByRole` do Testing Library encontram elementos ocultos por CSS a menos que a query filtre por `hidden: true`, e podem falhar com "found multiple elements"). Montar só um dos dois em JS elimina a classe inteira desse problema, ao custo de um listener de `matchMedia` — inicializado de forma síncrona no `<script>` do componente (antes do primeiro render), então não há flash de layout errado.

**Don't "fix" by:** voltar para `hidden md:flex`/`md:hidden` "porque é mais simples e é o padrão Tailwind" — funciona visualmente, mas reintroduz o duplo-DOM que motivou a decisão; inicializar `isDesktop` só dentro de `onMount` (mostraria o layout errado por um frame antes do primeiro paint, já que a leitura síncrona de `matchMedia` no `<script>` é o que evita isso).

---

### 37. Tabelas de largura fixa só a partir de `lg`, e `min-w-0` obrigatório em item de grid que contém faixa rolável

**Location:** `frontend/src/routes/Status.svelte` (`LIST_GRID` e o card herói), `Downloads.svelte` (`ROW_GRID`/`ROW_INDENT`), `AnimeDetail.svelte` (`EP_GRID`). Coberto por `frontend/tests/smoke/layout.spec.ts`.

**What it looks like:** duas coisas que parecem redundantes e não são.

1. A faixa "Downloads ativos" do card herói tem `overflow-x-auto` **e** a `<section>` em volta tem `min-w-0`. Parece cinto e suspensório — o `overflow-x-auto` sozinho deveria bastar para a faixa rolar.
2. As três tabelas trocam para cards empilhados em `lg` (1024px), não em `md` (768px), mesmo o app inteiro usando `md` como breakpoint desktop/mobile (inclusive o `matchMedia` da decisão 36, que continua em 768px).

**Why it's right:**

1. Item de grid tem `min-width: auto`, ou seja, seu piso é o tamanho intrínseco do conteúdo, não zero. Com muitos downloads a `<section>` era esticada até o max-content e o `overflow-x-auto` **nunca ativava**: medido num grid de 1280px, a section ia a 2494px, a faixa ficava com `clientWidth == scrollWidth` e a coluna da direita saía da tela. Com `min-w-0` a mesma section volta a 677px e a faixa rola de verdade (639px visíveis para 2456px de conteúdo). O `overflow-x-auto` descreve a intenção; o `min-w-0` é o que permite que ela aconteça.

2. É aritmética, não gosto: as trilhas fixas somam ~754px (Status), ~740px (Downloads) e ~710px (AnimeDetail), enquanto a largura útil em `md` é `768 − 92 (rail) − 48 (padding do main) ≈ 628px`. O corte em `md` estava errado desde sempre — a faixa 768–880px caía em rolagem horizontal — e só não incomodava porque ninguém abre o app nessa largura. `lg` é o primeiro breakpoint em que as colunas cabem. O breakpoint rail-vs-tab-bar continua em 768px de propósito: é sobre o **shell**, não sobre a largura que uma tabela exige.

**Don't "fix" by:** tirar o `min-w-0` "porque já tem `overflow-x-auto`" (é a ordem invertida: sem `min-w-0` o overflow é decorativo); tirar o `overflow-x-auto` "porque o `min-w-0` já resolve" (aí a faixa corta o conteúdo em vez de rolar); baixar as tabelas de volta para `md` por consistência com o resto do app (refaça a conta acima antes — a largura útil em `md` não cobre nenhuma das três); acrescentar uma coluna de largura fixa sem revisar o comentário de aritmética que mora junto de cada constante de grid.

---

### 38. Em cabeçalho `flex-wrap`, o item `flex-1` leva `min-w-[240px]` — e não `min-w-0`

**Location:** `frontend/src/routes/Downloads.svelte` (bloco do título, no topo do template). Coberto por `frontend/tests/smoke/layout.spec.ts` ("subtitle is not squeezed to one word per line").

**What it looks like:** o bloco de título/subtítulo é `min-w-[240px] flex-1`, contrariando o `min-w-0` que a decisão 37 exige nos itens de grid/flex do resto do app.

**Why it's right:** são problemas opostos. `min-w-0` serve para item que **contém uma faixa rolável** e por isso precisa poder encolher abaixo do conteúdo. Aqui o item é vizinho de uma caixa larga (a barra de banda ↓/↑, 433px de max-content) dentro de um container `flex-wrap`, e a quebra de linha de um container flex decide pelo **tamanho principal hipotético** de cada item — que para `flex-1` (flex-basis 0%) é ~0, e `min-width` é justamente o que entra nesse cálculo. Com `min-w-0` o título contribuía ~0, a barra nunca ia para a linha de baixo e o título ficava com as sobras: medido, 7 linhas de subtítulo em 500px e 4 em 768px, uma palavra por linha. O piso de 240px empurra a barra para a próxima linha em qualquer largura em que ela não caiba.

**Don't "fix" by:** trocar por `min-w-0` "para ficar igual ao resto do app" (é o bug de volta — a regra da decisão 37 vale para item que hospeda scroll, não para item vizinho de conteúdo largo); resolver com breakpoint (`flex-col sm:flex-row`) — conserta 375px e deixa a faixa dos 500–768px espremida, que é exatamente onde o sintoma era pior; assumir que `flex-wrap` sozinho já quebra a linha (ele quebra pelo tamanho hipotético, não pelo tamanho final depois de flexionar).

---

### 39. Faixa de escolhas FINITA quebra linha (`flex-wrap`); faixa de conteúdo ILIMITADO rola

**Location:** `frontend/src/components/DownloadsToolbar.svelte` (pills de filtro), o `<nav>` do índice em `frontend/src/routes/Config.svelte`, e a fileira de ações do daemon no cabeçalho de `frontend/src/routes/Status.svelte`. Cobertos por `frontend/tests/smoke/layout.spec.ts` ("every downloads filter pill…", "every config group and exit link…", "status header daemon actions stay on screen").

**What it looks like:** as três faixas são `flex flex-wrap`, sem `overflow-x-auto` e sem `min-w-0` — enquanto a faixa "Downloads ativos" do card herói (decisão 37) continua rolável.

A fileira do Status mostra a variante em que o sintoma não é rolagem e sim **corte**: ela era `flex shrink-0 gap-2`, sem faixa rolável nenhuma. Como item de um cabeçalho `flex-wrap`, o `shrink-0` travava sua largura no max-content de uma linha só (358px em `en`, 438px em pt-BR), então ela nunca recebia uma linha estreita em que pudesse quebrar e "Parar Daemon" saía pela direita em ≤414px no português. Tirar o `shrink-0` faz o piso passar a ser o min-content (o botão mais largo) e a quebra acontecer. Medido depois do fix: os botões mantêm 148/150/124px em toda largura — nenhum encolhe, então nenhum rótulo quebra no meio, e o `shrink-0` nos FILHOS (que a decisão pedia para as pills) não foi necessário aqui.

**Why it's right:** o critério é se a faixa oferece um conjunto FECHADO de escolhas ou uma lista de tamanho arbitrário. Escolha escondida atrás de arrasto é escolha que não existe: o usuário não sabe que há mais. As pills existem para responder "tem algo ali?" pela contagem, e "Semeando"/"Problemas" ficavam fora da vista em tela estreita. O índice do Config tem quatro grupos mais dois links de saída — em 390px, 358px de espaço para 644px de conteúdo, ou seja metade escondida, inclusive "Torrent search" e os dois links, que são o único caminho até Prioridades e Notificações a partir dessa tela. A faixa do herói é diferente porque a lista de downloads ativos não tem teto: nenhum layout põe N torrents na tela, e aí rolar é o degrade correto.

Nenhuma das duas leva `min-w-0`: o mínimo automático do item é o min-content (a pill/rótulo mais largo), que é exatamente o piso desejado — com `min-w-0` o item voltaria a estourar em vez de quebrar.

No índice do Config os divisores são `w-full`, então a quebra acontece neles: as três fileiras resultantes (grupos do dia a dia / grupo avançado / links de saída) são a arquitetura da tela, que no desktop a coluna diz implicitamente.

**Revisão:** até 2026-08-11 esta decisão listava o índice do Config no lado "rolável", classificando-o como "lista longa e homogênea". Estava errado nos fatos — são seis itens, não uma lista longa, e a medição mostrou metade fora da tela. O critério acima ("finito vs. ilimitado") é o que substitui aquele.

**Don't "fix" by:** devolver `shrink-0` à fileira do Status "para os botões não encolherem" (é o bug de volta — `shrink-0` no CONTAINER trava a quebra; o que protege o rótulo é `shrink-0` no filho, e aqui não é nem necessário); devolver `overflow-x-auto` a qualquer das três "por consistência com a faixa do herói" (o herói resolve outro problema — lista sem teto); tirar o `shrink-0` dos itens (passariam a encolher e o rótulo quebraria no meio, em vez de o item inteiro descer de linha); trocar o índice do Config por um `<select>` no mobile (`<option>` não estiliza, então o ponto de "falta preencher" viraria texto, e os dois links de saída deixariam de ser `<a>` — perdendo abrir-em-nova-aba, que é o motivo de serem `<a>`).

---

### 40. O `z-30` do `NavRail` não é redundante com o `z-50` do painel do `MoreMenu`

**Location:** `frontend/src/components/shell/NavRail.svelte` (classe do `<nav>`), `MoreMenu.svelte` (painel `absolute z-50`, backdrop `fixed inset-0 z-40`). Coberto por `frontend/tests/smoke/layout.spec.ts` ("the More menu paints above the page content", "a modal still paints above the nav rail").

**What it looks like:** o painel do menu já é `z-50`, e ainda assim o `<nav>` em volta precisa de `z-30` — parece número sobrando, e a tentação é remover um dos dois.

**Why it's right:** `position: sticky` cria contexto de empilhamento **sempre**, independente de z-index. Sem z-index no `<nav>`, aquele `z-50` só ordena coisas **dentro** do rail, e o rail inteiro pinta na camada z-auto da raiz, em ordem de árvore — perdendo para qualquer elemento posicionado que venha depois no DOM. Foi o que acontecia na tela de Prioridades, cujos cards usam a classe `.card` do daisyUI (`position: relative`): os cards apareciam na frente do menu aberto. Com `z-30` (o mesmo valor do `NavTabBar`, que é `fixed`), o rail inteiro sobe junto. O `z-40` do backdrop e o `z-50` do painel continuam necessários para ordenar o menu **contra o próprio rail**; Modal e Toasts ficam em `z-50` na raiz e por isso continuam acima dos 30 do rail.

**Don't "fix" by:** subir o rail para `z-50` "para ficar igual ao painel" (empataria com Modal/Toasts e o diálogo passaria a depender de ordem de árvore); tirar o `z-30` porque "sticky já fica na frente" (ele fica na frente do conteúdo **não posicionado** — qualquer `position: relative` depois no DOM ganha dele); tirar o `z-50` do painel achando que o `z-30` do nav basta (dentro do rail o painel precisa vencer o backdrop `z-40`).

---

### 41. A fila de downloads contém TODOS os incompletos, é persistida, e `queued` é o único slug que a rain não produz

**Location:** `torrents/queue.go` (`queue`, `enforce`, `markQueued`, `load`/`save`), `torrents/sessionmanager.go`, `torrents/status.go` (`StatusQueued`), `files.Config.MaxConcurrentDownloads`. Frontend: `lib/utils/torrentStatus.ts`, `torrentFilters.ts` (`KNOWN_STATUSES`, `isProblemTorrent`, `prioritizeOrder`), `routes/Downloads.svelte` (`canPause`/`canResume`/`canPrioritize`).

**What it looks like:** um monte de coisa que parece descuido, e é toda deliberada.

**(a) A fila é uma lista de TUDO, não só de quem ela pausou.** `queue.order` contém todo torrent incompleto que a sessão conhece; os `limit` primeiros que não estão em `queue.paused` baixam. A versão anterior guardava só os hashes que a fila tinha pausado, e os três defeitos que isso produzia eram estruturais: priorizar com o limite cheio não privava ninguém (não havia excesso a rebaixar nem folga a promover, então o priorizado só começava quando um slot vagasse por conta própria); priorizar em lote invertia a ordem (cada `pushFront` passava na frente do anterior); e não havia posição estável para mostrar na tela. Com a fila contendo tudo, nada é inserido no meio — a posição só muda por ação do usuário — e some a pergunta "onde este hash entra". Isso apagou `insertByAddedAt`, `leastProgressed` e o desempate por progresso. `AddedAt` sobrevive por um motivo só: dar ordem inicial aos torrents que a fila ainda não conhece (passo 2 do `enforce`).

**(b) `enforce` é reconciliação, não reação a evento — e o passo 0 é o que impede a fila de morrer em todo boot.** `SessionManager.list()` devolve `nil` enquanto não há sessão e slice vazio **não-nil** para uma sessão sem torrents. `nil` significa "não sei nada", não "não há nada": sem a guarda, o passo 1 podaria `order`/`paused` inteiros e o passo 5 gravaria o arquivo vazio por cima — em **todo** boot, porque `verification.go` chama `SetMaxActiveDownloads` (que roda `enforce`) **antes** do `Ensure`. É também o que garante que uma falha se conserta sozinha: todo passe de verificação roda `SetMaxActiveDownloads`, então um `pause` que a rain recusou ou um `stopping` que ficou no meio do caminho se resolve no ciclo seguinte, sem ticker novo nenhum.

**(c) O passo 4 itera `order`, NUNCA a sessão.** Torrent completo não entra em `order` (seeding não ocupa slot), então varrer a sessão marcaria todo seeder como "não deveria estar ativo" e **pausaria a biblioteca inteira**. `order` é a lista dos incompletos e é exatamente o conjunto sobre o qual a fila tem autoridade; o que está fora dela a fila não toca, nem para pausar nem para retomar. Por isso `Pause`/`Resume` de um torrent completo **bypassam a fila nos dois sentidos**, delegando direto para a rain — sem o bypass, e com o passo 4 restrito a `order` (como deve ser), pausar um seeder não pararia nada. Consequência: `paused` guarda só hashes incompletos.

**(d) `stopping` não é alvo de nada.** A pausa da rain é assíncrona (`Torrent.Stop()` volta na hora, o status só chega a `stopped` até ~5s depois). Tratá-lo como parado faria o `enforce` mandar `resume` por cima de um stop em andamento; tratá-lo como rodando faria o passo 4 pausá-lo de novo. Fica de fora dos dois lados e o passe seguinte decide sobre um estado estável.

**(e) A preempção escolhe por POSIÇÃO, não por progresso.** Sai o último dos ativos em ordem de fila — um download em 95% pode ser pausado se estiver depois na fila, coisa que o `leastProgressed` antigo evitava. Aceito: posição é o eixo que o usuário controla, e a alternativa é ressuscitar a peça que (a) apagou. Nada é perdido de qualquer forma: a rain zera `Bytes.Completed` ao parar, mas o bitfield das peças sobrevive.

**(f) `Resume` põe no FIM da fila, não inicia na hora.** Com limite de downloads simultâneos, um resume que iniciasse incondicionalmente furaria o limite em silêncio — e um limite que a UI mostra sendo violado é um limite em que ninguém confia. Quem quer "começa agora" chama `Prioritize`, que vai para a frente e preempta. Por isso a linha de um enfileirado tem **três** botões, cada um numa direção: Priorizar sobe, Play desce para o fim, Pause tira da rotação. Downloads manuais usam `daemon.addAndPrioritize` — furam a fila, não o limite.

**(g) `queued` é slug de status, não campo booleano — e a posição sai do mesmo mapa.** É o único valor de `TorrentInfo.Status` que não sai de `statusSlug()`: para a rain um enfileirado é só mais um torrent parado. A fonte é `q.queued`, o mapa que o passo 3 acabou de calcular, **não** `order` — marcar por pertencer a `order` pintaria a tela inteira de `queued`, já que agora ela contém os ativos também. Quem está em `paused` mostra `stopped` (o que a rain reporta), e `queued` fica significando só "a fila vai chegar nele". `QueuePosition` 0 = não enfileirado, o que inclui todo pausado: ele tem posição em `order`, mas ela não prevê nada, e publicar um número que não vai acontecer é pior do que não publicar nenhum. Quando um `pause` falha, o hash **sai de `q.queued`** mantendo a posição em `order` — a tela mostra o que está acontecendo, ao custo de um buraco na numeração que dura até o próximo passe.

**(h) A fila é persistida em `queue.json`, ao lado do banco de resume.** Mesmo lugar e mesmo motivo do `download_root.id`: é estado do torrent client, não configuração do usuário, e precisa acompanhar o banco de resume. Isso mata o teto que a versão anterior marcava com `ponytail:` — a pausa manual agora sobrevive a restart. A "adoção de boot", que rodava a cada sessão criada e retomava por engano o que o usuário tinha pausado, encolheu para um **latch de uma vez só**: quando não existe `queue.json`, a leitura arma `seedPaused` e o primeiro `enforce` **com sessão** marca como `paused` os incompletos que a rain reporta parados. Não dá para semear na leitura — em `NewSessionManager` ainda não existe sessão para consultar. O preço, real e inevitável: nesse boot de upgrade os torrents que a fila antiga (só memória) pausou são indistinguíveis dos que o usuário pausou, então os dois grupos congelam e precisam de um clique em Play ou Priorizar. É a troca certa — congelar é visível na tela e custa um clique; retomar sozinho é invisível e desfaz uma decisão do usuário gastando banda. Acontece uma vez na vida da instalação, e o boot loga quantos hashes foram semeados.

**(i) O lote ignora o que o individual rejeita.** `POST /torrents/prioritize` aplica **na ordem recebida** (quem decide é o frontend, que sabe o que o usuário clicou) e ignora hash desconhecido ou já completo: uma lista de 12 episódios não pode falhar inteira porque um deles terminou de baixar entre o render e o clique. `POST /torrents/{hash}/prioritize` mantém os erros de sempre — a validação vive em `SessionManager.Prioritize(hash)`, que checa **antes** de delegar para `PrioritizeAll`. Não é o mesmo que N chamadas ao endpoint individual: aquelas se atropelariam e inverteriam o lote. A mensagem de resultado no frontend também foge do `{success} of {total}` das outras ações em lote — é **uma** requisição, então não existe sucesso parcial para contar, e o denominador nem é verificável do lado do cliente.

**Don't "fix" by:** iterar a sessão em vez de `order` no passo 4 (pausa **todo seeder** — ver (c)); chamar `m.Get`/`m.List` de dentro de `PrioritizeAll` (passam por `markQueued`, que pega `queue.mu`: **deadlock**, o mesmo que a linha abaixo descreve para `enforce`); fazer `enforce` ler via `SessionManager.List()` em vez de `list()` (idem — foi assim que a primeira versão travou `TestSessionManagerDetectsRootSwap`); fazer a fila chamar `Pause`/`Resume` públicos em vez de `pause`/`resume` (aí sim recursão infinita); atualizar `lastSaved` sem o `Rename` ter dado certo (uma falha transitória de disco congelaria o arquivo na versão velha para sempre, porque o "só grava quando muda" nunca mais dispararia); inverter a ordem `paused`-antes-do-`pause` em `SessionManager.Pause` (um `enforce` disparado por uma conclusão promoveria justamente o torrent que o usuário está parando); tratar `all == nil` como sessão vazia no passo 0 (apaga a fila persistida em todo boot, silenciosamente); tirar `queued` de `isProblemTorrent` (0 peers é a definição de estar enfileirado, e a pill de problemas ficaria vermelha toda vez que o limite funcionasse); deixar torrent completo contar contra o limite (seeding não custa banda de download e parar de semear é o oposto do que se quer); pôr a posição da fila dentro de `statusLabel()` (a mesma função monta o filtro de status da toolbar, onde "Na fila #7" não significa nada); reordenar ou renumerar as linhas por grupo para os números ficarem contíguos (é justamente a não-contiguidade que comunica que a fila é global).

---

### 42. Todo estado persistido do `FileManager` grava com temp+rename sob `m.mu`, e `fetchAniListEntries` devolve `nil` de propósito

**Location:** `files/filemanager.go` (`writeAtomic`, `loadSavedEpisodesLocked`, `saveEpisodesLocked`, `loadBlockedEpisodesLocked`, `saveBlockedEpisodesLocked`), `files/parser.go` (`ParseEpisodes`), `api/endpoint_animes.go` (`handleAnimes`, `fetchAniListEntries`, `refreshOrphanAnimes`).

**What it looks like:** dois pares de convenções que parecem cerimônia e são cicatriz de um incidente real (07/08/2026): soltar 5 episódios de uma vez pela UI matou a API por 3 minutos e depois prendeu o daemon numa tempestade de 429 na AniList.

**(a) Os métodos públicos de episódios/bloqueados pegam `m.mu` e delegam para um `...Locked`.** Todos fazem read-modify-write do arquivo inteiro, e a UI dispara vários em paralelo — um `POST /release` por episódio, ao mesmo tempo que o daemon chama `UpsertEpisodes` ao organizar torrents. Sem o lock as atualizações se perdem (todas as goroutines leem o mesmo estado inicial e só a última escrita sobrevive) e, pior, dois `WriteFile` truncando o mesmo caminho ao mesmo tempo deixam o arquivo intercalado. Foi assim que `episodes` corrompeu: `LoadSavedEpisodes` passou a falhar, e como `/api/v1/animes` e `/api/v1/animes/{id}/episodes` tratam esse erro como fatal, **os dois passaram a devolver 500 até o restart**. `Config` e `AnimeSettings` já faziam isso certo; episódios e bloqueados eram o buraco. O mutex não é reentrante — daí os `...Locked`.

**(b) `writeAtomic` (temp+rename) para os quatro arquivos, não só o config.** Todo estado aqui é reescrito por inteiro a cada alteração, então `WriteFile` direto deixa uma janela em que o arquivo está pela metade — um leitor concorrente enxerga truncado, e uma queda de energia no meio (Raspberry Pi, cartão SD) deixa truncado para sempre. O `rename` é atômico no mesmo filesystem, então leitor nenhum vê estado intermediário.

**(c) `ParseEpisodes` reportava os DOIS erros** (o formato de texto legado foi removido em #52, então hoje só existe o erro do JSONL). Para um arquivo já migrado o erro do formato antigo é ruído (ele só reclama que a linha é JSON), e reportar só ele escondia a linha JSONL realmente quebrada. A mensagem que produzia — `invalid episode ID '{"episode_id"'` — apontava para a linha 1 de um arquivo cuja linha 1 estava íntegra, e foi o que mais atrasou o diagnóstico do incidente.

**(d) `fetchAniListEntries` devolve `nil` na falha e slice vazio (não-`nil`) no sucesso — e `handleAnimes` depende disso.** `refreshOrphanAnimes` busca na AniList os animes que ficaram fora do conjunto `covered` (hoje em lote, `GetAnimeInfoByIDs`; até 29/ago/2026, um `GetAnimeInfo` individual por anime). Quando a busca da lista falhava, `covered` ficava vazio e **todo anime com episódio baixado virava órfão**: 16 requests por poll de `/api/v1/animes`, ~10 polls por minuto, com o frontend aberto em duas abas. Isso multiplicava um 500 passageiro da AniList em ~200 requests/min contra um limite de 90 (hoje degradado para 30) — e o 429 resultante fazia a busca da lista falhar de novo, fechando o ciclo: o daemon não saía sozinho. Por isso, com qualquer conta falhando, o refresh de órfãos é **pulado inteiro**: sem a lista não dá para saber o que está coberto, e tratar "não coberto" como "precisa refresh" é justamente o amplificador. **O lote não revoga esta regra**: ele derruba o custo da rajada, mas "cobertura desconhecida" continua sendo o estado em que o refresh não tem o que decidir — e o `mediaId_in` de uma biblioteca inteira ainda é `ceil(N/50)` requisições por conta a cada poll. Os animes continuam visíveis com os dados locais — degradar campo desatualizado é barato, derrubar a AniList não.

**Don't "fix" by:** chamar o método público de dentro de um `...Locked` (deadlock — `sync.Mutex` não é reentrante); "simplificar" `writeAtomic` de volta para `WriteFile` porque "o lock já resolve" (o lock só protege este processo, não protege queda de energia no meio da escrita); fazer `fetchAniListEntries` devolver slice vazio na falha "porque nil é slice vazio em Go" (apaga a distinção entre "nenhum coberto" e "cobertura desconhecida" e ressuscita a tempestade); tratar o skip do refresh de órfãos como perda de funcionalidade e "só limitar a concorrência" (foi o que o extinto `maxConcurrentOrphanRefresh` fazia, e não ajudava — limitar paralelismo não reduz o total de requests por poll; quem reduziu foi o lote).

---

### 43. A identidade de um anime é `Media.Id`; status é uma pergunta POR CONTA (download = OR, deleção = AND)

**Location:** `anilist/anilist.go` (`DedupeByMedia`, `GetAnimeInfo`, `GetMediaListStatus`, `GetMediaIDForEntry`), `daemon/verification.go` (`searchAnilist`, `deletableMediaIDs`, `allAccountsAgreeOnDelete`), `daemon/migration.go` (`MigrateAnimeIDsToMedia`), `api/endpoint_animes.go`.

**What it looks like:** três coisas que parecem exageradas — `GetAnimeInfo` recebe a lista de usernames e faz uma requisição por conta para buscar UM anime; `DedupeByMedia` avisa no comentário que ninguém pode ler o `Status` do resultado; e `allAccountsAgreeOnDelete` dispara uma consulta extra por conta que não reportou o anime.

**Why it's right:** `MediaList.Id` é o id da **entrada**, que é por conta — o mesmo anime em duas contas chega como duas entradas com ids diferentes e o mesmo `Media.Id`. Enquanto o `AnimeID` gravado era o id da entrada, ele apontava para **uma** das contas, e o `GET /animes` listava o anime duas vezes: uma sob a chave `id:<AnimeID>` vinda do disco e outra pelo nome, porque a entrada da outra conta não batia com nada conhecido (bug real: "From Old Country Bumpkin to Master Swordsman II", entradas 488911345 e 583631757, media 194829). Pior que a duplicata: qual entrada "vence" dependia do progresso, então o id gravado em disco **mudava** conforme o usuário assistia, e a deleção por status, as settings por anime e a tela de detalhe passavam a procurar por um id que metade dos registros não tinha.

Com `Media.Id` como identidade isso some por construção. O que sobra é o que a identidade única *não* resolve: cada conta tem seu próprio `status` e `progress` para a mesma mídia, e colapsar os dois numa entrada só significa escolher. As escolhas são deliberadamente assimétricas, porque os erros não custam o mesmo:

- **`progress`: o MENOR entre as contas** (`DedupeByMedia`). Um episódio só é "assistido" quando todas as contas o viram; errar para o lado alto apaga episódio que alguém ainda não assistiu.
- **Download: OR** — basta UMA conta ter o anime em status de download. Já sai de graça da união das listas em `searchAnilist`, já que cada busca é filtrada por `status_in` no servidor.
- **Deleção: AND** — TODAS as contas que têm o anime precisam tê-lo em algum status de deleção, e não precisa ser o mesmo (`DROPPED` numa e `COMPLETED` noutra apaga). Uma conta que **não acompanha** o anime não vota; uma que o tem em status neutro (`PLANNING`) **veta**. Distinguir esses dois casos é exatamente o que a consulta extra de `allAccountsAgreeOnDelete` compra: a busca por lista só traz os deletáveis, então "não veio na lista" é ambíguo. Ela só roda para anime **com episódio em disco** que alguma conta quer apagar — um conjunto que se esvazia sozinho, porque o anime some do disco no mesmo passe.

Consequência direta: **ninguém pode ler o `Status` da entrada vencedora do dedup** — ele é de uma conta arbitrária. Quem responde sobre status é `deletableMediaIDs`.

**Migração:** `MigrateAnimeIDsToMedia` resolve cada `anime_id` antigo via `GetMediaIDForEntry` e reescreve `downloaded_episodes` e `anime_settings`, marcando `anime_ids_are_media_ids` no config. Roda no boot **e** no topo do passe de verificação, e o passe **aborta** enquanto ela não tiver rodado: com os ids no formato antigo nada em disco casa com a AniList, e um passe nesse estado rebaixaria a biblioteca inteira. Só escreve depois de resolver tudo, então uma falha de rede no meio não deixa metade convertida. Entradas que já não existem na AniList (404) ficam com o id antigo e são apenas logadas — os episódios continuam em disco.

**Don't "fix" by:** deduplicar por nome (dois animes distintos podem compartilhar título em inglês, e o nome de um registro vindo só do `episodes.json` não vem da AniList); tirar o `media { id }` de `GetFrontendAnimeList`/`GetAllCurrentAnime` por "complexidade" (é um escalar, e sem ele a chave do dedup é `0` — todas as entradas caem no ramo "sem media id" e o bug volta inteiro, silenciosamente); ramificar em `MediaList.Status` depois do dedup (é de uma conta sorteada); trocar o veto por "qualquer conta em status de deleção apaga" (basta uma conta completar um anime para os episódios sumirem debaixo de quem ainda assiste); tratar conta com busca falhada como concordância (transformaria uma falha passageira da AniList em deleção de arquivo, que é irreversível); rodar a migração sem a marca de conclusão (na segunda vez ela leria ids de mídia como ids de entrada).

---

### 44. O `tvshow.nfo` é escrito DEPOIS dos hardlinks, nunca sobrescreve, e falhar nele não falha o organize

**Location:** `internal/files/librarian.go` (`writeShowNFO`, chamado no fim de `Organize`, e `BackfillShowNFOs`); `internal/daemon/jobs.go` (`organizeTorrent` preenche `OrganizeRequest.AnimeID`); `cmd/daemon/main.go` (chamada do backfill no boot).

**What it looks like:** o nfo parece que deveria ser escrito logo depois do `MkdirAll` (é metadado da pasta, não dos arquivos), e um erro ao escrevê-lo parece um erro do `Organize`.

**Why it's right:** três coisas, todas por causa de invariantes que já existiam:

- **Depois dos links:** o rollback de `Organize` é `cleanupIfEmpty`, que usa `os.Remove` e só funciona em pasta **vazia**. Um nfo escrito antes de um hardlink que falha (cross-device, permissão) deixaria a pasta não-vazia e um diretório órfão na biblioteca a cada retry.
- **Não sobrescreve:** se o usuário corrigiu o match à mão no Jellyfin (que reescreve o nfo), o próximo episódio do mesmo anime não pode desfazer isso. `Organize` roda uma vez por torrent, então isso acontece o tempo todo.
- **Só loga em falha:** os hardlinks são o produto; devolver erro aqui faria o job repetir (backoff, e depois dropar) um trabalho que já deu certo, e o `LibraryPaths` nunca seria gravado.

O `AnimeID` é o id de **mídia** da AniList (decisions.md #43) — vem de `EpisodeStruct.AnimeID` já migrado. `AnimeID == 0` (registro antigo não resolvível) simplesmente pula o nfo: o Jellyfin volta a adivinhar pelo nome, que é o comportamento anterior.

**O backfill existe porque `Organize` nao re-roda:** `organizeTorrent` sai cedo quando os episodios do hash ja tem `LibraryPaths`, e `reconcileLibrary` so enfileira quem tem `LibraryPaths` vazio. Sem `BackfillShowNFOs` um anime ja terminado nunca ganharia o arquivo (um em andamento ganharia no proximo episodio). Ele roda **todo boot** e nao tem flag de "ja rodou": o "nao sobrescreve" ja o torna idempotente, e uma flag so criaria um estado a mais para dessincronizar. A pasta vem de `LibraryPaths`, nao de `sanitizeName(AnimeName)` — e o unico caminho que com certeza casa com o que existe em disco depois de renomeacoes manuais ou mudancas na sanitizacao.

**Ele e gated pela migracao de ids** (`main.go`, ramo `else` de `MigrateAnimeIDsToMedia`): com os ids no formato antigo o nfo sairia com um id de ENTRADA, e como nunca e reescrito isso seria permanente. Roda no `else` de propósito — falha de migracao apenas adia o backfill para o proximo boot.

**Requer no Jellyfin:** o plugin `jellyfin-plugin-anilist` como provedor de metadados; sem ele o `uniqueid type="anilist"` é ignorado.

**Don't "fix" by:** mover a escrita para junto do `MkdirAll`; sobrescrever sempre "para manter sincronizado"; propagar o erro de escrita para o retorno de `Organize`; dar uma flag de config ao backfill; derivar a pasta do backfill do nome do anime; rodar o backfill antes/independente da migracao de ids.

---

### 45. Uma pasta de biblioteca por ENTRADA da AniList — o marcador de season fica no nome

**Location:** `internal/files/librarian.go` — `sanitizeName` (era `sanitizeFolderName` + `sanitizeFileName`, agora uma so) e o `destDir` em `Organize`; teste `TestSanitizeNameKeepsSeasonMarker`.

**What it looks like:** o layout "certo" pro Jellyfin/TVDB e uma pasta por serie com `Season 03/` dentro, e ate `cd496c3` o codigo ia meio caminho nessa direcao: um `seasonPattern` apagava "Season 2"/"S2"/"2nd Season"/"Cour 2" do nome da pasta, juntando todas as seasons numa pasta so.

**Why it's right:** na AniList **nao existe** serie com varias seasons. Cada season e uma midia propria, com id, capa, sinopse e numeracao de episodio comecando em 1 — e os nossos dados sao exatamente assim (um `AnimeID` por season, `EpisodeNumber` reiniciando). Juntar as seasons numa pasta so obrigava o Jellyfin a escolher UMA entrada pra pasta inteira, e ele escolhia a season 1: toda season nova aparecia com a capa e os metadados da primeira (relato real: Mushoku Tensei — as entradas 108465, 127720, 146065 e 178789 caiam todas em `Mushoku Tensei Jobless Reincarnation`). O agrupamento nem era coerente: o regex comia "Season 2" mas nao "Part", entao "Season 2 Part 2" ia parar numa quinta pasta.

Com o `tvshow.nfo` (decisions.md #44) isso deixaria de ser um palpite e viraria erro fixo: a primeira season a chegar na pasta gravaria o `uniqueid` dela e, como o nfo nunca e sobrescrito, prenderia a pasta naquela season para sempre.

O layout de franquia com `Season NN/` tambem nao tem como funcionar aqui: o `tvshow.nfo` da franquia precisaria de um id que a AniList nao tem, e o `jellyfin-plugin-anilist` nao fornece metadado por season. Uma pasta por entrada e ao mesmo tempo o diff menor e o unico modelo que casa com o provider.

Como as duas funcoes de sanitizacao ficaram identicas depois de tirar o regex, viraram uma: `sanitizeName`.

**Don't "fix" by:** reintroduzir o strip de season "pra agrupar a franquia"; criar subpastas `Season NN/` (a numeracao de episodio da AniList e por entrada, entao E01 da season 3 e mesmo o episodio 1 da midia dela, nao o 25 da franquia); deduplicar pastas por titulo-base.

---

### 46. `GetFrontendAnimeList` é cacheado por 60s — o poll de `/api/v1/animes` é o que estoura a AniList

**Location:** `internal/anilist/anilist.go` — `ttlCache`, `frontendListCache`, `GetFrontendAnimeList`, `frontendListResponse`, `clearCaches` (chamado por `MockAniListDo`); testes em `anilist_test.go`.

**What it looks like:** um cache com TTL num getter que "só lê uma lista", devolvendo uma cópia da fatia em vez da fatia guardada. Parece otimização prematura em cima de um request que leva ~1s.

**Why it's right:** a AniList hoje limita a **30 req/min** por IP. `Status.svelte` faz poll de `/api/v1/animes` a cada 30s **por aba aberta**, e o handler faz uma busca por conta configurada (`fetchAniListEntries` = `GetCustomListsMap` + `GetFrontendAnimeList`). Com 2 contas e 2 abas isso sozinho são ~16 req/min só de UI parada — some com um F5, com o ciclo de verificação de 10 em 10 minutos e com `GetAnimeInfo` de órfãos e o 429 é garantido. Diagnóstico real (`daemon.log`, 10/08/2026 11:09): 15 requests a `/api/v1/animes` em 2 minutos, seguidos de 429 em cascata que derrubaram junto o `searchAnilist` do daemon (`verification.go:486`) — ou seja, a tela aberta impedia o download automático de rodar. Com o cache, N abas custam no máximo 1 request por minuto por conta.

O TTL de 60s é seguro porque essa lista só muda quando o usuário mexe na AniList; contagem de episódios baixados vem do `episodes.json` local e continua instantânea. A chave inclui username + statuses, então mudança de config gera chave nova (não precisa invalidação).

**Cópia, não a fatia guardada:** `fetchAniListEntries` sobrescreve `ml.CustomLists` nas entradas que recebe. Devolver a fatia do cache faria dois requests concorrentes de `/api/v1/animes` escreverem na mesma memória (corrida real, pega no `-race`) e vazaria o overlay de um chamador pro próximo.

**Don't "fix" by:**
- Cachear `GetAllCurrentAnime` junto — esse é o ciclo do daemon, que roda de 10 em 10 min e **precisa** de dado fresco pra decidir download/deleção.
- Aumentar o intervalo de poll do frontend em vez do cache — não resolve F5, múltiplas abas, nem múltiplos dispositivos na LAN.
- Devolver a fatia do cache direto "pra economizar a cópia".
- Cachear também o refresh de órfãos (`GetAnimeInfoByIDs`): aquele caminho já está desligado quando a lista falha (decisions.md #42d), que é quando ele amplifica — e desde que virou lote (#65) ele custa 1 requisição por conta por poll, a mesma ordem da busca de lista ao lado, que é cacheada.

---

### 47. Notificações são agrupadas por janela, e o body é escapado só quando o preset é JSON

**Location:** `internal/notifications/notifications.go` — `Notify`, `Flush`, `flushEvent`, `fireBatch`, `buildBatchVars`, `jsonEscape`/`escapeVarsForJSON`/`presetIsJSON`; `NotificationsConfig.BatchWindowSeconds` em `files/filemanager.go`; `notifications.Flush()` no shutdown de `cmd/daemon/main.go`.

**What it looks like:** três coisas que parecem opcionais num disparador de webhook: uma fila com timer, um `Flush()` síncrono no shutdown, e escape de JSON condicionado a um header.

**Why it's right:**

**(a) A janela.** Um backfill de biblioteca dispara um `JobOrganize` por torrent, e cada um fecha com um `download_completed`. Caso real (`daemon.log`, 09/08/2026 17:49→18:22): 67 organizes, os 24 primeiros passaram e o ntfy.sh respondeu **429 nos 53 restantes** — a cota estourou e as notificações foram perdidas sem retry. Como o intervalo mediano entre organizes era de 25s, janela curta não resolve: medido no próprio log, 10s levaria 67 → 53, e 60s leva 67 → 24. Por isso o default é 60s e o campo é configurável.

A fila é **por evento**, não global: um preset inscrito só em `download_failed` não pode receber texto de sucesso, e o filtro por evento que já existia (`slices.Contains(preset.Events, ...)`) continua valendo.

**(b) `len(items) == 1` sai idêntico ao não-agrupado.** `buildVars` virou um wrapper de `buildBatchVars` com um item. É isso que faz ligar o agrupamento não mudar a aparência das notificações de quem recebe uma de cada vez — e é o que os testes `TestNotify_SingleItemBatchMatchesUnbatched` travam.

**(c) `Flush()` é síncrono.** `fireBatch` normalmente dispara em goroutine, mas no shutdown isso é o mesmo que não disparar: o processo sai antes das goroutines rodarem. Daí o parâmetro `wait` e o `wg.Wait()`.

**(d) O escape de JSON depende do `Content-Type`.** Dos 9 presets embutidos, 6 embutem `{{message}}` dentro de uma string JSON (Discord, Slack, Telegram, Gotify, Pushover, Apprise) e 1 manda o body cru (ntfy: `body: '{{message}}'`; os presets de scan Jellyfin/Plex têm body vazio, que passa direto). O `\n` do agrupamento é JSON inválido dentro de string — e escapar o body do ntfy mandaria a barra-n literal em vez da quebra de linha. Então o escape é aplicado **só no body, e só quando o preset se declara `Content-Type: application/json`**. De brinde isso conserta um bug latente que existia antes do agrupamento: anime com aspas ou barra invertida no nome já quebrava o body desses 6 presets, e o serviço respondia 400 com a notificação sumindo (o log só diz "Webhook returned error status").

A URL fica fora porque escape de URL tem regras próprias, e os headers passam por um strip de `\n`/`\r`: um header com quebra de linha faz o `net/http` **recusar a request inteira**, e o preset do ntfy usa `Title: {{title}}`.

**Don't "fix" by:**
- Trocar a fila por evento por uma fila global "pra mandar menos mensagem ainda" — quebra o filtro `Events` do preset.
- Mandar `{{anime_name}}`/`{{episode}}` do primeiro item quando N > 1: o template passa a mentir sobre os outros N-1.
- Escapar sempre, ou escapar a URL/headers junto: o body de texto puro do ntfy recebe barra-n literal.
- Tirar o `wait` do `Flush` "porque goroutine é mais rápido".
- Adicionar retry com backoff no 429 esperando resolver o caso do log: era cota de serviço, não falha transitória. O que resolve é mandar menos mensagem.

---

### 48. `max_episodes_per_anime` nunca limitou batch — a regra agora vive na janela de packs, não num teto de contagem

**Ver também:** [`2026-08-13-batch-por-filtro-design.md`](../superpowers/specs/2026-08-13-batch-por-filtro-design.md) — a spec que substituiu o teto de contagem (`max_batch_episodes`) descrito abaixo por elegibilidade decidida pelo resultado da busca. As entradas 59-62 logo após esta documentam o desenho atual em detalhe.

**Location (histórico):** quando esta decisão foi escrita, `daemon/episodes.go` tinha `willBatchAnime` e `resolveSearchStrategy`; ambos foram removidos pela spec de batch-por-filtro. O que existe hoje: `daemon/episodes.go` — `selectEpisodes`, `effectiveMax`, `windowEnd`, `partitionSearchResults`, `pickBatches`, `packSet.covering`, `assignBatches`; `daemon/search.go` — `filterBySize`; `daemon/helpers.go` — `checkDiskSpace`/`ErrInsufficientDiskSpace`; `daemon/jobs.go` — `partiallyOrganized`; `api/endpoint_status.go` — `disk_low`.

**What it looks like:** um limite por anime que às vezes não se aplica, um filtro de tamanho que deixa passar tamanho zero, uma guarda de disco que só barra o `Add`, e um booleano de "disco baixo" calculado no servidor para o frontend.

**Why it's right:**

**(a) O limite se aplica só episódio a episódio.** Um batch é **um torrent**: um anime finalizado de 24 episódios com o limite em 12 baixava os 24 arquivos, o `organize` hardlinkava os 24, e `episodes.json` registrava 12 — a tela mostrava 12/24 para sempre com os 24 no disco. Limitar registros não limita bytes nem arquivos; só produz contagem mentirosa. Então, quando um pack cobre a janela pendente, o segundo `selectEpisodes` roda com o limite levantado (`len(episodes)+1`), e o limite efetivo passa a ser ilimitado para os episódios daquele pack.

O que decide elegibilidade a pack hoje **não é mais** um teto de contagem de episódios do anime (`max_batch_episodes`, removido) — é o próprio resultado da busca filtrada: `partitionSearchResults` separa packs de episódios soltos e aplica a cada um o SEU teto de tamanho (`max_batch_torrent_size_gb`/`max_episode_torrent_size_gb`) e o piso de seeders; `pickBatches` escolhe o mínimo de packs que cobre `[primeiroPendente, primeiroPendente + max_episodes_per_anime - 1]` (ou a janela toda, com o teto desligado). Ver entrada 59 para o porquê dessa troca.

**(b) O tampão.** Se a busca por pack não cobre a janela pendente (nenhum resultado, ou o filtro de tamanho a esvazia), a seleção original — com o limite por anime valendo — permanece: `episodesToDownload` nunca foi lançado, então não há nada a cortar de volta. A ordem do `airingSchedule` garante que sobram os mais antigos.

**(c) `Size == 0` passa o filtro de tamanho.** É o valor de `nyaa.parseSize` quando o Nyaa muda o formato da coluna. Descartar tamanho desconhecido trocaria "às vezes baixa um torrent grande" por "não baixa nada": um bug de parsing viraria paralisação silenciosa. O filtro roda **depois** da ordenação por prioridade e preserva a ordem, então o escolhido é o melhor **entre os que cabem**.

**(d) A guarda de disco barra o `Add`, não o passe.** A poda de assistidos, o `deleteEpisodesByStatus` e o `organize` são justamente o que **libera** espaço; um `if disco cheio { return }` no início do passe deixaria o app travado no estado em que não consegue se desentupir. Erro de `statfs` não bloqueia — um volume que não responde (rede, permissão) não é prova de disco cheio. Quando barra, não tenta nenhum magnet e não faz retry (o magnet não é o problema) e notifica `ReasonNoDiskSpace`, que o batch de notificações agrupa.

**(e) O limiar de disco vive no servidor.** `GET /status` devolve `disk_low`, calculado com `min_free_disk_percent`. Um único lugar decide o que é "baixo", e o aviso da tela não pode discordar do que o daemon está fazendo.

**(f) `organizeTorrent` não renotifica um grupo parcialmente organizado.** Se parte do grupo já tem `LibraryPaths`, o torrent já pousou e o webhook já saiu: organiza (é idempotente), grava o marcador nos registros novos, não notifica. Sem isso, o primeiro passe pós-upgrade de uma biblioteca existente — onde a regra (a) cria os registros que faltavam para packs já no disco — viraria enxurrada de `download_completed` duplicado.

**Don't "fix" by:**
- Aplicar `max_episodes_per_anime` em batch "para o limite valer sempre": volta a contagem mentirosa.
- Reintroduzir um teto de contagem de episódios do anime (tipo `max_batch_episodes`) para decidir elegibilidade a pack: contagem de episódios é proxy ruim de tamanho — ver decisions.md #59.
- Descartar torrent com `Size == 0` "porque tamanho desconhecido é suspeito".
- Consolidar a guarda de disco num `return` no topo do passe de verificação (ver (d)).
- Mover a guarda para `torrents.Session.Add` "porque é um site só": o pacote `torrents` não conhece `files.Config`, e passar a config para lá inverte a dependência.
- Reintroduzir um ratio hardcoded em `lib/utils/status.ts` para economizar um campo na resposta (ver (e)).

### 59. Elegibilidade a batch deixou de ser metadado e virou filtro de resultado

**Location:** `daemon/episodes.go` — `partitionSearchResults`, `pickBatches`, `packSet.covering`; `nyaa/nyaa.go` — `extractBatchInfo`/`ExtractBatchInfo`, `ScrapNyaaForAnime`.

**What it looks like:** nenhuma checagem de `FINISHED`, nenhuma checagem de contagem de episódios do `Media` antes de decidir se um anime "vira pack". A decisão inteira mora do lado de dentro do resultado da busca já filtrado.

**Why it's right:** o que importa para elegibilidade a pack é o torrent que está lá — tamanho, seeders, faixa coberta — e nada disso é conhecido **antes** da busca. Um teto de contagem de episódios (como o antigo `max_batch_episodes`) é proxy ruim de tamanho: medido, 26 episódios em remux passa de 300 GB (furaria qualquer teto de contagem razoável) e 500 episódios em 480p fica em ~80 GB (um teto de contagem baixo bloquearia um pack que caberia tranquilamente). E o gate de `FINISHED` excluía exatamente quem mais depende de pack — uma série longa em `RELEASING` (One Piece, Naruto) nunca teve outro jeito de não acumular 1000+ registros episódio-a-episódio.

Hoje quem decide é `partitionSearchResults` (separa packs de episódios soltos e aplica a CADA lista o seu próprio teto de tamanho — `max_batch_torrent_size_gb`/`max_episode_torrent_size_gb` — e o piso de seeders) seguido de `pickBatches` (escolhe o mínimo de packs, entre os que sobraram do filtro, que cobre a janela pendente).

**Ceiling conhecido (ponytail, guarda de faixa fantasma):** `ExtractBatchInfo`/`extractBatchInfo` guardava só o caso dominante de faixa fantasma — um marcador de resolução (`[720-1080p]`) sendo lido como episódios 720-1080 —, apostando que outras faixas fantasma (data, bitrate) cairiam em `EndEpisode == 0`, o fallback seguro. **RESOLVIDO** (ver #83): a aposta estava errada para o ano. `(2021-2022)` não cai em zero, vira a faixa 2021..2022, e um pack completo top-seeded era descartado em silêncio por não cobrir episódio nenhum.

**Ceiling conhecido (ponytail, piso de paginação):** `ScrapNyaaForAnime` fundiu pack e episódio solto numa busca só, e o piso de paginação (`enoughCandidates`, decisions.md #57) passou a contar as DUAS listas somadas. Uma página 1 com 3 packs que o filtro de tamanho descarta depois encerra a descida sem ter juntado episódio solto nenhum — antes, com duas buscas separadas, a segunda desceria por conta própria. **RESOLVIDO** (ver #80): apareceu medido, em One Piece. O teto de pack passou a ser empurrado para o pacote `nyaa` por `applyNyaaSettings` — o mesmo mecanismo de push de `SetMaxSearchPages`, e não leitura direta de `files.Config` —, e pack acima do teto sai antes de contar para o piso. O filtro de seeders continua só no daemon: o Nyaa devolve ordenado por seeders desc, então ele não trunca a descida do mesmo jeito.

**Don't "fix" by:**
- Reintroduzir um teto de contagem de episódios do anime: contagem é proxy ruim de tamanho (ver acima), e o gate de `FINISHED` excluía quem mais precisa de pack.
- Aceitar `EndEpisode == 0` como "faixa 0-0" em vez de "pack completo": um pack real cuja faixa não parseou desapareceria da seleção.

### 60. A janela de packs cobre a partir do primeiro pendente, e o corte é sempre de prefixo

**Location:** `daemon/episodes.go` — `windowEnd`, `pickBatches`, `assignBatches`.

**What it looks like:** `pickBatches(packs, firstPending, windowEnd)` anda um cursor a partir do primeiro episódio pendente, e `assignBatches` para de atribuir magnet no primeiro episódio da lista que nenhum pack escolhido cobre — em vez de dar a todo mundo o magnet do primeiro pack encontrado, ou tentar episódio solto para o resto.

**Why it's right:** sem a janela ancorada no primeiro pendente, `batchResult[0]` iria para todo episódio pendente e o episódio 800 de uma série longa receberia o magnet do pack 1-100 — dado errado, não "sem magnet". Sem o truncamento de prefixo em `assignBatches`, os episódios fora da cobertura cairiam no fallback de episódio solto (`magnetsByEpisode`), que em One Piece dispararia `DownloadFailed`/`ReasonNotFound` para ~1000 episódios **por passada** — nenhum deles vai aparecer solto no Nyaa, então é notificação de erro pura, sem chance de sucesso. O corte é sempre de **prefixo** (nunca "buraco no meio") porque a agenda (`anilist.EpisodeList`) vem em ordem crescente e os packs escolhidos por `pickBatches` são contíguos a partir do cursor — não há como um pack posterior cobrir um episódio anterior sem que `packSet.covering` já o tivesse escolhido primeiro.

**Don't "fix" by:**
- Aplicar o magnet do primeiro pack achado a todo episódio pendente "porque é um episódio de qualquer forma" — episódio fora da faixa do pack simplesmente não está no `.torrent`.
- Cair em episódio solto para o que sobra fora da janela: em série longa isso é notificação de erro garantida, não uma tentativa a mais.

### 61. Episódio de pack é imune à poda por limite E ao keep-set de assistidos

**Location:** `daemon/episodes.go` — `handleAlreadySavedEpisode` (guarda de deleção), `buildWatchedKeepSet` (guarda de keep-set).

**What it looks like:** `handleAlreadySavedEpisode` recusa deletar um episódio salvo com `IsBatch: true` mesmo quando ele está acima do limite por anime; `buildWatchedKeepSet` nunca inclui um episódio de pack na lista de candidatos a "manter mesmo assistido".

**Why it's right:** um pack é um torrent só — apagar o registro de UM dos seus episódios não libera byte nenhum, o torrent inteiro continua seedando no disco. Sem a guarda em `handleAlreadySavedEpisode`, o ciclo seguinte ao que trouxe o pack apagaria os registros acima do limite, o gate de espaço reabriria (o registro sumiu, mas o byte não), e o próximo ciclo tentaria baixar de novo — loop de baixar-e-apagar sobre um torrent que nunca saiu do disco. Sem a guarda em `buildWatchedKeepSet`: `watched_episodes_to_keep` não tem granularidade **dentro** de um pack — manter 3 dos 100 episódios de um pack não guarda 3 episódios, guarda o pack inteiro (`canRemoveTorrent` dá `false` para o grupo inteiro enquanto QUALQUER um dos 100 registros estiver fora do delete-set), então incluir episódios de pack no keep-set não muda o resultado, só confunde a contagem.

O contador (`downloadedEpisodes`) continua incrementando para episódios de pack **abaixo** do limite — só a deleção acima do limite é que é ignorada. É esse incremento normal, mais a imunidade à poda, que dá o **rodízio de packs sucessivos sem nenhuma config nova**: o pack atual acumula registros além do limite (imunes), o próximo ciclo busca de novo, `pickBatches` escolhe o próximo pack ainda não coberto, e assim por diante — só freado por `max_batch_torrent_size_gb` (por torrent) e `checkDiskSpace`.

**Don't "fix" by:**
- Deletar registros de pack acima do limite "para o limite valer sempre": o torrent continua no disco, e o próximo ciclo baixa de novo (loop de baixar-e-apagar).
- Dar granularidade ao keep-set dentro de um pack "para respeitar `watched_episodes_to_keep` à risca": a unidade real é o torrent, não o registro — não existe "manter 3 episódios" de um arquivo só.

### 62. Progresso de avulso mora em `AnimeSettings` e é injetado no `MediaList` sintético, não vira caminho paralelo

**Location:** `files/filemanager.go` — `AnimeSettings.Progress`; `api/endpoint_anime_settings.go` — `handleAnimeSettings` (`PUT` parcial); `daemon/standalone.go` / `api/standalone.go` — `withStandaloneProgress` (duas cópias, um por pacote, mesmo contrato); `daemon/manual_download.go` — `resolveAnimeDetails`.

**What it looks like:** `AnimeSettings` (por anime, arquivo separado de `Config`) ganha um campo `Progress int`; toda vez que um `MediaList` sintético de avulso é montado (`appendStandaloneAnimes`, `DownloadStandaloneAnime`, `resolveAnimeDetails` quando `GetAnimeInfo` devolve `nil`), `withStandaloneProgress` sobrescreve `ml.Progress` com o valor salvo antes de o `MediaList` ser usado por qualquer coisa.

**Why it's right:** com o dado morando no mesmo campo que uma entrada de lista real usa, `shouldSkipEpisode` (pula episódio `<= Progress`), `firstEpisodeToConsider` (começa em `Progress + 1`), `buildWatchedKeepSet` (usa `Progress` para achar quais salvos contam como assistidos) e a contagem `EpisodesWatched` da UI funcionam em avulso **sem nenhum `if isStandalone`** — é a mesma leitura de sempre, só que a fonte da verdade para avulso é `AnimeSettings` em vez da AniList. Um caminho paralelo (branch explícito em cada um desses pontos) duplicaria a lógica de "o que é assistido" em quatro lugares e um deles ia divergir cedo ou tarde.

`resolveAnimeDetails` chama `withStandaloneProgress` no MESMO branch de fallback que `appendStandaloneAnimes` usa (`GetAnimeInfo` devolve `nil` → `GetMediaByID`) — sem isso, `RunAnimeDebug` (que passa por `resolveAnimeDetails`) relatava um avulso com progresso gravado como se começasse do episódio 1, divergindo do pipeline real que ele existe para espelhar (decisions.md #58). Nos `ManualDownload*` o efeito é inócuo — eles selecionam episódio por número explícito, não por `Progress` — mas a chamada é a mesma para não depender de qual caminho um chamador futuro escolhe.

O `PUT /animes/{id}/settings` usa ponteiros (`*string`/`*int`) e faz merge parcial: um `PUT` que só manda `custom_search_query` não pode zerar `progress` (e vice-versa) — um `AnimeSettings{}` novo a cada `PUT` apagaria silenciosamente o outro campo. `progress < 0` é rejeitado com 400 na validação do handler, antes do merge.

**Don't "fix" by:**
- Um branch `if anime.IsStandalone` em `shouldSkipEpisode`/`firstEpisodeToConsider`/`buildWatchedKeepSet` "para deixar explícito": é exatamente a duplicação que injetar no `MediaList` evita.
- `PUT` substituindo o `AnimeSettings` inteiro em vez de merge parcial: zera o campo que o request não mencionou.
- Pular `withStandaloneProgress` em `resolveAnimeDetails` "porque `ManualDownload*` não usa `Progress`": `RunAnimeDebug` usa, e os dois passam pelo mesmo caminho.

### 49. Anime avulso: "acompanhado pela lista" é o snapshot que o daemon PROCESSA, e `DownloadStandaloneAnime` nunca chama `handleSavedEpisodes`

**Location:** `files/standalone.go`; `anilist/standalone.go` — `SearchMedia`/`GetMediaByID`; `daemon/standalone.go` — `appendStandaloneAnimes`/`DownloadStandaloneAnime`; `daemon/verification.go` — `searchAnilist`; `daemon/helpers.go` — `isConfigComplete`; `api/standalone_guard.go` — `blockReason`; `api/standalone.go` — `resolveMediaList`; `api/endpoint_config.go` — validação do `PUT`.

**What it looks like:** um arquivo `standalone_animes` com ids soltos; uma função de bloqueio que serve tanto o `POST` quanto o resultado da busca; um "baixar agora" que não faz nenhuma limpeza; e um anime avulso que nunca é apagado pela poda automática.

**Why it's right:**

**(a) O append acontece DEPOIS do `DedupeByMedia`.** `GetMediaByID` devolve um `MediaList` **sintético**: `Progress: 0`, `Status: ""`, `Id: 0`. `DedupeByMedia` mantém o **menor** progresso, então anexar antes do dedupe faria o zero sintético vencer a entrada real e o daemon rebaixaria episódios já assistidos. É ordem, não estilo.

**(b) "Acompanhado pela lista" = aparece no conjunto que o daemon processa,** ou seja as entradas que sobrevivem ao filtro de `download_statuses` + `DownloadMediaStatuses` — **não** "existe entrada na AniList". Um anime em `PLANNING` com `download_statuses = [CURRENT]` está numa lista e o daemon o ignora; adicioná-lo como avulso é o caso de uso mais óbvio da feature, não um erro a bloquear. A mesma definição alimenta os três consumidores (remoção automática, 409 do `POST`, `block_reason` da busca) porque duas definições diferentes produziriam um card cinza que o backend aceita, ou o inverso.

**(c) Uma função de bloqueio, dois consumidores.** `blockReason` decide o 409 e o `block_reason` de cada resultado da busca. "O front não deixa clicar" e "o back devolve erro" precisam concordar **por construção**, não por disciplina. Precedência `blacklist > standalone > tracked > downloaded`: blacklist vem primeiro porque é o único motivo em que adicionar mudaria o comportamento **para pior** — um blacklisted fora de `download_statuses` escapa do `searchAnilist`, o registro avulso sobrevive ao merge e o filtro do usuário é contornado (o `MediaList` sintético tem `CustomLists` nulo, então `animeIsInExcludedList` nunca dispara nele). `downloaded` só bloqueia com total **conhecido**: 12 registros de 24 episódios é o limite por anime, não "já baixado".

**(d) `DownloadStandaloneAnime` não pode chamar `handleSavedEpisodes`.** Ela chama `identifyEpisodesNotInWatching`, que compara **todos** os episódios salvos contra os `checkedEpisodes` recebidos e devolve os não cobertos para deleção. Com os episódios de um único anime na mão e `delete_watched_episodes` ligado, isso apagaria a biblioteca inteira. Pelo mesmo motivo não chama `deleteEpisodesByStatus` nem `DeleteEmptyFolders`: adicionar um anime não é ocasião para apagar nada.

**(e) Um avulso nunca é apagado por status.** `deletableMediaIDs` só avalia candidatos que apareceram na lista de alguma conta, e um avulso não aparece em nenhuma. Ele acumula no disco até o usuário deixar de acompanhá-lo, **ou** até marcar manualmente até onde assistiu — ver decisions.md #62 sobre `AnimeSettings.Progress`, que é a exceção deliberada: sem progresso manual `delete_watched_episodes`/`watched_episodes_to_keep` nunca tinham o que podar num avulso.

**(f) A remoção automática consome o registro, e isso pode chegar a apagar arquivo.** Um avulso que depois entra numa lista da AniList sai do arquivo no passe seguinte (a entrada real já vencia o dedupe, então o que o passe baixa não muda). Se **depois disso** o usuário tirar o anime da lista, o registro avulso já foi consumido e os episódios dele passam a cair em `identifyEpisodesNotInWatching` como os de qualquer anime tirado da lista. É a troca aceita: preservar os dois registros exigiria distinguir "avulso ativo" de "avulso dormente" no arquivo, no `AnimeInfo` e na tela, para cobrir um caminho que quem usa avulso não percorre. O `DELETE` explícito fecha a **outra** janela marcando os episódios `ManuallyManaged`.

**(g) Conta da AniList deixou de ser obrigatória.** `isConfigComplete`, `searchAnilist` e a validação de `PUT /config` exigiam pelo menos um `anilist_usernames`; com avulsos isso passou a ser uma exigência sem função — uma instalação que só usa avulsos nunca configura conta nenhuma, e o daemon a deixaria presa na tela de configuração para sempre. A **biblioteca** continua obrigatória nos três lugares: sem ela não há para onde baixar. `searchAnilist` sem conta não é erro — o passe ainda tem os avulsos a processar, e abortar ali faria a feature nunca rodar. A regra saiu também do `requiredChecks` do frontend: uma obrigatoriedade só na tela é uma regra que o servidor não conhece.

**(h) O conceito se chama `standalone` no código, "avulso" na UI, e nunca `manual`.** `manual` já significa outra coisa aqui — `ManuallyManaged`, `ManualDownloadEpisode` querem dizer "o usuário mexeu neste episódio à mão, o loop não deve tocar". Reaproveitar a palavra faria dois conceitos diferentes lerem igual.

**Don't "fix" by:**
- Anexar os avulsos antes do `DedupeByMedia` "porque o merge é um só" (ver (a)).
- Trocar `tracked` por "existe entrada na AniList" para evitar depender de `fetchAniListEntries` — isso quebra o caso `PLANNING`, que é o caso de uso da feature.
- Duplicar a regra de bloqueio no frontend para não precisar do campo `block_reason` na resposta da busca (ver (c)).
- Devolver quatro booleanos em vez de um `block_reason`: os motivos são mutuamente exclusivos por precedência e o card precisa de um rótulo só.
- Consolidar `DownloadStandaloneAnime` com `handleSavedEpisodes` "para reaproveitar o salvamento" (ver (d)).
- Filtrar o avulso por `DownloadMediaStatuses` no append: quem pediu o anime a mão quer acompanhá-lo também enquanto ele é `NOT_YET_RELEASED`.
- Reintroduzir `anilist_usernames` como campo obrigatório (em qualquer das quatro camadas) "porque o app é sobre a lista do AniList": ele deixou de ser (ver (g)).

### 50. Busca de avulso: o filtro de não lançados é server-side e por concatenação, e o card bloqueado vira link em vez de beco sem saída

**Location:** `anilist/standalone.go` — `SearchMedia`; `api/endpoint_anilist_search.go`; `frontend/src/routes/AddAnime.svelte`; `frontend/src/components/ui/Button.svelte`.

**What it looks like:** um `status_not: NOT_YET_RELEASED` **grudado na string** da query em vez de passado como variável GraphQL; um toggle que não sabe quantos resultados escondeu; e um `Button` que às vezes renderiza `<a>`.

**Why it's right:**

**(a) O filtro vive na query, não no Go.** `perPage` é 20. Filtrar depois de receber devolveria buscas com 4 resultados úteis em temporada de anúncios — o usuário leria isso como "a busca está quebrada", não como "escondi 16". O preço é que **nada sabe quantos foram escondidos**, então o toggle é cego: não existe "3 ocultos · mostrar". Foi a troca escolhida; inverter os dois é uma decisão de produto, não um bug.

**(b) Concatenação, e não uma variável GraphQL nula.** A AniList ignora argumentos com valor nulo hoje, e `status_not: $statusNot` com `$statusNot = null` funcionaria. Mas se essa semântica mudar, o filtro passa a esconder tudo ou nada **em silêncio** — o pior modo de falha para um filtro. Com a concatenação, `includeUnreleased = true` faz o argumento simplesmente não existir na query, o que é verdade em qualquer versão da API. Custa três linhas e o teste inspeciona o corpo enviado, não o resultado, justamente porque as duas implementações passariam num teste de resultado.

**(c) Só `NOT_YET_RELEASED` sai.** `CANCELLED` e `HIATUS` ficam: um anime interrompido no meio da exibição tem episódios no ar e baixáveis. "Ainda não lançou" é um status só.

**(d) `tracked`/`downloaded`/`standalone` viram link para `#/status/{id}`.** `anime_id` **é** o media id da AniList, então o href sai do próprio resultado da busca, sem lookup — e `AddStandaloneAnime` grava antes de responder, então o link funciona no instante seguinte ao clique em Adicionar, sem esperar ciclo do daemon. O motivo do bloqueio saiu do `tooltip` e virou uma linha do card: tooltip não existe no mobile, e a tela nasceu para a instalação sem conta, que é a que mais roda no celular. `blacklist` é a única exceção — não está no conjunto que o daemon processa, não tem página, continua apagado e desabilitado.

**(e) `Button` com `href` renderiza `<a>` em vez de um `on:click` que escreve `location.hash`.** Um botão que navega perde middle-click, "abrir em nova aba" e a URL na barra de status. A alternativa era copiar as classes de variante para dentro do card, o que duplicaria o estilo em cada tela que precisar disso. `disabled` é ignorado com `href` de propósito: não existe âncora desabilitada, e um link que não deve ser seguido não deve ser renderizado como link.

**Don't "fix" by:**
- Mover o filtro para o Go "para poder mostrar quantos foram escondidos": isso reintroduz a busca com 4 de 20 (ver (a)).
- Trocar a concatenação por `status_not: $var` com valor nulo "porque variável é mais limpo" (ver (b)).
- Estender o filtro para `CANCELLED`/`HIATUS` — esses têm episódios baixáveis (ver (c)).
- Devolver o `block_reason` para dentro de um `tooltip` "para o card ficar mais limpo": o motivo some no mobile inteiro.
- Fazer o card todo virar `<a>`: o botão Adicionar dentro da âncora exigiria `stopPropagation` e quebraria a navegação por teclado.

### 51. Episódio aceita 4 dígitos, exceto entre colchetes

**Location:** `nyaa/nyaa_regex.go` — `reEpisodePatterns`, `reBatchRange`, `reHasEpisode`, `reBatchPatterns`; `nyaa/nyaa.go` — `extractEpisodeNumber` (teto `< 10000`).

**What it looks like:** todos os padrões de episódio usam `\d{1,4}`, menos `\[(\d{1,3})\]`.

**Why it's right:** com `\d{1,3}` o One Piece (ep. 1123+) não casava em nenhum padrão, `extractEpisodeNumber` devolvia `nil` e **todo** resultado do Nyaa era descartado — o log mostrava 30 linhas boas e `results: 0`. `[05]` fica em 3 dígitos porque `[2025]` (ano) é muito mais comum no nome de um torrent que episódio de 4 dígitos entre colchetes, e um ano lido como episódio descarta o torrent certo.

**Don't "fix" by:** estender `\[(\d{1,3})\]` para 4 dígitos "por consistência".

### 52. A lista de episódios é sintetizada, e a chave de um episódio é (anime, número)

**Location:** `anilist/episodes.go` — `EpisodeList`, `lastAiredEpisode`; `anilist/anilist.go` — `Media.NextAiringEpisode`; `files/filemanager.go` — `EpisodeKey`, `EpisodeStruct.Key()`, `loadBlockedEpisodesLocked`; `daemon/episodes.go` — `firstEpisodeToConsider`; `api/endpoint_episode_actions.go` e as rotas `/animes/{id}/episodes/{episodeNumber}/*`.

**What it looks like:** nós de `AiringNode` **fabricados** (id zero) para episódios que a AniList não devolve; `EpisodeStruct` sem `EpisodeID`; `blocked_episodes` guardando objetos; e o `{episodeId}` das rotas virando `{episodeNumber}`.

**Why it's right:**

**(a) `airingSchedule` não é a lista de episódios de um anime.** A AniList mantém uma **janela** de ~500 entradas de agenda por mídia e descarta as antigas. Medido na API: `Media(21)` (One Piece) tem `pageInfo.total = 500` e a página 1 **começa no episódio 1123** — 1 a 1122 não existem na API; `Media(20)` (Naruto, 220 eps) e `Media(16498)` (Shingeki no Kyojin, 25 eps) voltam com `total = 0`, agenda **vazia**. Enquanto a lista saía dali, o daemon literalmente não tinha episódio para processar nesses animes (o log mostrava `Processing anime episodes` e nada depois), e no One Piece começava no 1123 como se os anteriores não existissem. Além disso o código nunca paginou a agenda, então mesmo dentro da janela só os 25 primeiros nós chegavam — e paginar não resolveria nada, porque as páginas seguintes só trazem episódios **mais novos**.

**(b) Por isso a lista é sintetizada, e o último no ar vem de três fontes.** `lastAiredEpisode` combina o maior episódio já exibido da agenda, `nextAiringEpisode.episode - 1` e `media.episodes` — este último **somente quando `FINISHED`**: num `RELEASING` ele é a contagem PREVISTA da temporada, e usá-lo mandaria o daemon caçar episódio que ainda não foi ao ar. Nenhuma das três serve sozinha (agenda clipada/vazia, `nextAiringEpisode` nulo em anime terminado).

**(c) A chave passou a ser `(AnimeID, EpisodeNumber)`, não um id sintético.** Um episódio fora da janela não tem id de nó — e inventar um (`-(mediaID*10000+ep)`) faria a identidade de um episódio depender de uma fórmula que nada valida, com colisão silenciosa como modo de falha. Número de episódio dentro de um anime é único por definição e é o que o usuário vê. `EpisodeStruct.EpisodeID` foi **removido** (não deixado como campo morto) de propósito: é o compilador que aponta cada lugar que ainda identificava episódio pelo id antigo.

**(d) `blocked_episodes` legado é descartado, não migrado.** Ele era `[416348, ...]` — ids de nó que nada mais resolve. Um parse que falha travaria `LoadBlockedEpisodes` e com ela o passe de verificação, então o arquivo antigo é lido, logado como aviso e tratado como lista vazia. O usuário refaz os bloqueios com um clique por episódio; adivinhar seria pior.

**(e) `firstEpisodeToConsider` recua para o menor episódio salvo.** A regra é `Progress + 1` (e anime avulso tem `Progress = 0`, então começa no 1 sem precisar de flag). Mas `watched_episodes_to_keep` e a poda de assistidos só operam sobre episódios que aparecem na lista: um episódio salvo abaixo do progresso ficaria fora dela, nunca seria "checado" e cairia direto em `identifyEpisodesNotInWatching` — apagado ignorando o "quantos assistidos manter".

**(f) `{episodeId}` virou `{episodeNumber}` nas rotas, e `episode_id` saiu do JSON.** As rotas já tinham a forma certa; manter o nome antigo com semântica nova seria a pior combinação possível. `AnimeInfo.latest_episode_id` virou `latest_episode_number` pelo mesmo motivo.

**Don't "fix" by:**
- Paginar `airingSchedule` "para pegar os episódios que faltam": as páginas seguintes só têm episódios mais novos (ver (a)).
- Usar `media.episodes` como último no ar em anime `RELEASING` (ver (b)).
- Reintroduzir um `EpisodeID` sintético "para não mexer no estado em disco" (ver (c)).
- Tentar converter o `blocked_episodes` antigo (ver (d)).
- Fazer a lista começar em `Progress + 1` sem o recuo (ver (e)).

### 53. Busca de episódio descarta batch e filme, e usa `hasMovieMarker` — não `isMovie`

**Location:** `nyaa/nyaa.go` — `parseRow` de `ScrapNyaa` e de `ScrapNyaaForAnime` (na época desta decisão, `ScrapNyaaForMultipleEpisodes`, depois fundida em `ScrapNyaaForAnime`), `isMovie`, `hasMovieMarker`; `nyaa/nyaa_regex.go` — `reOvaPattern`.

**What it looks like:** dois guards no começo dos dois `parseRow` de episódio (`isBatch` e `hasMovieMarker`), e um `hasMovieMarker` que é `isMovie` **menos** o ramo final.

**Why it's right:**

**(a) Sem o guard de batch, um pack de 220 episódios entra como "episódio 1".** `[Erai-raws] Naruto - 001 ~ 220 [480p]` casa `- 001` em `extractEpisodeNumber`, e a busca multi-episódio (ao contrário da single) não filtrava batch. Resultado medido, na época: 220 episódios num registro só, com `IsBatch: false`, furando o teto de batch da época (`max_batch_episodes`, removido pela spec de batch-por-filtro) **e** `max_episodes_per_anime` — e organizado como episódio único (renomeado "Naruto - E01" com `rename_files_for_jellyfin`). Descartar em vez de aceitar como batch porque a busca de pack rodava **antes** na resolução da estratégia: um batch que valesse a pena já teria sido pego lá, e o que vaza aqui é exatamente o que o teto rejeitou. O guard em si (marcar `IsBatch`/descartar packs vazados de dentro do parser de episódio) segue valendo em `ScrapNyaaForAnime` hoje.

**(a2) O guard só vale se o `isBatch` reconhecer a faixa.** O padrão de faixa exigia espaço DEPOIS do segundo número (`\s\d{2,4}\s*[-~]\s*\d{2,4}\s`), então `One Piece EP 001-501` e `One Piece 001-501.mkv` passavam como "episódio 1" — o mesmo bug de (a) por outro buraco, medido ao vivo na busca do episódio 1 do One Piece (2 dos 7 candidatos eram packs de 500 episódios). O fim da faixa agora aceita espaço, fim do nome, `.` ou `[`.

**(b) O ramo `!reHasEpisode && !isBatch` de `isMovie` não pode virar guard de episódio.** `reHasEpisode` só cobre `- 05`, `episode 05` e `S01E05`; `extractEpisodeNumber` cobre também `EP05`, `E05`, `[05]`, `" 05 ("`, `" 05.mkv"` e `" 5"`. Chamar `isMovie` inteiro rejeitaria todo release nessas formas (os testes de "Lucky Star EP015 / E015 / 15" pegam isso). Daí o split: `hasMovieMarker` (keywords + OVA/ONA + special) serve os dois lados, e `isMovie` continua sendo o `hasMovieMarker` mais o fallback "não tem marcador de episódio" que só faz sentido na busca de filme.

**(c) `reOvaPattern` precisa de `\b`.** Era `\(?(ova|ona)\)?`, sem boundary: "ona" casa dentro de **Persona**, "ova" dentro de **Nova**. Enquanto o padrão só era usado na direção permissiva (`ScrapNyaaForMovie`) isso passava; como guard de episódio viraria rejeição do anime inteiro.

**Don't "fix" by:**
- Trocar `hasMovieMarker` por `isMovie` no guard de episódio "para reusar a função" (ver (b)).
- Aceitar o batch vazado marcando `IsBatch: true` em vez de descartar (ver (a)).
- Tirar o `\b` de `reOvaPattern` para "pegar `(OVA)` também" — `\b` já casa antes de `(` e depois de `)` (ver (c)).

### 54. Piso de seeders: seeders ilegível é descartado, tamanho ilegível passa

**Location:** `daemon/search.go` — `filterBySeeders`, `filterSearchResults`, `filterBySize`; `files/filemanager.go` — `Config.MinSeeders` (default `1`); `nyaa/nyaa.go` — `ParseSeeders`.

**What it looks like:** dois filtros com contrato quase idêntico e uma assimetria de propósito — `Size == 0` (parse falhou) **passa** pelo teto de tamanho, `seeders == 0` (inclusive `"-"`) **não passa** pelo piso.

**Why it's right:**

**(a) Sem piso, torrent morto é candidato válido — e ganha quando é o único.** Medido: `[DB] Naruto Shippuuden Movie 3` com **0 peers** foi o único candidato do episódio 3 e foi baixado. O default `1` barra só isso; `0` desliga, seguindo a convenção de `max_batch_torrent_size_gb`.

**(b) A assimetria com `filterBySize` é o ponto, não um descuido.** Tamanho desconhecido significa "não sei se cabe", e descartar trocaria "às vezes baixa um torrent grande" por "não baixa nada" — um bug de parsing viraria paralisação silenciosa. Seeders desconhecido é diferente: a coluna é o **único** sinal de que existe alguém semeando, e um torrent sem semeador não completa nunca. Deixar passar não é o lado conservador — é travar o episódio num torrent que não baixa.

**(c) O filtro ficou no daemon, não no `parseRow` do nyaa.** O plano pedia no parse, "antes de qualquer ordenação". Como o sort é estável e os filtros preservam a ordem, o escolhido é o mesmo; e no daemon reusa o padrão do `filterBySize`, vale para todos os call sites de busca num ponto só (`filterSearchResults`) e mantém o log `"Found N results"` do nyaa mostrando o torrent morto com seus seeders — que é o que serve para diagnosticar. `nyaa.ParseSeeders` existe só para isso (mesmo padrão de `IsBatch`/`ExtractSeason`).

**Don't "fix" by:**
- Fazer `Size == 0` ser descartado "por simetria com os seeders" (ver (b)).
- Subir o default para "só torrent saudável": ranking é trabalho do critério `health`, não do piso — o piso existe para o que não baixa de jeito nenhum.

### 55. `health` compara FAIXA de seeders, e vem antes de `fansub`

**Location:** `nyaa/priorities.go` — `criterionCompare["health"]`, `healthTier`, `healthTierFloors`, `DefaultPriorities().CriteriaOrder`; `nyaa/nyaa.go` — `torrentHealthScore` (só log).

**What it looks like:** o comparador de `health` não usa `torrentHealthScore`; usa um índice de faixa (`0 / 1-4 / 5-19 / 20-99 / 100-399 / 400+`). E `health` está na 4ª posição da ordem default, não na 7ª.

**Why it's right:**

**(a) O sort é lexicográfico, então score cru = health sozinho decide ou nunca decide.** Com o score contínuo, 1 seeder de diferença já resolve a comparação e nenhum critério depois de `health` chega a ser consultado. Comparando faixa, 400 seeders vence 12 (faixas 5 e 3), mas 150 vs 300 empata e o `fansub` decide — que é o comportamento pretendido.

**(b) `health` na 7ª posição nunca era alcançado.** Medido no With You: os 3 candidatos eram 1080p, o `fansub` desempatou (subsplease=0, judas=2, ember=5) e `health` não foi consultado — um torrent com 12 seeders ganharia de um com 400 por ordem de fansub. A ordem nova é `uncensored, source, resolution, health, codec, fansub, audio, size`.

**(c) Faixa fixa, não razão.** "Empata a menos que um tenha 2x o outro" parece melhor (sem efeito de borda), mas não é transitivo — 100 ~ 150 ~ 220 e 100 < 220 — e `sort.SliceStable` com comparador intransitivo devolve ordem arbitrária. O preço da faixa fixa é a borda: 399 e 400 caem em faixas diferentes. Aceito de propósito; qualquer bucketização tem borda.

**(d) `torrentHealthScore` ficou só no log.** Ele combina seeders com a razão seeders/leechers, informação que a faixa (só seeders) descarta. Vale como diagnóstico no `matched_torrents` (`t=` faixa, `h=` score), não como critério.

**Nota de entrega:** um `config.json` existente **já tem** `criteria_order` gravado e `LoadConfigs` desserializa por cima do default, então a ordem nova NÃO chega sozinha em instalação antiga. O usuário edita à mão ou clica "restaurar defaults" na página de Prioridades.

**Don't "fix" by:**
- Voltar o comparador para `torrentHealthScore` "porque usa mais informação" (ver (a) e (d)).
- Trocar faixa por razão/tolerância percentual (ver (c)).
- Migrar `criteria_order` de config existente automaticamente: reordenar silenciosamente a preferência que o usuário gravou é pior que a nota de entrega.

### 56. Série longa busca o episódio também com zero-padding, e o gate é `LastAiredEpisode`

**Location:** `nyaa/nyaa.go` — `episodeQueries`, `longSeriesEpisodes`, `ScrapNyaa`; `anilist/episodes.go` — `LastAiredEpisode`; `daemon/episodes.go` e `daemon/manual_download.go` — `seriesLength` / `totalEpisodes`.

**What it looks like:** `ScrapNyaa` faz duas buscas (`one piece 1` e `one piece 001`) num anime longo, e o "é longo?" vem de `anilist.LastAiredEpisode`, não de `Media.Episodes`.

**Why it's right:**

**(a) `q=one piece 1` nunca acha o episódio 1.** A query casa tudo que contém "1" e, ordenada por seeders, as primeiras centenas de linhas são os episódios 1160-1173. Medido: 375 linhas sem o episódio 1. Grupos numeram série longa com padding (`[Judas] One Piece 001-574`, `[Erai-raws] Naruto Shippuuden - 001 ~ 079`).

**(b) Variante ADICIONAL, não substituição.** Padding é convenção, não regra: quem numera solto (`Show 5`) continua sendo pego pela query simples. As duas listas entram no mesmo pool, deduplicado por magnet.

**(c) O gate é `LastAiredEpisode`, e isso importa.** O plano pedia `Media.Episodes > 100`, mas num RELEASING sem fim anunciado esse campo é **nil** — ou seja, seria nil exatamente no One Piece, o caso que a mudança existe para consertar. `LastAiredEpisode` já combina as três fontes (agenda, `nextAiringEpisode - 1`, e `media.episodes` só quando FINISHED) e devolve ~1173 ali. Coberto por `TestSingleEpisodeSearch_ReceivesSeriesLength`, que usa um anime no formato One Piece (RELEASING, `Media.Episodes == nil`).

**(d) O teto de 100 e o `episode < 100`.** Cada variante custa fetches, então padding só em série longa; e episódio >= 100 já tem 3 dígitos, onde a variante seria idêntica à simples.

**(e) Erro numa variante não anula a outra.** `ScrapNyaa` só devolve erro se NENHUMA variante respondeu; se a página da query com padding falhar, a busca segue com o que a outra trouxe.

**Don't "fix" by:**
- Trocar o gate por `Media.Episodes` "que é o total de verdade" (ver (c)).
- Substituir a query simples pela com padding (ver (b)).
- Sempre buscar as duas formas: dobra o tráfego no Nyaa em todo anime curto, onde padding não muda nada (ver (d)).

### 57. Paginação adaptativa: sequencial, com piso de candidatos, sem orçamento por anime

**Location:** `nyaa/nyaa.go` — `fetchSearchPages`, `parsePagesWith`, `enoughCandidates`, `SetMaxSearchPages`/`ActiveMaxSearchPages`; `files/filemanager.go` — `Config.MaxSearchPages`, `applyNyaaSettings`.

**What it looks like:** um só helper de paginação usado pelas três buscas (`ScrapNyaa`, `ScrapNyaaForAnime`, `ScrapNyaaForMovie` — na época desta decisão eram quatro, `ScrapNyaaForBatch` e `ScrapNyaaForMultipleEpisodes` depois se fundiram em `ScrapNyaaForAnime`), que desce página por página **sequencialmente** enquanto tiver menos de 3 candidatos aceitos, com teto configurável.

**Why it's right:**

**(a) A página 2 era buscada SEMPRE.** Nas buscas que resolvem na página 1 — a maioria — isso era um fetch jogado fora. É essa economia que paga o teto de 5 páginas sem subir o tráfego médio: um teto fixo de 5 custaria fetches em disparada por passada contra o nyaa.si, onde se espera 429/Cloudflare.

**(b) Piso de candidatos aceitos, não de linhas.** O Nyaa devolve ordenado por seeders desc, então a partir de um punhado de candidatos a página seguinte só traz opção pior; 3 já dá escolha ao ranking. Página **vazia** também encerra: significa que a query acabou, e insistir até o teto seria fetch garantido inútil.

**(c) Sequencial, sem rajada paralela.** O plano previa 2-3 concorrentes "quando de fato precisar descer". Ficou de fora de propósito: cada página só é pedida porque a anterior não bastou (paralelizar exigiria pedir páginas que talvez não fossem necessárias, desfazendo (a)), o ganho é só de latência num caminho raro, e rajada paralela é justamente o que provoca 429/Cloudflare — o problema que o resto da decisão evita. Sem throttle no cliente, esse é o pior lugar para adicionar concorrência.

**(d) Batch e filme entraram no mesmo caminho.** Os dois liam só a página 1 com `httpGet` direto, sem passar por `fetchNyaaPage` — não paginavam e **não apareciam no log**. Agora usam o mesmo helper e ganham o log de página junto.

**(e) NÃO existe orçamento de fetches por anime por passada.** O plano pedia, para que um anime insolúvel não queime o teto toda passada. Medido: o pior caso é uma busca que nunca satisfaz o piso e vai até 5 páginas, ~2,3x os fetches de hoje, e **só** em anime que não encontra nada. O orçamento exigiria estado por anime atravessando `nyaaSearcher` e as funções exportadas do nyaa — plumbing desproporcional ao risco. Se o tráfego incomodar, o caminho mais curto é baixar `max_search_pages`.

**Don't "fix" by:**
- Buscar as páginas 1..N em paralelo "para ficar mais rápido" (ver (a) e (c)).
- Trocar o piso por "sempre buscar o teto" (ver (a)).
- Continuar descendo depois de uma página vazia (ver (b)).

---

### 58. `RunAnimeDebug` espelha `processAnimeEpisodes`: enumeração, resolução de pack e fallback single

**Location:** `daemon/debug.go` — `RunAnimeDebug`; espelha `daemon/episodes.go` — `processAnimeEpisodes`.

**What it looks like:** o debug de um anime não itera `anime.Media.AiringSchedule.Nodes`; monta a lista com `anilist.EpisodeList(anime, firstEpisodeToConsider(anime, nil))`, roda a mesma busca por anime + `partitionSearchResults`/`pickBatches` para decidir se um pack cobre a janela (e, se sim, re-roda `selectEpisodes` com o teto levantado), e cai em `searchSingleEpisode` quando nenhum episódio pendente recebeu magnet de pack/filme — três trechos duplicados do loop de produção. Ver a spec `2026-08-13-batch-por-filtro-design.md`: essa entrada documentava `willBatchAnime`/`resolveSearchStrategy`, removidos por essa spec — elegibilidade a pack agora é decidida pelo resultado da busca filtrada, não por metadado do AniList (ver entrada nova sobre isso).

**Why it's right:**

**(a) A agenda crua descreve outro pipeline.** No One Piece a página 1 da agenda vai do 1123 ao 1147; a produção enumera 1..1147 e pega os 12 primeiros pelo teto. Iterando os nós crus, o debug reportava ter buscado "1123 a 1147" — nenhum anime de agenda clipada era medido corretamente, e é exatamente a classe que a `EpisodeList` sintética (#52) existe para cobrir.

**(b) A regra pack↔limite nunca era avaliada.** `pickBatches`/`assignBatches` levantam o teto por anime em `processAnimeEpisodes` quando um pack cobre a janela; sem rodar o mesmo caminho, o debug era cego justamente à regra que a feature de animes avulsos declara como pré-requisito ("um anime de 1100 episódios adicionado a dedo cai no caminho um-a-um limitado, a menos que um pack cubra a janela").

**(c) Sem o fallback, série longa era falso positivo garantido.** A busca múltipla não leva número de episódio na query (`q=one piece`), e as primeiras páginas ordenadas por seeders são só os episódios recentes — medido: 750 linhas cruas, nenhuma do episódio 1-12. Quem acha o episódio antigo é `searchSingleEpisode`, que carrega o zero-padding (#56). Sem ele o relatório do `debug-batch` marcava One Piece e Naruto Shippuden como "0 magnets" por **não ter buscado**, não por o Nyaa não ter.

**(d) `savedEpisodes = nil` é intencional.** `firstEpisodeToConsider` só usa esse argumento para recuar o início abaixo de `Progress + 1`, e o debug já declara — no `ponytail:` e no `Warn` que emite — que trata todo episódio como não-baixado. Com `nil` resolve para `Progress + 1`, que num avulso é 1.

**Trade-off aceito:** `summary.Episodes` passa a ter uma entrada por episódio enumerado (~1147 no One Piece, a maioria com `would_search: false`), e um FINISHED curto entra em batch, alongando a rodada. É o preço da fidelidade; o `report.md` do `debug-batch` agrega.

**Don't "fix" by:**
- Voltar a iterar `AiringSchedule.Nodes` "porque é mais simples" — a simplicidade produz um diagnóstico de um pipeline que o daemon não executa.
- Parar na resolução de pack "porque é a etapa que decide a estratégia" — ela é só metade da busca; a outra metade é o fallback por episódio.
- Truncar `summary.Episodes` aos que têm `would_search: true` — a contagem "nada buscado" do relatório depende de ver os dois lados.

---

### 80. O teto de tamanho de pack é aplicado DUAS vezes: na busca do Nyaa e no filtro do daemon

**Location:** `nyaa/nyaa.go` — `maxBatchSizeBytes`, `SetMaxBatchTorrentSizeGB`, `batchTooBig`, aplicado no `parseRow` de `ScrapNyaaForAnime`; e `daemon/search.go` — `filterBySize`, via `partitionSearchResults`.

**What it looks like:** `max_batch_torrent_size_gb` já é aplicado pelo daemon em `filterBySize`, e mesmo assim o pacote `nyaa` recebe o mesmo teto por push (`applyNyaaSettings`) e descarta a linha do pack antes de acrescentá-la ao resultado. Parece filtro duplicado, e o comentário antigo do `ScrapNyaaForAnime` dizia explicitamente que empurrar filtro para dentro do `nyaa` era o que `applyNyaaSettings` evitava.

**Why it's right:** os dois filtros não fazem a mesma coisa, porque rodam em momentos diferentes do algoritmo. O do daemon decide **o que baixar**. O do `nyaa` decide **até onde descer as páginas** — `fetchSearchPages` para quando `accepted()` alcança `enoughCandidates` (3), e `accepted()` conta o que a busca aceitou. Um pack que o daemon vai descartar por tamanho, mas que a busca aceitou, gasta uma das três vagas do piso e encerra a descida cedo demais.

Medido no Nyaa (agosto/2026, `?q=one+piece&s=seeders&o=desc`, que é exatamente a query montada por `ScrapNyaaForAnime`):

| página | pack | tamanho |
|---|---|---|
| 1 | (pack completo) | 587,5 GiB |
| 1 | `[Judas] One Piece 001-574` | 171,8 GiB |
| 2 | `[Erai-raws] One Piece 0892~1089` | 190,3 GiB |
| 2 | (pack completo) | 1,3 TiB |
| 2 | `[Judas] One Piece 783-1023` | 86,2 GiB |
| 3 | `[Judas] One Piece 575-782` | 61,0 GiB |
| 3 | `One Piece S01E1109-E1122` | 20,0 GiB |
| 3 | `[ToonsHub] One Piece EP1144-EP1155` | 16,6 GiB |

Os três primeiros aceitos são todos gigantes, o piso fecha, e os packs de 86 / 61 / 20 / 16 GiB — que **cabem** no teto padrão de 100 — nunca são vistos. O sintoma reportado era "One Piece nunca baixa porque todos os packs achados passam do teto"; a causa não é o teto ser baixo, é a busca parar antes de achar os que cabem. Subir o teto para 200 esconderia o bug e ainda enfileiraria 171 GiB no lugar de 61 GiB.

O push da config para dentro do `nyaa` não é padrão novo: é o mesmo de `SetPriorities` e `SetMaxSearchPages` (o `nyaa` não pode importar `files` — ciclo —, então a config chega por push). O default é `0` (desligado), então o pacote usado sem push se comporta como antes.

**Trade-off aceito:** o teto de pack aparece em dois lugares e precisa continuar coerente entre eles (mesma regra de `Size == 0` passa livre). O do daemon continua sendo a autoridade — se o push não acontecer, nada vaza para download.

**Don't "fix" by:**
- Remover o filtro do `nyaa` "porque `filterBySize` já faz isso" — é justamente o que quebra a paginação.
- Remover o filtro do `daemon` "porque agora o `nyaa` já filtra" — `filterBySize` também cobre `max_episode_torrent_size_gb` e é o que garante o resultado quando a config não foi empurrada (testes, uso direto do pacote).
- Empurrar também `min_seeders` para o `nyaa` "por simetria" — o Nyaa já devolve ordenado por seeders desc, então o filtro de seeders não trunca a descida do mesmo jeito: os melhores vêm primeiro por construção.
- Subir `max_batch_torrent_size_gb` como "solução" para série longa — ver acima.

---

### 81. A precedência dos códigos do relatório é cascata, não conjunto

**Location:** `daemon/report.go` — `searchIssue`.

**What it looks like:** um episódio pode satisfazer três condições ao mesmo tempo — todos os candidatos acima do teto, todos abaixo do piso de seeders, e `len(magnets) == 0`. `searchIssue` escolhe **um** código com um `switch` cuja ordem parece arbitrária, e ignora que as outras condições também são verdadeiras.

**Why it's right:** a ordem é a regra de negócio. Quando um filtro esvazia a lista, "nenhum torrent encontrado" **também** é verdade — e é a resposta menos acionável das três, além de mentirosa: havia oito. "Todos os candidatos tinham 8 GB e seu teto é 3 GB" diz ao usuário o que fazer (subir o teto, ou ajustar a query customizada). "Nenhum torrent encontrado" não diz nada, e manda ele procurar no Nyaa um torrent que já tinha sido achado.

Mesma disciplina da cascata de `deriveAnimeChip` (`lib/domain/animeState.ts`): a primeira condição que casa vence, e a ordem é a regra.

**Trade-off aceito:** um episódio cortado pelos dois filtros aparece só como `all_above_size_limit`. É deliberado — o usuário resolve um de cada vez, e o de tamanho é o que ele controla diretamente.

**Don't "fix" by:**
- Devolver uma lista de códigos "porque as três condições valem" — a UI passaria a mostrar três linhas por episódio, duas delas ruído.
- Reordenar para pôr `no_torrent_found` primeiro "porque é o caso mais comum" — comum não é o critério; acionável é.
- Alimentar a cascata com o `packStats` de `partitionSearchResults` — o pack é decidido para o anime, não para o episódio, e o fallback por episódio é quem dá a última palavra sobre aquele episódio.

---

### 82. `SetLastCheckError` limpa o relatório da última verificação

**Location:** `daemon/state.go` — `SetLastCheckError`; `daemon/verification.go` — a ordem `SetLastCheckError(nil)` → `SetLastCheckReport(...)`.

**What it looks like:** um setter de erro que também apaga um campo não relacionado, e um `SetLastCheckReport` que **precisa** vir depois de um `SetLastCheckError(nil)` que já rodou três linhas antes. Parece acoplamento acidental, e a correção "óbvia" é separar as duas coisas.

**Why it's right:** as saídas antecipadas de `AnimeVerification` (config incompleta, probe de hardlink, backend não inicializado, `Ensure`, migração de ids, AniList — eram sete até a migração do `save_path` sair, ver #31) já chamam `SetLastCheckError`. Com a limpeza dentro dela, **nenhuma delas precisa de linha nova** — e nenhuma pode esquecer. E a semântica é a certa: um passe que abortou antes de olhar anime nenhum não tem relatório por anime, tem `pass_error`. Sem a limpeza, a tela mostraria os problemas do passe *anterior* lado a lado com um erro de passe novo, sugerindo que os dois vieram da mesma verificação.

A segunda consequência é igualmente desejada: o cancelamento (`verification.go`, no `select` do `ctx.Done()`) chama `SetLastCheckError(nil)` e retorna, então **passe interrompido não deixa relatório** — ele estava incompleto, e um relatório parcial diria "só este anime teve problema" quando os outros nem chegaram a ser olhados.

**Trade-off aceito:** a ordem das duas chamadas no fim de `AnimeVerification` é significativa e não é óbvia lendo só aquele trecho. É por isso que existe um teste dedicado (`TestSetLastCheckReport_AfterClearingError`) travando exatamente essa ordem.

**Don't "fix" by:**
- Tirar a limpeza de dentro de `SetLastCheckError` e pôr uma chamada explícita em cada saída antecipada — sete lugares para esquecer um.
- Mover `SetLastCheckReport` para antes do `SetLastCheckError(nil)` "porque lê melhor" — o relatório é apagado no instante seguinte e a feature inteira vira silêncio.
- Persistir o relatório em disco para "sobreviver ao restart" — `createStartFunc` chama `AnimeVerification` imediatamente ao iniciar (`loop.go`, antes do primeiro `time.After`), então ele se reconstrói em segundos; um arquivo custaria persistência, migração e a chance de mostrar um relatório de dias atrás como se fosse do último passe.

---

### 63. A dispensa do card de primeiros passos mora no `localStorage`, não no `config.json`

**Location:** `src/internal/frontend/src/lib/stores/onboarding.ts`; consumido por `routes/Status.svelte` (condição de render) e `routes/Config.svelte` (o botão "Mostrar novamente").

**What it looks like:** uma preferência do usuário que o app inteiro respeita, guardada no navegador em vez de no arquivo que o daemon já lê e escreve. Parece uma inconsistência — todo o resto da configuração está em `config.json`, e a "correção" óbvia é criar um campo `tutorial_dismissed` lá.

**Why it's right:** não é configuração do daemon; é preferência de UI, e o daemon não tem nada a fazer com ela. Um campo em `config.json` entraria no schema que o servidor valida, no corpo do `PUT /config` e — se alguém fosse coerente — no `requiredChecks` da tela de Configurações, três lugares para um booleano que só o navegador consome. E ele valeria para **todos** os dispositivos de uma vez: quem dispensou no desktop nunca mais veria o card no celular, onde ainda não conhece o app. `localStorage` é por navegador, que é exatamente o escopo certo, e é a mesma escolha que `theme` e `locale` já fazem neste frontend.

**Trade-off aceito:** trocar de navegador traz o card de volta, e limpar o storage do site também. Para um app self-hosted de um usuário só, ver de novo um card de três linhas é barato; o inverso — não conseguir vê-lo num dispositivo novo — é o caro.

Esconder o card com a biblioteca vazia deixa o app inoperante, e mesmo assim a dispensa é permanente: o passe de verificação continua abrindo `#/config?missingConfig=true` e o banner de lá continua aparecendo. É esse backstop que torna a dispensa permanente segura de oferecer.

**Don't "fix" by:**
- Criar `tutorial_dismissed` em `config.json` "para seguir o usuário" — põe preferência de UI num schema do daemon e uniformiza dispositivos que deveriam ser independentes.
- Trocar o botão "Não mostrar mais" por um `×` "porque é o padrão de card" — num card de três itens o `×` lê como "fechar até recarregar", e o comportamento é permanente. O rótulo é a documentação do comportamento.
- Fazer o card sumir sozinho depois de N dias/visitas — some quando o usuário marca os três, quando dispensa, ou quando a instalação já está configurada e rodando; tempo não significa nada.

---

### 64. Os passos do card de primeiros passos são marcados à mão, não derivados do estado do daemon

**Location:** `src/internal/frontend/src/lib/stores/onboarding.ts` (`onboardingDone`); pintado por `routes/Status.svelte`.

**What it looks like:** o app **sabe** se a pasta está configurada e se o passe já rodou — `lib/domain/onboarding.ts` calcula exatamente isso — e mesmo assim os checkmarks do card saem de uma lista guardada no `localStorage` que o usuário preenche clicando. Parece estado duplicado, e a "correção" óbvia é derivar os três dos dados que a tela já tem.

**Why it's right:** foi o que a primeira versão fazia, e numa instalação nova os passos ① e ③ nasciam verdes sozinhos — a pasta da biblioteca tem default e o passe de verificação roda a cada 10 minutos sem ninguém pedir. O tutorial aparecia dois terços concluído antes de o usuário ter lido a primeira linha, o que ao mesmo tempo mente sobre o que ele fez e tira o motivo de ler o resto. Marcar é a leitura confirmada, não o estado do disco.

A derivação continua existindo, mas com outro papel: `allDone(onboardingSteps(...))` é o portão de "esta instalação já está configurada e rodando, não ensine o óbvio", então quem já usa o app nunca vê o card. Dois usos, uma fonte de verdade cada.

**Don't "fix" by:**
- Derivar os checkmarks de novo "para não duplicar estado" — é exatamente o bug que isto conserta.
- Marcar o passo automaticamente quando a ação dele é feita no app (configurou a pasta → marca ①) — volta ao mesmo lugar para quem já tinha a pasta configurada, e faz o card se mexer sozinho enquanto está sendo lido.
- Trocar os números por ícones de status — a numeração é o que diz que há uma ordem; o número dobra de caixa de marcar justamente para não haver dois marcadores para um estado só.

---

### 65. O orçamento da AniList é gasto pelo refresh de órfãos, não pelo passe do daemon — e o campo `errors` de um 200 não é lido

**Location:** `src/internal/anilist/anilist.go` (`sendAnilistRequest`, `ttlCache`); `src/internal/api/endpoint_animes.go` (`refreshOrphanAnimes`); `src/internal/daemon/verification.go` (`searchAnilist`).

**Documentação oficial:** [errors](https://github.com/AniList/docs/blob/master/docs/guide/graphql/errors.md), [rate-limiting](https://github.com/AniList/docs/blob/master/docs/guide/rate-limiting.md), [considerations](https://github.com/AniList/docs/blob/master/docs/guide/considerations.md). **Os números de lá não são copiados para cá de propósito** — o limite documentado é 90/min mas hoje está degradado para 30/min, e a própria doc chama isso de temporário. Número copiado apodrece em silêncio e alguém dimensiona cache por ele seis meses depois. A doc é a fonte; aqui fica só o que é nosso.

**O que parece:** o daemon é o que fala com a AniList, então o passe de verificação parece ser o consumidor do limite, e os TTLs de `ttlCache` parecem cautela genérica.

**Por que está certo:** medindo por minuto, por conta configurada:

| Origem | Requisições | Observação |
|---|---|---|
| Passe do daemon | ~0,3/min | 3 por passe (`GetCustomListsMap` + `GetAllCurrentAnime` de download + `GetAllCurrentAnime` de delete), a cada `check_interval` (default 10 min) |
| Poll de `/animes` | ≤1,2/min | `GetFrontendAnimeList` (TTL 60s) + `GetCustomListsMap` (TTL 5min) |
| `refreshOrphanAnimes` | **1 por conta a cada 50 órfãos** | `GetAnimeInfoByIDs` (`mediaList(mediaId_in:)`), sem cache. Era **1 por anime órfão por conta**, com um semáforo de 5 concorrentes que limitava o paralelismo e não o total. Corrigido 29/ago/2026; o fallback de avulso na mesma função virou um `GetMediaByIDs` |
| `appendStandaloneEntries` | **1 por lote de 50 avulsos** | `GetMediaByIDs` (`Page(media(id_in:))`), nos dois lados (`api/standalone.go` e `daemon/standalone.go`). Era `GetMediaByID` em loop: medido 28/ago/2026, com 20 avulsos eram 20 dos 30 requests do balde, gastos só para montar uma tela. Corrigido 29/ago/2026; o cache continua **por id**, então o lote e o lookup avulso se aproveitam um do outro |

O passe do daemon é ruído. Quem estourava o limite era o refresh de órfãos: uma requisição por anime baixado que a busca filtrada não cobriu, disparada **a cada poll do frontend**. Numa biblioteca com dezenas de animes fora dos status configurados, isso era uma rajada de dezenas de requisições — e a AniList tem um *burst limiter* separado do limite por minuto, que pune exatamente esse formato. **Corrigido em 29/ago/2026 por lote**, junto com os avulsos: as duas linhas em negrito acima passaram a custar `ceil(N/50)` em vez de `N`.

**`mediaId_in` existe e é o que torna o lote possível** (introspecção + medição, 29/ago/2026). `Page.mediaList` aceita `mediaId_in: [Int]` ao lado de `userName`, devolve **no máximo uma entrada por id** (logo, 50 ids nunca passam de uma página — não há paginação a seguir), **omite** silenciosamente o id que a conta não acompanha, e custa 1 unidade como qualquer query. Duas armadilhas medidas: `userName` inexistente responde **500**, e conta privada responde **404 "Private User"** — nenhum dos dois é 400, então uma query malformada não se distingue de uma conta ruim pelo status.

É por isso que `mergeFailed` pula o refresh (`endpoint_animes.go:182`) em vez de tratar "não coberto" como "precisa refresh": com a lista falhada, todo anime baixado vira órfão aparente, e a rajada resultante realimenta a falha. Os TTLs de `frontendListCache` e `customListsCache` também não são cautela genérica — são o que torna o custo do poll **independente do número de abas abertas**. Sem eles, duas abas e duas contas já eram ~16 req/min só de frontend parado.

**O campo `errors` de um 200 nunca é lido.** A doc é explícita: um 200 pode carregar erro no envelope. `sendAnilistRequest` desserializa direto no struct de dados e descarta `errors`, então esse caso hoje vira resposta vazia sem diagnóstico. **Isto não é a causa do `customLists` null da decisão #11** — aquele foi verificado por curl e volta 200 *sem* campo `errors`, é orçamento de complexidade. São dois problemas distintos com o mesmo sintoma aparente, e confundi-los faz procurar a correção no lugar errado.

**Taxonomia documentada** (`errors[].status` quando presente, senão o HTTP):

| Código | Significado | Recuperável sozinho? |
|---|---|---|
| 429 | Rate limit. Timeout de 1 min, com `Retry-After` e `X-RateLimit-Reset` | Sim |
| 403 | API desligada por instabilidade | Só quando a AniList voltar |
| 403 (mensagem custom) | IP bloqueado manualmente | Não |
| 400 | Query inválida / campo inexistente → o schema mudou, é bug nosso | Não |
| 404 | ID não existe | Não é falha — vira `ErrNotFound` |
| 5xx | Erro interno da AniList | Sim, transitório |

`errors[].validation` só existe em mutation; o app é somente-leitura e ignora.

**Don't "fix" by:**
- Otimizar o passe do daemon "porque ele fala com a AniList" — ele custa 0,3 req/min; o ganho é zero e o risco não.
- Fazer `refreshOrphanAnimes` rodar mesmo com a lista falhada, "para a tela não ficar desatualizada" — é o loop de realimentação que produziu o 429 original.
- Remover os TTLs porque "o daemon só busca a cada 10 minutos" — quem busca a cada 30 segundos é cada aba do frontend.
- Copiar o limite atual da AniList para cá como constante — ele é temporário e muda sem aviso.
- Estimar o consumo com um contador nosso em vez de ler `X-RateLimit-Remaining` da resposta — ver #72, que mede o balde e mostra por que erro também custa cota.
- Voltar a um request por id onde cabe uma lista — `Page(media(id_in: [...]))` para media solta, `mediaList(userName:, mediaId_in: [...])` para entrada de lista. Uma query custa 1 unidade independente do número de ids (#72). Os dois caminhos já são lote: `GetMediaByIDs` e `GetAnimeInfoByIDs`.

---

### 66. Banner é estado degradado; toast é falha de ação — e o banner tem três fontes com precedência, não três banners

**Location:** política para `components/shell/AppShell.svelte` (slot único de banner) e `lib/api/client.ts` (`ApiError`, opção `silent`).

**O que parece:** cada origem de erro quer o seu aviso, e a saída óbvia é empilhar — um banner de AniList, um de backend, o de WebSocket que já existe em `routes/Downloads.svelte:554`.

**Por que está certo:** a distinção não é a gravidade, é a **duração**:

- **Toast** — falhou uma ação que o usuário pediu (salvar config, adicionar anime). Já existe, já funciona, some sozinho.
- **Banner** — o estado está degradado entre polls e explica por que a tela inteira parece errada. Persiste enquanto o estado persistir e **some no primeiro sucesso**, nunca por timer.

O valor do banner não é reportar o erro: é explicar por que a lista está incompleta ou velha. Hoje `/animes` responde 200 com uma conta faltando e o usuário conclui que sumiu anime.

As fontes são independentes, mas **empilhar é errado**: se o backend não responde, a saúde da AniList é informação velha vinda do último poll bem-sucedido, e mostrá-la ao lado de "daemon não responde" é ruído contraditório. Precedência, do mais a montante para o mais a jusante (`lib/domain/systemBanner.ts`, `pickBanner`):

1. **Backend inalcançável** (`fetch` não recebeu resposta) — nada abaixo disso é observável.
2. **Backend respondendo 5xx** — bug nosso.
3. **AniList indisponível** (403/5xx) — banner mudo, "tente mais tarde".
4. **AniList em rate limit** (429) — contagem regressiva derivada do `Retry-After`, que é o único caso em que se sabe o tempo exato.

Mostra-se **um**, o de maior precedência.

**O banner de WebSocket de `routes/Downloads.svelte` continua onde está.** Ele fala sobre aquela tela — o progresso ali é empurrado por WS e fica defasado sem ele —, enquanto o `SystemBanner` fala sobre o app inteiro. Consolidá-los exigiria mover a posse do `WebSocketClient` para o `AppShell` (hoje instanciado em `Status` e `Downloads`), o que é uma refatoração de outro assunto. A ausência do daemon já é coberta pelo estado `unreachable`, que vem do HTTP.

**O botão de reportar só aparece em bug nosso** — backend 5xx, e 400 da AniList (schema mudou, nenhuma espera resolve). Um 429 ou um outage não são bug: botão de reportar ali só gera issue que se fecha com "é a AniList", e treina o usuário a ignorar o botão quando ele importa.

**Ponto cego que isto fecha:** `getLastCheck` e `getTorrents` passam `silent: true` para não tostar a cada tick. Correto para toast — mas hoje significa que um 5xx repetido nesses endpoints é **invisível para sempre**. A saúde do backend é derivada de falhas *consecutivas* por endpoint (≥2), não de uma falha isolada, e por isso convive com `silent` sem reintroduzir o spam.

**O `pass_error` segue a mesma regra.** O banner de passe abortado da tela de Status mostrava `err.Error()` cru — que, com a AniList fora do ar, era o corpo JSON da resposta inteiro (`{"errors":[{"message":…,"locations":[{"line":1,"column":1}]}]}`) na cara do usuário. `CheckReport` ganhou `pass_error_code`, classificado em `daemon/passerror.go`, e a frase é montada em `lib/domain/passError.ts`. O texto cru continua em `pass_error` e é mostrado **recolhido** num `<details>`: é a única coisa colável numa issue, mas não pode ser a primeira coisa que se lê.

A causa viaja **dentro** do erro (sentinelas embrulhadas com `%w`), e não num parâmetro novo de `SetLastCheckError`: são ~15 chamadas, a maioria passando `nil` para limpar, e nenhuma delas precisou mudar. Um sítio de aborto novo que esqueça de embrulhar cai em `unknown`, que tem frase própria — degrada, não quebra.

**Don't "fix" by:**
- Empilhar os banners "porque são problemas diferentes" — com o backend fora, o estado da AniList na tela é mentira datada.
- Fazer o banner sumir por timer — o estado ou acabou (chegou um 200) ou não acabou.
- Pôr botão de reportar em todo banner "por simetria" — reportar rate limit é reportar comportamento normal.
- Trocar o banner de outage por toast "porque é erro" — toast some, o outage não; a tela continua errada depois que ele sumiu.
- Mandar frase pronta do Go em vez de código, "porque é uma string só" — o daemon não sabe o locale do navegador, e foi assim que o JSON cru chegou à tela.
- Remover o `<details>` do texto cru "porque o usuário não entende" — quem abre issue precisa dele, e recolhido ele não atrapalha quem não precisa.

### 67. Passe de verificação é um por vez, e o concorrente é descartado — não enfileirado

`AnimeVerification` pega `verificationMu.TryLock()` e **volta na hora** se outro passe está rodando.

O passe lê `episodes.json` **uma vez** no começo (fase 1) e só escreve no fim (fase 3, sequencial). Dois passes simultâneos — o do loop e o do `POST /check`, dois `/check`, ou um `/check` logo depois de um `UpdateInterval` — enxergam a mesma lista velha: o segundo não vê nada do que o primeiro acabou de baixar e baixa tudo **de novo**. Foi assim que um anime já coberto por um pack (`01 ~ 11`, episódios 6..11 registrados) ganhou torrents avulsos dos episódios 10 e 11, e a escrita do segundo passe sobrescreveu os registros do pack com os hashes avulsos — dois torrents dos mesmos episódios no disco e a tela mostrando o pack e os avulsos lado a lado.

Descartar, e não enfileirar, é deliberado: o passe descartado leria exatamente o estado que o passe em andamento está corrigindo, e o próximo tick do loop cobre o que faltar. Enfileirar só adiaria a mesma leitura velha.

**Don't "fix" by:**
- Recarregar `episodes.json` no meio do passe "para ver o que o outro salvou" — a janela continua aberta entre a leitura e a escrita, e as duas escritas continuam brigando.
- Trocar por `Lock()` (enfileirar) — vira uma fila de passes redundantes que rodam com estado velho, cada um segurando o próximo.
- Tirar o `TryLock` porque "o `POST /check` já roda em goroutine" — a goroutine é o que cria a concorrência, não o que a resolve.

### 68. Numeração do pack: o offset é adivinhado dos próprios arquivos, e a colisão de basename usa o caminho relativo

**Location:** `internal/files/librarian.go` — `packEpisodeOffset` e a guarda `if used[destName]` em `Organize`; `internal/daemon/jobs.go` — `OrganizeRequest.TotalEpisodes`. Testes em `librarian_test.go` (`TestOrganizeBatch*`, `TestPackEpisodeOffset`) e `orchestration_test.go` (`TestOrganizeTorrent_BatchContinuousNumbering`).

**(a) O offset sai dos arquivos, não da AniList.** Pack de season >= 2 passa pelo filtro de season (`nyaa.go`, ramo `isBatch`) carregando o marcador no nome do torrent, mas os arquivos lá dentro podem estar numerados continuamente (13-24 para uma entrada de 12 episódios). Como cada season é uma entrada própria com numeração começando em 1 (decisão #45), esses arquivos entravam na biblioteca como `E13`..`E24` numa pasta que o Jellyfin conhece com 12 episódios.

A fonte *correta* do offset é a contagem de episódios do PREQUEL — o que `daemon.ComputeEpisodeOffset` já calcula na busca —, mas ela não existe no organize: o job só tem o hash e os registros de `episodes.json`. Trazê-la pediria campo novo em `EpisodeStruct` (com migração) ou uma requisição à AniList dentro do job. `packEpisodeOffset` evita as duas: usa `AnimeTotalEpisodes`, que **já** está gravado no registro, e só desloca com evidência inequívoca — **todo** arquivo numerado acima do total da entrada **e** pelo menos um arquivo por episódio. Isso é um pack completo que começa no episódio 1 da season, então `min - 1` é o offset.

Os dois guardas importam: sem o "todo arquivo acima do total", um extra numerado 13 num pack 01-12 deslocaria a season inteira; sem o "pelo menos um arquivo por episódio", um pack parcial (23-24 de uma season de 12) seria tratado como completo e viraria `E01`/`E02` em vez de `E11`/`E12`. Pack parcial com numeração contínua fica **como está** — errado, mas não *mais* errado, e é o caso raro.

**(b) A colisão de basename passou a desempatar pelo caminho relativo.** `collectVideoFiles` achata subpastas e o destino saía de `filepath.Base(rel)`. Num pack multi-season com uma subpasta por season, `Season 1/Anime - 01.mkv` e `Season 2/Anime - 01.mkv` apontavam para o mesmo destino, e o segundo caía no ramo de "bytes diferentes com o mesmo nome" (feito para redownload/replace) **removendo o hardlink do primeiro**: um episódio sumia da biblioteca e `created` gravava o mesmo caminho duas vezes. O `used` já existia, mas só protegia o nome Jellyfin. Agora ele é checado para *qualquer* destino e o desempate é o caminho relativo, que é único dentro do torrent.

O ramo de substituição continua intacto para o que ele foi feito: colisão entre execuções diferentes de `Organize`. O que mudou é só a colisão **dentro da mesma execução**, que nunca é uma substituição legítima.

**Don't "fix" by:** aplicar o offset sem os dois guardas "porque pack contínuo é comum"; usar `max - total` no lugar de `min - 1` (concorda no pack completo e erra no pack parcial do começo da season); chamar a AniList de dentro do `JobOrganize` para pegar o PREQUEL; criar subpasta `Season NN/` na biblioteca para "resolver a numeração" (decisão #45); deduplicar a colisão de basename sobrescrevendo "porque o arquivo é o mesmo anime".

### 69. `codec` passou a valer para episódio, e nome sem tag de codec empata em vez de perder

`episodeCriteria` (`nyaa/priorities.go`) filtra `CriteriaOrder` para os critérios que fazem sentido
num episódio, e `codec` estava **fora**: a lista `priorities.codecs` aparecia na tela de Priorities e
no `config.json` sem afetar download de episódio nenhum — só `SortMovieResults` a lia. Como o
default é `["hevc","av1",...]`, o codec do episódio era decidido de raspão pelo `fansub`
(Judas/Ember são x265 10-bit; SubsPlease/Erai-raws são H264), e o usuário não tinha alavanca
nenhuma. Quem assiste por um cliente sem decode de HEVC (navegador — Chrome e Firefox não trazem
HEVC por licenciamento) paga isso como transcode no servidor, que além de queimar CPU **queima a
legenda ASS no vídeo** e dessincroniza. `codec` agora está em `episodeCriteria`.

**Só `source` continua fora.** Episódio semanal é WEB-DL em praticamente todo release; o critério
nunca desempataria nada e só gastaria uma comparação.

**A guarda que faz a mudança não sair pela culatra:** `priorityIndex` devolve `len(list)` para token
desconhecido, ou seja, **o pior**. E `extractCodec` devolve `""` para nome sem tag de codec — que é
exatamente o padrão do SubsPlease (`[SubsPlease] Anime - 07 (1080p) [HASH].mkv` não diz H264 em
lugar algum). Com `codec` antes de `fansub` na ordem default, ligar o critério cru faria todo
release sem tag perder para qualquer x265 tagueado: o **oposto** do que a lista de codecs configura.
Por isso `codecCompare` devolve empate quando qualquer um dos dois nomes não tem tag, deixando o
critério seguinte decidir. É a mesma regra que `sizeCompare`/`resCompare` já aplicam para dado
ausente, e a mesma de `filterBySize` (`nyaa.go`): dado que não deu para ler não é motivo para punir
o release.

**Tokens canônicos.** `extractCodec` só devolve `HEVC`, `AV1`, `H.264` e `XviD` (`reCodecPatterns`
mapeia `x265`→`HEVC` e `x264`→`H.264`). As entradas `"x265"` e `"x264"` do default de
`priorities.codecs` portanto **nunca casam com nada** — são inertes. Configurar preferência por
H264 exige escrever `h.264`, não `x264`.

**Don't "fix" by:** tirar a guarda de `codecCompare` "porque sem tag é sem tag"; pôr `source` em
`episodeCriteria` junto; mover `codec` para depois de `fansub` no default para contornar o problema
do token desconhecido (esconde o mesmo bug em vez de resolvê-lo).

### 70. Resolução canonicaliza no extrator, e os presets de codec são reordenação no frontend

**Location:** `nyaa/nyaa.go` — `extractResolution`, `canonicalResolutions`; `nyaa/priorities.go` —
defaults de `Resolutions`/`Codecs`/`Sources`; `frontend/src/lib/domain/priorityPresets.ts`;
`frontend/src/routes/Priorities.svelte`.

**(a) A canonicalização vive em `extractResolution`, não na ordem da lista.** `extractCodec` já
devolvia token canônico (`x265` → `HEVC`) e `extractResolution` devolvia a captura crua. Essa
assimetria **era** o bug: `4k` e `2160p` são a mesma resolução e ocupavam dois índices diferentes do
default, então qual vencia dependia só de como o grupo escreveu o nome do torrent; `1920x1080` é
capturável pelo regex, não existia na lista, e ranqueava como o pior. Resolver isso reordenando a
lista (pondo `4k` ao lado de `2160p`) não resolve nada: continuam sendo dois itens que o usuário
pode separar de novo com as setas da tela, e a duplicata segue representável. Canonicalizando na
saída do extrator ela deixa de existir, e a lista default encurta para cinco itens legíveis.

**Não quebra config existente.** A lista antiga já tem `1080p`, `720p`, `480p` e `2160p` — os
canônicos. `4k`, `uhd`, `fhd`, `hd` e `8k` viram inalcançáveis mas inofensivos, e um release 4K de
um usuário antigo passa a casar com o `2160p` que ele já tem.

**`8k` fica fora da lista de propósito**, não por esquecimento: o padrão `\b(8K)\b` continua
existindo, o token continua sendo devolvido cru, e por não estar na lista ranqueia como o pior —
tratamento certo para uma resolução que não existe em anime.

**(b) Preset de codec é reordenação no frontend, não dado servido pelo backend.** Só o frontend
consome (o CLI não menciona priorities); um endpoint existiria para servir a si mesmo — ~60 linhas
em seis arquivos, mudança de forma da resposta e `swag init`, contra ~30 linhas num arquivo só.

E preset **promove** tokens que já estão na lista do usuário em vez de carimbar um array literal:
typo vira no-op em vez de config inerte, token adicionado à mão desce em vez de sumir, e os tokens
canônicos continuam existindo num lugar só (`reCodecPatterns`). Preset é carimbo de uma vez, não
modo guardado: nada novo em `config.json`, nenhum estado "custom", nenhuma regra de precedência —
depois de aplicado vira lista comum e editável, salva pelo botão que já existe.

**Só `codecs` ganha presets.** `resolutions` tem default certo e duas setas na UI; `fansubs` é
gosto, não eixo técnico; `sources` e `audio` não afetam episódio (marcados "só filmes" na tela);
`criteria_order` é arma carregada (#55); `ignore_list` é aditiva, não ranking.

**Descartado: preset "priorizar qualidade".** Para anime em exibição — que é o que
`DownloadStatuses: [CURRENT, REPEATING]` baixa — SubsPlease e Erai-raws fazem remux do stream do
Crunchyroll/HIDIVE, sem reencode: isso já é o teto da fonte, e qualquer arquivo maior do mesmo
episódio é reencode (perda de geração) ou upscale. O primeiro caso já é barrado por `re-encode` no
`ignore_list` default. Para backlog o eixo existe, mas é **fonte** (BD vs WEB), não compressão.

**O teste que fecha a classe do bug:** `TestDefaultPriorities_TokensAreReachable`
(`tests/unit/priorities_test.go`) exige que todo token das listas default seja algo que
`ExtractCodec`/`ExtractResolution` consigam devolver. Foi assim que `x265`/`x264` ficaram anos como
config inerte e invisível (#69); agora quebra o CI.

**Limitação conhecida, deliberada:** nenhuma mudança de default alcança instalação existente —
`LoadConfigs` desserializa por cima de `getDefaultConfig()`. As listas se atualizam pelos botões
"resetar" que já existem na tela; `MaxEpisodeTorrentSizeGB`, `WatchedEpisodesToKeep` e `MinSeeders`
pedem uma edição manual, uma vez. Não há migração automática, pelo mesmo motivo de #47
(`BatchWindowSeconds`): mudar comportamento num update sem ninguém pedir.

**(c) Na tela, tirar um token da lista virou um checkbox, não um `✕`.** Token ausente da lista já
é tratado como o pior (`priorityIndex` devolve `len(list)`), então "remover" nunca foi remover — era
mandar para o fim, com o efeito colateral de o item sumir da tela e não haver como trazê-lo de
volta a não ser digitando o token exato de novo (e digitar errado dá config inerte). Agora cada
linha tem um checkbox: desmarcado sai do array salvo e fica visível, apagado, no fim da lista.

O `✕` sobrou só para o que **não** está no default servido por `GET /config/priorities/defaults`:
o que o usuário digitou e o legado inerte de um `config.json` antigo (`x265`, `4k`, `8k`) — que é
justamente o que precisa de uma forma de sair. E `criteria_order` perdeu o campo de adicionar: é conjunto
fechado — `sortByCriteria` pula em silêncio o critério que não está em `criterionCompare` e não há
validação no `PUT /config` —, então texto livre ali só produzia config inerte, e com o checkbox os
oito critérios já estão sempre na tela, marcados ou não. Não há o que adicionar.

**Don't "fix" by:** devolver a captura crua em `extractResolution` e "arrumar" a ordem da lista;
repor `4k`/`uhd`/`fhd`/`hd`/`8k` no default; criar endpoint de presets; guardar o preset escolhido
em `config.json`; escrever os defaults novos por cima de um `config.json` existente; pôr o `✕` de
volta nos tokens canônicos "porque o checkbox é um clique a mais"; devolver o campo de adicionar a
`criteria_order` (nem como texto livre, nem como select de um enum duplicado no frontend); validar
`criteria_order` no backend para "poder" aceitar texto livre de novo.

---

### 71. A AniList não tem id de franquia: a cadeia de `PREQUEL` é a única fonte, e busca por nome é armadilha

**Location:** `src/internal/anilist` (queries que pedem `relations`); `daemon.ComputeEpisodeOffset`
(`daemon/helpers.go`). Complementa a #9, que trata do *gate* do offset; aqui está a **fonte** dele.

**O que parece:** dá para identificar "a série" — a franquia inteira, atravessando cour e part — por
algum id da AniList, ou, na falta dele, por uma busca por nome (`search: "Monogatari"`).

**Por que está errado — medido por introspecção em 28/ago/2026.** O tipo `Media` tem 55 campos e
**nenhum** de franquia:

```
id, idMal, title, type, format, status, description, startDate, endDate, season, seasonYear,
seasonInt, episodes, duration, chapters, volumes, countryOfOrigin, isLicensed, source, hashtag,
trailer, updatedAt, coverImage, bannerImage, genres, synonyms, averageScore, meanScore,
popularity, isLocked, trending, favourites, tags, relations, characters, staff, studios,
isFavourite, isFavouriteBlocked, isAdult, nextAiringEpisode, airingSchedule, trends,
externalLinks, streamingEpisodes, rankings, mediaListEntry, reviews, recommendations, stats,
siteUrl, autoCreateForumThread, isRecommendationBlocked, isReviewBlocked, modNotes
```

Não existe entidade "série" no schema, e `idMal` tem **a mesma granularidade** (um id por cour). A
única aresta entre cours é `relations`. Caminhar `relationType == "PREQUEL"` filtrando
`format ∈ {TV, TV_SHORT}` não é uma opção entre várias — é a única fonte disponível.

**A busca por nome é o atalho que corrompe tudo.** Seria um request só, e foi medida:

- `search: "Monogatari"` → 28 resultados TV, quase todos de outras obras (`Ore Monogatari!!`,
  `Shouwa Monogatari`, `Perrine Monogatari`, `Gokinjo Monogatari`).
- `search: "Shingeki no Kyojin"` → 7 resultados, incluindo `Shingeki! Kyojin Chuugakkou` — a
  paródia chibi de 12 episódios, que entraria em qualquer soma de episódios e deslocaria a série
  inteira.
- E não devolve **ordem**: a sequência teria que ser inferida dos títulos, que é exatamente o
  problema de parsing de string que já derruba packs no `nyaa_match.go`.

O grafo de `relations` é autoritativo; o nome não é.

**Três fatos de `relations` que definem qualquer implementação:**

1. **`Media.relations` não aceita argumentos.** Sem filtro server-side por `relationType`, sem
   paginação: toda query traz *todos* os edges, e filtrar é sempre trabalho do cliente.
2. **`relations` pode ser aninhado, mas a AniList corta em 2 níveis.** Testado com profundidades de
   1 a 8: o corpo satura em 5865 bytes a partir da profundidade 3, e no terceiro nível `relations`
   vem presente com `edges: []`. Confirmado em Monogatari, Mushoku Tensei e Shingeki: **2 hops de
   PREQUEL por query, sempre.**
3. **`edges: []` no nível cortado NÃO significa raiz.** Esta é a armadilha séria: no nível em que a
   AniList para de expandir, a resposta é indistinguível de um anime que genuinamente não tem
   prequel. Tratar os dois como iguais para a caminhada cedo e produz um deslocamento
   **silenciosamente errado**. Quem implementar precisa marcar quais nodes vieram em posição
   autoritativa (níveis 0 e 1 da query) e re-buscar os do nível cortado antes de concluir qualquer
   coisa sobre eles. O teto de 2 é comportamento observado, não documentado — o loop deve consumir
   o que vier e enfileirar o que faltar, sem assumir profundidade nenhuma.

**A cadeia foi validada contra a API real** (28/ago/2026), e a validação forte é o Shingeki: a
soma acumulada dá `76..87` para o Final Season Part 2, que é **exatamente** a numeração que
SubsPlease e Erai-raws usam nos arquivos. A cadeia reproduz a convenção real de nomeação sem
nenhum ajuste. Monogatari (7 níveis, ordem cronológica torta) também fecha coerente.

**Errar para 0 é barato; errar para um número, não.** Offset ausente cai na numeração relativa, que
boa parte dos grupos usa. Offset errado manda a busca para um episódio que não existe. Toda dúvida
na caminhada resolve para 0.

**Don't "fix" by:** procurar um id de franquia no schema "que deve existir em algum lugar"; usar
`idMal` como identidade de série; substituir a caminhada por `search:` com o nome da franquia;
tratar `edges: []` como raiz; assumir a profundidade 2 como contrato; incluir `MOVIE`/`OVA`/`ONA`
na caminhada (ver #9).

---

### 72. O orçamento da AniList se mede pelos headers da resposta, não por um contador nosso

**Location:** `src/internal/anilist/anilist.go` — `sendAnilistRequest`; `health.go` — `Priority`,
`ErrBudgetLow`, `recordBudget`, `budgetAllows`, `retryAfter`, classificação `HealthRateLimited`.
Fixado por `budget_test.go`. Complementa a #65, que mede **quem** gasta; aqui está **como saber
quanto sobrou**.

**O que parece:** para não tomar 429, o caminho natural é um limiter client-side
(`golang.org/x/time/rate`) ou uma fila que segura requisição quando o consumo estimado passa do
teto.

**Por que está errado.** A AniList devolve o estado do balde em **toda** resposta, e o header é
estritamente mais preciso que qualquer contador interno. Medido em 28/ago/2026:

```
media minimo             HTTP 200  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 29
200 normal               HTTP 200  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 28
404 (media inexistente)  HTTP 404  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 27
query invalida           HTTP 400  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 26
query pesada aninhada    HTTP 200  X-RateLimit-Limit: 30  X-RateLimit-Remaining: 25
```

Quatro consequências, todas com efeito de projeto:

1. **Erro consome cota.** Um 404 e uma query malformada custam 1 cada. Retry cego em cima de erro
   queima orçamento, e um contador que só contasse sucessos ficaria otimista exatamente quando as
   coisas já estão indo mal.
2. **Complexidade não custa nada.** A query aninhada de 33 KB decrementou 1, igual à query mínima.
   Não há cobrança por complexidade nem por número de ids: **maximizar trabalho por request é
   estritamente correto, não há trade-off.** É por isso que `Page(media(id_in: [...]))` (até 50 ids,
   `perPage` máximo 50) vale sempre que houver um loop de busca por id. Aplicado em
   `GetMediaByIDs` (avulsos, `media(id_in:)`) e `GetAnimeInfoByIDs` (refresh de órfãos,
   `mediaList(mediaId_in:)`), ambos em 29/ago/2026.
3. **O header é autoritativo porque enxerga os outros consumidores.** Ele já soma o passe do
   daemon, o poll do frontend e as buscas de avulso — os três disputam o mesmo balde por IP, sem
   coordenação. Nenhum contador interno chega perto, porque nenhum vê os erros dos outros.
4. **O balde reseta de uma vez, não desliza.** Medido: `Remaining` foi de 24 → 20 numa rajada,
   ficou em 19 aos 21s e voltou a **29** aos 41s. É balde fixo com reset discreto, não janela
   deslizante com recarga gradual — na prática, **o orçamento se recupera inteiro em ≤ 60s**.

**`X-RateLimit-Reset` não pode ser usado.** Ele aparece anunciado em
`Access-Control-Expose-Headers`, mas **não está presente** em nenhuma resposta 200 observada. A
tabela da #65 o cita porque a documentação oficial o cita; a medição não o encontrou. Um 429 não
foi provocado de propósito (ele bloqueia o IP por ~1 min e o daemon do usuário compartilha esse IP),
então **quais headers um 429 carrega continua não verificado** — o que se sabe é que `health.go` já
lê `Retry-After`.

**O desenho que decorre disso: reserva por prioridade, não fila.** Gravar `X-RateLimit-Remaining` e
o instante da leitura num atômico ao lado de `health`, em toda resposta **inclusive erro**, e
comparar com um piso antes de emitir. Bloquear e esperar é a resposta errada — o poll do frontend
roda dentro de um handler HTTP e penduraria a tela. A resposta certa é degradar o que é barato
degradar:

| Chamador | Criticidade | Ação com orçamento baixo |
|---|---|---|
| Passe do daemon | alta — perder o passe é episódio não baixado | sempre passa |
| Poll do frontend (`GetFrontendAnimeList` **e** `GetCustomListsMap`) | baixa — já têm cache (60s e 5min) | recusa e serve cache velho — **as duas**, ou a degradação fica assimétrica |
| Busca de avulso pela UI | baixa | recusa, com erro visível a quem pediu |
| Trabalho de warm-up / prefetch | baixa — tem fallback | adia para o próximo passe |

**A saída obrigatória:** se todo mundo for recusado, ninguém emite request e o processo nunca
descobre que o balde resetou. A leitura precisa de validade — passados 60s da última observação,
tratar o orçamento como cheio de novo. É para isso que a medição do reset serve, e é o que impede o
gate de se auto-travar.

**Como ficou implementado** (29/ago/2026). `recordBudget` guarda `X-RateLimit-Remaining` e o
instante num `atomic.Pointer`, logo depois do `httpDo` — antes de qualquer ramo de status, para o
404 e o 400 contarem. `budgetAllows` é o gate, no topo de `sendAnilistRequest`: crítico passa
sempre; descartável passa se a leitura estiver vencida (60s) ou se `remaining >= budgetFloor`
(10 de 30). A recusa é `ErrBudgetLow` na hora — nunca espera.

`Priority` é **parâmetro obrigatório** de `sendAnilistRequest` e sobe até as quatro funções
públicas de chamador misto (`GetCustomListsMap`, `GetAnimeInfo`, `GetMediaByID`, `GetMediaByIDs`),
que o daemon e o frontend chamam com criticidades opostas. As demais fixam a sua no corpo. Não há
default implícito, e essa é a feature: uma query nova não compila sem alguém decidir. A alternativa
descartada era um wrapper com `sendAnilistRequest` virando "prioridade normal" — diff menor, mas
cria exatamente a classe de config inerte da #69.

**A linha divisória não é "daemon vs. frontend", é "se repete sozinho vs. alguém está esperando".**
Descartável é o tráfego recorrente com fallback: o poll de `/animes` (`GetFrontendAnimeList`,
`GetCustomListsMap`, `GetAnimeInfoByIDs`, e o `GetMediaByIDs` de `appendStandaloneEntries` e
`refreshStandaloneOrphans`) e a busca por tecla digitada (`SearchMedia`). Crítico é o passe do
daemon **e** toda ação de um clique — abrir a tela de detalhe, adicionar um avulso, baixar um
episódio à mão: são requisições raras e caras de recusar, e sacrificá-las não devolve orçamento
nenhum. `GetFrontendAnimeList` recusado serve o cache **vencido** (`ttlCache.getStale`) em vez de
derrubar o poll; os outros descartáveis já tratam falha parcial como dado ausente.

**O par do poll degrada junto ou não degrada.** `fetchAniListEntries` é `GetCustomListsMap` +
`GetFrontendAnimeList`, e recusar só um dos dois é pior que recusar os dois: a lista sai do cache
vencido e responde **com sucesso**, o `customLists` volta `nil`, e o merge conclui que nenhum anime
está em lista excluída. `GET /animes` mostra blacklistado como normal e o `standalone_guard` deixa
adicionar como avulso o que deveria recusar — tudo numa resposta que parece saudável. Por isso
`GetCustomListsMap` também serve `getStale` quando o gate recusa
(`TestCustomListsServesStaleCacheWhenRefused`). Sobra a janela em que não há leitura vencida
nenhuma (processo recém-subido com o balde já vazio); aí o `nil` volta e o overlay guard da #11
mantém o que a query complexa tiver trazido. Nulificar a lista inteira nesse caso seria pior: o
`nil` de `GetCustomListsMap` também acontece em falha de rede, e derrubaria a tela por causa da
query barata mesmo com a cara respondendo.

Nenhum chamador de warm-up existe ainda. `GetSeriesIndex` (#77) já recebe a `Priority`, mas
ninguém a chama em produção: o warm-up entra junto com a posse por cobertura, que é quem monta
a semente.

**Don't "fix" by:** adicionar `x/time/rate` ou qualquer token bucket client-side (adivinha em vez de
medir, e não contabiliza os erros dos outros consumidores); contar só respostas 200; depender de
`X-RateLimit-Reset`; bloquear a goroutine esperando o balde encher dentro de um handler HTTP;
recusar *todos* os chamadores quando o orçamento está baixo, sem a validade de 60s; dar um default
à `Priority` (wrapper, valor zero "seguro", ou derivar da query — `GetAllCurrentAnime` e
`GetFrontendAnimeList` podem ser a mesma query com criticidades opostas); usar `getStale` fora do
fallback do gate.

---

### 73. O frontend não busca direto na AniList, mesmo podendo

**Location:** política para `src/internal/frontend` e `src/internal/api/endpoint_animes.go` — hoje
todo acesso à AniList passa pelo backend.

**O que parece:** o limite é por IP público, e o frontend é o maior consumidor (ver #65). Se ele
falasse direto com a AniList, rodando num PC diferente do daemon o consumo cairia noutro balde.

**Os pré-requisitos técnicos existem** — medido em 28/ago/2026, não é o que impede:

- A AniList responde `Access-Control-Allow-Origin: *` e expõe os headers de rate limit em
  `Access-Control-Expose-Headers`. Um browser pode consultar direto, inclusive lendo o orçamento.
- O app **não usa token nenhum**: todas as queries são públicas, por username. Não há credencial
  para vazar para o browser.

**O furo é NAT.** O limite é por IP **público**, não por máquina. Daemon no desktop e browser no
notebook, na mesma casa, saem pelo mesmo IP → mesmo balde, ganho zero. O alívio só existe quando o
browser está numa rede genuinamente diferente (celular em dados móveis, acesso remoto, VPN) — o
cenário menos comum —, e o custo é pago em todos os cenários.

**O custo é duplicação silenciosa.** O frontend teria que replicar em TypeScript o `DedupeByMedia`,
o merge de `customLists`, o `withStandaloneProgress` e a classificação de health do `health.go`.
Lógica de Go duplicada em TS diverge sem ninguém perceber, e o daemon continuaria precisando dos
mesmos dados para o passe: dois caminhos para o mesmo dado, não um.

| | ganho | custo | funciona quando? |
|---|---|---|---|
| Frontend busca direto | move o poll para outro IP | lógica duplicada em TS | só se o browser estiver em outra rede |
| `id_in` em lote nos avulsos | N req/min → 1 | ~10 linhas | sempre |

**Veredito: não fazer.** A ideia aponta para um problema real, mas ataca a fatia errada com o
instrumento caro. **Reavaliar só se**, depois do `id_in` em lote e do gate por prioridade (#72), o
rate limit voltar a apertar.

**Don't "fix" by:** mover só "uma query pequena" para o frontend como meio-termo — o custo é a
duplicação da lógica de merge, e ela começa na primeira query.

---

### 74. A unidade do guard de exclusão de pack é o conteúdo do torrent, não a lista de registros

**Location:** `internal/daemon/episodes.go` (`canRemoveTorrent`, `hasUnclaimedContent`,
`declaredSpan`, `forgettableKeys`, `handleSavedEpisodes`). Fixado por
`TestRemoveEpisodesAndLinks_KeepsPackWithUnrecordedContent`,
`TestRemoveEpisodesAndLinks_RemovesFullyCoveredPack`,
`TestRemoveEpisodesAndLinks_RemovesPackDownloadedMidWatch`,
`TestRemoveEpisodesAndLinks_KeepsRecordsOfHeldPack`,
`TestRemoveEpisodesAndLinks_RemovesPackOnceEveryCourClaimsIt`,
`TestRemoveTorrentWithEpisodes_ForcesPastTheUnclaimedGuard` e
`TestHandleSavedEpisodes_NewEpisodesProtectSharedTorrent` (`internal/daemon/episodes_test.go`).

**What it looks like:** o guard que decide se um torrent compartilhado pode sair não pergunta mais
só "todo registro deste hash está no delete set?". Ele também compara a faixa declarada pelos
registros (`BatchStart`/`BatchEnd`) com os episódios **reivindicados** pelas entradas do grupo, e
recusa a remoção quando sobra conteúdo sem dono (`hasUnclaimedContent`). Três peças fazem a conta
fechar:

- **Quem reivindica é a entrada, pelo `AnimeTotalEpisodes` dela** — não o número de registros.
  Registro só nasce para episódio que o daemon baixou: um pack `01-11` pego com progresso 5 tem 6
  registros, e um episódio bloqueado encurta o grupo do mesmo jeito. Os episódios que a entrada não
  registrou são dela igual. Registro antigo não traz o total (`0`); aí a contagem de registros
  daquele `anime_id` é o que sobra, que é o comportamento anterior a esta correção.
- **`declaredSpan` mede por `anime_id`, não na união** — cada entrada grava a faixa na *sua*
  numeração local (#79), então `1..23` sob o cour 1 e `-10..12` sob o cour 2 são o mesmo pack de 23
  episódios; unir os dois daria 34, um pack que não existe.
- **O registro do pack que ficou no disco não é apagado** (`forgettableKeys`). Ele é o único lugar
  onde a faixa declarada existe — sem ele nenhum cour futuro consegue adotar o pack
  (`findCoveringPack` exige a faixa, #78) e a conta de conteúdo reivindicado nunca mais fecha.

`removeEpisodesAndLinks` ganhou um parâmetro `force` para a exclusão **manual** de torrent
(`RemoveTorrentWithEpisodes`, #32): ali a unidade de exclusão é o torrent e quem decidiu foi o
usuário — sem isso o clique devolveria sucesso sem apagar nada. O passe automático nunca usa
`force`. Além disso, `handleSavedEpisodes` passa `savedEpisodes ∪ newEpisodes` para
`removeEpisodesAndLinks`, não só o snapshot pré-passe.

**Why it's right:** "todo registro" não é "todo o conteúdo". Um pack de **season** baixado sob o
cour 1 (que só tem 11 episódios na AniList) grava 11 registros mas carrega 23 episódios de arquivo
— os 12 do cour 2 não têm registro nenhum porque ainda não foram baixados. Assistir o cour 1
colocava os 11 registros no delete set, o guard concluía "ninguém sobrevive" e apagava do disco um
conteúdo que o usuário nunca viu. O agravante do mesmo passe é a metade do snapshot: quando o cour
2 vira `CURRENT` e o passe escolhe o mesmo pack para o media id novo, `Session.Add` reusa o
infohash e os registros novos existem só em `newEpisodes` — invisíveis para o guard, que apagava o
arquivo recém-adotado.

**Por que não a contagem de registros** (foi a primeira versão desta decisão, e vazava disco): ela
confunde "episódio de outro cour, que ninguém baixou" com "episódio desta entrada que o daemon não
precisou registrar". Um pack `01-11` de uma entrada de 11 episódios, pego com progresso 5, tem 6
registros; `6 < 11` marcava conteúdo sem dono **para sempre**, o torrent nunca saía do disco e os
registros eram apagados assim mesmo — um pack órfão permanente, que é o oposto da proteção que o
guard existe para dar. Comparar os **números** dos episódios também não serve: o mesmo hash pode
ter registros de media ids diferentes, cada um na sua régua. O total da entrada é o único dado que
diz quanto do pack pertence a ela sem precisar de conversão.

**Teto conhecido (`ponytail:` no código):** pack cuja faixa nem o nome nem a lista de arquivos do
Nyaa resolveram grava `BatchEnd == 0` (desconhecida, ver `hasDeclaredRange` em #79) e continua
indetectável — o guard cai no comportamento antigo. Depois de #84 isso é o resto de um caso raro
(detalhe fora do ar, nome de arquivo ilegível); antes era **todo** pack sem faixa no nome.

**Don't "fix" by:** derivar a faixa do min/max dos episódios salvos (é a informação que falta, não
uma que dá para reconstruir — foi o bug que motivou `BatchStart`/`BatchEnd` existirem), nem
recarregar `episodes.json` dentro de `removeEpisodesAndLinks` (a Fase 3 é sequencial de propósito e
já tem os dois pedaços em memória).

---

### 75. O corte no marcador e a primeira tentativa do match de titulo, nao a unica

**Location:** `internal/nyaa/nyaa_match.go` (`titleMatchesQuery`, `altTitleCandidates`,
`tokensMatchQuery`, `firstMarkerIndex`). Fixado por `TestTitleMatchesQuery_AltTitleAfterMarker` e
`TestTitleMatchesQuery_AltTitleDoesNotAcceptWrongAnime` (`src/tests/unit/nyaa_test.go`), com nomes
reais colhidos do Nyaa.

**What it looks like:** `titleMatchesQuery` roda as duas checagens (todos os tokens da query
presentes + Jaccard) primeiro sobre o nome cortado no marcador — igualzinho a #18 — e, so se isso
falhar, repete sobre **cada** titulo alternativo que aparece DEPOIS do marcador, um de cada vez.
Cada trecho entre parenteses e cada segmento separado por barra e um candidato proprio.

**Why it's right:** a premissa de #18 ("tudo depois do marcador e ruido") quebra em uma familia
inteira de nomes. O EMBER escreve
`[EMBER] Mushoku Tensei: Jobless Reincarnation (2024) (Season 2 | Part 2) [...] (Mushoku Tensei II:
Isekai Ittara Honki Dasu Part 2) (Batch)`: o marcador `Season 2` esta DENTRO de um grupo, e o romaji
completo vem tres grupos adiante. Cortar deixava `[mushoku tensei jobless reincarnation]`, e a query
em romaji era rejeitada por falta de token — os UNICOS packs de Part 2 do anime morriam antes de
chegar ao caminho de batch. Diddy, NTRX, Fuchs e o VARYG fazem o mesmo com barra em vez de
parenteses.

**Por que um candidato por trecho, e nao um saco unico de tokens.** Foi a primeira tentativa, e
medindo ficou pior: juntar o titulo em ingles do prefixo com o romaji do rodape dilui os dois
(Jaccard 0.78 no caso do EMBER, contra **1.00** do romaji comparado sozinho), e no `debug-batch`
isso deixava um trecho emprestar token para o vizinho — os avulsos do Yameii de *SAO Alternative:
Gun Gale Online* passavam a casar a query de *Sword Art Online II*. Comparado trecho a trecho, cada
titulo e avaliado como titulo.

**Por que a segunda tentativa so roda depois da primeira falhar.** E o que preserva #18 inteiro.
O corte e o que protege query curta de afundar no Jaccard: o pack
`[EMBER] Kimetsu no Yaiba (2024) (Season 4) [...] (Kimetsu no Yaiba: Hashira Geiko-hen | Demon
Slayer: Kimetsu no Yaiba Hashira Training Arc) (Batch)` casa a query de 3 tokens no corte (1.00) e
nunca chega aos candidatos — onde marcaria 0.5 e seria rejeitado. Ampliar os tokens para todo mundo
trocaria um falso negativo por outro.

**Medido no `make debug-batch`** (corpus de `scripts/robustness-animes.txt`, antes e depois): nenhum
match perdido; ganhos em `108465`, onde o pack BDRip do EMBER de season 1 passou a casar e virou a
melhor opcao (492 seeders, contra 65 do Judas que liderava). No `166873`, que motivou a
investigacao, os tres packs de Part 2 do EMBER passaram a sobreviver ao filtro (eram zero).

**Limitacao conhecida, PRE-EXISTENTE e nao introduzida aqui:** a query `Sword Art Online II` casa os
avulsos de *SAO Alternative: Gun Gale Online II* (Jaccard 0.57, acima do 0.4 de query longa). Os
releases do Erai-raws desse mesmo spinoff ja casavam antes desta mudanca, pelo caminho do corte —
o que mudou foi so a consistencia entre releases. Consertar isso e mexer no `jaccardThreshold`, que
e outra feature.

**Don't "fix" by:** tokenizar o nome inteiro (traz titulo de episodio e tag de release, e afunda o
Jaccard), nem juntar os candidatos num conjunto so (medido acima), nem criar um threshold proprio
para a segunda tentativa (numero magico ajustado a cinco amostras).

---

### 76. `searchNyaaWithVariants` para na primeira variante que devolve qualquer resultado — medido, e fica assim

**Location:** `internal/daemon/search.go` (`searchNyaaWithVariants`, `buildTitleVariants`).

**What it looks like:** o laço de variantes de título só continua para a próxima quando a atual
devolveu **nada** (`result != nil` já encerra). Uma variante que devolve 24 episódios avulsos e zero
pack encerra a busca — a variante em inglês, que poderia trazer o pack, não roda.

**Why it's right:** a falha descrita acima é real no papel, e chegou a ser agendada como feature
("F3 · Variantes de título no caminho de pack"). Foi **medida** depois de #75 e não aparece na
prática: rodando `ScrapNyaaForAnime` + `partitionSearchResults` + `pickBatches` para **cada** variante
dos 8 animes de `scripts/robustness-animes.txt` mais `146065` e `166873` (29/ago/2026), não há um
único anime em que a variante 1 devolva resultado e uma variante posterior encontre pack que ela não
encontrou. Onde existe pack no Nyaa, o romaji já o acha:

| id | v1 (romaji) packs/batches | melhor variante posterior |
|---|---|---|
| 1735 Naruto Shippuuden | 1 / 1 | idem (pack diferente, mesma cobertura) |
| 1 Cowboy Bebop | 9 / 1 | variante única |
| 108465 Mushoku Tensei | 3 / 1 | idem |
| 20594 Sword Art Online II | 4 / 1 | variante única |
| 101922 Kimetsu no Yaiba | 3 / 2 | 14 / 1 (mais packs, cobertura pior) |
| 166873 Mushoku Tensei II Part 2 | 3 / 1 | idem |
| 235 Meitantei Conan | 0 / 0 | **0 / 0** em todas as quatro |

O `235` é o único anime do corpus que cai no caminho de episódio avulso, e nenhuma variante acha
pack para ele — o Nyaa não tem pack dos primeiros episódios de Detective Conan. É ausência de
conteúdo, não de variante.

Depois de #75, o custo de rodar a variante seguinte deixou de ter contrapartida: seria mais uma
rodada de busca no Nyaa **por anime, em todo passe** em que não houver pack — que é a maioria dos
animes em exibição, porque pack de temporada corrente só aparece no fim. Combinado com a paginação
adaptativa (#57), multiplica.

**Reabrir quando:** aparecer no `make debug-batch` um anime cuja variante 1 devolva resultado, zero
pack sobreviva ao filtro, e a busca manual com a variante em inglês mostre pack que cobre a janela.
Aí a mudança é de duas linhas: continuar o laço quando `partitionSearchResults` não devolver pack.

**Don't "fix" by:** rodar todas as variantes sempre e unir os resultados (paga N buscas por anime
para o ganho medido acima, que é zero), nem inverter a ordem das variantes (`GenerateSearchTitleVariants`
põe o romaji primeiro porque é ele que casa a maioria dos releases).

---

### 77. O eixo absoluto por série é um BFS de duas em duas gerações, e o nível cortado volta para a fila

**Location:** `src/internal/anilist/series.go` — `Series`, `GetSeriesIndex`, `walkSeries`,
`recordLink`, `prequelOf`, `resolveSeries`, `seriesCache`. Travado por `series_test.go`.
Implementa o que a #71 mediu; a #9 é o *gate* do offset de uma part só.

**O que parece:** para saber o episódio absoluto de um cour basta ler `relations` do anime e
somar os episódios do `PREQUEL` — que é o que `daemon.ComputeEpisodeOffset` faz.

**Por que não basta.** `ComputeEpisodeOffset` anda **um** salto e só quando o título declara
`Part >= 2`. Isso cobre `Shingeki no Kyojin: The Final Season Part 2` → `+16`, mas não a série:
o absoluto real desse cour é `76..87`, que exige somar as **cinco** entradas anteriores. Como uma
query da AniList expande no máximo 2 níveis de `relations` (#71), resolver uma cadeia de 7 níveis
não é uma leitura — é uma caminhada.

**O desenho, em três regras:**

1. **Cada request anda duas gerações, não uma.** A resposta de um media traz o `relations` dele
   (nível 0) e o do prequel dele (nível 1), ambos autoritativos. Daí saem **dois** elos gravados e
   **três** contagens de episódio por media buscada. `Page(media(id_in: [...]))` com até 50 ids
   custa a mesma unidade de orçamento que um id só (#72), então a largura é de graça e a única
   dimensão que custa é a altura: `ceil(altura/2)` rodadas.
2. **O avô volta para a fila.** O nó do nível 2 vem com id, formato e contagem confiáveis, mas seu
   `relations` chega como `edges: []` — o corte, indistinguível de "não tem prequel" (#71).
   Gravá-lo como raiz produz offset **silenciosamente errado**: no fixture do Shingeki, o Final
   Season Part 2 daria `38..49` em vez de `76..87`. Por isso `recordLink` só é chamada nos níveis
   0 e 1, e o nível 2 é reenfileirado. Só nó gravado por `recordLink` entra no cache — um nó de
   nível 2 sabe quantos episódios tem, mas não quem vem antes dele, e meio fato não é fato.
3. **O loop é agnóstico à profundidade.** O teto de 2 é comportamento observado, não contrato:
   consome o que vier, enfileira o que faltar. Uma AniList que passe a expandir 3 níveis só faria
   a caminhada terminar em menos rodadas, sem mudar uma linha.

**O cache é de 24h porque o dado é imutável e a caminhada é monotônica.** Só ancestrais entram, e
ancestral de anime terminado também terminou: um `FINISHED` com contagem de episódios não ganha
episódio nem prequel novo. Nó `RELEASING` ou sem contagem (`episodes: null`, não lançado) conta
como 0 e **não** é persistido — congelar esse zero por 24h deslocaria a série inteira para baixo
no dia em que a contagem aparecesse. Em memória, sem arquivo: o warm-up inteiro custa uns poucos
requests, então persistir em disco seria invalidação e migração a troco de nada.

**Ausente ≠ raiz, nos dois sentidos.** Id que a AniList não devolve fica **fora** do mapa (mesmo
contrato de `GetMediaByIDs`), e ancestral desconhecido no meio da cadeia para a soma ali. As duas
escolhas erram para o **menor** offset de propósito: offset ausente cai na numeração relativa,
que boa parte dos grupos usa; offset errado manda a busca para um episódio que não existe (#71).

**As cadeias medidas viraram fixture de teste, não markdown** — Mushoku Tensei (4 níveis),
Shingeki no Kyojin (6) e Monogatari (7). A validação forte é o Shingeki: `76..87` é exatamente a
numeração que SubsPlease e Erai-raws usam nos arquivos. O teste semeia **só as pontas** de cada
cadeia: semear todos os ids faria cada ancestral chegar como nível 0 e o corte nunca seria
exercitado — verificado por mutação, a versão anterior do teste passava com o bug da regra 2.

**Don't "fix" by:** tratar `edges: []` como raiz; assumir profundidade 2 como contrato; buscar um
ancestral por request (a altura vira o custo, e o orçamento da AniList é 30/min); cachear nó de
nível 2, `RELEASING` ou sem contagem; persistir o offset em disco ou no registro do episódio (ver
a alternativa descartada na #78 — valor persistido apodrece quando a AniList corrige uma contagem,
e o derivado se autocorrige no TTL seguinte); trocar a caminhada por `search:` com o nome da
franquia (#71).

### 78. A unidade de posse de um torrent é a cobertura no eixo absoluto, não a chave `(anime_id, episódio)`

**Location:** `src/internal/daemon/coverage.go` — `resolveSeriesIndex`, `adoptCoveredEpisodes`,
`findCoveringPack`, `dropAdopted`; `packDisplayName` (`episodes.go`), compartilhado com
`assignBatches`; chamada em `processAnimeEpisodes` (`episodes.go`) e em
`DownloadStandaloneAnime` (`standalone.go`). Travado por `coverage_test.go`. Consome a #77; é o
outro lado da #74.

**O problema.** A AniList quebra cour e part em **media ids separados**, cada um numerando a
partir de 1; os packs do Nyaa são quase sempre da **season inteira**, atravessando essa fronteira.
Com a posse expressa como `files.EpisodeKey{AnimeID, Episode}`, o pack de `1..23` baixado sob o
cour 1 (11 episódios) é invisível para o cour 2 — que não tem registro nenhum, não sabe que o
arquivo já está em disco, e rebaixa a season inteira do Nyaa. Caso real: Mushoku Tensei S1.

**A regra:** antes de qualquer busca, um episódio pendente é adotado quando existe registro de
pack, **em qualquer `anime_id`**, cuja faixa declarada contenha o episódio **no eixo absoluto da
série**. `abs = Series.Offset(anime_id) + número local` (#77) — comparar `1..23` com `1..12` sem
converter é comparar réguas diferentes.

**Adotar é gravar o registro apontando para o hash que já existe**, exatamente o que aconteceria
se a busca reencontrasse o mesmo torrent (`Session.Add` reusa o infohash). Isso é o que faz o
resto da máquina funcionar sem nenhuma exceção nova: a #74 passa a enxergar os irmãos do outro
cour, e `organizeTorrent` hardlinka os arquivos que nunca tiveram dono — ele já trata grupo
parcialmente organizado sem re-notificar. A faixa é **copiada como o dono a declara** (é a do nome
do torrent) e **convertida para a numeração local de quem adota** — `findCoveringPack` devolve o
offset do dono junto com o registro justamente para isso. Todo registro guarda a faixa na régua da
sua própria entrada (#79) e é assim que ela é lida de volta; copiar `1..23` do cour 1 para debaixo
do cour 2 (offset 11) faria esse registro ser lido como o absoluto `12..34`, e um cour 3 acharia
cobertura para episódio que o pack não tem — marcado como baixado, nunca buscado, arquivo
inexistente. Pinado por `TestCoverageOwnership_AdoptedRangeDoesNotShiftForTheNextCour`.

**A semente do índice inclui os `anime_id` de `episodes.json`, não só os do passe.** O cour
anterior está `COMPLETED` e por isso saiu do universo do passe — e é o offset **dele** que
converte a faixa do pack. Sem essa semente a feature não enxergaria o pack que a motivou. Custa
quase nada: largura é grátis (`perPage` 50, uma query = uma unidade — #72) e boa parte da semente
extra já entraria na caminhada por ser ancestral dos animes do passe.

**Quatro portas, todas obrigatórias** — cada uma cobre um jeito diferente de adotar errado:

| Porta | Sem ela |
|---|---|
| Faixa declarada (`BatchStart > 0`) | pack sem faixa no nome grava `0`, e desconhecida viraria "cobre tudo" — mesmo teto da #74 |
| Torrent ainda na sessão | registro com hash morto: episódio com dono e sem arquivo, invisível para a redescida |
| Mesma `Series.Key` | "absoluto 12" casaria com o pack de qualquer outro anime |
| Não é filme | `prequelOf` só segue `TV`/`TV_SHORT`, então um filme pós-season herda o offset dela e o "episódio 1" dele cai dentro da faixa do pack. Cobertura por range não distingue os dois |

**O filtro roda nos DOIS pontos de seleção.** `selectEpisodes` é pura e não sabe da adoção, então
a segunda passada (limite levantado, caminho de pack) traria os adotados de volta e o daemon
baixaria o que acabou de adotar.

**Alternativa descartada: persistir `AbsStart`/`AbsEnd` no `EpisodeStruct` na hora do download.**
O lookup ficaria local e offline, mas todo registro anterior à feature nasceria com `0` —
indistinguível de "desconhecido" —, ou seja, ela **não resolveria o caso que a motivou**, só
valeria para o que fosse baixado depois; e o backfill precisaria da derivação de qualquer forma.
Pior: valor persistido apodrece. Se a AniList corrigir uma contagem de episódios ou adicionar uma
relação que faltava, o número gravado fica errado para sempre; o derivado se autocorrige no TTL
seguinte. Guardar como fonte da verdade um número derivável cria uma segunda verdade que ninguém
invalida. **Reabrir só se** o lookup aparecer como custo **medido** — e aí ele vira cache
derivado, nunca fonte.

**Don't "fix" by:** comparar `BatchStart`/`BatchEnd` de media ids diferentes sem converter;
converter a faixa gravada para a numeração local do adotante (quebra `declaredSpan`); adotar com
faixa desconhecida, com o torrent fora da sessão ou sem checar `Series.Key`; mover a adoção para
dentro de `selectEpisodes` (ela roda duas vezes — duplicaria os registros); notificar
`NewEpisode`/`DownloadCompleted` numa adoção (nada foi baixado, e o webhook de conclusão já saiu
quando o pack pousou).

### 79. A escolha de pack pergunta "cobre a janela?", não "é da part N?" — e a numeração do pack é palpite entre três hipóteses

**Location:** `src/internal/daemon/episodes.go` — `packAxis`, `newPackAxis`, `packAxis.localRange`,
`coveringBatch`, `pickBatches`, `assignBatches`, `hasDeclaredRange`, `declaredSpan`;
`src/internal/nyaa/nyaa.go` — `extractPart`, `declaredParts` e o ramo `isBatch` de
`ScrapNyaaForAnime`. Travado por `packaxis_test.go` e pelas tabelas de `nyaa_test.go`. Consome a
#77 (eixo absoluto) e é o outro lado da #78 (posse por cobertura).

**O problema.** A AniList quebra a season em cours/parts, cada entrada numerando a partir de 1; os
grupos lançam o pack da **season inteira** e cada um escolhe a régua que quiser (`sources.md`,
"Granularidade e numeração dos packs": reinicia por season, reinicia por part, contínua pela
season, absoluta pela série). Duas consequências, as duas medidas em Mushoku Tensei:

1. **O filtro duro de part zerava os packs de toda entrada "Part N".** Com `requestedPart != nil`,
   pack **sem** marcador de part era descartado — e pack de season inteira normalmente não tem esse
   marcador, embora contenha os episódios pedidos.
2. **A faixa do nome era lida na régua errada.** `01 ~ 23` sob uma entrada de 12 episódios era
   comparada com `1..12` como se fosse a mesma numeração.

**A regra:** no caminho de pack, `part` só rejeita **conflito declarado** (o pack diz "Part 1" e
queremos a 2). Sem marcador, o pack passa e quem decide é a **cobertura da faixa**. O caminho de
episódio mantém o filtro duro: ali o marcador é o único jeito de distinguir o "- 05" de uma part do
"- 05" da outra.

**Teto conhecido — pack sem marcador de part E sem faixa no nome não é decidido por ninguém.**
`coveringBatch` devolve esse pack sem consultar o eixo (não há número para converter) e ele vale
como completo, então "quem decide é a cobertura" vale para o pack **com** faixa, não para esse.
Um `[Group] Anime S4 [Batch]` que na prática é só a Part 1 passa igual ao pack da season inteira —
antes desta decisão ele era rejeitado no scraper, e rejeitá-lo era pior: zerava os packs de toda
entrada "Part N", que é o problema 1 acima. Aceitar é a escolha medida; o desempate que falta é a
**lista de arquivos da página de detalhe do Nyaa** (item do `docs/TODO.md`), que diz quantos
arquivos o pack tem e com que numeração. Enquanto ela não existir, a ambiguidade fica.

**`packAxis` são três hipóteses e um desempate.** Como não dá para ler a convenção do nome, o
daemon testa quanto somar ao número local para chegar à régua do grupo: **0** (relativa à entrada),
**`ComputeEpisodeOffset`** (contínua pela season — o total do prequel imediato) e
**`anilist.Series.Offset`** (absoluta pela série, #77). Só concorre hipótese que de fato **cobre o
episódio pedido**; entre as que cobrem ganha a que faz o pack **terminar no último episódio da
entrada**. Esse desempate é a "contagem de arquivos" de `sources.md` expressa pelo span do nome: um
pack de 25 episódios sob uma entrada de 12 não pode ser local, um de 12 sob a mesma entrada não pode
ser outra coisa. Na dúvida (nenhuma bate o fim), vale o delta 0: offset ausente cai na numeração
relativa, que boa parte dos grupos usa, enquanto offset errado escolhe um pack que não tem o
episódio.

**A faixa GRAVADA é a convertida, e pode começar em zero ou abaixo.** Um pack de season baixado sob
o cour 2 começa antes do episódio 1 daquela entrada: `-12..12` é a representação correta, e é ela
que faz `owner.Offset + BatchStart` (#78) devolver a faixa absoluta certa e `declaredSpan` (#74)
contar os 25 episódios de conteúdo do pack. Por isso o sentinela de "faixa desconhecida" passou a
ser o **fim** (`hasDeclaredRange`: `BatchEnd > 0`) e não mais `BatchStart <= 0` — que já mentia
sobre `[Erai-raws] … - 00 ~ 12`. Só o **nome exibido** corta o começo em 1: "-10-12" não diz nada
na tela, e o que interessa ali é a fatia desta entrada.

**`extractPart` devolve `nil` para nome com duas parts.** "(Part 1 + Part 2)", "(Part 1+2)" e
"(Season 4 Part 03+04)" cobrem as duas metades; devolver o primeiro número fazia o pack ser
rejeitado justamente para a segunda. Sem número único, quem decide é a cobertura.

**As duas seleções de "primeiro PREQUEL de TV/TV_SHORT" continuam duplicadas de propósito.**
`anilist.prequelOf` caminha a cadeia **inteira** (hipótese "absoluta pela série");
`daemon.ComputeEpisodeOffset` dá **um** salto e é gated por `part >= 2` (hipótese "contínua pela
season"). São hipóteses diferentes do mesmo palpite — unificá-las apagaria a distinção que a
heurística usa. Os `TODO(F8)` que apontavam um para o outro viraram esta nota.

**Medição (29/ago/2026).** `make debug-batch` sobre `scripts/robustness-animes.txt`: episódios
buscados e encontrados idênticos aos da rodada anterior nos 8 animes (só seeders variaram, dado
vivo). Nos casos motivadores, todos com 12/12 magnets: `166873` e `127720` (Mushoku Tensei) e
`131681` (AoT Final Season Part 2), onde o pack `[EMBER] … (Season 4 Part 03+04)` passou a
sobreviver ao filtro.

**Don't "fix" by:** voltar a exigir marcador de part no caminho de pack; relaxar o filtro de part
no caminho de **episódio**; tratar começo de faixa ≤ 0 como desconhecido; converter a faixa gravada
"para ficar positiva" (quebra #74 e #78); acrescentar hipótese de numeração sem desempate que a
distinga das outras; unificar `prequelOf` com `ComputeEpisodeOffset`.

---

### 83. A faixa do nome do pack é o primeiro casamento plausível, não o primeiro casamento

**Location:** `nyaa/nyaa.go` — `batchRange`, chamado por `extractBatchInfo`; `nyaa/nyaa_regex.go` — `reBatchRange`.

**What it looks like:** um `FindAll` com `continue` em três guardas onde caberia um `FindString` com um `if`, e uma banda de números mágicos (1900..2100) no meio. Parece defesa especulativa contra casos que não acontecem.

**Why it's right:** `reBatchRange` é `(\d{1,4})\s*[-~]\s*(\d{1,4})` — ele casa **qualquer** par de números ligados por `-` ou `~`, e no nome de um release a maioria desses pares não é episódio. Os três medidos:

| Nome | Casamento | O que é |
|---|---|---|
| `[Group] Anime Complete Batch [720-1080p]` | `720-1080` | resolução |
| `[Erai-raws] Hibike! Euphonium 2 - 01 ~ 13 …` | `2 - 01` | temporada + primeiro episódio |
| `[EMBER] Mushoku Tensei … (2021-2022) (Season 1 …)` | `2021-2022` | ano de lançamento |

O ano é o que forçou a mudança: ele **não** cai no fallback seguro. `EndEpisode == 0` significa "faixa desconhecida" e todo chamador trata como pack completo; `2021..2022` é uma faixa *declarada* que nenhuma hipótese de `packAxis` cobre, então `coveringBatch` rejeita o pack e ele some da escolha sem uma linha de log que explique. Era um pack BDRip completo, primeiro colocado por seeders, descartado por um parêntese.

A banda `start >= 1900 && last <= 2100` só pega ano: nenhuma série chega ao episódio 1900, e `One Piece 1001-1100` — o caso real que forçou `\d{1,4}` em [#51](#51-episódio-aceita-4-dígitos-exceto-entre-colchetes) — passa intacto.

**`FindAll` e não `FindString`** porque um casamento reprovado não deve encerrar a busca: em `[Group] Anime (2021-2022) 01-24 [BD]` a faixa de verdade vem **depois** do ano.

**Trade-off aceito:** `FindAll` é **não-sobreposto**, então o Hibike continua sem faixa. O casamento `2 - 01` consome o `01`, e o `01 ~ 13` seguinte nunca é visto. Fica como está de propósito: varrer com sobreposição (reiniciar um caractere depois de cada rejeição) produz sub-casamentos lixo — `2021-2022` rejeitado viraria `021-2022`, que passa na banda e grava a faixa 21..2022. Um pack sem faixa cai no caminho seguro; um pack com faixa errada mente para a posse por cobertura ([#78](#78-a-unidade-de-posse-de-um-torrent-é-a-cobertura-no-eixo-absoluto-não-a-chave-anime_id-episódio)).

**Don't "fix" by:**
- Trocar por um sanitizador de tokens que remove `[...]`/`(...)` antes de casar: o ano vem entre parênteses, mas `(001-100)` também — é onde metade dos grupos põe a faixa de verdade.
- Estreitar a banda de ano para "só 19xx/20xx com diferença <= 5": a diferença entre os anos não distingue nada que a banda já não distinga, e vira mais um número mágico.
- Varrer com sobreposição para salvar o Hibike (ver trade-off).

---

### 84. A cobertura de um pack sem faixa no nome vem da lista de arquivos, e não da suposição de que ele cobre tudo

**Location:** `nyaa/nyaa.go` — `PackFileRange`, `TorrentResult.DetailURL`, `detailURL`, `fileListEntryName`; `daemon/episodes.go` — `packCandidate`, `packSet`, `maxPackDetailFetches`, `pickBatches`, `assignBatches`; `daemon/search.go` — `nyaaSearcher.packRange`.

**What it looks like:** a escolha de pack faz uma requisição HTTP a mais, no meio de uma função que antes era pura, e a faixa de um pack passou a ser resolvida preguiçosamente por um tipo com estado (`packSet`) em vez de sair de uma chamada a `ExtractBatchInfo`.

**Why it's right:** o nome do pack quase nunca traz a faixa. Medido em 29/ago/2026 nas 8 primeiras linhas de `?q=<anime>+batch&s=seeders` para três animes: **19 de 24 packs não declaram faixa nenhuma**. Isso não era o caso raro — era o caso comum, e o caminho que ele seguia gravava dado errado no `episodes.json`:

`EndEpisode == 0` valia como "pack completo", `pickBatches` parava o cursor ali, `assignBatches` gravava `1..total` e **todo** episódio da janela virava um `EpisodeStruct` apontando para o hash do pack. Os que o pack não tem ficam registrados como baixados para sempre: o Librarian linka só os arquivos que existem e nada reconcilia registro com disco depois, então eles nunca voltam para a busca.

O caso que forçou a mudança, medido em `nyaa.si/view/1891610`: `[EMBER] … (Season 2 | Part 2) … (Batch)` não declara faixa e traz os arquivos **13..24**. Sob a entrada Part 1 o daemon registrava 1..12 — interseção **zero** com o que o torrent contém.

A lista de arquivos da página de detalhe responde antes de baixar, e responde bem: nos 7 packs medidos, 574/574, 24/24, 23/23, 12/12 e 103/111 arquivos deram o número (os 8 que faltaram são OP/ED, que devem mesmo ficar de fora). Ela vem do próprio `.torrent`, então funciona em conteúdo antigo — ao contrário do `files[]` do AnimeTosho (ver `sources.md`).

**A faixa dos arquivos entra pelo mesmo `packAxis` da faixa do nome**, porque os grupos usam as mesmas quatro convenções de numeração nos dois lugares (`sources.md`, "Granularidade e numeração dos packs"). No caso do EMBER: 13..24, delta 12 da hipótese "continua pela season", vira 1..12 local.

**Por que sob demanda e com orçamento.** A página de detalhe custa ~30 KB (medido em 7 packs) contra ~125 KB de uma listagem — não é banda, é número de requisições contra o nyaa.si. `packSet.rangeOf` só busca quando a faixa é pedida para um pack sem faixa no nome, e memoriza inclusive a falha:

- resolver tudo de uma vez gastaria 3 requisições por anime por passe mesmo quando o primeiro pack já declara a faixa e cobre a janela — o caso mais comum;
- sem memória da falha, `assignBatches` (que varre os packs uma vez **por episódio**) viraria uma requisição por episódio.

`maxPackDetailFetches = 3` porque os packs vêm ordenados por qualidade e `pickBatches` raramente passa dos primeiros para fechar a janela.

**Faixa não resolvida grava ZERO, não `1..total`.** Chegar a `assignBatches` sem faixa significa que as duas fontes falharam (detalhe fora do ar, torrent de arquivo único, nenhum nome legível). O pack ainda é usado — recusá-lo faria um Nyaa fora do ar zerar os packs de todo mundo —, mas `1..total` era um palpite que `hasDeclaredRange` lia como faixa **declarada**, e dali a posse por cobertura ([#78](#78-a-unidade-de-posse-de-um-torrent-é-a-cobertura-no-eixo-absoluto-não-a-chave-anime_id-episódio)) adotava, sob outro cour da série, episódio que o pack podia não ter. "Não sei" é o único registro honesto.

**Trade-off aceito:** com faixa zero o `hasUnclaimedContent` de [#78](#78-a-unidade-de-posse-de-um-torrent-é-a-cobertura-no-eixo-absoluto-não-a-chave-anime_id-episódio) não tem span para comparar e o guard de deleção cai no comportamento antigo naquele grupo. É a troca certa: perder uma proteção contra deleção prematura num caso raro vale menos que espalhar uma cobertura inventada para os outros cours da série.

**Teto conhecido (`ponytail:` no código):** a faixa é o `min..max` dos números lidos. Pack cujos arquivos **reiniciam** a numeração por season (S01-S04, uma pasta cada) fica com a faixa da maior season em vez do total — o mesmo resultado de antes, sem piora. O desempate seria a contagem de arquivos por pasta, que [#79](#79-a-escolha-de-pack-pergunta-cobre-a-janela-não-é-da-part-n--e-a-numeração-do-pack-é-palpite-entre-três-hipóteses) já queria; entra quando aparecer medido.

**Don't "fix" by:**
- Buscar o detalhe dentro do `parseRow` das buscas: vira uma requisição **por linha** da listagem. É por isso que `DetailURL` só é capturada ali e a leitura acontece depois do filtro e da ordenação.
- Resolver a faixa de todos os packs de uma vez "para simplificar o `packSet`": gasta o orçamento inteiro em packs que nunca seriam escolhidos.
- Recusar o pack cuja faixa não foi resolvida: um Nyaa fora do ar deixaria de baixar pack nenhum, trocando um registro errado por nenhum download.
- Voltar a gravar `1..total` "porque o pack foi escolhido como completo": era exatamente o palpite que alimentava a adoção por cobertura com faixa inventada.
- Filtrar os arquivos por extensão numa lista própria do pacote `files`: `nyaa.IsVideoFile` é a mesma lista que o Librarian usa depois de baixar, e `files` importa `nyaa` (o contrário daria ciclo).
