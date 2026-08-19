# Relatório da última verificação — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fazer o passe automático produzir um relatório do que impediu downloads (agregado por anime), guardá-lo em memória no `State`, expô-lo em `GET /api/v1/last-check`, e mostrá-lo nas telas de Status e de detalhe do anime — sem mudar nenhuma regra de decisão do daemon.

**Architecture:** Nenhum coletor global e nenhum mutex novo. Os dois filtros de busca (`filterBySize`/`filterBySeeders`) passam a devolver quantos descartaram; `checkEpisode` passa a devolver o motivo do skip; e cada anime devolve seus `[]Issue` pelo canal `animeProcessResult` que já existe. `AnimeVerification` agrega no mesmo laço que já junta `newEpisodes`/`checkedEpisodes`/`keysToDelete` e chama `state.SetLastCheckReport` **depois** do `SetLastCheckError(nil)` — porque `SetLastCheckError` passa a limpar o relatório. Um endpoint só, com dois consumidores: o Status renderiza tudo, o detalhe do anime filtra por `anime_id`.

**Tech Stack:** Go 1.x (daemon, `net/http`), Svelte 5 + Vite + Paraglide i18n (frontend), vitest (unit/component), Playwright (smoke), swag (Swagger).

## Global Constraints

- Spec de origem: [`docs/superpowers/specs/2026-08-11-relatorio-ultima-verificacao-design.md`](../specs/2026-08-11-relatorio-ultima-verificacao-design.md).
- Repo root: `/home/icaro/Projetos/AAD/AutoAnimeDownloader`. Import path dos pacotes Go: `AutoAnimeDownloader/src/internal/<pkg>`.
- **`go test ./...`** (na raiz) tem de passar ao fim de CADA task. Frontend: `cd src/internal/frontend && bun run test:unit && bun run test:component`.
- **NUNCA commitar ou dar push sem permissão explícita do usuário** (CLAUDE.md). Os passos "Commit" abaixo significam: deixe o `git add` feito e **pergunte** antes do `git commit`. Se o usuário já autorizou commits nesta sessão, commite com a mensagem indicada.
- Comentários novos em Go seguem o estilo dos arquivos vizinhos: português **sem acentuação** em `daemon/`, explicando *por quê*, não *o quê*. Não remova comentário existente que continue verdadeiro.
- Import order em Go (conventions.md): pacotes internos, depois stdlib, depois terceiros.
- **Nada em disco.** Nenhum arquivo novo de estado, nenhuma migração, nenhuma dependência nova.
- **Nenhuma regra de decisão do daemon muda.** Toda task abaixo só *observa* o que já é decidido. Se uma mudança sua alterar o que é baixado, ela está errada.
- Uma mensagem i18n por código, em **`messages/en.json` e `messages/pt-BR.json`** — as duas, sempre. O backend manda código + números; o frontend monta a frase.
- Um `ponytail:` comment é obrigatório em cada simplificação deliberada com teto conhecido (os pontos estão marcados nas Tasks 2 e 4).

## Desvios da spec (decididos aqui, com o motivo)

1. **`batch_skipped` não tem o valor `"too_many_episodes"`.** A spec descreve `willBatchAnime` devolvendo false porque `*Media.Episodes > MaxBatchEpisodes`. **Esse código não existe mais**: `grep -rn "MaxBatchEpisodes\|willBatchAnime\|max_batch_episodes" src/ --include=*.go` não retorna nada. A elegibilidade a pack hoje é `!isAnimeMovie(anime) && len(sel.toDownload) > 1` seguida de `pickBatches(...)` sobre o resultado **já filtrado** (`episodes.go:120-143`, decisions.md sobre batch por filtro de busca). Os valores possíveis passam a ser os quatro que o código de fato produz: `""` (nunca elegível — filme, ou um só episódio pendente), `"no_result"` (a busca não devolveu linha de pack nenhuma), `"above_size_limit"` (havia linhas de pack e o filtro cortou todas), `"no_coverage"` (packs sobreviveram ao filtro mas nenhum cobre a janela pendente). O papel de `batch_skipped` na spec — ser o *porquê* de `max_episodes_per_anime` estar valendo naquele anime — fica intacto.
2. **São 7 call sites afetados, não 17.** `filterSearchResults`: `episodes.go:172`, `episodes.go:236`, `episodes.go:345`, `episodes.go:346`, `debug.go:138`. `checkEpisode`: `episodes.go:55`, `episodes_test.go:242`. Mais `filterBySize`/`filterBySeeders` diretos em `limits_test.go:235,239,260,265,276`. A contagem da spec é de uma versão anterior do arquivo.
3. **Os números de linha da spec são de uma versão anterior.** `checkEpisode` está em `episodes.go:443` (não 342), `attemptDownloadWithRetries` em `531` (não 408/439), a decisão de disco em `episodes.go:209`. As tasks abaixo usam as linhas atuais.
4. **`GetLastCheckReport` devolve valor, não ponteiro.** O handler precisa preencher `pass_error` sem escrever no objeto compartilhado. Um `CheckReport` por valor resolve isso sem clonar as slices (que nunca são mutadas depois de publicadas).
5. **A cascata de problemas é alimentada só pelo fallback por episódio** (`episodes.go:167-176`), nunca pelo `partitionSearchResults` ou pelo `resolveMovie`. Motivo: é o fallback que dá a última palavra sobre aquele episódio — se ele achou magnet, não houve problema; se não achou, o `dropStats` dele é o que descreve por quê. Marcado com `ponytail:` na Task 4.

---

### Task 1: Os filtros de busca passam a contar o que descartaram

Hoje a informação "cortei 8 torrents por tamanho" só existe dentro de um `logger.Debug()`. Ela vira valor de retorno. Nada de comportamento muda nesta task — só assinaturas.

**Files:**
- Modify: `src/internal/daemon/search.go:25-86` (`filterBySize`, `filterBySeeders`, `filterSearchResults`, + novo tipo `dropStats`)
- Modify: `src/internal/daemon/episodes.go:172`, `:236`, `:332-347` (`partitionSearchResults`), `:127` (chamador de `partitionSearchResults`)
- Modify: `src/internal/daemon/debug.go:112`, `:138`
- Test: `src/internal/daemon/limits_test.go:225-279` (os dois testes de filtro existentes) + um teste novo de `filterSearchResults`

**Interfaces:**
- Consumes: nada de tasks anteriores.
- Produces:
  - `type dropStats struct { Input, BySize, BySeeders int }` (pacote `daemon`, arquivo `search.go`)
  - `func filterBySize(results []nyaa.TorrentResult, maxGB float64) ([]nyaa.TorrentResult, int)` — o `int` é quantos saíram
  - `func filterBySeeders(results []nyaa.TorrentResult, minSeeders int) ([]nyaa.TorrentResult, int)`
  - `func filterSearchResults(results []nyaa.TorrentResult, maxGB float64, minSeeders int) ([]nyaa.TorrentResult, dropStats)`
  - `func partitionSearchResults(configs *files.Config, results []nyaa.TorrentResult) (packs, singles []nyaa.TorrentResult, packStats dropStats)`

- [ ] **Step 1: Write the failing test**

Substitua os corpos de `TestFilterBySize` e `TestFilterBySeeders` em `src/internal/daemon/limits_test.go:225` e `:250` (as chamadas mudam de arity) e acrescente `TestFilterSearchResults_DropStats` logo depois:

```go
func TestFilterBySize(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)
	results := []nyaa.TorrentResult{
		{Name: "small", Size: 1 * gib},
		{Name: "huge", Size: 300 * gib},
		{Name: "unknown", Size: 0},
		{Name: "medium", Size: 5 * gib},
	}

	if got, dropped := filterBySize(results, 0); len(got) != 4 || dropped != 0 {
		t.Errorf("teto 0 não deve filtrar nada, sobraram %d e caíram %d", len(got), dropped)
	}

	got, dropped := filterBySize(results, 10)
	if len(got) != 3 {
		t.Fatalf("esperava 3 resultados, obteve %d (%+v)", len(got), got)
	}
	if dropped != 1 {
		t.Errorf("esperava 1 descartado por tamanho, obteve %d", dropped)
	}
	// Size == 0 fica (parsing quebrado não pode virar paralisação) e a ordem é preservada.
	want := []string{"small", "unknown", "medium"}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("posição %d: esperava %q, obteve %q", i, name, got[i].Name)
		}
	}
}

func TestFilterBySeeders(t *testing.T) {
	results := []nyaa.TorrentResult{
		{Name: "alive", Seeders: "412"},
		{Name: "dead", Seeders: "0"},
		{Name: "unknown", Seeders: "-"},
		{Name: "weak", Seeders: "3"},
	}

	if got, dropped := filterBySeeders(results, 0); len(got) != 4 || dropped != 0 {
		t.Errorf("piso 0 não deve filtrar nada, sobraram %d e caíram %d", len(got), dropped)
	}

	// Default: só o literalmente morto sai (e "-" conta como 0).
	got, dropped := filterBySeeders(results, 1)
	want := []string{"alive", "weak"}
	if len(got) != len(want) {
		t.Fatalf("esperava %d resultados, obteve %d (%+v)", len(want), len(got), got)
	}
	if dropped != 2 {
		t.Errorf("esperava 2 descartados por seeders, obteve %d", dropped)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("posição %d: esperava %q, obteve %q", i, name, got[i].Name)
		}
	}

	if got, _ := filterBySeeders(results, 5); len(got) != 1 || got[0].Name != "alive" {
		t.Errorf("piso 5 deveria sobrar só o alive, obteve %+v", got)
	}
}

// TestFilterSearchResults_DropStats: o dropStats e a unica coisa que distingue "o Nyaa nao
// devolveu nada" de "o filtro cortou tudo", e o relatorio da ultima verificacao depende dessa
// distincao para nao mentir dizendo "nenhum torrent encontrado" quando havia oito.
func TestFilterSearchResults_DropStats(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)

	t.Run("entrada vazia", func(t *testing.T) {
		got, stats := filterSearchResults(nil, 3, 1)
		if len(got) != 0 {
			t.Errorf("esperava lista vazia, obteve %+v", got)
		}
		if stats != (dropStats{}) {
			t.Errorf("esperava dropStats zerado, obteve %+v", stats)
		}
	})

	t.Run("corte só por tamanho", func(t *testing.T) {
		results := []nyaa.TorrentResult{
			{Name: "huge-a", Size: 8 * gib, Seeders: "50"},
			{Name: "huge-b", Size: 9 * gib, Seeders: "40"},
		}
		got, stats := filterSearchResults(results, 3, 1)
		if len(got) != 0 {
			t.Fatalf("esperava lista vazia, obteve %+v", got)
		}
		want := dropStats{Input: 2, BySize: 2, BySeeders: 0}
		if stats != want {
			t.Errorf("esperava %+v, obteve %+v", want, stats)
		}
	})

	t.Run("corte só por seeders", func(t *testing.T) {
		results := []nyaa.TorrentResult{
			{Name: "dead-a", Size: 1 * gib, Seeders: "0"},
			{Name: "dead-b", Size: 1 * gib, Seeders: "0"},
			{Name: "dead-c", Size: 1 * gib, Seeders: "0"},
		}
		got, stats := filterSearchResults(results, 3, 1)
		if len(got) != 0 {
			t.Fatalf("esperava lista vazia, obteve %+v", got)
		}
		want := dropStats{Input: 3, BySize: 0, BySeeders: 3}
		if stats != want {
			t.Errorf("esperava %+v, obteve %+v", want, stats)
		}
	})

	t.Run("corte pelos dois", func(t *testing.T) {
		results := []nyaa.TorrentResult{
			{Name: "huge", Size: 8 * gib, Seeders: "50"},
			{Name: "dead", Size: 1 * gib, Seeders: "0"},
			{Name: "ok", Size: 1 * gib, Seeders: "50"},
		}
		got, stats := filterSearchResults(results, 3, 1)
		if len(got) != 1 || got[0].Name != "ok" {
			t.Fatalf("esperava só o ok, obteve %+v", got)
		}
		want := dropStats{Input: 3, BySize: 1, BySeeders: 1}
		if stats != want {
			t.Errorf("esperava %+v, obteve %+v", want, stats)
		}
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./src/internal/daemon/ -run 'TestFilterBySize|TestFilterBySeeders|TestFilterSearchResults_DropStats' -v`
Expected: FAIL na compilação — `assignment mismatch: 2 variables but filterBySize returns 1 value` e `undefined: dropStats`.

- [ ] **Step 3: Write minimal implementation — `search.go`**

Em `src/internal/daemon/search.go`, acrescente o tipo logo depois do bloco `import` (antes de `type nyaaSearchFunc`):

```go
// dropStats conta o que os filtros de busca descartaram numa chamada.
//
// Existe porque "a lista voltou vazia" tem tres causas diferentes e so o contador as distingue:
// o relatorio da ultima verificacao precisa poder dizer "oito candidatos, todos acima de 3 GB"
// em vez de "nenhum torrent encontrado" — que e verdade tambem, e e a resposta menos acionavel
// das tres. Input e quantos entraram, ANTES de qualquer filtro.
type dropStats struct {
	Input     int
	BySize    int
	BySeeders int
}
```

Troque as três funções de filtro (`search.go:31`, `:63`, `:84`) preservando todos os comentários existentes acima delas:

```go
func filterBySize(results []nyaa.TorrentResult, maxGB float64) ([]nyaa.TorrentResult, int) {
	if maxGB <= 0 || len(results) == 0 {
		return results, 0
	}
	maxBytes := int64(maxGB * 1024 * 1024 * 1024)
	filtered := make([]nyaa.TorrentResult, 0, len(results))
	for _, tr := range results {
		if tr.Size == 0 {
			logger.Logger.Debug().Str("torrent", tr.Name).Msg("Size filter: unknown size, passing without check")
			filtered = append(filtered, tr)
			continue
		}
		if tr.Size > maxBytes {
			logger.Logger.Debug().
				Str("torrent", tr.Name).
				Int64("size_bytes", tr.Size).
				Int64("max_bytes", maxBytes).
				Msg("Size filter: discarding torrent above the size ceiling")
			continue
		}
		filtered = append(filtered, tr)
	}
	return filtered, len(results) - len(filtered)
}
```

```go
func filterBySeeders(results []nyaa.TorrentResult, minSeeders int) ([]nyaa.TorrentResult, int) {
	if minSeeders <= 0 || len(results) == 0 {
		return results, 0
	}
	filtered := make([]nyaa.TorrentResult, 0, len(results))
	for _, tr := range results {
		if seeders := nyaa.ParseSeeders(tr.Seeders); seeders < minSeeders {
			logger.Logger.Debug().
				Str("torrent", tr.Name).
				Int("seeders", seeders).
				Int("min_seeders", minSeeders).
				Msg("Seeders filter: discarding torrent below the seeders floor")
			continue
		}
		filtered = append(filtered, tr)
	}
	return filtered, len(results) - len(filtered)
}
```

```go
// filterSearchResults aplica os dois filtros de busca (teto de tamanho + piso de seeders) na
// ordem em que os quatro pontos de busca precisam deles, e devolve o que cada um cortou.
func filterSearchResults(results []nyaa.TorrentResult, maxGB float64, minSeeders int) ([]nyaa.TorrentResult, dropStats) {
	bySize, sizeDropped := filterBySize(results, maxGB)
	final, seedersDropped := filterBySeeders(bySize, minSeeders)
	return final, dropStats{Input: len(results), BySize: sizeDropped, BySeeders: seedersDropped}
}
```

- [ ] **Step 4: Write minimal implementation — os 5 call sites de produção**

`src/internal/daemon/episodes.go:172` (dentro do laço de download), troque:

```go
			singleResults := filterSearchResults(searcher.searchSingleEpisode(ep, anime.Media.Title, anime.Media.Synonyms, anime.Media.Relations, customQuery, seriesLength), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
```

por:

```go
			singleResults, _ := filterSearchResults(searcher.searchSingleEpisode(ep, anime.Media.Title, anime.Media.Synonyms, anime.Media.Relations, customQuery, seriesLength), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
```

`src/internal/daemon/episodes.go:236` (dentro de `resolveMovie`), troque:

```go
	movieResult := filterSearchResults(searcher.searchMovie(anime.Media.Title, true, customQuery), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
```

por:

```go
	movieResult, _ := filterSearchResults(searcher.searchMovie(anime.Media.Title, true, customQuery), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
```

`src/internal/daemon/episodes.go:336-347` (`partitionSearchResults`), troque a assinatura e o `return`, preservando o comentário de doc acima e **acrescentando** o parágrafo novo:

```go
// packStats descreve o que o filtro fez com as linhas de PACK, e so com elas: e ele que responde
// se "nao houve batch" foi porque a busca nao devolveu pack nenhum ou porque todos foram cortados
// pelo teto. O relatorio usa isso como campo de detalhe do limite por anime.
func partitionSearchResults(configs *files.Config, results []nyaa.TorrentResult) (packs, singles []nyaa.TorrentResult, packStats dropStats) {
	for _, tr := range results {
		switch {
		case tr.IsBatch:
			packs = append(packs, tr)
		case tr.Episode != nil:
			singles = append(singles, tr)
		}
	}
	filteredPacks, packStats := filterSearchResults(packs, configs.MaxBatchTorrentSizeGB, configs.MinSeeders)
	filteredSingles, _ := filterSearchResults(singles, configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
	return filteredPacks, filteredSingles, packStats
}
```

> Atenção: `packStats` é um resultado nomeado, então `filteredPacks, packStats := ...` não compila (`no new variables on left side` para `packStats`). Use `filteredPacks, stats := filterSearchResults(...)` e `packStats = stats`, ou remova os nomes dos resultados. A forma que compila:
>
> ```go
> func partitionSearchResults(configs *files.Config, results []nyaa.TorrentResult) ([]nyaa.TorrentResult, []nyaa.TorrentResult, dropStats) {
> 	var packs, singles []nyaa.TorrentResult
> 	for _, tr := range results {
> 		switch {
> 		case tr.IsBatch:
> 			packs = append(packs, tr)
> 		case tr.Episode != nil:
> 			singles = append(singles, tr)
> 		}
> 	}
> 	filteredPacks, packStats := filterSearchResults(packs, configs.MaxBatchTorrentSizeGB, configs.MinSeeders)
> 	filteredSingles, _ := filterSearchResults(singles, configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
> 	return filteredPacks, filteredSingles, packStats
> }
> ```

`src/internal/daemon/episodes.go:127`, troque:

```go
		packs, singles := partitionSearchResults(configs, searcher.searchAnime(anime.Media.Title, anime.Media.Synonyms, episodeNumbers(sel.toDownload), customQuery))
```

por (o `_` vira uso real na Task 4):

```go
		packs, singles, _ := partitionSearchResults(configs, searcher.searchAnime(anime.Media.Title, anime.Media.Synonyms, episodeNumbers(sel.toDownload), customQuery))
```

`src/internal/daemon/debug.go:112`, troque:

```go
		packs, singles := partitionSearchResults(configs, searcher.searchAnime(anime.Media.Title, anime.Media.Synonyms, episodeNumbers(episodesToDownload), customQuery))
```

por:

```go
		packs, singles, _ := partitionSearchResults(configs, searcher.searchAnime(anime.Media.Title, anime.Media.Synonyms, episodeNumbers(episodesToDownload), customQuery))
```

`src/internal/daemon/debug.go:138`, troque:

```go
			singleResults := filterSearchResults(searcher.searchSingleEpisode(ep, anime.Media.Title, anime.Media.Synonyms, anime.Media.Relations, customQuery, seriesLength), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
```

por:

```go
			singleResults, _ := filterSearchResults(searcher.searchSingleEpisode(ep, anime.Media.Title, anime.Media.Synonyms, anime.Media.Relations, customQuery, seriesLength), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./src/internal/daemon/ -run 'TestFilterBySize|TestFilterBySeeders|TestFilterSearchResults_DropStats' -v`
Expected: PASS (3 testes, 4 subtestes no último)

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: PASS (nada de comportamento mudou — só arity)

- [ ] **Step 7: Commit**

```bash
git add src/internal/daemon/search.go src/internal/daemon/episodes.go src/internal/daemon/debug.go src/internal/daemon/limits_test.go
git commit -m "refactor: filtros de busca devolvem o que descartaram (dropStats)"
```

---

### Task 2: Tipos do relatório e armazenamento em memória

`Issue`, `CheckReport`, a agregação por (anime, código), e os três métodos novos do `State`. Ainda ninguém produz nem consome — esta task só cria o vocabulário e o lugar de guardar.

**Files:**
- Create: `src/internal/daemon/report.go`
- Create: `src/internal/daemon/report_test.go`
- Modify: `src/internal/daemon/state.go:20-29` (campo novo), `:92-102` (`SetLastCheckError` limpa), e três métodos novos no fim do arquivo

**Interfaces:**
- Consumes: nada de tasks anteriores.
- Produces:
  - Constantes de código: `IssueAllAboveSizeLimit`, `IssueNoSeeders`, `IssueNoTorrentFound`, `IssueDiskFull`, `IssueTorrentRejected`, `IssueMaxEpisodesPerAnime` (todas `string`)
  - Constantes de `batch_skipped`: `BatchSkippedNoResult = "no_result"`, `BatchSkippedAboveSizeLimit = "above_size_limit"`, `BatchSkippedNoCoverage = "no_coverage"`
  - `type Issue struct` e `type CheckReport struct` (campos exatos abaixo)
  - `func aggregateIssues(raw []Issue) (problems, limits []Issue)`
  - `func (s *State) SetLastCheckReport(r CheckReport)`
  - `func (s *State) GetLastCheckReport() CheckReport`

- [ ] **Step 1: Write the failing test**

Crie `src/internal/daemon/report_test.go`:

```go
package daemon

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// TestAggregateIssues: um Issue por par (anime, codigo). Dois episodios do mesmo anime com o
// mesmo codigo viram UMA entrada com dois numeros; codigos diferentes viram duas.
func TestAggregateIssues(t *testing.T) {
	raw := []Issue{
		{AnimeID: 2, AnimeName: "Bleach", Episodes: []int{12}, Code: IssueAllAboveSizeLimit, Candidates: 8, LimitGB: 3},
		{AnimeID: 2, AnimeName: "Bleach", Episodes: []int{15}, Code: IssueNoTorrentFound},
		{AnimeID: 2, AnimeName: "Bleach", Episodes: []int{13}, Code: IssueAllAboveSizeLimit, Candidates: 4, LimitGB: 3},
		{AnimeID: 1, AnimeName: "Aria", Code: IssueMaxEpisodesPerAnime, Downloaded: 12, Pending: 35, BatchSkipped: BatchSkippedNoResult},
	}

	problems, limits := aggregateIssues(raw)

	if len(limits) != 1 {
		t.Fatalf("esperava 1 limite, obteve %d (%+v)", len(limits), limits)
	}
	if limits[0].Code != IssueMaxEpisodesPerAnime || limits[0].Downloaded != 12 || limits[0].Pending != 35 {
		t.Errorf("limite errado: %+v", limits[0])
	}
	// Limite nao carrega numeros de episodio: o daemon parou de CONSIDERAR episodios ao atingir
	// a conta, entao nao existe "os episodios afetados".
	if len(limits[0].Episodes) != 0 {
		t.Errorf("limite não deve trazer Episodes, obteve %+v", limits[0].Episodes)
	}

	if len(problems) != 2 {
		t.Fatalf("esperava 2 problemas, obteve %d (%+v)", len(problems), problems)
	}
	byCode := map[string]Issue{}
	for _, p := range problems {
		byCode[p.Code] = p
	}
	size, ok := byCode[IssueAllAboveSizeLimit]
	if !ok {
		t.Fatalf("faltou o problema de tamanho em %+v", problems)
	}
	if !reflect.DeepEqual(size.Episodes, []int{12, 13}) {
		t.Errorf("esperava episódios [12 13], obteve %+v", size.Episodes)
	}
	if size.Candidates != 8 || size.LimitGB != 3 {
		t.Errorf("detalhe do primeiro episódio deve vencer, obteve %+v", size)
	}
	if _, ok := byCode[IssueNoTorrentFound]; !ok {
		t.Errorf("faltou o problema de torrent não encontrado em %+v", problems)
	}
}

// TestAggregateIssues_SortedByAnimeName: ordem estavel e previsivel, sem ranking de severidade.
func TestAggregateIssues_SortedByAnimeName(t *testing.T) {
	raw := []Issue{
		{AnimeID: 3, AnimeName: "Zeta", Episodes: []int{1}, Code: IssueNoTorrentFound},
		{AnimeID: 1, AnimeName: "Alpha", Episodes: []int{1}, Code: IssueNoTorrentFound},
		{AnimeID: 2, AnimeName: "Mu", Episodes: []int{1}, Code: IssueNoTorrentFound},
	}
	problems, _ := aggregateIssues(raw)
	want := []string{"Alpha", "Mu", "Zeta"}
	for i, name := range want {
		if problems[i].AnimeName != name {
			t.Errorf("posição %d: esperava %q, obteve %q", i, name, problems[i].AnimeName)
		}
	}
}

func TestAggregateIssues_Empty(t *testing.T) {
	problems, limits := aggregateIssues(nil)
	if problems != nil || limits != nil {
		t.Errorf("passe limpo deve produzir nil/nil, obteve %+v / %+v", problems, limits)
	}
}

// TestSetLastCheckError_ClearsReport: um passe que abortou antes de olhar anime nenhum nao tem
// relatorio por anime — tem pass_error. Sem a limpeza, a tela mostraria os problemas do passe
// ANTERIOR lado a lado com um erro de passe novo.
func TestSetLastCheckError_ClearsReport(t *testing.T) {
	s := NewState()
	s.SetLastCheckReport(CheckReport{
		FinishedAt: time.Now(),
		Problems:   []Issue{{AnimeID: 1, AnimeName: "Aria", Code: IssueNoTorrentFound}},
	})
	if len(s.GetLastCheckReport().Problems) != 1 {
		t.Fatal("o relatório deveria estar guardado antes do erro")
	}

	s.SetLastCheckError(errors.New("anilist caiu"))

	got := s.GetLastCheckReport()
	if len(got.Problems) != 0 || len(got.Limits) != 0 || !got.FinishedAt.IsZero() {
		t.Errorf("SetLastCheckError deve limpar o relatório, obteve %+v", got)
	}
}

// TestSetLastCheckReport_AfterClearingError: SetLastCheckError(nil) tambem limpa, entao o
// SetLastCheckReport do fim do passe TEM de vir depois dele — e sobreviver.
func TestSetLastCheckReport_AfterClearingError(t *testing.T) {
	s := NewState()
	s.SetLastCheckError(nil)
	s.SetLastCheckReport(CheckReport{
		FinishedAt: time.Now(),
		Limits:     []Issue{{AnimeID: 1, AnimeName: "Aria", Code: IssueMaxEpisodesPerAnime, Downloaded: 12, Pending: 35}},
	})
	if len(s.GetLastCheckReport().Limits) != 1 {
		t.Error("o relatório publicado depois de SetLastCheckError(nil) deve sobreviver")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./src/internal/daemon/ -run 'TestAggregateIssues|TestSetLastCheck' -v`
Expected: FAIL na compilação — `undefined: Issue`, `undefined: aggregateIssues`, `s.SetLastCheckReport undefined`.

- [ ] **Step 3: Write minimal implementation — `report.go`**

Crie `src/internal/daemon/report.go`:

```go
package daemon

import (
	"sort"
	"time"
)

// Codigos de PROBLEMA: algo que devia ter baixado e nao baixou.
const (
	IssueAllAboveSizeLimit = "all_above_size_limit"
	IssueNoSeeders         = "no_seeders"
	IssueNoTorrentFound    = "no_torrent_found"
	IssueDiskFull          = "disk_full"
	IssueTorrentRejected   = "torrent_rejected"
)

// Codigo de LIMITE: a config funcionando como configurada. Peso visual diferente na UI porque o
// usuario frequentemente nao relaciona o limite com o sintoma ("por que so baixou 12 de 47?").
const IssueMaxEpisodesPerAnime = "max_episodes_per_anime"

// Valores de Issue.BatchSkipped — o porque de max_episodes_per_anime estar valendo NAQUELE anime.
// Batch desligado nao bloqueia nada por si so: ele so deixa o limite por anime valendo, e o
// sintoma que o usuario sente e sempre o limite. Por isso e campo de detalhe, e nao codigo.
const (
	BatchSkippedNoResult       = "no_result"        // a busca nao devolveu linha de pack nenhuma
	BatchSkippedAboveSizeLimit = "above_size_limit" // havia packs, o filtro cortou todos
	BatchSkippedNoCoverage     = "no_coverage"      // packs sobreviveram, nenhum cobre a janela
)

// Issue e uma linha do relatorio: um par (anime, codigo), com os episodios afetados quando o
// codigo e de problema.
//
// Campos de detalhe achatados com omitempty, e nao um detail map[string]any: um mapa livre nao
// gera Swagger utilizavel nem tipo TS decente, e o preco e uma struct de dez campos com tres
// preenchidos por vez.
type Issue struct {
	AnimeID   int    `json:"anime_id" example:"269"`
	AnimeName string `json:"anime_name" example:"Bleach"`
	// Episodes so e preenchido em PROBLEMA: um problema acontece num episodio (ele foi buscado,
	// os candidatos foram avaliados, aquele episodio nao baixou). O limite e o contrario — o
	// daemon parou de considerar episodios ao atingir a conta, entao nao existe "os episodios
	// afetados", existe uma quantidade que sobrou (Downloaded/Pending).
	Episodes []int  `json:"episodes,omitempty"`
	Code     string `json:"code" example:"all_above_size_limit"`

	Candidates   int     `json:"candidates,omitempty" example:"8"`
	LimitGB      float64 `json:"limit_gb,omitempty" example:"3"`
	MinSeeders   int     `json:"min_seeders,omitempty" example:"1"`
	Downloaded   int     `json:"downloaded,omitempty" example:"12"`
	Pending      int     `json:"pending,omitempty" example:"35"`
	BatchSkipped string  `json:"batch_skipped,omitempty" example:"no_result"`
}

// CheckReport e o relatorio do ULTIMO passe, e so dele. Nao e historico.
type CheckReport struct {
	FinishedAt time.Time `json:"finished_at" example:"2026-08-19T12:00:00Z"`
	PassError  string    `json:"pass_error" example:""`
	Problems   []Issue   `json:"problems"`
	Limits     []Issue   `json:"limits"`
}

// isLimitCode separa as duas categorias. Hoje ha um unico codigo de limite; a funcao existe para
// que acrescentar o segundo seja uma linha e nao uma cacada por comparacoes espalhadas.
func isLimitCode(code string) bool {
	return code == IssueMaxEpisodesPerAnime
}

// aggregateIssues junta os Issues crus de todos os animes em um por par (anime, codigo) e separa
// em problemas e limites, cada lista ordenada por nome do anime.
//
// ponytail: quando dois episodios do mesmo anime batem no mesmo codigo com detalhes diferentes
// (ep 12 com 8 candidatos, ep 13 com 4), os campos de detalhe do PRIMEIRO vencem e os do segundo
// somem. Somar seria mentira ("12 candidatos" nao existiu em busca nenhuma) e listar os dois
// exigiria um detalhe por episodio, que e o relatorio-por-episodio que a spec descartou. Se um
// dia isso incomodar, o caminho e Detail []struct{Episode int; ...} dentro do Issue.
func aggregateIssues(raw []Issue) (problems, limits []Issue) {
	type key struct {
		animeID int
		code    string
	}
	order := make([]key, 0, len(raw))
	merged := make(map[key]*Issue, len(raw))

	for _, in := range raw {
		k := key{in.AnimeID, in.Code}
		existing, ok := merged[k]
		if !ok {
			cp := in
			cp.Episodes = append([]int(nil), in.Episodes...)
			merged[k] = &cp
			order = append(order, k)
			continue
		}
		existing.Episodes = append(existing.Episodes, in.Episodes...)
	}

	for _, k := range order {
		issue := *merged[k]
		if isLimitCode(issue.Code) {
			limits = append(limits, issue)
			continue
		}
		problems = append(problems, issue)
	}

	sortIssues(problems)
	sortIssues(limits)
	return problems, limits
}

// sortIssues ordena por nome do anime, com o codigo como desempate — estavel e previsivel, sem
// inventar um ranking de severidade que ninguem pediu.
func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].AnimeName != issues[j].AnimeName {
			return issues[i].AnimeName < issues[j].AnimeName
		}
		return issues[i].Code < issues[j].Code
	})
}
```

- [ ] **Step 4: Write minimal implementation — `state.go`**

Em `src/internal/daemon/state.go`, acrescente o campo à struct `State` (linha 26, logo abaixo de `lastCheckError error`):

```go
	lastCheckError error
	// lastCheckReport e o relatorio do ULTIMO passe. So memoria: createStartFunc chama
	// AnimeVerification imediatamente ao iniciar (loop.go, antes do primeiro time.After),
	// entao apos um restart ele se reconstroi em segundos. Um arquivo custaria persistencia,
	// migracao e a possibilidade de mostrar um relatorio de dias atras como se fosse do ultimo
	// passe.
	lastCheckReport CheckReport
```

Em `SetLastCheckError` (linha 92), acrescente a limpeza logo depois da atribuição do erro:

```go
func (s *State) SetLastCheckError(err error) {
	s.mu.Lock()
	s.lastCheckError = err
	// Limpar aqui, e nao em cada saida antecipada: as sete saidas de AnimeVerification ja
	// chamam esta funcao, entao nenhuma delas precisa de linha nova. E a semantica e a certa —
	// um passe que abortou antes de olhar anime nenhum nao tem relatorio por anime, tem
	// pass_error. Consequencia deliberada: SetLastCheckError(nil) no fim do passe tambem limpa,
	// entao SetLastCheckReport TEM de vir depois dele (ver AnimeVerification e decisions.md).
	s.lastCheckReport = CheckReport{}
	notifier := s.notifier
	statusSnapshot, lastCheckSnapshot, hasErrorSnapshot := s.notifyChange()
	s.mu.Unlock()

	if notifier != nil {
		notifier.NotifyStateChange(statusSnapshot, lastCheckSnapshot, hasErrorSnapshot)
	}
}
```

Acrescente os dois acessores no fim do arquivo, depois de `GetAll`:

```go
func (s *State) SetLastCheckReport(r CheckReport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCheckReport = r
}

// GetLastCheckReport devolve VALOR, nao ponteiro: o handler precisa preencher pass_error na
// resposta sem escrever no objeto compartilhado. As slices continuam sendo as mesmas, e isso e
// seguro porque um CheckReport publicado nunca e mutado — o passe seguinte publica um novo.
func (s *State) GetLastCheckReport() CheckReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCheckReport
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./src/internal/daemon/ -run 'TestAggregateIssues|TestSetLastCheck' -v`
Expected: PASS (5 testes)

- [ ] **Step 6: Run the full suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add src/internal/daemon/report.go src/internal/daemon/report_test.go src/internal/daemon/state.go
git commit -m "feat: tipos do relatório de verificação e armazenamento em memória"
```

---

### Task 3: `checkEpisode` devolve o motivo do skip

A condição do limite (`*downloadedEpisodes >= maxEpisodes`) fica onde é decidida. O chamador tem acesso ao ponteiro e poderia reproduzir a comparação — e aí a regra existiria em dois lugares para discordar depois.

**Files:**
- Modify: `src/internal/daemon/episodes.go:443-467` (`checkEpisode`), `:23-28` (`episodeSelection`), `:35-64` (`selectEpisodes`)
- Test: `src/internal/daemon/episodes_test.go:242` (call site) + testes novos

**Interfaces:**
- Consumes: `IssueMaxEpisodesPerAnime` (Task 2).
- Produces:
  - `func checkEpisode(configs *files.Config, maxEpisodes int, ep anilist.AiringNode, anime anilist.MediaList, alreadySaved bool, downloadedEpisodes *int, isInTorrents bool, keepWatched, isBatch bool) (shouldDownload bool, shouldDelete bool, skipCode string)` — `skipCode` é `""` para todo skip que não entra no relatório
  - `episodeSelection` ganha `limitSkipped int` (quantos episódios o limite barrou) e `downloaded int` (o contador final)

- [ ] **Step 1: Write the failing test**

Acrescente ao fim de `src/internal/daemon/episodes_test.go`:

```go
// TestCheckEpisode_SkipCode: so o limite por anime entra no relatorio. Todo skip normal
// (lista excluida, ja assistido, ainda nao lancado) devolve "" — um anime em dia gera dezenas
// deles por passe, e se entrassem no relatorio os problemas reais se perderiam no ruido.
func TestCheckEpisode_SkipCode(t *testing.T) {
	title := "SkipCode Anime"
	base := anilist.MediaList{
		Id:       400,
		Progress: 2,
		Media: anilist.Media{
			Id:    400,
			Title: anilist.Title{English: &title},
		},
	}

	t.Run("limite por anime atingido", func(t *testing.T) {
		configs := &files.Config{MaxEpisodesPerAnime: 2}
		downloaded := 2
		ep := anilist.AiringNode{ID: 1, Episode: 5, TimeUntilAiring: -100}

		shouldDownload, shouldDelete, code := checkEpisode(configs, 2, ep, base, false, &downloaded, false, false, false)

		if shouldDownload || shouldDelete {
			t.Errorf("limite atingido não baixa nem apaga, obteve (%v, %v)", shouldDownload, shouldDelete)
		}
		if code != IssueMaxEpisodesPerAnime {
			t.Errorf("esperava %q, obteve %q", IssueMaxEpisodesPerAnime, code)
		}
	})

	t.Run("já assistido", func(t *testing.T) {
		configs := &files.Config{MaxEpisodesPerAnime: 12}
		downloaded := 0
		ep := anilist.AiringNode{ID: 2, Episode: 1, TimeUntilAiring: -100}

		_, _, code := checkEpisode(configs, 12, ep, base, false, &downloaded, false, false, false)

		if code != "" {
			t.Errorf("skip normal não entra no relatório, obteve %q", code)
		}
	})

	t.Run("ainda não lançado", func(t *testing.T) {
		configs := &files.Config{MaxEpisodesPerAnime: 12}
		downloaded := 0
		ep := anilist.AiringNode{ID: 3, Episode: 9, TimeUntilAiring: 3600}

		_, _, code := checkEpisode(configs, 12, ep, base, false, &downloaded, false, false, false)

		if code != "" {
			t.Errorf("skip normal não entra no relatório, obteve %q", code)
		}
	})

	t.Run("episódio baixável", func(t *testing.T) {
		configs := &files.Config{MaxEpisodesPerAnime: 12}
		downloaded := 0
		ep := anilist.AiringNode{ID: 4, Episode: 5, TimeUntilAiring: -100}

		shouldDownload, _, code := checkEpisode(configs, 12, ep, base, false, &downloaded, false, false, false)

		if !shouldDownload {
			t.Error("esperava shouldDownload=true")
		}
		if code != "" {
			t.Errorf("episódio baixado não tem motivo de skip, obteve %q", code)
		}
	})
}

// TestSelectEpisodes_CountsLimitSkips: e do resultado FINAL de selectEpisodes que o relatorio
// tira downloaded/pending. Com 10 episodios pendentes e teto 3, sobram 7.
func TestSelectEpisodes_CountsLimitSkips(t *testing.T) {
	title := "Limit Count Anime"
	nodes := make([]anilist.AiringNode, 10)
	for i := range nodes {
		nodes[i] = anilist.AiringNode{ID: 500 + i, Episode: i + 1, TimeUntilAiring: -100}
	}
	anime := anilist.MediaList{
		Id: 500,
		Media: anilist.Media{
			Id:             500,
			Title:          anilist.Title{English: &title},
			AiringSchedule: anilist.AiringSchedule{Nodes: nodes},
		},
	}
	configs := &files.Config{MaxEpisodesPerAnime: 3}

	sel := selectEpisodes(configs, 3, anime, nodes, map[files.EpisodeKey]bool{}, map[files.EpisodeKey]files.EpisodeStruct{}, map[string]bool{}, nil, nil)

	if len(sel.toDownload) != 3 {
		t.Fatalf("esperava 3 para baixar, obteve %d", len(sel.toDownload))
	}
	if sel.downloaded != 3 {
		t.Errorf("esperava downloaded=3, obteve %d", sel.downloaded)
	}
	if sel.limitSkipped != 7 {
		t.Errorf("esperava limitSkipped=7, obteve %d", sel.limitSkipped)
	}
}
```

Atualize também o call site existente em `src/internal/daemon/episodes_test.go:242`:

```go
	shouldDownload, shouldDelete, _ := checkEpisode(configs, configs.MaxEpisodesPerAnime, ep, anime, true, &downloaded, false, false, false)
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./src/internal/daemon/ -run 'TestCheckEpisode_SkipCode|TestSelectEpisodes_CountsLimitSkips' -v`
Expected: FAIL na compilação — `assignment mismatch: 3 variables but checkEpisode returns 2 values` e `sel.downloaded undefined`.

- [ ] **Step 3: Write minimal implementation**

Em `src/internal/daemon/episodes.go:23-28`, acrescente os dois campos a `episodeSelection`:

```go
// episodeSelection e o resultado do laco de selecao de um anime.
type episodeSelection struct {
	toDownload   []anilist.AiringNode
	keysToDelete []files.EpisodeKey
	checked      []files.EpisodeKey
	// downloaded e limitSkipped sao o par que o relatorio publica como "baixou N, sobraram M".
	// Vem do resultado FINAL de selectEpisodes de proposito: quando um pack foi escolhido a
	// selecao roda de novo com o limite levantado, e ai limitSkipped e zero — que e o certo,
	// porque naquele anime o limite nao barrou nada.
	downloaded   int
	limitSkipped int
}
```

Em `selectEpisodes` (`episodes.go:45-63`), colete o motivo:

```go
	var sel episodeSelection
	downloadedEpisodesOfAnime := 0

	for _, ep := range episodes {
		key := files.EpisodeKey{AnimeID: anime.Media.Id, Episode: ep.Episode}
		sel.checked = append(sel.checked, key)

		savedEp := savedEpisodesFullMap[key]
		isInTorrents := episodeInTorrents(savedEp.EpisodeHash, torrentsHashSet)

		shouldDownload, shouldDelete, skipCode := checkEpisode(configs, maxEpisodes, ep, anime, savedEpisodesMap[key], &downloadedEpisodesOfAnime, isInTorrents, keepSet[key], savedEp.IsBatch)

		if skipCode == IssueMaxEpisodesPerAnime {
			sel.limitSkipped++
		}

		if shouldDownload && !blockedMap[key] {
			sel.toDownload = append(sel.toDownload, ep)
		} else if shouldDelete && !savedEp.ManuallyManaged {
			sel.keysToDelete = append(sel.keysToDelete, key)
		}
	}

	sel.downloaded = downloadedEpisodesOfAnime

	return sel
```

Em `checkEpisode` (`episodes.go:443`), acrescente o terceiro retorno. Repare que **só o ramo do limite** devolve código: `handleAlreadySavedEpisode` também compara `*downloadedEpisodes >= maxEpisodes`, mas aquilo é o caminho de DELEÇÃO, não "o limite barrou um download".

```go
// checkEpisode decide se um episodio deve ser baixado ou apagado. maxEpisodes e o limite
// EFETIVO por anime, calculado pelo chamador: em batch ele vem ilimitado (ver processAnimeEpisodes).
//
// skipCode e o motivo do skip QUANDO ele entra no relatorio da ultima verificacao, e "" no resto.
// A condicao fica aqui, e nao no chamador que tem o mesmo ponteiro, para a regra nao existir em
// dois lugares e discordar depois.
func checkEpisode(configs *files.Config, maxEpisodes int, ep anilist.AiringNode, anime anilist.MediaList, alreadySaved bool, downloadedEpisodes *int, isInTorrents bool, keepWatched, isBatch bool) (shouldDownload bool, shouldDelete bool, skipCode string) {
	epName := fmt.Sprintf("%s - Episode %d", getAnimeTitleSafe(anime), ep.Episode)

	if shouldSkipEpisode(configs, ep, anime, epName) {
		return false, alreadySaved && !keepWatched, ""
	}

	if alreadySaved {
		download, del := handleAlreadySavedEpisode(maxEpisodes, downloadedEpisodes, isInTorrents, isBatch, epName)
		return download, del, ""
	}

	if *downloadedEpisodes >= maxEpisodes {
		logger.Logger.Debug().
			Str("episode", epName).
			Int("downloaded_episodes", *downloadedEpisodes).
			Int("max_episodes", maxEpisodes).
			Msg("Skipping episode: max episodes per anime reached")
		return false, false, IssueMaxEpisodesPerAnime
	}

	*downloadedEpisodes++
	return true, false, ""
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./src/internal/daemon/ -run 'TestCheckEpisode|TestSelectEpisodes' -v`
Expected: PASS

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add src/internal/daemon/episodes.go src/internal/daemon/episodes_test.go
git commit -m "feat: checkEpisode devolve o motivo do skip que entra no relatório"
```

---

### Task 4: `processAnimeEpisodes` produz os `Issue`s

Os cinco problemas e o limite passam a ser emitidos onde já são decididos. **Nenhuma decisão muda** — cada `append` novo fica ao lado de um `logger`/`notifications` que já existia.

**Files:**
- Modify: `src/internal/daemon/verification.go:14-19` (`animeProcessResult`)
- Modify: `src/internal/daemon/episodes.go:84-220` (`processAnimeEpisodes`)
- Modify: `src/internal/daemon/report.go` (`searchIssue`)
- Test: `src/internal/daemon/report_test.go` (cascata) e `src/internal/daemon/limits_test.go` (fim-a-fim por `processAnimeEpisodes`)

**Interfaces:**
- Consumes: `dropStats` e `partitionSearchResults` de 3 retornos (Task 1); `Issue`, os códigos e os `BatchSkipped*` (Task 2); `episodeSelection.downloaded`/`.limitSkipped` (Task 3).
- Produces:
  - `animeProcessResult` ganha `issues []Issue`
  - `func searchIssue(animeID int, animeName string, episode int, stats dropStats, configs *files.Config) Issue`

- [ ] **Step 1: Write the failing test**

Acrescente a `src/internal/daemon/report_test.go`:

```go
// TestSearchIssue_PrecedenciaEmCascata: os tres primeiros problemas se sobrepoem — quando um
// filtro esvazia a lista, "nenhum torrent encontrado" tambem e verdade. O especifico TEM de
// ganhar do generico: "todos os candidatos tinham 8 GB e seu teto e 3 GB" e acionavel; "nenhum
// torrent encontrado" nao e, e ainda por cima e mentiroso — havia oito.
func TestSearchIssue_PrecedenciaEmCascata(t *testing.T) {
	configs := &files.Config{MaxEpisodeTorrentSizeGB: 3, MinSeeders: 5}

	t.Run("cortado por tamanho vence o genérico", func(t *testing.T) {
		got := searchIssue(1, "Bleach", 12, dropStats{Input: 8, BySize: 8}, configs)
		if got.Code != IssueAllAboveSizeLimit {
			t.Fatalf("esperava %q, obteve %q", IssueAllAboveSizeLimit, got.Code)
		}
		if got.Candidates != 8 || got.LimitGB != 3 {
			t.Errorf("detalhes errados: %+v", got)
		}
		if len(got.Episodes) != 1 || got.Episodes[0] != 12 {
			t.Errorf("esperava episódio [12], obteve %+v", got.Episodes)
		}
	})

	t.Run("tamanho vence seeders quando os dois cortaram", func(t *testing.T) {
		got := searchIssue(1, "Bleach", 12, dropStats{Input: 8, BySize: 5, BySeeders: 3}, configs)
		if got.Code != IssueAllAboveSizeLimit {
			t.Errorf("esperava %q, obteve %q", IssueAllAboveSizeLimit, got.Code)
		}
	})

	t.Run("cortado só por seeders", func(t *testing.T) {
		got := searchIssue(1, "Bleach", 12, dropStats{Input: 4, BySeeders: 4}, configs)
		if got.Code != IssueNoSeeders {
			t.Fatalf("esperava %q, obteve %q", IssueNoSeeders, got.Code)
		}
		if got.Candidates != 4 || got.MinSeeders != 5 {
			t.Errorf("detalhes errados: %+v", got)
		}
	})

	t.Run("busca realmente vazia", func(t *testing.T) {
		got := searchIssue(1, "Bleach", 12, dropStats{}, configs)
		if got.Code != IssueNoTorrentFound {
			t.Errorf("esperava %q, obteve %q", IssueNoTorrentFound, got.Code)
		}
		if got.Candidates != 0 || got.LimitGB != 0 || got.MinSeeders != 0 {
			t.Errorf("no_torrent_found não tem detalhe, obteve %+v", got)
		}
	})
}
```

Acrescente ao fim de `src/internal/daemon/limits_test.go` (usa os helpers `animeWithEpisodes`/`searcherFor` que já existem nesse arquivo — leia-os antes, em `limits_test.go:23` e `:45`, e confirme a assinatura de cada um):

```go
// TestProcessAnimeEpisodes_Issues: os motivos saem por processAnimeEpisodes, no mesmo canal que
// ja devolve newEpisodes/checkedEpisodes/keysToDelete. Nenhum coletor global, nenhum mutex novo.
func TestProcessAnimeEpisodes_Issues(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)

	t.Run("todos os candidatos acima do teto", func(t *testing.T) {
		anime := animeWithEpisodes(1, anilist.MediaStatusReleasing, true, anilist.MediaFormatTV)
		configs := &files.Config{
			CompletedAnimePath:      t.TempDir(),
			MaxEpisodesPerAnime:     12,
			EpisodeRetryLimit:       3,
			MaxEpisodeTorrentSizeGB: 3,
			MinSeeders:              1,
		}
		big := []nyaa.TorrentResult{
			{Name: "huge-a", Size: 8 * gib, Seeders: "50", MagnetLink: "magnet:a"},
			{Name: "huge-b", Size: 9 * gib, Seeders: "40", MagnetLink: "magnet:b"},
		}
		searcher := searcherFor(nil, nil, big, nil)

		result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, nil, "", searcher)

		if len(result.issues) != 1 {
			t.Fatalf("esperava 1 issue, obteve %d (%+v)", len(result.issues), result.issues)
		}
		got := result.issues[0]
		if got.Code != IssueAllAboveSizeLimit {
			t.Errorf("esperava %q, obteve %q", IssueAllAboveSizeLimit, got.Code)
		}
		if got.Candidates != 2 || got.LimitGB != 3 {
			t.Errorf("detalhes errados: %+v", got)
		}
		if got.AnimeID != anime.Media.Id || got.AnimeName == "" {
			t.Errorf("identificação do anime faltando: %+v", got)
		}
	})

	t.Run("busca vazia vira no_torrent_found", func(t *testing.T) {
		anime := animeWithEpisodes(1, anilist.MediaStatusReleasing, true, anilist.MediaFormatTV)
		configs := &files.Config{
			CompletedAnimePath:  t.TempDir(),
			MaxEpisodesPerAnime: 12,
			EpisodeRetryLimit:   3,
			MinSeeders:          1,
		}
		searcher := searcherFor(nil, nil, nil, nil)

		result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, nil, "", searcher)

		if len(result.issues) != 1 || result.issues[0].Code != IssueNoTorrentFound {
			t.Fatalf("esperava um no_torrent_found, obteve %+v", result.issues)
		}
	})

	t.Run("limite por anime com batch sem resultado", func(t *testing.T) {
		anime := animeWithEpisodes(10, anilist.MediaStatusReleasing, true, anilist.MediaFormatTV)
		configs := &files.Config{
			CompletedAnimePath:  t.TempDir(),
			MaxEpisodesPerAnime: 3,
			EpisodeRetryLimit:   3,
			MinSeeders:          1,
		}
		singles := make([]nyaa.TorrentResult, 3)
		for i := range singles {
			ep := i + 1
			singles[i] = nyaa.TorrentResult{Name: "ep", Size: gib, Seeders: "50", MagnetLink: "magnet:x", Episode: &ep}
		}
		searcher := searcherFor(nil, singles, nil, nil)

		result := processAnimeEpisodes(configs, torrents.NewFakeBackend(), anime, nil, nil, nil, "", searcher)

		var limit *Issue
		for i := range result.issues {
			if result.issues[i].Code == IssueMaxEpisodesPerAnime {
				limit = &result.issues[i]
			}
		}
		if limit == nil {
			t.Fatalf("esperava um max_episodes_per_anime, obteve %+v", result.issues)
		}
		if limit.Downloaded != 3 || limit.Pending != 7 {
			t.Errorf("esperava downloaded=3 pending=7, obteve %+v", limit)
		}
		if limit.BatchSkipped != BatchSkippedNoResult {
			t.Errorf("esperava batch_skipped=%q, obteve %q", BatchSkippedNoResult, limit.BatchSkipped)
		}
		if len(limit.Episodes) != 0 {
			t.Errorf("limite não carrega números de episódio, obteve %+v", limit.Episodes)
		}
	})
}
```

> Antes de rodar: confirme como os outros testes de `limits_test.go` constroem o backend falso (`grep -n "FakeBackend\|fakeBackend" src/internal/daemon/*_test.go src/internal/torrents/fakebackend.go`) e use exatamente o mesmo construtor. Se a assinatura de `processAnimeEpisodes` no seu checkout diferir da usada acima, copie a de `limits_test.go`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./src/internal/daemon/ -run 'TestSearchIssue|TestProcessAnimeEpisodes_Issues' -v`
Expected: FAIL — `undefined: searchIssue` e `result.issues undefined`.

- [ ] **Step 3: Write minimal implementation — `searchIssue` em `report.go`**

Acrescente ao fim de `src/internal/daemon/report.go` (e o import de `"AutoAnimeDownloader/src/internal/files"` no topo, antes dos stdlib):

```go
// searchIssue traduz o que a busca por episodio descartou no codigo de problema mais especifico
// que couber.
//
// A ordem e CASCATA, nao conjunto: quando um filtro esvazia a lista, len(magnets) == 0 tambem e
// verdade, e "nenhum torrent encontrado" e a resposta menos acionavel das tres — e mentirosa,
// porque havia oito. A primeira condicao que casa vence, e a ordem e a regra de negocio; mesma
// disciplina da cascata de deriveAnimeChip no frontend.
func searchIssue(animeID int, animeName string, episode int, stats dropStats, configs *files.Config) Issue {
	issue := Issue{AnimeID: animeID, AnimeName: animeName, Episodes: []int{episode}}
	switch {
	case stats.Input > 0 && stats.BySize > 0:
		issue.Code = IssueAllAboveSizeLimit
		issue.Candidates = stats.Input
		issue.LimitGB = configs.MaxEpisodeTorrentSizeGB
	case stats.Input > 0 && stats.BySeeders > 0:
		issue.Code = IssueNoSeeders
		issue.Candidates = stats.Input
		issue.MinSeeders = configs.MinSeeders
	default:
		issue.Code = IssueNoTorrentFound
	}
	return issue
}
```

- [ ] **Step 4: Write minimal implementation — `animeProcessResult`**

Em `src/internal/daemon/verification.go:14-19`:

```go
// animeProcessResult holds the per-anime outputs from processAnimeEpisodes.
type animeProcessResult struct {
	newEpisodes     []files.EpisodeStruct
	checkedEpisodes []files.EpisodeKey
	keysToDelete    []files.EpisodeKey
	// issues sao os motivos pelos quais este anime deixou de baixar algo. Vem pelo canal que ja
	// existe de proposito: um *passReport compartilhado entre as goroutines de
	// maxConcurrentAnimes precisaria de mutex, e o fan-in ja resolve isso de graca.
	issues []Issue
}
```

- [ ] **Step 5: Write minimal implementation — `processAnimeEpisodes`**

Em `src/internal/daemon/episodes.go`, no bloco de batch (`:122-146`), capture o `packStats` e decida o `batchSkipped`. Declare `batchSkipped` **antes** do `if` externo (o `sel` é reatribuído lá dentro):

```go
	var magnetsForEpisodes map[int]resolvedMagnets
	// batchSkipped e o porque de max_episodes_per_anime estar valendo neste anime. Fica fora do
	// if porque o relatorio o publica la embaixo, depois de sel ter sido possivelmente refeito.
	batchSkipped := ""

	if isAnimeMovie(anime) {
		sel.toDownload, magnetsForEpisodes = resolveMovie(configs, anime, animeTitle, sel.toDownload, customQuery, searcher)
	}

	if magnetsForEpisodes == nil && len(sel.toDownload) > 0 {
		packs, singles, packStats := partitionSearchResults(configs, searcher.searchAnime(anime.Media.Title, anime.Media.Synonyms, episodeNumbers(sel.toDownload), customQuery))

		// Elegibilidade a pack: nao e filme, tem mais de um episodio pendente e a busca FILTRADA
		// devolveu pack que cobre a janela. Nada disso e metadado do AniList — e o torrent que
		// esta la que decide (ver decisions.md).
		if !isAnimeMovie(anime) && len(sel.toDownload) > 1 {
			firstPending := sel.toDownload[0].Episode
			batches := pickBatches(packs, firstPending, windowEnd(configs, firstPending))
			switch {
			case len(batches) > 0:
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
			case packStats.Input == 0:
				batchSkipped = BatchSkippedNoResult
			case len(packs) == 0:
				batchSkipped = BatchSkippedAboveSizeLimit
			default:
				batchSkipped = BatchSkippedNoCoverage
			}
		}

		if magnetsForEpisodes == nil {
			magnetsForEpisodes = magnetsByEpisode(singles, sel.toDownload)
		}
	}

	result.checkedEpisodes = sel.checked
	result.keysToDelete = sel.keysToDelete
	episodesToDownload := sel.toDownload

	// O limite so vira linha do relatorio quando ele de fato barrou algo. Quando um pack foi
	// escolhido, a segunda selecao roda com o limite levantado e limitSkipped e zero — que e o
	// certo: naquele anime o limite nao impediu nada.
	if sel.limitSkipped > 0 {
		result.issues = append(result.issues, Issue{
			AnimeID:      anime.Media.Id,
			AnimeName:    animeTitle,
			Code:         IssueMaxEpisodesPerAnime,
			Downloaded:   sel.downloaded,
			Pending:      sel.limitSkipped,
			BatchSkipped: batchSkipped,
		})
	}
```

Ainda em `processAnimeEpisodes`, no laço de download (`:158-186`), hoiste o `dropStats` do fallback para fora do primeiro `if` e emita o problema no segundo:

```go
		// ponytail: o dropStats do fallback por episodio e a UNICA fonte da cascata de problemas —
		// nem partitionSearchResults nem resolveMovie alimentam o relatorio. E ele quem da a
		// ultima palavra sobre este episodio: se achou magnet nao houve problema, se nao achou e o
		// corte dele que descreve por que. Se um dia o pack precisar do proprio codigo, o caminho
		// e passar o packStats para ca em vez de sobrescrever este.
		var searchStats dropStats
		if len(magnets) == 0 {
			var singleResults []nyaa.TorrentResult
			singleResults, searchStats = filterSearchResults(searcher.searchSingleEpisode(ep, anime.Media.Title, anime.Media.Synonyms, anime.Media.Relations, customQuery, seriesLength), configs.MaxEpisodeTorrentSizeGB, configs.MinSeeders)
			for _, tr := range singleResults {
				magnets = append(magnets, tr.MagnetLink)
			}
		}

		// Sem magnet nao ha o que tentar: avisar "iniciando download" aqui mandava um push
		// falso a cada passada do loop enquanto o episodio nao aparecesse no Nyaa, e ainda
		// fazia attemptDownloadWithRetries logar "falhou apos todas as tentativas" com zero
		// tentativas.
		if len(magnets) == 0 {
			logger.Logger.Warn().
				Str("episode", epName).
				Msg("No torrent found for episode")
			result.issues = append(result.issues, searchIssue(anime.Media.Id, animeTitle, ep.Episode, searchStats, configs))
			notifications.Notify(configs, notifications.DownloadFailed, animeTitle, ep.Episode, notifications.ReasonNotFound)
			continue
		}
```

E no ramo de falha do download (`:208-217`):

```go
		} else {
			reason := notifications.ReasonDownloadRejected
			issue := Issue{
				AnimeID:    anime.Media.Id,
				AnimeName:  animeTitle,
				Episodes:   []int{ep.Episode},
				Code:       IssueTorrentRejected,
				Candidates: len(magnets),
			}
			if errors.Is(checkDiskSpace(configs), ErrInsufficientDiskSpace) {
				reason = notifications.ReasonNoDiskSpace
				issue.Code = IssueDiskFull
				// Disco cheio nao e sobre os magnets: nenhum foi tentado (attemptDownloadWithRetries
				// sai antes do primeiro Add), entao "N candidatos" seria numero sem significado.
				issue.Candidates = 0
			}
			result.issues = append(result.issues, issue)
			// O batch de notificacoes (BatchWindowSeconds) junta os N episodios do passe numa
			// mensagem so, entao disco cheio nao vira enxurrada.
			notifications.Notify(configs, notifications.DownloadFailed, animeTitle, ep.Episode, reason)
		}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./src/internal/daemon/ -run 'TestSearchIssue|TestProcessAnimeEpisodes_Issues' -v`
Expected: PASS

- [ ] **Step 7: Run the full suite**

Run: `go test ./...`
Expected: PASS. Se algum teste de `limits_test.go`/`batches_test.go` quebrar, **é regressão de comportamento** — o `switch` do batch tem de ser exatamente equivalente ao `if len(batches) > 0` anterior. Corrija o `switch`, nunca o teste.

- [ ] **Step 8: Commit**

```bash
git add src/internal/daemon/episodes.go src/internal/daemon/report.go src/internal/daemon/verification.go src/internal/daemon/limits_test.go src/internal/daemon/report_test.go
git commit -m "feat: processAnimeEpisodes emite os motivos de não-download"
```

---

### Task 5: `AnimeVerification` agrega e publica o relatório

**Files:**
- Modify: `src/internal/daemon/verification.go:279-306`
- Test: `src/internal/daemon/verification_test.go` (teste novo)

**Interfaces:**
- Consumes: `animeProcessResult.issues` (Task 4), `aggregateIssues` e `SetLastCheckReport` (Task 2).
- Produces: nada de novo — o relatório passa a existir em `state.GetLastCheckReport()` depois de um passe.

- [ ] **Step 1: Write the failing test**

Leia primeiro `src/internal/daemon/verification_test.go` e `src/internal/daemon/orchestration_test.go` para achar o helper que já monta um `AnimeVerification` completo (mock de AniList, `FileManagerInterface` falso, backend falso). Reaproveite-o. Acrescente:

```go
// TestAnimeVerification_PublishesReport: o relatorio e publicado DEPOIS do SetLastCheckError(nil)
// do fim do passe. Se a ordem inverter, SetLastCheckError limpa o que acabou de ser publicado e
// o relatorio nunca aparece — que e o unico jeito de esta feature falhar silenciosamente.
func TestAnimeVerification_PublishesReport(t *testing.T) {
	// ... monte o passe com um anime cujo Nyaa não devolve nada (o mock de nyaa já existe em
	// src/tests/mocks/nyaa/mock_server.go) e rode AnimeVerification até o fim.

	report := state.GetLastCheckReport()
	if report.FinishedAt.IsZero() {
		t.Fatal("o passe completo deve publicar um relatório com finished_at preenchido")
	}
	if len(report.Problems) == 0 {
		t.Errorf("esperava ao menos um problema, obteve %+v", report)
	}
}

// TestAnimeVerification_CancelledLeavesNoReport: passe interrompido nao deixa relatorio — ele
// estava incompleto, e um relatorio parcial diria "so este anime teve problema" quando os outros
// nem chegaram a ser olhados.
func TestAnimeVerification_CancelledLeavesNoReport(t *testing.T) {
	// ... monte o passe, cancele o ctx antes do fan-in e rode AnimeVerification.

	report := state.GetLastCheckReport()
	if !report.FinishedAt.IsZero() || len(report.Problems) != 0 || len(report.Limits) != 0 {
		t.Errorf("passe cancelado não deve deixar relatório, obteve %+v", report)
	}
}
```

> Estes dois testes são os únicos do plano que dependem de infraestrutura de teste que você precisa localizar em vez de escrever do zero. Se `verification_test.go` não tiver um helper reaproveitável para rodar `AnimeVerification` inteiro, use o de `orchestration_test.go`; se nenhum servir, escreva o segundo teste (cancelamento) e cubra o primeiro por `TestSetLastCheckReport_AfterClearingError` (Task 2), que já trava a ordem no `State`. **Não pule os dois.**

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./src/internal/daemon/ -run 'TestAnimeVerification_.*Report' -v`
Expected: FAIL — `finished_at` zerado, porque ninguém publica.

- [ ] **Step 3: Write minimal implementation**

Em `src/internal/daemon/verification.go`, no fan-in (`:279-285`), colete os issues:

```go
	var newEpisodes []files.EpisodeStruct
	var checkedEpisodes []files.EpisodeKey
	var issues []Issue
	for r := range resultCh {
		newEpisodes = append(newEpisodes, r.newEpisodes...)
		checkedEpisodes = append(checkedEpisodes, r.checkedEpisodes...)
		keysToDelete = append(keysToDelete, r.keysToDelete...)
		issues = append(issues, r.issues...)
	}
```

E depois do `SetLastCheckError(nil)` (`:305-306`):

```go
	state.SetLastCheck(time.Now())
	state.SetLastCheckError(nil)

	// DEPOIS do SetLastCheckError, nunca antes: ele limpa o relatorio (ver state.go). O
	// cancelamento acima tambem chama SetLastCheckError(nil) e retorna, entao passe interrompido
	// nao deixa relatorio — que e o certo, ele estava incompleto.
	problems, limits := aggregateIssues(issues)
	state.SetLastCheckReport(CheckReport{
		FinishedAt: time.Now(),
		Problems:   problems,
		Limits:     limits,
	})
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./src/internal/daemon/ -v`
Expected: PASS

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add src/internal/daemon/verification.go src/internal/daemon/verification_test.go
git commit -m "feat: o passe automático publica o relatório da última verificação"
```

---

### Task 6: `GET /api/v1/last-check`

**Files:**
- Create: `src/internal/api/endpoint_last_check.go`
- Create: `src/internal/api/endpoint_last_check_test.go`
- Modify: `src/internal/api/server.go:92` (registro da rota, logo abaixo de `/api/v1/status`)

**Interfaces:**
- Consumes: `daemon.CheckReport`, `State.GetLastCheckReport()`, `State.GetLastCheckError()` (Task 2/5).
- Produces: `GET /api/v1/last-check` → `SuccessResponse{data=daemon.CheckReport}`. 405 em qualquer outro método.

- [ ] **Step 1: Write the failing test**

Crie `src/internal/api/endpoint_last_check_test.go`:

```go
package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleLastCheck(t *testing.T) {
	t.Run("GET devolve o relatório", func(t *testing.T) {
		state := daemon.NewState()
		state.SetLastCheckReport(daemon.CheckReport{
			FinishedAt: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC),
			Problems: []daemon.Issue{{
				AnimeID:    269,
				AnimeName:  "Bleach",
				Episodes:   []int{12, 15},
				Code:       daemon.IssueAllAboveSizeLimit,
				Candidates: 8,
				LimitGB:    3,
			}},
			Limits: []daemon.Issue{{
				AnimeID:      21,
				AnimeName:    "One Piece",
				Code:         daemon.IssueMaxEpisodesPerAnime,
				Downloaded:   12,
				Pending:      35,
				BatchSkipped: daemon.BatchSkippedNoResult,
			}},
		})

		w := httptest.NewRecorder()
		handleLastCheck(&Server{State: state})(w, httptest.NewRequest(http.MethodGet, "/api/v1/last-check", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("esperava 200, obteve %d", w.Code)
		}
		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("resposta inválida: %v", err)
		}
		data, ok := response.Data.(map[string]interface{})
		if !ok {
			t.Fatal("esperava data como objeto")
		}
		if passErr, _ := data["pass_error"].(string); passErr != "" {
			t.Errorf("esperava pass_error vazio, obteve %q", passErr)
		}
		problems, ok := data["problems"].([]interface{})
		if !ok || len(problems) != 1 {
			t.Fatalf("esperava 1 problema, obteve %v", data["problems"])
		}
		first := problems[0].(map[string]interface{})
		if first["code"] != daemon.IssueAllAboveSizeLimit {
			t.Errorf("código errado: %v", first["code"])
		}
		if first["anime_name"] != "Bleach" {
			t.Errorf("anime errado: %v", first["anime_name"])
		}
		limits, ok := data["limits"].([]interface{})
		if !ok || len(limits) != 1 {
			t.Fatalf("esperava 1 limite, obteve %v", data["limits"])
		}
		// omitempty: o limite não carrega episodes.
		if _, present := limits[0].(map[string]interface{})["episodes"]; present {
			t.Error("limite não deve serializar episodes")
		}
	})

	t.Run("GET com relatório vazio", func(t *testing.T) {
		state := daemon.NewState()

		w := httptest.NewRecorder()
		handleLastCheck(&Server{State: state})(w, httptest.NewRequest(http.MethodGet, "/api/v1/last-check", nil))

		if w.Code != http.StatusOK {
			t.Fatalf("esperava 200, obteve %d", w.Code)
		}
		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("resposta inválida: %v", err)
		}
		if !response.Success {
			t.Error("esperava success=true")
		}
	})

	t.Run("pass_error vem do último erro de passe", func(t *testing.T) {
		state := daemon.NewState()
		state.SetLastCheckError(errors.New("anilist unreachable"))

		w := httptest.NewRecorder()
		handleLastCheck(&Server{State: state})(w, httptest.NewRequest(http.MethodGet, "/api/v1/last-check", nil))

		var response SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("resposta inválida: %v", err)
		}
		data := response.Data.(map[string]interface{})
		if data["pass_error"] != "anilist unreachable" {
			t.Errorf("esperava a mensagem do erro, obteve %v", data["pass_error"])
		}
	})

	t.Run("POST devolve 405", func(t *testing.T) {
		w := httptest.NewRecorder()
		handleLastCheck(&Server{State: daemon.NewState()})(w, httptest.NewRequest(http.MethodPost, "/api/v1/last-check", nil))

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("esperava 405, obteve %d", w.Code)
		}
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./src/internal/api/ -run TestHandleLastCheck -v`
Expected: FAIL — `undefined: handleLastCheck`.

- [ ] **Step 3: Write minimal implementation**

Crie `src/internal/api/endpoint_last_check.go`:

```go
package api

import (
	"AutoAnimeDownloader/src/internal/daemon"

	"net/http"
)

// @Summary      Get the last verification report
// @Description  Returns why the last automatic pass did not download episodes, aggregated per anime. `problems` are things that should have downloaded and did not; `limits` are the configuration working as configured. `pass_error` is non-empty when the pass itself aborted, and then both lists are empty. A clean pass answers 200 with two empty lists; a `finished_at` of zero means the daemon has not completed a pass yet. Manual downloads are out of scope — those report their failure in their own HTTP response.
// @Tags         status
// @Accept       json
// @Produce      json
// @Success      200  {object}  SuccessResponse{data=daemon.CheckReport}
// @Failure      405  {object}  SuccessResponse
// @Router       /last-check [get]
func handleLastCheck(server *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET method is allowed")
			return
		}

		// Copia por valor (ver GetLastCheckReport): preencher pass_error aqui nao pode escrever
		// no relatorio compartilhado.
		report := server.State.GetLastCheckReport()
		if err := server.State.GetLastCheckError(); err != nil {
			report.PassError = err.Error()
		}

		JSONSuccess(w, http.StatusOK, report)
	}
}

// _ mantem o import de daemon usado mesmo se a anotacao do swag for a unica referencia textual.
var _ = daemon.CheckReport{}
```

> Se o compilador aceitar `daemon` só pelo tipo de retorno de `GetLastCheckReport()` (ele aceita — `report` é um `daemon.CheckReport`), **remova a linha `var _`**: ela é ruído. Ela está aqui só para o caso de você reescrever o handler sem mencionar o pacote.

Registre a rota em `src/internal/api/server.go`, logo abaixo da linha 92:

```go
	apiMux.HandleFunc("/api/v1/status", handleStatus(s))
	apiMux.HandleFunc("/api/v1/last-check", handleLastCheck(s))
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./src/internal/api/ -run TestHandleLastCheck -v`
Expected: PASS (4 subtestes)

- [ ] **Step 5: Regenerate Swagger and run the full suite**

Run:
```bash
swag init -g src/cmd/daemon/main.go -o docs/swagger
go test ./...
```
Expected: `docs/swagger/{docs.go,swagger.json,swagger.yaml}` ganham `/last-check` e as definições `daemon.CheckReport` / `daemon.Issue`; `go test ./...` PASS.

- [ ] **Step 6: Commit**

```bash
git add src/internal/api/endpoint_last_check.go src/internal/api/endpoint_last_check_test.go src/internal/api/server.go docs/swagger
git commit -m "feat: endpoint GET /api/v1/last-check"
```

---

### Task 7: Frontend — cliente, i18n e a montagem das frases

Ainda nenhuma tela muda. Esta task entrega o tipo TS, a chamada, as mensagens nos dois idiomas e a função pura que traduz um `Issue` em frase — testável sem renderizar nada.

**Files:**
- Modify: `src/internal/frontend/src/lib/api/client.ts` (tipos + `getLastCheck`, ao lado de `getStatus` na linha 231)
- Create: `src/internal/frontend/src/lib/domain/checkIssue.ts`
- Create: `src/internal/frontend/tests/unit/checkIssue.test.ts`
- Modify: `src/internal/frontend/messages/en.json`, `src/internal/frontend/messages/pt-BR.json`

**Interfaces:**
- Consumes: o JSON de `GET /api/v1/last-check` (Task 6).
- Produces:
  - `export interface Issue` e `export interface CheckReport` em `client.ts`
  - `export async function getLastCheck(): Promise<CheckReport>`
  - `export function issueMessage(issue: Issue): string` e `export function batchNote(issue: Issue): string` (string vazia quando não há nota) em `lib/domain/checkIssue.ts`

- [ ] **Step 1: Write the failing test**

Crie `src/internal/frontend/tests/unit/checkIssue.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { issueMessage, batchNote } from '../../src/lib/domain/checkIssue.js'
import type { Issue } from '../../src/lib/api/client.js'

function issue(overrides: Partial<Issue> = {}): Issue {
  return { anime_id: 1, anime_name: 'Bleach', code: 'no_torrent_found', ...overrides }
}

describe('issueMessage', () => {
  it('interpola candidatos e teto no problema de tamanho', () => {
    const text = issueMessage(issue({ code: 'all_above_size_limit', candidates: 8, limit_gb: 3 }))
    expect(text).toContain('8')
    expect(text).toContain('3')
  })

  it('interpola candidatos e piso de seeders', () => {
    const text = issueMessage(issue({ code: 'no_seeders', candidates: 4, min_seeders: 5 }))
    expect(text).toContain('4')
    expect(text).toContain('5')
  })

  it('interpola baixados e pendentes no limite por anime', () => {
    const text = issueMessage(issue({ code: 'max_episodes_per_anime', downloaded: 12, pending: 35 }))
    expect(text).toContain('12')
    expect(text).toContain('35')
  })

  it('tem frase para cada código conhecido', () => {
    const codes = [
      'all_above_size_limit',
      'no_seeders',
      'no_torrent_found',
      'disk_full',
      'torrent_rejected',
      'max_episodes_per_anime',
    ]
    for (const code of codes) {
      expect(issueMessage(issue({ code })), code).not.toBe('')
    }
  })

  // Um codigo novo no backend nao pode virar linha em branco na tela: o fallback e o proprio
  // codigo, feio e legivel, em vez de silencio.
  it('cai no código cru quando não conhece a frase', () => {
    expect(issueMessage(issue({ code: 'codigo_do_futuro' }))).toContain('codigo_do_futuro')
  })
})

describe('batchNote', () => {
  it('explica cada motivo de batch desligado', () => {
    for (const value of ['no_result', 'above_size_limit', 'no_coverage']) {
      expect(batchNote(issue({ code: 'max_episodes_per_anime', batch_skipped: value })), value).not.toBe('')
    }
  })

  it('é vazio quando o anime nunca foi elegível a batch', () => {
    expect(batchNote(issue({ code: 'max_episodes_per_anime' }))).toBe('')
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd src/internal/frontend && bun run test:unit -- checkIssue`
Expected: FAIL — `Cannot find module '../../src/lib/domain/checkIssue.js'`.

- [ ] **Step 3: Add the messages (en + pt-BR)**

Acrescente a `src/internal/frontend/messages/en.json` (mantendo o arquivo ordenado como ele já está):

```json
  "lastcheck_section_problems": "Problems ({count})",
  "lastcheck_section_limits": "Limits applied ({count})",
  "lastcheck_episodes": "ep. {episodes}",
  "lastcheck_all_above_size_limit": "{candidates} torrents found, all above the {limit} GB ceiling.",
  "lastcheck_no_seeders": "{candidates} torrents found, none with at least {seeders} seeders.",
  "lastcheck_no_torrent_found": "No torrent found on Nyaa.",
  "lastcheck_disk_full": "Not enough free disk space.",
  "lastcheck_torrent_rejected": "The torrent client rejected all {candidates} magnets.",
  "lastcheck_max_episodes_per_anime": "Per-anime limit reached: {downloaded} downloaded, {pending} still waiting.",
  "lastcheck_batch_no_result": "No batch torrent was found for this anime.",
  "lastcheck_batch_above_size_limit": "A batch was found but it is above the batch size ceiling.",
  "lastcheck_batch_no_coverage": "The batches found do not cover the pending episodes.",
  "lastcheck_unknown": "Unknown reason ({code}).",
  "detail_lastcheck_notice": "The last verification could not download episodes of this anime.",
  "detail_lastcheck_episode_note": "not downloaded in the last check",
```

E os equivalentes em `src/internal/frontend/messages/pt-BR.json`:

```json
  "lastcheck_section_problems": "Problemas ({count})",
  "lastcheck_section_limits": "Limites aplicados ({count})",
  "lastcheck_episodes": "ep. {episodes}",
  "lastcheck_all_above_size_limit": "{candidates} torrents encontrados, todos acima do teto de {limit} GB.",
  "lastcheck_no_seeders": "{candidates} torrents encontrados, nenhum com pelo menos {seeders} seeders.",
  "lastcheck_no_torrent_found": "Nenhum torrent encontrado no Nyaa.",
  "lastcheck_disk_full": "Espaço em disco insuficiente.",
  "lastcheck_torrent_rejected": "O cliente de torrent recusou todos os {candidates} magnets.",
  "lastcheck_max_episodes_per_anime": "Limite por anime atingido: {downloaded} baixados, {pending} na espera.",
  "lastcheck_batch_no_result": "Nenhum torrent de batch foi encontrado para este anime.",
  "lastcheck_batch_above_size_limit": "Um batch foi encontrado, mas está acima do teto de tamanho de batch.",
  "lastcheck_batch_no_coverage": "Os batches encontrados não cobrem os episódios pendentes.",
  "lastcheck_unknown": "Motivo desconhecido ({code}).",
  "detail_lastcheck_notice": "A última verificação não conseguiu baixar episódios deste anime.",
  "detail_lastcheck_episode_note": "não baixou na última verificação",
```

- [ ] **Step 4: Add the client types and call**

Em `src/internal/frontend/src/lib/api/client.ts`, acrescente logo depois de `interface TorrentInfo` (linha 229):

```ts
/** Uma linha do relatório da última verificação: um par (anime, código). */
export interface Issue {
  anime_id: number
  anime_name: string
  /** Só vem em problema. Limite não tem "episódios afetados" — tem downloaded/pending. */
  episodes?: number[]
  code: string
  candidates?: number
  limit_gb?: number
  min_seeders?: number
  downloaded?: number
  pending?: number
  batch_skipped?: string
}

/** O relatório do ÚLTIMO passe, e só dele. Não é histórico. */
export interface CheckReport {
  finished_at: string
  pass_error: string
  problems: Issue[]
  limits: Issue[]
}
```

E a chamada, logo depois de `getStatus` (linha 233):

```ts
export async function getLastCheck(): Promise<CheckReport> {
  // Polled junto do poll de torrents do Status e do AnimeDetail: uma falha transitória deve
  // degradar em silêncio em vez de tostar a cada tick.
  return apiRequest<CheckReport>('GET', '/last-check', null, { silent: true })
}
```

> Confirme a assinatura de `apiRequest` (topo de `client.ts`) antes de copiar — use exatamente a forma de `getTorrents` (linha 333), que é o único outro `silent: true` do arquivo.

- [ ] **Step 5: Write the message builder**

Crie `src/internal/frontend/src/lib/domain/checkIssue.ts`:

```ts
// A fronteira é a mesma que lib/domain/animeState.ts documenta e defende: o backend manda
// CÓDIGO + NÚMEROS, o frontend monta a frase. Uma string pronta vinda do Go obrigaria o daemon
// a saber o locale do navegador.
import * as m from '../i18n/messages.js'
import type { Issue } from '../api/client.js'

export function issueMessage(issue: Issue): string {
  switch (issue.code) {
    case 'all_above_size_limit':
      return m.lastcheck_all_above_size_limit({
        candidates: issue.candidates ?? 0,
        limit: issue.limit_gb ?? 0,
      })
    case 'no_seeders':
      return m.lastcheck_no_seeders({
        candidates: issue.candidates ?? 0,
        seeders: issue.min_seeders ?? 0,
      })
    case 'no_torrent_found':
      return m.lastcheck_no_torrent_found()
    case 'disk_full':
      return m.lastcheck_disk_full()
    case 'torrent_rejected':
      return m.lastcheck_torrent_rejected({ candidates: issue.candidates ?? 0 })
    case 'max_episodes_per_anime':
      return m.lastcheck_max_episodes_per_anime({
        downloaded: issue.downloaded ?? 0,
        pending: issue.pending ?? 0,
      })
    default:
      // Um código novo no backend não pode virar linha em branco na tela.
      return m.lastcheck_unknown({ code: issue.code })
  }
}

/** A explicação de por que o batch não entrou naquele anime. "" quando ele nunca foi elegível. */
export function batchNote(issue: Issue): string {
  switch (issue.batch_skipped) {
    case 'no_result':
      return m.lastcheck_batch_no_result()
    case 'above_size_limit':
      return m.lastcheck_batch_above_size_limit()
    case 'no_coverage':
      return m.lastcheck_batch_no_coverage()
    default:
      return ''
  }
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd src/internal/frontend && bun run test:unit -- checkIssue`
Expected: PASS (o `test:unit` roda `i18n:compile` antes, então as mensagens novas viram funções `m.*`)

- [ ] **Step 7: Run the frontend suite**

Run: `cd src/internal/frontend && bun run test:unit && bun run check`
Expected: PASS, sem erro de tipo

- [ ] **Step 8: Commit**

```bash
git add src/internal/frontend/src/lib/api/client.ts src/internal/frontend/src/lib/domain/checkIssue.ts src/internal/frontend/tests/unit/checkIssue.test.ts src/internal/frontend/messages/
git commit -m "feat: cliente, i18n e montagem das frases do relatório de verificação"
```

---

### Task 8: Status.svelte mostra o relatório

O alerta de `has_error` passa a mostrar `pass_error` (na mesma posição), e um card *full-width* acima da lista de animes aparece **só quando há entradas**. Passe limpo não renderiza nada: um elemento que está sempre ali deixa de ser lido, e aí o dia em que ele tem conteúdo passa batido.

**Files:**
- Modify: `src/internal/frontend/src/routes/Status.svelte` — imports (linha 13-22), `T` (linha 56-87), estado (~linha 90), `loadTorrents` (linha 252), markup do alerta (linha 495-502) e um bloco novo logo depois
- Modify: `src/internal/frontend/tests/smoke/helpers.ts:113-118` (`setupStatusPageMocks`)
- Create: `src/internal/frontend/tests/component/Status.lastCheck.test.ts`

**Interfaces:**
- Consumes: `getLastCheck`, `CheckReport`, `issueMessage`, `batchNote` (Task 7).
- Produces: nada consumido por outras tasks.

- [ ] **Step 1: Write the failing test**

Crie `src/internal/frontend/tests/component/Status.lastCheck.test.ts` (siga o padrão de mock de módulo de `tests/component/AnimeDetail.batchRow.test.ts` — stube **todos** os named exports que `Status.svelte` importa, senão sobra binding indefinido):

```ts
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import { tick } from 'svelte'
import Status from '../../src/routes/Status.svelte'
import * as client from '../../src/lib/api/client.js'
import type { CheckReport, StatusResponse } from '../../src/lib/api/client.js'

vi.mock('../../src/lib/api/client.js', () => ({
  getStatus: vi.fn(),
  getAnimes: vi.fn(),
  getConfig: vi.fn(),
  triggerCheck: vi.fn(),
  startDaemon: vi.fn(),
  stopDaemon: vi.fn(),
  getTorrents: vi.fn(),
  getLastCheck: vi.fn(),
}))

vi.mock('../../src/lib/websocket/client.js', () => ({
  WebSocketClient: class {
    connect() {}
    disconnect() {}
  },
}))

function statusResponse(overrides: Partial<StatusResponse> = {}): StatusResponse {
  return {
    status: 'running',
    last_check: '2026-08-19T12:00:00Z',
    has_error: false,
    version: '1.0.0',
    disk_total: 100,
    disk_free: 90,
    disk_low: false,
    ...overrides,
  }
}

const emptyReport: CheckReport = {
  finished_at: '0001-01-01T00:00:00Z',
  pass_error: '',
  problems: [],
  limits: [],
}

describe('Status — relatório da última verificação', () => {
  it('não renderiza nada quando o passe foi limpo', async () => {
    vi.mocked(client.getStatus).mockResolvedValue(statusResponse())
    vi.mocked(client.getAnimes).mockResolvedValue([])
    vi.mocked(client.getConfig).mockResolvedValue({ check_interval: 60 } as never)
    vi.mocked(client.getTorrents).mockResolvedValue([])
    vi.mocked(client.getLastCheck).mockResolvedValue(emptyReport)

    render(Status)
    await tick()
    await tick()

    expect(screen.queryByTestId('last-check-report')).toBeNull()
  })

  it('lista problemas e limites quando existem', async () => {
    vi.mocked(client.getStatus).mockResolvedValue(statusResponse())
    vi.mocked(client.getAnimes).mockResolvedValue([])
    vi.mocked(client.getConfig).mockResolvedValue({ check_interval: 60 } as never)
    vi.mocked(client.getTorrents).mockResolvedValue([])
    vi.mocked(client.getLastCheck).mockResolvedValue({
      finished_at: '2026-08-19T12:00:00Z',
      pass_error: '',
      problems: [{ anime_id: 269, anime_name: 'Bleach', episodes: [12, 15], code: 'all_above_size_limit', candidates: 8, limit_gb: 3 }],
      limits: [{ anime_id: 21, anime_name: 'One Piece', code: 'max_episodes_per_anime', downloaded: 12, pending: 35, batch_skipped: 'no_result' }],
    })

    render(Status)
    await tick()
    await tick()

    const card = await screen.findByTestId('last-check-report')
    expect(card.textContent).toContain('Bleach')
    expect(card.textContent).toContain('One Piece')
    expect(card.textContent).toContain('8')
    expect(card.textContent).toContain('35')
  })

  it('mostra a mensagem do erro de passe no lugar do texto genérico', async () => {
    vi.mocked(client.getStatus).mockResolvedValue(statusResponse({ has_error: true }))
    vi.mocked(client.getAnimes).mockResolvedValue([])
    vi.mocked(client.getConfig).mockResolvedValue({ check_interval: 60 } as never)
    vi.mocked(client.getTorrents).mockResolvedValue([])
    vi.mocked(client.getLastCheck).mockResolvedValue({ ...emptyReport, pass_error: 'anilist unreachable' })

    render(Status)
    await tick()
    await tick()

    expect(await screen.findByText(/anilist unreachable/)).toBeTruthy()
  })
})
```

> Se `Status.svelte` importar outros módulos com efeito colateral no mount (`speedHistory`, `stallTracker`, `locale`), deixe-os reais — são stores puras. Só o cliente HTTP e o WebSocket precisam de mock. Ajuste o objeto de `getConfig` para o shape mínimo que `loadInitialData` consome (leia `Status.svelte:265-280`).

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd src/internal/frontend && bun run test:component -- Status.lastCheck`
Expected: FAIL — `getLastCheck is not a function` / `last-check-report` não encontrado.

- [ ] **Step 3: Write minimal implementation — script**

Em `src/internal/frontend/src/routes/Status.svelte`:

Acrescente ao import de `../lib/api/client.js` (linha 13-22): `getLastCheck,` e `type CheckReport,`.

Acrescente o import do domínio, junto dos outros `lib/domain/` (linha 38):

```ts
  import { issueMessage, batchNote } from "../lib/domain/checkIssue.js";
```

Acrescente ao objeto `T` (dentro do bloco `$: T = $locale && {`, linha 56):

```ts
    reportProblems: (count: number) => m.lastcheck_section_problems({ count }),
    reportLimits: (count: number) => m.lastcheck_section_limits({ count }),
```

Acrescente o estado, junto de `let torrents` (linha 92):

```ts
  // Relatório do último passe. Buscado junto do poll de torrents que esta tela já mantém —
  // é a mesma cadência e o mesmo custo de uma requisição a mais por tick.
  let lastCheck: CheckReport | null = null;
```

E o derivado, junto dos outros `$:` (depois da linha 122):

```ts
  $: hasReport = !!lastCheck && (lastCheck.problems.length > 0 || lastCheck.limits.length > 0);
  // O erro de passe vence o texto genérico quando existe; o genérico continua sendo o fallback
  // para o instante entre o has_error chegar e o relatório ser buscado.
  $: passErrorText = lastCheck?.pass_error || (T && T.errorAlert);
```

Em `loadTorrents` (linha 252), busque o relatório no mesmo tick. **Em `Promise.allSettled`, não `Promise.all`**: uma falha do relatório não pode derrubar o poll de torrents, que é o que alimenta o sparkline.

```ts
  async function loadTorrents() {
    const [torrentsResult, reportResult] = await Promise.allSettled([
      getTorrents(),
      getLastCheck(),
    ]);

    if (torrentsResult.status === "fulfilled") {
      torrents = torrentsResult.value;
      torrentsStale = false;
      // spec §8: alimentar o histórico SÓ em poll bem-sucedido. Não existe "repetir a última
      // amostra" — o sparkline congelar é o comportamento correto, fingir stream contínuo não.
      speedHistory.push(totalSpeeds(torrents).download);
      stallTracker.sync(torrents, Date.now());
    } else {
      torrentsStale = true;
      console.error("Failed to load torrents:", torrentsResult.reason);
    }

    if (reportResult.status === "fulfilled") {
      lastCheck = reportResult.value;
    }
  }
```

- [ ] **Step 4: Write minimal implementation — markup**

Em `Status.svelte:495-502`, troque o texto do alerta de erro:

```svelte
    {#if status.has_error && status.status !== "checking"}
      <div
        role="alert"
        class="flex items-center gap-2 rounded-field border border-warn-tint/32 bg-warn-tint/12 px-3.5 py-2.5 text-copy text-warn"
      >
        {passErrorText}
      </div>
    {/if}
```

E acrescente o card logo depois desse bloco, ainda **antes** do `<div class="grid gap-3.5 lg:grid-cols-[1.15fr_1fr]">`:

```svelte
    <!-- Estado vazio é a regra: passe limpo não renderiza nada. Um card permanente anunciando
         "0 problemas" deixa de ser lido, e aí o dia em que ele tem conteúdo passa batido. -->
    {#if hasReport && lastCheck}
      <section
        data-testid="last-check-report"
        class="rounded-card border border-default bg-card p-4.5"
      >
        {#if lastCheck.problems.length > 0}
          <p class="font-mono text-mono-label uppercase text-subtle">
            {T && T.reportProblems(lastCheck.problems.length)}
          </p>
          <ul class="mt-2 space-y-1.5">
            {#each lastCheck.problems as issue (`${issue.anime_id}-${issue.code}`)}
              <li class="text-copy text-heading">
                <span class="font-semibold">{issue.anime_name}</span>
                {#if issue.episodes && issue.episodes.length > 0}
                  <span class="text-subtle">
                    · {$locale && m.lastcheck_episodes({ episodes: issue.episodes.join(", ") })}
                  </span>
                {/if}
                <span class="text-tertiary">· {issueMessage(issue)}</span>
              </li>
            {/each}
          </ul>
        {/if}

        {#if lastCheck.limits.length > 0}
          <p
            class="font-mono text-mono-label uppercase text-subtle"
            class:mt-4={lastCheck.problems.length > 0}
          >
            {T && T.reportLimits(lastCheck.limits.length)}
          </p>
          <ul class="mt-2 space-y-1.5">
            {#each lastCheck.limits as issue (`${issue.anime_id}-${issue.code}`)}
              <li class="text-copy text-heading">
                <span class="font-semibold">{issue.anime_name}</span>
                <span class="text-tertiary">· {issueMessage(issue)}</span>
                {#if batchNote(issue)}
                  <span class="text-subtle">· {batchNote(issue)}</span>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/if}
```

- [ ] **Step 5: Update the smoke-test mocks**

Em `src/internal/frontend/tests/smoke/helpers.ts`, acrescente a rota em `setupStatusPageMocks` (depois da linha 116). Sem ela a página bate no servidor real do Playwright e o poll silencioso engole o erro — a tela funciona, mas o teste passa por acidente:

```ts
  await page.route('**/api/v1/last-check', route => route.fulfill({
    json: { success: true, data: { finished_at: '0001-01-01T00:00:00Z', pass_error: '', problems: [], limits: [] }, error: null },
  }))
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd src/internal/frontend && bun run test:component -- Status.lastCheck`
Expected: PASS (3 testes)

- [ ] **Step 7: Run the frontend suite**

Run: `cd src/internal/frontend && bun run test:unit && bun run test:component && bun run check`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add src/internal/frontend/src/routes/Status.svelte src/internal/frontend/tests/component/Status.lastCheck.test.ts src/internal/frontend/tests/smoke/helpers.ts
git commit -m "feat: tela de Status mostra o relatório da última verificação"
```

---

### Task 9: AnimeDetail.svelte mostra o motivo do anime

Mesma chamada, filtrada por `anime_id`: um aviso no topo da lista e uma marca nas linhas dos episódios listados em `Issue.episodes`.

**Files:**
- Modify: `src/internal/frontend/src/routes/AnimeDetail.svelte` — imports (linha 13-28), estado, `pollTorrents` (linha ~589), `episodeNotes` (linha 216), `buildRows` (linha ~325) e o markup acima da lista
- Modify: `src/internal/frontend/tests/component/AnimeDetail.batchRow.test.ts`, `AnimeDetail.progress.test.ts`, `AnimeDetail.torrentPolling.test.ts` (acrescentar `getLastCheck: vi.fn()` ao `vi.mock` e um `mockResolvedValue` de relatório vazio)
- Create: `src/internal/frontend/tests/component/AnimeDetail.lastCheck.test.ts`

**Interfaces:**
- Consumes: `getLastCheck`, `Issue`, `issueMessage`, `batchNote` (Task 7).
- Produces: nada consumido por outras tasks.

- [ ] **Step 1: Fix the existing mocks first**

`AnimeDetail.svelte` vai passar a importar `getLastCheck`, e os três `vi.mock` existentes listam **cada** named export explicitamente — sem a linha nova, o binding fica indefinido e os três arquivos quebram no mount. Em `tests/component/AnimeDetail.batchRow.test.ts`, `AnimeDetail.progress.test.ts` e `AnimeDetail.torrentPolling.test.ts`, acrescente ao objeto do `vi.mock('../../src/lib/api/client.js', ...)`:

```ts
  getLastCheck: vi.fn(),
```

E, em cada teste que renderiza o componente, junto dos outros `mockResolvedValue`:

```ts
    vi.mocked(client.getLastCheck).mockResolvedValue({
      finished_at: '0001-01-01T00:00:00Z', pass_error: '', problems: [], limits: [],
    })
```

> Se algum desses arquivos usar um helper compartilhado de setup, ponha o `mockResolvedValue` lá uma vez só.

- [ ] **Step 2: Write the failing test**

Crie `src/internal/frontend/tests/component/AnimeDetail.lastCheck.test.ts`, copiando o cabeçalho de mocks de `AnimeDetail.batchRow.test.ts` (incluindo `getLastCheck`) e:

```ts
describe('AnimeDetail — motivo da última verificação', () => {
  it('não mostra nada quando o anime não tem problema no relatório', async () => {
    const detail: AnimeDetailResponse = {
      anime_id: 42, total_episodes: 3, progress: 0, status: 'CURRENT',
      episodes: [{ episode_number: 1, airing_at: 0, time_until_airing: 0, is_aired: true, is_watched: false, is_downloaded: false }],
    }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])
    vi.mocked(client.getLastCheck).mockResolvedValue({
      finished_at: '2026-08-19T12:00:00Z', pass_error: '', limits: [],
      // Problema de OUTRO anime: não pode vazar para esta tela.
      problems: [{ anime_id: 999, anime_name: 'Outro', episodes: [1], code: 'no_torrent_found' }],
    })

    render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    expect(screen.queryByTestId('anime-last-check')).toBeNull()
  })

  it('mostra o aviso e marca o episódio afetado', async () => {
    const detail: AnimeDetailResponse = {
      anime_id: 42, total_episodes: 3, progress: 0, status: 'CURRENT',
      episodes: [
        { episode_number: 1, airing_at: 0, time_until_airing: 0, is_aired: true, is_watched: false, is_downloaded: false },
        { episode_number: 2, airing_at: 0, time_until_airing: 0, is_aired: true, is_watched: false, is_downloaded: false },
      ],
    }
    vi.mocked(client.getAnimeDetail).mockResolvedValue(detail)
    vi.mocked(client.getAnimes).mockResolvedValue([animeInfo({})])
    vi.mocked(client.getTorrents).mockResolvedValue([])
    vi.mocked(client.getLastCheck).mockResolvedValue({
      finished_at: '2026-08-19T12:00:00Z', pass_error: '', limits: [],
      problems: [{ anime_id: 42, anime_name: 'Test Anime', episodes: [2], code: 'all_above_size_limit', candidates: 8, limit_gb: 3 }],
    })

    render(AnimeDetail, { props: { params: { id: '42' } } })
    await tick(); await tick()

    const notice = await screen.findByTestId('anime-last-check')
    expect(notice.textContent).toContain('8')

    const rows = screen.getAllByTestId ? [] : []
    // A marca vive na linha do episódio 2 e NÃO na do 1.
    const marks = await screen.findAllByText(/última verificação|last check/i)
    expect(marks.length).toBeGreaterThan(0)
  })
})
```

> Ajuste o segundo `expect` ao que o markup do Step 4 realmente produzir. O ponto que o teste tem de travar é: o aviso aparece, e a nota do episódio aparece **só** para os números listados em `Issue.episodes`. Se ficar mais fácil, dê `data-testid={"ep-note-" + row.ep.episode_number}` à `<p>` de `row.notes` e asserte nela.

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd src/internal/frontend && bun run test:component -- AnimeDetail`
Expected: FAIL em `AnimeDetail.lastCheck` (`anime-last-check` não encontrado); os outros três devem PASSAR já com o Step 1 aplicado.

- [ ] **Step 4: Write minimal implementation**

Em `src/internal/frontend/src/routes/AnimeDetail.svelte`:

Acrescente ao import de `../lib/api/client.js`: `getLastCheck,` e `type Issue,`.

Acrescente, junto dos outros imports de domínio (linha ~46):

```ts
  import { issueMessage, batchNote } from "../lib/domain/checkIssue.js";
```

Acrescente o estado, junto de `let torrents` (procure a declaração em `AnimeDetail.svelte`):

```ts
  // Só as entradas deste anime. Mesma chamada do Status, filtrada por anime_id: um endpoint com
  // dois consumidores em vez da mesma informação espalhada por três formas que precisariam
  // concordar entre si.
  let animeIssues: Issue[] = [];
```

E os derivados, junto dos outros `$:`:

```ts
  // Só problema marca episódio: limite não tem "episódios afetados" (o daemon parou de
  // considerar episódios ao atingir a conta).
  $: issueByEpisode = new Map<number, Issue>(
    animeIssues.flatMap((i) => (i.episodes ?? []).map((n) => [n, i] as [number, Issue])),
  );
```

Em `pollTorrents` (linha ~589), busque o relatório no mesmo tick, em `allSettled` pelo mesmo motivo do Status:

```ts
  async function pollTorrents() {
    const [torrentsResult, reportResult] = await Promise.allSettled([
      getTorrents(),
      getLastCheck(),
    ]);

    if (torrentsResult.status === "fulfilled") {
      torrents = torrentsResult.value;
      lastPollAt = Date.now();
      stallTracker.sync(torrents, lastPollAt);
    } else {
      // Best-effort accessory data — no toast, keep the last known snapshot.
      console.error("Failed to load torrents:", torrentsResult.reason);
    }

    if (reportResult.status === "fulfilled") {
      const report = reportResult.value;
      animeIssues = [...report.problems, ...report.limits].filter((i) => i.anime_id === animeId);
    }

    scheduleNextTorrentPoll();
  }
```

> Repare que o `finally { scheduleNextTorrentPoll() }` original vira uma chamada direta no fim — `Promise.allSettled` nunca rejeita, então não há mais nada para o `try/catch` cobrir. Se preferir manter o `try/finally`, mantenha; o que **não** pode acontecer é o reagendamento parar de rodar.

Em `episodeNotes` (linha 216), acrescente o parâmetro e a nota:

```ts
  function episodeNotes(ep: AnimeEpisodeInfo, issue: Issue | undefined): string {
    const notes: string[] = [];
    if (ep.is_watched) notes.push(m.detail_badge_watched());
    if (ep.is_manually_managed) notes.push(m.detail_flag_no_delete_short());
    if (ep.is_blocked) notes.push(m.detail_flag_no_download_short());
    // A marca do relatório vai na mesma linha das outras notas: é uma frase curta, não um
    // estado ao vivo — deriveAnimeChip fica fora disto de propósito (ver decisions.md).
    if (issue) notes.push(m.detail_lastcheck_episode_note());
    return notes.join(" · ");
  }
```

No `buildRows`, passe o issue no ponto onde a linha de episódio é montada (linha ~333):

```ts
        notes: episodeNotes(ep, issueByEpisode.get(ep.episode_number)),
```

E torne a reatividade explícita, na linha do `$: rows` (linha ~344):

```ts
  $: rows = $locale && issueByEpisode && buildRows(allEpisodes, torrentsByEpisode);
```

Acrescente o aviso no markup, logo **acima** do cabeçalho da lista de episódios (o bloco `<!-- Desktop -->`, linha ~944):

```svelte
{#if animeIssues.length > 0}
  <div
    data-testid="anime-last-check"
    role="status"
    class="mb-3 rounded-field border border-warn-tint/32 bg-warn-tint/12 px-3.5 py-2.5 text-copy text-warn"
  >
    <p>{$locale && m.detail_lastcheck_notice()}</p>
    <ul class="mt-1 space-y-1 text-caption">
      {#each animeIssues as issue (issue.code)}
        <li>
          {issueMessage(issue)}{#if batchNote(issue)} · {batchNote(issue)}{/if}
        </li>
      {/each}
    </ul>
  </div>
{/if}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd src/internal/frontend && bun run test:component -- AnimeDetail`
Expected: PASS (os quatro arquivos)

- [ ] **Step 6: Run the frontend suite**

Run: `cd src/internal/frontend && bun run test:unit && bun run test:component && bun run check`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add src/internal/frontend/src/routes/AnimeDetail.svelte src/internal/frontend/tests/component/
git commit -m "feat: detalhe do anime mostra o motivo do último passe"
```

---

### Task 10: Fechamento — docs, Swagger e a suíte inteira

**Files:**
- Modify: `docs/agents/architecture.md` (tabela de API, seção de `state.go`, seções de `search.go`/`episodes.go`)
- Modify: `docs/agents/decisions.md` (duas entradas novas, #60 e #61)
- Modify: `docs/swagger/` (regenerado)

**Interfaces:**
- Consumes: tudo das tasks anteriores.
- Produces: nada de código.

- [ ] **Step 1: Regenerate Swagger**

Run: `swag init -g src/cmd/daemon/main.go -o docs/swagger`
Expected: `docs/swagger/swagger.json` contém `"/last-check"` e as definições `daemon.CheckReport` e `daemon.Issue`. Confirme:

```bash
grep -c 'last-check' docs/swagger/swagger.json && grep -c 'daemon.Issue' docs/swagger/swagger.json
```

- [ ] **Step 2: Update `docs/agents/architecture.md` — tabela de API**

Acrescente a linha logo depois da de `/api/v1/status` (linha 75):

```markdown
| `GET` | `/api/v1/last-check` | `handleLastCheck` | `endpoint_last_check.go` — o relatório do último passe automático: `problems` (o que devia ter baixado e não baixou) e `limits` (a config funcionando como configurada), um `Issue` por par (anime, código), ordenado por `anime_name`. `pass_error` é `State.GetLastCheckError()`, e quando ele existe as duas listas estão vazias (`SetLastCheckError` limpa o relatório). Só memória: um passe limpo devolve listas vazias e um `finished_at` zero significa que o daemon ainda não completou um passe. Download manual fica fora — aquele caminho já devolve o erro na própria resposta HTTP |
```

- [ ] **Step 3: Update `docs/agents/architecture.md` — símbolos**

Na seção `### src/internal/daemon/state.go` (linha 241), acrescente:

```markdown
- `SetLastCheckReport(CheckReport)` / `GetLastCheckReport() CheckReport` — o relatório do último passe, em memória. `GetLastCheckReport` devolve **valor**, para o handler poder preencher `pass_error` sem escrever no objeto compartilhado.
- `SetLastCheckError` **limpa** `lastCheckReport`. Consequência: `SetLastCheckReport` tem de ser chamado depois do `SetLastCheckError(nil)` do fim do passe (ver decisions.md #61).
```

Acrescente uma seção nova para o arquivo novo, logo antes de `### src/internal/daemon/state.go`:

```markdown
### `src/internal/daemon/report.go`

- `Issue` / `CheckReport` — os tipos do relatório da última verificação, serializados direto pelo endpoint `/last-check`. Campos de detalhe achatados com `omitempty` (nunca um `map[string]any`: não gera Swagger nem tipo TS utilizável).
- Códigos: `IssueAllAboveSizeLimit`, `IssueNoSeeders`, `IssueNoTorrentFound`, `IssueDiskFull`, `IssueTorrentRejected` (problemas) e `IssueMaxEpisodesPerAnime` (limite). `BatchSkippedNoResult` / `BatchSkippedAboveSizeLimit` / `BatchSkippedNoCoverage` são detalhe do limite, não códigos.
- `searchIssue(...)` — a cascata de precedência dos três problemas de busca (ver decisions.md #60).
- `aggregateIssues(raw)` — um `Issue` por par (anime, código), separado em problemas e limites, ordenado por `AnimeName`.
```

Nas seções de `search.go` e `episodes.go` (procure-as; se não existirem como seções próprias, acrescente as linhas onde os símbolos já são citados):

```markdown
- `filterBySize` / `filterBySeeders` devolvem `([]nyaa.TorrentResult, int)` — o `int` é quantos descartaram. `filterSearchResults` devolve `([]nyaa.TorrentResult, dropStats)` e `partitionSearchResults` devolve `(packs, singles, packStats)`: é o que distingue "o Nyaa não devolveu nada" de "o filtro cortou tudo".
- `checkEpisode` devolve um terceiro valor, `skipCode string` — `IssueMaxEpisodesPerAnime` quando o limite por anime barrou o episódio, `""` em todo skip normal.
```

- [ ] **Step 4: Update `docs/agents/decisions.md`**

Acrescente as duas entradas no fim do arquivo (a última hoje é a #59):

````markdown
---

### 60. A precedência dos códigos do relatório é cascata, não conjunto

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

### 61. `SetLastCheckError` limpa o relatório da última verificação

**Location:** `daemon/state.go` — `SetLastCheckError`; `daemon/verification.go` — a ordem `SetLastCheckError(nil)` → `SetLastCheckReport(...)`.

**What it looks like:** um setter de erro que também apaga um campo não relacionado, e um `SetLastCheckReport` que **precisa** vir depois de um `SetLastCheckError(nil)` que já rodou três linhas antes. Parece acoplamento acidental, e a correção "óbvia" é separar as duas coisas.

**Why it's right:** as sete saídas antecipadas de `AnimeVerification` (config incompleta, migração do `save_path`, probe de hardlink, backend não inicializado, `Ensure`, migração de ids, AniList) já chamam `SetLastCheckError`. Com a limpeza dentro dela, **nenhuma delas precisa de linha nova** — e nenhuma pode esquecer. E a semântica é a certa: um passe que abortou antes de olhar anime nenhum não tem relatório por anime, tem `pass_error`. Sem a limpeza, a tela mostraria os problemas do passe *anterior* lado a lado com um erro de passe novo, sugerindo que os dois vieram da mesma verificação.

A segunda consequência é igualmente desejada: o cancelamento (`verification.go`, no `select` do `ctx.Done()`) chama `SetLastCheckError(nil)` e retorna, então **passe interrompido não deixa relatório** — ele estava incompleto, e um relatório parcial diria "só este anime teve problema" quando os outros nem chegaram a ser olhados.

**Trade-off aceito:** a ordem das duas chamadas no fim de `AnimeVerification` é significativa e não é óbvia lendo só aquele trecho. É por isso que existe um teste dedicado (`TestSetLastCheckReport_AfterClearingError`) travando exatamente essa ordem.

**Don't "fix" by:**
- Tirar a limpeza de dentro de `SetLastCheckError` e pôr uma chamada explícita em cada saída antecipada — sete lugares para esquecer um.
- Mover `SetLastCheckReport` para antes do `SetLastCheckError(nil)` "porque lê melhor" — o relatório é apagado no instante seguinte e a feature inteira vira silêncio.
- Persistir o relatório em disco para "sobreviver ao restart" — `createStartFunc` chama `AnimeVerification` imediatamente ao iniciar (`loop.go`, antes do primeiro `time.After`), então ele se reconstrói em segundos; um arquivo custaria persistência, migração e a chance de mostrar um relatório de dias atrás como se fosse do último passe.
````

- [ ] **Step 5: Run everything**

```bash
go test ./...
cd src/internal/frontend && bun run test:unit && bun run test:component && bun run check
```
Expected: PASS em tudo.

- [ ] **Step 6: Run the smoke tests**

Run: `cd src/internal/frontend && bun run test:smoke`
Expected: PASS. Se `status.spec.ts` ou `anime-detail.spec.ts` falhar por causa da chamada nova, a rota do Step 5 da Task 8 não cobriu aquele spec — acrescente o mesmo `page.route('**/api/v1/last-check', ...)` no setup que ele usa.

- [ ] **Step 7: Commit**

```bash
git add docs/
git commit -m "docs: relatório da última verificação (arquitetura, decisões, swagger)"
```

---

## Self-Review

**Cobertura da spec:**

| Seção da spec | Task |
|---|---|
| Objetivo 1 (o passe produz um relatório agregado por anime) | 4, 5 |
| Objetivo 2 (tela de Status mostra o relatório) | 8 |
| Objetivo 3 (detalhe do anime mostra o motivo dele) | 9 |
| Objetivo 4 (a mensagem de erro de passe deixa de ser descartada) | 6 (`pass_error`), 8 (alerta) |
| Escopo: só o passe automático | Nenhuma task toca `ManualDownloadEpisode` nem `endpoint_episode_actions.go` |
| Taxonomia — 5 problemas | 2 (constantes), 4 (emissão) |
| Taxonomia — 1 limite + `batch_skipped` | 2, 3, 4 |
| Precedência é cascata | 4 (`searchIssue` + teste), 10 (decisions.md #60) |
| Skips normais ficam fora | 3 (`skipCode == ""` + teste) |
| Coleta: assinatura 1 (`filterSearchResults` → `dropStats`) | 1 |
| Coleta: assinatura 2 (`checkEpisode` → `skipCode`) | 3 |
| Coleta: assinatura 3 (`animeProcessResult.issues`) | 4 |
| Armazenamento em memória, `SetLastCheckError` limpa | 2, 5, 10 (decisions.md #61) |
| API: `GET /last-check`, structs, agregação, `pass_error` | 2, 6 |
| Frontend: `client.ts`, `Status.svelte`, `AnimeDetail.svelte`, estado vazio, i18n | 7, 8, 9 |
| `deriveAnimeChip` não é tocado | Nenhuma task o modifica; a nota do episódio vai em `episodeNotes` |
| Testes listados na spec | 1 (dropStats), 4 (cascata), 3 (`skipCode`), 2 (`SetLastCheckError` limpa / ordem), 2 (agregação), 6 (`handleLastCheck`) |
| Fechamento (`go test`, swag, architecture.md, decisions.md) | 10 |

**Item da spec deliberadamente não implementado:** `batch_skipped == "too_many_episodes"`. O código que o produziria (`willBatchAnime` / `MaxBatchEpisodes`) não existe mais no repo — ver "Desvios da spec" no topo. Os três valores que o substituem cobrem o mesmo papel.

**Consistência de tipos:** `Issue`/`CheckReport` são definidos uma vez (Task 2, `daemon/report.go`), serializados direto pelo endpoint (Task 6) e espelhados em TS uma vez (Task 7). `dropStats` é criado na Task 1 e consumido nas Tasks 1 e 4. `skipCode` é criado na Task 3 e consumido na Task 3 (`selectEpisodes`). `episodeSelection.downloaded`/`.limitSkipped` são criados na Task 3 e consumidos na Task 4. `issueMessage`/`batchNote` são criados na Task 7 e consumidos nas Tasks 8 e 9.
