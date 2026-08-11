# Reorganização da tela de Configurações

Data: 2026-08-11
Arquivo principal: `src/internal/frontend/src/routes/Config.svelte`

## Problema

A tela tem 18 campos repartidos em quatro grupos que não descrevem o que o usuário está tentando
decidir:

| Grupo hoje | Campos |
|---|---|
| Downloads (9) | pasta de animes completos\*, deletar assistidos + qtd. a manter, máx. ep. batch, tam. máx. batch, tam. máx. episódio, mín. seeders, máx. páginas de busca, espaço livre mín., renomear p/ Jellyfin |
| Anilist (4) | usuários, status para baixar, status de exibição, status para deletar |
| Automação (4) | intervalo de verificação, máx. episódios por anime, limite de tentativas, máx. downloads simultâneos |
| Filtros (1) | listas excluídas |

Quatro defeitos concretos:

1. **"Downloads" mistura três assuntos** — onde os arquivos ficam (pasta, renomear, disco), quanto
   se guarda (deletar assistidos) e como o torrent é escolhido (seeders, páginas, tamanhos).
2. **A retenção está partida em dois grupos** — `deletar assistidos` em Downloads e
   `máx. episódios por anime` em Automação respondem à mesma pergunta ("quantos episódios ficam no
   disco") e o usuário precisa trocar de grupo no meio da decisão.
3. **"Filtros" tem um campo só**, e esse campo é uma lista do próprio AniList.
4. **Nenhuma hierarquia visual dentro do card** — todo campo é uma linha de largura total com
   rótulo em cima, dica no meio e input embaixo. Um número de dois dígitos ocupa a mesma largura
   que um caminho de sistema de arquivos, e o olho desce em zigue-zague em vez de escanear uma
   coluna de valores.

## Não-objetivos

- Nenhuma mudança de comportamento de escrita. Salvar continua o único caminho até `PUT /config`,
  sem autosave e sem debounce — `PUT /config` valida tudo de uma vez e faz I/O de filesystem
  (`Librarian.ProbePath`), então salvar durante a digitação renderia 400 a cada tecla.
- Nenhum campo criado, removido ou renomeado no backend. `Config` (Go) e o contrato da API ficam
  intactos; isto é reorganização de frontend.
- Prioridades e Notificações continuam telas próprias, com Salvar próprio.

## Arquitetura de informação

Quatro grupos, agrupados pelo objetivo do usuário, mais dois links de saída no fim do índice.

### Biblioteca (`library`) — grupo inicial

| Campo | Controle | Variante |
|---|---|---|
| `completed_anime_path` \* | texto | empilhada |
| `rename_files_for_jellyfin` | toggle | inline |
| `min_free_disk_percent` | número, sufixo `%` | inline |

Vira o grupo inicial (hoje é `downloads`) porque hospeda o único campo obrigatório da tela, e é
para onde `#/config?missingConfig=true` precisa levar — hoje o daemon manda o usuário para essa URL
e ele cai num grupo que pode não conter o campo que falta.

### Anilist (`anilist`)

| Campo | Controle | Variante |
|---|---|---|
| `anilist_usernames` | chips | empilhada |
| `excluded_lists` | chips | empilhada |
| `download_statuses` | pills | empilhada |
| `download_media_statuses` | pills | empilhada |
| `delete_statuses` | pills | empilhada |

`excluded_lists` era o grupo "Filtros" inteiro. Fica logo abaixo de `anilist_usernames`, antes dos
status, para a seção ler como "quais contas → o que ignorar nelas → o que baixar/deletar".

### Downloads (`downloads`)

| Campo | Controle | Variante |
|---|---|---|
| `check_interval` | número, sufixo `min` | inline |
| `max_concurrent_downloads` | número | inline |
| `max_episodes_per_anime` | número | inline |
| `delete_watched_episodes` | toggle | inline |
| `watched_episodes_to_keep` | número, revelado pelo toggle acima | inline |

`max_episodes_per_anime` vem de "Automação" para ficar adjacente a `delete_watched_episodes` — as
duas são retenção. `watched_episodes_to_keep` continua condicionado a `delete_watched_episodes`,
mas perde o recuo `pl-11`: aquele recuo alinhava o campo com o rótulo do toggle quando a chave
ficava à esquerda, e na variante inline a chave passa para a direita. Ele fica no mesmo bloco de
linha do toggle (sem divisor entre os dois), que é o que expressa a dependência agora.

### Busca de torrents (`search`)

| Campo | Controle | Variante |
|---|---|---|
| `min_seeders` | número | inline |
| `max_search_pages` | número | inline |
| `episode_retry_limit` | número | inline |
| `max_batch_episodes` | número | inline |
| `max_batch_torrent_size_gb` | número, sufixo `GiB` | inline |
| `max_episode_torrent_size_gb` | número, sufixo `GiB` | inline |

É o grupo "avançado", mas com nome real em vez de gaveta de sobras: os seis campos são o mesmo
assunto — como um candidato do Nyaa é aceito ou descartado. Os três que valem para toda busca vêm
primeiro; os três de batch/tamanho depois.

### Índice lateral

```
┌───────────────┐
│ Biblioteca    │
│ Anilist       │
│ Downloads     │
│ ───────────── │  divisor: o que vem abaixo é ajuste fino
│ Busca de      │
│ torrents      │
│ ───────────── │  divisor: o que vem abaixo sai da tela
│ Prioridades ↗ │  navega para #/priorities
│ Notificações ↗│  navega para #/notifications
└───────────────┘
```

Os dois últimos itens são **navegação**, não grupos: mudam a rota em vez de trocar `activeGroup`, e
carregam um ícone de seta externa para dizer isso antes do clique. Eles existem para que
Configurações seja a porta única de tudo que é configuração — hoje Prioridades e Notificações só
são alcançáveis pelo menu "Mais", apesar de gravarem no mesmo `PUT /config`. Continuam também no
menu "Mais" (`navItems.ts` não muda).

No mobile o índice já é uma faixa rolável horizontal; os divisores viram separadores verticais e os
dois links ficam no fim da faixa.

## Anatomia da linha

Duas variantes, escolhidas pelo tipo de dado. Forçar um chips input ou um grupo de pills numa
coluna estreita à direita seria pior que o problema que estamos consertando.

**Inline** — números e toggles. Rótulo e dica na coluna esquerda, controle à direita:

```
┌──────────────────────────────────────────────┐
│ Espaço livre mínimo em disco       [ 10 ] %  │
│ Abaixo dessa porcentagem o download é        │
│ pausado. Use 0 para desligar a guarda.       │
└──────────────────────────────────────────────┘
```

**Empilhada** — caminho, chips e pills. Rótulo, dica, controle em largura total; é o que a tela já
faz hoje.

Abaixo de 768px tudo colapsa para empilhado, sem exceção.

### Implementação

`Input.svelte` ganha dois props opcionais, ambos com default que preserva o comportamento atual
(o componente é usado também em AddAnime, AnimeDetail e Notifications — nenhuma dessas telas muda):

- `inline: boolean = false`
- `suffix: string = ""` — texto de unidade renderizado ao lado do input, `aria-hidden` (a unidade
  já está na dica ou é redundante com o rótulo; anunciá-la duplicaria).

O layout inline sai só de posicionamento explícito de grid sobre a marcação que já existe, sem
duplicar o `<input>` (que já está duplicado entre o ramo `number` e o ramo genérico por causa da
limitação do Svelte com `type` dinâmico + `bind:value` — duplicar de novo por causa de layout
multiplicaria os ramos para quatro):

| Elemento | Colocação quando `inline` |
|---|---|
| container | `md:grid md:grid-cols-[1fr_auto] md:gap-x-4` |
| `<label>` | `md:col-start-1 md:row-start-1` |
| `<input>` | `md:col-start-2 md:row-start-1 md:w-24 md:self-start` |
| dica | `md:col-start-1 md:row-start-2` |
| erro | `md:col-start-1 md:row-start-3` |

`Toggle.svelte` ganha `inline: boolean = false`, que aplica `w-full flex-row-reverse
justify-between` no `<label>` externo — chave à direita, rótulo à esquerda, sem tocar na estrutura
do input nem na associação de rótulo.

## Textos

### Seções

| Chave | pt-BR | en |
|---|---|---|
| `config_section_library` (nova) | Biblioteca | Library |
| `config_section_anilist` | Anilist | Anilist |
| `config_section_downloads` | Downloads | Downloads |
| `config_section_search` (nova) | Busca de torrents | Torrent search |
| `config_section_automation` | *removida* | *removida* |
| `config_section_filters` | *removida* | *removida* |

### Rótulos

Quatro rótulos perdem a unidade entre parênteses, que passa a ser o `suffix` do campo:

| Chave | pt-BR novo | en novo | suffix |
|---|---|---|---|
| `config_label_check_interval` | Intervalo de verificação | Check Interval | `min` |
| `config_label_min_free_disk` | Espaço livre mínimo em disco | Min Free Disk Space | `%` |
| `config_label_max_batch_size` | Tamanho máx. do torrent batch | Max Batch Torrent Size | `GiB` |
| `config_label_max_episode_size` | Tamanho máx. do torrent de episódio | Max Episode Torrent Size | `GiB` |

Os sufixos são literais no template, sem chave de i18n: `min`, `%` e `GiB` são as mesmas três
strings nos dois idiomas.

Os rótulos em **pt-BR** passam de Title Case para sentence case. Hoje a mesma tela mostra
"Máx. Episódios por Anime" ao lado de "Deletar episódios assistidos automaticamente" — a Title Case
é convenção do inglês e chegou por tradução literal. Em `en` a Title Case fica como está.

| Chave | pt-BR novo |
|---|---|
| `config_label_completed_path` | Pasta de animes completos |
| `config_label_watched_keep` | Episódios assistidos a manter |
| `config_label_max_episodes` | Máx. episódios por anime |
| `config_label_retry_limit` | Limite de tentativas por episódio |
| `config_label_max_concurrent` | Máx. downloads simultâneos |
| `config_label_max_batch_episodes` | Máx. episódios para batch |
| `config_label_min_seeders` | Mínimo de seeders |
| `config_label_max_search_pages` | Máx. de páginas na busca |
| `config_label_download_statuses` | Status para baixar |
| `config_label_download_media_statuses` | Status de exibição para baixar |
| `config_label_delete_statuses` | Status para deletar |
| `config_label_excluded_list` | Listas excluídas |

`config_label_username` ("Usuários"), `config_label_delete_watched` e `config_label_rename_jellyfin`
já estão em sentence case e não mudam.

### Dicas

`config_hint_rename_jellyfin` passa a ser renderizada sempre, como `subtitle` da linha. Hoje ela só
aparece depois que o toggle é ligado, o que esconde exatamente a informação que ajuda a decidir se
vale ligar.

Nenhuma outra dica muda de texto.

### Links de saída

Reutilizam `nav_priorities` e `nav_notifications` — as strings já existem e dizem a mesma coisa.

### Recompilação

Mensagens vivem em `messages/{pt-BR,en}.json` e são compiladas para `src/lib/i18n/messages/` pelo
paraglide (`bun run i18n:compile`, já embutido em todos os scripts de build e teste). O diretório
compilado é gitignored — só os dois JSON entram no commit.

## Validação

`requiredChecks` continua uma lista de `{ group, ok, message }` e continua sendo fonte única de
duas coisas: o toast do Salvar e o ponto de pendência no índice. É essa unicidade que impede o
ponto de mentir quando uma regra mudar. Só o campo `group` é remapeado:

| Validação | Grupo novo |
|---|---|
| `completed_anime_path` não vazio | `library` |
| `min_free_disk_percent` 0..99 | `library` |
| `check_interval` > 0 | `downloads` |
| `max_episodes_per_anime` > 0 | `downloads` |
| `max_concurrent_downloads` >= 0 | `downloads` |
| `watched_episodes_to_keep` >= 0 | `downloads` |
| `episode_retry_limit` >= 0 | `search` |
| `max_batch_episodes` >= 0 | `search` |
| tamanhos de torrent >= 0 | `search` |
| `min_seeders` >= 0 | `search` |
| `max_search_pages` >= 0 | `search` |

Nenhuma regra é adicionada nem removida. Continua sem validação de conta AniList no frontend: com
animes avulsos (`#/add`) o app funciona sem lista nenhuma, e o backend não exige a conta — uma
obrigatoriedade só no cliente seria uma regra que o servidor desconhece.

## Testes

`tests/smoke/config.spec.ts` está com **8 de 10 testes vermelhos no master** antes desta mudança.
Três causas, todas anteriores a esta mudança:

1. **O fixture `baseConfig` (`tests/smoke/helpers.ts`) está incompleto.** Faltam sete campos
   (`max_concurrent_downloads`, `max_batch_episodes`, os dois de tamanho, `min_seeders`,
   `max_search_pages`, `min_free_disk_percent`). `loadConfig` faz `config = { ...data }`, então
   ausente vira `undefined`, e `undefined >= 0` é `false` — toda validação numérica reprova e o
   Salvar nunca chega ao `PUT`. Consertar o fixture é pré-requisito de qualquer teste de Salvar.
2. Testes esperam **Anilist como grupo inicial**, mas o inicial é `downloads` desde a fase 6.
3. Um teste espera a validação de **"pelo menos um usuário AniList"**, removida quando a conta
   virou opcional.

O arquivo é reescrito:

1. **Um teste por grupo** — abre o grupo pelo índice e afirma que seus rótulos estão na tela. É a
   única forma de um campo se perder nesta mudança: o `PUT` manda `config` inteiro por spread, então
   o payload não corre risco; o risco é um campo ficar invisível por não ter sido movido.
2. **Troca de grupo pelo índice** — Biblioteca → Downloads, com o campo do grupo anterior sumindo
   do DOM (o layout continua sendo um grupo por vez).
3. **Validação traz o grupo do campo reprovado à tela** — esvazia a pasta, navega para outro grupo,
   clica em Salvar, afirma o toast e que Biblioteca voltou; e que nenhum `PUT` saiu.
4. **Ponto de pendência por grupo** — com config incompleta, o índice marca só os grupos com
   pendência, e a marca some quando o campo é preenchido. Cobre os dois caminhos de binding que
   alimentam `pendingGroups`: `Input.svelte` (pasta) e `ChipsInput` (usuários).
5. **Pills de baixar/deletar mutuamente exclusivas**, com o resultado chegando no corpo do `PUT`.
6. **Chips adiciona com Enter e o valor chega no `PUT`.**
7. **Os dois links de saída navegam** para `#/priorities` e `#/notifications`.

**Sem teste automatizado para a variante inline.** Os props novos de `Input.svelte` e
`Toggle.svelte` são opt-in com default idêntico ao comportamento atual, e o que eles mudam é
colocação de grid — um teste que afirmasse as classes estaria testando a string de classe, não um
comportamento. A verificação é visual, com `bun run dev`, nas duas larguras (desktop e < 768px).

## Documentação

- `docs/agents/architecture.md` linha 628, entrada `routes/Config.svelte`: hoje lista
  "(Anilist / Downloads / Automation / Filters)" e fala em "the six validations" — as duas
  informações estão desatualizadas (são onze validações desde a spec de limite de batch). Passa a
  listar os quatro grupos novos, os dois links de saída e a contagem correta.
- `docs/agents/architecture.md` linha 654, entrada `Input.svelte`: documenta os props `inline` e
  `suffix`. A frase "Only `Config.svelte` consumes it" precisa ser conferida contra os consumidores
  reais antes de ser mantida.
- `docs/agents/config.md`: descreve os campos do `config.json`, não a UI — não muda.
- `docs/agents/decisions.md`: nada a acrescentar. A escolha de duas variantes de linha em vez de
  uma é convencional, não um padrão incomum que alguém tentaria "consertar".
- Sem mudança de API, então sem `swag init`.
