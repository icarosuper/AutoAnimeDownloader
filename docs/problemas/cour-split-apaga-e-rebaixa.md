# Cour dividido: o pack da season inteira é apagado e rebaixado

Descoberto em 2026-08-28, a partir de um caso real: Mushoku Tensei S1 baixado como um único
torrent da season inteira, sob a entrada do **cour 1** da AniList. Ao marcar o cour 1 como
terminado e começar o cour 2, o daemon apaga o torrent e baixa outro.

Nada disso está corrigido — este arquivo é o registro do diagnóstico.

> **Este problema tem raiz comum com o outro deste diretório.** A análise conjunta, a
> investigação da AniList e o plano de correção estão em
> [raiz-comum-eixo-absoluto.md](raiz-comum-eixo-absoluto.md).

## Cenário

- Um torrent, hash `H`, cobre a season inteira (ex.: episódios 1–23).
- Ele foi baixado sob a entrada do cour 1 (media id `X`, 11 episódios), então
  `downloaded_episodes` tem as chaves `{X,1}..{X,11}`, todas com `EpisodeHash = H` e
  `IsBatch = true`.
- O cour 2 é **outro media id** na AniList (`Y`), com numeração própria 1–12.

## O que acontece no passe seguinte

### 1. O cour 1 sai do universo do passe

`download_statuses` default é `["CURRENT","REPEATING"]` (`src/internal/files/filemanager.go:206`).
Marcado como `COMPLETED`, o anime some da lista buscada em `searchAnilist`
(`src/internal/daemon/verification.go:513`) → `processAnimeEpisodes` nunca roda para ele →
nenhuma das 11 chaves entra em `checkedEpisodes`.

### 2. Os registros viram órfãos e são podados

`identifyEpisodesNotInWatching` (`src/internal/daemon/episodes.go:865`) devolve todo registro
salvo que não foi checado neste passe. Com `delete_watched_episodes: true` (default,
`filemanager.go:201`), as 11 chaves vão para o delete set em `handleSavedEpisodes`
(`episodes.go:764`).

### 3. O batch guard não segura

`allEpisodesInDeleteSet` (`episodes.go:853`) agrupa os registros por hash e só preserva o torrent
se **algum irmão sobreviver**. Como todos os registros de `H` são do cour 1 e todos estão sendo
apagados, o grupo fecha → `backend.Remove(H, keepData=false)` (`episodes.go:835`) → **torrent e
dados apagados do disco**.

`watched_episodes_to_keep` não salva: registro de pack é explicitamente excluído do keep-set
(`episodes.go:489`), e de qualquer forma o anime nem chega a ser processado.

### 4. O cour 2 baixa do zero

A chave é `files.EpisodeKey{AnimeID, Episode}`. Não existe nenhuma dedupe por infohash entre
animes: o único teste de "já tenho isso" é `episodeInTorrents(savedEp.EpisodeHash, ...)`, que
consulta o hash **do registro daquela chave**. O cour 2 (`Y`) não tem registro nenhum → busca no
Nyaa e baixa.

## O agravante: apagar no mesmo passe o que acabou de baixar

Se, no mesmo passe, a busca do cour 2 escolher **o mesmo torrent** (o batch da season inteira
aparecendo na busca do "Part 2"):

1. `Session.Add` detecta o infohash já presente e **reusa** o hash
   (`src/internal/torrents/session.go:76`), sem erro.
2. Os registros novos `{Y,1}..{Y,12}` são salvos apontando para `H`.
3. A Fase 3 (`verification.go:299-306`) chama `handleSavedEpisodes` passando `savedEpisodes` —
   o snapshot lido **antes** do passe. `removeEpisodesAndLinks` (`episodes.go:791`) monta o
   `byHash` a partir desse snapshot, onde `H` só tem os registros do cour 1.
4. O guard não enxerga os irmãos recém-criados → remove o torrent com `keepData=false`.

Resultado: os registros do cour 2 apontam para um hash que não existe mais, e no passe seguinte
tudo é rebaixado.

**Ponto do bug:** `episodes.go:791` — `removeEpisodesAndLinks` recebe o snapshot pré-passe em vez
do estado posterior a `saveEpisodesToFile`. Correção provável: incluir `data.newEpisodes` no
`byHash` do guard, dentro de `handleSavedEpisodes`.

## Contornos hoje, sem mudar código

- Marcar os episódios do cour 1 como **`ManuallyManaged`**. A flag é checada tanto em
  `identifyEpisodesNotInWatching` quanto em `deleteEpisodesByStatus` (`episodes.go:744`) e blinda
  o registro contra a poda automática. É o caminho previsto para exatamente este caso.
- Ou `delete_watched_episodes: false`, mas aí nada é podado nunca.

Nenhum dos dois impede o cour 2 de baixar um torrent novo. Isso é estrutural: o daemon não tem
noção de que um hash já cobre episódios pertencentes a outra entrada da AniList.

## Se um dia for resolver de verdade

O problema de fundo é que a unidade de posse do torrent é a chave `(anime_id, episódio)`, e um
pack de season atravessa vários `anime_id`. As duas pontas:

- **Não apagar**: o guard olhar o hash em toda a `downloaded_episodes`, não só nos registros do
  anime sendo podado — o que já resolveria o agravante do mesmo passe de quebra.
- **Não rebaixar**: antes de buscar no Nyaa, procurar um torrent já baixado cujo `BatchStart..
  BatchEnd` (mais o offset de `ComputeEpisodeOffset`) cubra o episódio pedido, em qualquer
  `anime_id`.
