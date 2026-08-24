# Filtro por idioma de legenda nas prioridades

**Data:** 2026-08-24
**Status:** desenhado, não implementado
**Fecha:** "Filtro por idioma de legenda nas prioridades (ex.: PT-BR)" do
[TODO-competitive.md](../../TODO-competitive.md)
**Escopo:** `internal/nyaa` (config, sinal, ordenação, categoria de busca),
`internal/daemon/search.go` (filtro duro), `internal/files` (campo de config),
`frontend/routes/Priorities.svelte` (UI).

---

## O problema, e por que a resposta óbvia está errada

A pergunta que abriu o desenho foi: *"teria que abrir as páginas dos torrents pra ler a
descrição ou então listar os arquivos do torrent pra descobrir isso?"*

As duas hipóteses foram medidas contra o Nyaa real em 2026-08-24. As duas estão erradas como
caminho principal, e a medição achou um terceiro bloqueio que nenhuma das duas via.

### Listar arquivos do torrent não serve

Legenda de anime é **softsub, muxada dentro do `.mkv`**. A view page de
`[Erai-raws] Mushoku Tensei III - 09 [MultiSub]` (id 2150451) tem `#### File list` com **um
arquivo**: o próprio `.mkv`. Nenhum `.ass` solto.

Isso vale também para o `files[]` do AnimeTosho descrito em [sources.md](../../agents/sources.md):
ele diz `filename`/`size`/`crc32`, e nenhum deles sabe idioma de legenda. A lista de arquivos
responde "quais episódios o pack cobre", não "que legenda tem dentro".

### A descrição serve, mas para poucos uploaders

A view page (`/view/<id>`, ~17 KB, 1 GET) traz um bloco livre. Erai-raws:

```
`Subtitles Info:` **English** (CR) ASS **Portuguese** (Brazilian_CR) ASS
**Spanish** (Latin_America_CR) ASS **Arabic** (CR) ASS ... **Polish** (CR) ASS
```

ToonsHub usa outro formato (`Subtitles:** English, Arabic, ...`). SubsPlease não publica nada.
Os grupos PT-BR nativos também não — eles põem no título.

**Não dá para buscar por isso.** `?q=Brazilian_CR&c=1_2` volta **0 resultados**: o Nyaa não
indexa a descrição. A view page só se alcança um id por vez.

### O bloqueio real: a categoria está fixa em inglês

`nyaa.go:461`, `nyaa.go:624` e `nyaa.go:765` fixam `c=1_2` = *Anime - English-translated*.

Todo release PT-BR **nativo** mora em `c=1_3` = *Anime - Non-English-translated*. **Hoje o
daemon não enxerga nenhum deles.** Nenhuma quantidade de leitura de descrição resolve isso,
porque esses torrents nem chegam a aparecer na busca.

E em `c=1_3` o marcador vem no título, de graça:

```
[WF] BLEACH: Sennen Kessen-hen - 44 [WEB-DL AMZN 1080p H.264 DDP][PT-BR]
[Pharaoh] Yu-Gi-Oh! 1998 - 06 [1080p][HEVC x265 10bits][PT-BR]
[AT] Paranoia Agent (Mousou Dairinin) [1080p][PT_br][Legendado][AnimesTorrent.com]
[Kanshin] Card Captor Sakura 720p (PT-BR) (legendado)
[Unison FanSub] Naruto Classic - 12 (Portuguese sub)
```

### `c=1_0` é armadilha

A saída preguiçosa seria trocar `c=1_2` por `c=1_0` (*Anime*, todas as subcategorias) e deixar o
resto do pipeline filtrar. Medido, `q=frieren`:

| categoria | English-translated | Non-English-translated |
|---|---|---|
| `c=1_0` | **73** | **2** |
| `c=1_2` | 75 | 0 |
| `c=1_3` | 0 | 75 |

`s=seeders&o=desc` enterra o PT-BR sob o release inglês, que tem uma ordem de grandeza mais
seeders. É a **forma exata** do bug de [decisions.md #59](../../agents/decisions.md) (pack
enterrado sob episódios avulsos). Ver `1_3` exige **busca separada**, não um parâmetro mais largo.

---

## As duas populações PT-BR

| | onde | sinal no título | cobertura medida |
|---|---|---|---|
| **A. MultiSub** (Erai-raws, ToonsHub) | `c=1_2` — já baixado hoje | só `[MultiSub]` / `Multi Subs`, sem enumerar | ~todo anime em exibição |
| **B. PT-BR nativo** (WF, Pharaoh, AT, Kanshin) | `c=1_3` — invisível hoje | `[PT-BR]`, `(Legendado)` | esparso: 2 hits para Frieren, **0** para One Piece e Mushoku Tensei |

Consequência que orienta todo o resto: **para anime em exibição, o PT-BR que já está em disco
veio do Erai-raws sem ninguém saber.** Para backlog, o PT-BR nativo existe mas é raro por anime.
Um desenho que ignore a população A entrega uma feature que não funciona justamente no caso comum.

### Heurística por grupo não funciona — medido

A tentação é dizer "Erai-raws e ToonsHub sempre têm PT-BR" e pular a view page. Erai-raws é
consistente (3/3 releases distintos com `**Portuguese** (Brazilian_CR)`, formato idêntico), mas
só porque é Crunchyroll e nada mais. ToonsHub, 3/3 distintos:

| release | fonte | legendas |
|---|---|---|
| Daemons of the Shadow Realm S01E20 | NF | English, Chinese (Traditional), Indonesian, Korean, Malay, Thai, Vietnamese — **sem português** |
| Chainsmoker Cat S01E08 | NF | …, **Portuguese (Brazil)**, … |
| BLEACH TYBW S01E43 | DSNP | com português |

"Multi Subs" do ToonsHub reflete o conjunto regional do streaming de origem — Netflix Ásia versus
Netflix Brasil. A heurística erraria ~1/3 das vezes, **e erraria dizendo "tem" quando não tem**,
que é a direção que quebra o modo duro. Por isso o modo duro precisa mesmo da view page.

---

## Decisões

| # | Decisão | Alternativa descartada |
|---|---|---|
| 1 | Preferência por padrão, com flag global para virar filtro duro | Só duro (esvazia anime em exibição); só preferência (não atende quem exige PT-BR) |
| 2 | Busca extra em `c=1_3`, e **só** quando há idioma não-inglês configurado | `c=1_0` (enterra por seeders); sempre (dobra requisições de quem não usa) |
| 3 | Flag do modo duro é global, no `Priorities` | Por anime — cabe em `AnimeSettings`, mas custa endpoint + UI por anime. Anotado no TODO |
| 4 | View page só no modo duro, só para candidato ambíguo, com curto-circuito e cache | View page para todo candidato do top-N (multiplica requisições no caminho quente por um dado que só o modo duro consome) |
| 5 | Grep no texto da descrição, sem parser estruturado | Parser do bloco `Subtitles Info:` — divergente entre uploaders e some sem aviso |
| 6 | Vocabulário fechado de idiomas | Lista livre de strings: `c=1_3` não saberia se deve rodar |

---

## Desenho

### 1. Config

Em `internal/nyaa/priorities.go`:

```go
type Priorities struct {
    // ... campos existentes
    // SubtitleLanguages é a ordem de preferência de idioma de legenda, por chave
    // conhecida (ver subtitleLangs). Vazia = feature desligada.
    SubtitleLanguages []string `json:"subtitle_languages"`
    // RequireSubtitleLanguage transforma a preferência em filtro duro: um release
    // que não tenha nenhum dos idiomas de SubtitleLanguages é descartado.
    RequireSubtitleLanguage bool `json:"require_subtitle_language"`
}
```

`DefaultPriorities()` devolve `nil` e `false`. Um `config.json` existente carrega desligado —
`LoadConfigs` desserializa por cima dos defaults, então nenhuma instalação acorda com
comportamento novo. Mesmo contrato de `notifications.batch_window_seconds`
([config.md](../../agents/config.md)).

`CriteriaOrder` ganha `"subtitle"` no default, **entre `health` e `fansub`**:

```go
CriteriaOrder: []string{"uncensored", "source", "resolution", "health", "subtitle", "fansub", "audio", "size"}
```

Idioma importa mais que qual grupo assinou, mas não a ponto de aceitar torrent morto. Mesma
ressalva de release que já vale para `criteria_order`: um `config.json` existente **mantém a
ordem que já tem** e nunca vê o critério novo até editar à mão ou clicar em "restaurar padrões".
Com `subtitle` ausente da ordem, `sortByCriteria` simplesmente pula (o `if !ok { continue }` já
existe) — degradação silenciosa e correta.

### 2. Vocabulário de idiomas

```go
type subtitleLang struct {
    TitleAliases []string // casados contra o nome do torrent
    DescAliases  []string // casados contra o corpo da view page (superset)
    NyaaCategory string   // "1_2" ou "1_3": onde este idioma vive
}

var subtitleLangs = map[string]subtitleLang{
    "pt-br": {
        TitleAliases: []string{"pt-br", "ptbr", "pt_br", "pt br", "português", "portugues", "portuguese", "legendado"},
        DescAliases:  []string{"portugu", "pt-br", "brazil"},
        NyaaCategory: "1_3",
    },
    "en": {TitleAliases: []string{"eng", "english"}, DescAliases: []string{"english"}, NyaaCategory: "1_2"},
    "fr": {
        TitleAliases: []string{"vostfr", "subfrench", "french"},
        DescAliases:  []string{"french", "français", "francais"},
        NyaaCategory: "1_3",
    },
    "es-la": {
        TitleAliases: []string{"es-la", "esla", "castellano", "español", "espanol", "spanish", "subtitulado"},
        DescAliases:  []string{"spanish", "español", "espanol"},
        NyaaCategory: "1_3",
    },
}
```

**Esses quatro são o conjunto inicial completo** — `pt-br`, `en`, `fr`, `es-la`. Não é uma amostra
a ser expandida na implementação: `fr` e `es-la` entram porque são baratos (mesmo mecanismo, só
mais aliases) e porque validam que o desenho não é um caso especial de PT-BR. Idioma novo depois
é uma entrada no map e nada mais.

Fechado de propósito (decisão 6): é `NyaaCategory` que diz se a busca em `c=1_3` precisa rodar.

**Duas portas, dois comportamentos, e não é contradição:** `handleUpdateConfig` **rejeita** chave
desconhecida com HTTP 400 (é erro de digitação do usuário, e falhar alto é o que o poupa de
descobrir semanas depois); `LoadConfigs` apenas **ignora** com `logger.Warn` (um `config.json`
escrito por uma versão futura, ou editado à mão, não pode impedir o daemon de subir).

Casamento por substring case-insensitive sobre o nome, como `ShouldIgnore` e `fansubPriority` já
fazem. **Sem `\b`** nos aliases curtos com hífen (`pt-br` aparece dentro de `[PT-BR]` e de
`[1080p][PT_br]`), mas `"pt br"` e `"eng"` precisam de fronteira para não casar dentro de
palavra — `"eng"` casaria em "Avenger", "Revenge", "Kingdom". Regex por idioma, pré-compilada no
package init como o resto de `nyaa_regex.go`, é o caminho: os regexes rodam dentro de laço
por-torrent e compilar na chamada é lento (comentário no topo de `nyaa_regex.go`).

### 3. Três estados, não dois

```go
type SubtitleMatch int

const (
    SubtitleNo      SubtitleMatch = iota // idioma diferente identificado no título
    SubtitleUnknown                      // marcador multi sem enumerar idiomas
    SubtitleYes                          // alias do idioma pedido no título
)
```

| estado | como se chega | exemplo |
|---|---|---|
| `SubtitleYes` | alias de um idioma de `SubtitleLanguages` no título | `[WF] ... [PT-BR]`, `... (Legendado)` |
| `SubtitleUnknown` | `reMultiSub` casa e nenhum alias específico casa | `[Erai-raws] ... [MultiSub]`, `[ToonsHub] ... [Multi Subs]` |
| `SubtitleNo` | nada disso | `[SubsPlease] ... (1080p)`, `... VOSTFR` quando o pedido é pt-br |

```go
var reMultiSub = regexp.MustCompile(`(?i)\bmulti[\s._-]?subs?\b|\bmultiple\s+subtitle\b|\bmulti\s+subs\b`)
```

Um release sem marcador nenhum em `c=1_2` é `SubtitleNo` para pt-br pela regra geral — a
categoria já afirma "English-translated". Não é necessário raciocinar sobre categoria no código:
sem alias e sem marcador multi, cai em `SubtitleNo`.

**Falso negativo conhecido e aceito:** um release de `c=1_3` sem marcador nenhum também cai em
`SubtitleNo`, mesmo estando numa categoria que garante que ele *não* é inglês. Medido:
`[RLSP] One Piece 733-736 [BD 720p]` e `[KiyoshiiSubs] One Piece - 1172v2 [1080p]` são assim — o
idioma está só no nome do grupo. Marcá-los `Unknown` pela categoria seria mais preciso, mas no
modo duro dispararia um GET de view page para cada release francês, húngaro ou russo que a busca
em `1_3` trouxer — e são a maioria dela. Fica `No`: perde-se um grupo PT-BR sem marcador, evita-se
uma enxurrada de fetches inúteis. Se aparecer demanda, o caminho é o usuário pôr o grupo em
`priorities.fansubs`, não a categoria virar sinal.

`TorrentResult` ganha dois campos:

```go
SubtitleMatch SubtitleMatch `json:"-"`               // derivado, não serializado
ViewURL       string        `json:"viewUrl,omitempty"` // "/view/<id>", só usado no modo duro
```

`ViewURL` sai de `cells.Eq(1).Find("a").Not(".comments").AttrOr("href", "")` — hoje essa célula é
lida só pelo texto (o nome) e o href é descartado. Os três `parseRow` passam a preenchê-lo.

### 4. Ordenação — modo preferência (o default)

Critério novo em `criterionCompare`:

```go
"subtitle": func(a, b TorrentResult) int { return int(b.SubtitleMatch) - int(a.SubtitleMatch) },
```

Ordem resultante: **Yes < Unknown < No**.

`SubtitleUnknown` no meio é deliberado. Um `[MultiSub]` do Erai-raws *provavelmente* tem PT-BR
(3/3 medidos); rebaixá-lo ao nível do SubsPlease jogaria fora a melhor aposta disponível sem
nenhuma evidência contrária. Promovê-lo a `Yes` seria a mentira que a medição do ToonsHub
desmentiu.

`subtitle` entra em `episodeCriteria` (o subset que `SortTorrentResults` usa) — senão o critério
só valeria para filme, que é o caso raro.

**Neste modo o custo de rede é zero.** Tudo sai do título, que já está em memória.

### 5. Busca em `c=1_3`

Nos três pontos o `c` deixa de ser literal:

```go
// nyaaCategories devolve as categorias a varrer: sempre 1_2, mais 1_3 quando
// algum idioma configurado vive lá.
func nyaaCategories() []string
```

Com `SubtitleLanguages` vazia devolve `["1_2"]` — **contagem de requisições idêntica à de hoje**.
Com `pt-br` configurado devolve `["1_2", "1_3"]`, e cada busca roda o `fetchSearchPages` uma vez
por categoria, concatenando em `results` antes do `SortTorrentResults`. O
`deduplicateByMagnet` que já existe cuida de sobreposição.

O descenso de páginas (`fetchSearchPages`, que só vai além da página 1 enquanto tiver menos de 3
candidatos aceitos) roda **por categoria**, não sobre o total: senão 3 candidatos ingleses na
página 1 de `1_2` encerrariam o descenso e `1_3` nunca seria varrida a fundo. É a mesma armadilha
do `c=1_0`, só que dentro do loop.

### 6. Modo duro

Quarto filtro em `filterSearchResults` (`daemon/search.go:96`), depois do teto de tamanho e do
piso de seeders, e **só** quando `RequireSubtitleLanguage`.

A assinatura precisa mudar: hoje é `filterSearchResults(results, maxGB, minSeeders)` e o
curto-circuito depende de `EpisodeRetryLimit`. Com três parâmetros de config já em jogo, passar
`*files.Config` inteiro é mais barato que um quarto escalar — e é o que os cinco pontos de
chamada já têm em mãos: `debug.go:138`, `episodes.go:216`, `:293`, `:407` e `:408`. Vira
`filterSearchResults(results, maxGB, minSeeders, configs)`; `maxGB` continua explícito porque
alterna entre `MaxBatchTorrentSizeGB` e `MaxEpisodeTorrentSizeGB` conforme o caminho.

O algoritmo:

```
para cada resultado, na ordem já ordenada:
    SubtitleYes     → aceita, sem fetch
    SubtitleNo      → descarta, sem fetch
    SubtitleUnknown → 1 GET na ViewURL; strings.ToLower no corpo; regex dos DescAliases
                      aceita ou descarta conforme o resultado
    para de resolver ao juntar EpisodeRetryLimit aceitos; o resto cai sem fetch
```

**Curto-circuito:** `attemptDownloadWithRetries` (`episodes.go:599`) só tenta os primeiros
`EpisodeRetryLimit` magnets. Resolver além disso é fetch jogado fora. Como a lista chega
ordenada, parar em `EpisodeRetryLimit` aceitos custa exatamente o mesmo que resolver na hora de
baixar — **sem** mexer na assinatura `magnets []string`, que hoje já achatou o `TorrentResult`
antes do download (`episodes.go:218`).

**Cache:** `map[string]bool` guardado por `ViewURL`, protegido por `sync.RWMutex`, no pacote
`nyaa`. Sem TTL e sem persistência: o conteúdo de uma view page do Nyaa não muda, e perder o
cache no restart custa um fetch. Sem ele, um anime que nunca resolve refaria os fetches **a cada
passada de 10 min** — que é o caso ruim que o desenho existe para evitar.

**Fetcher injetável**, no padrão de `MockNyaaHttpGet`/`SetPriorities`:

```go
func SetSubtitleFetcher(fn func(viewURL string) (string, error)) (restore func())
```

Assim `filterSearchResults` continua função de lista→lista testável sem rede.

**Sem parser estruturado** (decisão 5): `Subtitles Info:` do Erai-raws e `Subtitles:**` do
ToonsHub já divergem hoje, e um terceiro uploader diverge de novo amanhã. Grep no corpo inteiro
em minúsculas.

**Falha de rede vale como aceite.** Um GET que estoura timeout não é evidência de que falta
PT-BR, e tratar como recusa transformaria instabilidade de rede em "nenhum episódio baixa".
Loga em `Warn` e o resultado passa. Falha **não** entra no cache — senão um blip de rede
envenenaria o veredito até o restart.

### 7. Relatório

O corte por idioma precisa entrar no `dropStats` de `search.go`, ao lado de `sizeDropped` e
`seedersDropped`, e chegar ao `searchIssue` (`episodes.go:231`). Sem isso o relatório diz
"nenhum torrent encontrado" quando a verdade é "3 encontrados, todos sem PT-BR" — e essa é
exatamente a diferença entre o usuário entender ou não entender por que o episódio não baixou.
Mesma lógica que motivou o `dropStats` original.

### 8. Frontend

`Priorities.svelte` é genérica sobre `LISTS`, todas `string[]`. `subtitle_languages` é `string[]`
e entra como mais uma entrada — mas com duas diferenças:

- **Vocabulário fechado:** o campo de adicionar vira `<select>` com as chaves de
  `subtitleLangs`, em vez do `<input>` de texto livre. As chaves vêm do
  `GET /config/priorities/defaults`, que passa a devolver a lista de idiomas suportados junto —
  sem endpoint novo.
- **`require_subtitle_language` é bool**, não cabe na lista genérica: checkbox logo abaixo,
  desabilitado enquanto `subtitle_languages` estiver vazia (o flag não significa nada sem idioma
  configurado).

Rótulo em português literal, como as demais entradas de `LISTS` (`Priorities.svelte:13-21`).

---

## Testes

Em `nyaa/priorities_test.go` e `daemon/search_test.go`, no padrão de mock de
[testing.md](../../agents/testing.md).

**Unit — detecção de estado.** Tabela título → `SubtitleMatch` para pt-br, cobrindo os três
estados: `[PT-BR]`, `PTBR`, `[PT_br]`, `(Legendado)`, `Portuguese sub` → `Yes`; `[MultiSub]`,
`Multi Subs`, `Multiple Subtitle` → `Unknown`; `[SubsPlease] ... (1080p)`, `... VOSTFR` → `No`.
Mais o caso de fronteira: um título com `"Avenger"` **não** pode casar `en`.

**Unit — ordenação.** Três resultados idênticos em tudo menos o estado, confirmando
Yes < Unknown < No, e que `subtitle` ausente de `criteria_order` não quebra o sort.

**Unit — modo duro com fetcher mockado.** `Unknown` cujo corpo contém "portuguese (brazil)"
passa; `Unknown` com só "english, korean, thai" cai; o **segundo** encontro com a mesma `ViewURL`
não chama o fetcher (cache); erro do fetcher deixa passar e **não** popula o cache.

**Unit — curto-circuito.** Lista com 10 `Unknown` e `EpisodeRetryLimit = 3` chama o fetcher no
máximo 3 vezes.

**Unit — categorias.** `nyaaCategories()` devolve `["1_2"]` com lista vazia e com `["en"]`;
devolve `["1_2","1_3"]` com `["pt-br"]`.

**Integração — a feature é grátis para quem não usa.** Com `subtitle_languages` vazia, a
contagem de GETs ao Nyaa numa passada de verificação é **idêntica** à de hoje. É o teste que
protege a decisão 2, e o único que falha se alguém "simplificar" `nyaaCategories` mais tarde.

---

## Checklists a seguir

Campo de config ([conventions.md](../../agents/conventions.md#adding-a-new-config-field--checklist)):
o campo mora em `nyaa.Priorities`, que já é serializado dentro de `Config.Priorities` — nada a
fazer em `filemanager.go` além de o default vir de `DefaultPriorities()`. Validação em
`handleUpdateConfig`: rejeitar chave de idioma desconhecida com HTTP 400.

Depois da implementação, conforme [CLAUDE.md](../../../CLAUDE.md):

1. `go test ./...`
2. `docs/agents/config.md` — linhas de `priorities.subtitle_languages` e
   `priorities.require_subtitle_language`, com a nota de release sobre `criteria_order` existente
   não ganhar `subtitle` sozinho
3. `docs/agents/architecture.md` — campos novos de `TorrentResult`, `nyaaCategories`
4. `docs/agents/decisions.md` — entrada para "por que busca separada em `1_3` e não `c=1_0`",
   ancorando na medição 73/2 de Frieren
5. `docs/agents/sources.md` — a seção do Nyaa afirma "Hoje o daemon usa `f=0`" e descreve `c=1_2`
   como fixo; passa a ser condicional
6. `swag init -g src/cmd/daemon/main.go -o docs/swagger` — o payload de
   `/config/priorities/defaults` muda

---

## Fora de escopo, deliberado

- **Override por anime.** Faz sentido (aceitar inglês num shounen e exigir PT-BR no que se
  assiste acompanhado) e `AnimeSettings` é o lugar, mas custa endpoint + UI por anime. Fica
  anotado no `TODO-competitive.md`.
- **Listar arquivos do torrent.** Softsub vive dentro do `.mkv`; não há o que listar. Medido.
- **AnimeTosho como fonte de idioma.** O `files[]` dele traz `filename`/`size`/`crc32` e nenhum
  sabe idioma de legenda. Não muda o veredito de [sources.md](../../agents/sources.md).
- **Selecionar a faixa de legenda no player / remuxar.** O app entrega o arquivo; qual faixa o
  Jellyfin escolhe é configuração do Jellyfin.
- **`c=1_1` (AMV) e `c=1_4` (Raw).** Nenhum dos dois hospeda episódio legendado.
