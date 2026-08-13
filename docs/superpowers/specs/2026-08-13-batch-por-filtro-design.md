# Batch por filtro de busca, não por bloqueio prévio

Data: 2026-08-13

Substitui a regra batch↔limite de
[`2026-08-10-limite-episodios-batch-design.md`](2026-08-10-limite-episodios-batch-design.md) na
parte em que ela decide elegibilidade **antes** de buscar. O resto daquela spec (teto de tamanho,
guarda de disco, a regra de que o limite por anime não se aplica a batch) continua valendo.

## Problema

`willBatchAnime` (`daemon/episodes.go:174`) decide se vale buscar batch olhando só metadado do
AniList: `FINISHED`, não-filme, `Media.Episodes` conhecido e dentro de `max_batch_episodes`
(default 30). Quem não passa nunca chega ao Nyaa.

Isso descarta a decisão pelo motivo errado. O que importa é **o torrent** que está lá — seu
tamanho, seus seeders, que episódios ele cobre — e nada disso é conhecido antes da busca. Três
consequências:

1. **Contagem de episódios é proxy ruim de tamanho.** Um pack de 26 episódios em remux 1080p tem
   300 GB e passa; um de 500 episódios em 480p tem 80 GB e é bloqueado. O teto que mede a
   restrição real (`max_batch_torrent_size_gb`) já existe e já roda — só nunca é alcançado por
   quem o teto de contagem barrou antes.
2. **O gate de `FINISHED` exclui exatamente quem mais depende de batch.** One Piece é `RELEASING`,
   então `willBatchAnime` devolve false por status, independente de qualquer teto. Série longa em
   exibição é justamente onde episódio solto antigo não existe no Nyaa: o que existe é pack.
3. **`Media.Episodes == nil` é inelegível.** Regra que só faz sentido porque havia uma contagem
   para comparar.

Série longa expõe, junto, três coisas que hoje não aparecem porque pack quase nunca chega:

4. **Anime avulso não tem progresso.** `anilist.GetMediaByID` (`anilist/standalone.go`) devolve
   `Progress: 0` fixo. Sem progresso, nada nunca é "assistido", a poda nunca roda e o rodízio de
   packs sucessivos não tem o que o mova.
5. **Batch e multi-episódio pedem a MESMA URL ao Nyaa, duas vezes.** Ver item 6.
6. **A tela de episódios trata pack como 100 episódios independentes.** Ver item 7.

## Objetivo

1. Elegibilidade a batch deixa de ser decidida por metadado: passa a ser decidida pelo resultado
   filtrado da busca no Nyaa.
2. `max_batch_episodes` é **removido**. `max_batch_torrent_size_gb` fica como guarda única e ganha
   default positivo.
3. Pack parcial (`One Piece 001-100` para uma série de 1100) vira candidato seguro: só os
   episódios dentro da faixa recebem o magnet dele, e packs sucessivos cobrem a janela pedida.
4. Nenhum episódio de pack é apagado por estourar `max_episodes_per_anime`.
5. Anime avulso ganha progresso editável e passa a percorrer exatamente os caminhos do anime de
   lista — sem um `if isStandalone` novo em lugar nenhum.
6. Batch e multi-episódio passam a ser uma requisição só.
7. A tela de episódios mostra um pack como uma linha, com as ações que valem para um pack.

Não-objetivo: mudar a ordenação de resultados ou o caminho de filme.

## A mudança

### 1. `willBatchAnime` sai

Elegibilidade vira `!isAnimeMovie(anime)`. Some o gate de status, o teto de contagem e a regra do
`nil` — sem comparação de contagem, contagem desconhecida deixa de importar. A função inteira é
deletada; o predicado é curto o bastante para viver inline nos dois pontos de uso.

### 2. Ordem invertida: seleção → busca → re-seleção

Hoje o loop de episódios roda **uma vez** com o limite já levantado por um palpite (`willBatch`), e
o tampão de `episodes.go:87` corta a lista depois quando o palpite erra. Com todo anime não-filme
elegível, o palpite passa a ser sempre "levanta" — e aí o limite por anime deixa de podar
episódios salvos para todo mundo, porque `handleAlreadySavedEpisode` nunca dispara com
`maxEpisodes = len(episodes) + 1`. O tampão não conserta isso: ele corta `episodesToDownload`, não
restaura `keysToDelete`.

O loop vira função pura, extraída de `processAnimeEpisodes`:

```go
type episodeSelection struct {
	toDownload   []anilist.AiringNode
	keysToDelete []files.EpisodeKey
	checked      []files.EpisodeKey
}

func selectEpisodes(configs *files.Config, maxEpisodes int, anime anilist.MediaList,
	episodes []anilist.AiringNode, savedEpisodesMap map[files.EpisodeKey]bool,
	savedEpisodesFullMap map[files.EpisodeKey]files.EpisodeStruct,
	torrentsHashSet map[string]bool, keepSet, blockedMap map[files.EpisodeKey]bool) episodeSelection
```

Sem I/O e sem estado, então chamá-la duas vezes é de graça. `processAnimeEpisodes` passa a:

```go
sel := selectEpisodes(configs, effectiveMax(configs, episodes), ...)  // decisões de deleção corretas

var batches []nyaa.TorrentResult
if !isAnimeMovie(anime) && len(sel.toDownload) > 1 {
	firstPending := sel.toDownload[0].Episode
	batches = pickBatches(batchCandidates, firstPending, windowEnd(configs, firstPending))
	if len(batches) > 0 {
		sel = selectEpisodes(configs, len(episodes)+1, ...)           // limite levantado
	}
}
```

A primeira chamada é a que vale quando não há batch, e é ela que produz as deleções por limite.
A segunda só existe para que os registros de `episodes.json` cubram os packs inteiros — a razão já
documentada em `episodes.go:55-57`.

`effectiveMax` é onde `max_episodes_per_anime == 0` vira `len(episodes) + 1` (ver Config): o mesmo
truque de "ilimitado" que a função já usava, agora também para o teto desligado. `windowEnd` é o
outro lado do mesmo campo — `firstPending + max_episodes_per_anime - 1`, ou `math.MaxInt` com o
teto desligado (item 3).

Com isso **saem**: o tampão (`episodes.go:83-93`), `resolvedIsBatch`, e a Estratégia 2 de
`resolveSearchStrategy`, que fica só filme → multi-episódio. O caminho de filme não muda em nada,
inclusive o episódio sintético que ele cria.

`debug.go:70-88` espelha esse mesmo trecho e passa a chamar `selectEpisodes` também, o que elimina
a duplicação que hoje precisa ser mantida em sincronia à mão.

### 3. Cobertura: o mínimo de packs que fecha a janela

Sem o teto de 30 episódios, um pack parcial vira candidato para série longa. Hoje isso não
machuca porque só pack de anime curto chegava lá; passa a machucar, porque `resolveSearchStrategy`
atribui `batchResult[0]` a **todos** os episódios pendentes — o episódio 800 receberia o magnet do
pack 1-100.

A janela a cobrir é o primeiro episódio pendente mais o limite por anime:

```
janela = [primeiroPendente, primeiroPendente + max_episodes_per_anime - 1]   se o teto > 0
janela = [primeiroPendente, ∞)                                               se o teto == 0
```

`pickBatches` anda essa janela com um cursor, sobre os resultados **já ordenados por qualidade e
já filtrados** por tamanho e seeders:

```go
// pickBatches devolve o mínimo de packs que cobre [firstPending, windowEnd], em ordem.
// results já vem ordenado por SortTorrentResults, então "o primeiro que cobre o cursor"
// é "o melhor que cobre o cursor".
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

Termina sempre: `coveringBatch` só devolve pack com `EndEpisode >= cursor`, então `cursor` cresce
estritamente a cada volta e nenhum pack pode ser escolhido duas vezes.

Cada episódio recebe o magnet do **seu** pack — o map por episódio de `resolveSearchStrategy` já
suporta isso sem mudar de tipo. O `overrideName` deixa de ser só `animeTitle` e passa a
`"<anime> <start>-<end>"` quando a faixa é conhecida: dois packs do mesmo anime no mesmo ciclo
apareceriam com nomes idênticos na tela de downloads. O nome da PASTA da biblioteca não muda —
`Organize` usa `AnimeName`, nunca `EpisodeName`.

Quando há cobertura parcial, `sel.toDownload` é truncado aos episódios que receberam magnet. Os de
fora não caem em episódio solto neste ciclo: em One Piece, o fallback para solto não acha nada e
dispararia `DownloadFailed`/`ReasonNotFound` para ~1000 episódios por passada. `episodes` vem da
agenda em ordem crescente e os packs escolhidos são contíguos a partir do primeiro pendente, então
o truncamento é sempre um corte de prefixo.

`extractBatchInfo` (`nyaa/nyaa.go:130`) existe e hoje é código morto — não é chamada por nenhum
caminho de produção. Passa a ser exportada como `ExtractBatchInfo`, na forma do shim `IsBatch` que
já está logo abaixo dela.

**Falso positivo de resolução.** `reBatchRange` é `(\d{1,4})\s*[-~]\s*(\d{1,4})`, que casa
`720-1080` em `[720-1080p]` e produziria uma faixa fantasma. Guarda: usar
`FindStringSubmatchIndex` e descartar o casamento quando o caractere imediatamente seguinte for
`p`, `P`, `i` ou `I`. Cobre o caso dominante; fica um comentário `ponytail:` apontando que outras
faixas fantasma (datas, bitrate) só entrariam com um sanitizador de tokens, que não se paga hoje.

**"Sem teto" custa caro de propósito.** Com `max_episodes_per_anime = 0`, a janela é aberta e One
Piece resolve ~14 packs num ciclo só. Os freios são `max_batch_torrent_size_gb`, por torrent, e
`checkDiskSpace`, que só freia depois que o disco encheu. É uma escolha explícita de quem zerou o
campo, mas vale a nota de release.

### 4. Episódio de pack não é apagado pelo limite por anime

No ciclo seguinte ao que trouxe o pack, o limite volta a `max_episodes_per_anime` e
`handleAlreadySavedEpisode` apagaria os episódios 13-100 que acabaram de ser baixados. Como o gate
de espaço reabriria em seguida, isso vira loop de baixar-e-apagar.

Mesma lógica de `episodes.go:55-57` — "limitar registros de um pack não limita bytes" — aplicada no
ponto de deleção em vez do de levantamento. `handleAlreadySavedEpisode` recebe o `IsBatch` do
registro salvo (já disponível em `savedEpisodesFullMap`) e, ao estourar o limite:

```go
if *downloadedEpisodes >= maxEpisodes {
	if isBatch {
		// O pack é a unidade: apagar um registro não libera byte nenhum, e o torrent
		// continua no disco. Não apaga — e também não baixa mais nada deste anime.
		return false, false
	}
	... deleta como hoje ...
}
```

O contador **continua** sendo incrementado pelos episódios de pack abaixo do limite, e é isso que
dá o ritmo de aquisição de graça: com um pack de 100 salvo e o limite em 12, o contador estoura no
episódio 12 e nenhum pack novo é buscado. Conforme o progresso avança, `shouldSkipEpisode` descarta
os assistidos antes de eles chegarem ao contador, a poda libera espaço, e o ciclo seguinte busca o
pack da faixa seguinte.

**O pack é a unidade do rodízio.** Com pack `1-100` e progresso marcado em 100, os 100 registros
entram no delete set, `allEpisodesInDeleteSet` (`episodes.go:639`) passa, o torrent inteiro sai, o
espaço volta e o ciclo seguinte pega `101-200`. Com progresso em 50, nada sai (há irmãos vivos),
nada de espaço volta e nada novo entra. Você só ganha o pack seguinte quando termina o atual, o
que é a leitura honesta de "tem espaço" quando um torrent é indivisível.

**`watched_episodes_to_keep` quebraria exatamente isso**, então passa a ignorar episódios de pack:
`buildWatchedKeepSet` pula os registros com `IsBatch`. Com `keep = 3` e um pack de 100, marcar
progresso 100 deletaria 97 registros e manteria 3 — `allEpisodesInDeleteSet` daria false, o torrent
ficaria no disco, o espaço não voltaria e o pack seguinte nunca viria. Manter 3 dos 100 de um pack
não guarda 3 episódios: guarda o pack inteiro. O keep-set não tem a granularidade que promete ali,
e é a mesma justificativa do parágrafo acima.

Isso preserva o comportamento atual do caso comum: anime finalizado de 26 episódios com pack
completo hoje não sofre poda (o limite fica levantado todo ciclo); depois da mudança também não
(os registros são `IsBatch`).

### 5. Progresso manual em anime avulso

O ritmo do item 4 depende de o progresso avançar, e anime avulso não tem progresso: `GetMediaByID`
devolve `Progress: 0` fixo. Sem isso, depois do primeiro pack aquele anime para para sempre —
e pack parcial, que é o objetivo central desta spec, não serviria para nada em avulso.

A saída não é uma exceção para avulso: é **dar a ele o dado que falta**, e deixá-lo percorrer os
mesmos caminhos.

**Onde mora.** `files.AnimeSettings` (`filemanager.go:149`) ganha `Progress int`. O store por anime
já existe (`anime_settings.json`, `map[int]AnimeSettings`) com `GET`/`PUT /animes/{id}/settings` já
roteados — nenhum arquivo novo, rota nova ou migração. Campo ausente lê como `0`, que é o
comportamento de hoje.

**Merge no PUT, não sobrescrita.** `animeSettingsRequest` hoje monta um `AnimeSettings` do zero, o
que já significa que um `PUT` sem `custom_search_query` o apaga. Com um segundo campo isso vira bug
de verdade: o `updateAnimeSettings(id, { custom_search_query })` que a tela dispara zeraria o
progresso. Os dois campos do request viram ponteiros e o handler carrega o salvo, sobrepondo só o
que veio não-nulo.

**Onde é injetado.** Os pontos que montam o `MediaList` sintético do avulso passam a sobrescrever
`ml.Progress` com o valor salvo:

- `appendStandaloneAnimes` (`daemon/standalone.go:54`) — o passe automático
- `DownloadStandaloneAnime` (`daemon/standalone.go:91`) — o download na hora de adicionar
- `resolveMediaList` (`api/standalone.go:38,52`) — as telas

São três linhas em cada, sobre a interface de FileManager do próprio pacote; não vale um helper
compartilhado porque `daemon` e `api` têm interfaces diferentes. Depois disso, `shouldSkipEpisode`,
`firstEpisodeToConsider`, `buildWatchedKeepSet`, a poda de assistidos e `EpisodesWatched`
(`endpoint_animes.go:263`, que já é `ml.Progress`) funcionam em avulso sem saber que é avulso.

A ordem de `verification.go:539` continua valendo e pelo mesmo motivo: os avulsos entram DEPOIS do
`DedupeByMedia`, e um id que também esteja numa lista real é removido do arquivo em vez de
competir. O progresso salvo nunca disputa com o da AniList.

**UI.** Os três pontos que hoje escondem a contagem de assistidos em avulso —
`Status.svelte:774,814` e `AnimeDetail.svelte:690` — passam a mostrá-la, e no `AnimeDetail` o
número vira input que salva pelo `PUT` acima. Cada linha de episódio ganha "Assisti até aqui" no
`⋯`, que grava aquele número como progresso: é o gesto natural de quem acabou de ver o episódio, e
o campo do cabeçalho é o que mostra e corrige o valor.

Consequência a documentar na tela: marcar progresso alto apaga o que ficou para trás, exatamente
como no caso AniList (respeitando `watched_episodes_to_keep` fora de pack, e o pack inteiro dentro
dele). Com `delete_watched_episodes` desligado nada é apagado e nenhum pack novo vem — também
igual ao caso AniList.

### 6. Uma requisição em vez de duas

`ScrapNyaaForBatch` (`nyaa.go:705`) e `ScrapNyaaForMultipleEpisodes` (`nyaa.go:552`) montam a
**mesma** URL — `?f=0&c=1_2&q=<título>&s=seeders&o=desc` — e discordam só no filtro de linha: uma
mantém apenas `isBatch(name)`, a outra apenas descarta. Duas descidas de até `max_search_pages`
sobre o mesmo HTML, por anime, por ciclo.

O saneamento da query também já é o mesmo: `reSeasonNamePatterns` (batch) e
`reSeasonStrip` + `rePartStrip` (multi) removem o conjunto idêntico — `Season N`, `S N`,
`Nª Season`, `Cour N`, `Part N` — os dois com `ReplaceAllString`. Unificar não muda a query em
caractere nenhum.

As duas viram `ScrapNyaaForAnime(animeName string, episodes []int, season, part *int)`, que devolve
**uma** lista: linha de pack entra com `IsBatch: true` (como `ScrapNyaaForBatch` já faz), linha de
episódio entra com `Episode != nil`. Quem particiona é o daemon, com um loop de duas linhas — e é
lá que precisa ser particionado de qualquer jeito, porque os tetos de tamanho são diferentes
(`MaxBatchTorrentSizeGB` para pack, `MaxEpisodeTorrentSizeGB` para episódio) e cada lista recebe o
seu antes de `pickBatches` / do map por episódio.

Devolver uma lista só é o que mantém `searchNyaaWithVariants` intocada: ela para na primeira
variante de título com resultado, e "resultado" continua sendo uma fatia não-vazia. Uma assinatura
de par exigiria generalizá-la ou duplicá-la.

`SortTorrentResults` roda sobre a lista mista; particionar depois preserva a ordem relativa, então
cada lista sai ordenada corretamente. `nyaaSearcher` troca os campos `searchBatch` e
`searchMultiple` por um `searchAnime`.

O piso de paginação (`enoughCandidates`, hoje 3) passa a contar as duas listas somadas. Fica um
`ponytail:` marcando o teto: uma página 1 com 3 packs que o filtro de tamanho depois descarta
encerra a descida sem ter juntado episódio solto nenhum, onde hoje a segunda busca desceria por
conta própria. O caminho de saída, se aparecer, é empurrar os filtros para dentro do contador — o
que exige o `nyaa` conhecer a config, que é justamente o que `applyNyaaSettings` evita.

### 7. Pack é uma linha no `AnimeDetail`

Com pack virando comum, One Piece renderiza 1122 linhas em que 100 delas oferecem "Excluir" e
"Rebaixar" que não fazem o que dizem: excluir um episódio de dentro de um pack não libera byte
nenhum (o hardlink da biblioteca e a cópia de seed são o mesmo inode, e o torrent fica), e
"Rebaixar" hoje é bug — `handleRedownloadEpisode` remove só aquele registro e depois chama
`ManualDownloadEpisode`, que busca **episódio solto**: em One Piece o registro some, a busca não
acha nada, volta 500, e o arquivo fica no disco órfão de registro.

A derivação de `rows` (`AnimeDetail.svelte:248`) passa a agrupar por `episode_hash` quando o hash
aparece em ≥2 episódios — a mesma regra que o backend já usa em `buildTorrentResponse:144`
("mais de um episódio no mesmo info hash é batch, diga o que disser a flag"). O grupo vira UMA
linha `kind: "batch"`, rotulada `Episódios <min>–<max>`, com o mesmo chip e a mesma barra de
progresso das outras (o join por hash de `indexTorrentsByEpisode` já entrega o torrent).

Ações da linha de grupo:

- **Excluir** → `DELETE /torrents/{hash}`, que já existe, já remove o torrent e todos os registros
  do grupo como unidade, e já tem o `TorrentDeleteDialog` com `keep_data`/`block`.
- **Substituir por magnet** → `POST /animes/{id}/replace`, que já existe e já é a operação de
  "trocar o pack inteiro".

Não há "Rebaixar" na linha de grupo: para um pack ele seria alias de Excluir, já que o loop
rebusca no ciclo seguinte assim que o espaço volta. Frontend puro — nenhum endpoint, campo de
resposta ou handler novo. As linhas de episódio solto não mudam em nada.

## Config

| Campo | Antes | Depois |
|---|---|---|
| `max_batch_episodes` | `int`, default `30` | **removido** de `Config`, de `getDefaultConfig`, da validação em `endpoint_config.go:129`, de `client.ts`, de `Config.svelte` e das mensagens `config_label/hint/val_max_batch_episodes` (en + pt-BR) |
| `max_batch_torrent_size_gb` | `float64`, default `0` (desligado) | default `100` |
| `max_episodes_per_anime` | `int`, default `12`, **rejeita `0`** | `0` passa a valer "sem teto"; sai a validação de `endpoint_config.go:109` e a de `Config.svelte:245`; o hint (en + pt-BR) passa a dizer isso |

`max_episodes_per_anime` era o único teto do projeto que proibia zero: `max_batch_torrent_size_gb`,
`min_seeders`, `watched_episodes_to_keep` e o próprio `max_batch_episodes` todos tratam `<= 0` como
desligado. A mudança alinha o campo com a casa, e `effectiveMax` (item 2) é a única linha nova que
ela custa.

`AnimeSettings` (por anime, não é `Config`) ganha `progress int`. Ausente lê `0`, que é o
comportamento de hoje.

Sem migração: `encoding/json` ignora campo desconhecido, então um `config.json` que ainda tem
`max_batch_episodes` continua carregando. O default novo de `max_batch_torrent_size_gb` só vale
para instalação nova — quem já tem `0` gravado segue com o filtro desligado e precisa ajustar à
mão. É o comportamento correto (`0` gravado é uma escolha explícita de "sem teto"), mas vale a
nota de release.

`100` é arbitrário como qualquer número aqui. Ele cabe pack completo de série de temporada em
1080p e não cabe pack completo de One Piece — para série longa o que passa é pack parcial, que é
exatamente o que o item 3 trata.

## Testes

Go, em `daemon`:

- `selectEpisodes` com limite `12` e 50 episódios salvos não-batch → 38 em `keysToDelete`. É o
  bug que o item 2 evita: hoje, com o limite levantado por palpite, esse conjunto vem vazio.
- `selectEpisodes` chamado duas vezes com os mesmos argumentos devolve o mesmo resultado (pureza:
  o contador de `downloadedEpisodes` não vaza entre chamadas).
- `effectiveMax` com `max_episodes_per_anime = 0` → `len(episodes)+1`; com `12` → `12`.
- Elegibilidade: `RELEASING` de 1100 episódios **busca** batch (hoje não busca); filme não busca;
  `Media.Episodes == nil` busca; anime com 1 episódio pendente não busca.
- `pickBatches`, um pack: entre `[X] Anime 001-100` e `[X] Anime 101-200`, com primeiro pendente
  `150` e limite `12`, escolhe só o segundo; com primeiro pendente `1`, só o primeiro; sem nenhum
  que cubra, devolve vazio e o fluxo cai em multi-episódio.
- `pickBatches`, janela cruzando a borda: primeiro pendente `95`, limite `12`, packs `001-100` e
  `101-200` → devolve os DOIS, nessa ordem.
- `pickBatches`, teto desligado: primeiro pendente `1`, limite `0`, packs `1-100`/`101-200`/
  `201-300` → devolve os três.
- `pickBatches` respeita a ordem de qualidade: dois packs que cobrem a mesma faixa → vence o que
  vem antes no resultado ordenado.
- `pickBatches` termina: lista com um pack `50-60` e cursor `55` não entra em laço nem repete o
  mesmo pack.
- Pack sem faixa no nome (`[X] Anime Complete Batch`) → um pack só, nenhum truncamento.
- Truncamento: pack `001-100` com pendentes `1..500` → `toDownload` fica com 100 itens e nenhuma
  notificação `ReasonNotFound` é emitida para `101..500`.
- Dois packs escolhidos → cada episódio recebe o magnet do pack da SUA faixa, e os `overrideName`
  são distintos.
- Filtro de tamanho esvazia o resultado de batch → `pickBatches` devolve vazio, cai em
  multi-episódio, e o limite por anime vale (o caminho que o tampão cobria, agora sem tampão).
- `handleAlreadySavedEpisode` com `IsBatch` e contador estourado → `(false, false)`; sem `IsBatch`
  → `(false, true)`. Dois ciclos seguidos sobre o mesmo estado não produzem deleção nenhuma
  (não-churn).
- `buildWatchedKeepSet` ignora registros `IsBatch`: pack de 100 todo assistido com
  `watched_episodes_to_keep = 3` → keep-set vazio, os 100 vão para `keysToDelete`, e
  `allEpisodesInDeleteSet` passa (é o teste que prova que o rodízio não trava).
- Rodízio completo: pack `1-100` salvo, progresso `100` → ciclo apaga o grupo inteiro e o
  seguinte escolhe `101-200`. Com progresso `50` → nada é apagado e nenhum pack novo é buscado.
- Avulso com progresso salvo `48` → `appendStandaloneAnimes` devolve o `MediaList` com
  `Progress: 48`, e `firstEpisodeToConsider` começa em `49`.
- Avulso sem `AnimeSettings` gravado → `Progress: 0`, comportamento de hoje.
- `RunAnimeDebug` reporta os mesmos episódios que `processAnimeEpisodes` selecionaria, agora por
  construção (mesma função).

Em `nyaa`:

- `ScrapNyaaForAnime` sobre uma página com pack e episódio devolve os dois na mesma lista, os
  packs com `IsBatch: true` e os episódios com `Episode != nil`; a partição no daemon reproduz
  exatamente o que as duas funções antigas devolviam para a mesma página.
- A query gerada por `ScrapNyaaForAnime` é idêntica, caractere a caractere, à que
  `ScrapNyaaForBatch` e `ScrapNyaaForMultipleEpisodes` geravam para o mesmo título (é a garantia
  de que a unificação do saneamento não regride).
- O piso de paginação conta as duas listas: página 1 com 2 packs e 1 episódio não desce para a 2.
- Em `src/tests/unit/nyaa_test.go`, `TestExtractBatchInfo_ExtractsRangeAndSeason` (hoje um
  `t.Skip`, justamente por a função não ser exportada) é reativado e ganha os casos da guarda de
  resolução: `"[X] Anime [720-1080p]"` → `EndEpisode == 0`; `"[X] Anime 01-12 [1080p]"` → `1..12`.

Em `api`:

- `PUT /animes/{id}/settings` só com `custom_search_query` preserva o `progress` salvo, e
  vice-versa (é o merge do item 5; sem ele o teste falha zerando o outro campo).
- `PUT` com `progress` inválido (negativo) → 400.

Os casos de `debug_test.go` que hoje montam `Config{MaxBatchEpisodes: 30}` perdem o campo; o par
de testes de `willBatchAnime` acima/abaixo do teto (`debug_test.go:146-162`) é substituído pelos
casos de elegibilidade acima.

Vitest / componentes:

- O teste de config perde `max_batch_episodes` e ganha o caso de `max_episodes_per_anime = 0`
  válido; `tests/smoke/helpers.ts:90` idem.
- `AnimeDetail`: 100 episódios com o mesmo `episode_hash` renderizam UMA linha rotulada
  `Episódios 1–100`; um episódio com hash próprio continua renderizando a sua; a linha de grupo
  não oferece "Rebaixar".
- `AnimeDetail`: anime avulso mostra a contagem de assistidos e o input de progresso; anime de
  lista mostra a contagem sem input.

## Docs a atualizar

- `config.md`: remover a linha de `max_batch_episodes`; atualizar o default e a descrição de
  `max_batch_torrent_size_gb` (guarda única, default `100`); tirar `max_batch_episodes` da lista de
  campos `>= 0` e mover `max_episodes_per_anime` para a lista dos que aceitam `0` como desligado;
  documentar `progress` em `AnimeSettings`.
- `architecture.md`: `willBatchAnime` sai da tabela de símbolos, entram `selectEpisodes`,
  `pickBatches`, `coveringBatch` e `ScrapNyaaForAnime` (com `ScrapNyaaForBatch` e
  `ScrapNyaaForMultipleEpisodes` saindo); a lista de estratégias de busca (linha 163) perde a
  Estratégia 2 e ganha a descrição dos packs resolvidos antes do loop.
- `decisions.md`: (a) por que elegibilidade a batch deixou de ser metadado e virou filtro de
  resultado — com o "don't fix by" de reintroduzir um teto de contagem de episódios do anime;
  (b) por que os packs precisam cobrir a janela a partir do primeiro episódio pendente, e por que
  os episódios fora da cobertura **não** caem em episódio solto; (c) por que episódio `IsBatch` é
  imune tanto à poda por limite quanto ao keep-set de assistidos, e que é isso que dá o rodízio de
  packs sem config nova; (d) por que progresso de avulso mora em `AnimeSettings` e é injetado no
  `MediaList` sintético em vez de virar um caminho paralelo. A entrada existente sobre
  `max_batch_episodes` (linhas 785-801) é reescrita apontando para esta spec.
- `testing.md`: o mock de `nyaaSearcher` perde dois campos e ganha um.
- `swag init -g src/cmd/daemon/main.go -o docs/swagger` — `Config` e `animeSettingsRequest` mudam.

## Tamanho

Go: `daemon/episodes.go` (o grosso), `daemon/debug.go`, `daemon/search.go`, `daemon/standalone.go`,
`nyaa/nyaa.go`, `nyaa/nyaa_regex.go`, `files/filemanager.go`, `api/endpoint_config.go`,
`api/endpoint_anime_settings.go`, `api/standalone.go`. Frontend: `client.ts`, `Config.svelte`,
`AnimeDetail.svelte`, `Status.svelte`, `episodeActions.ts`, `en.json`, `pt-BR.json`,
`tests/smoke/helpers.ts`. Nenhum arquivo novo, nenhuma dependência nova; o tráfego contra o
nyaa.si **cai**, porque a busca unificada substitui duas descidas de páginas por uma.

Saldo de linhas: saem `willBatchAnime`, `resolvedIsBatch`, o tampão, a Estratégia 2, uma das duas
cópias do `parseRow` de ~100 linhas, um campo de config com sua validação, seu input e suas quatro
mensagens de i18n. Entram `pickBatches`/`coveringBatch`, o campo de progresso com sua injeção em
três pontos, e o agrupamento de linhas do `AnimeDetail`.
