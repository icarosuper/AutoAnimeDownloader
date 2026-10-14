# Lista de arquivos do torrent nas telas de Downloads e do anime

**Data:** 2026-08-28
**Status:** desenhado
**Origem:** "quero ver os arquivos que o torrent traz — em qual formato/codec os episódios vieram"
**Escopo:** `internal/torrents` (método novo no backend + `FileCount` em `TorrentInfo`),
`internal/api` (endpoint novo), `frontend` (`TorrentFiles.svelte`, `Downloads.svelte`,
`AnimeDetail.svelte`). Muda a API → `swag init` obrigatório. Sem campo novo em `config.json`.

---

## O caminho até aqui

A pergunta que abriu a investigação foi "a rain não consegue ver os arquivos do torrent, certo?
Então o jeito seria scrapar a página do torrent no Nyaa". As duas metades estavam erradas, e
descartá-las é metade do desenho.

**A rain vê.** `Torrent.Files()` (`torrent.go:421`) devolve `path` e `length` de cada arquivo, já
excluindo padding files, e funciona sempre que o metadata existe — inclusive com o torrent pausado,
porque `Stop()` libera `t.pieces` e não `t.info`. `Torrent.FileStats()` (`torrent.go:437`) devolve o
mesmo mais `BytesCompleted` **por arquivo**, e essa exige o torrent rodando (`len(t.pieces) == 0` →
`"torrent not running so file stats unavailable"`).

**Scrapar o Nyaa seria pior em toda dimensão.** Custa uma requisição HTTP por torrent, num site que
rate-limita, para obter um dado que já está em memória. E o dado de lá é o que o uploader
escreveu; o da rain é o que o swarm está entregando.

O único cenário em que a página do Nyaa (ou o `?show=torrent` do AnimeTosho) ganha é ver os arquivos
**antes de adicionar o torrent** — filtro pré-download, que é o item 4 da ordem recomendada de
`docs/agents/sources.md` e está fora deste spec.

---

## Decisões, e o que foi descartado

**Nome de arquivo cru, sem mapear para "Ep NN" nas Downloads.** O motivo de olhar a lista é
descobrir em que formato o episódio veio — `[Judas]`, `1080p`, `x265`, `FLAC`, `10bit`. Rotular a
linha como "Ep 07" jogaria fora exatamente a informação procurada.

**Um endpoint, um componente, dois modos de render.** As duas telas querem recortes diferentes da
mesma lista, não listas diferentes: Downloads quer tudo (inclusive o lixo do pack), AnimeDetail quer
progresso por episódio. Dois endpoints ou dois componentes duplicariam fetch, loading e erro para
diferenciar um `filter` e um rótulo.

**O `episode` é calculado no backend, não no frontend.** `nyaa.ExtractEpisodeNumber` já existe, já é
testada, e cobre `S01E05`, `- 05`, `EP05`, `Episode 05`, ` 05.mkv` (`nyaa_regex.go:59-76`).
Reimplementar essa cascata de dez regexes em TypeScript criaria uma segunda verdade para divergir da
primeira. O pacote `api` já importa `nyaa` (`endpoint_priorities.go:4`), então não há dependência
nova.

**Painel read-only. Não existe selecionar nem excluir arquivo de dentro do pack.** Medido no módulo:
o único `Priority` da `rain v2.3.1` é de *peer* (`internal/peerpriority`, `internal/addrlist`); não
há `SetWanted`, `FilePriority` nem skip por arquivo — `Files()` e `FileStats()` são read-only. Bate
com o que `docs/agents/sources.md` já registrava. Apagar o arquivo no disco também não serve: o
torrent segue seedando aquele piece e o próximo `Verify`/restart acha o arquivo faltando e rebaixa.
As saídas reais — fork da rain, trocar de cliente, ou debrid — são todas troca de arquitetura, não
feature de UI.

**Seta de expandir só com `file_count > 1`.** Torrent de episódio único tem um arquivo, cujo nome já
está na linha; um painel ali seria um clique para reler o que já estava na tela.

**Fetch sob demanda, nunca no poll.** A lista só é buscada ao expandir. Enquanto aberta, ela se
refaz no tick de poll que a tela já tem (2s, `Downloads.svelte:56`); ao fechar, para. Enfiar a lista
na resposta de `GET /torrents` multiplicaria por N o payload de um poll de 2 segundos para um dado
que quase sempre ninguém está olhando.

**Descartado: seção "outros arquivos" no AnimeDetail.** Foi oferecida e recusada — a tela do anime é
sobre episódio; lixo se vê nas Downloads. Consequência aceita conscientemente: arquivo que a
heurística não casar não aparece ali. O custo é limitado porque a mesma lista, completa, está a um
clique nas Downloads.

---

## Backend

### `internal/torrents`

```go
// FileInfo é um arquivo dentro de um torrent. Path é o caminho relativo CRU, como está no
// metadata — é o que o usuário lê para saber fansub, resolução e codec.
type FileInfo struct {
    Path string
    Size int64
    // BytesCompleted é nil quando o progresso por arquivo é desconhecido: com o torrent
    // parado a rain libera as pieces e FileStats() falha, enquanto Files() continua
    // respondendo. nil é "não sei", que a UI mostra como "—"; 0 seria a mentira "nada baixado".
    BytesCompleted *int64
}
```

Novo método na interface `TorrentBackend`:

```go
// Files lista os arquivos do torrent. Slice vazio (sem erro) quando o metadata ainda não
// chegou — um magnet recém-adicionado não tem lista, e isso é estado normal, não falha.
Files(hash string) ([]FileInfo, error)
```

- `Session.Files`: pega o torrent; tenta `FileStats()`; se falhar, cai para `Files()` com
  `BytesCompleted` nil; se `Files()` também falhar por metadata ausente, devolve `nil, nil`. O
  handler serializa slice nulo como `[]`, nunca `null` — a UI não deve precisar distinguir os dois.
- `SessionManager.Files`: delega sob `RLock`, `ErrSessionNotReady` quando não há sessão — mesma
  forma de `Announce` (`sessionmanager.go:362`).
- `FakeBackend.Files`: lê de um `map[string][]FileInfo` que os testes preenchem.

Campo novo em `TorrentInfo`:

```go
// FileCount é o número de arquivos do torrent, 0 até o metadata chegar. Sai do mesmo Stats()
// que preenche todo o resto (Stats.FileCount) — nenhuma chamada extra, o que respeita a regra
// do struct de uma Stats() por torrent por snapshot. Existe para a UI decidir se a linha
// ganha seta de expandir.
FileCount int
```

Ele entra também em `TorrentResponse` como `file_count`.

### `internal/api`

`GET /api/v1/torrents/{hash}/files`

```json
{ "success": true, "data": [
  { "path": "[Judas] Frieren - S01E03.mkv", "size": 1490000000,
    "bytes_completed": 780000000, "episode": 3 },
  { "path": "NCOP01.mkv", "size": 212000000, "bytes_completed": 212000000, "episode": null }
]}
```

Só GET (405 no resto), 404 quando o hash não está na sessão — a mesma checagem por
`server.Torrents.Get` que `torrentAction` já faz (`endpoint_torrents.go`). `episode` vem de
`nyaa.ExtractEpisodeNumber(path)`, `null` quando não casa. A ordem é a do metadata do torrent, não
reordenada: é a ordem em que o release foi montado.

---

## Frontend

### `TorrentFiles.svelte`

Props: `hash`, `mode: "raw" | "episodes"`. Dono do fetch, do loading e do erro. Falha de fetch
degrada em silêncio como o resto da tela já faz — o painel some, a linha continua.

- **`raw`** (Downloads): todos os arquivos, na ordem recebida, `path` cru + tamanho + percentual.
- **`episodes`** (AnimeDetail): só `episode != null`, rótulo `Ep NN`, ordenado por número.

Metadata ainda não chegou (lista vazia) → uma linha "aguardando metadata", não um painel vazio.

### Mapeamento episódio → arquivo (modo `episodes`)

Helper puro em `lib/utils/`, testável sem componente. Dois arquivos casando o mesmo número —
`NCOP 01.mkv` casa o padrão ` 05.mkv` e vira "Ep 01" — resolvem por **maior arquivo vence**. Sem
isso um NCOP de 200 MB rouba a barra do episódio 1.

### `Downloads.svelte`

Terceiro nível do accordion (grupo por anime → linha de torrent → arquivos), aberto pela seta na
linha, só quando `file_count > 1`.

### `AnimeDetail.svelte`

A linha `kind: "batch"` de `buildRows` (`AnimeDetail.svelte:307`) ganha a seta. Ela é a única com
lacuna: episódio com torrent próprio já mostra progresso individual hoje (`episodeMeta`, `:245`), e
os episódios cobertos por um pack são engolidos numa linha só de propósito — não há linha por
episódio dentro do pack para receber barra sem expandir.

Ganho lateral: a faixa que a linha exibe hoje vem de `BatchStart`/`BatchEnd`
(`filemanager.go:48-55`), lida do **nome** do torrent. O painel é a primeira vez que a tela mostra o
que o pack realmente contém.

---

## Testes

**Go** (`endpoint_torrents_test.go`, `FakeBackend`): lista normal com `episode` preenchido; hash
desconhecido → 404; método errado → 405; sessão inexistente → erro tratado, não pânico; torrent
parado → `bytes_completed` nulo no JSON; metadata ausente → `data: []` com 200.

**TS** (vitest, no helper de mapeamento): arquivo sem match sai da lista; dois arquivos no mesmo
número → fica o maior; ordenação por número, não pela ordem do torrent.

**Docs:** endpoint novo em `docs/agents/architecture.md` e `swag init -g src/cmd/daemon/main.go -o
docs/swagger`. Nada para `decisions.md`: o desenho não tem padrão incomum a defender.

---

## Riscos aceitos

**Pack numerado em absoluto sobre entrada de temporada.** Um `One Piece 575-782` sobre uma entrada
do AniList numerada 1..N não casa episódio nenhum: o painel do AnimeDetail fica vazio. O das
Downloads continua mostrando tudo, que é onde o dado está garantido.

**A heurística de episódio erra em nome exótico.** Consequência de "não mostra o que não casou": o
arquivo some daquele painel. Mitigação é a própria arquitetura — a lista completa e crua está nas
Downloads, sempre.

**Sem torrent na sessão, sem painel.** A lista vem da rain, não do registro salvo. Episódio antigo
cujo torrent foi removido não tem lista em tela nenhuma. Guardar a lista em `downloaded_episodes`
para sobreviver à remoção é maior que esta feature e não foi pedido.

---

## Fora de escopo

- Selecionar/excluir arquivos do pack (a rain não tem; ver Decisões).
- Ver arquivos **antes** de adicionar o torrent — é AnimeTosho `?show=torrent&id=`, item 4 de
  `docs/agents/sources.md`.
- Usar a lista real para corrigir `BatchStart`/`BatchEnd` nos registros salvos.
