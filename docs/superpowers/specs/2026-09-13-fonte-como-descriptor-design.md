# A fonte de torrent vira descriptor externo — tirar o Nyaa do repositório

**Data:** 2026-09-13
**Status:** desenhado, não executado
**Origem:** "tirar a dependência hard do nyaa do código pra ficar mais seguro de publicar nos
lugares. creio que nesse caso faria um sistema de plugin tipo o app de ler mangá mihon"
**Escopo previsto:** `src/internal/nyaa` (quebra em dois pacotes), `src/internal/daemon/search.go`
(`nyaaSearcher` passa a carregar a fonte), `src/internal/files` (carregar o descriptor da pasta de
configs), `src/tests/mocks` e `src/tests/unit/nyaa_test.go` (fixtures deixam de citar `nyaa.si`).

---

## O objetivo é publicabilidade, não múltiplas fontes

Esta spec **não** é a continuação da [Etapa 0](2026-08-16-fontes-multiplas-etapa0-design.md). Aquela
pergunta se existe swarm alternativo para anime velho, continua sem resposta, e nada aqui a
responde. Trocar de indexador continua não ressuscitando swarm.

O que esta spec quer é uma coisa só: **o repositório e o binário deixarem de conter o endereço, a
query e o layout do Nyaa**. Que isso destrave uma segunda fonte é consequência, não motivo — e por
isso o desenho abaixo se recusa a construir agregação, merge de resultados ou catálogo.

**O que ela não compra:** blindagem jurídica. O app continua sendo "baixa anime por torrent
automaticamente". O que sai é o artefato mais direto — o código que raspa um site nomeado. Precedente
útil: o repo do Tachiyomi sobreviveu, o `tachiyomi-extensions` é que levou o takedown e teve de sair
da org (virou keiyoushi), e mesmo assim o app tomou C&D depois. Vale desenhar sabendo disso.

## Plugin compilado está fora

O pacote `plugin` da stdlib é Linux-only, exige que app e plugin sejam compilados com o **mesmo**
compilador e as **mesmas** versões de todas as dependências em comum, e não funciona com
`CGO_ENABLED=0`. O projeto tem tray em Windows (`internal/tray`). Um ABI de plugin em Go seria uma
matriz de build por plataforma e por release, para hospedar ~150 linhas de CSS selector.

Runtime de script embutido (goja, starlark, yaegi, wasm) resolveria, ao custo de uma dependência
nova e de um sandbox para código de terceiro. Desproporcional: as três fontes já medidas em
[`sources.md`](../../agents/sources.md) são todas *"GET uma URL → linhas"*. Isso é dado, não código.

**Decisão: descriptor declarativo em JSON, sem execução de código.**

## A fronteira já está medida

O pacote `internal/nyaa` tem 2217 linhas. O que é específico do Nyaa:

| o que | onde |
|---|---|
| URL base e parâmetros da query (`f`, `c`, `s`, `o`, `p`) | `nyaa.go:56`, e repetido nas três `Scrap*` |
| posição das células — `cells.Eq(1/2/3/5/6)` | dentro dos três `parseRow` |
| seletor de linha `.torrent-list tbody tr` | `parsePagesWith` (`nyaa.go:423`) |
| formato de data `2006-01-02 15:04` | `parseNyaaDate` (`nyaa.go:1048`) |
| lista de arquivos do detalhe — `div.torrent-file-list`, `span.file-size` | `PackFileRange` (`nyaa.go:224`), `fileListEntryName` |
| montagem do `/view/<id>` | `detailURL` (`nyaa.go:277`) |

São ~150 linhas. **Todo o resto — ~2050 — não é Nyaa:** `nyaa_match.go` inteiro (jaccard de título),
`nyaa_regex.go` inteiro, `priorities.go` inteiro (`ShouldIgnore`, sort por critério), e os
`Extract*` / `parseSize` / `ParseSeeders` / `SortTorrentResults` do `nyaa.go`. Isso é parsing de
**nome de release**, que segue a mesma convenção de cena em qualquer fonte.

Prova empírica de que a fronteira cai exatamente no `TorrentResult`: o `sources.md` registra que o
JSON do AnimeTosho "mapeia quase 1:1 em `nyaa.TorrentResult`".

> **Por isso o plugin NÃO inclui o tratamento.** O Mihon é má analogia aqui: site de mangá não tem
> convenção de nome, então a extensão precisa parsear. Release de anime tem —
> `[Grupo] Nome - 01 (1080p) [HEVC]` é a mesma coisa em toda fonte. Mandar o tratamento para o
> descriptor duplicaria 2050 linhas por fonte e jogaria fora os testes que já existem sobre elas.

## O descriptor

```jsonc
{
  "name": "exemplo",
  "search": {
    "url": "https://exemplo.tld/?q={query}&s=seeders&o=desc&p={page}",
    "kind": "html",
    "row": ".torrent-list tbody tr",
    "fields": {
      "name":     "td:eq(1) a:not(.comments)",
      "magnet":   "td:eq(2) a:eq(1)@href",
      "detail":   "td:eq(1) a:not(.comments)@href",
      "size":     "td:eq(3)",
      "seeders":  "td:eq(5)",
      "leechers": "td:eq(6)",
      "date":     "td:eq(4)"
    },
    "dateFormat": "2006-01-02 15:04"
  },
  "detail": { "kind": "html", "fileList": "div.torrent-file-list li", "fileSize": "span.file-size" }
}
```

`{query}` e `{page}` são os dois únicos placeholders. A **montagem da query** (`extractSeasonFromName`,
`episodeQueries`, as variantes de título de `GenerateSearchTitleVariants`) continua no app: é texto de
busca, não é específico de fonte. O teto de páginas continua sendo `max_search_pages` do
`config.json`, não campo do descriptor.

`kind` aceita `html` (goquery, já instalado), `json` (`encoding/json`) e `rss` (`encoding/xml`).
Nenhuma dependência nova. Para `json`/`rss`, `fields` carrega caminho de campo em vez de seletor CSS.

`detail` é opcional. Fonte sem ele desliga o `packRange` — pack sem faixa no nome grava faixa zero,
que é exatamente o fallback que [decisions.md #84](../../agents/decisions.md#84-a-cobertura-de-um-pack-sem-faixa-no-nome-vem-da-lista-de-arquivos-e-não-da-suposição-de-que-ele-cobre-tudo)
já definiu.

### O contrato que a fonte precisa cumprir — e quem reprova

Declarar isso no formato é o que impede o descriptor de virar uma linguagem de programação:

1. **Ordenar por seeders decrescente.** A descida de páginas de
   [decisions.md #57](../../agents/decisions.md#57-paginação-adaptativa-sequencial-com-piso-de-candidatos-sem-orçamento-por-anime)
   para no piso de candidatos porque confia que o topo é o melhor.
2. **Paginar de verdade.** `{page}` tem que devolver página diferente.
3. **Expor seeders por linha.** `filterBySeeders` e `min_seeders` não têm o que filtrar sem isso.

Medido em `sources.md`, isso já reprova: o RSS do Nyaa (ignora `s=` e `p=`), o JSON do AnimeTosho
(`order=seeders-d` silenciosamente ignorado, `seeders` em cache de ~22 dias de mediana) e o
TokyoTosho (RSS sem tag de seeders). **Nenhuma escape hatch para eles até que uma fonte real e
conforme apareça pedindo** — inclusive o magnet em base32 do TokyoTosho, que não se normaliza aqui.

## A refatoração real: `parseRow` se parte em três

O trabalho não é o descriptor, é que hoje a extração do DOM e a classificação estão no mesmo
`parseRow` (`nyaa.go:714` é o caso completo). Ele faz, na mesma função: lê célula por posição,
chama `ShouldIgnore`, casa título, extrai season/part/episode, aplica os portões de season/part,
aplica `batchTooBig`, e acumula. Só a primeira dessas coisas é do Nyaa.

Quebra proposta:

```
extract(linha) → RawRow{Name, Magnet, DetailURL, Seeders, Leechers, Size, Date}   ← descriptor
classify(RawRow) → TorrentResult                                                  ← genérico
accept(TorrentResult, critérios) → bool                                           ← genérico
```

O laço de páginas (`fetchSearchPages`) fica no lado genérico, porque o `accepted()` que ele conta
depende do `accept` — só o fetch de uma página e o `extract` de uma linha atravessam para a fonte.

**Sem interface.** Vai existir uma implementação só (a dirigida por descriptor); um `Source` struct
basta. Os testes não precisam de fake: apontar o descriptor para um `httptest.Server` é mais barato
que o `MockNyaaHttpGet` de hoje, e deixa de existir `getNyaaBaseURL`/`NYAA_URL`.

### Pacotes depois da quebra

| pacote | conteúdo |
|---|---|
| `internal/release` | `nyaa_match.go`, `nyaa_regex.go`, `priorities.go`, todos os `Extract*`, `parseSize`, `ParseSeeders`, `SortTorrentResults`, `TorrentResult` |
| `internal/source` | descriptor, fetch, `extract`, paginação, lista de arquivos do detalhe |

Custo mecânico medido: 21 arquivos importam `internal/nyaa`, com 277 ocorrências de `nyaa.`.
É o grosso do diff, e é busca-e-substitui. Efeito colateral bom: `ScrapNyaaForAnime`,
`searchNyaaForMovie` e companhia param de mentir no nome.

O `nyaaSearcher` de
[decisions.md #14](../../agents/decisions.md#14-nyaasearcher--dependency-injection-for-nyaa-search-in-processanimeepisodes)
sobrevive quase intacto — os quatro campos já são os quatro certos, incluindo o `packRange`. Muda o
nome e o `defaultNyaaSearcher()` passa a receber a fonte carregada em vez de fechar sobre funções de
pacote.

## Onde o descriptor mora

- **No repo do app: nenhum.** Nem de exemplo com host real. `docs/` descreve o formato; um
  `exemplo.json` apontando para `localhost` serve de referência.
- **Em disco:** ao lado do `config.json` (`files.configFileName`, `filemanager.go:17`), em
  `sources/*.json`. Sem fonte configurada, o daemon **sobe normalmente** e a busca vira no-op com
  mensagem clara na UI — a tela de animes, o histórico e os torrents em andamento continuam
  funcionando. Não é erro fatal.
- **Instalação:** o usuário põe o arquivo lá. Sem índice oficial, sem catálogo embutido, sem botão
  "instalar do repositório da comunidade". O catálogo é justamente a peça que te dá dono — foi o que
  aconteceu com o `tachiyomi-extensions`.
- **Comunidade:** o formato documentado basta para alguém manter o próprio índice. Você não hospeda
  e não linka.

## Testes

`src/tests/mocks/` já tem mock de Nyaa e o `docker-compose.test.yml` já injeta `NYAA_URL`
(`testing.md:229`). Vira: descriptor de teste apontando para o mock. Os quatro literais `nyaa.si`
em `src/tests/unit/nyaa_test.go:1179-1238` viram host de fixture.

O teste ao vivo `nyaa/live_pack_measure_test.go` (gate `AAD_LIVE_NYAA=1`) é o único lugar que
**precisa** de um host real. Ou sai do repo, ou passa a ler o host de env sem default.

## Ordem de execução

1. **Quebrar `internal/nyaa` em `release` + `source`, com o Nyaa ainda hardcoded.** Diff grande,
   comportamento idêntico, `go test ./...` verde é o critério. Nada de descriptor ainda.
2. **`extract`/`classify`/`accept` separados** dentro de `source`, ainda hardcoded.
3. **Descriptor JSON dirigindo o `extract`**, com o `nyaa.json` gerado a partir do que estava
   hardcoded — e usado só como fixture de teste local.
4. **Tirar o hardcoded**, carregar de `sources/*.json`, tratar ausência como no-op com aviso.
5. **Limpar repo e docs**: fixtures, `NYAA_URL`, `environment.md:17`, `sources.md`, `testing.md`.

Cada etapa é commitável sozinha. Se parar na 2, o projeto já melhorou; se parar na 4, o objetivo
está cumprido.

## Fora do escopo

- **Múltiplas fontes ao mesmo tempo**, merge e deduplicação entre fontes. Uma fonte ativa por vez.
  Reabrir só quando a Etapa 0 confirmar que existe swarm alternativo.
- **`kind: json` e `kind: rss`** podem ficar para depois da etapa 4 — o Nyaa é `html`, e nenhuma
  fonte conforme ao contrato acima está identificada hoje. Desenhados aqui para que o formato não
  precise mudar quando chegarem.
- **Normalização de magnet base32, seeders ausente, ordenação client-side.** Tudo isso é escape
  hatch para fonte que não cumpre o contrato. Não construir preventivamente.
- **UI de gerenciamento de fontes.** Arquivo em disco basta; tela só depois de existir mais de uma.

## Docs a atualizar quando executar

`environment.md` (sai `NYAA_URL`), `config.md` (pasta `sources/`), `architecture.md` (os dois
pacotes novos, e o fato negativo "não existe `internal/nyaa`"), `testing.md` (mock por descriptor),
`sources.md` (passa a descrever o contrato, não a fonte), e entrada nova em `decisions.md` para
"por que o tratamento não vai no descriptor" e "por que não há interface `Source`".
