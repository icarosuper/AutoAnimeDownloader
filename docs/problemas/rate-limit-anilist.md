# O orçamento da AniList: quem gasta, e como não tomar 429

Escrito em 2026-08-28, a partir da investigação que começou em
[raiz-comum-eixo-absoluto.md](raiz-comum-eixo-absoluto.md) mas é assunto próprio: o rate limit da
AniList é um problema **pré-existente**, que existe hoje sem nenhuma feature nova, e o item de ação
principal deste arquivo (`id_in` nos avulsos) não depende de nada daquele.

O limite é **30 requests por minuto por IP público**. Hoje três consumidores disputam esse balde
sem nenhuma coordenação entre si.

## De onde vem o consumo, em regime

| Consumidor | Custo por minuto |
|---|---|
| `GetFrontendAnimeList` — TTL 60s, compartilhado entre abas | 1 por conta |
| `GetCustomListsMap` — TTL 5min, ou 30s quando a resposta vem vazia | 0,2 a 2 por conta |
| **`appendStandaloneEntries` — `GetMediaByID` um por avulso, TTL 60s cada** | **N, onde N = nº de avulsos** |
| Passe do daemon — `GetAllCurrentAnime`, a cada `CheckInterval` (10min) | 0,1 por conta |

O poll do frontend já está bem contido: as abas compartilham o cache, então o custo não cresce com o
número de abas. **O termo que explode é N.**

---

## AÇÃO: trocar o loop de avulsos por uma query em lote

**Onde:** `src/internal/api/standalone.go:56`, em `appendStandaloneEntries`.

**O que acontece hoje:** o handler de `GET /api/v1/animes` percorre os avulsos chamando
`anilist.GetMediaByID(id)` **um por um**. Cada um tem cache próprio de 60s
(`mediaByIDTTL`, `standalone.go:16`), então em regime isso é **N requests por minuto**.

Com 20 avulsos são 20 dos 30 requests do balde, gastos só para montar uma tela — e o 429 chega sem
nada de novo ter sido adicionado ao projeto. O mesmo loop existe no lado do daemon
(`src/internal/daemon/standalone.go:54`).

**A correção:** `Page(media(id_in: [...]))` aceita até 50 ids numa query, e uma query custa uma
unidade de rate limit independente do tamanho (medido — ver adiante). Isso leva **N req/min para 1**.

É o mesmo mecanismo já validado na pesquisa das cadeias de PREQUEL, então não é técnica nova para
este projeto.

**Cuidados:**

- `perPage` máximo é 50: mais de 50 avulsos viram 2 queries. Ainda é 2 contra 50.
- O cache hoje é por id (`mediaByIDCache`). Em lote, o certo é continuar gravando **por id** —
  senão um avulso adicionado invalida o lote inteiro. Buscar só os ids que erraram o cache.
- `Page(media(id_in:))` **omite silenciosamente** ids que não existem (media apagado da AniList).
  O loop atual distingue isso via `ErrNotFound` e loga. Em lote, id pedido que não voltou = ausente,
  e precisa do mesmo tratamento que o `GetMediaByID` dá hoje, senão um avulso morto some da tela sem
  nenhum aviso.
- Os dois lados (`api/standalone.go` e `daemon/standalone.go`) fazem o mesmo loop. Corrigir só um
  deixa metade do consumo de pé.

---

## Os headers de rate limit

Medido em 2026-08-28. A AniList devolve o estado do orçamento em **toda** resposta, o que abre um
caminho melhor que qualquer limiter que tente adivinhar o consumo.

**Os headers vêm sempre, inclusive em erro:**

```
media minimo             HTTP 200  {'X-RateLimit-Limit': '30', 'X-RateLimit-Remaining': '29'}
200 normal               HTTP 200  {'X-RateLimit-Limit': '30', 'X-RateLimit-Remaining': '28'}
404 (media inexistente)  HTTP 404  {'X-RateLimit-Limit': '30', 'X-RateLimit-Remaining': '27'}
query invalida           HTTP 400  {'X-RateLimit-Limit': '30', 'X-RateLimit-Remaining': '26'}
query pesada aninhada    HTTP 200  {'X-RateLimit-Limit': '30', 'X-RateLimit-Remaining': '25'}
```

Três consequências:

1. **Erro consome cota.** Um 404 e uma query malformada custam 1 cada. Retry cego em cima de erro
   queima orçamento — um contador client-side que só contasse sucessos ficaria otimista e levaria ao
   429 justamente quando as coisas já estão indo mal.
2. **Complexidade não custa nada.** A query aninhada de 33 KB decrementou 1, igual à query mínima.
   Reforça a escolha do nesting: trabalho por request é de graça.
3. **O header é autoritativo.** Ele já soma o passe do daemon, o poll do frontend e as buscas de
   avulsos — os três disputam o mesmo balde hoje, sem coordenação nenhuma. Nenhum contador interno
   chega perto dessa precisão, porque nenhum enxerga os erros dos outros.

**O balde reseta de uma vez, não desliza:**

```
t=+  0s  Remaining=24   (gastando 4 de uma vez)
t=+  1s  Remaining=20   <- depois da rajada
t=+ 21s  Remaining=19
t=+ 41s  Remaining=29   <- balde resetou inteiro
```

Entre 21s e 41s o orçamento voltou cheio de uma vez. É balde fixo com reset discreto, não janela
deslizante com recarga gradual. Consequência prática: **o orçamento se recupera inteiro em ≤ 60s.**

**`X-RateLimit-Reset` não vem.** O header aparece anunciado em
`Access-Control-Expose-Headers: X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset, …`,
mas **não está presente** em nenhuma resposta 200 observada. Pode ser que só venha no 429, que não
foi testado (ver abaixo). Não dá para depender dele.

O comentário de `retryAfter` (`health.go`) afirmava que a AniList "tambem manda X-RateLimit-Reset
com o unix timestamp" — já foi corrigido para registrar o que foi medido e o que não foi.

### O que NÃO foi medido

Não provoquei um 429 de propósito: ele bloqueia o IP por cerca de um minuto, e o daemon do usuário
compartilha esse IP — o custo cairia num passe real. Portanto, **quais headers um 429 carrega
continua não verificado**. O que se sabe vem do código existente: `health.go` já lê `Retry-After` de
um 429 e o classifica como `HealthRateLimited`.

### O desenho: reserva por prioridade, não fila

`health.go` já tem o formato certo para isso — um estado de pacote atualizado por
`sendAnilistRequest`, que **toda** chamada atravessa. O mesmo lugar recebe o orçamento:

- gravar `X-RateLimit-Remaining` e o instante da leitura num atômico ao lado de `health`, **em toda
  resposta, inclusive erro** (é onde a precisão mora);
- antes de emitir, comparar com um piso.

O ponto delicado é o que fazer quando está baixo. **Bloquear e esperar é a resposta errada**: o poll
do frontend roda dentro de um handler HTTP e penduraria a tela. A resposta certa é **degradar o que
é barato degradar**:

| Chamador | Criticidade | Ação com orçamento baixo |
|---|---|---|
| Passe do daemon | alta — perder o passe é episódio não baixado | sempre passa |
| Poll do frontend (`GetFrontendAnimeList`) | baixa — já tem cache de 60s | recusa e serve cache velho |
| Busca de avulso pela UI | baixa | recusa, erro visível ao usuário que pediu |
| Warm-up das cadeias (BFS) | baixa — tem fallback | adia para o próximo passe |

Como o balde se recupera em ≤ 60s e o passe do daemon roda a cada 10 min, o daemon praticamente
sempre encontra orçamento cheio se o frontend for contido nos momentos de aperto.

**A saída obrigatória:** se todo mundo for recusado quando o orçamento está baixo, ninguém faz
request e o processo nunca descobre que o balde resetou. Por isso a leitura precisa de validade —
passados 60s da última observação, tratar o orçamento como cheio de novo. É exatamente para isso que
a medição do reset serve, e é o que impede o gate de se auto-travar.

Isso torna o warm-up do BFS seguro por construção: uma lista gigante que precisasse de muitos
requests seria espalhada por vários passes sozinha, sem nenhum `K` mágico.

### Efeito colateral: o limiter externo deixa de fazer sentido

Isto **substitui** a opção `x/time/rate` que estava na tabela de alternativas, e é melhor em três
frentes: não adiciona dependência, mede em vez de adivinhar, e enxerga os erros que um token bucket
client-side não contabilizaria. Também resolve o problema pré-existente de os três consumidores
disputarem o mesmo orçamento sem coordenação — que hoje é um 429 esperando acontecer,
independentemente desta feature.


---

## Ideia avaliada: o frontend buscar direto na AniList

Proposta: hoje o frontend sempre lê a AniList através do backend. Se ele buscasse por conta própria,
quando estivesse rodando num PC diferente do daemon o consumo cairia em outro IP e aliviaria o balde.

**Os pré-requisitos técnicos existem.** Medido:

- A AniList responde `Access-Control-Allow-Origin: *` e expõe os headers de rate limit em
  `Access-Control-Expose-Headers`. Um browser pode consultar direto, inclusive lendo o orçamento.
- O app **não usa token nenhum** — todas as queries são públicas, por username. Não há credencial
  para vazar para o browser.

**O furo é NAT.** O limite é por **IP público**, não por máquina. Daemon no desktop e browser no
notebook, na mesma casa, saem pelo mesmo IP público → **mesmo balde, ganho zero**. O alívio só
acontece quando o browser está numa rede genuinamente diferente: celular em dados móveis, acesso
remoto do trabalho, VPN. Esse é o cenário menos comum, e o custo seria pago em todos os cenários.

**O custo é duplicação.** O frontend teria que replicar em TypeScript o `DedupeByMedia`, o merge de
`customLists`, o `withStandaloneProgress` e a classificação de health do `health.go`. Lógica de Go
duplicada em TS diverge em silêncio, e o daemon continuaria precisando dos mesmos dados para o passe
— ou seja, dois caminhos para o mesmo dado, não um.

**Comparação para o mesmo problema:**

| | ganho | custo | funciona quando? |
|---|---|---|---|
| Frontend busca direto | move o poll para outro IP | lógica duplicada em TS | só se o browser estiver em outra rede |
| **`id_in` em `appendStandaloneEntries`** | N/min → 1/min | ~10 linhas | sempre |

**Veredito: não fazer por ora.** A ideia aponta para um problema real — o frontend é responsável
pela maior fatia do consumo, via avulsos —, mas ataca a fatia errada com o instrumento caro. O
`id_in` resolve o mesmo problema, sempre, sem duplicar nada. Reavaliar só se, depois do `id_in` e do
gate por prioridade, o rate limit voltar a apertar.

## Ordem de execução

1. **`id_in` nos avulsos**, nos dois lados (`api/standalone.go:56` e `daemon/standalone.go:54`).
   É o consumidor dominante e a correção é pequena.
2. **Gate por prioridade lendo `X-RateLimit-Remaining`** em `sendAnilistRequest`. Vale por si só e
   torna qualquer feature futura segura por construção.
3. Frontend direto: só se 1 e 2 não bastarem. Provavelmente não vai ser preciso.
