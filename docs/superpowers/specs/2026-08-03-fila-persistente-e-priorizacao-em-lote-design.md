# Fila persistente, priorização em lote, posição visível e tooltips

Data: 2026-08-03

Sucede [2026-08-02-limite-downloads-simultaneos-design.md](2026-08-02-limite-downloads-simultaneos-design.md).
Aquela spec entregou o limite e a fila; esta corrige o modelo da fila e completa a UI.

## Problema

A fila entregue guarda **só os torrents que ela pausou** (`queued []string`). Torrent ativo
não está na fila, e é isso que produz os três defeitos abaixo.

**(a) Ordem invertida.** Corrigido em duas etapas antes desta spec — o desempate de
`leastProgressed` ia no primeiro e o rebaixado entrava na frente da fila, com `enforce`
rodando a cada `Add`. Ficou funcionando, mas às custas de duas peças (`AddedAt` na
ordenação, `insertByAddedAt`) que existem só para reconstruir uma ordem que a estrutura
joga fora.

**(b) Priorizar não priva ninguém.** Verificado em teste: com limite 2, `a`/`b` baixando e
`c` parado, `Prioritize("c")` põe `c` na frente da fila e o `enforce` não faz nada — não há
excesso a rebaixar (`len(active) == limit`) nem folga a promover. `c` só começa quando um
slot vagar. O `decisions.md` §41(b) afirma que Priorizar "rebaixa o ativo menos adiantado";
a documentação está errada, não o código.

**(c) Priorizar em lote não é composição de priorizar um.** Chamar `Prioritize` N vezes
inverte a ordem (cada `pushFront` passa na frente do anterior) e cada `enforce`
intermediário reembaralha. Um botão "priorizar o anime inteiro" precisa de uma operação
única no backend.

Há ainda uma quarta questão, de UX: reordenar a fila à mão e perder a ordem num restart do
daemon é inaceitável — hoje a fila é só memória.

## Objetivo

1. Fila como **lista ordenada de todos os torrents incompletos**; os `N` primeiros baixam.
2. Priorizar preempta: o priorizado começa na hora, quem for empurrado para fora do limite
   pausa.
3. Priorizar em lote — um anime inteiro, ou a seleção da tela de Downloads.
4. Fila e pausas manuais persistidas.
5. Posição na fila visível na tela de Downloads.
6. Tooltip nos botões só-ícone da tela de Downloads.

Não-objetivo: arrastar-e-soltar para reordenar a fila; UI dedicada de fila; **reordenar as
linhas da tela pela posição na fila**. A ordem passa a ser legível (o número no chip) e
alterável (Priorizar/Retomar), e isso basta. Reordenar as linhas seria o oposto: brigaria
com o sort que o usuário escolheu e com o agrupamento por anime, para dizer o que o número
já diz sem mexer em nada.

## Arquitetura

### A fila passa a conter tudo

```go
type queue struct {
	mu     sync.Mutex
	limit  int      // 0 ou negativo = ilimitado
	order  []string // todo torrent incompleto, na ordem em que devem baixar
	paused []string // pausados pelo usuário: não ocupam slot, nunca sobem sozinhos
	// queued é o resultado do passo 3 do último enforce: quem está em order, não está em
	// paused e ficou fora dos `limit` primeiros — mapeado para a POSICAO (1-based) na
	// espera. Existe para markQueued não ter de refazer a conta a cada List (a UI faz
	// polling), e é o único lugar de onde saem o slug e a posição.
	queued map[string]int
	path      string // <dir do banco de resume>/queue.json
	lastSaved []byte // último JSON gravado; a escrita só acontece quando o novo difere
}
```

**Invariante:** `order` contém exatamente os hashes incompletos que a sessão conhece.
Os ativos são os `limit` primeiros de `order` que não estão em `paused`. O resto de `order`
está parado: com status `stopped` se estiver em `paused`, com `queued` caso contrário.

Como nada é inserido no meio, some a pergunta "onde este hash entra": a posição só muda
por ação do usuário. Isso apaga `insertByAddedAt`, `leastProgressed` e o desempate de
progresso, e reduz a adoção de boot a um latch de uma vez só (ver Persistência — ela
encolhe, não desaparece). `AddedAt` continua existindo por um motivo só: dar ordem inicial
aos torrents que a fila ainda não conhece.

**Torrent completo não entra em `order`.** Seeding não ocupa slot (regra da spec anterior,
inalterada). Ao completar, o hash sai de `order` e um slot vaga.

### `enforce` vira uma reconciliação

Sob `queue.mu`, sem argumento `protected` — a preempção deixa de ser caso especial porque
priorizar já reposiciona o hash no início de `order`:

0. **Sem sessão, sai na hora.** `SessionManager.list()` devolve `nil` enquanto `m.session`
   é `nil` (config sem `SavePath`, ou o boot antes do primeiro `Ensure`); `Session.List()`
   devolve slice vazio **não-nil** para uma sessão sem torrents (`session.go:94`). A
   distinção já existe e é exatamente a que importa aqui: `all == nil` significa "não sei
   nada", não "não há nada". Sem esta guarda o passo 1 poda `order`/`paused` inteiros e o
   passo 5 grava o arquivo vazio por cima — e isso aconteceria em **todo boot**, porque
   `verification.go:86` chama `SetMaxActiveDownloads` (que roda `enforce`) antes do
   `Ensure`. A fila persistida morreria antes de ser usada uma vez. Sessão vazia de verdade
   continua podando normalmente.
1. **Poda**: sai de `order`/`paused` quem sumiu da sessão ou completou.
2. **Anexa**: todo incompleto da sessão ausente de `order` entra no fim, ordenado por
   `AddedAt` (é o caso de `Add`, e o do boot com um `queue.json` desatualizado).
3. **Calcula o desejado**: percorre `order` pulando `paused`, marcando os `limit` primeiros
   como ativos e o resto como enfileirados, **numerados a partir de 1 na ordem em que vão
   começar**. `limit <= 0` marca todos como ativos. O resultado fica em `q.queued`, e é dele
   que `markQueued` lê. A numeração não custa uma segunda passada: é o contador da mesma
   varredura que já decide quem espera.
4. **Aplica a diferença, iterando `order` — não a sessão**: `resume` em quem deveria estar
   ativo e está `stopped`; `pause` em quem não deveria e está rodando. Nada é chamado
   quando o estado já bate — sem isso, o `enforce` que roda a cada `Add` e a cada torrent
   completado ficaria pausando e religando o mesmo torrent.

   **O domínio da iteração é a parte que não pode ficar implícita.** Varrer a sessão inteira
   pausaria **todo seeder**: completo nunca entra em `order`, logo nunca está no conjunto
   desejado, logo "não deveria estar ativo". `order` é a lista dos incompletos e é
   exatamente o conjunto sobre o qual a fila tem autoridade; o que está fora dela a fila não
   toca, nem para pausar nem para retomar.
5. **Salva** se `order`/`paused` mudaram em relação ao último estado gravado.

**`stopping` não é alvo de nada.** A pausa da rain é assíncrona (`Torrent.Stop()` volta na
hora, o status só chega a `stopped` até ~5s depois). Tratar `stopping` como parado faria o
`enforce` mandar `resume` por cima de um stop em andamento; tratá-lo como rodando faria o
passo 4 pausá-lo de novo. Fica de fora dos dois lados e o passe seguinte decide sobre um
estado estável.

Uma falha de `pause`/`resume` é logada e o hash mantém a posição — a próxima passada tenta
de novo. O estado da fila não mente sobre o que a rain fez.

**Quem garante que existe uma próxima passada:** `verification.go` chama
`SetMaxActiveDownloads` a cada passe de verificação (a rede de segurança para um
`config.json` editado à mão), e todo `SetMaxActiveDownloads` roda `enforce`. É isso que
transforma o `enforce` em reconciliação de verdade em vez de uma reação a eventos: um
`stopping` que ficou no meio do caminho, ou um `pause` que a rain recusou, se resolve
sozinho no próximo ciclo, sem ticker novo nenhum.

### `markQueued` lê o passo 3, não a lista inteira

No modelo antigo `queued` era a lista dos pausados pela fila, então pertencer a ela era a
definição do slug. Agora `order` contém os ativos também — marcar por pertencer a `order`
pintaria a tela inteira de `queued`. O slug passa a sair de `q.queued`, o mapa que o
passo 3 acabou de calcular. Consequência: **quem está em `paused` mostra `stopped`**, que é
o que a rain reporta e o que o usuário pediu; `queued` fica significando só "a fila vai
chegar nele".

`markQueued` escreve dois campos, não um: o slug e `TorrentInfo.QueuePosition`, a posição
que veio do mesmo mapa. **0 = não enfileirado**, que é o caso de todo ativo, todo completo e
todo pausado — pausado tem posição em `order`, mas ela não prevê nada (o torrent nunca sobe
sozinho), e publicar um número que não vai acontecer é pior do que não publicar nenhum.

**`q.queued` é o desejado, e a tela não pode mostrar o desejado como se fosse o real.** Um
`pause` que a rain recusou deixaria o hash no mapa e a linha exibindo `Na fila #3` num
torrent baixando a todo vapor — que é exatamente o oposto de "o estado da fila não mente
sobre o que a rain fez". Então o passo 4 **remove de `q.queued` o hash cujo `pause` falhou**:
a posição em `order` fica (a fila ainda quer aquilo), o slug e o número saem (a tela mostra o
que está acontecendo). O efeito colateral é um buraco na numeração — `#1, #3, #4` — que dura
até o próximo passe e é preferível a um número confiante e falso.

### Operações do usuário

| Operação | Efeito em `order` / `paused` |
|---|---|
| `Prioritize(hashes)` | move para o **início** os que já estão em `order`, **insere** no início os que ainda não estão, na ordem recebida; tira de `paused`; `enforce` |
| `Resume(hash)` | move para o **fim**; tira de `paused`; `enforce` |
| `Pause(hash)` | entra em `paused`, sem mexer na posição; `enforce` |
| `Remove(hash)` | sai dos dois; `enforce` |
| `SetMaxActiveDownloads(n)` | só troca `limit`; `enforce` |

É "move **ou insere**" porque o hash pode ser incompleto e ainda não estar em `order`: a
sessão foi recriada e nenhum `enforce` rodou desde então, ou o `Add` falhou o passo 2. Tratar
só o caso "já está lá" faria o priorizado sumir da fila em vez de ir para a frente.

**Torrent completo continua fora da fila nas DUAS direções.** `Pause` e `Resume` de um
seeder delegam direto para a rain e não passam por `order`/`paused` — hoje isso existe só no
`Resume` (`sessionmanager.go:311`) e precisa passar a existir também no `Pause`. Sem o
bypass explícito, e com o passo 4 restrito a `order` (como deve ser), pausar um seeder não
pararia nada e retomá-lo não religaria nada: a tabela acima descreve escrituração de fila, e
completo não tem escrituração de fila. Consequência a escrever no tipo: **`paused` guarda só
hashes incompletos.**

`Prioritize(hash)` continua existindo como `Prioritize([]string{hash})` — o botão
individual passa a preemptar, igual ao de lote.

**Ordem dentro do `Pause`:** entra em `paused` **antes** de mandar pausar, nunca depois. É o
mesmo motivo do `drop`-antes-do-`pause` de hoje (`sessionmanager.go:294`): no intervalo entre
as duas chamadas, um `enforce` disparado por um torrent que acabou de completar promoveria
justamente o torrent que o usuário está parando.

Retomar continua significando "vai para o fim da fila": com limite ativo, um resume que
iniciasse na hora furaria o limite em silêncio.

**`Pause` num torrent enfileirado passa a ser operação com sentido**, e a UI deixa de
escondê-la (`canPause` hoje exclui `queued`). Com `paused` explícito, pausar um enfileirado
quer dizer "não inicie sozinho quando o slot vagar" — antes disso, a única forma de impedir
que um episódio começasse era deletar o torrent. O status vira `stopped`, que é justamente
a distinção que o slug novo carrega: `stopped` = o usuário parou, `queued` = a fila vai
chegar nele.

A linha de um enfileirado fica então com três botões, cada um numa direção diferente:
Priorizar sobe, Play desce para o fim, Pause tira da rotação.

`Pause` não mexe na posição, mas isso não é promessa nenhuma ao usuário: sair de `paused`
só acontece por `Resume` (que manda para o fim) ou `Prioritize` (que manda para o início),
e os dois definem a posição explicitamente. A posição preservada só evita que um torrent
pausado e removido da lista reapareça em lugar arbitrário caso `paused` e `order` saiam de
sincronia.

### Persistência

`queue.json`, ao lado do banco de resume da rain (`filepath.Dir(dbPath)`), no mesmo lugar
e pelo mesmo motivo que o `download_root.id` de `rootmarker.go`: é estado do torrent
client, não configuração do usuário, e precisa acompanhar o banco de resume. Nada de campo
novo no `FileManager` nem de fiação no `main.go` — só o `path` que o `NewSessionManager`
monta a partir do `dbPath` que já recebe, do mesmo jeito que faz com o `idPath`.

```json
{ "order": ["hash1", "hash2"], "paused": ["hash9"] }
```

Duas funções, sem tipo novo: `loadQueue(path)` e `saveQueue(path, order, paused)`. O "só
grava quando muda" é comparar o JSON recém-marshalado com `lastSaved` — uma comparação que
já produz o payload da escrita. A escrita é tmp + `Rename`, o mesmo padrão de
`filemanager.go:272` (`saveConfigsLocked`).

- **Escrita**: só no passo 5 do `enforce`, e só quando o par difere do último gravado.
  Falha de escrita é warning; a fila continua correta em memória até o próximo restart.
  **`lastSaved` só é atualizado depois do `Rename` bem-sucedido.** Atualizá-lo junto com a
  tentativa transforma uma falha transitória (disco cheio por um minuto) em perda
  permanente: o estado em memória passa a bater com o `lastSaved`, o "só grava quando muda"
  nunca mais dispara, e o arquivo fica congelado na versão velha até alguém mexer na fila de
  novo.
- **A escrita acontece com `queue.mu` segurado**, e `markQueued` disputa esse mutex no
  caminho do polling da UI. Aceito e documentado, não resolvido: são algumas centenas de
  bytes com tmp + `Rename`, e só quando a fila muda. Se a listagem engasgar, o conserto é
  marshalar/comparar sob o lock e escrever fora dele — mas aí a ordem de duas escritas
  concorrentes deixa de ser garantida pelo lock, então não faça antes de ter o problema.
- **Leitura**: uma vez, na criação do `SessionManager`. Arquivo ausente, vazio ou ilegível
  = fila vazia + warning, e o passo 2 do `enforce` reconstrói tudo por `AddedAt`. **Nunca**
  impede o daemon de subir.
- **Arquivo ausente arma um latch que semeia `paused` no primeiro `enforce` com sessão.**
  Sem isso o primeiro boot depois deste deploy repete exatamente o bug que a spec existe
  para matar: sem `queue.json`, o passo 2 anexa *todo* incompleto — inclusive os que o
  usuário tinha pausado à mão, que a rain persiste como `stopped` — e o passo 4 retoma os
  `limit` primeiros.

  **Não dá para semear na leitura**, e a versão anterior desta spec dizia que dava: em
  `NewSessionManager` (`sessionmanager.go:58`) ainda não existe sessão, `list()` devolve
  `nil` e não há o que consultar. A leitura só pode **armar** o latch (`seedPaused bool`,
  ligado quando o arquivo está ausente); quem o **consome** é o primeiro `enforce` que passe
  do passo 0, entre os passos 1 e 2, marcando como `paused` os incompletos que a rain
  reporta `stopped`. O latch desliga em seguida e nunca mais liga.

  Isso significa que a **adoção de boot não é apagada pelo modelo novo — é renomeada e
  restringida**, e a spec não deve vender o contrário. O que muda é o alcance: a adoção de
  hoje roda em toda sessão criada (`resetAdoption` em cada `Ensure` que cria) e decide quem
  a fila retoma; o latch novo roda uma vez na vida da instalação, só quando não há
  `queue.json`, e só escreve em `paused` — nunca inicia nada. É menos código e muito menos
  superfície, mas não é zero.

  O preço, que é real e não dá para eliminar: nesse boot de upgrade os torrents que a
  **fila antiga** pausou (por estarem acima do limite) estão parados na rain exatamente
  como os que o usuário pausou, e a fila antiga era só memória — não sobrou nenhum sinal
  que os separe. Os dois grupos entram em `paused` e os enfileirados de antes do upgrade
  precisam de um clique em Play ou Priorizar para voltar. É a troca certa: congelar é
  visível na tela e custa um clique, retomar sozinho é invisível e desfaz uma decisão do
  usuário gastando banda. Vale um log no boot dizendo quantos hashes foram semeados, para o
  "por que tudo está parado?" ter resposta no arquivo de log. Só acontece uma vez: do
  segundo boot em diante existe `queue.json` e os dois grupos são distintos.
- **Reconciliação**: o passo 1 descarta hash que a sessão não tem mais (episódio apagado
  com o daemon fora do ar) e o passo 2 anexa o que a rain tem e o arquivo não.

Com `paused` no disco, cai o teto marcado pelo comentário `ponytail:` no tipo: a pausa
manual passa a sobreviver a restart, e a "adoção de boot" — que hoje roda a cada sessão
criada e retoma por engano o que o usuário tinha pausado — encolhe para o latch de uma vez
só descrito acima.

Troca de `save_path` (sessão recriada) **não** zera a fila: os hashes são os mesmos, e a
poda/anexação do `enforce` acerta qualquer diferença.

### API

```
POST /api/v1/torrents/prioritize
{ "hashes": ["...", "..."] }
```

Aplica **na ordem recebida** — quem decide a ordem é o frontend, que é quem sabe o que o
usuário clicou. Hash desconhecido ou já completo é ignorado (não é erro: a lista pode ter
envelhecido entre o render e o clique). Corpo vazio ou sem hashes = 400. A resposta segue
o padrão das outras ações de torrent.

`POST /api/v1/torrents/{hash}/prioritize` continua e **mantém os erros que já devolve
hoje** — hash inexistente ou já completo é erro, não silêncio. Só o lote ignora, porque
uma lista de 12 episódios não pode falhar inteira porque um deles terminou de baixar entre
o render e o clique. A validação fica no `SessionManager.Prioritize(hash)`, que checa e só
então delega para `PrioritizeAll`.

**`PrioritizeAll` não pode chamar `m.Get`** (nem `List`): `Get` passa por `markQueued`, que
pega `queue.mu`. Chamado de dentro do lock, é o mesmo deadlock que o "Don't fix by" do §41 já
descreve para `enforce`. O desenho acima respeita isso porque a validação é sequencial —
`Prioritize(hash)` chama `Get` **antes** de delegar, nunca durante. Vale como linha nova no
"Don't fix by", que é onde alguém vai olhar antes de "simplificar" movendo a checagem para
dentro.

`TorrentBackend` ganha `PrioritizeAll(hashes []string) error`; `FakeBackend` registra as
chamadas como já faz com `Prioritize`.

A rota nova é um segmento literal onde `/api/v1/torrents/{hash}` tem um curinga, e o
ServeMux do Go 1.22+ dá precedência ao literal — `POST /api/v1/torrents/prioritize` não
colide com o handler genérico (e nenhum info hash é a string "prioritize": são 40 chars hex).
O comentário de `server.go:106-111`, que enumera os casos de não-colisão, **já está
desatualizado hoje** (não menciona `/prioritize` de `:116`) e passa a precisar também deste
caso; sem isso o próximo a mexer ali refaz a análise do zero.

`TorrentResponse` ganha `queue_position int` (`json:"queue_position"`), espelhando
`TorrentInfo.QueuePosition` em `buildTorrentResponse`. Não é ponteiro: 0 já significa "não
está na fila" sem ambiguidade — não existe posição 0 — e um `*int` obrigaria todo consumidor
a desreferenciar para chegar na mesma conclusão.

### Frontend

**Botão do anime** — quarto ícone (`ChevronsUp`) no cabeçalho do grupo, ao lado de
pausar/retomar/excluir, chamando `prioritizeTorrents` com os episódios do grupo que aceitam
priorizar (`canPrioritize`), **em ordem crescente de `episode_number`** — batch e episódio
sem número vão no fim, preservando a ordem de exibição entre eles.

**Botão da seleção** — "Priorizar" na barra de ações em lote (`DownloadsToolbar`), na
ordem em que o usuário marcou as linhas. O `Set` de seleção já preserva ordem de inserção;
o `bulkTargets` atual reordena para a ordem de exibição, então o priorizar em lote itera a
seleção diretamente em vez de usar `bulkTargets`.

**Botão Pause da linha** — são **duas** mudanças, e a segunda é a que faz o botão existir.
`canPause` deixa de excluir `queued` (vira só `status !== "stopped" && status !== "stopping"`),
**e** o `{#if canResume(t)}` Play `{:else}` Pause `{/if}` de `Downloads.svelte:757-777` vira
dois `{#if}` independentes. Hoje os dois são mutuamente exclusivos: um `queued` tem
`canResume === true`, cai no ramo do Play, e o Pause nem chega ao DOM — relaxar `canPause`
sozinho não renderiza nada. Os "três botões, cada um numa direção" dependem da quebra do
`else`, não do predicado.

**Posição na fila** — no chip de status que já existe na linha, sem coluna nova e sem mexer
na altura: `t.queue_position > 0 ? m.downloads_queue_position({ position }) : statusLabel(t.status)`.
Só isso.

`statusLabel()` **continua pura e não recebe a posição**: o `DownloadsToolbar` chama a mesma
função para montar o filtro de status, onde "Na fila #7" não faria sentido nenhum. A posição
entra no render da linha, não no vocabulário de status.

Os números aparecem **não-contíguos** dentro de um grupo (`#2`, `#5`, `#6`) sempre que outro
anime está intercalado na fila. Isso é a feature, não um defeito de apresentação: é o que
comunica, sem uma palavra de texto, que a fila é global e que tem coisa de outro anime na
frente. Reordenar ou renumerar por grupo destruiria exatamente essa informação.

**Tooltips** — daisyUI 4, wrapper `<div class="tooltip tooltip-*" data-tip=...>` em volta do
botão, o mesmo padrão de `NavRail.svelte:141` (na v4 o tooltip sai do `data-tip` do wrapper;
`.tooltip-content` é v5). Nos botões só-ícone da tela: os do cabeçalho do grupo, os da linha
e o limpar-busca da toolbar. `tooltip-left` nos do cabeçalho, que ficam colados na borda
direita.

O texto reaproveita as chaves que já alimentam o `aria-label`, sem chave nova de i18n — mas
é **só o rótulo da ação**: os `aria-label` de grupo e de linha concatenam `" — {nome}"`, que
no tooltip é ruído. Exceção: as duas chaves novas do Priorizar em grupo/lote.

Chaves novas em `en.json` e `pt-BR.json`: rótulo do priorizar em lote, `aria-label` do
botão de grupo, `downloads_queue_position` (`"Queued #{position}"` / `"Na fila #{position}"`)
e a mensagem de resultado.

**A mensagem do priorizar não segue o `{success} of {total}` das outras.** As demais ações
em lote são N requisições, então sucesso parcial é um resultado real. Esta é **uma**
requisição: ou aplicou tudo o que dava, ou falhou inteira. Contar "12 de 12" seria contar
uma coisa só duas vezes, e o backend ignora completo/desconhecido em silêncio, então nem dá
para verificar o denominador do lado do cliente. Uma chave, `{count} priorizados`, com o
`count` do que o frontend enviou; erro cai no `toast.error` comum. **Com plural**: o botão
individual da linha também passa por aqui, então `count === 1` é o caso comum, não a
exceção.

## Erros e casos de borda

| Caso | Comportamento |
|---|---|
| Priorizar mais torrents que o limite | Os `limit` primeiros iniciam; o resto fica no topo de `order`, em ordem |
| Priorizar um torrent já baixando | Sobe em `order`; nenhum pause/resume — a diferença do passo 4 é vazia |
| Priorizar um completo | Ignorado (não ocupa slot) |
| Preempção escolhe quem rebaixar | **Por posição em `order`, não por progresso** — sai o último dos ativos. Um download em 95% pode ser pausado se estiver depois na fila; era protegido pelo `leastProgressed` antigo. Aceito: posição é o eixo que o usuário controla, e a alternativa é ressuscitar a peça que esta spec apaga |
| Torrent em `stopping` no meio de um `enforce` | Nem pausado nem retomado; o passe seguinte (pior caso: o `SetMaxActiveDownloads` do próximo ciclo de verificação) decide sobre estado estável |
| Pausar um enfileirado | Entra em `paused`, status vira `stopped`, mantém a posição e nunca sobe sozinho |
| Primeiro boot sem `queue.json` | `paused` semeado com os incompletos que a rain reporta parados, para não retomar pausa manual de antes do upgrade |
| `Pause`/`Resume` de torrent completo | Bypassa a fila nos dois sentidos: delega para a rain, não entra em `order`/`paused` |
| Sessão só com torrents completos | `enforce` não pausa nada — o passo 4 itera `order`, e completo não está lá |
| `pause` do passo 4 falha | Hash mantém a posição em `order`, sai de `q.queued`; a linha mostra o status real da rain e a numeração fica com um buraco até o próximo passe |
| Priorizar hash incompleto que não está em `order` | Inserido no início (sessão recém-criada, `enforce` ainda não anexou) |
| Preempção pausa download com progresso | Aceito: a rain zera `Bytes.Completed` ao parar, mas o bitfield das peças sobrevive e o retomar continua de onde parou |
| `queue.json` corrompido | Warning + fila reconstruída por `AddedAt`; daemon sobe |
| Hash no arquivo que a sessão não tem | Podado no passo 1 |
| Torrent na sessão fora do arquivo | Anexado no fim no passo 2 |
| `limit <= 0` | Todos ativos, menos os de `paused`; `order` continua sendo mantido para quando o limite voltar |
| `enforce` sem sessão (boot, config sem `SavePath`) | Sai no passo 0 sem podar nem gravar — `all == nil` é "não sei nada", sessão vazia é `[]` |
| `queue_position` de ativo, completo ou pausado | 0. O chip mostra o status normal; só linha `queued` ganha número |
| `limit <= 0` e posições | Ninguém fica `queued`, então toda posição é 0 — não há espera para numerar |
| Posições dentro de um grupo | Não-contíguas quando outro anime está na fila; é o sinal de que a fila é global |

## Testes

**Antes de escrever qualquer caso: o duplo tem de devolver slice vazio não-nil.**
`queue_test.go:19` faz `append([]TorrentInfo(nil), f.torrents...)`, que com `torrents` vazio
devolve **`nil`** — e `nil` agora é a saída do passo 0. Todo teste que comece com sessão
vazia viraria um no-op que passa por engano, sem nunca exercitar a fila. É a armadilha mais
cara desta spec, porque a suíte fica verde enquanto mede nada.

**Fila** (`queue_test.go`, reescrito com o modelo novo):
- os `N` primeiros de `order` baixam e o resto fica `queued`;
- priorizar preempta: com o limite cheio, o priorizado inicia e o deslocado pausa;
- priorizar um lote maior que o limite: inicia `limit`, o excedente fica no topo em ordem;
- priorizar respeita a ordem recebida, sem reordenar por `AddedAt`;
- pausa manual não é desfeita pelo `enforce` e não ocupa slot;
- pausa manual mantém a posição; retomar manda para o fim;
- completar libera slot e promove o próximo da ordem;
- torrent novo entra no fim por `AddedAt`;
- `enforce` idempotente: a segunda passada não chama `pause`/`resume` nenhum;
- status: os enfileirados saem como `queued`, os de `paused` saem como `stopped` e os
  ativos mantêm o slug da rain;
- posição: os enfileirados são numerados 1..N na ordem de `order`; ativo, completo e
  pausado saem com 0; priorizar renumera (o priorizado que não coube vira `#1`);
- `stopping` não recebe `pause` nem `resume`;
- **`enforce` sem sessão não poda nem grava** — a regressão mais cara possível (a fila
  persistida some em todo boot) e a mais silenciosa, porque tudo continua "funcionando";
- **seeding não é tocado pelo `enforce`**: uma sessão só de completos não recebe `pause`
  nenhum (é o teste que pega a iteração sobre a sessão em vez de sobre `order`);
- `Pause`/`Resume` de torrent completo delegam para a rain e não entram em `order`/`paused`;
- `pause` que falha tira o hash de `q.queued`: a linha volta a mostrar o status da rain, não
  `queued`;
- `Prioritize` de hash incompleto ausente de `order` **insere** no início em vez de sumir.

**Persistência**: round-trip `order`/`paused`; arquivo ausente; arquivo corrompido não
propaga erro; grava só quando muda; **arquivo ausente com torrents parados na rain semeia
`paused` e não os retoma** (o caso do upgrade — e o latch tem de ser consumido pelo primeiro
`enforce` **com sessão**, não na leitura); falha de escrita não atualiza `lastSaved`, e a
gravação seguinte com o mesmo estado tenta de novo.

**API**: handler em lote aplica na ordem recebida; hash desconhecido é ignorado; lista
vazia é 400.

**Frontend**: a ordenação por `episode_number` do botão de grupo e a ordem de clique do
lote são funções puras — teste unitário nelas, sem tocar em componente.

## Documentação

- `decisions.md` §41 reescrita: o modelo é outro, e (b) está factualmente errado hoje.
  Entram: por que a fila contém tudo, por que `paused` é explícito, por que a persistência
  fica junto do banco de resume, por que a preempção passa a escolher por posição e não por
  progresso (o item (d)/(e) atual descreve peças que deixam de existir), e por que
  `stopping` fica fora dos dois lados do passo 4. O item (a) atual afirma que a fila não é
  persistida — passa a ser o oposto, com o latch de upgrade no lugar da adoção.
  Entram no "Don't fix by": iterar a sessão em vez de `order` no passo 4 (pausa todo
  seeder), chamar `Get`/`List` de dentro de `PrioritizeAll` (deadlock em `queue.mu`),
  atualizar `lastSaved` sem o `Rename` ter dado certo, e inverter a ordem
  `paused`-antes-do-`pause`.
- `backend.go`: os comentários de `Pause`, `Resume` e `Prioritize` na interface descrevem o
  modelo antigo (`Prioritize` "pausando o menos adiantado", `Pause` "tira da fila").
- `server.go:106-111`: o comentário dos casos de não-colisão de rota, já desatualizado
  quanto a `/prioritize` por hash, ganha o `/torrents/prioritize` sem `{hash}`.
- `architecture.md`: símbolos da `queue`, o endpoint novo, `PrioritizeAll`, `queue.json`,
  `QueuePosition`/`queue_position`.
- Swagger regenerado (campo novo no `TorrentResponse` + endpoint de lote).
