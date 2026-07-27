# Pasta única de download — Design

Remove `save_path` da configuração do usuário e deriva o diretório de download da biblioteca:
`<completed_anime_path>/.autoAnimeDownloader`. Inclui a migração dos dados de quem já tem
`save_path` configurado.

**Motivação:** ter dois caminhos obrigatórios ("Pasta de Download" e "Pasta de animes completos")
é confuso — pro usuário só importa onde a biblioteca fica. Pior, os dois precisam estar no mesmo
volume por causa dos hardlinks (`decisions.md #21`), uma restrição que hoje só aparece como erro
no momento de salvar. Derivando o caminho, a restrição deixa de ser possível de violar.

Este é o **spec A** de três. Ver "Próximos specs" no fim.

---

## 1. O modelo

A biblioteca (`completed_anime_path`) passa a ser o único caminho que o usuário escolhe.
O diretório de download é `filepath.Join(CompletedAnimePath, ".autoAnimeDownloader")` —
calculado a cada uso, nunca armazenado.

### `Config`

`internal/files/filemanager.go`:

- `SavePath` vira campo **legado**: `json:"save_path,omitempty"`, comentado como lido apenas
  pela migração. Depois de migrar, fica `""` e some do `config.json` pelo `omitempty`.
- Novo método `Config.DownloadPath() string`:
  - `CompletedAnimePath == ""` → retorna `""`. **Essa guarda é obrigatória**: sem ela,
    `filepath.Join` produziria o caminho relativo `.autoAnimeDownloader` e a sessão da rain
    seria criada no diretório de trabalho do processo. Com `""`, `SessionManager.Ensure`
    continua devolvendo `ErrSessionNotReady`, que é o comportamento atual para config incompleta.
  - senão → `filepath.Join(CompletedAnimePath, ".autoAnimeDownloader")`.

Todos os pontos que hoje leem `configs.SavePath` passam a chamar `configs.DownloadPath()`:
`daemon/verification.go:72` e `daemon/manual_download.go:51,86,130`. Dois pontos deixam de
precisar do caminho: `api/endpoint_status.go:38` passa a medir disco na biblioteca (§4) e
`DeleteEmptyFolders` perde um argumento (§3).

`SessionManager.Ensure` **não muda** — continua recebendo um caminho e não precisa saber que
ele virou derivado.

### Validação

- `isConfigComplete` (`daemon/helpers.go:91`) perde o teste de `SavePath`:
  `len(AnilistUsernames) > 0 && CompletedAnimePath != ""`.
- `PUT /config` (`api/endpoint_config.go:87`) perde o bloco `"Save path is required"`.

### A sonda de hardlink continua existindo

O cheque de volume cruzado virou impossível de falhar por construção, mas ainda há sistemas de
arquivos que **não suportam hardlink nenhum**: exFAT, FAT32, alguns mounts SMB/NFS. A sonda
permanece, com um argumento a menos.

`Librarian.ProbePaths(savePath, completedPath) error` → `Librarian.ProbePath(completedPath) error`:

1. `MkdirAll` da biblioteca.
2. `MkdirAll` do `.autoAnimeDownloader` dentro dela.
3. Cria o `.ignore` (ver §2) se ainda não existir.
4. Escreve `.aad_link_probe` no diretório de download, linka na biblioteca, apaga os dois.

Mensagem de erro muda de *"save path and completed path are on different volumes"* para
*"this filesystem does not support hardlinks"*.

Os dois chamadores continuam os mesmos: `PUT /config` e o passe de verificação
(`decisions.md #26` segue válido — a justificativa de rodar a cada passe não muda).
As interfaces `Librarian` em `api/server.go` e `daemon/helpers.go` acompanham a assinatura.

## 2. Convivência com o Jellyfin

O diretório de download passa a viver dentro da pasta que o Jellyfin varre, com arquivos
parciais. Duas defesas, aplicadas juntas:

- **Prefixo com ponto** — o scanner do Jellyfin ignora ocultos no Linux. Não cobre Windows,
  onde o ponto não marca oculto.
- **Arquivo `.ignore` vazio** dentro do `.autoAnimeDownloader`, criado por `ProbePath`. O
  Jellyfin honra esse marcador para pular a pasta, independente de plataforma.

## 3. `DeleteEmptyFolders` precisa de uma guarda

`FileManager.DeleteEmptyFolders(savePath, completedAnimeSaveFolder)`
(`internal/files/filemanager.go:440`) varre os dois caminhos apagando diretórios vazios.
Com um dentro do outro, ele passaria a apagar os diretórios `<id>` que a rain acabou de alocar
e ainda não preencheu.

A varredura da biblioteca passa a **pular explicitamente** a entrada `.autoAnimeDownloader`.
Como os dois argumentos agora descrevem caminhos aninhados, a assinatura passa a receber só a
biblioteca: `DeleteEmptyFolders(completedAnimeSaveFolder string) error`. Acompanham a mudança a
única chamada (`daemon/verification.go:263`) e as duas declarações de interface
(`api/server.go:29`, `daemon/helpers.go:24`).

## 4. Card de espaço em disco

`api/endpoint_status.go:38` mede o espaço livre em `cfg.SavePath`. Passa a medir em
`cfg.CompletedAnimePath`. Mesmo volume por construção, mesmo número — some uma dependência.
`decisions.md #19` (disco lido via stat do OS) continua valendo, só muda o caminho.

## 5. Migração

Arquivo novo: `internal/daemon/migration.go`, com
`MigrateSavePath(fs files.FileSystem, fm FileManagerInterface, backend torrents.TorrentBackend) error`.

O `FileSystem` entra explicitamente porque `FileManagerInterface` não expõe o dele e a migração
precisa de `Rename`/`MkdirAll`/`Stat`/`Remove` — todos já na interface
(`internal/files/filesystem.go:8`). Os dois chamadores (`cmd/daemon/main.go` e o passe de
verificação) já têm um `FileSystem` em mãos.

### Quando roda

Dois pontos, os mesmos que já chamam `Ensure` (`decisions.md #25`):

1. **No boot**, em `cmd/daemon/main.go`, imediatamente **antes** de `jobQueue.Start()` e portanto
   antes de `ensureStartupSession`. Rodar antes de `Start()` é deliberado: a sessão temporária
   aberta no caminho antigo pode disparar uma conclusão, e `Start()` carrega a lista de jobs
   persistida **por cima** do que está em memória, descartando o enfileiramento espúrio. A
   reconciliação de startup re-enfileira o que for preciso. É a mesma ordem que `decisions.md #25`
   já fixa.
2. **No topo do passe de verificação**, antes do `backend.Ensure`. Cobre quem bootou com config
   incompleta e só configurou a biblioteca depois.

A função é idempotente; chamá-la nos dois lugares é seguro.

### Algoritmo

```
cfg := fm.LoadConfigs()

se cfg.SavePath == ""                       -> nada a fazer
se cfg.CompletedAnimePath == ""             -> nada a fazer (config incompleta; o passe tenta de novo)
se cfg.SavePath == cfg.DownloadPath()       -> limpa o campo, salva, fim

dest := cfg.DownloadPath()

backend.Ensure(cfg.SavePath)
dataDirs := [t.DataDir for t in backend.List()]
backend.Close()

fs.MkdirAll(dest, 0755)

para cada dir em dataDirs:
    se dir é ancestral de dest, ou igual a dest  -> pula (ver guarda abaixo)
    target := filepath.Join(dest, filepath.Base(dir))
    se fs.Stat(target) tem sucesso               -> pula (re-execução)
    fs.Rename(dir, target)                       -> erro: aborta

cfg.SavePath = ""
fm.SaveConfigs(cfg)
fs.Remove(savePathAntigo)   // só tem efeito se ficou vazio; sobra do usuário fica lá, com log
```

### Por que listar pela sessão em vez de varrer o diretório

Move exatamente as pastas que são torrents e nada que o usuário tenha deixado no `save_path`.
Os IDs da rain são base64-url de UUID (`session_add.go:267`), então dava para filtrar por formato,
mas a sessão é a fonte de verdade e não custa nada — é um open/list/close no boot, uma vez.

Detalhe de `SessionManager`: `Close()` zera `m.session` mas mantém `m.savePath`. O `Ensure`
seguinte, no caminho novo, cai no ramo de criação porque `m.session == nil`. Não há bug nessa
sequência, mas ela precisa continuar assim.

### Por que o rename é seguro

`ProbePaths` sempre exigiu mesmo volume, então **qualquer config que funcionava** tem origem e
destino no mesmo filesystem. `rename` preserva o inode: o torrent continua semeando dos mesmos
bytes e os hardlinks já criados na biblioteca continuam apontando para eles. Nada rebaixa,
nada reverifica.

Isso importa porque a rain resolve o diretório de cada torrent como
`filepath.Join(DataDir, torrentID)` em tempo de execução (`session_storage.go:29`) — o caminho
**não** vai para o resume data. Trocar o `save_path` sem mover os dados faria todo torrent
existente apontar para um diretório vazio, reverificar, achar nada e rebaixar tudo.

### A guarda do ancestral

O default do Docker é `SAVE_PATH=/app/downloads` com `COMPLETED_ANIME_PATH=/app/downloads/completed`
— a biblioteca **dentro** do save path. O destino vira
`/app/downloads/completed/.autoAnimeDownloader`, descendente da origem.

Mover os filhos `<id>` para lá funciona normalmente. A guarda existe para o caso patológico de um
`DataDir` que seja ancestral de (ou igual a) `dest`, que produziria um rename de uma pasta para
dentro dela mesma. Na prática não acontece (os `DataDir` são sempre `<save>/<uuid>`), mas o
layout aninhado do Docker é comum o bastante para não deixar isso implícito.

### Fechando a porta de trás

Se `save_path` puder ser reescrito por qualquer via, a migração re-arma sozinha. Três vedações:

- **`PUT /config` zera `config.SavePath`** antes de persistir. A API nunca consegue reintroduzir
  o campo. Depois disso, a migração só pode ser disparada por um `config.json` escrito por uma
  versão anterior — que é exatamente o caso de uso.
- **`docker/entrypoint.sh`** para de emitir a linha `"save_path"` e perde a variável `SAVE_PATH`.
  Sem isso, um container com `FORCE_CONFIG_UPDATE` reescreveria o campo e rodaria a migração a
  cada boot. `environment.md` documenta `SAVE_PATH` como removida/ignorada.
- **Testes de integração** param de mandar `save_path` no PUT; `TEST_SAVE_PATH` sai de
  `docker/docker-compose.test.yml` e de `environment.md`. Eles reconfiguram um daemon real
  (`decisions.md #23`) e dispararia a migração nele.

### Quando falha

Erro em qualquer passo: loga, grava em `LastCheckError` para aparecer na WebUI, **não** limpa
`save_path`, e **aborta o passe de verificação** — o mesmo tratamento que a sonda de hardlink já
recebe hoje. Seguir em frente com o caminho novo enquanto os dados ficaram no antigo significa
rebaixar tudo em silêncio, que é o desastre que a migração existe para evitar.

## 6. Frontend

- `routes/Config.svelte`: remove o campo "Pasta de Download" e sua validação. O campo restante
  ganha um hint com o caminho derivado em texto morto:
  *"Os downloads em andamento ficam em `<caminho>/.autoAnimeDownloader`"*, atualizado
  reativamente conforme o usuário digita.
- `messages/en.json` e `messages/pt-BR.json`: saem `config_label_save_path`,
  `config_hint_save_path`, `config_val_save_path`. Entra uma chave para o hint derivado.
- `lib/api/client.ts`: o tipo `Config` perde `save_path`.
- `tests/smoke/`: os testes que preenchem o formulário de configuração são ajustados.

## 7. Testes

Núcleo em `internal/daemon/migration_test.go`:

| Teste | O que prova |
|---|---|
| `MovesTorrentDirs` | Diretórios reais em temp + `FakeBackend` reportando `DataDir`s: moveu tudo e `save_path` ficou vazio no config. |
| `PreservesHardlinks` | Arquivo real no save antigo, hardlink na biblioteca, roda a migração, `os.SameFile` continua verdadeiro. É o teste que sustenta "nada rebaixa". Usa `NewOSFileSystem()` sobre `t.TempDir()` — inode e hardlink não sobrevivem a um fs falso. Espelha `TestRemoveEpisodesAndLinks_RealHardlinks` (`daemon/orchestration_test.go`). |
| `Idempotent` | Rodar duas vezes; a segunda é no-op. |
| `SkipsAncestorOfDest` | O layout do Docker (`/app/downloads` + `/app/downloads/completed`). |
| `AbortsOnFailure` | Destino não gravável: `save_path` **permanece** no config e o erro sobe. |

Fora dele:

- `Config.DownloadPath()` com `CompletedAnimePath` vazio → `""`.
- `DeleteEmptyFolders` não apaga `.autoAnimeDownloader`.
- `ProbePath` cria a biblioteca, o diretório de download e o `.ignore`.
- `endpoint_config_test.go`: um PUT sem `save_path` é aceito; um PUT **com** `save_path` persiste
  o campo vazio.

`FakeBackend` (`internal/torrents/fakebackend.go`) ganha um ajuste: hoje `Ensure` é no-op e os
`DataDir` são sintéticos. Passa a devolver `DataDir`s sob o `savePath` que recebeu em `Ensure`,
que é o comportamento real e o que os testes de migração precisam.

## 8. Docs

- **`decisions.md`**: entrada nova sobre o modelo de pasta única. Emenda `#21` (o "mesmo
  filesystem" deixa de ser uma restrição sobre configuração do usuário e vira invariante),
  `#22` e `#26` (a sonda vira de caminho único).
- **`architecture.md`**: `Config.DownloadPath()`, `daemon/migration.go`, assinaturas de
  `ProbePath` e `DeleteEmptyFolders`.
- **`config.md`**: remove `save_path` dos campos configuráveis; documenta o caminho derivado.
- **`environment.md`**: `SAVE_PATH` e `TEST_SAVE_PATH` removidas.
- **Swagger**: regerar (`swag init -g src/cmd/daemon/main.go -o docs/swagger`) — o schema de
  `Config` mudou.

---

## Próximos specs

Decisões já tomadas, para não se perderem:

### B — Tela de downloads (frontend, risco baixo)

- **Deletar torrent** = `Remove(hash, false)` e **nada mais**: sem bloquear o episódio, para que
  o próximo passe rebaixe. Não precisa de lógica nova no backend — é exatamente o que
  `HandleTorrentFailure` (`daemon/helpers.go:75`) já faz, e o passe rebaixa porque `isInTorrents`
  vira falso (`daemon/episodes.go:49`). Se o episódio já estava organizado, o hardlink velho é
  substituído pelo novo (`decisions.md #28`). Só falta o endpoint.
- **Seleção em massa**, **filtros** e **ordenação** na tela `#/downloads`.
- **Progresso inline em `/status/:id`**: na lista de episódios que já existe, os que têm torrent
  ativo ganham barra + velocidade + status. Sem modal e sem endpoint novo — `GET /torrents` já
  devolve tudo, o join é por `EpisodeHash`.

### C — Fila de downloads ativos (backend, risco médio)

A rain **não tem** prioridade, fila nem limite de ativos (verificado no fonte da v2.3.1: não
existe `SetPriority` nem equivalente). Seria 100% nosso. Escopo escolhido, mínimo:

- Config novo: máximo de downloads simultâneos. O excedente fica parado.
- Ordem default FIFO, com um botão "priorizar" por linha na tela de downloads que joga o torrent
  para o topo — um campo de rank persistido, sem drag-and-drop.
- Precisa distinguir "parado pela fila" de "pausado pelo usuário", senão a fila despausa o que o
  usuário pausou.

### Fora de escopo (premissa corrigida)

*"Mover o episódio para a pasta escolhida assim que terminar de baixar"* já é o comportamento
atual. `JobOrganize` é disparado pelo evento de conclusão e roda em ≤5s (`daemon/jobs.go:26`),
criando o **hardlink** em `completed_anime_path` (`files/librarian.go:170`). O episódio aparece na
biblioteca em segundos, não depois de semear. O que fica no diretório de download é o *mesmo
arquivo* — mesmo inode, mesmos bytes, zero espaço extra. Um "mover" de verdade quebraria o seed,
e é precisamente por isso que o design escolheu hardlink (`decisions.md #21`).
