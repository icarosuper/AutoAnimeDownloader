# 60. The pack window covers from the first pending episode, and the cut is always a prefix

**Location:** `daemon/episodes.go` — `windowEnd`, `pickBatches`, `assignBatches`.

**What it looks like:** `pickBatches(packs, firstPending, windowEnd)` walks a cursor from the first pending episode, and `assignBatches` stops assigning a magnet at the first episode in the list that no chosen pack covers — instead of giving everyone the magnet of the first pack found, or trying a loose episode for the rest.

**Why it's right:** without the window anchored on the first pending, `batchResult[0]` would go to every pending episode and episode 800 of a long series would receive the magnet of pack 1-100 — wrong data, not "no magnet". Without the prefix truncation in `assignBatches`, episodes outside coverage would fall into the loose-episode fallback (`magnetsByEpisode`), which on One Piece would search and report ~1000 episodes as not found **per pass** — none of them will show up loose on Nyaa, so it is pure noise, with no chance of success. The cut is always a **prefix** (never "a hole in the middle") because the schedule (`anilist.EpisodeList`) comes in increasing order and the packs `pickBatches` chooses are contiguous from the cursor — there is no way a later pack covers an earlier episode without `packSet.covering` already having picked it first.

**Don't "fix" by:**
- Applying the magnet of the first pack found to every pending episode "because it's an episode either way" — an episode outside the pack's range simply is not in the `.torrent`.
- Falling back to a loose episode for what remains outside the window: on a long series that is a guaranteed error notification, not one more attempt.
