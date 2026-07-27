# Correções do Code Review — Cliente BitTorrent Embutido

Achados do code review da implementação de `docs/plans/2026-07-24-embedded-torrent-client-design.md`
(cliente rain/v2 embutido, remoção do qBittorrent externo), com as soluções já decididas.

**Estado no momento da escrita:** tudo não-commitado na branch `feature/torrent-integrado`.
`go build ./...`, `go vet ./...` e `go test ./...` passam. Os bugs abaixo não são de compilação —
são de comportamento no caminho de upgrade e de propagação de erro.

---

## Como usar este documento

Cada pacote de trabalho (WP) abaixo é **auto-contido** e pode ir para um agente diferente.
O agrupamento foi feito para **não colidir nos mesmos arquivos**:

| WP | Tema | Arquivos que ele possui (exclusivos) |
|---|---|---|
| WP1 | Persistência de episódios + propagação de erro | `daemon/episodes.go`, `files/filemanager.go`, `api/endpoint_episode_actions.go` |
| WP2 | Librarian (hardlink) | `files/librarian.go`, `files/librarian_test.go` |
| WP3 | Ciclo de vida da sessão + gate de config | `cmd/daemon/main.go`, `daemon/verification.go`, `daemon/helpers.go`, `torrents/sessionmanager.go`, `torrents/sessionmanager_test.go` (novo) |
| WP4 | Limpeza e docs | `docs/**`, `.gitignore`, `frontend/messages/*.json`, `.github/workflows/build.yml` |
| WP5 | Lacunas de teste de orquestração | `daemon/orchestration_test.go`, `torrents/session_internal_test.go` |

**Ordem recomendada:** WP1 e WP2 primeiro (são a causa raiz do bug crítico e um depende
conceitualmente do outro — ver nota no WP2). WP3, WP4 e WP5 podem rodar em paralelo com eles.

**Um único ponto de contato entre WPs:** WP2 corrige `Organize` para comparar inode; WP1 corrige
a persistência que alimenta o nome do destino. Os dois juntos fecham o bug crítico, mas cada um
é uma melhoria válida sozinho e eles tocam arquivos diferentes.

**Números de linha são indicativos** (do estado atual da working tree). Confirme lendo o arquivo
antes de editar — não aplique patch cego em número de linha.

---

## Contexto compartilhado (leitura obrigatória para qualquer agente)

Projeto: **AutoAnimeDownloader** — daemon Go que baixa episódios de anime automaticamente.
Integra AniList (lista de acompanhamento), Nyaa (scraper de torrent) e, desde esta mudança, um
cliente BitTorrent **embutido** (`github.com/cenkalti/rain/v2`). Tem WebUI Svelte embutida + API REST.

### Modelo mental da mudança que está sendo revisada

- O torrent baixa para `<SavePath>/<infohash>/<conteúdo>` e **continua semeando lá**.
- Um `Librarian` (`src/internal/files/librarian.go`) cria **hardlinks** dos arquivos de vídeo em
  `<CompletedAnimePath>/<AnimeSanitizado>/`, com nome no formato Jellyfin (`Anime - E05.mkv`)
  quando é episódio único. O arquivo semeado **nunca** é renomeado (quebraria o seeding).
- Hardlink exige mesmo volume. **Não há fallback de cópia** — isso é intencional (plano §4),
  não "conserte" isso.
- Apagar um episódio apaga **os dois links** (hardlink da biblioteca + torrent semeado), senão
  o espaço não é liberado. Há uma **guarda de batch**: vários episódios podem compartilhar um
  `EpisodeHash`; o torrent só é removido quando nenhum episódio salvo o referencia mais.
- `downloaded_episodes` (JSONL em `~/.autoAnimeDownloader/`, sem extensão) é a lista durável de
  episódios baixados, indexada por `EpisodeID` (ID do airing node do AniList). O campo
  `EpisodeHash` (= infohash) é o join com o cliente de torrent.
- A conclusão do download virou **event-driven**: um listener por torrent
  (`torrents/session.go`) chama `OnComplete(hash)` → o daemon enfileira `JobOrganize(hash)` →
  `organizeTorrent` (`daemon/jobs.go`) faz o hardlink e dispara o webhook `download_completed`.
  `LibraryPaths` vazio no registro do episódio é o marcador de "ainda não organizado".

### Regras do repositório (de `CLAUDE.md`)

1. **Rodar `go test ./...`** ao terminar. Não declare pronto sem a saída verde.
2. **Atualizar docs**: mexeu em endpoint, campo de config, env var ou padrão → atualize o doc
   relevante em `docs/agents/`. Padrão não-óbvio → entrada em `docs/agents/decisions.md`.
3. **Regenerar Swagger** se a API mudou: `swag init -g src/cmd/daemon/main.go -o docs/swagger`.
4. **NUNCA commitar ou dar push.** O usuário faz isso.

### Docs úteis para navegar

- `docs/agents/architecture.md` — mapa de pacotes, símbolos, fluxo de dados, endpoints
- `docs/agents/conventions.md` — tratamento de erro, padrão de handler, nomenclatura
- `docs/agents/decisions.md` — escolhas arquiteturais intencionais; **leia antes de "consertar"
  algo que parece estranho**
- `docs/agents/testing.md` — padrões de mock

---

# WP1 — Persistência de episódios e propagação de erro

**Arquivos:** `src/internal/daemon/episodes.go`, `src/internal/files/filemanager.go`,
`src/internal/api/endpoint_episode_actions.go` (+ testes correspondentes em
`src/tests/unit/` e `src/internal/api/`).

**Severidade:** 1 crítico, 2 importantes.

---

## P1.1 — [CRÍTICO] Episódios migrados são organizados como `Anime - E00.mkv`

### O problema

`EpisodeNumber` é um campo **novo** em `EpisodeStruct` (`files/filemanager.go`, struct por volta
da linha 22). Todo registro pré-existente em `downloaded_episodes` desserializa com
`EpisodeNumber == 0`, e **nada faz backfill** — o bloco de migração em `LoadSavedEpisodes`
(`filemanager.go` ~283-300) só corrige `DownloadDate`.

O caminho de upgrade padrão (previsto no plano §8) leva exatamente a isso:

1. Usuário atualiza. A sessão rain nasce vazia (BoltDB novo), então `episodeInTorrents`
   (`daemon/helpers.go` ~84) é falso para todo episódio salvo.
2. `handleAlreadySavedEpisode` (`daemon/episodes.go` ~292-315) devolve `shouldDownload = true`
   com a mensagem "Redownloading episode: was missing from torrents".
3. `processAnimeEpisodes` monta um `EpisodeStruct` novo **com o `EpisodeNumber` correto**
   (`episodes.go` ~99-101) e entrega para `saveEpisodesToFile` (`episodes.go` ~497).
4. `saveEpisodesToFile` chama `FileManager.SaveEpisodesToFile`, que **deduplica por `EpisodeID`
   e descarta a atualização inteira** (`filemanager.go:320-332` — monta `existingMap` e só
   apenda o que não existe). O registro velho sobrevive com `EpisodeNumber: 0`.
5. O torrent conclui → `organizeTorrent` casa por hash → `req.EpisodeNumber = &0`
   (`daemon/jobs.go` ~365) → destino `My Anime - E00.mkv`.

### Por que isso é crítico e não cosmético

Combinado com P2.1 (WP2): **todo** episódio daquele anime resolve para o mesmo destino
`Anime - E00.mkv`. O `Organize` atual vê que o arquivo existe, considera "já linkado", grava
`LibraryPaths` apontando para o arquivo do episódio 1 e dispara o webhook de conclusão. Os
episódios 2..N **nunca chegam à biblioteca e o registro mente dizendo que chegaram**. Pior:
apagar o episódio 1 depois remove o arquivo que todos os registros referenciam.

### A solução

`saveEpisodesToFile` (`daemon/episodes.go` ~490-500) deve fazer **merge seletivo** em vez de
"apenda só o que é novo":

- Registro **novo** (EpisodeID não existe): apenda como hoje.
- Registro **existente**: atualiza `EpisodeHash`, `EpisodeNumber`, `IsBatch`, `EpisodeName`,
  `AnimeName`, `AnimeID`, `AnimeTotalEpisodes`; **reseta `LibraryPaths` para nil** (é um arquivo
  novo, precisa ser organizado de novo); **preserva `ManuallyManaged`** (flag do usuário, o loop
  automático não pode apagar); `DownloadDate` pode ser atualizada (o episódio foi de fato
  re-baixado — ela só alimenta exibição em `api/endpoint_animes.go` ~107, não a retenção).

**Não** troque simplesmente `SaveEpisodesToFile` por `UpsertEpisodes`: `UpsertEpisodes`
(`filemanager.go:345`) faz **substituição total do registro** (`filemanager.go:366-370`), o que
clobbaria `ManuallyManaged`. Ele existe para o write-back de `LibraryPaths` no `organizeTorrent`
e deve continuar servindo a esse propósito.

Implemente o merge onde fizer mais sentido — seja um novo método no `FileManager` (adicionado à
`FileManagerInterface` em `daemon/helpers.go` **e** `api/server.go`, que declaram a mesma
interface), seja em `saveEpisodesToFile` usando `LoadSavedEpisodes` + `UpsertEpisodes` que já
estão nas duas interfaces. A segunda opção evita mexer em interface.

**Defesa em profundidade (obrigatória):** mesmo com o merge, adicione a guarda de
`EpisodeNumber == 0` descrita em **P2.3 (WP2)** — nenhum caminho deve gerar `E00`.

### Critérios de aceite

- [ ] Teste: registro salvo com `EpisodeNumber: 0` + re-download com `EpisodeNumber: 5` →
      o registro persistido tem `EpisodeNumber: 5` e o `EpisodeHash` novo.
- [ ] Teste: `ManuallyManaged: true` sobrevive a um re-download automático.
- [ ] Teste: `LibraryPaths` é zerado no re-download (senão `organizeTorrent` acha que já
      organizou e nunca cria o link novo).
- [ ] Teste: episódio genuinamente novo continua sendo apendado (não regride o comportamento atual).

---

## P1.2 — [IMPORTANTE] Hash desatualizado quebra o join do `JobOrganize`

Mesma causa raiz e mesma correção que P1.1 — listado à parte porque é uma **segunda** falha,
independente do nome do arquivo.

No desenho antigo, `JobRenameFile`/`JobNotifyOnComplete` carregavam o hash no payload do job,
então um registro salvo desatualizado era inofensivo. O `JobOrganize` novo casa
**episódios salvos → torrent por hash** (`daemon/jobs.go` ~330-341).

Se o loop re-baixa um episódio e o Nyaa devolve um release **diferente** (hash diferente), o
hash novo é descartado pelo dedupe. `organizeTorrent` não acha nenhum episódio com aquele hash,
cai no ramo "no saved episode matches hash yet, retrying" e retenta até
`maxRetriesOrganize = 20` (`jobs.go:27`) com backoff de 30s·2ⁿ — cerca de 2,5h — e desiste. O
episódio nunca chega à biblioteca e o loop o re-adiciona em todo passe, para sempre.

O merge seletivo de P1.1 resolve. Adicione um teste específico: hash salvo `aaa...`, re-download
devolve `bbb...` → o registro persistido tem `bbb...`.

---

## P1.3 — [IMPORTANTE] `DELETE` e redownload retornam 200 mesmo quando a deleção falha

### O problema

`RemoveEpisodesWithLinks` (`daemon/episodes.go:351`) é declarada **sem retorno de erro**. Dentro
dela, a falha de `LoadSavedEpisodes` só loga e retorna (`episodes.go:356-359`), e a falha de
`DeleteEpisodesFromFile` também só loga (`episodes.go` ~461-463).

Os handlers chamam assim, sem checar nada:

- `api/endpoint_episode_actions.go:135` — delete manual
- `api/endpoint_episode_actions.go:248` — redownload
- `api/endpoint_episode_actions.go:344` — replace episode
- `api/endpoint_episode_actions.go:425` — replace anime

O código anterior devolvia HTTP 500 nesse caso:

```go
if err := server.FileManager.DeleteEpisodesFromFile([]int{episodeId}); err != nil {
    JSONInternalError(w, err); return
}
```

O redownload é o pior dos quatro: se a remoção do registro falha, o handler **não aborta** —
segue para `ManualDownloadEpisode` e adiciona um torrent novo, enquanto o registro velho (com o
hash velho e `LibraryPaths` velho) permanece. O torrent novo fica sem rastreio.

### A solução

Dê a `RemoveEpisodesWithLinks` um retorno `error`, propagando **no mínimo** a falha de
`LoadSavedEpisodes` e a de `DeleteEpisodesFromFile`. Falhas de `RemoveFromLibrary` e de
`backend.Remove` podem continuar como log best-effort — o objetivo delas é liberar espaço, e uma
falha ali não deve impedir o registro de sair do arquivo.

Nos quatro call sites, trate o erro com `JSONInternalError` seguindo o padrão de handler descrito
em `docs/agents/conventions.md`. Nos caminhos de redownload/replace, **aborte antes** de adicionar
o torrent novo.

Atenção: `removeEpisodesAndLinks` (a função interna, `episodes.go` ~440-480) também é chamada
pelo loop automático (`deleteEpisodesByStatus`, `handleSavedEpisodes`). Lá o comportamento
best-effort com log está correto — não faça o loop automático abortar por causa disso.

### Critérios de aceite

- [ ] Teste de API: `DeleteEpisodesFromFile` falhando → `DELETE /animes/{id}/episodes/{epId}`
      devolve 500.
- [ ] Teste de API: no redownload, falha na remoção → 500 **e nenhum torrent adicionado**
      (verifique via `FakeBackend`).
- [ ] O loop automático continua tolerante a falha (não aborta o passe inteiro).

---

# WP2 — Librarian (hardlink)

**Arquivos:** `src/internal/files/librarian.go`, `src/internal/files/librarian_test.go`.

**Severidade:** 1 crítico, 1 importante.

---

## P2.1 — [CRÍTICO] A idempotência do `Organize` não distingue "já linkado" de "outro arquivo com o mesmo nome"

### O problema

`librarian.go:134-138`:

```go
if _, statErr := o.fs.Stat(dest); statErr == nil {
    // Idempotent: already linked.
    created = append(created, dest)
    continue
}
```

`Stat(dest) == nil → pula` assume que qualquer arquivo naquele caminho é o nosso hardlink. Isso é
o que transforma P1.1 em **perda silenciosa** em vez de erro barulhento. E é errado por conta
própria no fluxo de replace/redownload: se o destino existe mas aponta para bytes diferentes
(inode diferente), o arquivo do torrent novo **nunca é linkado** e o chamador recebe sucesso.

`TestOrganizeIdempotent` (`librarian_test.go` ~123) só exercita o caminho mesmo-arquivo — ele
**passaria mesmo com essa lógica quebrada**.

### A solução (decidida pelo usuário: substituir)

Quando `dest` existe, compare identidade com `src`:

- **Mesmo inode** (`os.SameFile` sobre os dois `FileInfo`) → no-op verdadeiro, apenda a `created`
  e continua. Este é o caso de reconciliação/retry.
- **Inode diferente** → **substitui**: remove o destino e cria o hardlink novo. É o que o fluxo de
  redownload/replace-episode quer, porque o usuário pediu a troca explicitamente. Logue em nível
  `Info` quando substituir, com origem e destino, para o comportamento ser auditável.

`os.SameFile` funciona sobre `FileInfo` de ambos os lados e é portátil (usa inode no Unix e o
índice de arquivo no Windows). Note que a interface `FileSystem` (`files/filesystem.go`) é
injetável — o `Stat` já passa por `o.fs`. Mantenha a comparação passando pela interface para os
testes continuarem podendo injetar um FS falso; se `MockFileSystem` (`src/tests/unit/mock_filesystem.go`)
não devolver `FileInfo` com identidade utilizável, faça o teste de substituição como teste de
integração com `t.TempDir()` e `OSFileSystem` real, que é o padrão já usado em `librarian_test.go`.

### Critérios de aceite

- [ ] Teste: destino existe e é **o mesmo arquivo** (hardlink já criado) → no-op, sem erro,
      `created` contém o caminho, link count não muda.
- [ ] Teste: destino existe e é um arquivo **diferente** → destino passa a apontar para o `src`
      (verifique com `os.SameFile`), conteúdo bate com o da origem.
- [ ] Teste: origem continua intacta e `Nlink == 2` depois da substituição (prova que o seeding
      não foi quebrado).

---

## P2.2 — [IMPORTANTE] `CompletedAnimePath` vazio faz hardlink no diretório de trabalho

`librarian.go:107`:

```go
destDir := filepath.Join(req.CompletedPath, sanitizeFolderName(req.AnimeName))
```

Com `CompletedPath == ""`, `filepath.Join` devolve um caminho **relativo** — o `MkdirAll` logo
abaixo cria `./My Anime/` no CWD do processo e os hardlinks vão parar lá.

Isso é alcançável: a fila de jobs roda sem gate de `isConfigComplete` (`daemon/jobs.go` ~163-171),
e `ManualDownloadEpisode` chama `Ensure(configs.SavePath)` sem exigir `CompletedAnimePath`.

**Solução:** no início de `Organize`, se `req.CompletedPath == ""`, retorne erro explícito
("completed anime path is not configured"). O chamador `organizeTorrent` (`daemon/jobs.go` ~370)
já trata erro retornando `false` (retry com backoff), o que é o comportamento certo — a config
pode ser preenchida antes do retry estourar.

Faça o mesmo em `ProbePaths` (`librarian.go:167`) se ele ainda não rejeita string vazia.

- [ ] Teste: `Organize` com `CompletedPath: ""` retorna erro e **não cria nada** no CWD.

---

## P2.3 — [IMPORTANTE] Guarda contra `E00` (defesa em profundidade para P1.1)

`librarian.go` ~120:

```go
useJellyfin := !req.IsBatch && req.RenameJellyfin && req.EpisodeNumber != nil && len(videoFiles) == 1
```

Adicione `*req.EpisodeNumber > 0` à condição. Episódio 0 não existe na numeração do AniList, então
`EpisodeNumber == 0` significa inequivocamente "dado ausente" — e nesse caso o certo é cair no
nome bruto do arquivo (que é único por release) em vez de gerar um `E00` que colide entre todos os
episódios do anime.

O WP1 corrige a causa; esta guarda garante que nenhum outro caminho reintroduza o problema.

- [ ] Teste: `EpisodeNumber: 0` + `RenameJellyfin: true` → o hardlink usa o nome original do
      arquivo, não `Anime - E00.ext`.

---

# WP3 — Ciclo de vida da sessão e gate de config

**Arquivos:** `src/cmd/daemon/main.go`, `src/internal/daemon/verification.go`,
`src/internal/daemon/helpers.go`, `src/internal/torrents/sessionmanager.go`,
`src/internal/torrents/sessionmanager_test.go` (novo).

**Severidade:** 3 importantes.

---

## P3.1 — [IMPORTANTE] Torrent parado com erro fica morto para sempre

### O problema

`cmd/daemon/main.go` ~270-272: o callback `onFailed` **só loga**.

Quando o rain para um torrent com erro, ele o deixa em estado **Stopped dentro da sessão** e
**não reinicia sozinho**. A goroutine do listener sai depois do case `NotifyStop`
(`torrents/session.go` ~134-142) e não é re-armada. Consequência em cascata: `episodeInTorrents`
(`daemon/helpers.go` ~84) continua enxergando o hash na sessão, então o daemon **acha que o
episódio foi baixado**. Ele nunca retenta, nunca organiza, nunca notifica.

Isso também é desvio do plano §3, que especificava `Notify(DownloadFailed)` nesse caminho.

### A solução (decidida pelo usuário)

No `onFailed(hash, err)`:

1. Dispare o webhook `DownloadFailed` via `notifications.Notify` (siga a assinatura usada em
   `daemon/jobs.go` no `organizeTorrent` para o `DownloadCompleted`; o nome do anime sai do
   registro salvo casado por hash).
2. Chame `Remove(hash, false)` para tirar o torrent da sessão.

Com o torrent fora da sessão, `episodeInTorrents` passa a ser falso no próximo passe e o loop
re-adiciona o magnet — retry automático, reaproveitando a máquina que já existe.

**Risco aceito, documente mas não implemente:** um torrent genuinamente morto (sem seeds) entra em
ciclo de re-add a cada passe de verificação, porque `EpisodeRetryLimit` reinicia a cada passe. A
correção seria uma blacklist por hash com TTL. Registre isso como entrada em
`docs/agents/decisions.md` em vez de implementar especulativamente.

- [ ] Teste: `FakeBackend` dispara falha → webhook `download_failed` disparado **e** o hash sai
      da sessão.

---

## P3.2 — [IMPORTANTE] Parar o loop pela UI para o seeding inteiro

`daemon/verification.go:54` é o único lugar que chama `backend.Ensure()` fora dos handlers de
download manual. Ou seja, a sessão só nasce no primeiro passe de verificação.

O plano §2 previa criação no startup (`main.go`). Do jeito que está, parar o loop pela WebUI
**para o seeding** e pula a reconciliação de startup — mudança de comportamento em relação ao
qBittorrent externo, que seguia semeando independentemente. Relevante para quem usa tracker
privado com exigência de ratio.

### A solução (decidida pelo usuário: seeding continua)

Chame `Ensure` uma vez no `main.go`, no startup, **quando a config estiver completa**, junto da
criação da `JobQueue`. Preserve o comportamento preguiçoso para o caso de config incompleta:
se `SavePath` estiver vazio no boot, a sessão não é criada, e o `Ensure` do passe de verificação
continua sendo o caminho que a cria quando a config for salva depois.

Mantenha a reconciliação de startup onde ela está hoje (no passe de verificação) — ela re-roda
como rede de segurança e isso é um desvio **bom** do plano. Só garanta que ela roda ao menos uma
vez depois da sessão nascer no startup.

Não mexa no bloco de shutdown (`main.go` ~278-283): a ordem LIFO ali (`jobQueue.Stop()` drena,
depois `torrentManager.Close()` faz flush do bbolt) está correta e comentada.

- [ ] Sessão existe e semeia com o loop do daemon parado.
- [ ] Boot com config incompleta não cria sessão e não quebra (comportamento atual preservado).
- [ ] Entrada em `docs/agents/decisions.md` explicando que seeding é independente do loop.

---

## P3.3 — [IMPORTANTE] A restrição de mesmo-volume só é verificada no `PUT /config`

### O problema

`ProbePaths` tem **exatamente um chamador em produção**: `api/endpoint_config.go:100`.
(Confira: `grep -rn "ProbePaths" src/ --include="*.go" | grep -v _test.go`.)

Considere um usuário pré-upgrade que configurou `completed_anime_path` num volume **diferente** do
`save_path`. Isso era perfeitamente legal antes — o comportamento antigo era **mover** o arquivo,
o que funciona entre dispositivos. Depois do upgrade:

- `isConfigComplete` (`daemon/helpers.go` ~44) passa, porque só checa que os campos não estão vazios.
- O daemon inicia, baixa normalmente, e **todo** `JobOrganize` falha com `EXDEV`.
- `daemon/jobs.go` ~370 loga `Warn "Organize: failed to hardlink into library"` e devolve `false`;
  o job retenta 20 vezes com backoff e é descartado.
- Nada chama `state.SetLastCheckError`. **A UI mostra um daemon saudável e a biblioteca fica
  vazia para sempre.**

O mesmo furo vale para `docker/entrypoint.sh` (~26-40), que escreve `config.json` diretamente a
partir de env vars (e reescreve com `FORCE_CONFIG_UPDATE`), sem nunca passar pelo endpoint.

Isso contradiz o invariante declarado no plano §3: *"Falha EXDEV não deve acontecer em runtime
porque a sonda no save já rejeitou caminhos cross-device"* — verdadeiro só para configs salvas
pela API depois do upgrade, que é exatamente a população que o guia de migração **não** cobre.

### A solução

Rode `ProbePaths` no gate de `isConfigComplete` (`daemon/verification.go` ~32-43), e em caso de
falha chame `state.SetLastCheckError(...)` com a mesma mensagem que o endpoint devolve, para a
WebUI mostrar o erro acionável. Em caso de falha, **não siga com o passe** — baixar sem conseguir
organizar só enche o disco.

Custo: uma escrita de arquivo + um link + duas remoções por passe. Se preferir, faça cache do
resultado por par de caminhos e reavalie só quando a config mudar — mas a versão simples já é
barata o bastante.

- [ ] Teste: config com `SavePath` e `CompletedAnimePath` em volumes diferentes (injete um `link`
      que devolve `EXDEV`) → o passe não roda e `LastCheckError` fica preenchido.
- [ ] Teste: caminhos válidos → passe roda normalmente.

---

## P3.4 — [MENOR] Sem nenhum teste para `SessionManager`

`torrents/sessionmanager.go` não tem teste nenhum. Crie `sessionmanager_test.go` cobrindo:
criação preguiçosa (`Ensure` com `SavePath` vazio), recriação ao trocar `SavePath`,
`ErrSessionNotReady`, e `Close`.

Aproveite para avaliar `sessionmanager.go` ~80-92: `current()` libera `m.mu` antes de delegar a
chamada, então um `Ensure` concorrente pode fechar a sessão entre a leitura e o `s.Add(...)`.
O `Close()` do rain zera o mapa interno de torrents, então um panic de mapa nil é teoricamente
alcançável. Segurar o lock durante a chamada delegada (ou usar `RWMutex` com read lock) fecha a
janela.

---

# WP4 — Limpeza e documentação

**Arquivos:** `docs/**`, `.gitignore`, `src/internal/frontend/messages/*.json`,
`.github/workflows/build.yml`, e `gofmt` nos arquivos listados.

**Severidade:** todos menores. Nenhum muda comportamento — pode ir num commit só.

| # | Onde | O quê |
|---|---|---|
| P4.1 | `frontend/messages/en.json:81` e `pt-BR.json:81` | Chave órfã **nova**: `config_label_use_completed_path`. O checkbox "usar pasta separada" foi removido do `Config.svelte`; a chave ficou nos dois arquivos, referenciada por ninguém. Remova de ambos. (Outras chaves não-referenciadas — `config_btn_reload`, `detail_btn_*`, `status_col_episodes` — já eram órfãs antes desta mudança; **não** são escopo daqui.) |
| P4.2 | `.gitignore:20` | Ainda whitelista `!runtime/qbittorrent-config/qBittorrent/qBittorrent.conf`, caminho que não pode mais existir. É a última string "qbittorrent" fora de docs/testes/comentários. Remova a linha. |
| P4.3 | `docs/agents/architecture.md` (~112 e ~126) | Duas assinaturas desatualizadas: `AnimeVerification` agora recebe também `backend torrents.TorrentBackend, librarian files.Librarian` (ver `daemon/verification.go:23`); `ManualDownloadEpisode` agora recebe o backend como primeiro argumento (ver `api/endpoint_episode_actions.go:63`). Corrija conferindo as assinaturas reais. |
| P4.4 | `docs/agents/architecture.md` | A nota sobre caminhos no Windows diz que tudo fica em `%APPDATA%\AutoAnimeDownloader\` "exceto `session.db`, em `%APPDATA%\.autoAnimeDownloader\`". Está errado: **todos** os arquivos de estado usam a pasta com ponto — `getJobsFilePath` e `getPIDFilePath` (`cmd/daemon/main.go`) e `configsFolder = ".autoAnimeDownloader"` (`files/filemanager.go:16`). Não existe variante sem ponto. Afirme que tudo fica em `%APPDATA%\.autoAnimeDownloader\` e remova a cláusula de exceção. |
| P4.5 | `docs/guides/migration-embedded-torrent.md:60-63` | A seção diz que os webhooks estão inalterados. A **chave** do evento está mesmo inalterada (`download_completed`) — isso está certo e bem apurado. Mas o valor de `{{reason}}` do `download_failed` mudou de `"qBittorrent rejeitou o torrent"` para `"torrent rejeitado"` (`notifications/notifications.go:25`). Quem faz match de string em `{{reason}}` vê mudança. Adicione um bullet. |
| P4.6 | `.github/workflows/build.yml:26-30` | O passo de build do frontend na perna Windows roda sob `pwsh`, que só propaga o exit code do **último** comando: um `bun install --frozen-lockfile` falhando não reprova o passo. Adicione `shell: bash` (disponível no `windows-latest`) para as duas pernas da matriz terem a mesma semântica de falha. |
| P4.7 | `cmd/daemon/main.go`, `daemon/episodes.go`, `daemon/orchestration_test.go` | Fora do `gofmt` (as versões em HEAD estavam limpas): ordem de import, linha em branco no fim, alinhamento de campo. Rode `gofmt -w` nos três. **Coordene com quem estiver nos WP1/WP3**, que também editam dois deles — rode este item por último, ou deixe cada WP formatar o que tocou. |
| P4.8 | `torrents/backend.go:20-21` | Comentário desatualizado: diz que "a Session e o FakeBackend implementam" a interface, mas `Session` não tem `Ensure` — quem implementa é o `SessionManager`. Corrija o comentário. |
| P4.9 | `daemon/episodes.go:29` | Parâmetro morto: `jobQueue *JobQueue` em `processAnimeEpisodes` não é mais usado. Remova (coordene com WP1, mesmo arquivo). |
| P4.10 | `torrents/session.go` ~160-193 | `parseInfoHash` diverge do parser do rain apesar do comentário dizer que o espelha: pega só `xts[0]`, então um magnet híbrido v1/v2 cujo primeiro `xt` é `urn:btmh:` é rejeitado antes de chegar no rain (que suporta `btmh`); e aplica `strings.ToUpper` no base32, que o rain não faz. Itere todos os `xt` preferindo `urn:btih:`, e suavize o comentário. **CORREÇÃO (aplicada):** a premissa "o rain suporta btmh" não se sustenta ponta a ponta — em `rain/v2@v2.3.1/internal/magnet/magnet.go` o `New` também usa só `xts[0]` (linha 45) e o ramo `urn:btmh:` (linhas 138-146) exige `len(b) == 20`, enquanto um multihash v2 real tem 34 bytes. Ou seja, o `AddURI` do rain rejeita qualquer v2 real de todo jeito. A mudança vale como robustez do nosso parser (o magnet btmh-primeiro deixa de morrer com `"invalid magnet"` nosso), mas **não** entrega download de magnet híbrido; isso exigiria reescrever o magnet antes do `AddURI`. O `strings.ToUpper` foi **removido** em vez de mantido: normalizar aceitaria magnets que o `AddURI` rejeitaria depois, com o risco pior de ID ≠ `InfoHash()`. |
| P4.11 | `torrents/session.go:56` | Detecção de duplicata por `strings.Contains(err.Error(), "duplicate torrent id")`. Funciona hoje (o literal existe no rain v2.3.1), mas quebra silenciosamente em qualquer upgrade da lib. Um pré-check com `Get(hash)` seria mais robusto. |
| P4.12 | `torrents/fakebackend.go:78` | `FakeBackend.Remove` devolve erro para hash inexistente, enquanto o `Session.Remove` real devolve `nil` (o `RemoveTorrent` do rain devolve `(nil, nil)` para id desconhecido). Um teste que assertar nesse erro não reflete produção. Alinhe o fake ao real. |

**Não faça:** a corrida do `-race` em `src/tests/unit/daemon_test.go:171` (buffer de log compartilhado
entre goroutines paralelas de fetch) é **pré-existente**, está no harness de teste e **não bloqueia
a CI** — `.github/workflows/build.yml` roda `go test` sem `-race` (linhas 65, 83, 136). Fica como
melhoria opcional, fora do escopo deste review.

---

# WP5 — Lacunas de teste de orquestração

**Arquivos:** `src/internal/daemon/orchestration_test.go`,
`src/internal/torrents/session_internal_test.go`.

Todas são lacunas contra o plano §7. Nenhuma exige mudança em código de produção — se algum desses
testes falhar, o bug correspondente já está descrito nos WPs acima.

## P5.1 — Callback do fake nunca chega até a execução do job

`FakeBackend.CompleteTorrent` dispara `onComplete`, mas o único assinante é uma closure local em
`TestFakeBackendLifecycle` (`session_internal_test.go` ~76). Nenhum teste liga a ponta a ponta.

O plano §7 pede explicitamente: *"fake dispara complete → JobOrganize enfileirado **e executado**"*.
Escreva esse teste: registre o callback real do daemon, dispare a conclusão no fake, e verifique
que `JobOrganize` foi enfileirado **e** que `processDueJobs` o executou (hardlink criado,
`LibraryPaths` gravado, webhook disparado).

## P5.2 — "Apaga AMBOS os links" nunca é testado com arquivos reais

`TestRemoveEpisodesAndLinks_BatchGuard` (`orchestration_test.go` ~114) nunca popula `LibraryPaths`,
então metade do plano §5 fica sem cobertura. Adicione uma variante com hardlink de verdade em disco
(`t.TempDir()` + `OSFileSystem`) afirmando que:

- na deleção completa, o arquivo da biblioteca **some** e o torrent é removido;
- com um irmão de batch vivo, o hardlink daquele episódio some mas o torrent **permanece**.

> **CORREÇÃO (aplicada) — o segundo bullet estava errado sobre o comportamento atual.** Para
> `IsBatch: true`, `removeEpisodesAndLinks` (`episodes.go` ~444-448) **pula** a remoção da
> biblioteca de propósito enquanto o torrent sobrevive — é a "batch-hygiene deletion limitation"
> já documentada em `decisions.md` #22. O ramo "link some, torrent fica" só é alcançável com
> episódios **não-batch** compartilhando o mesmo hash. `TestRemoveEpisodesAndLinks_RealHardlinks`
> cobre as duas formas separadamente (subteste 2 = não-batch compartilhando hash; subteste 3 =
> batch, torrent **e** arquivos preservados). Nenhum código de produção foi alterado por causa
> disso — não "conserte" o batch para casar com o bullet original.

## P5.3 — Reconciliação de startup

Confirme que a reconciliação enfileira `JobOrganize` para torrents em `Seeding` sem
`LibraryPaths`, e que **não** enfileira para os que já têm.

Nota: a reconciliação usa `LibraryPaths` vazio como marcador (`daemon/verification.go` ~284-290)
em vez de "o hardlink existe em disco", como o plano §2 dizia. Isso é um desvio **intencional e
melhor** (não recria arquivos que o usuário apagou de propósito) — teste o comportamento atual, não
o do plano.

---

# Apêndice — Achados verificados e conscientemente **não** tratados

Não "conserte" estes. Estão aqui para evitar retrabalho e para o revisor seguinte não reabrir.

1. **Sem fallback de cópia no cross-device** (`librarian.go` ~140-146) — intencional, plano §4.
2. **Link parcial em batch**: se o link do arquivo 1 sucede e o do 2 falha, o link 1 permanece
   (`cleanupIfEmpty`, `librarian.go` ~231-237, só remove diretório vazio). Auto-cura por
   idempotência no retry. Na prática o `EXDEV` falha já no arquivo 1. Risco aceito.
3. **`LibraryPaths` replicado em todo episódio do batch** (`jobs.go` ~377-379): um batch de 12
   episódios guarda 12 cópias de 12 caminhos, e `RemoveFromLibrary` é chamada 144 vezes na deleção
   total. Correto, só verboso.
4. **`RemoveFromLibrary(path)` em vez de `RemoveFromLibrary(animeName, episodeNumber)`** — desvio
   do plano §4 e **melhoria**: usa os caminhos gravados em vez de recalcular.
5. **Jellyfin naming exige `len(videoFiles) == 1`** (`librarian.go` ~120): um torrent de episódio
   único que traga NCOP/NCED junto fica com nomes brutos em tudo. Aceito.
6. **Rename do evento `QBittorrentDownloadCompleted` → `DownloadCompleted` NÃO quebra webhooks.**
   O plano §9.8 errou ao afirmar que sim: `eventString()` devolve `"download_completed"` antes e
   depois (`notifications/notifications.go:36`). Já está corretamente documentado em
   `decisions.md` e no guia de migração. Não reintroduza um aviso de quebra.
7. **A premissa do plano de que `NotifyComplete` é one-shot e precisa ser re-armado é conservadora
   demais** — o rain **fecha** o canal (`torrent_pieces.go:25`), então é broadcast repetível e um
   listener armado depois da conclusão dispara na hora. A implementação está correta de qualquer
   forma; a janela entre `Add` e `armListener` (`session.go` ~64) não perde eventos.
8. **Corrida do `-race` em `daemon_test.go:171`** — pré-existente, no harness, e a CI não roda
   `-race`. Ver nota no fim do WP4.
9. **Mensagem de erro de cross-device chega em inglês para usuário pt-BR**
   (`endpoint_config.go:101` repassa `err.Error()`). É o padrão pré-existente de todas as
   validações de servidor. Corrigir de verdade exigiria mapear códigos de erro para chaves i18n —
   melhoria separada, não escopo deste review.
10. **`ProbePaths` roda antes das validações numéricas baratas** (`endpoint_config.go` ~99-105) e
    tem efeito colateral em disco (`MkdirAll` nos dois caminhos). Um PUT com caminhos válidos mas
    `check_interval: 0` cria as duas árvores de diretório antes de devolver 400. A limpeza em si
    está correta (`defer` remove a origem em todos os caminhos). Mover a sonda para depois das
    validações numéricas é polimento opcional.
