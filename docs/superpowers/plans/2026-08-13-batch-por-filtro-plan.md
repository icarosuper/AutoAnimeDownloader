# Batch por filtro de busca — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Trocar a elegibilidade a batch de "metadado do AniList" para "o que a busca filtrada no Nyaa devolveu", de forma que pack parcial de série longa (One Piece `001-100`) vire candidato seguro, com rodízio de packs sucessivos.

**Architecture:** O laço de seleção de episódios sai de `processAnimeEpisodes` como função pura (`selectEpisodes`) e passa a ser chamado duas vezes: uma com o limite por anime valendo (é ela que produz as deleções corretas) e, só quando um pack foi de fato escolhido, outra com o limite levantado. A escolha de packs (`pickBatches`) roda sobre o resultado **já filtrado** por tamanho e seeders, cobrindo a janela `[primeiroPendente, primeiroPendente + max_episodes_per_anime - 1]`. As duas buscas do Nyaa que montavam a mesma URL (`ScrapNyaaForBatch` e `ScrapNyaaForMultipleEpisodes`) viram uma (`ScrapNyaaForAnime`), particionada no daemon. Episódio de pack fica imune à poda por limite e ao keep-set de assistidos, e é isso que dá o rodízio sem config nova. Anime avulso ganha progresso editável em `AnimeSettings`, injetado no `MediaList` sintético — sem nenhum `if isStandalone` novo.

**Tech Stack:** Go 1.x (daemon, `net/http`, `goquery`), Svelte 5 + Vite + Paraglide i18n (frontend), vitest (unit/component), swag (Swagger).

## Global Constraints

- Spec de origem: [`docs/superpowers/specs/2026-08-13-batch-por-filtro-design.md`](../specs/2026-08-13-batch-por-filtro-design.md). Ela substitui a regra batch↔limite de `2026-08-10-limite-episodios-batch-design.md` apenas na parte que decide elegibilidade **antes** de buscar.
- Repo root: `/home/icaro/Projetos/AAD/AutoAnimeDownloader`. Import path dos pacotes Go: `AutoAnimeDownloader/src/internal/<pkg>`.
- **`go test ./...`** (na raiz) tem de passar ao fim de CADA task. Frontend: `cd src/internal/frontend && bun run test:unit && bun run test:component`.
- **NUNCA commitar ou dar push sem permissão explícita do usuário** (CLAUDE.md). Os passos "Commit" abaixo significam: deixe o `git add` feito e **pergunte** ao usuário antes do `git commit`. Se o usuário já autorizou commits nesta sessão, commite com a mensagem indicada.
- Comentários novos em Go seguem o estilo dos arquivos vizinhos: português **sem acentuação** em `daemon/` e `nyaa/`, explicando *por quê*, não *o quê*. Não remova comentário existente que continue verdadeiro.
- Nenhum arquivo novo de produção, nenhuma dependência nova. Arquivos de teste novos são permitidos.
- Nenhuma migração de `config.json`: `encoding/json` ignora campo desconhecido.
- Um `ponytail:` comment é obrigatório em cada simplificação deliberada com teto conhecido (os dois pontos estão marcados nas tasks 2 e 3).

## Desvios da spec (decididos aqui, com o motivo)

1. **`Status.svelte:774,814` não muda.** A spec diz que esses pontos "escondem a contagem de assistidos em avulso"; eles são o chip `Avulso`. A tela de Status já renderiza `watched` para todo anime via `TripleProgressBar` (`row.breakdown.watched`, alimentado por `episodes_watched`), então ela passa a mostrar o progresso do avulso **de graça** assim que a injeção da Task 9 entra. O único ponto que realmente esconde é `AnimeDetail.svelte:690` (`detail_counts_no_watched`), tratado na Task 10.
2. **"Assisti até aqui" só aparece em anime avulso.** Em anime de lista o progresso vem da AniList e é sobrescrito a cada passe (`info.EpisodesWatched = ml.Progress`), então a ação gravaria um número que nunca é lido — pior que não existir.
3. **A unificação da busca (item 6 da spec) entra ANTES da mudança de seleção**, na Task 2, preservando o comportamento atual (`batchResult[0]` para todos, gate de `willBatchAnime`). É o que mantém cada task compilando e testável isoladamente: a Task 2 entrega "uma requisição em vez de duas" sem mudar decisão nenhuma, e a Task 5 muda a decisão sem mexer em rede.

---

### Task 1: `ExtractBatchInfo` exportada, com guarda de falso positivo de resolução

`extractBatchInfo` existe e hoje é código morto. Ela passa a ser a fonte da faixa de episódios de um pack, então precisa (a) ser exportada e (b) parar de ler `[720-1080p]` como a faixa `720..1080`.

**Files:**
- Modify: `src/internal/nyaa/nyaa.go:128-149` (`extractBatchInfo`), e um shim exportado logo abaixo de `IsBatch` (`nyaa.go:157-160`)
- Test: `src/tests/unit/nyaa_test.go:1079-1083` (o `t.Skip` atual)

**Interfaces:**
- Consumes: nada de tasks anteriores.
- Produces: `func nyaa.ExtractBatchInfo(torrentName string) nyaa.BatchInfo`, com `BatchInfo{StartEpisode, EndEpisode, Season int; IsComplete bool}` (struct já existente, inalterada). Contrato usado por todas as tasks seguintes: **`EndEpisode == 0` significa "faixa desconhecida", e quem chama trata isso como pack completo.**

- [ ] **Step 1: Write the failing test**

Substitua o corpo de `TestExtractBatchInfo_ExtractsRangeAndSeason` em `src/tests/unit/nyaa_test.go:1079` (hoje um `t.Skip`) por:

```go
func TestExtractBatchInfo_ExtractsRangeAndSeason(t *testing.T) {
	cases := []struct {
		name             string
		wantStart        int
		wantEnd          int
		wantSeason       int
	}{
		{"[Judas] Anime 01-12 [1080p]", 1, 12, 0},
		{"[SubsPlease] Anime (001-100) [1080p]", 1, 100, 0},
		{"[Group] Anime S2 01-24 [BD]", 1, 24, 2},
		// Guarda de falso positivo: [720-1080p] nao e faixa de episodio. Sem ela, este pack
		// completo seria lido como "cobre os episodios 720 a 1080".
		{"[X] Anime Complete Batch [720-1080p]", 0, 0, 0},
		{"[X] Anime Complete Batch", 0, 0, 0},
	}

	for _, tc := range cases {
		got := nyaa.ExtractBatchInfo(tc.name)
		if got.StartEpisode != tc.wantStart || got.EndEpisode != tc.wantEnd {
			t.Errorf("%q: esperava faixa %d-%d, obteve %d-%d", tc.name, tc.wantStart, tc.wantEnd, got.StartEpisode, got.EndEpisode)
		}
		if got.Season != tc.wantSeason {
			t.Errorf("%q: esperava temporada %d, obteve %d", tc.name, tc.wantSeason, got.Season)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./src/tests/unit/ -run TestExtractBatchInfo -v`
Expected: FAIL com `undefined: nyaa.ExtractBatchInfo`.

- [ ] **Step 3: Write the implementation**

Em `src/internal/nyaa/nyaa.go`, troque o bloco de faixa de `extractBatchInfo` (linhas 133-140) por:

```go
	// FindStringSubmatchIndex (e nao FindStringSubmatch) porque a guarda precisa do caractere
	// SEGUINTE ao casamento: reBatchRange casa "720-1080" dentro de "[720-1080p]" e produziria
	// uma faixa fantasma de 361 episodios.
	//
	// ponytail: cobre so o caso dominante (resolucao). Outras faixas fantasma — data
	// ("2020-2021"), bitrate — so entrariam com um sanitizador de tokens, que nao se paga hoje:
	// faixa desconhecida cai em EndEpisode == 0, que o chamador ja trata como pack completo.
	if loc := reBatchRange.FindStringSubmatchIndex(torrentName); loc != nil {
		end := loc[1]
		isResolution := end < len(torrentName) && strings.ContainsRune("pPiI", rune(torrentName[end]))
		if !isResolution {
			if start, err := strconv.Atoi(torrentName[loc[2]:loc[3]]); err == nil {
				info.StartEpisode = start
			}
			if last, err := strconv.Atoi(torrentName[loc[4]:loc[5]]); err == nil {
				info.EndEpisode = last
			}
		}
	}
```

E, logo abaixo de `IsBatch` (`nyaa.go:157-160`), adicione o shim exportado no mesmo formato:

```go
// ExtractBatchInfo e a versao exportada de extractBatchInfo: o daemon precisa da faixa de
// episodios do pack para decidir QUAIS episodios recebem o magnet dele (pickBatches).
func ExtractBatchInfo(name string) BatchInfo {
	return extractBatchInfo(name)
}
```

`strings` e `strconv` já estão importados em `nyaa.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./src/tests/unit/ -run TestExtractBatchInfo -v && go test ./...`
Expected: PASS em tudo.

- [ ] **Step 5: Commit** (ver Global Constraints — pedir permissão)

```bash
git add src/internal/nyaa/nyaa.go src/tests/unit/nyaa_test.go
git commit -m "feat(nyaa): exportar ExtractBatchInfo com guarda de faixa de resolucao"
```

---

### Task 2: Uma requisição em vez de duas (`ScrapNyaaForAnime`)

`ScrapNyaaForBatch` e `ScrapNyaaForMultipleEpisodes` montam a **mesma** URL (`?f=0&c=1_2&q=<título>&s=seeders&o=desc`) e discordam só no filtro de linha. Viram uma função que devolve **uma** lista mista; quem particiona é o daemon, porque é lá que os tetos de tamanho são diferentes. Nesta task o comportamento de escolha **não muda**.

**Files:**
- Modify: `src/internal/nyaa/nyaa.go:550-700` (deleta `ScrapNyaaForMultipleEpisodes`) e `:702-811` (deleta `ScrapNyaaForBatch`), somando `ScrapNyaaForAnime` no lugar
- Modify: `src/internal/daemon/search.go:11-25` (campos do `nyaaSearcher`), `:159-177` (as duas funções de busca viram uma)
- Modify: `src/internal/daemon/episodes.go:196-286` (`resolveSearchStrategy` passa a buscar uma vez e particionar)
- Test: `src/tests/unit/nyaa_test.go` (todos os `ScrapNyaaForBatch` / `ScrapNyaaForMultipleEpisodes`), `src/internal/daemon/limits_test.go:44-54`, `src/internal/daemon/episodes_test.go:419-447` e `:979-986`

**Interfaces:**
- Consumes: `nyaa.ExtractBatchInfo` (Task 1) — não usado aqui, mas o pacote já está tocado.
- Produces:
  - `func nyaa.ScrapNyaaForAnime(animeName string, episodes []int, requestedSeason, requestedPart *int) ([]nyaa.TorrentResult, error)` — uma lista: linha de pack com `IsBatch: true`, linha de episódio com `Episode != nil`, ordenada por `SortTorrentResults`.
  - Campo `searchAnime func(titles anilist.Title, synonyms []string, episodes []int, customQuery string) []nyaa.TorrentResult` no `daemon.nyaaSearcher` (substitui `searchBatch` e `searchMultiple`).
  - `func daemon.partitionSearchResults(configs *files.Config, results []nyaa.TorrentResult) (packs, singles []nyaa.TorrentResult)` — cada lista já filtrada pelo SEU teto de tamanho e pelo piso de seeders.

- [ ] **Step 1: Write the failing test**

Adicione ao fim de `src/tests/unit/nyaa_test.go`:

```go
// A unificacao nao pode mudar a query: reSeasonNamePatterns (batch) e reSeasonStrip+rePartStrip
// (multi) removiam o MESMO conjunto (Season N, S N, Nª Season, Cour N, Part N). Este teste e a
// garantia de que o saneamento unificado nao regride.
func TestScrapNyaaForAnime_QueryMatchesTheTwoOldSearches(t *testing.T) {
	titles := []struct{ in, wantQuery string }{
		{"Show", "Show"},
		{"Show Season 2", "Show"},
		{"Show S2", "Show"},
		{"Show 2nd Season", "Show"},
		{"Show Cour 2", "Show"},
		{"Show Part 2", "Show"},
	}

	for _, tc := range titles {
		var gotURL string
		restore := nyaa.MockNyaaHttpGet(func(url string) (*http.Response, error) {
			gotURL = url
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(mockHtml(nil)))}, nil
		})
		if _, err := nyaa.ScrapNyaaForAnime(tc.in, []int{1}, nil, nil); err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		restore()

		if !strings.Contains(gotURL, "q="+url.QueryEscape(tc.wantQuery)+"&") && !strings.HasSuffix(gotURL, "q="+url.QueryEscape(tc.wantQuery)) {
			t.Errorf("%q: esperava a query %q na URL, obteve %q", tc.in, tc.wantQuery, gotURL)
		}
	}
}

// Uma pagina com pack e episodio devolve os DOIS na mesma lista, distinguiveis por IsBatch /
// Episode — a particao no daemon reproduz o que as duas funcoes antigas devolviam.
func TestScrapNyaaForAnime_ReturnsPacksAndEpisodesInOneList(t *testing.T) {
	html := mockHtml([]string{
		"[SubsPlease] Frieren (01-28) [1080p]",
		"[SubsPlease] Frieren - 05 [1080p]",
		"[SubsPlease] Frieren - 09 [1080p]",
	})
	restore := mockHttpGet(html)
	defer restore()

	results, err := nyaa.ScrapNyaaForAnime("Frieren", []int{5}, nil, nil)
	if err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}

	var packs, singles int
	for _, r := range results {
		switch {
		case r.IsBatch:
			packs++
		case r.Episode != nil:
			singles++
			if *r.Episode != 5 {
				t.Errorf("episodio fora da lista pedida entrou: %d", *r.Episode)
			}
		default:
			t.Errorf("resultado sem IsBatch e sem Episode: %q", r.Name)
		}
	}
	if packs != 1 {
		t.Errorf("esperava 1 pack, obteve %d", packs)
	}
	if singles != 1 {
		t.Errorf("esperava 1 episodio (so o 5 foi pedido), obteve %d", singles)
	}
}

// O piso de paginacao (enoughCandidates = 3) conta as DUAS listas somadas.
func TestScrapNyaaForAnime_PaginationFloorCountsBothLists(t *testing.T) {
	pages := 0
	restore := nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		pages++
		html := mockHtml([]string{
			"[G] Show (01-12) [1080p]",
			"[G] Show (13-24) [1080p]",
			"[G] Show - 05 [1080p]",
		})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(html))}, nil
	})
	defer restore()

	if _, err := nyaa.ScrapNyaaForAnime("Show", []int{5}, nil, nil); err != nil {
		t.Fatalf("ScrapNyaaForAnime error: %v", err)
	}
	if pages != 1 {
		t.Errorf("2 packs + 1 episodio ja atingem o piso: esperava 1 pagina, obteve %d", pages)
	}
}
```

Se `net/url` e `io` ainda não estiverem importados em `nyaa_test.go`, adicione-os. `mockHtml` e `mockHttpGet` já existem no arquivo.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./src/tests/unit/ -run TestScrapNyaaForAnime -v`
Expected: FAIL com `undefined: nyaa.ScrapNyaaForAnime`.

- [ ] **Step 3: Write the implementation — `nyaa.ScrapNyaaForAnime`**

Em `src/internal/nyaa/nyaa.go`, **delete** `ScrapNyaaForMultipleEpisodes` (linhas 550-700) e `ScrapNyaaForBatch` (linhas 702-811) e ponha no lugar:

```go
// ScrapNyaaForAnime busca UMA vez a pagina de resultados do anime e devolve packs e episodios na
// MESMA lista. ScrapNyaaForBatch e ScrapNyaaForMultipleEpisodes montavam a mesma URL
// (?f=0&c=1_2&q=<titulo>&s=seeders&o=desc) e discordavam so no filtro de linha, o que custava
// duas descidas de ate max_search_pages sobre o mesmo HTML, por anime, por ciclo.
//
// Linha de pack entra com IsBatch: true; linha de episodio entra com Episode != nil. Quem
// particiona e o daemon (partitionSearchResults), que e onde os tetos de tamanho de pack e de
// episodio sao diferentes.
//
// Devolve UMA lista de proposito: searchNyaaWithVariants para na primeira variante de titulo com
// resultado, e "resultado" e uma fatia nao-vazia — uma assinatura de par exigiria generaliza-la.
func ScrapNyaaForAnime(animeName string, episodes []int, requestedSeason, requestedPart *int) ([]TorrentResult, error) {
	query := strings.TrimSpace(extractSeasonFromName(animeName))

	params := url.Values{}
	params.Set("f", "0")   // Filtro: sem filtro
	params.Set("c", "1_2") // Categoria: anime (english)
	params.Set("q", query)
	params.Set("s", "seeders")
	params.Set("o", "desc")

	nyaaURL := fmt.Sprintf("%s/?%s", getNyaaBaseURL(), params.Encode())

	logger.Logger.Debug().
		Str("url", nyaaURL).
		Str("anime_name", animeName).
		Int("episodes_count", len(episodes)).
		Msg("Searching Nyaa for anime (packs + episodes)")

	var results []TorrentResult

	parseRow := func(_ int, s *goquery.Selection) {
		cells := s.Find("td")

		name := strings.TrimSpace(cells.Eq(1).Find("a").Not(".comments").Text())
		if name == "" {
			return
		}
		logger.Logger.Debug().Str("name", name).Msg("Raw Nyaa row")

		if shouldIgnoreTorrent(name) {
			return
		}
		// Filtrar por titulo base (garantir que o torrent pertence ao anime)
		if query != "" && !titleMatchesQuery(name, query) {
			return
		}

		season := extractSeason(name)
		part := extractPart(name)
		res := extractResolution(name)

		row := TorrentResult{
			Name:       name,
			Seeders:    strings.TrimSpace(cells.Eq(5).Text()),
			Leechers:   parseSeeders(strings.TrimSpace(cells.Eq(6).Text())),
			MagnetLink: cells.Eq(2).Find("a").Eq(1).AttrOr("href", ""),
			Season:     season,
			Part:       part,
			Resolution: &res,
			Size:       parseSize(strings.TrimSpace(cells.Eq(3).Text())),
			Fansub:     extractFansub(name),
		}

		if isBatch(name) {
			// Temporada no caminho de pack: sem pedido explicito, pack de qualquer temporada
			// serve. Era o comportamento de ScrapNyaaForBatch, e e diferente do caminho de
			// episodio logo abaixo — por isso os dois filtros nao podem ser fundidos.
			if requestedSeason != nil && (season == nil || *season != *requestedSeason) {
				return
			}
			if requestedPart != nil && (part == nil || *part != *requestedPart) {
				return
			}
			row.IsBatch = true
			results = append(results, row)
			return
		}

		// Filme/OVA/special nao e episodio: "Naruto Shippuuden Movie 3" casa o padrao " 3 (" de
		// extractEpisodeNumber e passaria como episodio 3.
		if hasMovieMarker(name) {
			return
		}
		// Temporada no caminho de episodio: sem pedido explicito, so temporada 1 (ou sem
		// marcador) passa.
		if requestedSeason != nil {
			if season == nil || *season != *requestedSeason {
				return
			}
		} else if season != nil && *season != 1 {
			return
		}
		if requestedPart != nil && (part == nil || *part != *requestedPart) {
			return
		}

		episode := extractEpisodeNumber(name)
		if episode == nil || !slices.Contains(episodes, *episode) {
			return
		}
		row.Episode = episode
		results = append(results, row)
	}

	// ponytail: o piso de paginacao conta as duas listas somadas. Uma pagina 1 com 3 packs que o
	// filtro de tamanho depois descarta encerra a descida sem ter juntado episodio solto nenhum,
	// onde antes a segunda busca desceria por conta propria. A saida, se aparecer, e empurrar os
	// filtros para dentro do contador — o que exige o nyaa conhecer a config, que e justamente o
	// que applyNyaaSettings evita.
	if err := fetchSearchPages(nyaaURL, enoughCandidates, func() int { return len(results) }, parsePagesWith(parseRow)); err != nil {
		return nil, err
	}
	results = deduplicateByMagnet(results)

	logger.Logger.Debug().
		Str("anime_name", animeName).
		Int("results", len(results)).
		Strs("matched_torrents", torrentSummaries(results)).
		Msg("Found Nyaa results for anime")

	if len(results) == 0 {
		return nil, nil
	}

	// Ordena a lista mista: particionar depois preserva a ordem relativa, entao cada lista sai
	// ordenada corretamente.
	return SortTorrentResults(results), nil
}
```

Se `go vet` reclamar de import não usado (`slices` continua usado aqui; `math` etc. permanecem), rode `gofmt -w` e ajuste.

- [ ] **Step 4: Write the implementation — `daemon/search.go`**

Em `src/internal/daemon/search.go`, troque os campos e o construtor:

```go
type nyaaSearcher struct {
	searchAnime         func(titles anilist.Title, synonyms []string, episodes []int, customQuery string) []nyaa.TorrentResult
	searchSingleEpisode func(ep anilist.AiringNode, titles anilist.Title, synonyms []string, relations anilist.MediaRelations, customQuery string, totalEpisodes int) []nyaa.TorrentResult
	searchMovie         func(titles anilist.Title, isFormatMovie bool, customQuery string) []nyaa.TorrentResult
}

func defaultNyaaSearcher() nyaaSearcher {
	return nyaaSearcher{
		searchAnime:         searchNyaaForAnime,
		searchSingleEpisode: searchNyaaForSingleEpisode,
		searchMovie:         searchNyaaForMovie,
	}
}
```

E substitua `searchNyaaForBatch` (`:159-164`) e `searchNyaaForMultipleEpisodes` (`:172-177`) por:

```go
// searchNyaaForAnime e a busca unica por anime: devolve packs e episodios na mesma lista (ver
// nyaa.ScrapNyaaForAnime).
func searchNyaaForAnime(titles anilist.Title, synonyms []string, episodes []int, customQuery string) []nyaa.TorrentResult {
	season, part := ExtractAnimeSeasonPart(titles, synonyms)
	return searchNyaaWithVariants(titles, customQuery, func(title string) ([]nyaa.TorrentResult, error) {
		return nyaa.ScrapNyaaForAnime(title, episodes, season, part)
	}, "anime")
}
```

- [ ] **Step 5: Write the implementation — partição no daemon**

Em `src/internal/daemon/episodes.go`, adicione (logo acima de `resolveSearchStrategy`):

```go
// partitionSearchResults separa a lista unica de ScrapNyaaForAnime em packs e episodios, e aplica
// a cada uma o SEU teto de tamanho: os dois tetos sao independentes (um pack de 40 GiB continua
// valido com max_episode_torrent_size_gb = 1.5). A ordem relativa vinda de SortTorrentResults e
// preservada, entao cada lista sai ordenada por qualidade.
func partitionSearchResults(configs *files.Config, results []nyaa.TorrentResult) (packs, singles []nyaa.TorrentResult) {
	for _, tr := range results {
		switch {
		case tr.IsBatch:
			packs = append(packs, tr)
		case tr.Episode != nil:
			singles = append(singles, tr)
		}
	}
	return filterSearchResults(packs, configs.MaxBatchTorrentSizeGB, configs.MinSeeders),
		filterSearchResults(singles, configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
}

// episodeNumbers e a lista de numeros que a busca por anime usa para filtrar linha de episodio.
func episodeNumbers(episodes []anilist.AiringNode) []int {
	eps := make([]int, 0, len(episodes))
	for _, ep := range episodes {
		eps = append(eps, ep.Episode)
	}
	return eps
}
```

Dentro de `resolveSearchStrategy`, substitua o bloco que hoje vai de `if len(episodesToDownload) == 0 { return result }` (linha 231) até o fim da função pelo trecho abaixo — **mesma decisão de antes**, só com uma busca:

```go
	if len(episodesToDownload) == 0 {
		return result
	}

	packs, singles := partitionSearchResults(configs, searcher.searchAnime(anime.Media.Title, anime.Media.Synonyms, episodeNumbers(episodesToDownload), customQuery))

	// O teto de max_batch_episodes entra aqui tambem: sem isso um anime de 720 episodios com o
	// limite por anime em 12 continuaria baixando o pack inteiro para 12 registros.
	if willBatchAnime(configs, anime) && len(episodesToDownload) > 1 && len(packs) > 0 {
		logger.Logger.Info().
			Str("anime", animeTitle).
			Str("torrent", packs[0].Name).
			Msg("Using batch torrent for finished anime")
		for _, ep := range episodesToDownload {
			result[ep.Episode] = resolvedMagnets{
				magnets:       []string{packs[0].MagnetLink},
				skipSubfolder: true,
				overrideName:  animeTitle,
			}
		}
		return result
	}

	byEpisode := make(map[int][]nyaa.TorrentResult)
	for _, tr := range singles {
		byEpisode[*tr.Episode] = append(byEpisode[*tr.Episode], tr)
	}
	for _, ep := range episodesToDownload {
		trs, ok := byEpisode[ep.Episode]
		if !ok {
			continue
		}
		var magnets []string
		for _, tr := range trs {
			magnets = append(magnets, tr.MagnetLink)
		}
		result[ep.Episode] = resolvedMagnets{magnets: magnets}
	}

	return result
```

- [ ] **Step 6: Adapt the Go tests that named the two old searches**

Em `src/internal/daemon/limits_test.go:44-54`, troque `searcherFor` (mesma assinatura, para não tocar os 10 chamadores):

```go
// searcherFor builds a searcher whose strategies return the given results. batch e multiple viram
// UMA lista (a busca por anime devolve as duas juntas): pack ganha IsBatch, episodio ja vem com
// Episode preenchido por multipleFor.
func searcherFor(batch, multiple, single, movie []nyaa.TorrentResult) nyaaSearcher {
	anime := make([]nyaa.TorrentResult, 0, len(batch)+len(multiple))
	for _, tr := range batch {
		tr.IsBatch = true
		anime = append(anime, tr)
	}
	anime = append(anime, multiple...)

	return nyaaSearcher{
		searchAnime: func(anilist.Title, []string, []int, string) []nyaa.TorrentResult { return anime },
		searchSingleEpisode: func(anilist.AiringNode, anilist.Title, []string, anilist.MediaRelations, string, int) []nyaa.TorrentResult {
			return single
		},
		searchMovie: func(anilist.Title, bool, string) []nyaa.TorrentResult { return movie },
	}
}
```

Em `limits_test.go:106-129` (`TestWillBatch_AboveCeilingFallsBackToLimitedOneByOne`), a busca de pack deixou de ser separável: remova o `searcher.searchBatch = ...` e o `if batchCalled`, e prove o mesmo pela saída:

```go
func TestWillBatch_AboveCeilingFallsBackToLimitedOneByOne(t *testing.T) {
	anime := animeWithEpisodes(39, anilist.MediaStatusFinished, true, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "pack", MagnetLink: fakeMagnet(9001)}}, multipleFor(39, 0), nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 12 {
		t.Errorf("esperava 12 episódios (limite por anime), obteve %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.IsBatch {
			t.Fatal("acima do teto de max_batch_episodes nenhum episódio deve vir de pack")
		}
	}
	if result.newEpisodes[0].EpisodeNumber != 1 {
		t.Errorf("o corte deve preservar os episódios mais antigos, obteve o primeiro = %d", result.newEpisodes[0].EpisodeNumber)
	}
}
```

Em `src/internal/daemon/episodes_test.go:419-447`, renomeie o mock e a asserção (o ponto do teste — "nada é buscado quando todos os episódios já estão no cliente" — continua válido, porque `resolveSearchStrategy` retorna antes de buscar com `len(episodesToDownload) == 0`):

```go
	// Mock do Nyaa: se a busca por anime for chamada, o teste deve falhar
	searchAnimeCalled := false
	mockSearcher := nyaaSearcher{
		searchAnime: func(_ anilist.Title, _ []string, _ []int, _ string) []nyaa.TorrentResult {
			searchAnimeCalled = true
			return []nyaa.TorrentResult{{MagnetLink: "magnet:?xt=urn:btih:fakehash", IsBatch: true}}
		},
		searchSingleEpisode: func(_ anilist.AiringNode, _ anilist.Title, _ []string, _ anilist.MediaRelations, _ string, _ int) []nyaa.TorrentResult {
			return nil
		},
		searchMovie: func(_ anilist.Title, _ bool, _ string) []nyaa.TorrentResult {
			return nil
		},
	}
```

e `if searchAnimeCalled { t.Error("a busca por anime não deve ser chamada: todos os episódios já estão no cliente pelo hash") }`.

Em `episodes_test.go:979-986`, o `noResults`:

```go
	noResults := nyaaSearcher{
		searchAnime: func(anilist.Title, []string, []int, string) []nyaa.TorrentResult { return nil },
		searchSingleEpisode: func(anilist.AiringNode, anilist.Title, []string, anilist.MediaRelations, string, int) []nyaa.TorrentResult {
			return nil
		},
		searchMovie: func(anilist.Title, bool, string) []nyaa.TorrentResult { return nil },
	}
```

- [ ] **Step 7: Adapt the nyaa tests that called the two old functions**

Em `src/tests/unit/nyaa_test.go`, adicione um helper local e reescreva as chamadas:

```go
// packsAndSingles reproduz a particao que o daemon faz sobre a lista unica de ScrapNyaaForAnime,
// para que os testes das duas buscas antigas continuem valendo sem mudar de asserção.
func packsAndSingles(results []nyaa.TorrentResult) (packs, singles []nyaa.TorrentResult) {
	for _, r := range results {
		switch {
		case r.IsBatch:
			packs = append(packs, r)
		case r.Episode != nil:
			singles = append(singles, r)
		}
	}
	return packs, singles
}
```

Depois, mecanicamente:
- `nyaa.ScrapNyaaForBatch(name, season, part)` → `nyaa.ScrapNyaaForAnime(name, nil, season, part)` e a asserção passa a rodar sobre `packs` de `packsAndSingles(results)`. (`episodes = nil` faz `slices.Contains` sempre falso, então nenhuma linha de episódio entra — é exatamente a busca de pack antiga.)
- `nyaa.ScrapNyaaForMultipleEpisodes(name, eps, season, part)` → `nyaa.ScrapNyaaForAnime(name, eps, season, part)` e a asserção roda sobre `singles`.
- `TestScrapNyaaForMultipleEpisodes_IgnoresBatch` muda de intenção: o pack não é mais **descartado**, é **classificado**. Renomeie para `TestScrapNyaaForAnime_ClassifiesBatchInsteadOfDiscarding` e afirme `len(singles) == 0` + `len(packs) == 1` para o HTML que hoje ele usa (`[Erai-raws] Naruto - 001 ~ 220 [480p]` + episódios soltos) — a garantia que importa (pack não entra como "episódio 1") continua provada.
- `TestScrapNyaaForBatch_PaginatesLikeEpisodeSearch` e `TestScrapNyaaForBatch_FiltersBySeason` seguem o mesmo mapeamento.

- [ ] **Step 8: Run the full suite**

Run: `go test ./...`
Expected: PASS. Se algum teste de `src/tests/unit/nyaa_test.go` falhar por ordenação, confira que a asserção usa `packs`/`singles` e não `results` cru.

- [ ] **Step 9: Commit**

```bash
git add src/internal/nyaa/nyaa.go src/internal/daemon/search.go src/internal/daemon/episodes.go src/internal/daemon/limits_test.go src/internal/daemon/episodes_test.go src/tests/unit/nyaa_test.go
git commit -m "refactor(nyaa): unificar busca de pack e de episodio numa requisicao"
```

---

### Task 3: `selectEpisodes` puro, `effectiveMax` e `windowEnd`

Refatoração sem mudança de comportamento: o laço de seleção sai de `processAnimeEpisodes` como função pura, para poder rodar **duas vezes** no mesmo passe (Task 5). `effectiveMax` passa a ser onde `max_episodes_per_anime == 0` vira "sem teto"; `windowEnd` é o outro lado do mesmo campo.

**Files:**
- Modify: `src/internal/daemon/episodes.go:22-79` (extração), `:288-308` (`buildWatchedKeepSet` — só a chamada muda nesta task, não o corpo)
- Modify: `src/internal/daemon/debug.go:70-88` (passa a chamar `selectEpisodes`)
- Test: `src/internal/daemon/selection_test.go` (novo)

**Interfaces:**
- Consumes: nada das tasks anteriores.
- Produces:
  - `type episodeSelection struct { toDownload []anilist.AiringNode; keysToDelete []files.EpisodeKey; checked []files.EpisodeKey }`
  - `func selectEpisodes(configs *files.Config, maxEpisodes int, anime anilist.MediaList, episodes []anilist.AiringNode, savedEpisodesMap map[files.EpisodeKey]bool, savedEpisodesFullMap map[files.EpisodeKey]files.EpisodeStruct, torrentsHashSet map[string]bool, keepSet, blockedMap map[files.EpisodeKey]bool) episodeSelection`
  - `func effectiveMax(configs *files.Config, episodes []anilist.AiringNode) int`
  - `func windowEnd(configs *files.Config, firstPending int) int`

- [ ] **Step 1: Write the failing tests**

Crie `src/internal/daemon/selection_test.go`:

```go
package daemon

import (
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
)

// savedNonBatch monta o mapa de episodios salvos (1..n) sem marcacao de pack.
func savedNonBatch(animeID, n int) (map[files.EpisodeKey]bool, map[files.EpisodeKey]files.EpisodeStruct) {
	set := make(map[files.EpisodeKey]bool, n)
	full := make(map[files.EpisodeKey]files.EpisodeStruct, n)
	for i := 1; i <= n; i++ {
		key := files.EpisodeKey{AnimeID: animeID, Episode: i}
		set[key] = true
		full[key] = files.EpisodeStruct{AnimeID: animeID, EpisodeNumber: i, EpisodeHash: "h"}
	}
	return set, full
}

// O bug que a ordem invertida evita: com o limite levantado por palpite, este conjunto vinha
// VAZIO — handleAlreadySavedEpisode nunca disparava com maxEpisodes = len(episodes)+1, e o
// tampao cortava episodesToDownload sem restaurar keysToDelete.
func TestSelectEpisodes_LimitProducesDeletions(t *testing.T) {
	anime := animeWithEpisodes(50, anilist.MediaStatusFinished, true, "")
	savedMap, savedFull := savedNonBatch(anime.Media.Id, 50)
	episodes := anilist.EpisodeList(anime, 1)

	sel := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{"h": true}, nil, nil)

	if len(sel.keysToDelete) != 38 {
		t.Errorf("esperava 38 episódios acima do limite em keysToDelete, obteve %d", len(sel.keysToDelete))
	}
	if len(sel.checked) != 50 {
		t.Errorf("todo episódio da lista deve ser checado, obteve %d", len(sel.checked))
	}
}

// Pureza: o contador de downloadedEpisodes e local, entao duas chamadas iguais devolvem o mesmo
// resultado. E o que torna a segunda selecao (limite levantado) de graca.
func TestSelectEpisodes_IsPure(t *testing.T) {
	anime := animeWithEpisodes(50, anilist.MediaStatusFinished, true, "")
	savedMap, savedFull := savedNonBatch(anime.Media.Id, 50)
	episodes := anilist.EpisodeList(anime, 1)

	first := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{"h": true}, nil, nil)
	second := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{"h": true}, nil, nil)

	if len(first.toDownload) != len(second.toDownload) || len(first.keysToDelete) != len(second.keysToDelete) {
		t.Errorf("chamadas iguais devolveram resultados diferentes: %d/%d vs %d/%d",
			len(first.toDownload), len(first.keysToDelete), len(second.toDownload), len(second.keysToDelete))
	}
}

func TestEffectiveMaxAndWindowEnd(t *testing.T) {
	episodes := make([]anilist.AiringNode, 30)

	unlimited := &files.Config{MaxEpisodesPerAnime: 0}
	if got := effectiveMax(unlimited, episodes); got != 31 {
		t.Errorf("teto 0 é sem teto: esperava 31, obteve %d", got)
	}
	if got := windowEnd(unlimited, 5); got != math.MaxInt {
		t.Errorf("teto 0 abre a janela: esperava MaxInt, obteve %d", got)
	}

	limited := &files.Config{MaxEpisodesPerAnime: 12}
	if got := effectiveMax(limited, episodes); got != 12 {
		t.Errorf("esperava 12, obteve %d", got)
	}
	if got := windowEnd(limited, 95); got != 106 {
		t.Errorf("janela de 12 a partir do 95 termina em 106, obteve %d", got)
	}
}
```

Adicione `"math"` aos imports do arquivo de teste.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./src/internal/daemon/ -run 'TestSelectEpisodes|TestEffectiveMax' -v`
Expected: FAIL com `undefined: selectEpisodes` / `undefined: effectiveMax`.

- [ ] **Step 3: Write the implementation**

Em `src/internal/daemon/episodes.go`, adicione (acima de `processAnimeEpisodes`):

```go
// episodeSelection e o resultado do laco de selecao de um anime.
type episodeSelection struct {
	toDownload   []anilist.AiringNode
	keysToDelete []files.EpisodeKey
	checked      []files.EpisodeKey
}

// selectEpisodes decide, para cada episodio da lista, se ele deve ser baixado ou apagado.
//
// E PURA: sem I/O e sem estado entre chamadas (o contador de episodios baixados e local), e por
// isso pode rodar duas vezes no mesmo passe — uma com o limite por anime valendo (que e a que
// produz as delecoes) e outra com ele levantado quando um pack foi escolhido. Ver decisions.md.
func selectEpisodes(
	configs *files.Config,
	maxEpisodes int,
	anime anilist.MediaList,
	episodes []anilist.AiringNode,
	savedEpisodesMap map[files.EpisodeKey]bool,
	savedEpisodesFullMap map[files.EpisodeKey]files.EpisodeStruct,
	torrentsHashSet map[string]bool,
	keepSet, blockedMap map[files.EpisodeKey]bool,
) episodeSelection {
	var sel episodeSelection
	downloadedEpisodesOfAnime := 0

	for _, ep := range episodes {
		key := files.EpisodeKey{AnimeID: anime.Media.Id, Episode: ep.Episode}
		sel.checked = append(sel.checked, key)

		savedEp := savedEpisodesFullMap[key]
		isInTorrents := episodeInTorrents(savedEp.EpisodeHash, torrentsHashSet)

		shouldDownload, shouldDelete := checkEpisode(configs, maxEpisodes, ep, anime, savedEpisodesMap[key], &downloadedEpisodesOfAnime, isInTorrents, keepSet[key])

		if shouldDownload && !blockedMap[key] {
			sel.toDownload = append(sel.toDownload, ep)
		} else if shouldDelete && !savedEp.ManuallyManaged {
			sel.keysToDelete = append(sel.keysToDelete, key)
		}
	}

	return sel
}

// effectiveMax e o limite por anime aplicado ao passe. max_episodes_per_anime == 0 significa SEM
// TETO, e "sem teto" e expresso como len(episodes)+1 — o mesmo truque que o caminho de pack ja
// usava para levantar o limite, agora tambem para o teto desligado.
func effectiveMax(configs *files.Config, episodes []anilist.AiringNode) int {
	if configs.MaxEpisodesPerAnime <= 0 {
		return len(episodes) + 1
	}
	return configs.MaxEpisodesPerAnime
}

// windowEnd e o ultimo episodio que os packs precisam cobrir neste ciclo: o outro lado de
// max_episodes_per_anime. Com o teto desligado a janela e aberta, o que e caro de proposito (os
// freios passam a ser max_batch_torrent_size_gb, por torrent, e checkDiskSpace).
func windowEnd(configs *files.Config, firstPending int) int {
	if configs.MaxEpisodesPerAnime <= 0 {
		return math.MaxInt
	}
	return firstPending + configs.MaxEpisodesPerAnime - 1
}
```

Adicione `"math"` aos imports de `episodes.go`.

Substitua o laço de `processAnimeEpisodes` (linhas 42-79, do `downloadedEpisodesOfAnime := 0` até o fim do `for`) por:

```go
	episodes := anilist.EpisodeList(anime, firstEpisodeToConsider(anime, savedEpisodes))
	keepSet := buildWatchedKeepSet(configs.WatchedEpisodesToKeep, anime.Media.Id, episodes, savedEpisodesMap, anime.Progress)

	totalEpisodes := 0
	if anime.Media.Episodes != nil {
		totalEpisodes = *anime.Media.Episodes
	}

	// Em batch o limite por anime nao se aplica: um batch e um torrent so, entao limitar os
	// registros nao limitaria os bytes nem os arquivos que vao para a biblioteca — so produziria
	// uma contagem mentirosa em episodes.json.
	maxEpisodes := effectiveMax(configs, episodes)
	if willBatchAnime(configs, anime) {
		maxEpisodes = len(episodes) + 1
	}

	sel := selectEpisodes(configs, maxEpisodes, anime, episodes, savedEpisodesMap, savedEpisodesFullMap, torrentsHashSet, keepSet, blockedMap)
	result.checkedEpisodes = sel.checked
	result.keysToDelete = sel.keysToDelete
	episodesToDownload := sel.toDownload
```

O resto de `processAnimeEpisodes` (tampão, laço de download) continua igual, agora lendo `episodesToDownload`.

- [ ] **Step 4: Use `selectEpisodes` in `debug.go`**

Em `src/internal/daemon/debug.go`, substitua as linhas 72-88 por:

```go
	maxEpisodes := effectiveMax(configs, episodes)
	if willBatchAnime(configs, anime) {
		maxEpisodes = len(episodes) + 1
	}

	// A MESMA funcao da producao: o debug reportava o pipeline errado quando este trecho era uma
	// copia mantida em sincronia a mao. savedEpisodes/torrents/keepSet/blocked vazios porque o
	// debug ja declara que trata todo episodio como nao-baixado.
	sel := selectEpisodes(configs, maxEpisodes, anime, episodes, nil, nil, nil, nil, nil)

	wouldSearch := make(map[int]bool, len(sel.toDownload))
	for _, ep := range sel.toDownload {
		wouldSearch[ep.Episode] = true
	}
	for _, ep := range episodes {
		summary.Episodes = append(summary.Episodes, EpisodeDebugResult{
			Episode:     ep.Episode,
			WouldSearch: wouldSearch[ep.Episode],
		})
	}
	episodesToDownload := sel.toDownload
```

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: PASS — nenhum comportamento mudou nesta task.

- [ ] **Step 6: Commit**

```bash
git add src/internal/daemon/episodes.go src/internal/daemon/debug.go src/internal/daemon/selection_test.go
git commit -m "refactor(daemon): extrair selectEpisodes puro, effectiveMax e windowEnd"
```

---

### Task 4: `pickBatches` e `coveringBatch`

Funções puras, testadas isoladamente, ainda **não** ligadas ao fluxo. Elas escolhem o mínimo de packs que cobre a janela pedida, sobre resultados já ordenados por qualidade e já filtrados.

**Files:**
- Modify: `src/internal/daemon/episodes.go` (adiciona as duas funções)
- Test: `src/internal/daemon/batches_test.go` (novo)

**Interfaces:**
- Consumes: `nyaa.ExtractBatchInfo` (Task 1); `windowEnd` (Task 3).
- Produces:
  - `func pickBatches(results []nyaa.TorrentResult, firstPending, windowEnd int) []nyaa.TorrentResult`
  - `func coveringBatch(results []nyaa.TorrentResult, episode int) *nyaa.TorrentResult`

- [ ] **Step 1: Write the failing tests**

Crie `src/internal/daemon/batches_test.go`:

```go
package daemon

import (
	"math"
	"testing"

	"AutoAnimeDownloader/src/internal/nyaa"
)

func pack(name string, n int) nyaa.TorrentResult {
	return nyaa.TorrentResult{Name: name, MagnetLink: fakeMagnet(n), IsBatch: true}
}

func names(results []nyaa.TorrentResult) []string {
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.Name)
	}
	return out
}

func TestPickBatches_OnePackCoversTheWindow(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1), pack("[X] Anime 101-200 [1080p]", 2)}

	if got := names(pickBatches(results, 150, 161)); len(got) != 1 || got[0] != "[X] Anime 101-200 [1080p]" {
		t.Errorf("primeiro pendente 150 deve escolher só o segundo pack, obteve %v", got)
	}
	if got := names(pickBatches(results, 1, 12)); len(got) != 1 || got[0] != "[X] Anime 001-100 [1080p]" {
		t.Errorf("primeiro pendente 1 deve escolher só o primeiro pack, obteve %v", got)
	}
	if got := pickBatches(results, 500, 511); len(got) != 0 {
		t.Errorf("sem pack que cubra, esperava vazio (o fluxo cai em episódio solto), obteve %v", names(got))
	}
}

// Janela cruzando a borda: 95..106 nao cabe num pack de 100, entao os DOIS entram, em ordem.
func TestPickBatches_WindowCrossingTheBoundaryPicksBoth(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1), pack("[X] Anime 101-200 [1080p]", 2)}

	got := names(pickBatches(results, 95, 106))
	if len(got) != 2 || got[0] != "[X] Anime 001-100 [1080p]" || got[1] != "[X] Anime 101-200 [1080p]" {
		t.Errorf("esperava os dois packs em ordem, obteve %v", got)
	}
}

func TestPickBatches_CeilingOffCoversEverything(t *testing.T) {
	results := []nyaa.TorrentResult{
		pack("[X] Anime 1-100 [1080p]", 1),
		pack("[X] Anime 101-200 [1080p]", 2),
		pack("[X] Anime 201-300 [1080p]", 3),
	}

	if got := pickBatches(results, 1, math.MaxInt); len(got) != 3 {
		t.Errorf("com o teto desligado esperava os três packs, obteve %v", names(got))
	}
}

// A ordem da lista E a ordem de qualidade (SortTorrentResults rodou antes), entao "o primeiro que
// cobre o cursor" e "o melhor que cobre o cursor".
func TestPickBatches_RespectsQualityOrder(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[Good] Anime 001-100 [1080p]", 1), pack("[Worse] Anime 001-100 [480p]", 2)}

	got := names(pickBatches(results, 1, 12))
	if len(got) != 1 || got[0] != "[Good] Anime 001-100 [1080p]" {
		t.Errorf("esperava o primeiro da lista ordenada, obteve %v", got)
	}
}

// Terminacao: coveringBatch so devolve pack com EndEpisode >= cursor, entao o cursor cresce
// estritamente e nenhum pack e escolhido duas vezes.
func TestPickBatches_Terminates(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime 50-60 [1080p]", 1)}

	got := pickBatches(results, 55, math.MaxInt)
	if len(got) != 1 {
		t.Errorf("esperava exatamente 1 pack, obteve %d (%v)", len(got), names(got))
	}
}

// Pack sem faixa no nome vale como pack completo: cobre o resto da janela e encerra a busca.
func TestPickBatches_UnknownRangeCountsAsComplete(t *testing.T) {
	results := []nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1), pack("[X] Anime 001-100", 2)}

	got := pickBatches(results, 1, math.MaxInt)
	if len(got) != 1 || got[0].Name != "[X] Anime Complete Batch" {
		t.Errorf("esperava só o pack completo, obteve %v", names(got))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./src/internal/daemon/ -run TestPickBatches -v`
Expected: FAIL com `undefined: pickBatches`.

- [ ] **Step 3: Write the implementation**

Em `src/internal/daemon/episodes.go` (perto de `partitionSearchResults`):

```go
// pickBatches devolve o minimo de packs que cobre [firstPending, windowEnd], em ordem.
//
// results ja vem ordenado por SortTorrentResults e ja filtrado por tamanho e seeders, entao "o
// primeiro que cobre o cursor" e "o melhor que cobre o cursor" — e por isso a escolha de pack
// deixou de ser decidida por metadado do AniList e passou a ser decidida pelo resultado da busca.
//
// Termina sempre: coveringBatch so devolve pack com EndEpisode >= cursor, entao o cursor cresce
// estritamente a cada volta e nenhum pack pode ser escolhido duas vezes.
func pickBatches(results []nyaa.TorrentResult, firstPending, windowEnd int) []nyaa.TorrentResult {
	var picked []nyaa.TorrentResult
	for cursor := firstPending; cursor <= windowEnd; {
		next := coveringBatch(results, cursor)
		if next == nil {
			break
		}
		picked = append(picked, *next)
		info := nyaa.ExtractBatchInfo(next.Name)
		if info.EndEpisode == 0 {
			break // faixa desconhecida vale como pack completo: cobre o resto da janela
		}
		cursor = info.EndEpisode + 1
	}
	return picked
}

// coveringBatch devolve o primeiro pack da lista que cobre o episodio. Pack sem faixa no nome
// (EndEpisode == 0) conta como completo.
func coveringBatch(results []nyaa.TorrentResult, episode int) *nyaa.TorrentResult {
	for i := range results {
		info := nyaa.ExtractBatchInfo(results[i].Name)
		if info.EndEpisode == 0 || (info.StartEpisode <= episode && episode <= info.EndEpisode) {
			return &results[i]
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./src/internal/daemon/ -run TestPickBatches -v && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/internal/daemon/episodes.go src/internal/daemon/batches_test.go
git commit -m "feat(daemon): pickBatches escolhe o minimo de packs que cobre a janela"
```

---

### Task 5: Elegibilidade pelo resultado da busca (o coração da spec)

`willBatchAnime` sai. Elegibilidade vira `!isAnimeMovie(anime) && len(toDownload) > 1 && pickBatches(...) != vazio`. Saem também `resolvedIsBatch`, o tampão e a Estratégia 2. Cada episódio recebe o magnet do **seu** pack, e os episódios fora da cobertura são truncados em vez de cair em episódio solto.

**Files:**
- Modify: `src/internal/daemon/episodes.go:22-157` (`processAnimeEpisodes`), `:159-286` (deleta `willBatchAnime`, `resolvedIsBatch`; `resolveSearchStrategy` vira `resolveMovie` + `magnetsByEpisode` + `assignBatches`)
- Modify: `src/internal/daemon/debug.go:72-103`
- Test: `src/internal/daemon/limits_test.go` (elegibilidade), `src/internal/daemon/batches_test.go` (atribuição e truncamento)

**Interfaces:**
- Consumes: `selectEpisodes`, `effectiveMax`, `windowEnd` (Task 3); `pickBatches`, `coveringBatch` (Task 4); `partitionSearchResults`, `episodeNumbers`, `searcher.searchAnime` (Task 2).
- Produces:
  - `func resolveMovie(configs *files.Config, anime anilist.MediaList, animeTitle string, episodes []anilist.AiringNode, customQuery string, searcher nyaaSearcher) ([]anilist.AiringNode, map[int]resolvedMagnets)` — devolve `(episodes, nil)` quando não há filme, e o fluxo cai na busca por anime.
  - `func assignBatches(animeTitle string, episodes []anilist.AiringNode, batches []nyaa.TorrentResult) ([]anilist.AiringNode, map[int]resolvedMagnets)` — devolve só os episódios cobertos (corte de prefixo) e o magnet do pack de cada um.
  - `func magnetsByEpisode(singles []nyaa.TorrentResult, episodes []anilist.AiringNode) map[int]resolvedMagnets`
  - `resolveSearchStrategy` **deixa de existir**.

- [ ] **Step 1: Write the failing tests**

Substitua em `src/internal/daemon/limits_test.go` os testes de `willBatchAnime` (`TestWillBatch_AboveCeilingFallsBackToLimitedOneByOne`, `TestWillBatch_ReleasingStaysLimited`, `TestWillBatch_UnknownTotalStaysLimited`, `TestWillBatch_CeilingZeroDisablesTheCeiling`, `TestWillBatch_EmptyBatchSearchReAppliesLimit`) por:

```go
// RELEASING de 1100 episódios BUSCA pack (antes desta spec não buscava, por status), e o pack
// parcial cobre a janela do limite por anime.
func TestEligibility_ReleasingLongSeriesUsesPartialPack(t *testing.T) {
	anime := animeWithEpisodes(1100, anilist.MediaStatusReleasing, false, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime 001-100 [1080p]", MagnetLink: fakeMagnet(1)}}, nil, nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 100 {
		t.Fatalf("esperava os 100 episódios do pack registrados, obteve %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if !ep.IsBatch {
			t.Fatalf("episódio %d deveria estar marcado como batch", ep.EpisodeNumber)
		}
	}
}

// Media.Episodes == nil passa a ser elegível: sem comparação de contagem, contagem desconhecida
// deixa de importar.
func TestEligibility_UnknownTotalIsEligible(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, false, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime 01-26 [1080p]", MagnetLink: fakeMagnet(1)}}, nil, nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 26 {
		t.Errorf("contagem desconhecida deve poder usar pack, obteve %d", len(result.newEpisodes))
	}
}

// Um episódio pendente não busca pack: um pack para um episódio é o caminho de episódio solto.
func TestEligibility_SinglePendingEpisodeDoesNotUsePack(t *testing.T) {
	anime := animeWithEpisodes(1, anilist.MediaStatusFinished, true, "")
	searcher := searcherFor([]nyaa.TorrentResult{{Name: "[X] Anime 01-26 [1080p]", MagnetLink: fakeMagnet(1)}}, multipleFor(1, 0), nil, nil)

	result := processAnimeEpisodes(limitsConfig(), torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 1 || result.newEpisodes[0].IsBatch {
		t.Errorf("esperava 1 episódio solto, obteve %+v", result.newEpisodes)
	}
}

// O caminho que o tampão cobria, agora sem tampão: o filtro de tamanho esvazia os packs, pickBatches
// devolve vazio, o fluxo cai em episódio solto e o limite por anime VALE.
func TestEligibility_SizeCeilingEmptiesPacksAndLimitApplies(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	configs := limitsConfig()
	configs.MaxBatchTorrentSizeGB = 10
	const gib = int64(1024 * 1024 * 1024)
	searcher := searcherFor(
		[]nyaa.TorrentResult{{Name: "[X] Anime 01-26 remux [1080p]", MagnetLink: fakeMagnet(9002), Size: 300 * gib}},
		multipleFor(26, gib),
		nil, nil,
	)

	result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, map[files.EpisodeKey]bool{}, "", searcher)

	if len(result.newEpisodes) != 12 {
		t.Errorf("esperava 12 episódios individuais, obteve %d", len(result.newEpisodes))
	}
	for _, ep := range result.newEpisodes {
		if ep.IsBatch {
			t.Fatal("nenhum episódio deveria vir marcado como batch")
		}
	}
}
```

E adicione a `src/internal/daemon/batches_test.go`:

```go
// Dois packs escolhidos: cada episódio recebe o magnet do pack da SUA faixa, e os overrideName
// são distintos (na tela de downloads dois packs do mesmo anime apareceriam com nomes idênticos).
func TestAssignBatches_EachEpisodeGetsItsOwnPack(t *testing.T) {
	anime := animeWithEpisodes(200, anilist.MediaStatusReleasing, false, "")
	first := pack("[X] Anime 001-100 [1080p]", 1)
	second := pack("[X] Anime 101-200 [1080p]", 2)

	episodes := anilist.EpisodeList(anime, 1)
	covered, magnets := assignBatches("Anime", episodes, []nyaa.TorrentResult{first, second})

	if len(covered) != 200 {
		t.Fatalf("esperava 200 episódios cobertos, obteve %d", len(covered))
	}
	if magnets[50].magnets[0] != first.MagnetLink || magnets[150].magnets[0] != second.MagnetLink {
		t.Error("cada episódio deve receber o magnet do pack da sua faixa")
	}
	if magnets[50].overrideName == magnets[150].overrideName {
		t.Errorf("os overrideName devem ser distintos, ambos são %q", magnets[50].overrideName)
	}
	if magnets[50].overrideName != "Anime 1-100" {
		t.Errorf("esperava \"Anime 1-100\", obteve %q", magnets[50].overrideName)
	}
	if !magnets[50].skipSubfolder {
		t.Error("episódio de pack precisa de skipSubfolder")
	}
}

// Cobertura parcial: os episódios de fora NÃO caem em episódio solto (em One Piece isso dispararia
// ~1000 notificações ReasonNotFound por passada). O corte é de prefixo.
func TestAssignBatches_TruncatesUncoveredEpisodes(t *testing.T) {
	anime := animeWithEpisodes(500, anilist.MediaStatusReleasing, false, "")
	episodes := anilist.EpisodeList(anime, 1)

	covered, magnets := assignBatches("Anime", episodes, []nyaa.TorrentResult{pack("[X] Anime 001-100 [1080p]", 1)})

	if len(covered) != 100 {
		t.Fatalf("esperava 100 episódios cobertos, obteve %d", len(covered))
	}
	if covered[99].Episode != 100 {
		t.Errorf("o corte deve preservar o prefixo, último coberto = %d", covered[99].Episode)
	}
	if _, ok := magnets[101]; ok {
		t.Error("episódio fora da cobertura não pode receber magnet")
	}
}

// Pack sem faixa no nome cobre tudo: nenhum truncamento e o nome cai no título do anime.
func TestAssignBatches_UnknownRangeUsesAnimeTitle(t *testing.T) {
	anime := animeWithEpisodes(26, anilist.MediaStatusFinished, true, "")
	episodes := anilist.EpisodeList(anime, 1)

	covered, magnets := assignBatches("Anime", episodes, []nyaa.TorrentResult{pack("[X] Anime Complete Batch", 1)})

	if len(covered) != 26 {
		t.Fatalf("esperava 26 episódios cobertos, obteve %d", len(covered))
	}
	if magnets[1].overrideName != "Anime" {
		t.Errorf("faixa desconhecida deve usar o título do anime, obteve %q", magnets[1].overrideName)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./src/internal/daemon/ -run 'TestEligibility|TestAssignBatches' -v`
Expected: FAIL — `undefined: assignBatches`, e os de elegibilidade falham por contagem (hoje `RELEASING` não busca pack).

- [ ] **Step 3: Write the implementation — as três funções de resolução**

Em `src/internal/daemon/episodes.go`, **delete** `willBatchAnime` (linhas 165-182), `resolvedIsBatch` (184-191) e `resolveSearchStrategy` (193-286), e ponha no lugar:

```go
// resolveMovie e o caminho de filme, inalterado: quando o filme e achado, todo episodio pendente
// (ou um episodio sintetico, se nao havia nenhum) recebe o magnet dele. Devolve (episodios, nil)
// quando nao ha filme, e o fluxo cai na busca por anime — que e o fallback que sempre existiu.
func resolveMovie(configs *files.Config, anime anilist.MediaList, animeTitle string, episodes []anilist.AiringNode, customQuery string, searcher nyaaSearcher) ([]anilist.AiringNode, map[int]resolvedMagnets) {
	logger.Logger.Info().
		Str("anime", animeTitle).
		Msg("Detected movie - searching for movie torrent")

	movieResult := filterSearchResults(searcher.searchMovie(anime.Media.Title, true, customQuery), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
	if len(movieResult) == 0 {
		return episodes, nil
	}

	if len(episodes) == 0 {
		episodes = append(episodes, anilist.AiringNode{Episode: 1})
		logger.Logger.Info().
			Str("anime", animeTitle).
			Msg("Created fake episode for movie download")
	}

	result := make(map[int]resolvedMagnets, len(episodes))
	for _, ep := range episodes {
		result[ep.Episode] = resolvedMagnets{
			magnets:       []string{movieResult[0].MagnetLink},
			skipSubfolder: true,
			overrideName:  animeTitle,
		}
	}
	logger.Logger.Info().
		Str("anime", animeTitle).
		Str("torrent", movieResult[0].Name).
		Msg("Using movie torrent")
	return episodes, result
}

// assignBatches da a cada episodio o magnet do SEU pack e devolve so os episodios cobertos.
//
// O truncamento e obrigatorio: sem ele, os episodios fora da cobertura cairiam em episodio solto e
// em One Piece o fallback nao acharia nada, disparando DownloadFailed/ReasonNotFound para ~1000
// episodios por passada. A lista vem da agenda em ordem crescente e os packs escolhidos sao
// contiguos a partir do primeiro pendente, entao o corte e sempre de prefixo.
//
// O overrideName leva a faixa quando ela e conhecida: dois packs do mesmo anime no mesmo ciclo
// apareceriam com nomes identicos na tela de downloads. O nome da PASTA da biblioteca nao muda —
// Organize usa AnimeName, nunca EpisodeName.
func assignBatches(animeTitle string, episodes []anilist.AiringNode, batches []nyaa.TorrentResult) ([]anilist.AiringNode, map[int]resolvedMagnets) {
	result := make(map[int]resolvedMagnets, len(episodes))
	var covered []anilist.AiringNode

	for _, ep := range episodes {
		batch := coveringBatch(batches, ep.Episode)
		if batch == nil {
			break
		}

		name := animeTitle
		if info := nyaa.ExtractBatchInfo(batch.Name); info.EndEpisode > 0 {
			name = fmt.Sprintf("%s %d-%d", animeTitle, info.StartEpisode, info.EndEpisode)
		}

		result[ep.Episode] = resolvedMagnets{
			magnets:       []string{batch.MagnetLink},
			skipSubfolder: true,
			overrideName:  name,
		}
		covered = append(covered, ep)
	}

	return covered, result
}

// magnetsByEpisode e o caminho de episodio solto: cada episodio recebe os magnets das linhas que
// casaram com o SEU numero, na ordem de qualidade.
func magnetsByEpisode(singles []nyaa.TorrentResult, episodes []anilist.AiringNode) map[int]resolvedMagnets {
	byEpisode := make(map[int][]nyaa.TorrentResult, len(singles))
	for _, tr := range singles {
		byEpisode[*tr.Episode] = append(byEpisode[*tr.Episode], tr)
	}

	result := make(map[int]resolvedMagnets, len(episodes))
	for _, ep := range episodes {
		trs, ok := byEpisode[ep.Episode]
		if !ok {
			continue
		}
		magnets := make([]string, 0, len(trs))
		for _, tr := range trs {
			magnets = append(magnets, tr.MagnetLink)
		}
		result[ep.Episode] = resolvedMagnets{magnets: magnets}
	}
	return result
}
```

- [ ] **Step 4: Write the implementation — o novo fluxo de `processAnimeEpisodes`**

Em `src/internal/daemon/episodes.go`, substitua o trecho que vai da seleção até o fim do tampão (o que a Task 3 deixou como `maxEpisodes := effectiveMax(...)` … até o `if willBatch && !resolvedIsBatch(...)`, inclusive) por:

```go
	// A primeira selecao e a que vale quando nao ha pack, e e ela que produz as delecoes por
	// limite: com o limite levantado por palpite, handleAlreadySavedEpisode nunca disparava e
	// keysToDelete vinha vazio para todo mundo.
	sel := selectEpisodes(configs, effectiveMax(configs, episodes), anime, episodes, savedEpisodesMap, savedEpisodesFullMap, torrentsHashSet, keepSet, blockedMap)

	var magnetsForEpisodes map[int]resolvedMagnets

	if isAnimeMovie(anime) {
		sel.toDownload, magnetsForEpisodes = resolveMovie(configs, anime, animeTitle, sel.toDownload, customQuery, searcher)
	}

	if magnetsForEpisodes == nil && len(sel.toDownload) > 0 {
		packs, singles := partitionSearchResults(configs, searcher.searchAnime(anime.Media.Title, anime.Media.Synonyms, episodeNumbers(sel.toDownload), customQuery))

		// Elegibilidade a pack: nao e filme, tem mais de um episodio pendente e a busca FILTRADA
		// devolveu pack que cobre a janela. Nada disso e metadado do AniList — e o torrent que
		// esta la que decide (ver decisions.md).
		if !isAnimeMovie(anime) && len(sel.toDownload) > 1 {
			firstPending := sel.toDownload[0].Episode
			if batches := pickBatches(packs, firstPending, windowEnd(configs, firstPending)); len(batches) > 0 {
				logger.Logger.Info().
					Str("anime", animeTitle).
					Int("packs", len(batches)).
					Strs("torrents", batchNames(batches)).
					Msg("Using batch torrents to cover the pending window")

				// Segunda selecao, com o limite levantado: os registros de episodes.json precisam
				// cobrir o pack INTEIRO, senao a contagem mente e a poda apagaria o que o pack
				// acabou de trazer.
				sel = selectEpisodes(configs, len(episodes)+1, anime, episodes, savedEpisodesMap, savedEpisodesFullMap, torrentsHashSet, keepSet, blockedMap)
				sel.toDownload, magnetsForEpisodes = assignBatches(animeTitle, sel.toDownload, batches)
			}
		}

		if magnetsForEpisodes == nil {
			magnetsForEpisodes = magnetsByEpisode(singles, sel.toDownload)
		}
	}

	result.checkedEpisodes = sel.checked
	result.keysToDelete = sel.keysToDelete
	episodesToDownload := sel.toDownload
```

E adicione o helper de log:

```go
// batchNames e so para o log: qual pack cobriu qual faixa e a primeira coisa que se quer saber ao
// auditar uma escolha de pack.
func batchNames(batches []nyaa.TorrentResult) []string {
	out := make([]string, 0, len(batches))
	for _, b := range batches {
		out = append(out, b.Name)
	}
	return out
}
```

- [ ] **Step 5: Update `debug.go` to the same flow**

Em `src/internal/daemon/debug.go`, troque o cálculo do limite (o `willBatchAnime` que a Task 3 manteve) por `maxEpisodes := effectiveMax(configs, episodes)` — sem levantamento por palpite —, e substitua a chamada a `resolveSearchStrategy` (linha 103) por:

```go
	searcher := defaultNyaaSearcher()

	var magnetsForEpisodes map[int]resolvedMagnets
	if isAnimeMovie(anime) {
		episodesToDownload, magnetsForEpisodes = resolveMovie(configs, anime, animeTitle, episodesToDownload, customQuery, searcher)
	}
	if magnetsForEpisodes == nil {
		packs, singles := partitionSearchResults(configs, searcher.searchAnime(anime.Media.Title, anime.Media.Synonyms, episodeNumbers(episodesToDownload), customQuery))
		if !isAnimeMovie(anime) && len(episodesToDownload) > 1 {
			firstPending := episodesToDownload[0].Episode
			if batches := pickBatches(packs, firstPending, windowEnd(configs, firstPending)); len(batches) > 0 {
				episodesToDownload, magnetsForEpisodes = assignBatches(animeTitle, episodesToDownload, batches)
			}
		}
		if magnetsForEpisodes == nil {
			magnetsForEpisodes = magnetsByEpisode(singles, episodesToDownload)
		}
	}
```

O debug não faz a segunda seleção porque não tem registros salvos para relevar (ele já declara que trata todo episódio como não-baixado).

- [ ] **Step 6: Fix `debug_test.go`**

Em `src/internal/daemon/debug_test.go`, os casos que montam `Config{... MaxBatchEpisodes: 30 ...}` continuam compilando nesta task (o campo só sai na Task 6), mas o par de testes de `willBatchAnime` das linhas 146-162 perde sentido: substitua os dois por um caso só, que prova que o teto por anime agora só vale sem pack:

```go
// Sem pack no resultado, o teto por anime vale no debug igual à produção.
func TestRunAnimeDebug_PerAnimeLimitAppliesWithoutPack(t *testing.T) {
	summary := runDebugFor(t, 39, anilist.MediaStatusFinished,
		&files.Config{MaxEpisodesPerAnime: 12, EpisodeRetryLimit: 3, AnilistUsernames: []string{"user"}})

	searched := 0
	for _, ep := range summary.Episodes {
		if ep.WouldSearch {
			searched++
		}
	}
	if searched != 12 {
		t.Errorf("esperava 12 episódios selecionados, obteve %d", searched)
	}
}
```

Adapte ao helper que o arquivo já usa para montar o cenário (o teste da linha 133 mostra a forma); mantenha o nome do helper existente em vez de criar outro.

- [ ] **Step 7: Run the full suite**

Run: `go test ./...`
Expected: PASS. `TestWillBatch_FinishedWithinCeilingIgnoresPerAnimeLimit` continua passando (26 episódios, pack completo) — se falhar, verifique que o nome do pack no fixture case com `isBatch`/`ExtractBatchInfo` (ex.: `"[X] Anime 01-26 [1080p]"`, não `"batch"`), porque `pickBatches` lê a faixa do NOME.

- [ ] **Step 8: Commit**

```bash
git add src/internal/daemon/episodes.go src/internal/daemon/debug.go src/internal/daemon/limits_test.go src/internal/daemon/batches_test.go src/internal/daemon/debug_test.go
git commit -m "feat(daemon): decidir batch pelo resultado filtrado da busca, com cobertura de janela"
```

---

### Task 6: Config — `max_batch_episodes` sai, `max_episodes_per_anime = 0` passa a valer

**Files:**
- Modify: `src/internal/files/filemanager.go:85-87` (campo), `:185` (default), `:91` (comentário do teto de pack) e `:185-186` (novo default `100`)
- Modify: `src/internal/api/endpoint_config.go:109-112` (a validação de `> 0` sai), `:129-132` (a de `MaxBatchEpisodes` sai)
- Modify: `src/internal/frontend/src/lib/api/client.ts:148`, `src/internal/frontend/src/routes/Config.svelte:50-51,115,245-247,263-267,645-655`
- Modify: `src/internal/frontend/messages/en.json` e `messages/pt-BR.json` (remove 3 chaves, reescreve 2 hints)
- Modify: `src/internal/frontend/tests/smoke/helpers.ts:90`
- Test: `src/internal/api/endpoint_config_test.go`

**Interfaces:**
- Consumes: `effectiveMax`/`windowEnd` (Task 3) já tratam `0` como "sem teto" — esta task só remove a validação que proibia gravar `0`.
- Produces: `files.Config` sem `MaxBatchEpisodes`; `MaxBatchTorrentSizeGB` com default `100`.

- [ ] **Step 1: Write the failing test**

Adicione a `src/internal/api/endpoint_config_test.go` (use o helper de config válida que o arquivo já tem; se não houver, copie o literal da linha 47):

```go
// max_episodes_per_anime = 0 passa a significar "sem teto" e deve ser aceito: era o unico teto do
// projeto que proibia zero.
func TestSaveConfig_AcceptsZeroMaxEpisodesPerAnime(t *testing.T) {
	server, _ := newTestServerWithConfig(t)
	config := validConfig()
	config.MaxEpisodesPerAnime = 0

	rec := putConfig(t, server, config)

	if rec.Code != http.StatusOK {
		t.Errorf("esperava 200 para o teto desligado, obteve %d: %s", rec.Code, rec.Body.String())
	}
}
```

Se `newTestServerWithConfig`/`validConfig`/`putConfig` não existirem com esses nomes, siga o padrão do teste de config mais próximo no arquivo (há vários montando `files.Config` inline e chamando o handler) — **não** crie helpers novos se o arquivo já tiver o seu.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./src/internal/api/ -run TestSaveConfig_AcceptsZero -v`
Expected: FAIL com 400 ("Max episodes per anime must be greater than 0").

- [ ] **Step 3: Go — remover o campo e a validação**

`src/internal/files/filemanager.go`:
- Delete as linhas 85-87 (comentário + campo `MaxBatchEpisodes`).
- Atualize o comentário de `MaxEpisodesPerAnime` (linha 81-83) acrescentando: `0 significa SEM TETO (ver effectiveMax/windowEnd em daemon/episodes.go).`
- Reescreva o comentário de `MaxBatchTorrentSizeGB` (linhas 88-90):

```go
	// MaxBatchTorrentSizeGB / MaxEpisodeTorrentSizeGB descartam da busca torrents acima do teto,
	// em GiB. 0 desliga. O de pack e a guarda UNICA de pack desde que a elegibilidade deixou de
	// ser contagem de episodios: 100 cabe pack completo de serie de temporada em 1080p e nao cabe
	// pack completo de One Piece — para serie longa o que passa e pack parcial.
```
- Em `getDefaultConfig` (linha 185): remova `MaxBatchEpisodes: 30,` e acrescente `MaxBatchTorrentSizeGB: 100,`.

`src/internal/api/endpoint_config.go`:
- Delete o bloco 109-112 (`MaxEpisodesPerAnime <= 0`) e ponha no lugar:

```go
		// max_episodes_per_anime aceita 0 = sem teto, alinhado com os outros tetos do projeto
		// (max_batch_torrent_size_gb, min_seeders, watched_episodes_to_keep).
		if config.MaxEpisodesPerAnime < 0 {
			JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Max episodes per anime must be non-negative")
			return
		}
```
- Delete o bloco 129-132 (`MaxBatchEpisodes < 0`).

Rode `grep -rn "MaxBatchEpisodes" src/` e limpe o que sobrar em testes (`limits_test.go:80`, `debug_test.go:133,149,162,201`).

- [ ] **Step 4: Frontend — client, tela, i18n, smoke**

`src/internal/frontend/src/lib/api/client.ts`: delete a linha `max_batch_episodes: number`.

`src/internal/frontend/src/routes/Config.svelte`:
- Delete `labelMaxBatchEpisodes` / `hintMaxBatchEpisodes` (linhas 50-51), `max_batch_episodes: 30,` (linha 115), o bloco de validação de `max_batch_episodes` (263-267) e o `<Input id="max_batch_episodes">` (645-655).
- Troque a validação de `max_episodes_per_anime` (linhas 243-247) para `ok: config.max_episodes_per_anime >= 0,`.

`messages/en.json` — delete `config_label_max_batch_episodes`, `config_hint_max_batch_episodes`, `config_val_max_batch_episodes`; e reescreva:

```json
  "config_hint_max_episodes": "How many episodes of one anime exist at the same time. Applies to the episode-by-episode path only — a pack is a single torrent, so limiting records would not limit bytes. Set to 0 for no limit (a pack search then covers every pending episode at once).",
  "config_val_max_episodes": "Max episodes per anime must be non-negative",
  "config_hint_max_batch_size": "Pack torrents above this size are discarded from the search results. This is the only ceiling on packs: ~40 GiB fits a 26-episode 1080p pack, and the 100 GiB default deliberately does not fit a full One Piece pack (a partial pack does). Set to 0 for no limit."
```

`messages/pt-BR.json` — as mesmas três chaves removidas, e:

```json
  "config_hint_max_episodes": "Quantos episódios de um anime existem ao mesmo tempo. Vale apenas no caminho episódio a episódio — um pack é um torrent só, então limitar registros não limitaria bytes. Use 0 para não limitar (aí a busca de pack cobre todos os episódios pendentes de uma vez).",
  "config_val_max_episodes": "Máx. episódios por anime não pode ser negativo",
  "config_hint_max_batch_size": "Torrents de pack acima desse tamanho são descartados do resultado da busca. É a única guarda de pack: ~40 GiB cabe um pack de 26 episódios em 1080p, e o default de 100 GiB de propósito NÃO cabe um pack completo de One Piece (pack parcial cabe). Use 0 para não limitar."
```

Se o `config_hint_max_episodes` não existir com esse nome, confirme com `grep -n "config_hint_max_episodes" messages/en.json` e use a chave real do hint desse campo.

`src/internal/frontend/tests/smoke/helpers.ts:90`: delete a linha `max_batch_episodes: 30,`.

- [ ] **Step 5: Run the suites**

Run: `go test ./... && cd src/internal/frontend && bun run i18n:compile && bun run test:unit && bun run test:component && bun run check`
Expected: PASS; `bun run check` sem erro de chave i18n inexistente.

- [ ] **Step 6: Commit**

```bash
git add -A src/internal/files src/internal/api src/internal/frontend src/internal/daemon
git commit -m "feat(config): remover max_batch_episodes e aceitar teto por anime desligado"
```

---

### Task 7: Episódio de pack é imune à poda por limite e ao keep-set

Sem isto, o ciclo seguinte ao que trouxe o pack apagaria os episódios 13-100 e, com o gate de espaço reabrindo, viraria loop de baixar-e-apagar.

**Files:**
- Modify: `src/internal/daemon/episodes.go` (`checkEpisode`, `handleAlreadySavedEpisode`, `selectEpisodes`, `buildWatchedKeepSet` e sua chamada)
- Modify: `src/internal/daemon/debug.go` (a chamada de `checkEpisode`, se ainda houver — depois da Task 3 quem chama é `selectEpisodes`)
- Test: `src/internal/daemon/selection_test.go`

**Interfaces:**
- Consumes: `selectEpisodes` (Task 3).
- Produces:
  - `func checkEpisode(configs *files.Config, maxEpisodes int, ep anilist.AiringNode, anime anilist.MediaList, alreadySaved bool, downloadedEpisodes *int, isInTorrents bool, keepWatched, isBatch bool) (shouldDownload, shouldDelete bool)`
  - `func handleAlreadySavedEpisode(maxEpisodes int, downloadedEpisodes *int, isInTorrents, isBatch bool, epName string) (shouldDownload, shouldDelete bool)`
  - `func buildWatchedKeepSet(n int, animeID int, episodes []anilist.AiringNode, savedEpisodesFullMap map[files.EpisodeKey]files.EpisodeStruct, progress int) map[files.EpisodeKey]bool` (o mapa completo substitui o de bool).

- [ ] **Step 1: Write the failing tests**

Adicione a `src/internal/daemon/selection_test.go`:

```go
// Registro de pack estourando o limite NAO e apagado: o pack e a unidade, apagar um registro nao
// libera byte nenhum e o torrent continua no disco. E tambem nao baixa mais nada deste anime.
func TestHandleAlreadySavedEpisode_BatchIsImmuneToTheLimit(t *testing.T) {
	counter := 12

	if dl, del := handleAlreadySavedEpisode(12, &counter, true, true, "ep"); dl || del {
		t.Errorf("pack estourando o limite deve devolver (false, false), obteve (%v, %v)", dl, del)
	}
	if counter != 12 {
		t.Errorf("o contador não deve avançar no caminho imune, obteve %d", counter)
	}

	counter = 12
	if dl, del := handleAlreadySavedEpisode(12, &counter, true, false, "ep"); dl || !del {
		t.Errorf("episódio solto estourando o limite deve devolver (false, true), obteve (%v, %v)", dl, del)
	}
}

// Nao-churn: dois ciclos seguidos sobre o mesmo estado nao produzem delecao nenhuma.
func TestSelectEpisodes_BatchStateDoesNotChurn(t *testing.T) {
	anime := animeWithEpisodes(100, anilist.MediaStatusReleasing, false, "")
	savedMap := make(map[files.EpisodeKey]bool, 100)
	savedFull := make(map[files.EpisodeKey]files.EpisodeStruct, 100)
	for i := 1; i <= 100; i++ {
		key := files.EpisodeKey{AnimeID: anime.Media.Id, Episode: i}
		savedMap[key] = true
		savedFull[key] = files.EpisodeStruct{AnimeID: anime.Media.Id, EpisodeNumber: i, EpisodeHash: "pack", IsBatch: true}
	}
	episodes := anilist.EpisodeList(anime, 1)

	for cycle := 1; cycle <= 2; cycle++ {
		sel := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{"pack": true}, nil, nil)
		if len(sel.keysToDelete) != 0 {
			t.Fatalf("ciclo %d: pack salvo não deve gerar deleção, obteve %d", cycle, len(sel.keysToDelete))
		}
		if len(sel.toDownload) != 0 {
			t.Fatalf("ciclo %d: nada novo deve ser baixado, obteve %d", cycle, len(sel.toDownload))
		}
	}
}

// watched_episodes_to_keep quebraria o rodizio: manter 3 dos 100 de um pack nao guarda 3
// episodios, guarda o pack inteiro (allEpisodesInDeleteSet daria false e o torrent ficaria).
func TestBuildWatchedKeepSet_IgnoresBatchRecords(t *testing.T) {
	anime := animeWithEpisodes(100, anilist.MediaStatusReleasing, false, "")
	savedFull := make(map[files.EpisodeKey]files.EpisodeStruct, 100)
	for i := 1; i <= 100; i++ {
		key := files.EpisodeKey{AnimeID: anime.Media.Id, Episode: i}
		savedFull[key] = files.EpisodeStruct{AnimeID: anime.Media.Id, EpisodeNumber: i, EpisodeHash: "pack", IsBatch: true}
	}
	episodes := anilist.EpisodeList(anime, 1)

	keepSet := buildWatchedKeepSet(3, anime.Media.Id, episodes, savedFull, 100)
	if len(keepSet) != 0 {
		t.Errorf("esperava keep-set vazio para pack, obteve %d", len(keepSet))
	}

	// O par: registro sem IsBatch continua respeitando o keep.
	for key, ep := range savedFull {
		ep.IsBatch = false
		savedFull[key] = ep
	}
	if keepSet := buildWatchedKeepSet(3, anime.Media.Id, episodes, savedFull, 100); len(keepSet) != 3 {
		t.Errorf("episódio solto deve manter 3, obteve %d", len(keepSet))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./src/internal/daemon/ -run 'TestHandleAlreadySaved|TestSelectEpisodes_BatchState|TestBuildWatchedKeepSet' -v`
Expected: FAIL de compilação (assinaturas antigas).

- [ ] **Step 3: Write the implementation**

`handleAlreadySavedEpisode` (`episodes.go:383`):

```go
func handleAlreadySavedEpisode(maxEpisodes int, downloadedEpisodes *int, isInTorrents, isBatch bool, epName string) (shouldDownload bool, shouldDelete bool) {
	if *downloadedEpisodes >= maxEpisodes {
		if isBatch {
			// Mesma logica de "limitar registros de um pack nao limita bytes", agora no ponto de
			// DELECAO em vez do de levantamento: o pack e a unidade, apagar um registro nao libera
			// byte nenhum e o torrent continua no disco. Sem isso, o ciclo seguinte ao que trouxe o
			// pack apagaria os episodios acima do limite e o gate de espaco reabriria em seguida —
			// loop de baixar-e-apagar. Nao apaga, e tambem nao baixa mais nada deste anime.
			logger.Logger.Debug().
				Str("episode", epName).
				Msg("Skipping episode: batch record above the per-anime limit is never deleted")
			return false, false
		}
		logger.Logger.Info().
			Str("episode", epName).
			Int("downloaded_episodes", *downloadedEpisodes).
			Int("max_episodes", maxEpisodes).
			Msg("Deleting episode: max episodes exceeded")
		return false, true
	}

	*downloadedEpisodes++
	...resto igual...
}
```

`checkEpisode` (`episodes.go:331`): acrescente `isBatch bool` ao fim da lista de parâmetros e repasse: `return handleAlreadySavedEpisode(maxEpisodes, downloadedEpisodes, isInTorrents, isBatch, epName)`.

`selectEpisodes`: passe `savedEp.IsBatch` na chamada de `checkEpisode`.

`buildWatchedKeepSet` (`episodes.go:288`): troque o parâmetro `savedEpisodesMap map[files.EpisodeKey]bool` por `savedEpisodesFullMap map[files.EpisodeKey]files.EpisodeStruct` e o teste de pertinência por:

```go
	for _, ep := range episodes {
		key := files.EpisodeKey{AnimeID: animeID, Episode: ep.Episode}
		saved, ok := savedEpisodesFullMap[key]
		// Registro de pack fica FORA do keep-set: manter 3 dos 100 de um pack nao guarda 3
		// episodios, guarda o pack inteiro — allEpisodesInDeleteSet daria false, o torrent ficaria
		// no disco, o espaco nao voltaria e o pack seguinte nunca viria. O keep-set nao tem a
		// granularidade que promete dentro de um pack.
		if ep.Episode <= progress && ok && !saved.IsBatch {
			watchedSaved = append(watchedSaved, key)
		}
	}
```

E na chamada em `processAnimeEpisodes`, passe `savedEpisodesFullMap`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: PASS. Se `episodes_test.go` tiver caso de `watched_episodes_to_keep` com registros marcados `IsBatch`, ele muda de expectativa — nesse caso ajuste o fixture para `IsBatch: false` (o caso que ele quer provar é o de episódio solto) e adicione um comentário apontando esta task.

- [ ] **Step 5: Add the full-rotation test**

Ainda em `selection_test.go`:

```go
// Rodizio completo: com pack 1-100 salvo e progresso 100, os 100 registros entram no delete set,
// allEpisodesInDeleteSet passa e o torrent inteiro sai — e e isso que libera o espaco para o
// 101-200 no ciclo seguinte. Com progresso 50, nada sai (ha irmaos vivos) e nada novo entra.
func TestSelectEpisodes_PackRotation(t *testing.T) {
	anime := animeWithEpisodes(200, anilist.MediaStatusReleasing, false, "")
	build := func(progress int) (map[files.EpisodeKey]bool, map[files.EpisodeKey]files.EpisodeStruct, anilist.MediaList) {
		a := anime
		a.Progress = progress
		savedMap := make(map[files.EpisodeKey]bool, 100)
		savedFull := make(map[files.EpisodeKey]files.EpisodeStruct, 100)
		for i := 1; i <= 100; i++ {
			key := files.EpisodeKey{AnimeID: a.Media.Id, Episode: i}
			savedMap[key] = true
			savedFull[key] = files.EpisodeStruct{AnimeID: a.Media.Id, EpisodeNumber: i, EpisodeHash: "pack", IsBatch: true}
		}
		return savedMap, savedFull, a
	}

	savedMap, savedFull, watchedAll := build(100)
	episodes := anilist.EpisodeList(watchedAll, firstEpisodeToConsider(watchedAll, nil))
	keepSet := buildWatchedKeepSet(3, watchedAll.Media.Id, episodes, savedFull, watchedAll.Progress)
	sel := selectEpisodes(limitsConfig(), 12, watchedAll, episodes, savedMap, savedFull, map[string]bool{"pack": true}, keepSet, nil)

	deleteSet := make(map[files.EpisodeKey]bool, len(sel.keysToDelete))
	for _, k := range sel.keysToDelete {
		deleteSet[k] = true
	}
	group := make([]files.EpisodeStruct, 0, 100)
	for _, ep := range savedFull {
		group = append(group, ep)
	}
	if !allEpisodesInDeleteSet(group, deleteSet) {
		t.Errorf("com progresso 100 o grupo inteiro deve sair (%d de 100 no delete set)", len(deleteSet))
	}

	savedMap, savedFull, halfWatched := build(50)
	episodes = anilist.EpisodeList(halfWatched, firstEpisodeToConsider(halfWatched, nil))
	keepSet = buildWatchedKeepSet(3, halfWatched.Media.Id, episodes, savedFull, halfWatched.Progress)
	sel = selectEpisodes(limitsConfig(), 12, halfWatched, episodes, savedMap, savedFull, map[string]bool{"pack": true}, keepSet, nil)
	if len(sel.toDownload) != 0 {
		t.Errorf("com progresso 50 nada novo deve ser buscado, obteve %d", len(sel.toDownload))
	}
}
```

Rode: `go test ./src/internal/daemon/ -run TestSelectEpisodes_PackRotation -v` → PASS.

- [ ] **Step 6: Commit**

```bash
git add src/internal/daemon
git commit -m "feat(daemon): registro de pack imune a poda por limite e ao keep-set de assistidos"
```

---

### Task 8: `AnimeSettings.Progress` e o `PUT` que faz merge

O ritmo do rodízio depende do progresso avançar. Anime avulso não tem progresso (`GetMediaByID` devolve `Progress: 0` fixo), então ele ganha o dado que falta — no store por anime que já existe.

**Files:**
- Modify: `src/internal/files/filemanager.go:149-151` (`AnimeSettings`)
- Modify: `src/internal/api/endpoint_anime_settings.go:11-13,47-64`
- Modify: `src/internal/frontend/src/lib/api/client.ts:279-281` (`AnimeSettings`)
- Test: `src/internal/api/endpoint_anime_settings_test.go` (novo, se não existir; caso exista, some os casos)

**Interfaces:**
- Consumes: nada.
- Produces:
  - `files.AnimeSettings{ CustomSearchQuery string \`json:"custom_search_query,omitempty"\`; Progress int \`json:"progress,omitempty"\` }`
  - `PUT /animes/{id}/settings` com corpo parcial: campo ausente preserva o salvo; `progress` negativo → 400.
  - TS: `interface AnimeSettings { custom_search_query?: string; progress?: number }`

- [ ] **Step 1: Write the failing tests**

Crie `src/internal/api/endpoint_anime_settings_test.go` (ou some ao existente; o mock de FileManager está em `endpoint_config_test.go` e é reaproveitável no pacote):

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// O PUT que a tela dispara manda UM campo. Sem merge, salvar a busca customizada zeraria o
// progresso e vice-versa.
func TestPutAnimeSettings_PartialUpdateMergesOverTheSaved(t *testing.T) {
	server, fm := newSettingsTestServer(t)
	fm.animeSettings = map[int]files.AnimeSettings{7: {CustomSearchQuery: "one piece 1080", Progress: 48}}

	rec := putSettings(t, server, 7, `{"custom_search_query":"one piece"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}
	if got := fm.animeSettings[7]; got.Progress != 48 || got.CustomSearchQuery != "one piece" {
		t.Errorf("esperava progresso preservado e query nova, obteve %+v", got)
	}

	rec = putSettings(t, server, 7, `{"progress":52}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}
	if got := fm.animeSettings[7]; got.Progress != 52 || got.CustomSearchQuery != "one piece" {
		t.Errorf("esperava query preservada e progresso novo, obteve %+v", got)
	}
}

func TestPutAnimeSettings_RejectsNegativeProgress(t *testing.T) {
	server, _ := newSettingsTestServer(t)

	if rec := putSettings(t, server, 7, `{"progress":-1}`); rec.Code != http.StatusBadRequest {
		t.Errorf("esperava 400 para progresso negativo, obteve %d", rec.Code)
	}
}
```

Monte `newSettingsTestServer` e `putSettings` **reaproveitando** o mock de FileManager que o pacote já tem (`mockFileManager` em `endpoint_config_test.go`); se ele não guardar `AnimeSettings` num mapa gravável, acrescente o campo lá em vez de criar um segundo mock. `putSettings` é:

```go
func putSettings(t *testing.T, server *Server, id int, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/animes/"+strconv.Itoa(id)+"/settings", strings.NewReader(body))
	req.SetPathValue("id", strconv.Itoa(id))
	rec := httptest.NewRecorder()
	handleAnimeSettings(server)(rec, req)
	return rec
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./src/internal/api/ -run TestPutAnimeSettings -v`
Expected: FAIL — `Progress` não existe / o handler sobrescreve.

- [ ] **Step 3: Write the implementation**

`src/internal/files/filemanager.go:149`:

```go
type AnimeSettings struct {
	CustomSearchQuery string `json:"custom_search_query,omitempty"`
	// Progress e o progresso MANUAL, e existe para o anime avulso: GetMediaByID devolve
	// Progress: 0 fixo, e sem progresso nada nunca e "assistido", a poda nunca roda e o rodizio
	// de packs sucessivos nao tem o que o mova. Ausente le 0, que e o comportamento de antes.
	// Anime de lista nunca usa este campo: quem manda la e a AniList.
	Progress int `json:"progress,omitempty"`
}
```

`src/internal/api/endpoint_anime_settings.go`:

```go
// Os dois campos sao PONTEIROS porque o PUT e parcial: a tela dispara
// updateAnimeSettings(id, { custom_search_query }) e, com um segundo campo no struct, montar um
// AnimeSettings do zero zeraria o progresso salvo (e vice-versa).
type animeSettingsRequest struct {
	CustomSearchQuery *string `json:"custom_search_query"`
	Progress          *int    `json:"progress"`
}
```

e o caso `http.MethodPut`:

```go
		case http.MethodPut:
			var req animeSettingsRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				JSONError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
				return
			}
			if req.Progress != nil && *req.Progress < 0 {
				JSONError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Progress must be non-negative")
				return
			}

			existing, err := server.FileManager.LoadAnimeSettings(id)
			if err != nil {
				logger.Logger.Error().Err(err).Int("anime_id", id).Msg("Failed to load anime settings")
				JSONInternalError(w, err)
				return
			}

			settings := files.AnimeSettings{}
			if existing != nil {
				settings = *existing
			}
			if req.CustomSearchQuery != nil {
				settings.CustomSearchQuery = *req.CustomSearchQuery
			}
			if req.Progress != nil {
				settings.Progress = *req.Progress
			}

			if err := server.FileManager.SaveAnimeSettings(id, settings); err != nil {
				logger.Logger.Error().Err(err).Int("anime_id", id).Msg("Failed to save anime settings")
				JSONInternalError(w, err)
				return
			}

			JSONSuccess(w, http.StatusOK, nil)
```

`src/internal/frontend/src/lib/api/client.ts`:

```ts
export interface AnimeSettings {
  custom_search_query?: string
  /** Progresso manual — só é lido para anime avulso (o de lista vem da AniList). */
  progress?: number
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... && cd src/internal/frontend && bun run check`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/internal/files/filemanager.go src/internal/api/endpoint_anime_settings.go src/internal/api/endpoint_anime_settings_test.go src/internal/frontend/src/lib/api/client.ts
git commit -m "feat(api): progresso por anime em AnimeSettings, com PUT parcial"
```

---

### Task 9: Injetar o progresso salvo no `MediaList` sintético

Três linhas em cada um dos três pontos que montam o avulso. Depois disso `shouldSkipEpisode`, `firstEpisodeToConsider`, `buildWatchedKeepSet`, a poda de assistidos e `EpisodesWatched` funcionam em avulso sem saber que é avulso.

**Files:**
- Modify: `src/internal/daemon/standalone.go:54-68` (`appendStandaloneAnimes`), `:91-98` (`DownloadStandaloneAnime`)
- Modify: `src/internal/api/standalone.go:30-39` (`resolveMediaList`), `:47-62` (`appendStandaloneEntries`), e os 3 chamadores em `endpoint_anime_episodes.go:79` e `endpoint_animes.go:174,241`
- Test: `src/internal/daemon/standalone_test.go`, `src/internal/api/` (o teste de listagem/detalhe mais próximo)

**Interfaces:**
- Consumes: `files.AnimeSettings.Progress` (Task 8).
- Produces:
  - `func daemon.withStandaloneProgress(fm FileManagerInterface, ml *anilist.MediaList) *anilist.MediaList`
  - `func api.withStandaloneProgress(fm FileManagerInterface, ml *anilist.MediaList) *anilist.MediaList`
  - `api.resolveMediaList(fm FileManagerInterface, id int, usernames []string, standalone map[int]bool)` e `api.appendStandaloneEntries(fm FileManagerInterface, entries []anilist.MediaList, standalone, covered map[int]bool)` — ganham `fm` como primeiro parâmetro.

- [ ] **Step 1: Write the failing tests**

Em `src/internal/daemon/standalone_test.go`:

```go
// Sem progresso, depois do primeiro pack o avulso para para sempre: nada nunca e "assistido", a
// poda nunca roda e o rodizio nao tem o que o mova.
func TestAppendStandaloneAnimes_InjectsSavedProgress(t *testing.T) {
	restore := anilist.MockGetMediaByID(func(id int) (*anilist.MediaList, error) {
		title := "Standalone Anime"
		return &anilist.MediaList{Id: id, Media: anilist.Media{Id: id, Title: anilist.Title{English: &title}}}, nil
	})
	defer restore()

	fm := &mockStandaloneFM{settings: map[int]files.AnimeSettings{42: {Progress: 48}}}

	merged := appendStandaloneAnimes(fm, nil, []int{42})

	if len(merged) != 1 {
		t.Fatalf("esperava 1 anime, obteve %d", len(merged))
	}
	if merged[0].Progress != 48 {
		t.Errorf("esperava Progress 48, obteve %d", merged[0].Progress)
	}
	if got := firstEpisodeToConsider(merged[0], nil); got != 49 {
		t.Errorf("a lista deve começar no 49, obteve %d", got)
	}
}

// Sem AnimeSettings gravado: Progress 0, comportamento de antes.
func TestAppendStandaloneAnimes_NoSettingsMeansZeroProgress(t *testing.T) {
	restore := anilist.MockGetMediaByID(func(id int) (*anilist.MediaList, error) {
		title := "Standalone Anime"
		return &anilist.MediaList{Id: id, Media: anilist.Media{Id: id, Title: anilist.Title{English: &title}}}, nil
	})
	defer restore()

	merged := appendStandaloneAnimes(&mockStandaloneFM{}, nil, []int{42})

	if len(merged) != 1 || merged[0].Progress != 0 {
		t.Errorf("esperava Progress 0, obteve %+v", merged)
	}
}
```

**Antes de escrever isso**, confirme como `standalone_test.go` já injeta `anilist.GetMediaByID` (`grep -n "GetMediaByID" src/internal/daemon/standalone_test.go src/internal/anilist/standalone.go`) e use o mecanismo existente — se não houver mock, o teste deve montar o cenário do jeito que os testes vizinhos de `appendStandaloneAnimes` já montam, sem introduzir indireção nova em produção. Da mesma forma, reaproveite o mock de FileManager que o arquivo já tem em vez de criar `mockStandaloneFM` se ele já existir com outro nome.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./src/internal/daemon/ -run TestAppendStandaloneAnimes -v`
Expected: FAIL com `Progress 0` onde se espera 48.

- [ ] **Step 3: Write the implementation — daemon**

Em `src/internal/daemon/standalone.go`:

```go
// withStandaloneProgress da ao MediaList sintetico do avulso o progresso salvo em AnimeSettings.
//
// E a alternativa a um caminho paralelo: com o dado no lugar, shouldSkipEpisode,
// firstEpisodeToConsider, buildWatchedKeepSet, a poda de assistidos e EpisodesWatched funcionam em
// avulso sem nenhum "if isStandalone". Falha de leitura vale como progresso 0 — que e o
// comportamento anterior a este campo.
func withStandaloneProgress(fm FileManagerInterface, ml *anilist.MediaList) *anilist.MediaList {
	if ml == nil {
		return nil
	}
	if s, err := fm.LoadAnimeSettings(ml.Media.Id); err == nil && s != nil {
		ml.Progress = s.Progress
	}
	return ml
}
```

Em `appendStandaloneAnimes`, depois do guard de `ml == nil` (linha 64), passe por ela: `merged = append(merged, *withStandaloneProgress(fileManager, ml))`.

Em `DownloadStandaloneAnime`, depois do guard `anime == nil` (linha 97): `anime = withStandaloneProgress(fm, anime)`.

- [ ] **Step 4: Write the implementation — api**

Em `src/internal/api/standalone.go`, acrescente o gêmeo (as interfaces de FileManager dos dois pacotes são diferentes, então não há helper compartilhado) e passe `fm` às duas funções:

```go
// withStandaloneProgress — ver o gemeo em daemon/standalone.go. Sem ele a tela mostraria 0
// assistidos para um avulso cujo progresso o usuario acabou de gravar.
func withStandaloneProgress(fm FileManagerInterface, ml *anilist.MediaList) *anilist.MediaList {
	if ml == nil {
		return nil
	}
	if s, err := fm.LoadAnimeSettings(ml.Media.Id); err == nil && s != nil {
		ml.Progress = s.Progress
	}
	return ml
}

func resolveMediaList(fm FileManagerInterface, id int, usernames []string, standalone map[int]bool) (*anilist.MediaList, error) {
	ml, err := anilist.GetAnimeInfo(id, usernames)
	if err != nil {
		return nil, err
	}
	if ml != nil || !standalone[id] {
		return ml, err
	}
	ml, err = anilist.GetMediaByID(id)
	if err != nil {
		return nil, err
	}
	return withStandaloneProgress(fm, ml), nil
}
```

Em `appendStandaloneEntries`, a assinatura passa a `(fm FileManagerInterface, entries []anilist.MediaList, standalone, covered map[int]bool)` e o append vira `entries = append(entries, *withStandaloneProgress(fm, ml))`.

Atualize os chamadores:
- `endpoint_anime_episodes.go:79` → `resolveMediaList(server.FileManager, id, config.AnilistUsernames, loadStandaloneSet(server.FileManager))`
- `endpoint_animes.go:174` → `appendStandaloneEntries(server.FileManager, nil, standaloneSet, covered)`
- `endpoint_animes.go:241` → `resolveMediaList(fm, info.AnimeID, usernames, standalone)` — passe o FileManager pelo mesmo caminho que `usernames`/`standalone` já chegam nessa função (se ela não recebe o server, adicione o parâmetro na assinatura dela e no seu chamador).

A ordem de `verification.go:539` continua valendo e pelo mesmo motivo: os avulsos entram DEPOIS do `DedupeByMedia`, e um id que também esteja numa lista real é removido do arquivo em vez de competir — o progresso salvo nunca disputa com o da AniList.

- [ ] **Step 5: Add the API-side test**

No teste de listagem de animes do pacote `api` (o que exercita `mergeCurrentAniListAnimes`/`handleAnimes`), acrescente um caso: avulso com `AnimeSettings{Progress: 48}` responde `episodes_watched: 48`. Reaproveite o servidor de teste do arquivo.

- [ ] **Step 6: Run the suites**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add src/internal/daemon src/internal/api
git commit -m "feat: injetar progresso salvo no MediaList sintetico do anime avulso"
```

---

### Task 10: `AnimeDetail` — contagem de assistidos e progresso editável em avulso

**Files:**
- Modify: `src/internal/frontend/src/routes/AnimeDetail.svelte` (cabeçalho: linhas 688-706; script: estado + handler; `runAction`/`ACTION_LABEL`; `episodeActions` do menu)
- Modify: `src/internal/frontend/src/lib/domain/episodeActions.ts` (nova ação `watchedHere`)
- Modify: `src/internal/frontend/messages/en.json`, `messages/pt-BR.json`
- Test: `src/internal/frontend/tests/unit/episodeActions.test.ts`, `src/internal/frontend/tests/component/AnimeDetail.progress.test.ts` (novo)

**Interfaces:**
- Consumes: `updateAnimeSettings(animeId, { progress })` (Task 8); `anime.episodes_watched` já preenchido para avulso (Task 9).
- Produces: `EpisodeActionId` ganha `'watchedHere'`; `episodeActions(ep, torrent, opts?: { standalone?: boolean })` — o parâmetro é opcional para não mexer nos chamadores de teste existentes.

- [ ] **Step 1: Write the failing tests**

Em `src/internal/frontend/tests/unit/episodeActions.test.ts`, acrescente:

```ts
describe('watchedHere', () => {
  it('offers "watched up to here" only for standalone animes', () => {
    const ep = { episode_number: 5, airing_at: 0, time_until_airing: 0, is_aired: true, is_watched: false, is_downloaded: true }

    const standalone = episodeActions(ep, undefined, { standalone: true })
    expect(standalone.menu.map(a => a.id)).toContain('watchedHere')

    const fromList = episodeActions(ep, undefined)
    expect(fromList.menu.map(a => a.id)).not.toContain('watchedHere')
  })
})
```

Crie `src/internal/frontend/tests/component/AnimeDetail.progress.test.ts`, no molde de `AnimeDetail.torrentPolling.test.ts` (mesmo `vi.mock` do client — **inclua todos os exports que a tela importa**, incluindo `deleteTorrent`, `removeStandaloneAnime` e `getTorrents`):

```ts
it('standalone anime shows the watched count and a progress input', async () => {
  vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({ is_standalone: true, episodes_watched: 48 })])
  vi.mocked(client.getAnimeDetail).mockResolvedValue(detailWith(42))
  vi.mocked(client.getTorrents).mockResolvedValue([])

  const { getByLabelText, container } = render(AnimeDetail, { params: { id: '42' } })
  await tick(); await tick()

  const input = getByLabelText(/assistidos|watched/i) as HTMLInputElement
  expect(input.value).toBe('48')
  expect(container.textContent).toMatch(/48/)
})

it('anime from a list shows the watched count without an input', async () => {
  vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({ is_standalone: false, episodes_watched: 7 })])
  vi.mocked(client.getAnimeDetail).mockResolvedValue(detailWith(42))
  vi.mocked(client.getTorrents).mockResolvedValue([])

  const { queryByLabelText, container } = render(AnimeDetail, { params: { id: '42' } })
  await tick(); await tick()

  expect(queryByLabelText(/assistidos|watched/i)).toBeNull()
  expect(container.textContent).toMatch(/7/)
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/internal/frontend && bun run test:unit && bun run test:component`
Expected: FAIL (`watchedHere` inexistente; nenhum input de progresso).

- [ ] **Step 3: i18n**

Acrescente a `messages/en.json`:

```json
  "detail_progress_label": "Watched episodes",
  "detail_progress_hint": "Manual progress for a standalone anime. Raising it deletes what is behind it (respecting \"watched episodes to keep\" outside a pack, and the whole pack inside one). With \"delete watched episodes\" off nothing is deleted and no new pack is fetched.",
  "detail_btn_watched_here": "Watched up to here",
  "detail_toast_progress_saved": "Progress saved",
  "detail_toast_progress_error": "Failed to save progress"
```

e a `messages/pt-BR.json`:

```json
  "detail_progress_label": "Episódios assistidos",
  "detail_progress_hint": "Progresso manual do anime avulso. Aumentar apaga o que ficou para trás (respeitando \"quantos assistidos manter\" fora de pack, e o pack inteiro dentro dele). Com \"apagar episódios assistidos\" desligado nada é apagado e nenhum pack novo vem.",
  "detail_btn_watched_here": "Assisti até aqui",
  "detail_toast_progress_saved": "Progresso salvo",
  "detail_toast_progress_error": "Falha ao salvar o progresso"
```

- [ ] **Step 4: `episodeActions.ts`**

```ts
export type EpisodeActionId = 'download' | 'redownload' | 'delete' | 'release' | 'replace' | 'watchedHere'

const WATCHED_HERE: Action = { id: 'watchedHere', labelKey: 'watchedHere', variant: 'ghost' }

export interface EpisodeActionOptions {
  /**
   * Anime avulso. "Assisti até aqui" só existe aí: em anime de lista o progresso vem da AniList
   * e é sobrescrito a cada passe, então gravá-lo à mão salvaria um número que ninguém lê.
   */
  standalone?: boolean
}

export function episodeActions(
  ep: AnimeEpisodeInfo,
  torrent: TorrentInfo | undefined,
  opts: EpisodeActionOptions = {},
): EpisodeActionSet {
  const set = classify(ep, torrent)
  if (!opts.standalone || !ep.is_aired) return set
  return { ...set, menu: [...set.menu, WATCHED_HERE] }
}
```

Renomeie o corpo atual da função para `classify` (mesma cascata, mesmos comentários) e mantenha `episodeActions` como o wrapper acima.

- [ ] **Step 5: `AnimeDetail.svelte`**

No script:

```ts
  // Progresso manual do avulso. Prefill de `anime.episodes_watched`, que já traz o valor salvo
  // (o backend injeta AnimeSettings.progress no MediaList sintético).
  let progressInput = 0;
  let progressSaving = false;
  $: progressInput = anime?.episodes_watched ?? 0;

  async function saveProgress(value: number) {
    progressSaving = true;
    try {
      await updateAnimeSettings(animeId, { progress: value });
      toast.success(m.detail_toast_progress_saved());
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_toast_progress_error());
    } finally {
      progressSaving = false;
    }
  }
```

Acrescente `watchedHere: m.detail_btn_watched_here,` ao `ACTION_LABEL` e `case "watchedHere": return saveProgress(ep.episode_number);` ao `runAction`.

Na derivação de `rows`, passe o flag: `const actions = episodeActions(ep, torrent, { standalone: !!anime?.is_standalone });`.

No cabeçalho (linhas 688-706), a contagem passa a ser a mesma para os dois casos, e o avulso ganha o input:

```svelte
        {#if anime}
          <p class="mt-2 font-mono text-caption text-subtle">
            {$locale && m.detail_counts({
              downloaded: anime.episodes_downloaded,
              total: anime.total_episodes || "?",
              watched: anime.episodes_watched,
            })}
            {#if detail?.status}
              · {m.detail_anilist_status({ status: detail.status })}
            {/if}
          </p>

          <!-- Avulso não tem progresso na AniList: é aqui que ele mora, e é o progresso que move
               o rodízio de packs (sem ele, depois do primeiro pack o anime para para sempre). -->
          {#if anime.is_standalone}
            <div class="mt-2 flex flex-wrap items-center gap-2">
              <label for="standalone-progress" class="text-copy text-body">
                {$locale && m.detail_progress_label()}
              </label>
              <input
                id="standalone-progress"
                type="number"
                min="0"
                bind:value={progressInput}
                class="w-20 rounded-field border border-default bg-control px-2 py-1 text-copy text-heading outline-none focus:border-accent"
                on:keydown={(e) => { if (e.key === 'Enter') saveProgress(progressInput); }}
              />
              <Button variant="ghost" disabled={progressSaving} on:click={() => saveProgress(progressInput)}>
                {progressSaving ? "..." : ($locale && m.common_save())}
              </Button>
            </div>
            <p class="mt-1 text-caption text-subtle">{$locale && m.detail_progress_hint()}</p>
          {/if}
        {/if}
```

`detail_counts_no_watched` deixa de ser usada: remova a chave dos dois `messages/*.json` (`grep -rn "detail_counts_no_watched" src/` para confirmar que não sobrou chamador).

- [ ] **Step 6: Run the suites**

Run: `cd src/internal/frontend && bun run i18n:compile && bun run test:unit && bun run test:component && bun run check`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add src/internal/frontend
git commit -m "feat(ui): progresso manual e contagem de assistidos no anime avulso"
```

---

### Task 11: `AnimeDetail` — pack é uma linha

Com pack virando comum, One Piece renderizaria 1122 linhas em que 100 oferecem "Excluir" e "Rebaixar" que não fazem o que dizem (excluir um episódio de dentro de um pack não libera byte nenhum; "Rebaixar" remove o registro e busca episódio solto, que em One Piece não existe → 500 e arquivo órfão).

**Files:**
- Modify: `src/internal/frontend/src/routes/AnimeDetail.svelte` (derivação de `rows`, os dois `{#each}`, imports)
- Modify: `src/internal/frontend/messages/en.json`, `messages/pt-BR.json`
- Modify: `src/internal/frontend/tests/component/AnimeDetail.torrentPolling.test.ts` (somar `deleteTorrent` ao `vi.mock`)
- Test: `src/internal/frontend/tests/component/AnimeDetail.batchRow.test.ts` (novo)

**Interfaces:**
- Consumes: `deleteTorrent(hash, { keepData, block })` e `replaceAnimeWithMagnet(animeId, magnet)` (ambos já existem em `client.ts`); `TorrentDeleteDialog` (já existe).
- Produces: nada para outras tasks. Frontend puro — nenhum endpoint, campo de resposta ou handler novo.

- [ ] **Step 1: Write the failing test**

Crie `src/internal/frontend/tests/component/AnimeDetail.batchRow.test.ts`:

```ts
it('renders 100 episodes sharing one hash as a single row', async () => {
  const episodes = Array.from({ length: 100 }, (_, i) => ({
    episode_number: i + 1, airing_at: 0, time_until_airing: 0,
    is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'pack',
  }))
  episodes.push({
    episode_number: 101, airing_at: 0, time_until_airing: 0,
    is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'solo',
  })

  vi.mocked(client.getAnimeDetail).mockResolvedValue({ anime_id: 42, total_episodes: 101, progress: 0, status: 'CURRENT', episodes })
  vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
  vi.mocked(client.getTorrents).mockResolvedValue([])

  const { container, getAllByText } = render(AnimeDetail, { params: { id: '42' } })
  await tick(); await tick()

  // Desktop e mobile renderizam a mesma definição, então cada linha aparece duas vezes no DOM.
  const rows = container.querySelectorAll('[data-episode-row]')
  expect(rows.length).toBe(2 * 2) // 1 linha de pack + 1 episódio solto, nas duas vistas
  expect(getAllByText(/1\s*[–-]\s*100/).length).toBeGreaterThan(0)
})

it('the pack row does not offer redownload', async () => {
  const episodes = [1, 2].map(n => ({
    episode_number: n, airing_at: 0, time_until_airing: 0,
    is_aired: true, is_watched: false, is_downloaded: true, episode_hash: 'pack',
  }))
  vi.mocked(client.getAnimeDetail).mockResolvedValue({ anime_id: 42, total_episodes: 2, progress: 0, status: 'CURRENT', episodes })
  vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
  vi.mocked(client.getTorrents).mockResolvedValue([])

  const { queryByText } = render(AnimeDetail, { params: { id: '42' } })
  await tick(); await tick()

  expect(queryByText(/rebaixar|redownload/i)).toBeNull()
})
```

Copie o bloco `vi.mock('../../src/lib/api/client.js', ...)` de `AnimeDetail.torrentPolling.test.ts` e **acrescente** `deleteTorrent: vi.fn()` e `removeStandaloneAnime: vi.fn()`; faça a mesma adição no arquivo original (a tela passa a importar `deleteTorrent`, e um export faltando no mock deixa o binding `undefined`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/internal/frontend && bun run test:component`
Expected: FAIL — 202 linhas em vez de 4.

- [ ] **Step 3: i18n**

`messages/en.json`: `"detail_batch_row_title": "Episodes {first}–{last}"`, `"detail_batch_actions_menu": "More actions for the pack"`.
`messages/pt-BR.json`: `"detail_batch_row_title": "Episódios {first}–{last}"`, `"detail_batch_actions_menu": "Mais ações do pack"`.

- [ ] **Step 4: Agrupar as linhas**

No script de `AnimeDetail.svelte`, importe `deleteTorrent` do client e `TorrentDeleteDialog from "../components/TorrentDeleteDialog.svelte"`, e substitua a derivação de `rows` (linhas 246-263) por:

```ts
  // Uma passada monta tudo que cada linha precisa, para desktop e mobile lerem a MESMA
  // estrutura — é isso que impede as duas cópias de divergirem de novo.
  //
  // Um pack é UMA linha. A regra de detecção é a mesma que o backend já usa em
  // buildTorrentResponse: mais de um episódio no mesmo info hash é batch, diga o que disser a
  // flag. Excluir um episódio de dentro de um pack não libera byte nenhum (o hardlink da
  // biblioteca e a cópia de seed são o mesmo inode) e "Rebaixar" buscaria episódio solto, que
  // numa série longa não existe — então a linha de grupo oferece só o que é verdade para um pack.
  function buildRows(episodes: AnimeEpisodeInfo[], byEpisode: Map<number, TorrentInfo>) {
    const counts = new Map<string, number>();
    for (const ep of episodes) {
      if (ep.episode_hash) counts.set(ep.episode_hash, (counts.get(ep.episode_hash) ?? 0) + 1);
    }

    const out = [];
    const seenPack = new Set<string>();

    for (const ep of episodes) {
      const hash = ep.episode_hash;
      if (hash && (counts.get(hash) ?? 0) >= 2) {
        if (seenPack.has(hash)) continue;
        seenPack.add(hash);

        const group = episodes.filter((e) => e.episode_hash === hash);
        const numbers = group.map((e) => e.episode_number);
        const torrent = byEpisode.get(numbers[0]);
        out.push({
          kind: "batch" as const,
          key: `pack-${hash}`,
          hash,
          ep: group[0],
          torrent,
          inFlight: !!torrent && !torrent.completed,
          chip: episodeChip(group[0], torrent),
          notes: "",
          meta: episodeMeta(group[0], torrent),
          principal: { id: "replace" as const, labelKey: "replace", variant: "ghost" as const },
          menu: [{ id: "delete", label: m.detail_btn_delete(), destructive: true as const }],
          title: m.detail_batch_row_title({ first: Math.min(...numbers), last: Math.max(...numbers) }),
          label: `${Math.min(...numbers)}–${Math.max(...numbers)}`,
        });
        continue;
      }

      const torrent = byEpisode.get(ep.episode_number);
      const actions = episodeActions(ep, torrent, { standalone: !!anime?.is_standalone });
      out.push({
        kind: "episode" as const,
        key: `ep-${ep.episode_number}`,
        hash: ep.episode_hash,
        ep,
        torrent,
        inFlight: !!torrent && !torrent.completed,
        chip: episodeChip(ep, torrent),
        notes: episodeNotes(ep),
        meta: episodeMeta(ep, torrent),
        principal: actions.principal,
        menu: menuItems(actions.menu),
        title: ep.episode_name || m.detail_ep_title({ number: ep.episode_number }),
        label: String(ep.episode_number),
      });
    }

    return out;
  }

  $: rows = $locale && buildRows(allEpisodes, torrentsByEpisode);

  // Ações da linha de grupo: Excluir vai pelo DELETE /torrents/{hash}, que já remove o torrent e
  // TODOS os registros do grupo como unidade; Substituir por magnet reusa o replace de anime, que
  // já é a operação de "trocar o pack inteiro". Não há "Rebaixar": para um pack ele seria alias de
  // Excluir, já que o loop rebusca no ciclo seguinte assim que o espaço volta.
  let packDeleteOpen = false;
  let pendingPackHash = "";
  let pendingPackLabel = "";

  function runPackAction(id: string, row: { hash: string; label: string }) {
    if (id === "delete") {
      pendingPackHash = row.hash;
      pendingPackLabel = row.label;
      packDeleteOpen = true;
      return;
    }
    replaceAnimeMagnet = "";
    replaceAnimeOpen = true;
  }

  async function confirmPackDelete(opts: { keepData: boolean; block: boolean }) {
    packDeleteOpen = false;
    try {
      await deleteTorrent(pendingPackHash, opts);
      toast.success(m.detail_toast_deleted({ number: pendingPackLabel }));
      await loadData(animeId);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : m.detail_toast_del_error());
    }
  }
```

Adicione o diálogo junto dos outros (perto da linha 597):

```svelte
<TorrentDeleteDialog
  bind:open={packDeleteOpen}
  name={pendingPackLabel}
  scope="anime"
  standalone={!!anime?.is_standalone}
  on:confirm={(e) => confirmPackDelete(e.detail)}
  on:cancel={() => (packDeleteOpen = false)}
/>
```

Nos dois `{#each}`, troque a chave e despache por tipo:

```svelte
        {#each rows || [] as row (row.key)}
          {@const isLoading = !!actionLoading[row.ep.episode_number]}
          {@const isSelected = row.kind === "episode" && selectedEpisodes.has(row.ep.episode_number)}
```

- No lugar do `<span>{row.ep.episode_number}</span>`, use `{row.label}`.
- O `Checkbox` da linha só é renderizado quando `row.kind === "episode"` (numa linha de grupo ele mentiria: as ações em lote são por episódio, e para um pack elas são exatamente as que não fazem o que dizem). Deixe a célula vazia (`<span></span>`) na linha de grupo para o grid não desalinhar.
- Os handlers: `on:click={() => row.kind === "batch" ? runPackAction(row.principal.id, row) : row.principal && runAction(row.principal.id, row.ep)}` e, no `ActionMenu`, `on:select={(e) => row.kind === "batch" ? runPackAction(e.detail, row) : runAction(e.detail as EpisodeActionId, row.ep)}`, com `triggerLabel={row.kind === "batch" ? m.detail_batch_actions_menu() : m.detail_actions_menu({ number: row.ep.episode_number })}`.
- A seleção em lote (`allSelected`, `selectedList`, `toggleSelectAll`) continua sobre `allEpisodes` — ela é por episódio e não muda.

- [ ] **Step 5: Run the suites**

Run: `cd src/internal/frontend && bun run i18n:compile && bun run test:unit && bun run test:component && bun run check`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add src/internal/frontend
git commit -m "feat(ui): pack renderiza como uma linha, com as acoes que valem para um pack"
```

---

### Task 12: Docs, Swagger e verificação final

**Files:**
- Modify: `docs/agents/config.md`, `docs/agents/architecture.md`, `docs/agents/decisions.md`, `docs/agents/testing.md`
- Modify: `docs/swagger/*` (gerado)
- Modify: `docs/TODO.md` (se houver item desta feature em aberto)

**Interfaces:** consome os nomes finais de todas as tasks anteriores.

- [ ] **Step 1: `config.md`**

- Delete a linha de `max_batch_episodes` (linha 16).
- `max_batch_torrent_size_gb`: default passa a `100`, descrição passa a "guarda única de pack" com a nota de que `100` cabe pack completo de série de temporada em 1080p e não cabe pack completo de One Piece (para série longa o que passa é pack parcial).
- Na lista de campos `>= 0` (linha 84): tire `max_batch_episodes` e mova `max_episodes_per_anime` para a lista dos que aceitam `0` como desligado.
- Documente `progress` em `AnimeSettings` (por anime, não é `Config`): progresso manual do avulso, ausente lê `0`.
- Nota de release, nas duas pontas: (a) quem já tem `max_batch_torrent_size_gb: 0` gravado segue com o filtro desligado e precisa ajustar à mão — `0` gravado é escolha explícita; (b) `max_episodes_per_anime = 0` abre a janela e One Piece resolve ~14 packs num ciclo só, freado apenas por `max_batch_torrent_size_gb` e `checkDiskSpace`.

- [ ] **Step 2: `architecture.md`**

- Tire `willBatchAnime` da tabela de símbolos (linha 135) e some `selectEpisodes`, `effectiveMax`, `windowEnd`, `pickBatches`, `coveringBatch`, `assignBatches`, `resolveMovie`, `magnetsByEpisode`, `partitionSearchResults`, `ScrapNyaaForAnime` e `ExtractBatchInfo`.
- Tire `ScrapNyaaForBatch`, `ScrapNyaaForMultipleEpisodes`, `resolveSearchStrategy` e `resolvedIsBatch`.
- Na lista de estratégias de busca (linha 163): a Estratégia 2 sai; entra a descrição de "packs resolvidos antes do laço, cobrindo a janela a partir do primeiro pendente".

- [ ] **Step 3: `decisions.md`**

Reescreva a entrada existente sobre `max_batch_episodes` (linhas 785-801) apontando para esta spec, e acrescente quatro entradas:

(a) **Elegibilidade a batch deixou de ser metadado e virou filtro de resultado.** O que importa é o torrent que está lá — tamanho, seeders, faixa coberta — e nada disso é conhecido antes da busca. `don't fix by`: reintroduzir um teto de contagem de episódios do anime; contagem de episódios é proxy ruim de tamanho (26 episódios em remux = 300 GB e passava; 500 em 480p = 80 GB e era bloqueado), e o gate de `FINISHED` excluía exatamente quem mais depende de pack.

(b) **Por que os packs cobrem a janela a partir do primeiro pendente, e por que os episódios fora da cobertura não caem em episódio solto.** Sem a janela, `batchResult[0]` iria para todos os pendentes e o episódio 800 receberia o magnet do pack 1-100. Sem o truncamento, o fallback de episódio solto dispararia `DownloadFailed`/`ReasonNotFound` para ~1000 episódios por passada em One Piece. O corte é sempre de prefixo porque a agenda vem em ordem crescente e os packs escolhidos são contíguos.

(c) **Episódio `IsBatch` é imune à poda por limite E ao keep-set de assistidos, e é isso que dá o rodízio de packs sem config nova.** O contador continua sendo incrementado pelos episódios de pack abaixo do limite — é isso que dá o ritmo de aquisição de graça. `watched_episodes_to_keep` não tem granularidade dentro de um pack: manter 3 dos 100 guarda o pack inteiro.

(d) **Por que o progresso de avulso mora em `AnimeSettings` e é injetado no `MediaList` sintético** em vez de virar caminho paralelo: com o dado no lugar, `shouldSkipEpisode`, `firstEpisodeToConsider`, `buildWatchedKeepSet`, a poda e `EpisodesWatched` funcionam sem nenhum `if isStandalone`. E o `PUT` parcial (ponteiros) porque um `PUT` só com `custom_search_query` zeraria o progresso.

Registre também o `ponytail:` do piso de paginação (Task 2) e o da guarda de faixa fantasma (Task 1) como tetos conhecidos.

- [ ] **Step 4: `testing.md`**

O mock de `nyaaSearcher` perde `searchBatch` e `searchMultiple` e ganha `searchAnime(titles, synonyms, episodes, customQuery)`, que devolve packs (`IsBatch: true`) e episódios (`Episode != nil`) na mesma lista. Mostre o novo `searcherFor` como referência.

- [ ] **Step 5: Swagger**

Run: `swag init -g src/cmd/daemon/main.go -o docs/swagger`
`Config` (perdeu um campo) e `animeSettingsRequest` (ganhou `progress`, campos viraram ponteiros) mudam. Se `swag` não estiver instalado, **não** edite `docs/swagger` à mão: informe o usuário e deixe o passo pendente.

- [ ] **Step 6: Full verification**

Run:
```bash
go test ./... && \
cd src/internal/frontend && bun run i18n:compile && bun run test:unit && bun run test:component && bun run check
```
Expected: tudo PASS. Cole a saída real ao reportar — nenhuma afirmação de "passou" sem o output (skill `verification-before-completion`).

Confirme também que nada ficou para trás:
```bash
grep -rn "MaxBatchEpisodes\|max_batch_episodes\|willBatchAnime\|resolvedIsBatch\|ScrapNyaaForBatch\|ScrapNyaaForMultipleEpisodes\|detail_counts_no_watched" src/ | grep -v _test.go
```
Expected: nenhuma linha.

- [ ] **Step 7: Commit**

```bash
git add docs src/internal
git commit -m "docs: atualizar config, arquitetura, decisoes e swagger para batch por filtro"
```

---

## Notas de execução

- **Tasks 1→7 são backend e sequenciais** (cada uma depende dos símbolos da anterior). **Tasks 8→9** também. **Tasks 10 e 11** tocam o mesmo arquivo (`AnimeDetail.svelte`) e devem rodar nesta ordem, não em paralelo. **Task 12** é a última.
- Se um teste existente falhar por um motivo que a spec não previu, pare e reporte antes de reescrevê-lo: um teste que quebra é a evidência mais barata de que uma premissa estava errada.
- `pickBatches` lê a faixa do **nome** do torrent. Todo fixture de pack em teste precisa de nome realista (`"[X] Anime 001-100 [1080p]"`), não `"batch"` — um nome sem faixa cai em "pack completo" e muda o resultado esperado.
