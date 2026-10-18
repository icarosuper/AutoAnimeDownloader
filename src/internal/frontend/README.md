# Frontend

Svelte 5 SPA embedded in the daemon (`//go:embed dist` in `embed.go`). The package manager is
**Bun**, not npm.

```bash
bun install
bun run dev      # Vite, default http://localhost:5173
bun run build    # writes dist/, which the daemon serves
```

Day-to-day: `make dev` from the repo root runs Vite and the Go daemon together. There is no
`.env` file — Vite's port is `VITE_PORT` (default 5173); the daemon proxies `/api`.

Contributor workflow: [development.md](../../../docs/guides/development.md).
Pages: [webui.md](../../../docs/guides/webui.md).
