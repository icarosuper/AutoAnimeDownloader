# TODO

Grouped by release: each section is **one** release, and its bump is the largest bump of the tasks
inside it — a task that doesn't touch the binary (docs, things outside the repo) rides along without
changing the number.
Current version: `v2.6.0`. Stamp a version (tag, GitHub Release, AUR):
[guides/release.md](guides/release.md).

The order follows a single rule: **nothing that stamps the project's name ships before the rename**
(Windows installer, ghcr image, README, social preview, landing page), otherwise it's a double
migration. If presenting the repo to employers gets urgent, `v3.0.0` can move up to right after
`v2.6.0`.

> The old Stage 1 (close the ceilings of what already runs) closed on 2026-08-30 **with no code**:
> all three items were measured and all three said not to touch anything. Pack numbering by folder
> and per-episode pack ceiling in [decisions.md #84](agents/decisions.md#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything)
> and [sources.md](agents/sources.md); frontend cost on AniList in
> [decisions.md #85](agents/decisions.md#85-the-frontends-anilist-cost-is-per-account-and-per-orphan-not-per-tab),
> which closes the re-evaluation of [#73](agents/decisions.md#73-the-frontend-doesnt-query-anilist-directly-even-though-it-could).
> The instrument for the first two lives in `nyaa/live_pack_measure_test.go` (`AAD_LIVE_NYAA=1`).

> The **local anime cache** closed on 2026-09-07: AniList is served from an on-disk snapshot when it
> goes down, with the snapshot coming back with TTL zero so no screen gets staler than it was
> ([#92](agents/decisions.md#92-o-cache-em-disco-da-anilist-volta-com-ttl-zero-e-o-valor-servido-dele-vem-acompanhado-de-errfromcache)).
> Covers came for free (the URL is a `Media` field, already in the snapshot) and a pass in cache
> mode deletes, by explicit decision.

## `v2.4.0` — MyAnimeList integration

- ~~Add MyAnimeList integration~~ — **released in `v2.4.0` on 2026-10-13** ([decisions.md #95](agents/decisions.md#95-myanimelist-is-a-list-source-not-a-metadata-source)).
  MAL is a **list source** only (status + progress); identity, episodes and metadata stay on
  AniList, mapped through `idMal`. The list comes from the unofficial `load.json`, so no client ID
  is needed; the list must be public.

## `v2.5.0` — bug report

- ~~Bug report mechanism~~ — **released in `v2.5.0` on 2026-10-18** ([decisions.md #102](agents/decisions.md#102-the-bug-report-has-no-backend-consent-is-the-act-of-attaching-the-file),
  [#103](agents/decisions.md#103-log-redaction-is-by-key-and-a-test-forces-every-new-key-to-be-classified)).
  A redacted diagnostics bundle plus crash capture, downloaded from the web UI and attached to a
  GitHub issue. No backend: consent is the act of attaching the file.

## `v2.6.0` — CLI parity

- ~~Bring the CLI to parity with the API / webApp~~ — **released in `v2.6.0` on 2026-10-18**
  ([decisions.md #104](agents/decisions.md#104---yes-is-a-cli-gate-the-api-keeps-its-query-defaults)).
  Nested commands, `--json` as the agent contract, `--yes` on destructive actions, no prompts.
  A new API endpoint ships with its CLI command in the same change.

## `v3.0.0` — Serval

Working checklist (one box per change, for coordinating chats): [v3.0.0.md](v3.0.0.md).

Everything that stamps the project's name and look, in a single release. The major comes from the
rename, and the Nyaa descriptor comes along so whoever upgrades goes through **one** migration, not
two.

- Rename the project to **Serval** — Go module name, binaries (`daemon`/`cli`), GitHub repo,
  `config.json` and the user data directory, ghcr image, webApp titles. Breaks the data
  path for existing users → needs a migration or an explicit release note
- **Take Nyaa out of the repository (source as an external descriptor)** — spec designed on
  2026-09-13 and never executed:
  [source-as-descriptor](superpowers/specs/2026-09-13-source-as-descriptor-design.md).
  The goal is **publishability**, not multiple sources: Nyaa's address, query and layout leave the
  binary for a JSON file the user puts next to `config.json`. A compiled Go plugin is out (the
  `plugin` package is Linux-only and requires the same compiler and the same deps). The processing
  does **not** go in the descriptor: of the 2217 lines in `internal/nyaa`, ~150 are Nyaa-specific
  and ~2050 are release-name parsing, the same for any source. The bulk of the diff is the split
  into `internal/release` + `internal/source` (21 files import the package, 277 occurrences of
  `nyaa.`), and steps 1-2 of the spec stand on their own — they can ship early in a minor, since
  they don't change behavior. Whoever upgrades without the descriptor is left with no source: the
  rename migration covers that
- UI/UX overhaul of the webApp
- Visual identity — logo, palette, favicon; feeds the social preview and the landing page
- Publish the image on ghcr — the biggest win among the options measured in
  [distribution.md](distribution.md), the Dockerfile already exists. A `curl` `install.sh` is the
  next step from there, if needed. `go install`, Homebrew, Scoop, .deb/.rpm and Flatpak were
  evaluated and discarded
- Update the README — after the image, because that's what changes the install instructions
- Upload a social preview on GitHub — today the shared link card is GitHub's generic one.
  Settings > General > Social preview > Upload, 1280x640, using the new visual identity. Web only,
  `gh repo edit` doesn't cover it
- Create a landing page

## `v3.1.0` — proper Windows release

After the rename: the installer, the service name and the data path stamp the name, and Windows UI
tweaks would be redone by the `v3.0.0` UI/UX overhaul.

- Authenticate the app with a Microsoft account
- Install instead of running from an executable file
- Detach and run as a background service
- UI changes to work better on Windows
- Test thoroughly

## No release — promote

After `v3.1.0`: promoting without Windows cuts off a large part of the audience.

## No release — subtitle language filter

Ships as a patch whenever it is tackled.

- Filter torrent ranking (and optionally hard-filter) by subtitle language in priorities
  (e.g. PT-BR). Spec designed on 2026-08-24 and never implemented:
  [subtitle-language-filter](superpowers/specs/2026-08-24-subtitle-language-filter-design.md).
  The language is read from the release **name** (`[PT-BR]`, `MultiSub`, …), not from the
  torrent file list and not from scraping the Nyaa description.

## Fixes with no fixed release — download old and long anime properly

Ships as a patch (`v2.x.y`/`v3.x.y`) whenever it's tackled. The symptom is a single one: **old or
long anime doesn't download**. Current-season anime downloads fine. The cause isn't finding a
candidate — it's that the candidate is either dead (0-2 seeders, blocked by `min_seeders` or stuck
at 0%) or a huge pack the size ceiling rejects.

> The three **download hygiene** items closed on 2026-09-07: the
> disk guard now receives the torrent size
> ([#93](agents/decisions.md#93-a-guarda-de-disco-tem-dois-gatilhos-e-o-de-tamanho-vale-mesmo-com-min_free_disk_percent--0)),
> a torrent failure now keeps the bytes already downloaded (amendment to
> [#24](agents/decisions.md#24-a-failed-torrent-is-dropped-from-the-session-and-re-added-by-the-next-pass--no-blacklist))
> and a torrent at zero peers for more than an hour is now dropped through the same failure path
> ([#94](agents/decisions.md#94-a-stalled-torrent-is-dropped-through-the-failure-path-and-only-one-that-is-trying-counts-as-stalled)).
> None of the three was measured live — the tests are unit tests, and the live franchise pass
> below is what actually exercises them.

- **Download part of a pack** — for the One Piece 1-12 window, the only live pack that covers it is
  171.8 GiB and the 100 GB ceiling blocks it; the loose episodes exist and are dead. No indexer
  swap solves this, because 574 episodes weigh what they weigh. The three exits, each in its own
  spec: per-anime size ceiling in `AnimeSettings` (smallest diff, costs disk), **per-file
  selection** (`rain` v2.3.1 doesn't have it — `AddTorrentOptions` only exposes
  `Stopped`/`StopAfterDownload`/`StopAfterMetadata`, and `Files()`/`FileStats()` are read-only;
  `anacrolix/torrent` has per-file priority) and Debrid
- **Live pass on a multi-part franchise (Attack on Titan)** — the per-series absolute axis, the
  `packAxis` and ownership by coverage ([#77](agents/decisions.md#77-the-per-series-absolute-axis-is-a-bfs-two-generations-at-a-time-and-the-cut-level-goes-back-to-the-queue)/[#78](agents/decisions.md#78-a-unidade-de-posse-de-um-torrent-é-a-cobertura-no-eixo-absoluto-não-a-chave-anime_id-episódio)/[#79](agents/decisions.md#79-a-escolha-de-pack-pergunta-cobre-a-janela-não-é-da-part-n--e-a-numeração-do-pack-é-palpite-entre-três-hipóteses)/[#84](agents/decisions.md#84-a-pack-without-a-range-in-its-name-gets-its-coverage-from-the-file-list-not-from-assuming-it-covers-everything))
  were written in August and only have unit tests plus a **listing** measurement. No real
  end-to-end download has run on the S1 → Final Season Part 3 chain, which is exactly where the
  four numbering conventions from the "Granularidade" section of [sources.md](agents/sources.md)
  collide. The instrument already exists: `scripts/robustness-animes.txt` + `make debug-batch`
