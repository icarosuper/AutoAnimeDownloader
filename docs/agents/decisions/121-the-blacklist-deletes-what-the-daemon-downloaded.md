# 121. The blacklist deletes what the daemon downloaded and spares what the user downloaded by hand

**Location:** `internal/daemon/episodes.go` — `checkEpisode` (the `shouldSkipEpisode` branch marks
every saved episode of a blacklisted anime for deletion) and `selectEpisodes`, whose
`!savedEp.ManuallyManaged` guard takes the hand-downloaded ones back out;
`internal/daemon/standalone.go` — `appendStandaloneAnimes`, which drops the standalone record once
the anime shows up in the processed list, blacklisted or not. Pinned by
`TestProcessAnimeEpisodes_BlacklistedAnime_PopulatesKeysToDelete`,
`TestHandleSavedEpisodes_BlacklistedAnime_DeletesTorrents` and
`TestProcessAnimeEpisodes_BlacklistedAnime_KeepsManuallyManaged`.

**What it looks like:** two rules that seem to contradict each other. Putting an anime on an
`excluded_lists` list deletes its episodes, unwatched ones included, because the blacklist check
runs before the progress check. Yet an anime the user added as standalone and then blacklisted
keeps every file. And the standalone record is thrown away the moment the anime appears in the
list, even though the blacklist then stops it from being followed.

**Why it's right:** these are two different intents, and each one is kept where it lives.
- **The blacklist means "stop downloading this".** For an anime the daemon downloaded on its own,
  stopping also means freeing the disk, so with `delete_watched_episodes` on its records leave.
- **A hand download means "I want these files".** That intent lives on the episode record
  (`manually_managed: true`, set by the standalone add, the manual episode download and the magnet
  download), not on the standalone record. No automatic delete path removes such a record: not the
  blacklist, not watched pruning, not `delete_statuses`, not leaving the list. Only the user, from
  the UI or the CLI, deletes it.

The real case behind this: the user downloaded Bocchi the Rock! and ONIMAI as standalone batches,
then put them on REPEATING in AniList and on the `AutoDownloader` list. The goal was "keep what I
downloaded, don't download anything else". That is what happens: no new download, and the 24
records stay.

So the standalone record can go. It only says "keep following this anime and download what comes
next", and the blacklist overrules exactly that. The rule is "the later intent wins": the add
guard ignores the blacklist, because there the click is the later intent
([#49](049-standalone-anime-tracked-by-the-list-is-the.md) (c));
here the blacklist came after the add. Keeping the record would only matter once the anime leaves the list. Then it would quietly bring
the anime back under the daemon, which the user had turned off. That is the "dormant standalone"
rejected in `appendStandaloneAnimes`' comment.

The protection is per episode, not per anime. A blacklisted anime with some hand-downloaded
episodes and some downloaded by the daemon loses only the daemon's.

**Don't "fix" by:** making the blacklist stop downloading without deleting, for every anime; letting
the blacklist delete `manually_managed` records; keeping the standalone record of a blacklisted
anime "to remember the intent", since the files already carry it; letting a standalone record survive
once the anime shows up blacklisted in a processed list.
