# 89. There is no OpenAPI spec: the `architecture.md` table is the only endpoint doc

**Location:** `api/server.go` — `SetupRoutes` (there is no `/swagger/` route); `docs/agents/architecture.md`
— "API" section. Negative fact: **there is no** `docs/swagger/`, nor a `// @` annotation on a
handler, nor `swag` in `go.mod`.

**What it looks like:** forgetfulness. Every daemon with REST has a `/swagger/`, the handlers are
ready to take `@Summary`/`@Param`, and putting it back is a `swag init`.

**Why it left:** measured — `swaggo/http-swagger` + `swaggo/swag` cost **11.2 MB in a 34 MB binary
(33%)**, which goes in every AUR and GitHub release on three platforms. In exchange: 343 lines of
annotation in 17 files, 6,482 generated lines, ~15 indirect `go-openapi` deps and a mandatory
regeneration step on every commit that touched the API.

What decided was not the size, it was **who read it**: nothing consumed the spec. There is no
client generator in `package.json`, in the `Makefile` or in the scripts — `lib/api/client.ts` is
written by hand. The only reader was the `/swagger/` console in the browser, which
`curl localhost:8091/api/v1/...` replaces.

`@Param`/`@Success` only repeated type and HTTP code and died with no substitute. But **11
`@Description`s carried an invariant that was nowhere else** — `bytes_completed` null on a paused
torrent because rain frees the piece data; `resume` puts it at the END of the queue and is not
"start now"; `prioritize` demotes the least-progressed and rain keeps the bitfield; batch
prioritize ignores an unknown hash on purpose; the deletion boundary is the torrent, not the
episode. Those became a **normal doc comment** on top of the handler, which is better than the
annotation: it shows up in `go doc` and in the editor hover, and does not depend on regeneration
to be right.

**Don't "fix" by:**
- Re-adding `swag` "because the new endpoint deserves docs": the new endpoint's docs go in the
  `architecture.md` table (step 7 of the checklist in `conventions.md`) and, when there is a
  behaviour invariant, in a doc comment on top of the handler.
- Generating the spec without serving the route "because it's free": it is not — it brings back
  the regeneration step on every API commit and a second copy of the contract to go stale. If one
  day it needs importing into a Bruno/Insomnia, generating on demand and not versioning is cheaper
  than keeping it.
