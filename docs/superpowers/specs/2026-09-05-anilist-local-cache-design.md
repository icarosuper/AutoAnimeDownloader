# Cache local dos animes — manter o app utilizável com a AniList fora do ar

**Data:** 2026-09-05
**Status:** implementado em 2026-09-07. A decisão final está registrada em
[decisions.md #92](../../agents/decisions.md#92-o-cache-em-disco-da-anilist-volta-com-ttl-zero-e-o-valor-servido-dele-vem-acompanhado-de-errfromcache);
este arquivo fica como o levantamento que a originou, com as correções de rota que a
implementação obrigou (ver "O que mudou na implementação" no fim)
**Origem:** "salvar as informações dos animes em disco ou memória pra poder continuar funcionando o
app mesmo quando o anilist cai — a coisa mais agravante é não poder ver e gerenciar episódios na
página do anime"
**Escopo previsto:** `src/internal/anilist` (persistência dos `ttlCache`, ampliação do `getStale`,
**cache novo ou desvio de leitura para o caminho por media id**), `src/internal/files` (arquivo novo
na pasta de configs), `src/internal/api` (fallback do `resolveMediaList`; rota de capa, se o item 4
entrar), `src/internal/daemon` (regra do passe em modo cache, item 5).

---

## O estado hoje

O cache da AniList é todo `ttlCache` em memória (`src/internal/anilist/anilist.go:23`) e morre no
restart. O fallback de dado vencido existe — `getStale` (`anilist.go:46`) — mas só é acionado quando
o **gate de orçamento** recusa a leitura (`ErrBudgetLow`, [decisions.md #72](../../agents/decisions.md#72-o-orçamento-da-anilist-se-mede-pelos-headers-da-resposta-não-por-um-contador-nosso)).
Numa queda real da API (403 de IP bloqueado, 5xx, timeout) o erro sobe inteiro e a tela quebra.

O que torna isso barato: **a integração é somente leitura**. Não existe nenhuma mutation para a
AniList em lugar nenhum do código Go — `grep -rn 'mutation' src/internal/anilist` não devolve nada.
O cache portanto não precisa de fila de escrita, nem de resolução de conflito, nem de reconciliação
na volta.

### Os cinco caches que existem, e o buraco entre eles

| cache | chave | conteúdo | TTL |
|---|---|---|---|
| `frontendListCache` (`anilist.go:81`) | usuário + status | `[]MediaList` **com progresso** | 60s |
| `customListsCache` (`anilist.go:80`) | usuário + status | `map[int]CustomLists` | 5min / 30s vazio |
| `mediaByIDCache` (`standalone.go:23`) | media id | `*MediaList` **sem progresso de conta** | 60s |
| `searchCache` (`standalone.go:48`) | termo | `[]MediaSearchResult` | 10min |
| `seriesCache` (`series.go:43`) | media id | elo prequel/sequel, só `FINISHED` | 24h |

O caminho que os itens 1 e 2 abaixo usam — `getMediaListEntry` (`anilist.go:753`), por baixo de
`GetAnimeInfo` (`anilist.go:663`) — **não passa por cache nenhum**. Ou seja: "gravar em disco o que
já se guarda em memória" **não** cobre o item mais agravante. É a primeira coisa a desenhar, não um
detalhe de implementação.

## O que passa a funcionar, em ordem de dor

### 1. Ver e gerenciar episódios na página do anime — *o mais agravante*

`handleAnimeEpisodes` chama `resolveMediaList` (`src/internal/api/endpoint_anime_episodes.go:74` →
`src/internal/api/standalone.go:30`) com `PriorityCritical`, e por baixo cai em `GetAnimeInfo` —
sem cache. Falhou, devolve 500 e a tela não abre, nem para olhar.

Todo o resto daquela tela já é local: bloquear, desbloquear, marcar como manual, apagar, re-baixar,
e o progresso do avulso (que grava em `AnimeSettings`, não na AniList). O único dado que falta é a
`MediaList` — dela sai a lista de episódios (`anilist.EpisodeList`), o `progress` que marca os
assistidos e o total. Com ela em disco, a tela inteira volta.

**Forma preguiçosa, sem cache novo:** essa `MediaList`, com progresso e tudo, já vem inteira dentro
do `frontendListCache` para qualquer anime em status de download. Fazer `resolveMediaList` cair num
lookup por media id na lista persistida cobre exatamente o caso que dói — anime acompanhado — e
deixa de fora só o avulso fora de status, que é o mesmo limite já aceito adiante.

### 2. Baixar episódio à mão

`resolveAnimeDetails` (`src/internal/daemon/manual_download.go:63`) precisa de título, `relations` e
status para montar a busca no Nyaa. Mesmo caminho do item 1 (`GetAnimeInfo`, com fallback para
`GetMediaByID`) e portanto mesma solução: resolvido o item 1, o download manual sobrevive à queda —
o Nyaa não caiu junto.

### 3. A lista de animes não encolher

`GET /animes` já sobrevive **parcialmente**: o arquivo `downloaded_episodes` segura nome, total e
episódios baixados, e a falha da busca de lista é tratada (`mergeFailed`,
`src/internal/api/endpoint_animes.go:189`, com o skip em `:214`) justamente para não virar uma
rajada de `GetAnimeInfo` por anime órfão.

O mapa é semeado pelos episódios em disco (`endpoint_animes.go:126-131`) e **depois** mesclado com a
lista da AniList — o merge existe precisamente para o anime de zero episódio aparecer
(`endpoint_animes.go:182`). É esse merge que cai: com a AniList fora, anime acompanhado com **zero**
episódio baixado some da tela. É o caso do anime que acabou de entrar na lista — exatamente o que se
quer ver.

### 4. Capas

O `cover_image` é URL do CDN da AniList (`s4.anilist.co`), servida direto pelo `Cover.svelte`
(`src/internal/frontend/src/components/ui/Cover.svelte`), que já tem fallback hachurado para quem
não tem capa. Baixar o arquivo uma vez para a pasta de configs e servir por rota própria resolve
dois casos: o CDN cair junto com a API, e o anime cuja capa nunca mais será buscada.

Menos urgente que os três acima — nada deixa de funcionar sem capa. É o que faz a tela *parecer*
funcionando em vez de parecer quebrada.

### 5. Ciclo de verificação do daemon

`AnimeVerification` aborta com `errCauseAnilist` quando a busca de lista falha
(`src/internal/daemon/verification.go:197`). Com cache dá para rodar o passe em cima da última lista
conhecida: os episódios que já tinham ido ao ar continuam baixáveis, e o Nyaa está no ar.

**O risco é o `progress` vencido — e é o cache que o cria.** A degradação segura de hoje não é
acidente: está escrita e comentada em dois lugares. Conta sem resposta não entra em `inDeleteStatus`
(`verification.go:178-179`), e `allAccountsAgreeOnDelete` devolve `false` tanto para conta ausente
do mapa (`verification.go:449-453`) quanto para `GetMediaListStatus` com erro (`:463-466`). Sem
resposta, ninguém deleta.

Servir lista do disco **desfaz** essa proteção sem tocar em nenhuma dessas linhas: a entrada vencida
entra em `inDeleteStatus` como se fosse resposta fresca, e a unanimidade passa a se formar sobre
status de ontem — apagando episódio de anime que o usuário moveu de status desde então. A decisão a
tomar antes de escrever código não é tornar explícito um acidente, é o contrário: **decidir que o
modo cache não alimenta `inDeleteStatus`**, ou seja, passe em modo cache baixa e nunca deleta.

### 6. Índice de séries

`resolveSeriesIndex` (`src/internal/daemon/coverage.go:31`) é `PriorityCritical` e alimenta a
cobertura de pack — mas **não é ponto de quebra**: `GetSeriesIndex` devolve o que conseguiu resolver
e id ausente só não participa da adoção neste passe (`coverage.go:29-30`). O efeito da queda é
qualidade de cobertura, não tela quebrada.

Em troca, é o item mais barato: o `seriesCache` já tem TTL de 24h e guarda só `FINISHED`
(`series.go:43,45,199`) porque relação prequel/sequel de anime lançado não muda mais. O dado de
maior validade de todos já é o único com cache longo — persistir esse mapa é quase só serializar.

## O que não dá para resolver com cache

- **Busca da tela de adicionar anime** — termo livre; só volta do cache o termo já buscado antes
  (`searchCache`, TTL de 10 min, `src/internal/anilist/standalone.go:48`). Persistir ajuda pouco e
  não se sustenta sozinho como motivo.
- **Adicionar anime que nunca passou pelo app** — sem `MediaList` guardada não há o que servir.
  Aceitável: é a única tela que legitimamente exige a AniList no ar.

## Forma preguiçosa

Um arquivo JSON na pasta de configs, ao lado de `downloaded_episodes` e companhia, gravado pelo
`writeAtomic` que já existe (`src/internal/files/filemanager.go:369`), com o conteúdo do
`frontendListCache` (que já é `[]MediaList` com progresso) mais o `seriesCache`. Carrega no boot
para dentro dos `ttlCache` que já estão lá, grava a cada leitura bem-sucedida.

Sem banco e sem camada nova: o `ttlCache` continua sendo a única frente de leitura e o disco é só o
que o faz sobreviver ao restart. As mudanças de comportamento são duas, e ambas mexem em regra
documentada:

1. `getStale` passa a valer para qualquer falha, não só `ErrBudgetLow` — o que contradiz o comentário
   dele hoje ("Nada mais deve chamar", `anilist.go:43-45`) e a [decisions.md #72](../../agents/decisions.md#72-o-orçamento-da-anilist-se-mede-pelos-headers-da-resposta-não-por-um-contador-nosso).
   Entrada nova em `decisions.md` junto.
2. `resolveMediaList` ganha um fallback por lookup na lista persistida, em vez de exigir cache novo
   por media id.

Para o aviso na tela, aproveitar o que já existe: `anilist.Health` (`src/internal/anilist/health.go:25`)
e o `SystemBanner.svelte` (`src/internal/frontend/src/components/shell/SystemBanner.svelte`) já
mostram a AniList degradada. Basta dizer "servindo do cache de \<data\>" em vez de inventar aviso
novo.

## O que falta decidir

- Um arquivo só ou um por tipo de dado (lista × índice de séries)?
- TTL em disco: cache de ontem serve? De uma semana? Ou serve sempre, e o que importa é só avisar
  a idade na tela?
- O passe do daemon em modo cache: baixa e nunca deleta (recomendado, item 5)? Nem roda?
- Capas: baixar por demanda na primeira renderização ou junto do passe? (item 4)
- Avulso fora de status de download não aparece no `frontendListCache`. Aceitar o buraco, ou
  persistir também o `mediaByIDCache` (que não tem progresso de conta, e portanto não marca
  assistidos)?

---

## O que mudou na implementação

Três coisas que o levantamento errou e a leitura do código corrigiu:

1. **A `MediaList` da tela de detalhe não está no `frontendListCache`.** A query do poll não pede
   `relations` nem o `status` da entrada — os dois campos que o download manual usa para montar a
   busca no Nyaa. Serviu de nada reaproveitar; entrou um cache próprio sobre o `getMediaListEntry`,
   que já pedia o superconjunto. Daí os três caches de `MediaList`, e não um.
2. **O item 4 (capas) não existe como trabalho.** `coverImage` é campo de `Media`, então persistir o
   `frontendListCache` já persiste as URLs. Nenhum arquivo em disco, nenhuma rota nova.
3. **O item 5 estava com o dedo no lugar errado, e a decisão foi a oposta da recomendada.** Não é o
   `progress` que morde (não existe deleção por progresso — `deletableMediaIDs` decide só por status
   de lista), e a degradação segura de hoje não é acidente: está comentada em três pontos de
   `verification.go`. O risco real é o cache **desfazer** essa proteção. O dono do projeto decidiu
   deixar o passe em modo cache deletar; o cenário concreto e o caminho de volta de uma linha estão
   na #92 e num `ponytail:` em `verification.go`.

E uma coisa que o levantamento não tinha: **`timeUntilAiring` é relativo ao instante da busca**, e é
o campo que todo gate de "já foi ao ar" lê. Cacheado cru, ele congela e o passe conclui para sempre
que nada novo estreou. `stampAiringAt`/`rebaseAiring` resolvem, e é isso que faz o episódio que
faltava dez minutos quando a AniList caiu entrar na busca sozinho.

### Como as perguntas abertas foram respondidas

- **Um arquivo só**, `anilist_cache`, com todas as coleções dentro.
- **Sem TTL em disco.** Serve sempre, e a idade aparece na tela (`anilist.cache_saved_at` →
  `SystemBanner`). O que protege contra dado velho não é o TTL do arquivo, é o snapshot voltar com
  TTL zero — visível só para o caminho de falha.
- **O passe baixa e também deleta**, por decisão explícita (acima).
- **Capas**: nem por demanda nem no passe — a URL vem de graça no snapshot da lista.
- **Avulso**: coberto. O `mediaByIDCache` entrou no snapshot e ganhou fallback, então a parte
  avulsa da lista também para de encolher. Continua de fora só o anime que **nunca** passou pelo
  app, que era o limite já aceito.
