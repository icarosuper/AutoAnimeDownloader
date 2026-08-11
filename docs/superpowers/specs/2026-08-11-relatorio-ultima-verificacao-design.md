# Relatório da última verificação: por que o episódio não baixou

Data: 2026-08-11

## Problema

O daemon tem hoje seis maneiras distintas de não baixar um episódio, e **nenhuma delas chega ao
frontend**. Todas terminam em `logger.Logger.Debug()` ou `Warn()` — o usuário precisa abrir
`daemon.log` e saber o que procurar para descobrir por que um anime está parado.

Duas classes de falha, ambas invisíveis:

**A. O passe inteiro aborta.** `AnimeVerification` tem sete saídas antecipadas (config incompleta,
migração do `save_path` falhou, probe de hardlink falhou, backend não inicializado, `Ensure` falhou,
migração de ids falhou, AniList caiu). Todas chamam `state.SetLastCheckError(err)`, então o erro
*existe* em memória — e o `StatusResponse` o descarta: manda `has_error: bool` e joga a mensagem
fora. O frontend literalmente não tem como mostrar o texto.

**B. O episódio não baixou.** `processAnimeEpisodes` (`daemon/episodes.go:98-154`) decide isso em
seis pontos:

- nenhum torrent encontrado no Nyaa (`episodes.go:119`) — só `Warn` + webhook `ReasonNotFound`
- todos os candidatos acima de `max_*_torrent_size_gb` — `filterBySize` (`search.go:33`) poda a
  lista e loga em `Debug`; quem chama recebe uma slice vazia e **não tem como saber** se o Nyaa não
  devolveu nada ou se o filtro cortou tudo
- todos os candidatos abaixo de `min_seeders` — `filterBySeeders` (`search.go:65`), idem
- espaço em disco insuficiente (`episodes.go:147`) — webhook `ReasonNoDiskSpace`
- o cliente de torrent recusou todos os magnets — `attemptDownloadWithRetries` (`episodes.go:439`),
  webhook `ReasonDownloadRejected`
- `max_episodes_per_anime` atingido (`checkEpisode`, `episodes.go:342`) — só `Debug`

A taxonomia de motivos já existe parcialmente (`notifications.ReasonNotFound` /
`ReasonDownloadRejected` / `ReasonNoDiskSpace`, `notifications.go:26`), mas só alimenta webhooks.
Quem não configurou webhook não tem nada.

## Objetivo

1. O passe automático produz um **relatório** do que impediu downloads, agregado por anime.
2. A tela de Status mostra esse relatório: um resumo com os animes e episódios afetados.
3. A tela de detalhe do anime mostra o motivo do episódio dele.
4. A mensagem de erro de passe (classe A) deixa de ser descartada.

Não-objetivo: mudar qualquer regra de decisão do daemon. Nada aqui altera o que é baixado — só
torna visível o que já é decidido. Também não é histórico: o relatório é o do último passe, e ponto.

## Escopo: só o passe automático

Download manual (`ManualDownloadEpisode` e os endpoints de ação por episódio) fica fora. Aquele
caminho já devolve o erro na resposta HTTP do próprio botão que o usuário clicou; duplicar isso no
relatório criaria duas verdades sobre o mesmo evento, que discordariam na hora em que o passe
automático rodasse entre o clique e o poll.

## Taxonomia

Duas categorias, com peso visual diferente na UI. A distinção é: **problema** é algo que devia ter
baixado e não baixou; **limite** é a config funcionando como configurada, e que o usuário
frequentemente não relaciona com o sintoma ("por que só baixou 12 de 47?").

### Problemas

| Código | Detecção | Campos de detalhe |
|---|---|---|
| `all_above_size_limit` | `filterSearchResults` recebeu N>0 e devolveu 0, tendo cortado por tamanho | `candidates`, `limit_gb` |
| `no_seeders` | idem, tendo cortado pelo piso de seeders | `candidates`, `min_seeders` |
| `no_torrent_found` | `len(magnets) == 0` depois do fallback single-ep (`episodes.go:119`) | — |
| `disk_full` | `checkDiskSpace` bloqueou (`episodes.go:147`) | — |
| `torrent_rejected` | `attemptDownloadWithRetries` (`episodes.go:408`) esgotou os magnets, todo `backend.Add` falhou | `candidates` (magnets tentados) |

### Limites

| Código | Detecção | Campos de detalhe |
|---|---|---|
| `max_episodes_per_anime` | `checkEpisode` (`episodes.go:342`) | `downloaded`, `pending`, `batch_skipped` |

### Precedência é cascata, não conjunto

Os três primeiros problemas se sobrepõem: quando um filtro esvazia a lista, `len(magnets) == 0`
também é verdade. **O motivo específico tem que ganhar do genérico**, na ordem da tabela — senão
"todos os candidatos tinham 8 GB e seu teto é 3 GB" (acionável: subir o teto ou ajustar a query)
vira "nenhum torrent encontrado" (não acionável, e mentiroso: havia oito).

Mesma disciplina da cascata de `deriveAnimeChip` (`lib/domain/animeState.ts`): a primeira condição
que casa vence, e a ordem é a regra de negócio.

### Skips normais ficam fora

`shouldSkipEpisode` descarta episódio em lista excluída, já assistido e ainda não lançado. Um anime
em dia gera dezenas dessas por passe. Se entrassem no relatório, os três problemas reais se
perderiam numa lista de duzentas linhas de ruído — que é exatamente a situação do `daemon.log` hoje,
só com CSS melhor. Episódio bloqueado pelo usuário também fica fora: ele já aparece marcado na tela
de detalhe do anime.

### "Batch com episódios demais" não é um problema, é a causa de um limite

Quando `willBatchAnime` devolve false porque `*Media.Episodes > MaxBatchEpisodes`, **nada é
bloqueado** — só a Estratégia 2 é desligada, e o download segue um-a-um com
`max_episodes_per_anime` valendo. O sintoma que o usuário sente é sempre
`max_episodes_per_anime`; o batch desligado é o *porquê* de aquele limite estar valendo naquele
anime.

Por isso é campo de detalhe, não código próprio: `batch_skipped` assume
`"too_many_episodes"` (teto de `max_batch_episodes`), `"above_size_limit"` (o batch foi encontrado
mas `filterBySize` o cortou, e `episodes.go:87` reaplicou o limite), `"no_result"` (busca de batch
voltou vazia) ou `""` (o anime nunca foi elegível a batch). Uma linha no relatório com a explicação
inteira, em vez de duas que o usuário tem que juntar de cabeça.

## Coleta: três assinaturas, nenhum coletor global

A tentação aqui é um `*passReport` passado por parâmetro para todo mundo alimentar. Isso vira
estado compartilhado entre as goroutines de `maxConcurrentAnimes` e precisa de mutex. O fluxo atual
já resolve isso: cada anime roda numa goroutine e devolve um `animeProcessResult` por canal.

1. **`filterSearchResults(results, maxGB, minSeeders) → ([]nyaa.TorrentResult, dropStats)`**, com
   `dropStats{Input, BySize, BySeeders int}`. `filterBySize` e `filterBySeeders` devolvem seus
   contadores; a informação que hoje só existe dentro do `logger.Debug` deles passa a ser retorno.
   Quem chama decide o código: sobrou 0, entrou N>0 e `BySize > 0` → `all_above_size_limit`.

2. **`checkEpisode(...) → (shouldDownload, shouldDelete bool, skipCode string)`**. A condição do
   limite (`*downloadedEpisodes >= maxEpisodes`) fica onde é decidida. O chamador *tem* acesso ao
   ponteiro `downloadedEpisodes` e poderia reproduzir a comparação — e aí a regra existiria em dois
   lugares para discordar depois. `skipCode` é `""` para todo skip que não entra no relatório.

3. **`animeProcessResult` ganha `issues []Issue`**. Já é o canal por onde cada anime devolve
   resultado (`verification.go:281-285`); a agregação entra no mesmo loop que já junta
   `newEpisodes`, `checkedEpisodes` e `keysToDelete`.

Call sites afetados: 17, em `search.go`, `episodes.go`, `debug.go` (produção) e `episodes_test.go`,
`limits_test.go` (teste).

## Armazenamento: memória, e `SetLastCheckError` limpa

`State` ganha `lastCheckReport *CheckReport`, com `SetLastCheckReport` / `GetLastCheckReport` no
mesmo padrão dos outros campos (mutex, snapshot sob lock).

**Nada em disco.** `createStartFunc` chama `AnimeVerification` imediatamente ao iniciar
(`loop.go:53`, antes do primeiro `time.After`), então após um restart o relatório se reconstrói em
segundos. Um arquivo novo custaria persistência, migração e a possibilidade de mostrar um relatório
obsoleto de dias atrás como se fosse do último passe.

**`SetLastCheckError` limpa o relatório.** Todas as sete saídas antecipadas já chamam essa função;
com a limpeza dentro dela, nenhuma delas precisa de linha nova. E a semântica é a correta: um passe
que abortou antes de olhar anime nenhum não tem relatório por anime — tem `pass_error`. Sem isso, a
tela mostraria os problemas do passe anterior lado a lado com um erro de passe novo, sugerindo que
os dois vieram da mesma verificação.

Consequências dessa escolha, ambas desejadas:

- `SetLastCheckError(nil)` no fim do passe (`verification.go:306`) também limpa, então
  `SetLastCheckReport(report)` tem de vir **depois** dela.
- O cancelamento (`verification.go:290`) chama `SetLastCheckError(nil)` e retorna: passe
  interrompido não deixa relatório, que é o certo — ele estava incompleto.

## API

`GET /api/v1/last-check` → `handleLastCheck` em `api/endpoint_last_check.go`. GET-only, 405 no
resto, `JSONSuccess` — o padrão dos outros handlers.

```go
type Issue struct {
	AnimeID   int    `json:"anime_id"`
	AnimeName string `json:"anime_name"`
	Episodes  []int  `json:"episodes,omitempty"`
	Code      string `json:"code"`
	// detalhes por código, todos omitempty
	Candidates   int     `json:"candidates,omitempty"`
	LimitGB      float64 `json:"limit_gb,omitempty"`
	MinSeeders   int     `json:"min_seeders,omitempty"`
	Downloaded   int     `json:"downloaded,omitempty"`
	Pending      int     `json:"pending,omitempty"`
	BatchSkipped string  `json:"batch_skipped,omitempty"`
}

type CheckReport struct {
	FinishedAt time.Time `json:"finished_at"`
	PassError  string    `json:"pass_error"`
	Problems   []Issue   `json:"problems"`
	Limits     []Issue   `json:"limits"`
}
```

Campos achatados com `omitempty`, não um `detail map[string]any`: um mapa livre não gera Swagger
utilizável nem tipo TS decente, e o preço é uma struct de dez campos com três preenchidos por vez.

**Um endpoint, dois consumidores.** O Status renderiza tudo; o detalhe do anime chama o mesmo
endpoint e filtra por `anime_id`. A alternativa (contador em `StatusResponse` + campo `issue` em
`AnimeEpisodeInfo` + este endpoint) espalharia a mesma informação por três formas diferentes que
precisariam concordar.

**Agregação:** um `Issue` por par (anime, código). Bleach com ep 12 cortado pelo teto e ep 15 sem
torrent = duas entradas. Ordenado por `AnimeName` dentro de cada categoria — estável e previsível,
sem inventar um ranking de severidade que ninguém pediu.

**`Episodes` só é preenchido em problema.** Um problema acontece *num episódio*: ele foi buscado, os
candidatos foram avaliados, e aquele episódio específico não baixou. `max_episodes_per_anime` é o
contrário — o daemon parou de considerar episódios ao atingir a conta, então não existe "os
episódios afetados", existe uma quantidade que sobrou: `downloaded` e `pending`. Listar números de
episódio ali obrigaria a inventar quais seriam os próximos da fila, informação que a decisão não
produz. Por isso o campo é `omitempty`: nas entradas de `limits` ele simplesmente não vem.

**`pass_error`** é `GetLastCheckError().Error()`, ou `""`. `StatusResponse.HasError` continua
existindo e intocado — nenhum consumidor atual quebra.

## Frontend

**Estado vazio é a regra.** Passe limpo devolve arrays vazios e a UI **não renderiza nada**;
`finished_at` zero (daemon nunca rodou) idem. Nenhum card permanente anunciando "0 problemas" — um
elemento que está sempre ali deixa de ser lido, e aí o dia em que ele tem conteúdo passa batido.

1. **`lib/api/client.ts`** — `getLastCheck()` e os tipos `Issue` / `CheckReport`.

2. **`routes/Status.svelte`** — o alerta de `has_error` (hoje `status_error_alert`, um texto
   genérico) passa a mostrar `pass_error`, na mesma posição. Abaixo dele, um card *full-width* acima
   da lista de animes, renderizado só quando há entradas: seção "Problemas (N)" e seção "Limites
   aplicados (M)", cada linha nomeando o anime e os episódios afetados. Busca junto do poll de
   torrents que a tela já mantém (`TORRENTS_POLL_MS`).

3. **`routes/AnimeDetail.svelte`** — mesma chamada, filtrada por `anime_id`: um aviso no topo e uma
   marca nas linhas dos episódios listados em `Issue.Episodes`.

`deriveAnimeChip` não é tocado. Aquela cascata decide estado a partir de torrent **ao vivo**
(`peers_total`, `progress`, `status`); enfiar estado de relatório nela misturaria duas fontes com
frequências de atualização diferentes num único chip, e o `noSeeds` do `stallTracker` já cobre o
caso ao vivo que mais se parece com um problema de relatório.

**i18n:** uma mensagem por código em `messages/en.json` e `messages/pt-BR.json`, com os números
interpolados (`candidates`, `limit_gb`, `min_seeders`, `downloaded`, `pending`) e uma variante por
valor de `batch_skipped`. O backend manda código + números; o frontend monta a frase. É o mesmo
boundary que `lib/domain/animeState.ts` documenta e defende — uma string pronta vinda do Go
obrigaria o daemon a saber o locale do navegador.

## Testes

- `filterSearchResults`: `dropStats` correto para lista vazia na entrada, corte só por tamanho, só
  por seeders, e pelos dois.
- Precedência da cascata: com candidatos cortados pelo teto, o código é `all_above_size_limit` e
  **não** `no_torrent_found`.
- `checkEpisode`: `skipCode == "max_episodes_per_anime"` no limite, `""` em todo skip normal.
- `SetLastCheckError` limpa o relatório; `SetLastCheckReport` depois de `SetLastCheckError(nil)`
  sobrevive.
- Agregação: dois episódios do mesmo anime com o mesmo código viram um `Issue` com dois números em
  `Episodes`; códigos diferentes viram dois `Issue`s.
- `handleLastCheck`: 200 com relatório, 200 com relatório vazio, 405 em POST.

## Fechamento

- `go test ./...`
- `swag init -g src/cmd/daemon/main.go -o docs/swagger` (a API mudou)
- `docs/agents/architecture.md`: endpoint novo, símbolos novos em `state.go` e `search.go`, mudança
  de assinatura de `checkEpisode` e `filterSearchResults`
- `docs/agents/decisions.md`: a cascata de precedência dos códigos e o "`SetLastCheckError` limpa o
  relatório" — as duas coisas que alguém vai tentar "consertar" depois sem saber por que estão assim
