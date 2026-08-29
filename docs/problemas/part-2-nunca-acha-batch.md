# Anime com "Part 2" no título nunca acha batch

Descoberto em 2026-08-28, a partir de um caso real: Mushoku Tensei S2 Part 2 (AniList `166873`)
adicionado como avulso baixou os 12 episódios **um a um**, mesmo com packs de Part 2 na primeira
página do Nyaa.

Nada disso está corrigido — este arquivo é o registro do diagnóstico.

> **Este problema tem raiz comum com o outro deste diretório.** A análise conjunta, a
> investigação da AniList e o plano de correção estão em
> [raiz-comum-eixo-absoluto.md](raiz-comum-eixo-absoluto.md).

## Evidência

`~/.autoAnimeDownloader/daemon.log`, passe das 18:01:

- linha 414/415: `Found anime torrents on Nyaa` com `results: 24` — **todos episódios avulsos**
  Erai-raws, nenhum pack.
- linha 416+: `Attempting to download episode … Episode 1..12`, sem a linha
  `Using batch torrents to cover the pending window` (`src/internal/daemon/episodes.go:153`).

Comparativo que fecha o caso: o Part 1 (`146065`, romaji sem "Part") tem `requestedPart == nil`,
o filtro duro de part nem roda, e ele pegou o pack Erai-raws normalmente (linha 217 do mesmo log).

## Os packs existiam e cada um morreu de um jeito

Valores conferidos rodando `isBatch` / `extractPart` / `extractSeason` / `titleMatchesQuery` do
pacote `nyaa` com os nomes reais das linhas `Raw Nyaa row` do passe.

Query usada: `extractSeasonFromName("Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2")` →
`"Mushoku Tensei II: Isekai Ittara Honki Dasu"`, tokens
`[mushoku tensei ii isekai ittara honki dasu]`.

| Torrent | batch | part | match |
|---|---|---|---|
| `[EMBER] … (2024) (Season 2 \| Part 2) … (Mushoku Tensei II: Isekai Ittara Honki Dasu Part 2) (Batch)` (3 releases) | sim | 2 | **não** |
| `[Diddy] Mushoku Tensei - S02 (BD 1080p HEVC Opus) …` | sim | nil | não |
| `[NTRX] … S02 (Season 2) REPACK …` | sim | nil | não |
| `[Fuchs] … - S02 … (Season 2) (Part 1 + Part 2)` | sim | **1** | não |
| `[Erai-raws] … Honki Dasu - 00 ~ 12 [BATCH]` | sim | nil | sim |

### 1. Causa principal — os únicos packs de Part 2 caem no match de título

`truncateAtFirstMarker` (`src/internal/nyaa/nyaa_match.go:63`) corta o nome no primeiro marcador
de temporada. No nome do EMBER esse marcador é `(Season 2 |`, que vem **antes** do romaji entre
parênteses. Sobram os tokens `[mushoku tensei jobless reincarnation]`; `isekai/ittara/honki/dasu`
somem, e `titleMatchesQuery` exige todos os tokens da query no torrent → rejeitado em
`src/internal/nyaa/nyaa.go:652`, antes mesmo de chegar ao caminho de batch.

### 2. Filtro duro de part derruba os packs de season inteira

`src/internal/nyaa/nyaa.go:679`: com `requestedPart != nil`, pack sem marcador de part explícito
é descartado. Isso mata Diddy, NTRX e o Erai `00 ~ 12` — os dois primeiros contêm os episódios do
Part 2, o último é Part 1 mesmo (descarte correto).

O Fuchs é o caso mais irônico: cobre `Part 1 + Part 2`, mas `extractPart` (`nyaa.go:892`) devolve
o **primeiro** número que casar, ou seja `1` ≠ `2` → descartado.

### 3. Agravante — a variante em inglês nunca é tentada

`searchNyaaWithVariants` (`src/internal/daemon/search.go:116`) para na primeira variante que
devolve resultado, e `GenerateSearchTitleVariants` (`nyaa.go:64`) põe romaji primeiro. O romaji
achou os 24 episódios avulsos, então
`"Mushoku Tensei: Jobless Reincarnation Season 2 Part 2"` nunca rodou — e com ela os packs do
EMBER casariam, já que os tokens truncados (`mushoku tensei jobless reincarnation`) são
exatamente os da variante inglesa.

## Se um dia for resolver

Três frentes independentes, em ordem de retorno:

- **Match**: não descartar o texto depois do marcador quando ele contém um título alternativo
  entre parênteses/pipes — ou casar contra **todas** as variantes de título, não só a que
  originou a busca.
- **Part**: pack sem marcador de part não deveria ser rejeitado de cara quando ele é da season
  pedida e cobre a janela pendente — o `pickBatches` já decide cobertura. E `extractPart`
  precisa entender `Part 1 + Part 2` como "cobre as duas".
- **Variantes**: no caminho de pack, tentar as outras variantes quando a primeira não devolveu
  nenhum batch (hoje ela só continua se não devolveu nada).
