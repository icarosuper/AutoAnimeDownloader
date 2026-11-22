# 108. Empty first search skips the picker; custom q drops variants; description is on demand; add-with-magnet writes the id after the torrent

**Location:** Add's first `getTorrentCandidates` (`AddAnime.svelte`); `PreviewWholeTitleTorrents`
(`daemon/preview.go`) for a custom `q`; `GET /api/v1/animes/{id}/torrent-candidates/{nyaaId}`;
`DownloadStandaloneAnimeWithMagnet` (`daemon/standalone.go`) and `POST /standalone-animes`.

**The rule:**

1. **Empty first search skips the picker.** Add's first candidates GET (no `q`) with `items: []`
   POSTs `/standalone-animes` without a magnet — singles, or track-with-zero for an unreleased
   title — and stays on the card. A later empty custom query does **not**: Enter on an empty
   field is a no-op, and a custom `q` that returns no packs keeps the picker open. The
   automatic-singles fall-through exists only for that first, daemon-query search.
2. **Custom `q` drops variants.** A non-empty `q` is the Nyaa query as typed: no AniList
   title-variant loop, no season/part row filter. What still runs: `ShouldIgnore`, `min_seeders`,
   size ceiling, packs-only (or movie, when AniList format is `MOVIE`), Priorities ranking, cap 8,
   `titleMatchesQuery` against the query tokens.
3. **Description is on demand.** `/view/<id>` is fetched only for the selected row
   (`GET …/torrent-candidates/{nyaaId}`). The candidates list does not scrape it; range on that
   list comes from the name (`ExtractBatchInfo`).
4. **Add-with-magnet writes the id after the torrent.** The magnet enters the session first,
   then the standalone id, then coverage records. If `backend.Add` (or the disk guard) refuses,
   the id is never written. If the id is written and a later step fails, `RemoveStandaloneAnime`
   runs before the error returns. Coverage is only the aired episodes the chosen torrent covers
   (name, then one `PackFileRange` if the name has none); unknown range writes **no** episode
   records, not `1..total` ([#84](084-a-pack-without-a-range-in-its-name-gets-its.md)).
5. **`name` / `nyaa_id` / `size_bytes` are optional on POST** because a magnet URI cannot rebuild
   coverage or the Nyaa size. The picker sends them from the selected row; a pasted magnet may
   omit them (coverage then unknown, disk guard without a size).

**Why:** the ranking is still the right default; the picker is a confirm step, not a Nyaa
browser. Fetching eight `/view/<id>` pages up front is the cost [sources.md](../sources.md) called
out. Writing the id before the torrent leaves a tracked anime with nothing in the session.

**Don't "fix" by:** falling through to automatic singles on a custom empty query; fetching
`/view/<id>` inside the candidates handler "so the pane is instant"; writing the standalone id
before `backend.Add`; persisting the picker cache or the custom `q`; inventing `1..total`
coverage from a magnet that has no `name`/`nyaa_id`.
