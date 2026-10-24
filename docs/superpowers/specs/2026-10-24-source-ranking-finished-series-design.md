# Source ranking for finished series

**Date:** 2026-10-24
**Status:** implemented
**Origin:** complete / standalone anime is now a real download path. Blu-ray vs WEB is the
quality axis that matters there. Weekly airing stays WEB. The Priorities screen already has
a source list (BDRemux, BD, BDRip, WEB-DL, …) labelled "movies only"; `SortMovieResults`
reads it and `SortTorrentResults` does not.
**Scope:** `internal/nyaa` (sort, source extractor, health bands, default `criteria_order`),
`internal/daemon` (gate on `Media.Status == FINISHED`, re-rank after search),
`frontend/routes/Priorities.svelte` (label + note), docs (`config.md`, `decisions.md` #69
revision, new decision).

---

## The problem

Decision [#69](../../agents/decisions.md#69-codec-now-applies-to-episodes-and-a-name-with-no-codec-tag-ties-instead-of-losing)
left `source` out of `episodeCriteria` because a weekly episode is WEB-DL in almost every
release, so the criterion never broke a tie. That was right for `RELEASING`.

A finished series — watch-list catch-up or standalone add — is a different population. Nyaa
carries season packs tagged `BluRay`/`[BD]`/`BDRip`, volume packs (Japanese BD volumes of
2–4 episodes), and individual BD episodes. WEB packs of the same title sit next to them.
Today fansub and resolution decide; a WEB SubsPlease batch beats a BD pack for being
higher on the fansub list.

The user-facing source list already exists. It is inert for every TV row.

## What this is not

- Not a hard filter. A WEB release of a finished series still downloads when no BD
  candidate survives `min_seeders` and the size ceilings.
- Not `MediaListStatus` (`CURRENT`, `COMPLETED`, `REPEATING`). The gate is AniList
  **`Media.Status`**.
- Not audio on episodes. `audio` stays movies-only.
- Not a per-anime override. Ranking stays global `nyaa.ActivePriorities()`.
- Not an auto-redownload of weekly WEB when the BD lands months later. Already-saved
  episodes stay; source only ranks **new** picks.
- Not a change to `min_seeders`. Dead torrents (default: fewer than 3 seeders) are
  dropped before any sort. Health ranks the survivors.

## Gate

Source enters the series sort if and only if `anime.Media.Status == anilist.MediaStatusFinished`.

| `Media.Status` | Source in series sort |
|---|---|
| `FINISHED` | yes |
| `RELEASING`, `NOT_YET_RELEASED`, `HIATUS`, `CANCELLED` | no |

Movies keep `SortMovieResults` (full `CriteriaOrder`, source included) regardless of
status. A `FINISHED` movie is unchanged in kind; it rides the new default order and the
new health bands like everything else.

The gate is applied at the daemon, which already has `MediaList`. `nyaa` does not import
`anilist`.

## Where it runs

The same ranked list feeds packs, singles, and a standalone add: `ScrapNyaaForAnime`
returns one mixed list, `partitionSearchResults` splits it, `searchSingleEpisode` is the
per-episode fallback. Re-ranking the mixed list **before** the partition covers pack and
episode in one step. The singles fallback is re-ranked the same way.

Call sites that have `Media.Status` today:

- `processAnimeEpisodes` (verification pass) — after `searchAnime`, before
  `partitionSearchResults`; after `searchSingleEpisode`.
- `RunAnimeDebug` — mirrors that pipeline.
- Manual per-episode download/redownload — `resolveAnimeDetails` already stores
  `isFinished` (`manual_download.go`) and does not use it; this work consumes that flag.

A helper on the daemon side, not a new parameter on `ScrapNyaa` / `ScrapNyaaForAnime`:

```go
func rankSeriesResults(results []nyaa.TorrentResult, finished bool) []nyaa.TorrentResult {
    if finished {
        return nyaa.SortFinishedSeriesResults(results)
    }
    return results // already sorted by SortTorrentResults inside the scrape
}
```

`ScrapNyaa*` keep calling `SortTorrentResults` (no source). Tests of the scrape stay
valid. Finished ranking is a second, full sort with source allowed — `sort.SliceStable`
over the finished criteria, not a patch on the first order.

When the torrent-picker work on `v2.7.0` lands (`preview.go` /
`GET /animes/{id}/torrent-candidates`), it calls the same helper with the media's
status. This spec does not implement the picker.

## Ranking

### Default `criteria_order`

```
uncensored, resolution, health, source, codec, fansub, audio, size
```

Uncensored is a content cut, not a quality axis, and uncensored swarms are smaller; it
stays first. Resolution before source: a 1080p WEB beats a 720p Blu-ray. Health before
source: a WEB in a better seeder band beats a BD in a worse one. Source before fansub:
among 1080p torrents in the same band, BD wins.

`SortMovieResults` uses this order as-is (it already applies every criterion).

### Health bands

`healthTierFloors` becomes `{10, 50}`.

With default `min_seeders = 3`, the bands the sort actually sees:

| Seeders | Tier | Meaning |
|---|---|---|
| 3–9 | 0 | Alive, fragile. Packs stall. |
| 10–49 | 1 | Completes. Packs may be slow. |
| 50+ | 2 | The user's connection is the limit. 50 vs 5000 is the same download. |

The floor `3` is not a health band: `min_seeders` already dropped 0–2. A first floor of
`10` is the first split among survivors.

This constant is **not** in `config.json`. The new bands apply on the next process
start, including to `RELEASING` and to movies. That is intended: 150 vs 400 tying so
fansub/source can decide is the right behaviour everywhere.

Worked examples, all `FINISHED`, 1080p unless noted, after `min_seeders`:

| A | B | Winner | Why |
|---|---|---|---|
| BD, 8 seeders | WEB, 80 seeders | WEB | health (0 vs 2) |
| BD, 60 seeders | WEB, 400 seeders | BD | health ties (both 50+), source |
| BD 720p, 200 seeders | WEB 1080p, 200 seeders | WEB 1080p | resolution |
| uncensored WEB, 8 seeders | censored WEB, 80 seeders | uncensored | uncensored first |

### Empty source tag = `web-dl`

Weekly and many finished WEB packs have no source token
(`[SubsPlease] Anime - 07 (1080p)`). `priorityIndex` treats an unknown token as worst.
The codec-style "either side empty ⇒ tie" would also make a tagged BD tie with untagged
WEB, which throws away the whole feature.

`sourceCompare` canonicalizes `extractSource == ""` to `web-dl` (already in the default
list). Then:

- untagged vs `WEB-DL` → tie on source
- untagged vs `BD` / `BDRip` / `BDRemux` → BD wins
- untagged vs untagged → tie

This comparator is the one `criterionCompare["source"]` already uses, so movies get it
too. An untagged movie no longer loses to a tagged WEB-DL for having no tag.

### Extractor: `[BD]`, and drop bare `TV`

`reSourcePatterns` today matches `BDRemux`, `BDRip`/`BD Ray`, `Blu-ray`/`BluRay`,
`WEB-DL`, `WEBRip`, `HDTV`, `\bTV\s*Rip|\bTV\b`, `DVD`. It does **not** match `[BD]`,
the usual tag on anime packs.

Add, **after** the more specific BD patterns so `BDRip`/`BDRemux` still win:

```
(?i)\[BD\]|\bBD\b  →  "BD"
```

`\bBD\b` does not match `BDRip` (D and R are both word characters).

Drop the bare `\bTV\b` alternative. `(TV)` in a series name is the AniList format
marker, not a broadcast source. Keep `TVRip` and `HDTV`, and match `\[TV\]` so the
default list token `tv` stays reachable. `(TV)` in a title no longer counts as source.

## UI

`Priorities.svelte`: `sources` is no longer `moviesOnly`. New copy (en + pt-BR):

- heading hint: "movies and finished series" / "filmes e séries encerradas"
- `priorities_note_criteria_order`: audio stays movies-only; source applies to movies
  and to series whose AniList media status is FINISHED.

`audio` keeps `moviesOnly`.

No new config field, no new endpoint, no CLI command. `config set` already writes
`priorities.*`. Restore-defaults on that screen is how an existing install picks up the
new `criteria_order`.

## Rollout

`LoadConfigs` unmarshals over `getDefaultConfig()`. An existing `config.json` **keeps**
its `criteria_order`. With the old order (`uncensored, source, resolution, health, …`)
source still sits before health, so a fragile BD still beats a healthy WEB until the
user hits restore-defaults (or edits the list). Same rule as
[#55](../../agents/decisions.md#55-health-compares-a-seeders-band-and-it-comes-before-fansub):
do not migrate a saved `criteria_order`.

The health bands and `SortFinishedSeriesResults` do not wait on that click: bands are a
code constant; including source for `FINISHED` is code. Only the relative order of
source vs health vs resolution is what restore-defaults fixes.

Document this in `config.md` next to `priorities.criteria_order` and
`priorities.sources`.

## Tests

- `SortFinishedSeriesResults`: the four worked examples in the table above.
- `SortTorrentResults` (no source): a tagged BD does **not** beat a tagged WEB on source;
  behaviour stays the RELEASING one.
- `sourceCompare`: untagged vs `WEB-DL` ties; untagged vs `[BD]` prefers BD.
- `extractSource("[Group] Anime S01 [BD] [1080p]") == "BD"`.
- `extractSource` on `(TV)` without `TVRip` does not return `TV`.
- `healthTier`: 9 → 0, 10 → 1, 49 → 1, 50 → 2.
- `DefaultPriorities().CriteriaOrder` is the new slice.
- `TestDefaultPriorities_TokensAreReachable` also walks `Sources` through
  `extractSource` (same inert-token class as #70).
- Daemon: a `FINISHED` anime re-ranks a 60-seeder BD pack above a 400-seeder WEB pack;
  the same two torrents on a `RELEASING` anime keep the scrape order (source ignored).

## Docs at implementation

- `docs/agents/config.md` — `criteria_order` default, `sources` no longer "movie sort
  only", health-band note (bands are not a config field; mention next to `min_seeders`).
- `docs/agents/architecture.md` — sorting paragraph: source is in the finished-series
  subset; health floors `10/50`.
- `docs/agents/decisions.md` — new entry (next number after the current highest): gate
  is `Media.Status`, empty source is `web-dl`, bands `10/50`, no `criteria_order`
  migration. Revise #69 "Only `source` stays out" / "Don't put `source` in
  `episodeCriteria` too" to "source stays out unless `FINISHED`".
- `docs/guides/webui.md` — Priorities blurb.

## Out of scope

- Applying source to `RELEASING` / `HIATUS` / `CANCELLED`.
- Putting `audio` on episodes.
- Auto-replacing a saved WEB episode when a BD appears.
- Per-anime source override.
- Migrating an existing `criteria_order`.
- Changing `min_seeders`.
- The torrent picker UI (`v2.7.0`); only the rank helper it will call.
