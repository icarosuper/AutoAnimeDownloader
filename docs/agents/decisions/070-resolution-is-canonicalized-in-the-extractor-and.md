# 70. Resolution is canonicalized in the extractor, and codec presets are a frontend reordering

**Location:** `nyaa/nyaa.go` — `ExtractResolution`, `canonicalResolutions`; `nyaa/priorities.go` —
defaults of `Resolutions`/`Codecs`/`Sources`; `frontend/src/lib/domain/priorityPresets.ts`;
`frontend/src/routes/Priorities.svelte`.

**(a) Canonicalization lives in `ExtractResolution`, not in the list order.** `ExtractCodec` already
returned a canonical token (`x265` → `HEVC`) and `ExtractResolution` returned the raw capture. That
asymmetry **was** the bug: `4k` and `2160p` are the same resolution and occupied two different
default indexes, so which one won depended only on how the group wrote the torrent name;
`1920x1080` is capturable by the regex, did not exist in the list, and ranked as the worst. Solving
that by reordering the list (putting `4k` next to `2160p`) solves nothing: they remain two items the
user can split again with the screen arrows, and the duplicate stays representable. Canonicalizing
at the extractor's output it stops existing, and the default list shortens to five readable items.

**Does not break existing config.** The old list already has `1080p`, `720p`, `480p` and `2160p` —
the canonicals. `4k`, `uhd`, `fhd`, `hd` and `8k` become unreachable but harmless, and a 4K release
from an old user starts matching the `2160p` they already have.

**`8k` stays off the list on purpose**, not by forgetfulness: the `\b(8K)\b` pattern still exists,
the token is still returned raw, and by not being in the list it ranks as the worst — the right
treatment for a resolution that does not exist in anime.

**(b) A codec preset is a frontend reordering, not data served by the backend.** Only the frontend
consumes it (the CLI does not mention priorities); an endpoint would exist to serve itself — ~60
lines in six files, a response-shape change and `swag init`, against ~30 lines in one file.

And a preset **promotes** tokens already in the user's list instead of stamping a literal array: a
typo becomes a no-op instead of inert config, a token added by hand drops instead of vanishing, and
the canonical tokens keep existing in one place (`reCodecPatterns`). A preset is a one-shot stamp,
not a stored mode: nothing new in `config.json`, no "custom" state, no precedence rule — after
apply it becomes an ordinary editable list, saved by the button that already exists.

**Only `codecs` gets presets.** `resolutions` has the right default and two arrows in the UI;
`fansubs` is taste, not a technical axis; `sources` and `audio` do not affect episodes (marked
"movies only" on the screen); `criteria_order` is a loaded gun (#55); `ignore_list` is additive, not
ranking.

**Discarded: a "prioritize quality" preset.** For airing anime — which is what
`DownloadStatuses: [CURRENT, REPEATING]` downloads — SubsPlease and Erai-raws remux the
Crunchyroll/HIDIVE stream, with no reencode: that is already the source ceiling, and any larger
file of the same episode is a reencode (generation loss) or an upscale. The first case is already
barred by `re-encode` in the default `ignore_list`. For backlog the axis exists, but it is
**source** (BD vs WEB), not compression.

**The test that closes the bug class:** `TestDefaultPriorities_TokensAreReachable`
(`tests/unit/priorities_test.go`) requires every token of the default lists to be something
`ExtractCodec`/`ExtractResolution` can return. That is how `x265`/`x264` sat for years as inert
invisible config (#69); now it breaks CI.

**Known limitation, deliberate:** no default change reaches an existing install — `LoadConfigs`
unmarshals over `DefaultConfig()`. The lists update through the "reset" buttons that already
exist on the screen; `MaxEpisodeTorrentSizeGB`, `WatchedEpisodesToKeep` and `MinSeeders` ask for a
manual edit, once. There is no automatic migration, for the same reason as #47
(`BatchWindowSeconds`): changing behaviour in an update with nobody asking.

**(c) On the screen, taking a token off the list became a checkbox, not an `✕`.** A token absent
from the list is already treated as the worst (`priorityIndex` returns `len(list)`), so "remove"
was never remove — it was send to the end, with the side effect of the item vanishing from the
screen and no way to bring it back except typing the exact token again (and typing it wrong gives
inert config). Now each row has a checkbox: unchecked it leaves the saved array and stays visible,
greyed, at the end of the list.

The `✕` is left only for what is **not** in the default served by `GET /config/priorities/defaults`:
what the user typed and the inert legacy of an old `config.json` (`x265`, `4k`, `8k`) — which is
exactly what needs a way out. And `criteria_order` lost the add field: it is a closed set —
`sortByCriteria` silently skips a criterion that is not in `criterionCompare` and there is no
validation on `PUT /config` — so free text there only produced inert config, and with the checkbox
the eight criteria are always on the screen, ticked or not. There is nothing to add.

**Don't "fix" by:** returning the raw capture from `ExtractResolution` and "fixing" the list order;
putting `4k`/`uhd`/`fhd`/`hd`/`8k` back in the default; creating a presets endpoint; storing the
chosen preset in `config.json`; writing the new defaults over an existing `config.json`; putting
the `✕` back on canonical tokens "because the checkbox is one more click"; putting the add field
back on `criteria_order` (neither as free text, nor as a select of an enum duplicated in the
frontend); validating `criteria_order` on the backend so we can "afford" to accept free text again.
