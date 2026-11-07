# Dependency upgrades — November 2026

Working checklist for bringing the toolchain and libraries current, split into
three tiers: **what ships in the next release (2.9.0)**, **what is worth doing
but not in this release**, and **what is not worth doing**.

Audit date: **2026-11-07**. Versions below were the latest published that day.
This replaces the October 2026 checklist that lived on the
`chore/dependency-upgrades` branch (commit `b5ebff5`). That branch only held the
checklist; nothing in it was ever applied to `master`.

**IDs are stable** and match the October checklist (`G0`, `F3`, ...). Refer to
them in chat (`taking G1–G3`, `F4 is done`). Do not renumber. Claim a slice by
writing `in progress (YYYY-MM-DD)` next to it; check the box when it is on
`master`.

---

## Rules for every slice

1. **G0 goes first.** rain ≥ 2.4.2 needs Go 1.26+, and the newer `golang.org/x/*`
   resolve better on the new toolchain.
2. One commit per slice (`G0`, `G1`, ...), so a bad slice can be reverted alone.
3. `go test ./...` after every Go slice. After a frontend slice, from
   `src/internal/frontend/`: `bun run test:unit`, `bun run test:component`,
   `bun run check`.
4. Pin the **language** version in `go.mod` and the **image / CI** version in
   the Dockerfiles and workflows in the same slice. Go embeds the stdlib in the
   binary: updating the host Go does not fix a daemon built with 1.25.
5. Before the release: `make sandbox` and check that the daemon built with the
   new toolchain starts, searches and downloads.

---

## Tier 1 — in this release (2.9.0)

Low risk, no API or UI rewrite, and G0/G2 close real security gaps.

### G0 — Go 1.25 → 1.27, Alpine 3.19 → 3.24

Go 1.25 has been EOL since 19 Aug 2026. The October 2026 releases (1.27.2 /
1.27.3) carry HTTP/2 and `crypto/x509` fixes that 1.25 will never get. Alpine
3.19 has been EOL since Nov 2025. The development machine already has go1.27.3.

- [x] `go.mod`: `go 1.25.0` → `go 1.27.0`
- [x] `.github/workflows/build.yml`: `go-version: '1.25'` → `'1.27'` (every job)
- [x] `docker/Dockerfile`: `golang:1.25-alpine` → `golang:1.27-alpine`, `alpine:3.19` → `alpine:3.24`
- [x] `docker/Dockerfile.build.{amd64,arm64,windows}`: `golang:1.25` → `golang:1.27`
- [x] `docker/docker-compose.test.yml`: `golang:1.25-alpine` → `golang:1.27-alpine`
- [x] `src/tests/mocks/{anilist,nyaa}`: `go.mod` and Dockerfile from Go 1.24 → 1.27, Alpine 3.19 → 3.24
- [x] Go version mentioned in `docs/guides/build.md`, `docs/guides/development.md`, `README.md`

### G1 — goquery 1.8.0 → 1.13.0

Oldest pin in the repo (Oct 2021). The API is stable.

- [x] `go get github.com/PuerkitoBio/goquery@v1.13.0`
- [x] Nyaa / HTML parser tests

### G2 — rain 2.3.1 → 2.5.1

The October checklist targeted 2.4.4. Since then 2.4.5, 2.5.0 and 2.5.1 (the
latter on 2026-11-07) came out. **Go straight to 2.5.1**: it rejects torrents
named `.` / `..` and storage paths outside the root directory (a security fix).

**Behavior change in 2.5.0 that affects us:** when deleting a torrent's data
fails, `RemoveTorrent` now returns the error and leaves the torrent in the
session, stopped. Before, the torrent was removed anyway. We call it in
`Session.Remove` (`src/internal/torrents/session.go`), used by
`RemoveTorrentWithEpisodes` (`src/internal/daemon/episodes.go`).

- [x] `go get github.com/cenkalti/rain/v2@v2.5.1`
- [x] Check that `RemoveTorrentWithEpisodes` leaves the episodes and the saved
      state consistent when the removal returns an error, and that the UI/CLI
      show the error instead of a half-removed torrent
- [x] `./src/internal/torrents/...` tests + a real download in the sandbox

Result (2026-11-07): a throwaway test seeded a local torrent from a raw rain
session and downloaded it through our `Session` by magnet (`x.pe`). Download,
seeding and `Remove` with data deletion all worked. With the data dir made
undeletable, `Remove` returned `cannot remove torrent data: ... permission
denied` and the torrent stayed in the session, stopped; a retry removed it.
`removeEpisodesAndLinks` only logs that error and still forgets the episode
records, so the torrent then shows up as an orphan the user can delete by hand
(before 2.5.0 it vanished from the session and left the files on disk unseen).

### G3 — websocket import path

`nhooyr.io/websocket` is deprecated; the maintained line is
`github.com/coder/websocket`, with the same API. Its version number (1.8.15) is
lower than the old pin (1.8.17) because the line was renumbered on the new
path — it is not a downgrade.

- [x] Swap the module in `go.mod` and every import (`src/internal/api/websocket.go` and tests)
- [x] `go get github.com/coder/websocket@latest`

### G4 — remaining Go bumps (patch/minor)

- [x] `github.com/rs/zerolog` 1.34.0 → 1.35.1
- [x] `github.com/jedib0t/go-pretty/v6` 6.7.7 → 6.8.3
- [x] `fyne.io/systray` 1.12.0 → 1.12.2
- [x] `github.com/yuin/goldmark` 1.8.6 → 1.8.7 (new since the October audit)
- [x] `golang.org/x/net` 0.48.0 → 0.62.0 (now a direct dependency)
- [x] `golang.org/x/sys` 0.46.0 → 0.49.0
- [x] `golang.org/x/crypto` 0.46.0 → 0.58.0, `x/text` 0.32.0 → 0.43.0, `x/sync` 0.21.0 → 0.24.0
- [x] `go mod tidy`, `go test ./...`

`lumberjack.v2` 2.2.1 is already the latest.

### F1 — frontend lock refresh inside the current majors

No major bump in `package.json`; only bring the lock to the top of each `^`
range (`bun update` without `--latest`).

- [x] svelte 5.55.0 → 5.57.3, svelte-check 4.4.5 → 4.7.8,
      `@lucide/svelte` 1.27.0 → 1.70.0, `@playwright/test` 1.60.0 → 1.65.0,
      `@inlang/paraglide-js` 2.15.1 → 2.29.3, `@testing-library/svelte` 5.3.1 → 5.4.2,
      typescript 6.0.2 → 6.0.3, vite 6.4.1 → 6.4.5, vitest 3.2.4 → 3.2.7,
      postcss, autoprefixer, `@types/node` 25.x
- [x] Make sure no package jumped a major (vite, vitest, tailwind, jsdom,
      spa-router, jest-dom, typescript, paraglide must stay on their current majors)
- [x] Test trio + `bun run build`, then a look at the sandbox

Local Bun is already 1.4.4; CI uses `bun-version: latest`.

---

## Tier 2 — worth doing, but not in this release

Each one is a breaking major and deserves its own cycle, with a visual pass in
the sandbox. Do them right after 2.9.0, one per slice.

### C1 — GitHub Actions majors

Cheap, but it can only be validated on CI — breaking the release workflow right
when shipping is the worst moment. **First thing after 2.9.0.**

- [ ] `actions/checkout@v4` → v7, `actions/setup-go@v5` → v7 (Node 24)
- [ ] `docker/setup-buildx-action@v3`, `actions/upload-artifact@v4` /
      `download-artifact@v4` → current major
- [ ] Check whether `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24` is still needed

### F2 — Vite 8 + vite-plugin-svelte 7 + Vitest 5

The three move together. Vite 8 uses Rolldown.

- [ ] vite ^6 → ^8 (8.3.6), `@sveltejs/vite-plugin-svelte` ^5 → ^7 (7.3.1), vitest ^3 → ^5 (5.1.0)
- [ ] Read the Vite 7 and 8 migration notes; review `vite.config.js` and `vitest.config.ts`
- [ ] Test trio + production build (embedded in `src/internal/frontend/dist`)

### F3 — Tailwind 3.4 → 4.3

CSS-first config rewrite (`bunx @tailwindcss/upgrade`). Touches `postcss.config.js`,
`src/app.css`, `src/lib/design/tokens.css`. Needs a full visual pass (desktop +
mobile) — bad timing right after the round of UI fixes going into 2.9.0.

### F4 — svelte-spa-router 4 → 5

Svelte 5 rewrite. Touches every route (`src/App.svelte`, `src/lib/navItems.ts`,
every `push(...)` and `params` reader) and the tests that set `window.location.hash`.

### F5 — test DOM stack

- [ ] jsdom 25 → 31 (was 30 in October), `@testing-library/jest-dom` 6 → 7

---

## Tier 3 — not worth doing (for now)

| Item | Why not |
|---|---|
| **O1** — urfave/cli v2 → v3 | Different module and API; would rewrite the whole `src/cmd/cli/`. Nothing is broken on v2.27.7 (the last v2). Revisit only if v2 starts missing something we need. |
| **O2** — TypeScript 7 | Blocked: `svelte-check@4.7.8` still declares peer `typescript: ^5 \|\| ^6`. Revisit when that peer moves. |
| Paraglide 3 | New major that came out after the October audit. Paraglide 2 works and F1 already brings it to 2.29. No gain worth a migration. |
| Svelte 6 / SvelteKit 3 | Svelte 5 is the current major; we are a Vite SPA, not SvelteKit. |

---

## When this file is done

When Tier 1 is checked, record "dependencies updated in v2.9.0" in
`docs/TODO.md` and remove Tier 1 from here. When Tier 2 is done too, delete this
file — do not leave a finished checklist as the source of truth.
