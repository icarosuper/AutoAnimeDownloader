# `debug-batch`: robustez da busca em animes fora da lista

Data: 2026-08-12

## Problema

A feature de animes avulsos ([`2026-08-10-download-animes-avulsos-design.md`](2026-08-10-download-animes-avulsos-design.md))
tem a plumbing bem coberta: `standalone_guard_test.go`, `endpoint_standalone_animes_test.go`,
`endpoint_anilist_search_test.go`, `api/standalone_test.go`, `daemon/standalone_test.go` e os
testes de merge cobrem guarda, precedência, arquivo, merge pós-dedupe, remoção automática e
`DELETE`. Testes determinísticos não vão dizer nada novo ali.

O que a feature abriu e ninguém mediu é outra coisa: **o pipeline de busca passou a receber
animes que nunca estiveram na lista de ninguém**. Até então todo anime processado vinha de uma
lista do AniList — filtrado por `download_statuses`, com `Progress` real, em geral da temporada
corrente. Um avulso pode ser filme, OVA, série de 1000+ episódios, sequência marcada só por
numeral romano, anime velho que só existe em batch, título que a comunidade posta traduzido.
Todos entram com `Progress: 0`, ou seja, do episódio 1.

`resolveSearchStrategy`, `titleMatchesQuery`, `GenerateSearchTitleVariants` e o filtro de
season/part nunca foram exercitados contra essa população. É onde a robustez é de fato
desconhecida — e não é testável de forma determinística, porque a resposta está no Nyaa ao vivo.

### O que já existe

`make debug-anime ID=<mediaId>` (`daemon.RunAnimeDebug`, `daemon/debug.go`) já roda o pipeline
real contra o Nyaa ao vivo, sem baixar nada e sem tocar `~/.autoAnimeDownloader`, e escreve
`debug.jsonl` (trace completo, raw rows + matched) e `summary.json` (por episódio:
`would_search`, `magnets_found`) em `.debug_<id>_<N>/`.

E — o detalhe que faz dele a ferramenta certa aqui — `resolveAnimeDetails`
(`daemon/manual_download.go:62`) já cai em `GetMediaByID` quando `GetAnimeInfo` devolve `nil`.
O `debug-anime` já funciona para anime que não está em lista nenhuma, que é exatamente o caso
avulso.

Falta o lote: rodar sobre uma população curada e agregar o resultado.

### E um bug que invalidaria o lote

`RunAnimeDebug` **diverge do loop real na seleção de episódios**, e a divergência bate justo nos
animes grandes:

| | `RunAnimeDebug` (`debug.go:67`) | `processAnimeEpisodes` (`episodes.go:43`) |
|---|---|---|
| Enumeração | `anime.Media.AiringSchedule.Nodes` cru | `anilist.EpisodeList(anime, firstEpisodeToConsider(anime, savedEpisodes))` |
| Teto por anime | `configs.MaxEpisodesPerAnime`, sempre | `len(episodes)+1` quando `willBatchAnime(configs, anime)` |

**1. Conjunto de episódios errado em série longa.** O comentário de `episodes.go:45` registra o
caso: no One Piece a página 1 da agenda vai do **1123 ao 1147**. Um avulso tem `Progress: 0`,
então `firstEpisodeToConsider` devolve 1 e a produção enumera 1..1147 — pegando os 12 primeiros
pelo teto. O debug hoje itera só os 25 nós crus e reporta ter buscado **1123 a 1147**. Nenhum
anime de agenda clipada é medido corretamente. É precisamente a classe de anime que a
`EpisodeList` sintética (decision 52) existe para cobrir, e a única que o debug não usa.

**2. A regra batch↔limite nunca é avaliada.** `willBatchAnime` (`episodes.go:174`) exige
`FINISHED`, não-filme, `Episodes > 0` e `Episodes <= max_batch_episodes` (default 30); quando
verdadeira, `processAnimeEpisodes` desliga o teto por anime. O debug nunca a chama. A spec de
avulsos declara essa regra como **pré-requisito** — "um anime de 1100 episódios adicionado a
dedo cai no caminho um-a-um limitado em vez de virar um torrent único" — e é justamente a regra
para a qual o `debug-anime` é cego.

Rodar o lote sobre animes grandes sem corrigir isso produziria um relatório descrevendo um
pipeline que o daemon não executa, e o fine-tuning seria feito contra ficção.

## Objetivo

Um alvo de make que roda o `--debug-anime` sobre uma lista curada e versionada de animes fora da
lista do usuário, e entrega um `report.md` **triado** — não os traces crus.

A triagem é o requisito, não conveniência: um `debug.jsonl` tem centenas de raw rows do Nyaa por
anime. Despejar 12 deles num agente gasta o contexto em ruído antes de chegar ao caso
interessante. O relatório aponta os suspeitos; só esses têm o `debug.jsonl` aberto.

Não-objetivo: subir daemon, chamar `POST /standalone-animes`, verificar estado do arquivo de
avulsos, baixar torrent. Isso é a plumbing, e ela já tem teste determinístico.

## Arquitetura

### O alvo se chama `debug-batch`, e não `test-*`

Não é suite: usa rede ao vivo, leva minutos, e não tem pass/fail — o resultado é um relatório
para leitura humana ou de agente. Com prefixo `test-` ele acaba entrando em
`scripts/run-all-tests.sh` e o CI passa a depender da disponibilidade do Nyaa. Fica em **Dev
targets** no `help`, ao lado do `debug-anime`.

### Correção em `RunAnimeDebug`

Espelhar as duas linhas do `processAnimeEpisodes`, dentro de `daemon/debug.go`:

```go
episodes := anilist.EpisodeList(anime, firstEpisodeToConsider(anime, nil))

maxEpisodes := configs.MaxEpisodesPerAnime
if willBatchAnime(configs, anime) {
    maxEpisodes = len(episodes) + 1
}

for _, ep := range episodes {
    shouldDownload, _ := checkEpisode(configs, maxEpisodes, ep, anime, false, &downloadedEpisodes, false, false)
    // ...igual ao que já está lá
}
```

`nil` em `firstEpisodeToConsider` não é omissão: a função só usa `savedEpisodes` para recuar o
início abaixo de `Progress + 1`, e o debug já declara — no `ponytail:` de `RunAnimeDebug` e no
`Warn` que ele emite — que trata todo episódio como não-baixado. Com `nil` resolve para
`Progress + 1`, que num avulso é 1.

`resolveSearchStrategy` já é o de produção; a divergência é só na seleção e no teto, e é só isso
que muda.

**Efeito no `summary.json`:** `summary.Episodes` passa a ter uma entrada por episódio enumerado —
em One Piece, ~1147 em vez de 25, a maioria com `would_search: false` depois do teto. É o preço
da fidelidade, e o relatório agrega. Quem lê o `summary.json` direto lê pelo `jq`.

**Efeito na duração:** um anime `FINISHED` de até 30 episódios agora entra em batch com o teto
desligado, e `resolveSearchStrategy` recebe todos os episódios. Se o batch resolver, é uma busca
só; se não resolver, cai no multi/single e vira N buscas para um anime só. Isso alonga a rodada —
e é informação legítima sobre o comportamento real, que é o que a rodada existe para produzir.

### `scripts/robustness-animes.txt`

Um `<mediaId>` por linha, comentário à direita com o caso que cobre. `#` no início e linhas
vazias ignoradas.

**Restrição que precisa estar escrita no topo do arquivo:** `resolveAnimeDetails` tenta
`GetAnimeInfo(id, usernames)` **antes** do `GetMediaByID`. Se o anime estiver numa lista do
usuário, o debug usa a entrada real, com o `Progress` real — e a rodada testou o caminho normal,
não o avulso. **Os IDs têm que ser de animes que o usuário não acompanha.**

Lista semente, cobrindo as bifurcações de `willBatchAnime` e os casos que os commits recentes
tocaram:

| Caso | Semente | O que exercita |
|---|---|---|
| Longo + `RELEASING` | One Piece, Detective Conan | agenda clipada, `EpisodeList` sintética, zero-padding (decision 56), teto de 12 |
| Longo + `FINISHED` acima de `max_batch_episodes` | Naruto Shippuden | `willBatch = false` pelo teto → um-a-um limitado. A regra da spec de avulsos |
| Curto + `FINISHED` dentro do teto | Death Note | `willBatch = true` → teto desligado, batch em `resolveSearchStrategy` |
| Filme | Kimi no Na wa | `isAnimeMovie`, episódio falso, `hasMovieMarker` (decision 53) |
| Split-cour / part | Mushoku Tensei | `ExtractAnimeSeasonPart`, offset por PREQUEL |
| Sequência em numeral romano | Sword Art Online II | `reRomanSeason` (decision 20) |
| Título postado traduzido | Kimetsu no Yaiba (postado como "Demon Slayer") | `GenerateSearchTitleVariants`, Jaccard de `titleMatchesQuery` |

A semente é por **título**, e o ID de cada um se confere na primeira rodada: o relatório imprime
o `anime_name` que a AniList devolveu em cada linha, então um ID trocado aparece sozinho — nome
que não bate, ou linha na seção `ERROS`. Conferir antes de tratar qualquer resultado da primeira
rodada como sinal.

O arquivo cresce por uso: todo anime que falhar na vida real vira uma linha aqui.

### `scripts/debug-batch.sh`

1. `go build -o build/aad-debug ./src/cmd/daemon` — uma vez, não por anime.
2. `rm -rf .debug_batch && mkdir -p .debug_batch`.
3. Para cada id: roda o binário (por caminho absoluto, resolvido antes do loop) com
   `--debug-anime <id>` e **CWD dentro de `.debug_batch/`**, e dorme 2s.
4. Gera `.debug_batch/report.md` com `jq` sobre os `summary.json`, e ecoa o caminho no stdout.

**Por que o CWD em vez de parsear log.** `runDebugAnime` (`main.go:178`) monta o diretório com
`daemon.NextDebugDir(".", animeId)` — sempre no CWD — e não imprime o caminho no stdout (o
`logger.InitDebug` manda tudo para o arquivo). Rodando com o CWD num diretório recém-apagado,
`NextDebugDir` não acha nada e o `N` é **sempre 1**: o caminho vira `.debug_batch/.debug_<id>_1/`,
previsível, sem parse nenhum. Zero alteração em Go para isso.

`.debug_batch/` já cai no `.gitignore` existente (`.debug_*/`), e `.debug_batch` não casa com
`debugDirPattern` (`^\.debug_(\d+)_(\d+)$`), então não confunde a varredura do `NextDebugDir`.

**`runDebugAnime` faz `os.Exit(1)` em falha**, então o loop não pode rodar sob `set -e`: o
código de saída é capturado por anime e vira linha na seção `ERROS`. Um anime inexistente ou a
AniList fora do ar não podem matar a rodada no terceiro de doze. O `debug.jsonl` existe mesmo
nesses casos — `InitDebug` roda antes de qualquer caminho de erro do `RunAnimeDebug` — então o
ponteiro no relatório continua útil.

**`sleep 2` entre animes.** A AniList é 1 requisição por anime, folgado nos 30/min. O Nyaa é a
preocupação: `resolveSearchStrategy` dispara várias buscas por anime, e mais ainda desde a
paginação adaptativa (decision 57). 2s é seguro barato contra throttle.

**A rodada usa o `config.json` real do usuário** (`files.NewDefaultFileManager` →
`LoadConfigs`): `max_episodes_per_anime`, `max_batch_episodes`, `min_seeders`,
`max_episode_torrent_size_gb`, `anilist_usernames`. É desejável — o que se quer medir é o
comportamento com as configurações dele, não com um default sintético. Vale registrar no
relatório os valores usados, para que duas rodadas comparadas não difiram por config esquecida.

### `report.md`

```markdown
# debug-batch — 12 animes — <data da rodada>

config: max_episodes_per_anime=12, max_batch_episodes=30, min_seeders=N

| id | anime | buscados | com magnet | sem magnet |
|---|---|---|---|---|
| 21 | One Piece | 12 | 12 | 0 |
| 21519 | Kimi no Na wa | 1 | 0 | 1 |

## SUSPEITOS (would_search, 0 magnets)
- 21519 Kimi no Na wa — eps 1 → .debug_batch/.debug_21519_1/debug.jsonl

## NADA BUSCADO (nenhum episódio selecionado)
- 12345 Fulano → .debug_batch/.debug_12345_1/debug.jsonl

## ERROS (saída ≠ 0)
- 99999 → .debug_batch/.debug_99999_1/debug.jsonl
```

As três seções são categorias de causa diferentes, e é isso que justifica separá-las em vez de
uma lista só de "deu ruim":

- **SUSPEITOS** — o pipeline buscou e o Nyaa não rendeu magnet. Causa em busca ou filtragem;
  comparar raw rows contra matched no `debug.jsonl` (Passo 6 do
  [troubleshooting](../../agents/troubleshooting-downloads.md)).
- **NADA BUSCADO** — nenhum episódio passou o `checkEpisode`. Causa em progresso, agenda ou
  status, não em busca. `shouldSkipEpisode` já loga o motivo no `debug.jsonl`; não se duplica
  aqui.
- **ERROS** — nem chegou lá. AniList fora, ID inexistente, config faltando.

Cada rodada apaga `.debug_batch/`. Para comparar antes/depois de um ajuste, copiar o `report.md`
antes de rodar de novo — a alternativa (diretório por timestamp) obrigaria o leitor a descobrir
qual rodada abrir, para um ganho que um `cp` resolve.

## Testes

Go, em `daemon/debug_test.go`, um por divergência corrigida — os dois falham no código atual:

- **Série longa com agenda clipada:** `MediaList` com `Progress: 0`, `Episodes: 1147`,
  `AiringSchedule.Nodes` só de 1123 a 1147. `RunAnimeDebug` seleciona a partir do episódio **1**,
  e o `summary.Episodes` cobre 1..1147. Hoje devolve 1123..1147.
- **`FINISHED` dentro de `max_batch_episodes`:** anime `FINISHED` de 26 episódios com
  `max_episodes_per_anime = 12`. Todos os 26 saem com `would_search: true` — o teto por anime
  não se aplica em batch. Hoje sai 12.
- **`FINISHED` acima do teto:** anime `FINISHED` de 500 episódios com `max_batch_episodes = 30`.
  `willBatch` é falsa, o teto vale, saem 12. É o par do anterior: sem ele, "desligar o teto"
  poderia virar "teto nunca vale".

Usam o `nyaaSearcher` mockado que os testes de `episodes_test.go` já usam — nenhum destes toca
a rede.

Shell: nada. `debug-batch.sh` é orquestração e formatação; a lógica que pode errar está no Go, e
o próprio relatório é a saída que se inspeciona.

## Docs a atualizar

- `commands.md`: `make debug-batch` na seção de dev, com a restrição de que os IDs precisam
  estar fora da lista do usuário.
- `troubleshooting-downloads.md`: no Passo 0, uma linha apontando o `debug-batch` como a versão
  em lote, e o `report.md` como ponto de entrada.
- `architecture.md`: `scripts/robustness-animes.txt` e `scripts/debug-batch.sh`; a nota de que
  `RunAnimeDebug` espelha a seleção de episódios do `processAnimeEpisodes`.
- `decisions.md` #58: `RunAnimeDebug` enumera por `anilist.EpisodeList` e avalia `willBatchAnime`
  — iterar `AiringSchedule.Nodes` cru é a regressão a evitar, com o One Piece (agenda 1123–1147,
  produção 1..1147) como o caso que a versão errada quebra. "Don't fix by": manter a enumeração
  crua "porque é mais simples" — a simplicidade produz um diagnóstico que descreve outro
  pipeline.
- Swagger: nada. Nenhuma API muda.

## Fora de escopo

**Alvo com `LIST=` para apontar outro arquivo.** O arquivo é editável e o ciclo de fine-tuning
roda sempre sobre a mesma lista, que é o que torna duas rodadas comparáveis. Adicionar quando
existir uma segunda lista de verdade.

**Chamar `claude -p` dentro do alvo.** Acoplaria o make ao CLI e transformaria o fine-tuning em
one-shot. Ele é conversa: ler o relatório, abrir um `debug.jsonl`, propor um ajuste em
`titleTechnicalTokens` ou num regex, rodar de novo.

**Verificar `episodes.json` e torrents no debug.** A limitação já está declarada no `ponytail:`
de `RunAnimeDebug` e no `Warn` da rodada. Para um avulso recém-adicionado, "nenhum episódio
baixado" é a simulação correta — é o estado real no momento da adição.

**Diretório por rodada, diff automático entre relatórios.** Um `cp report.md report.antes.md`
cobre. Adicionar quando comparar rodadas virar rotina, não hipótese.

**Amostragem aleatória do AniList.** Cobertura mais ampla, mas cada rodada testaria uma
população diferente — impossível dizer se um ajuste melhorou ou piorou. A lista curada é o que
torna o ciclo mensurável.

## Tamanho

2 arquivos novos em `scripts/`, 1 alvo no `Makefile`, 1 arquivo Go tocado (`daemon/debug.go`,
~6 linhas) + 3 testes, 4 docs.
