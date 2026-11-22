# 45. One library folder per AniList ENTRY — the season marker stays in the name

**Location:** `internal/files/librarian.go` — `sanitizeName` (used to be `sanitizeFolderName` + `sanitizeFileName`, now one) and `destDir` in `Organize`; test `TestSanitizeNameKeepsSeasonMarker`.

**What it looks like:** the "right" layout for Jellyfin/TVDB is one folder per series with `Season 03/` inside, and until `cd496c3` the code went halfway in that direction: a `seasonPattern` stripped "Season 2"/"S2"/"2nd Season"/"Cour 2" from the folder name, joining every season into one folder.

**Why it's right:** on AniList a series with several seasons **does not exist**. Each season is its own media, with id, cover, synopsis and episode numbering starting at 1 — and our data is exactly like that (one `AnimeID` per season, `EpisodeNumber` restarting). Joining the seasons into one folder forced Jellyfin to pick ONE entry for the whole folder, and it picked season 1: every new season showed up with the first's cover and metadata (real report: Mushoku Tensei — entries 108465, 127720, 146065 and 178789 all landed in `Mushoku Tensei Jobless Reincarnation`). The grouping was not even coherent: the regex ate "Season 2" but not "Part", so "Season 2 Part 2" ended up in a fifth folder.

With `tvshow.nfo` (#44) that would stop being a guess and become a fixed error: the first season to arrive in the folder would write its `uniqueid` and, because the nfo is never overwritten, lock the folder to that season forever.

A franchise layout with `Season NN/` cannot work here either: the franchise `tvshow.nfo` would need an id AniList does not have, and `jellyfin-plugin-anilist` does not provide per-season metadata. One folder per entry is at once the smaller diff and the only model that matches the provider.

Because the two sanitization functions became identical after removing the regex, they became one: `sanitizeName`.

**Don't "fix" by:** reintroducing the season strip "to group the franchise"; creating `Season NN/` subfolders (AniList episode numbering is per entry, so E01 of season 3 is that media's episode 1, not episode 25 of the franchise); deduplicating folders by base title.
