# 88. The progress bar derives "downloaded" from `episodes_pending`, not from `episodes_downloaded`

**Location:** `frontend/src/lib/utils/status.ts` — `breakdown()`, consumed by the three
`TripleProgressBar`s of `routes/Status.svelte`. The backend that produces the number is in
`api/endpoint_animes.go` — `countPendingEpisodes`.

**What it looks like:** a detour. `AnimeInfo` has `episodes_downloaded` right there, the bar wants
the "downloaded" segment, and `released - downloaded` is the obvious subtraction. That is what the
code did.

**Why it exists:** `episodes_downloaded` is not "how many I have already downloaded", it is **how
many files are on disk right now** — it comes from counting `LoadSavedEpisodes()`. The daemon
prunes a watched episode (`daemon/episodes.go`, `buildWatchedKeepSet` / `watched_episodes_to_keep`,
default 2), so that counter **drops** as the user watches. The subtraction then throws an episode
already downloaded AND seen into the yellow segment, which is supposed to mean "still to
download".

Measured symptom: an anime with `5 watched · 5 downloaded · 10 released of 13` painted 5/13 yellow
with the **Up to date** chip next to it, on the same row. Eps 6-10 were on disk and 1-5 had been
pruned; nothing was missing.

`countPendingEpisodes` walks **by episode number** (`for n := watched+1; n <= released`), not
`released - downloaded`, exactly to survive the prune — its comment says so. `episodes_pending` is
the only immune counter, and the bar's `downloaded` is `released - pending` (floored at
`watched`).

The "behind" chip had already taken this same fix earlier, in `lib/domain/animeState.ts` — the
comment there records that the naive subtraction "marked half the list as behind". The bar lagged
and repeated the bug; they are the two consumers of the same number.

**The legend left with it.** It was a `legend: string` prop built in `Status.svelte` with the
CUMULATIVES ("5 watched · 5 downloaded · 10 released of 13") — impossible to match with the
colours, because the purple there had zero width while the text said "5 downloaded". Today it
lives inside `TripleProgressBar`, in four terms that are the DELTAS, each coloured with its
segment's colour: `X watched` (--ok) · `Y to watch` (--accent) · `Z to download` (--warn) ·
`W unreleased` (track). The same four numbers feed width and text, so they cannot diverge again.
A ready-made string has nowhere to hang the `<span>` — that is why the prop died instead of
becoming `legend: string[]`.

**Don't "fix" by:** passing `anime.episodes_downloaded` straight to `TripleProgressBar`'s
`downloaded` prop "because the name matches" — the names match, the semantics do not. Nor putting
the legend back on a string prop "so the component doesn't import i18n". Covered by
`tests/unit/status.utils.test.ts` (`describe('breakdown')`) and
`tests/component/TripleProgressBar.test.ts` (word ↔ colour ↔ width).

**A zeroed term vanishes per anime, stays on Library.** That is what the `keepZeros` prop decides,
and the default (`false`) is the common case. Per anime, zero is noise — most rows have at least
one, and `LIST_GRID`'s 290px was measured for a THREE-term legend, so four in full wrap onto two
lines and the row height oscillates. On the *Library* card it is the opposite: there the four
numbers are the card's content, and a term vanishing would make the sum stop closing with the
total.

When all four terms are zero (anime with no released episode) `0 watched` remains, on purpose: an
empty `<p>` would shrink the row and bring back exactly the height oscillation the omission was
supposed to avoid.

**Watch the separator:** it is `{' · '}`, an expression and not literal text. Svelte trims
whitespace at the start/end of an element's children, and `<span> · </span>` renders `·` glued to
the words. Covered — the tests compare the whole `textContent` of the `<p>`, with spaces.
