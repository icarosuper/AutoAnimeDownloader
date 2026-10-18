package daemon

import (
	"math"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
)

// savedNonBatch builds the saved-episodes map (1..n) with no pack marker.
func savedNonBatch(animeID, n int) (map[files.EpisodeKey]bool, map[files.EpisodeKey]files.EpisodeStruct) {
	set := make(map[files.EpisodeKey]bool, n)
	full := make(map[files.EpisodeKey]files.EpisodeStruct, n)
	for i := 1; i <= n; i++ {
		key := files.EpisodeKey{AnimeID: animeID, Episode: i}
		set[key] = true
		full[key] = files.EpisodeStruct{AnimeID: animeID, EpisodeNumber: i, EpisodeHash: "h"}
	}
	return set, full
}

// The bug the inverted order avoids: with the limit lifted by guesswork, this set came back
// EMPTY — handleAlreadySavedEpisode never fired with maxEpisodes = len(episodes)+1, and the
// buffer cut episodesToDownload without restoring keysToDelete.
func TestSelectEpisodes_LimitProducesDeletions(t *testing.T) {
	anime := animeWithEpisodes(50, anilist.MediaStatusFinished, true, "")
	savedMap, savedFull := savedNonBatch(anime.Media.Id, 50)
	episodes := anilist.EpisodeList(anime, 1)

	sel := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{"h": true}, nil, nil)

	if len(sel.keysToDelete) != 38 {
		t.Errorf("want 38 episodes above the limit in keysToDelete, got %d", len(sel.keysToDelete))
	}
	if len(sel.checked) != 50 {
		t.Errorf("every episode in the list must be checked, got %d", len(sel.checked))
	}
}

// Purity: the downloadedEpisodes counter is local, so two identical calls return the same
// result. That is what makes the second selection (limit lifted) free.
func TestSelectEpisodes_IsPure(t *testing.T) {
	anime := animeWithEpisodes(50, anilist.MediaStatusFinished, true, "")
	savedMap, savedFull := savedNonBatch(anime.Media.Id, 50)
	episodes := anilist.EpisodeList(anime, 1)

	first := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{"h": true}, nil, nil)
	second := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{"h": true}, nil, nil)

	if len(first.toDownload) != len(second.toDownload) || len(first.keysToDelete) != len(second.keysToDelete) {
		t.Errorf("identical calls returned different results: %d/%d vs %d/%d",
			len(first.toDownload), len(first.keysToDelete), len(second.toDownload), len(second.keysToDelete))
	}
}

// A pack record overflowing the limit is NOT deleted: the pack is the unit, deleting one
// record frees no bytes and the torrent stays on disk. And it also downloads nothing more
// from this anime.
func TestHandleAlreadySavedEpisode_BatchIsImmuneToTheLimit(t *testing.T) {
	counter := 12

	if dl, del := handleAlreadySavedEpisode(12, &counter, true, true, "ep"); dl || del {
		t.Errorf("a pack overflowing the limit must return (false, false), got (%v, %v)", dl, del)
	}
	if counter != 12 {
		t.Errorf("the counter must not advance on the immune path, got %d", counter)
	}

	counter = 12
	if dl, del := handleAlreadySavedEpisode(12, &counter, true, false, "ep"); dl || !del {
		t.Errorf("a loose episode overflowing the limit must return (false, true), got (%v, %v)", dl, del)
	}
}

// No-churn: two consecutive cycles over the same state produce no deletion.
func TestSelectEpisodes_BatchStateDoesNotChurn(t *testing.T) {
	anime := animeWithEpisodes(100, anilist.MediaStatusReleasing, false, "")
	savedMap := make(map[files.EpisodeKey]bool, 100)
	savedFull := make(map[files.EpisodeKey]files.EpisodeStruct, 100)
	for i := 1; i <= 100; i++ {
		key := files.EpisodeKey{AnimeID: anime.Media.Id, Episode: i}
		savedMap[key] = true
		savedFull[key] = files.EpisodeStruct{AnimeID: anime.Media.Id, EpisodeNumber: i, EpisodeHash: "pack", IsBatch: true}
	}
	episodes := anilist.EpisodeList(anime, 1)

	for cycle := 1; cycle <= 2; cycle++ {
		sel := selectEpisodes(limitsConfig(), 12, anime, episodes, savedMap, savedFull, map[string]bool{"pack": true}, nil, nil)
		if len(sel.keysToDelete) != 0 {
			t.Fatalf("cycle %d: a saved pack must not produce deletions, got %d", cycle, len(sel.keysToDelete))
		}
		if len(sel.toDownload) != 0 {
			t.Fatalf("cycle %d: nothing new must be downloaded, got %d", cycle, len(sel.toDownload))
		}
	}
}

// watched_episodes_to_keep would break the rotation: keeping 3 of a pack's 100 does not keep
// 3 episodes, it keeps the whole pack (canRemoveTorrent would return false and the torrent
// would stay).
func TestBuildWatchedKeepSet_IgnoresBatchRecords(t *testing.T) {
	anime := animeWithEpisodes(100, anilist.MediaStatusReleasing, false, "")
	savedFull := make(map[files.EpisodeKey]files.EpisodeStruct, 100)
	for i := 1; i <= 100; i++ {
		key := files.EpisodeKey{AnimeID: anime.Media.Id, Episode: i}
		savedFull[key] = files.EpisodeStruct{AnimeID: anime.Media.Id, EpisodeNumber: i, EpisodeHash: "pack", IsBatch: true}
	}
	episodes := anilist.EpisodeList(anime, 1)

	keepSet := buildWatchedKeepSet(3, anime.Media.Id, episodes, savedFull, 100)
	if len(keepSet) != 0 {
		t.Errorf("want an empty keep-set for a pack, got %d", len(keepSet))
	}

	// The pair: a record without IsBatch still respects the keep.
	for key, ep := range savedFull {
		ep.IsBatch = false
		savedFull[key] = ep
	}
	if keepSet := buildWatchedKeepSet(3, anime.Media.Id, episodes, savedFull, 100); len(keepSet) != 3 {
		t.Errorf("a loose episode must keep 3, got %d", len(keepSet))
	}
}

// Full rotation: with pack 1-100 saved and progress 100, the 100 records enter the delete set,
// canRemoveTorrent passes and the whole torrent leaves — and that is what frees the space for
// 101-200 on the next cycle. With progress 50, nothing leaves (there are live siblings) and
// nothing new comes in.
func TestSelectEpisodes_PackRotation(t *testing.T) {
	anime := animeWithEpisodes(200, anilist.MediaStatusReleasing, false, "")
	build := func(progress int) (map[files.EpisodeKey]bool, map[files.EpisodeKey]files.EpisodeStruct, anilist.MediaList) {
		a := anime
		a.Progress = progress
		savedMap := make(map[files.EpisodeKey]bool, 100)
		savedFull := make(map[files.EpisodeKey]files.EpisodeStruct, 100)
		for i := 1; i <= 100; i++ {
			key := files.EpisodeKey{AnimeID: a.Media.Id, Episode: i}
			savedMap[key] = true
			savedFull[key] = files.EpisodeStruct{AnimeID: a.Media.Id, EpisodeNumber: i, EpisodeHash: "pack", IsBatch: true}
		}
		return savedMap, savedFull, a
	}

	// firstEpisodeToConsider needs the real saved list: it is what makes the window recede to
	// episode 1 of the pack. Without that the list would start at progress+1 (101) and the
	// watched pack would never be checked, so it would never enter the delete set — the same
	// thing processAnimeEpisodes does.
	savedMap, savedFull, watchedAll := build(100)
	savedSlice := make([]files.EpisodeStruct, 0, len(savedFull))
	for _, ep := range savedFull {
		savedSlice = append(savedSlice, ep)
	}
	episodes := anilist.EpisodeList(watchedAll, firstEpisodeToConsider(watchedAll, savedSlice))
	keepSet := buildWatchedKeepSet(3, watchedAll.Media.Id, episodes, savedFull, watchedAll.Progress)
	sel := selectEpisodes(limitsConfig(), 12, watchedAll, episodes, savedMap, savedFull, map[string]bool{"pack": true}, keepSet, nil)

	deleteSet := make(map[files.EpisodeKey]bool, len(sel.keysToDelete))
	for _, k := range sel.keysToDelete {
		deleteSet[k] = true
	}
	group := make([]files.EpisodeStruct, 0, 100)
	for _, ep := range savedFull {
		group = append(group, ep)
	}
	if !canRemoveTorrent(group, deleteSet) {
		t.Errorf("with progress 100 the whole group must leave (%d of 100 in the delete set)", len(deleteSet))
	}

	savedMap, savedFull, halfWatched := build(50)
	episodes = anilist.EpisodeList(halfWatched, firstEpisodeToConsider(halfWatched, nil))
	keepSet = buildWatchedKeepSet(3, halfWatched.Media.Id, episodes, savedFull, halfWatched.Progress)
	sel = selectEpisodes(limitsConfig(), 12, halfWatched, episodes, savedMap, savedFull, map[string]bool{"pack": true}, keepSet, nil)
	if len(sel.toDownload) != 0 {
		t.Errorf("with progress 50 nothing new must be searched, got %d", len(sel.toDownload))
	}
}

func TestEffectiveMaxAndWindowEnd(t *testing.T) {
	episodes := make([]anilist.AiringNode, 30)

	unlimited := &files.Config{MaxEpisodesPerAnime: 0}
	if got := effectiveMax(unlimited, episodes); got != 31 {
		t.Errorf("ceiling 0 is no ceiling: want 31, got %d", got)
	}
	if got := windowEnd(unlimited, 5); got != math.MaxInt {
		t.Errorf("ceiling 0 opens the window: want MaxInt, got %d", got)
	}

	limited := &files.Config{MaxEpisodesPerAnime: 12}
	if got := effectiveMax(limited, episodes); got != 12 {
		t.Errorf("want 12, got %d", got)
	}
	if got := windowEnd(limited, 95); got != 106 {
		t.Errorf("a window of 12 from 95 ends at 106, got %d", got)
	}
}
