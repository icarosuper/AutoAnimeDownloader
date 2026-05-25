# Test Infrastructure — Plano de Melhorias

## Status atual

- Testes de backend (unit + integração) já funcionam
- Lentidão nos testes de integração foi resolvida: `time.Sleep` fixo substituído por polling (`pollStatus`/`pollLastCheck` a 100ms) — de ~11s para ~1s de tempo real
- Testes de frontend não existem ainda

---

## 1. Dockerfile de integração sem build do frontend

**Problema:** o Dockerfile atual de testes builda o frontend completo (bun install + bun build ≈ 12s) desnecessariamente — os testes de integração só usam a API.

**Fix:** criar Dockerfile separado para testes que gera um `dist` fake para satisfazer o `//go:embed`:

```dockerfile
FROM golang:1.24-alpine AS go-builder
WORKDIR /build
COPY go.mod ./
RUN go mod download
COPY . .
RUN mkdir -p src/internal/frontend/dist && \
    echo "<html></html>" > src/internal/frontend/dist/index.html
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/daemon ./src/cmd/daemon
```

Usar esse Dockerfile no `docker-compose.test.yml` em vez do `docker/Dockerfile` principal.

---

## 2. Testes de frontend

Implementar três camadas, todas com backend mockado (sem daemon real):

| Tipo | Ferramenta | O que testa |
|---|---|---|
| Unit | Vitest | Funções/stores isolados |
| Component | Vitest + Testing Library | Componentes Svelte renderizados |
| Smoke | Playwright | Fluxos críticos da UI end-to-end |

Nenhum desses testes precisa do daemon rodando — o backend é sempre mockado.

E2E real (frontend + backend integrados) é fora de escopo por enquanto.

---

## 3. Makefile — targets de teste + polida geral

### Targets a adicionar

```makefile
make test                       # tudo
make test-backend               # unit + integration
make test-backend-unit          # go test ./src/tests/unit/... ./src/internal/...
make test-backend-integration   # Docker + daemon (scripts/run-integration-tests.sh)
make test-frontend              # unit + component + smoke
make test-frontend-unit         # Vitest (unit)
make test-frontend-component    # Vitest (component)
make test-frontend-smoke        # Playwright
```

Usar hífens (convenção Make — tem autocomplete no shell).

### `make test` — script com resumo

`make test` chama `scripts/run-all-tests.sh` em vez de invocar targets diretamente. O script:
- Roda cada suite e captura exit code (continua mesmo se uma falha)
- Imprime resumo no final:

```
=== Test Summary ===
[PASS] backend-unit
[FAIL] backend-integration
[PASS] frontend-unit
===
1 failed
```

Targets individuais (`make test-backend-unit` etc.) ficam simples — sem script, sem resumo. Só o `make test` tem esse comportamento.

### Polida geral

Dar uma olhada nas opções atuais de build/release e ver o que pode ser removido ou agrupado para não poluir o `make help`.

---

## 4. CI — novos jobs

Espelhar os targets do Makefile em jobs separados no `build.yml`:

```
test-backend-unit        # já existe (dentro do job "build"), extrair
test-backend-integration # já existe, manter
test-frontend-unit       # novo — só precisa de bun
test-frontend-component  # novo — só precisa de bun
test-frontend-smoke      # novo — só precisa de Playwright (backend mockado)
```

Os jobs de frontend não precisam do Docker nem do daemon.

---

## Ordem sugerida de implementação

1. Dockerfile de teste sem frontend (ganho imediato no build de integração)
2. Polida no Makefile + targets de teste (backend já funciona, frontend como stubs)
3. Testes de frontend (Vitest unit → Vitest component → Playwright smoke)
4. Jobs de frontend no CI
