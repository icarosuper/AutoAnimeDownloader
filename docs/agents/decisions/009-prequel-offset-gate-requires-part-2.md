# 9. PREQUEL offset gate requires part ≥ 2

**Location:** `internal/daemon/helpers.go` — `ComputeEpisodeOffset`

**What it looks like:** `ComputeEpisodeOffset` always returns 0 unless `part >= 2`, even when a PREQUEL relation exists. Looks like a missing case.

**Why it's right:** The offset (PREQUEL episode count) is only needed for split-season Part 2 releases where the torrent site numbers episodes globally (e.g. episode 14 on Nyaa when Anilist calls it episode 1 of Part 2). Kaguya-sama Season 2, for example, also has a PREQUEL relation with 12 episodes — applying a +12 offset there would search for episode 13 when the correct episode is 1. The gate `part >= 2` restricts the offset to the specific case it was designed for, preventing all single-season animes with prior seasons from receiving a wrong offset.

The same reasoning applies to the PREQUEL **format**: only `TV`/`TV_SHORT` enter the count. A recap
movie or an OVA also arrives as `PREQUEL` with `Episodes` filled, but they are not part of the
numbering fansubs follow — adding them would send the search to an episode that does not exist.
When there is no TV PREQUEL, the offset is 0 and the search falls back to relative numbering, which
is what a large share of groups uses. Erring toward 0 is cheap; erring toward a number is a search
that finds nothing.

**Don't "fix" by:** removing the gate or applying the offset whenever a PREQUEL exists. That breaks every multi-season anime that does not use split-part numbering. Nor by accepting a PREQUEL of any format.

**Amendment (see [#133](133-an-ona-entry-counts-its-ona-prequel-in-the-season.md)):** when the **entry itself is an ONA**, an ONA prequel counts too —
a streaming series split into ONA parts (Stone Ocean Part 2) is one show. The part gate and the
rule for a TV entry stay as above; movies and OVAs still never count. `ComputeEpisodeOffset` now
takes the entry's format.
