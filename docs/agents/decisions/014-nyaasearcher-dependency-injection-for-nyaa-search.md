# 14. `nyaaSearcher` — dependency injection for Nyaa search in `processAnimeEpisodes`

**Location:** `internal/daemon/search.go` (`nyaaSearcher` struct + `defaultNyaaSearcher()`); `internal/daemon/episodes.go` (`processAnimeEpisodes`).

**What it looks like:** `processAnimeEpisodes` receives a `nyaaSearcher` struct with function-valued fields (`searchAnime`, `searchSingleEpisode`, `searchMovie`) instead of calling `searchNyaaForAnime`, `searchNyaaForMovie`, etc. directly. Looks like unnecessary indirection — these are pure functions with no state.

**Why it's right:** The `episodeInTorrents` hash-check fix prevents batch torrents from being re-downloaded in a loop. Without injection, it's impossible to write a test that proves the loop can't recur: a test that calls the real Nyaa makes a live HTTP request, is flaky, and can't observe whether `searchAnime` was invoked. Injection lets the regression test (`TestProcessAnimeEpisodes_BatchNoRedownload`) confirm both that `searchAnime` is never called and that `POST /add` is never sent when all episode hashes already match.

**Don't "fix" by:** removing the `nyaaSearcher` parameter and going back to direct package calls. That makes the regression test impossible to write, and the loop bug would be undetectable until it reappears in production.
