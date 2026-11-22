# 114. Two picker confirms; a paste does not override Download this

**Location:** `TorrentPicker.svelte` (`confirmSelected` / `confirmMagnet`);
`GET /api/v1/animes/{id}/torrent-candidates` `media` object.

**The rule:** the picker has two confirms. **Download this** always POSTs the selected
row (add: magnet + `name` / `nyaa_id` / `size_bytes`; replace: that magnet).
**Download magnet** POSTs the trimmed field (add: `{ magnet }` only; replace: that
URI). A non-empty paste does not change **Download this**. Both may be enabled at
once; the click chooses. #108.5 stays: the API still accepts a magnet without
coverage fields. `media` on the candidates payload is the `GetMediaByID` already
done for the search — no second AniList round-trip, no `seasonYear`.

**Why:** one solid button that took `pastedMagnet.trim() || selected.magnet` made a
paste silently override the highlighted row. The screen is a confirm of a ranked
pack and a magnet escape hatch, not one control with two meanings.

**Don't "fix" by:** merging the two buttons again; reading `GET /animes/{id}` for
the add-mode header (the anime is not tracked yet); widening `GetMediaByID` for a
year.
