# Limite de downloads simultâneos + fila com priorização

Data: 2026-08-02

## Problema

O daemon adiciona à sessão da rain todos os torrents que um passe de verificação
encontrar. Com uma lista grande de animes em `RELEASING`, um passe pode abrir dezenas de
downloads ao mesmo tempo: a banda se divide, nenhum episódio termina cedo, e o disco
trabalha em N arquivos ao mesmo tempo.

A rain (v2.3.1) **não tem** limite de downloads ativos nem fila nativa — `torrent.Config`
só expõe limites de velocidade, de peers e de tamanho. A fila é nossa.

## Objetivo

1. Limitar quantos torrents **incompletos** ficam ativos ao mesmo tempo, configurável, com
   default razoável.
2. Dar ao usuário um botão para passar um episódio na frente da fila.

Não-objetivo: limitar seeding. Torrents completos seguem semeando sem restrição — é de
graça em banda de download e é o comportamento que a comunidade espera.

## Arquitetura

### Onde a fila vive

`src/internal/torrents/queue.go`, uma struct `queue` sem conhecimento da rain, operando
sobre uma interface mínima do pacote:

```go
type queueOps interface {
	List() []TorrentInfo
	pause(hash string) error  // delegação crua, sem reentrar na fila
	resume(hash string) error
}
```

Os métodos são **não-exportados de propósito**: só implementáveis dentro do pacote
`torrents`, o que impede que a política de fila vaze para a API ou para o daemon.

A `queue` é um campo do **`SessionManager`**, não da `Session`. O `SessionManager` é o
`TorrentBackend` de produção e o dono do ciclo de vida da sessão; pendurar a fila nele faz
o estado sobreviver a uma troca de `save_path` (que destrói e recria a `Session`).

```go
type queue struct {
	mu      sync.Mutex
	limit   int      // 0 = ilimitado
	queued  []string // hashes que a fila pausou, em ordem de promoção
	adopted bool     // a adoção de boot já rodou
}
```

### `enforce(protected string)` — o único ponto de decisão

Toda a política mora aqui. Passos, sob o mutex da `queue`:

1. **Poda**: remove de `queued` os hashes que sumiram da sessão ou já completaram.
2. **Adoção de boot** (só na primeira chamada com sessão viva, guardada por `adopted`):
   todo torrent incompleto e `stopped` que não está em `queued` é anexado ao fim da fila.
   Sem isso, os torrents que a fila pausou antes de um restart nunca voltariam — a rain
   persiste o estado parado, a nossa fila não.
3. **Excesso**: `ativos` = incompletos com status ≠ `stopped`/`stopping`. Enquanto
   `len(ativos) > limit`, pausa o ativo de **menor progresso** e o insere na **frente** de
   `queued`. O hash `protected` nunca é escolhido.
4. **Folga**: enquanto `len(ativos) < limit` e `queued` não está vazia, tira o primeiro da
   fila e chama `resume`.

Progresso para a escolha do rebaixado vem de `PiecesHave/PiecesTotal`, não de
`BytesCompleted`: pausar libera os dados das peças na rain e zera `BytesCompleted`,
enquanto o bitfield sobrevive (é o mesmo motivo pelo qual `buildTorrentResponse` já usa
essa razão como fallback). Rebaixar quem tem menos progresso joga fora o mínimo de
trabalho — e, como o bitfield sobrevive, nada é de fato perdido.

`limit <= 0` faz `enforce` promover tudo o que estiver em `queued` e não pausar nada.

### Gatilhos

`enforce` roda depois de: `Add`, callback de conclusão, `Prioritize`, `Resume`, `Pause`,
`Remove`, `SetMaxActiveDownloads`, e o `Ensure` que cria uma sessão nova.

O callback de conclusão é o gatilho principal — é o momento em que um slot libera. O
`SessionManager` **embrulha** o `onComplete` recebido em `SetCallbacks`: roda `enforce`
primeiro, depois chama o callback do chamador (que enfileira o `JobOrganize`).

### Disciplina de lock

`SessionManager.mu` é um `RWMutex` segurado durante toda a chamada delegada — soltá-lo
cedo deixaria um `Ensure` concorrente derrubar a sessão no meio. `enforce` chama
`List`/`pause`/`resume`, que pegam o `RLock` de novo.

**Regra**: `enforce` nunca é chamada com `m.mu` segurado. Todo método público delega sob o
lock, **solta**, e só então chama `enforce`. Um `RLock` reentrante trava de vez se um
escritor entrar na fila entre os dois — é o modo de falha clássico do `RWMutex` do Go, e a
razão de a regra estar escrita aqui.

### Mudanças na interface `TorrentBackend`

```go
// Prioritize move o torrent para a frente da fila e o inicia, rebaixando o ativo de
// menor progresso se isso estourar o limite. Torrent completo ou inexistente é erro.
Prioritize(hash string) error
// SetMaxActiveDownloads define quantos torrents incompletos podem ficar ativos ao
// mesmo tempo. 0 (ou negativo) desliga o limite.
SetMaxActiveDownloads(n int)
```

Semântica dos métodos existentes, agora mediados pela fila:

| Método | Comportamento |
|--------|---------------|
| `Add` | Adiciona; `enforce` decide se fica ativo ou pausado na fila |
| `Pause` | Remove de `queued` (é um pause **do usuário**, a fila não retoma) e pausa |
| `Resume` | Anexa ao **fim** de `queued` e roda `enforce` — inicia na hora se houver slot, senão fica `queued` |
| `Prioritize` | Move para a **frente** de `queued` e roda `enforce(protected=hash)` |
| `Remove` | Remove de `queued` |

`Resume` deixa de significar "inicia agora" e passa a significar "volta para a fila".
É a única leitura coerente quando existe limite: sem isso, um `Resume` num torrent parado
furaria o limite silenciosamente.

## Status `queued`

`TorrentInfo.Status` ganha um slug novo, `queued`, escrito pela camada de fila em
`SessionManager.List`/`Get` para todo hash presente em `queued`.

É o **único** slug que não vem de `statusSlug()` (o mapa do enum da rain) — entra em
`decisions.md`. A alternativa, um campo booleano `queued` paralelo ao `Status`, obrigaria
o frontend a renderizar dois eixos de estado no chip, no filtro, no contador e na escolha
de botões. Um slug a mais reaproveita `statusLabel`, `statusVariant`, `KNOWN_STATUSES` e o
filtro de status que já existem.

## API

`POST /api/v1/torrents/{hash}/prioritize`, registrado no mesmo padrão de mux dos outros e
implementado com o helper `torrentAction` existente (POST-only, 404 quando `Get(hash)`
falha, chamada ao backend por último). 409 quando o torrent já completou.

`TorrentResponse` não ganha campo: o status novo já viaja em `status`.

## Config

`max_concurrent_downloads int` em `files.Config` (`json:"max_concurrent_downloads"`),
default **3** em `getDefaultConfig()`. `0` = ilimitado.

Aplicado com `backend.SetMaxActiveDownloads(configs.MaxConcurrentDownloads)` em três
pontos:

- `cmd/daemon/main.go`, no boot, depois do `Ensure` inicial;
- `PUT /api/v1/config` (`endpoint_config.go`), para valer sem reiniciar;
- início de `AnimeVerification`, como rede de segurança — é o mesmo lugar onde o passe já
  recarrega a config depois da migração.

Sem migração: `LoadConfigs` desserializa **sobre** `getDefaultConfig()`, então uma
`config.json` escrita antes deste upgrade — sem a chave — carrega o campo já valendo 3. Um
`0` literal no arquivo é o usuário pedindo ilimitado explicitamente.

## Frontend

**Config.svelte** — campo numérico `max_concurrent_downloads` no grupo `automation`, junto
de `check_interval`/`max_episodes_per_anime`, `min="0"`. Tipo em `client.ts`, rótulo e
dica em `messages/en.json` e `messages/pt-BR.json`.

**Downloads.svelte** — botão **Priorizar** / **Prioritize**, ícone `ChevronsUp` (lucide, já
é a fonte dos outros ícones da linha), visível quando `status` é `queued` **ou**
`stopped`. Nos dois estados os dois botões convivem e têm efeitos distintos:

| Status | Play (`resumeTorrent`) | Priorizar (`prioritizeTorrent`) |
|--------|------------------------|----------------------------------|
| `queued` | volta pro fim da fila | vai pro topo da fila, inicia já |
| `stopped` | entra no fim da fila | vai pro topo da fila, inicia já |

`torrentStatus.ts`: `queued` entra em `STATUS_SLUGS`, ganha rótulo ("Na fila" / "Queued")
em `statusLabel` e variante neutra em `statusClass`/`statusVariant`. `KNOWN_STATUSES` em
`torrentFilters.ts` também.

`canResume(t)` hoje é `status === "stopped"`; passa a incluir `queued`. `canPause(t)` é
`status !== "stopped" && status !== "stopping"`; passa a excluir `queued` também (pausar o
que já está pausado não faz nada, e o efeito real — tirar da fila — merece intenção
explícita, não um botão que parece inócuo).

Sem ação em massa de priorizar por ora: priorizar N torrents de uma vez é o mesmo que não
priorizar nenhum.

## Testes

**`torrents/queue_test.go`** (pacote `torrents`, test double local implementando
`queueOps`):

- respeita o limite: 5 torrents com limite 3 ⇒ 3 ativos, 2 em `queued`;
- promove na conclusão: completar um ativo promove o primeiro da fila;
- rebaixa o de menor progresso, nunca o `protected`;
- `Prioritize` move pra frente e inicia; num torrent já ativo é no-op;
- `Pause` do usuário tira da fila e não é retomado por `enforce` subsequente;
- `Resume` anexa ao fim, não à frente;
- adoção de boot roda uma vez só: incompletos parados entram na fila; um pause feito
  depois da adoção não é readotado;
- `limit = 0` promove tudo e não pausa nada.

**`FakeBackend`** ganha `Prioritize` e `SetMaxActiveDownloads` (registrando as chamadas,
como os outros métodos já fazem) para os testes de `daemon` e `api` continuarem compilando.

**`api/endpoint_torrents_test.go`**: rota `prioritize` — 405 em não-POST, 404 em hash
desconhecido, 200 no caminho feliz, chamada delegada ao backend.

**`api/endpoint_config_test.go`**: `max_concurrent_downloads` sobrevive ao round-trip do
`PUT` e dispara `SetMaxActiveDownloads`.

## Simplificações deliberadas

**A fila não é persistida em disco.** Estado em memória + adoção de boot. O custo: um
torrent pausado pelo usuário é adotado pela fila no boot seguinte e pode ser retomado
sozinho — o pause manual não sobrevive a um restart do daemon. Persistir custaria mais um
arquivo de estado, mais um caminho de migração e mais um modo de falha (fila em disco
divergindo da sessão) para proteger uma ação que já é transitória por natureza. Fica um
comentário `ponytail:` no `queue`, nomeando o teto e o caminho de upgrade.

**Download manual entra na frente da fila, não fura o limite.** Os endpoints manuais
(`download`, `redownload`, `replace`) chamam `Prioritize` logo depois do `Add`. O torrent
começa na hora, mas o total de ativos continua respeitando
`max_concurrent_downloads` — um limite que a UI mostra sendo violado é um limite que
ninguém confia.

## Documentação a atualizar

- `docs/agents/architecture.md` — `queue.go` no mapa de `torrents/`, os dois métodos novos
  da interface, a rota `prioritize`, o slug `queued`;
- `docs/agents/config.md` — `max_concurrent_downloads`;
- `docs/agents/decisions.md` — (a) `queued` é o único slug fora do `statusSlug()` da rain;
  (b) `Resume` significa "volta para a fila", não "inicia agora"; (c) fila em memória com
  adoção de boot, e o pause manual que se perde no restart;
- `swag init -g src/cmd/daemon/main.go -o docs/swagger` pela rota nova.
