# CLAUDE.md

Guidance for Claude Code (claude.ai/code) in this repo.

## Project Overview

AutoAnimeDownloader = Go daemon. Auto-downloads anime episodes. Integrates Anilist (watch list), Nyaa (torrent scraper), and an embedded BitTorrent client (github.com/cenkalti/rain/v2 — no external qBittorrent). Has embedded Svelte web UI + REST API.

## Important — After Any Change

1. **Run tests**: `go test ./...`
2. **Update docs**: if you changed endpoints, config fields, env vars, patterns, or added/removed files → update the relevant doc in `docs/agents/`; if you introduced a non-obvious or unusual pattern → add an entry to `decisions.md`
   - Arquivo `.go` novo em `src/internal/` ou `src/cmd/` → seção nova em `architecture.md`, listando cada símbolo **exportado** (e os não exportados que importam) com uma linha de propósito. Arquivo apagado → seção apagada. É isso que mantém o `architecture.md` sendo a fonte única de "o que existe e onde"
   - Entrada nova em `decisions.md` → número seguinte (o maior que existe hoje é **#82**, e as entradas não estão em ordem numérica no arquivo) + uma linha no Índice do topo. Ao referenciar uma decisão, sempre com âncora: `[decisions.md #43](decisions.md#43-a-identidade-de-um-anime-é-mediaid-status-é-uma-pergunta-por-conta-download--or-deleção--and)`
3. **Regenerate Swagger**: if API changed → `swag init -g src/cmd/daemon/main.go -o docs/swagger`
4. **NEVER COMMIT OR PUSH WITHOUT PERMISSION**: the user will do that themselves

## Padrão de commit

Conventional Commits, mensagem em **português**, só a linha de assunto — sem corpo, sem
`Co-Authored-By`, sem ponto final.

`tipo: verbo no infinitivo + o que mudou` — tudo minúsculo depois do `tipo:`.

Tipos em uso: `feat`, `fix`, `docs`, `refactor`, `test`, `chore` (`!` depois do tipo para breaking
change, ex.: `chore!: remover migração de save_path`).

O assunto diz o comportamento, não o arquivo: descreva a regra que passou a valer, e quando houver
um "em vez de" que explique a mudança, ele cabe no assunto.

```
feat: escolher pack por cobertura da janela em vez de marcador de part
fix: servir cache vencido de customLists quando o gate recusa
docs: registrar que pack sem faixa não passa pela cobertura
```

## Branch

O padrão é commitar direto na `master` — não crie branch por conta própria. Só quando for uma
feature maior, que vai exigir vários commits, **pergunte antes** se o usuário quer uma branch.

## Important — Código ruim no caminho

Achou código ruim **perto** do que você está mexendo — mal escrito, deprecado, não utilizado,
desatualizado, não otimizado, duplicado, comentário mentindo sobre o código — **pergunte antes de
agir**. Sempre as três opções, e o usuário escolhe:

1. **aproveitar e ajustar agora**
2. **só deixar um TODO** (no código, junto do trecho)
3. **ignorar**

Nunca decida sozinho por nenhuma das três — nem "arrumo de passagem porque é rápido", nem "deixo
quieto porque está fora do escopo". Diga o que é, onde está (`arquivo:linha`) e por que é ruim, em
uma ou duas linhas, e pergunte. Vale para o que você encontra de passagem; o que o pedido do
usuário já cobre não é "de passagem", é o trabalho.

## Docs

- [Architecture](docs/agents/architecture.md) — read when navigating code or adding features; update when adding packages, symbols, or API endpoints; maps all packages, symbols, data flow, API endpoints
- [Conventions](docs/agents/conventions.md) — read when writing Go code; error handling, handler pattern, naming, checklists for new endpoints/config fields
- [Config Reference](docs/agents/config.md) — read when touching `config.json` fields
- [Environment Variables](docs/agents/environment.md) — read when touching env vars or dev setup
- [Testing](docs/agents/testing.md) — read when writing or running tests; mock patterns
- [Commands](docs/agents/commands.md) — read when building or running the daemon/frontend
- [Decisions](docs/agents/decisions.md) — read before "fixing" unusual patterns; 82 entradas autocontidas. **Comece pelo Índice do topo e leia só a entrada apontada** — o arquivo inteiro tem 2k linhas e nunca precisa ser lido de ponta a ponta
- [Troubleshooting Downloads](docs/agents/troubleshooting-downloads.md) — follow when animes fail to download; diagnose search/filter logic bugs step by step
- [Sources](docs/agents/sources.md) — read before adding a torrent source; measured limits of Nyaa, the AnimeTosho JSON API, and why the alternatives were rejected
