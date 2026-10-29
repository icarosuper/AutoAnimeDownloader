package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// testLibrarian returns a real Librarian; with empty LibraryPaths it performs no FS ops.
func testLibrarian() files.Librarian {
	return files.NewLibrarian(files.NewOSFileSystem())
}

// fakeWithTorrents returns a FakeBackend pre-populated with completed torrents by hash.
func fakeWithTorrents(hashes ...string) *torrents.FakeBackend {
	fb := torrents.NewFakeBackend()
	for _, h := range hashes {
		fb.AddCompleted(h, "/data/"+h)
	}
	return fb
}

// mockFileManagerForEpisodes implements FileManagerInterface minimally for episode tests.
type mockFileManagerForEpisodes struct {
	deletedEpisodeKeys []files.EpisodeKey
	blockedEpisodeKeys []files.EpisodeKey
	blockErr           error
	standaloneAnimes   []int
	removedStandalone  []int
	savedEpisodes      []files.EpisodeStruct
	settings           map[int]files.AnimeSettings
	upsertErr          error
}

func (m *mockFileManagerForEpisodes) LoadConfigs() (*files.Config, error) { return nil, nil }
func (m *mockFileManagerForEpisodes) SaveConfigs(*files.Config) error     { return nil }
func (m *mockFileManagerForEpisodes) LoadSavedEpisodes() ([]files.EpisodeStruct, error) {
	return m.savedEpisodes, nil
}
func (m *mockFileManagerForEpisodes) SaveEpisodesToFile([]files.EpisodeStruct) error { return nil }
func (m *mockFileManagerForEpisodes) UpsertEpisodes(episodes []files.EpisodeStruct) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	byKey := make(map[files.EpisodeKey]int, len(m.savedEpisodes))
	for i, ep := range m.savedEpisodes {
		byKey[ep.Key()] = i
	}
	for _, ep := range episodes {
		if i, ok := byKey[ep.Key()]; ok {
			m.savedEpisodes[i] = ep
			continue
		}
		byKey[ep.Key()] = len(m.savedEpisodes)
		m.savedEpisodes = append(m.savedEpisodes, ep)
	}
	return nil
}
func (m *mockFileManagerForEpisodes) DeleteEpisodesFromFile(keys []files.EpisodeKey) error {
	m.deletedEpisodeKeys = append(m.deletedEpisodeKeys, keys...)
	return nil
}
func (m *mockFileManagerForEpisodes) DeleteEmptyFolders(string) error { return nil }
func (m *mockFileManagerForEpisodes) LoadBlockedEpisodes() ([]files.EpisodeKey, error) {
	return nil, nil
}
func (m *mockFileManagerForEpisodes) BlockEpisode(key files.EpisodeKey) error {
	m.blockedEpisodeKeys = append(m.blockedEpisodeKeys, key)
	return m.blockErr
}
func (m *mockFileManagerForEpisodes) UnblockEpisode(files.EpisodeKey) error  { return nil }
func (m *mockFileManagerForEpisodes) UnmanageEpisode(files.EpisodeKey) error { return nil }
func (m *mockFileManagerForEpisodes) LoadAllAnimeSettings() (map[int]files.AnimeSettings, error) {
	return nil, nil
}
func (m *mockFileManagerForEpisodes) LoadAnimeSettings(id int) (*files.AnimeSettings, error) {
	if s, ok := m.settings[id]; ok {
		return &s, nil
	}
	return nil, nil
}
func (m *mockFileManagerForEpisodes) SaveAnimeSettings(int, files.AnimeSettings) error { return nil }
func (m *mockFileManagerForEpisodes) LoadStandaloneAnimes() ([]int, error) {
	return m.standaloneAnimes, nil
}
func (m *mockFileManagerForEpisodes) AddStandaloneAnime(id int) error {
	m.standaloneAnimes = append(m.standaloneAnimes, id)
	return nil
}
func (m *mockFileManagerForEpisodes) RemoveStandaloneAnime(id int) error {
	m.removedStandalone = append(m.removedStandalone, id)
	var kept []int
	for _, existing := range m.standaloneAnimes {
		if existing != id {
			kept = append(kept, existing)
		}
	}
	m.standaloneAnimes = kept
	return nil
}

func containsHash(hashes []string, target string) bool {
	for _, h := range hashes {
		if h == target {
			return true
		}
	}
	return false
}

func containsID[T comparable](items []T, target T) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// epKey builds the episode key the production code uses, so the tests don't repeat the literal in
// every assertion.
func epKey(animeID, episode int) files.EpisodeKey {
	return files.EpisodeKey{AnimeID: animeID, Episode: episode}
}

// TestEpisodeInTorrents_HashMatch checks that a saved episode whose hash is present in the torrent
// client counts as "in torrents", even when the torrent name doesn't match. Covers batch torrents
// with the group's original name (e.g. [EMBER]) that doesn't match the anime title used as the key.
func TestEpisodeInTorrents_HashMatch(t *testing.T) {
	const savedHash = "601d1465c25e4e47da30d891ebfeea046bfefdee"

	hashSet := map[string]bool{
		"[EMBER] Mairimashita! Iruma-kun (2021) (Season 2) [1080p]": false, // name key, not relevant
		savedHash: true,
	}

	if !episodeInTorrents(savedHash, hashSet) {
		t.Error("the episode must count as in torrents when the hash matches")
	}
}

// TestEpisodeInTorrents_HashMissing checks that an episode whose torrent was removed from the
// torrent client is marked for re-download.
func TestEpisodeInTorrents_HashMissing(t *testing.T) {
	hashSet := map[string]bool{
		"outrohash": true,
	}

	if episodeInTorrents("601d1465c25e4e47da30d891ebfeea046bfefdee", hashSet) {
		t.Error("the episode must not count as in torrents when the hash is missing")
	}
}

// TestEpisodeInTorrents_EmptyHash checks that an episode with no saved hash (not downloaded) never
// counts as "in torrents", even when the empty string is a key in the map.
func TestEpisodeInTorrents_EmptyHash(t *testing.T) {
	hashSet := map[string]bool{
		"": true, // ensures the savedHash != "" guard is needed
	}

	if episodeInTorrents("", hashSet) {
		t.Error("an empty hash must not count as in torrents")
	}
}

// TestDeleteEpisodesByStatus_DroppedAnime checks that the episodes of a dropped anime are deleted
// from the torrent client and removed from the episodes file.
func TestDeleteEpisodesByStatus_DroppedAnime(t *testing.T) {
	const animeID = 100
	const episodeID = 42
	const episodeHash = "abc123hash"

	deletableMedia := map[int]bool{animeID: true}

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeNumber: episodeID,
			AnimeID:       animeID,
			EpisodeHash:   episodeHash,
			AnimeName:     "Dropped Anime",
			DownloadDate:  time.Now(),
		},
	}

	backend := fakeWithTorrents(episodeHash)
	fm := &mockFileManagerForEpisodes{}

	deleteEpisodesByStatus(deletableMedia, fm, backend, testLibrarian(), savedEpisodes)

	if _, ok := backend.Get(episodeHash); ok {
		t.Errorf("expected torrent %q removed from the client, but it is still there", episodeHash)
	}
	if !containsID(fm.deletedEpisodeKeys, epKey(animeID, episodeID)) {
		t.Errorf("expected episode ID %d removed from the file, got %v", episodeID, fm.deletedEpisodeKeys)
	}
}

// TestDeleteEpisodesByStatus_ManuallyManagedNotDeleted checks that episodes marked as manually
// managed aren't deleted even when the anime is dropped.
func TestDeleteEpisodesByStatus_ManuallyManagedNotDeleted(t *testing.T) {
	const animeID = 200
	const episodeID = 99
	const episodeHash = "manualhash"

	deletableMedia := map[int]bool{animeID: true}

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeNumber:   episodeID,
			AnimeID:         animeID,
			EpisodeHash:     episodeHash,
			AnimeName:       "Dropped Anime",
			ManuallyManaged: true,
		},
	}

	backend := fakeWithTorrents(episodeHash)
	fm := &mockFileManagerForEpisodes{}

	deleteEpisodesByStatus(deletableMedia, fm, backend, testLibrarian(), savedEpisodes)

	if _, ok := backend.Get(episodeHash); !ok {
		t.Error("a manually managed episode must not have its torrent removed")
	}
	if containsID(fm.deletedEpisodeKeys, epKey(animeID, episodeID)) {
		t.Error("a manually managed episode must not be removed from the file")
	}
}

// TestCheckEpisode_BlacklistedEpisodeMarkedForDeletion checks that an already downloaded episode of
// a blacklisted anime is marked for deletion (shouldDelete=true).
func TestCheckEpisode_BlacklistedEpisodeMarkedForDeletion(t *testing.T) {
	englishTitle := "Blacklisted Anime"
	anime := anilist.MediaList{
		Id:       300,
		Progress: 0,
		Media: anilist.Media{
			Title: anilist.Title{English: &englishTitle},
		},
		CustomLists: anilist.CustomLists{"Blacklist": true},
	}

	ep := anilist.AiringNode{ID: 55, Episode: 1, TimeUntilAiring: -100}

	configs := &files.Config{
		ExcludedLists:       []string{"Blacklist"},
		MaxEpisodesPerAnime: 12,
	}

	downloaded := 0
	shouldDownload, shouldDelete, _ := checkEpisode(configs, configs.MaxEpisodesPerAnime, ep, anime, true, &downloaded, false, false, false)

	if shouldDownload {
		t.Error("an episode of a blacklisted anime must not be downloaded")
	}
	if !shouldDelete {
		t.Error("an already downloaded episode of a blacklisted anime must be marked for deletion")
	}
}

// TestHandleSavedEpisodes_BlacklistedAnime_DeletesTorrents checks that episodes marked for deletion
// (e.g. a blacklisted anime) are actually deleted from the torrent client.
func TestHandleSavedEpisodes_BlacklistedAnime_DeletesTorrents(t *testing.T) {
	const episodeID = 55
	const episodeHash = "blacklisthash"

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeNumber: episodeID,
			AnimeID:       300,
			EpisodeHash:   episodeHash,
			AnimeName:     "Blacklisted Anime",
		},
	}

	configs := &files.Config{
		DeleteWatchedEpisodes: true,
		MaxEpisodesPerAnime:   12,
	}

	backend := fakeWithTorrents(episodeHash)
	fm := &mockFileManagerForEpisodes{}

	data := handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		keysToDelete:    []files.EpisodeKey{epKey(300, episodeID)},
		checkedEpisodes: []files.EpisodeKey{epKey(300, episodeID)},
	}

	handleSavedEpisodes(fm, configs, backend, testLibrarian(), data)

	if _, ok := backend.Get(episodeHash); ok {
		t.Errorf("expected torrent %q removed from the client, but it is still there", episodeHash)
	}
}

// TestProcessAnimeEpisodes_BlacklistedAnime_PopulatesKeysToDelete checks that already downloaded
// episodes of a blacklisted anime are included in the result's keysToDelete.
func TestProcessAnimeEpisodes_BlacklistedAnime_PopulatesKeysToDelete(t *testing.T) {
	const episodeNumber = 1
	const animeID = 300

	englishTitle := "Blacklisted Anime"
	anime := anilist.MediaList{
		Id:       animeID,
		Progress: 0,
		Status:   anilist.MediaListStatusCurrent,
		Media: anilist.Media{
			Id:     animeID,
			Status: anilist.MediaStatusReleasing,
			Title:  anilist.Title{English: &englishTitle},
			AiringSchedule: anilist.AiringSchedule{
				Nodes: []anilist.AiringNode{
					{Episode: episodeNumber, TimeUntilAiring: -100},
				},
			},
		},
		CustomLists: anilist.CustomLists{"Blacklist": true},
	}

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeNumber: episodeNumber,
			AnimeID:       animeID,
			EpisodeHash:   "somehash",
			AnimeName:     "Blacklisted Anime",
		},
	}

	configs := &files.Config{
		ExcludedLists:       []string{"Blacklist"},
		MaxEpisodesPerAnime: 12,
	}

	backend := torrents.NewFakeBackend()

	result := processAnimeEpisodes(configs, backend, anime, nil, savedEpisodes, nil, map[files.EpisodeKey]bool{}, defaultNyaaSearcher(), nil)

	if !containsID(result.keysToDelete, epKey(animeID, episodeNumber)) {
		t.Errorf("expected episode %d in keysToDelete, got %v", episodeNumber, result.keysToDelete)
	}
	if len(result.newEpisodes) > 0 {
		t.Error("a blacklisted anime must not get new episodes downloaded")
	}
}

// TestHandleSavedEpisodes_BlacklistedAnime_NoDeleteWhenFlagOff checks that episodes marked for
// deletion are NOT deleted when DeleteWatchedEpisodes=false.
func TestHandleSavedEpisodes_BlacklistedAnime_NoDeleteWhenFlagOff(t *testing.T) {
	const episodeID = 55
	const episodeHash = "blacklisthash"

	savedEpisodes := []files.EpisodeStruct{
		{
			EpisodeNumber: episodeID,
			AnimeID:       300,
			EpisodeHash:   episodeHash,
			AnimeName:     "Blacklisted Anime",
		},
	}

	configs := &files.Config{
		DeleteWatchedEpisodes: false,
		MaxEpisodesPerAnime:   12,
	}

	backend := fakeWithTorrents(episodeHash)
	fm := &mockFileManagerForEpisodes{}

	data := handleEpisodesData{
		savedEpisodes:   savedEpisodes,
		keysToDelete:    []files.EpisodeKey{epKey(300, episodeID)},
		checkedEpisodes: []files.EpisodeKey{epKey(300, episodeID)},
	}

	handleSavedEpisodes(fm, configs, backend, testLibrarian(), data)

	if _, ok := backend.Get(episodeHash); !ok {
		t.Error("the torrent must not be removed when DeleteWatchedEpisodes=false")
	}
}

// TestProcessAnimeEpisodes_BatchNoRedownload is a regression test for the bug where completed
// animes were re-downloaded in a loop because the check used the name instead of the hash. The
// torrent "[EMBER] Mairimashita!..." has a name that never matches the name-based search key, but
// the hash "601d1465..." matches — the correct code must detect that and not trigger a re-download.
func TestProcessAnimeEpisodes_BatchNoRedownload(t *testing.T) {
	const batchHash = "601d1465c25e4e47da30d891ebfeea046bfefdee"
	const animeID = 101972
	englishTitle := "Mairimashita! Iruma-kun Season 2"

	// 12 episode nodes, all already aired
	nodes := make([]anilist.AiringNode, 12)
	for i := range nodes {
		nodes[i] = anilist.AiringNode{ID: 1000 + i, Episode: i + 1, TimeUntilAiring: -100}
	}

	anime := anilist.MediaList{
		Id:       animeID,
		Progress: 0,
		Status:   anilist.MediaListStatusCurrent,
		Media: anilist.Media{
			Id:     animeID,
			Status: anilist.MediaStatusFinished,
			Title:  anilist.Title{English: &englishTitle},
			AiringSchedule: anilist.AiringSchedule{
				Nodes: nodes,
			},
		},
	}

	// All 12 episodes already saved, all with the same hash as the batch torrent
	savedEpisodes := make([]files.EpisodeStruct, 12)
	for i := range savedEpisodes {
		savedEpisodes[i] = files.EpisodeStruct{
			EpisodeNumber: i + 1, // must match the node's Episode (i+1); a number != the nodes' Episode made
			// alreadySaved always false and masked the test (pre-existing issue, outside this task)
			AnimeID:      animeID,
			EpisodeHash:  batchHash,
			AnimeName:    englishTitle,
			DownloadDate: time.Now(),
		}
	}

	// The client has the batch torrent with the group's original name (doesn't match by name)
	dlTorrents := []torrents.TorrentInfo{
		{
			Name: "[EMBER] Mairimashita! Iruma-kun (2021) (Season 2) [1080p][HEVC][AAC]",
			Hash: batchHash,
		},
	}

	// Nyaa mock: if the anime search is called, the test must fail
	searchAnimeCalled := false
	mockSearcher := nyaaSearcher{
		searchAnime: func(_ anilist.Title, _ []string, _ []int) []nyaa.TorrentResult {
			searchAnimeCalled = true
			return []nyaa.TorrentResult{{MagnetLink: "magnet:?xt=urn:btih:fakehash", IsBatch: true}}
		},
		searchSingleEpisode: func(_ anilist.AiringNode, _ anilist.Title, _ []string, _ anilist.MediaRelations, _ int) []nyaa.TorrentResult {
			return nil
		},
		searchMovie: func(_ anilist.Title, _ bool) []nyaa.TorrentResult {
			return nil
		},
	}

	configs := &files.Config{
		MaxEpisodesPerAnime: 12,
		EpisodeRetryLimit:   1,
	}

	backend := torrents.NewFakeBackend()

	result := processAnimeEpisodes(configs, backend, anime, dlTorrents, savedEpisodes, nil, map[files.EpisodeKey]bool{}, mockSearcher, nil)

	if searchAnimeCalled {
		t.Error("the anime search must not be called: every episode is already in the client by hash")
	}
	if len(backend.List()) > 0 {
		t.Errorf("Add must not be called, but the client has %d torrent(s)", len(backend.List()))
	}
	if len(result.newEpisodes) > 0 {
		t.Errorf("newEpisodes must be empty, got %d episode(s)", len(result.newEpisodes))
	}
	if len(result.keysToDelete) > 0 {
		t.Errorf("keysToDelete must be empty, got %v", result.keysToDelete)
	}
}

// TestDedupeAnimesByMedia checks that the same anime linked in different accounts collapses into a
// single entry, keeping the one with the LOWEST progress (the most conservative, so as not to
// delete episodes some account hasn't watched yet).
func TestDedupeAnimesByMedia(t *testing.T) {
	list := []anilist.MediaList{
		{Id: 1, Progress: 10, Media: anilist.Media{Id: 500}}, // account A, ahead
		{Id: 2, Progress: 3, Media: anilist.Media{Id: 500}},  // account B, behind (same anime)
		{Id: 3, Progress: 7, Media: anilist.Media{Id: 999}},  // another anime, account A only
	}

	got := anilist.DedupeByMedia(list)

	if len(got) != 2 {
		t.Fatalf("expected 2 animes after dedup, got %d", len(got))
	}

	var media500 *anilist.MediaList
	for i := range got {
		if got[i].Media.Id == 500 {
			media500 = &got[i]
		}
	}
	if media500 == nil {
		t.Fatal("anime media 500 sumiu do resultado")
	}
	if media500.Progress != 3 {
		t.Errorf("expected progress 3 (the lowest across accounts), got %d", media500.Progress)
	}
	if media500.Id != 2 {
		t.Errorf("expected to keep account B's entry (Id 2), got Id %d", media500.Id)
	}
}

// TestDedupeAnimesByMedia_NoMediaID ensures entries without a media id (an unexpected case) are
// kept instead of all collapsing into id 0.
func TestDedupeAnimesByMedia_NoMediaID(t *testing.T) {
	list := []anilist.MediaList{
		{Id: 1, Progress: 5},
		{Id: 2, Progress: 8},
	}

	got := anilist.DedupeByMedia(list)

	if len(got) != 2 {
		t.Errorf("entries without a media id must not collapse, expected 2, got %d", len(got))
	}
}

// --- saveEpisodesToFile: selective merge on re-download (P1.1 / P1.2) ---

const (
	oldHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	newHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// tempFileManager returns a real files.FileManager over a temp dir, so the merge tests
// exercise the actual JSONL round-trip instead of a mock's in-memory slice.
func tempFileManager(t *testing.T) *files.FileManager {
	t.Helper()
	dir := t.TempDir()
	return files.NewManager(
		files.NewOSFileSystem(),
		filepath.Join(dir, "config.json"),
		filepath.Join(dir, "downloaded_episodes"),
		filepath.Join(dir, "blocked_episodes"),
		filepath.Join(dir, "anime_settings"),
		filepath.Join(dir, "standalone_animes"),
	)
}

// loadEpisodeByID reads the persisted episodes back and returns the one with the given ID.
func loadEpisodeByID(t *testing.T, fm FileManagerInterface, id int) files.EpisodeStruct {
	t.Helper()
	all, err := fm.LoadSavedEpisodes()
	if err != nil {
		t.Fatalf("LoadSavedEpisodes: %v", err)
	}
	for _, ep := range all {
		if ep.EpisodeNumber == id {
			return ep
		}
	}
	t.Fatalf("episode %d not found in saved episodes %+v", id, all)
	return files.EpisodeStruct{}
}

// TestSaveEpisodesToFile_RedownloadUpdatesRecord: an already saved episode that's re-downloaded
// must end up with the NEW EpisodeHash in the file — SaveEpisodesToFile is append-only and would
// discard the update, leaving the record with the old hash, which is what JobOrganize uses to find
// the torrent (without it the hardlink never comes out).
func TestSaveEpisodesToFile_RedownloadUpdatesRecord(t *testing.T) {
	fm := tempFileManager(t)
	old := time.Now().Add(-48 * time.Hour)
	if err := fm.SaveEpisodesToFile([]files.EpisodeStruct{{
		EpisodeNumber: 5,
		AnimeID:       7,
		AnimeName:     "My Anime",
		EpisodeHash:   oldHash,
		EpisodeName:   "My Anime - Episode 5",
		DownloadDate:  old,
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	saveEpisodesToFile(fm, []files.EpisodeStruct{{
		AnimeID:            7,
		AnimeTotalEpisodes: 12,
		AnimeName:          "My Anime",
		EpisodeHash:        newHash,
		EpisodeName:        "My Anime - Episode 5",
		EpisodeNumber:      5,
		IsBatch:            true,
		TorrentSize:        7 << 30,
		DownloadDate:       time.Now(),
	}})

	all, err := fm.LoadSavedEpisodes()
	if err != nil {
		t.Fatalf("LoadSavedEpisodes: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 record (merge, not a duplicate), got %d: %+v", len(all), all)
	}

	got := all[0]
	if got.EpisodeNumber != 5 {
		t.Errorf("EpisodeNumber: expected 5, got %d", got.EpisodeNumber)
	}
	if got.EpisodeHash != newHash {
		t.Errorf("EpisodeHash: expected %s, got %s", newHash, got.EpisodeHash)
	}
	if !got.IsBatch {
		t.Error("IsBatch must be updated to true")
	}
	// The disk budget reads a queued torrent's size from here (newDiskBudget).
	if got.TorrentSize != 7<<30 {
		t.Errorf("TorrentSize: expected %d, got %d", 7<<30, got.TorrentSize)
	}
	if got.AnimeTotalEpisodes != 12 {
		t.Errorf("AnimeTotalEpisodes: expected 12, got %d", got.AnimeTotalEpisodes)
	}
	if !got.DownloadDate.After(old) {
		t.Errorf("DownloadDate must be updated on re-download, got %v", got.DownloadDate)
	}
}

// TestSaveEpisodesToFile_PreservesManuallyManaged ensures the merge doesn't erase the user's flag —
// otherwise the automatic loop could start deleting an episode managed by hand.
func TestSaveEpisodesToFile_PreservesManuallyManaged(t *testing.T) {
	fm := tempFileManager(t)
	if err := fm.SaveEpisodesToFile([]files.EpisodeStruct{{
		EpisodeNumber:   5,
		AnimeID:         7,
		EpisodeHash:     oldHash,
		ManuallyManaged: true,
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// The automatic loop never marks ManuallyManaged.
	saveEpisodesToFile(fm, []files.EpisodeStruct{{
		EpisodeNumber: 5,
		AnimeID:       7,
		EpisodeHash:   newHash,
	}})

	got := loadEpisodeByID(t, fm, 5)
	if !got.ManuallyManaged {
		t.Error("ManuallyManaged must survive an automatic re-download")
	}
	if got.EpisodeHash != newHash {
		t.Errorf("EpisodeHash must be updated even while preserving ManuallyManaged, got %s", got.EpisodeHash)
	}
}

// TestSaveEpisodesToFile_ResetsLibraryPaths ensures the library paths of the OLD release are
// zeroed: empty LibraryPaths is the "not yet organized" marker that makes JobOrganize create the
// new hardlink and fire the webhook.
func TestSaveEpisodesToFile_ResetsLibraryPaths(t *testing.T) {
	fm := tempFileManager(t)
	if err := fm.SaveEpisodesToFile([]files.EpisodeStruct{{
		EpisodeNumber: 5,
		AnimeID:       7,
		EpisodeHash:   oldHash,
		LibraryPaths:  []string{"/library/My Anime/My Anime - E05.mkv"},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	saveEpisodesToFile(fm, []files.EpisodeStruct{{
		EpisodeNumber: 5,
		AnimeID:       7,
		EpisodeHash:   newHash,
	}})

	got := loadEpisodeByID(t, fm, 5)
	if len(got.LibraryPaths) != 0 {
		t.Errorf("LibraryPaths must be zeroed on re-download, got %v", got.LibraryPaths)
	}
}

// TestSaveEpisodesToFile_AppendsNewEpisode ensures the merge doesn't regress the current behavior:
// a genuinely new episode keeps being appended without touching the existing ones.
func TestSaveEpisodesToFile_AppendsNewEpisode(t *testing.T) {
	fm := tempFileManager(t)
	if err := fm.SaveEpisodesToFile([]files.EpisodeStruct{{
		EpisodeNumber:   1,
		AnimeID:         7,
		EpisodeHash:     oldHash,
		ManuallyManaged: true,
		LibraryPaths:    []string{"/library/My Anime/My Anime - E01.mkv"},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	saveEpisodesToFile(fm, []files.EpisodeStruct{{
		EpisodeNumber: 2,
		AnimeID:       7,
		EpisodeHash:   newHash,
	}})

	all, err := fm.LoadSavedEpisodes()
	if err != nil {
		t.Fatalf("LoadSavedEpisodes: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 records, got %d: %+v", len(all), all)
	}

	untouched := loadEpisodeByID(t, fm, 1)
	if !untouched.ManuallyManaged || len(untouched.LibraryPaths) != 1 || untouched.EpisodeHash != oldHash {
		t.Errorf("an unrelated record was changed: %+v", untouched)
	}
	added := loadEpisodeByID(t, fm, 2)
	if added.EpisodeHash != newHash || added.EpisodeNumber != 2 {
		t.Errorf("the new episode was not appended correctly: %+v", added)
	}
}

// --- error propagation on removal (P1.3) ---

// failingFM is a FileManager mock whose load/delete can be made to fail.
type failingFM struct {
	mockFileManagerForEpisodes
	saved     []files.EpisodeStruct
	loadErr   error
	deleteErr error
	deleted   []files.EpisodeKey
}

func (m *failingFM) LoadSavedEpisodes() ([]files.EpisodeStruct, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return m.saved, nil
}

func (m *failingFM) DeleteEpisodesFromFile(keys []files.EpisodeKey) error {
	m.deleted = append(m.deleted, keys...)
	return m.deleteErr
}

// TestRemoveEpisodesWithLinks_PropagatesLoadError: a failure reading the episodes file must become
// an error for the caller (the API answers 500) instead of a silent "success".
func TestRemoveEpisodesWithLinks_PropagatesLoadError(t *testing.T) {
	fm := &failingFM{loadErr: errors.New("disk on fire")}
	err := RemoveEpisodesWithLinks(fm, torrents.NewFakeBackend(), testLibrarian(), []files.EpisodeKey{{Episode: 1}})
	if err == nil {
		t.Fatal("expected an error when LoadSavedEpisodes fails")
	}
}

// TestRemoveEpisodesWithLinks_PropagatesDeleteError: a failure removing from the file must become
// an error.
func TestRemoveEpisodesWithLinks_PropagatesDeleteError(t *testing.T) {
	const hash = "cccccccccccccccccccccccccccccccccccccccc"
	fm := &failingFM{
		saved:     []files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: hash}},
		deleteErr: errors.New("write failed"),
	}
	backend := fakeWithTorrents(hash)

	err := RemoveEpisodesWithLinks(fm, backend, testLibrarian(), []files.EpisodeKey{{Episode: 1}})
	if err == nil {
		t.Fatal("expected an error when DeleteEpisodesFromFile fails")
	}
	// Freeing space is best-effort and happens first: the torrent goes anyway.
	if _, ok := backend.Get(hash); ok {
		t.Error("the torrent must be removed even when writing the file fails")
	}
}

// TestRemoveEpisodesWithLinks_SuccessReturnsNil ensures the happy path stays error-free.
func TestRemoveEpisodesWithLinks_SuccessReturnsNil(t *testing.T) {
	const hash = "dddddddddddddddddddddddddddddddddddddddd"
	fm := &failingFM{saved: []files.EpisodeStruct{{EpisodeNumber: 1, EpisodeHash: hash}}}
	if err := RemoveEpisodesWithLinks(fm, fakeWithTorrents(hash), testLibrarian(), []files.EpisodeKey{epKey(0, 1)}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !containsID(fm.deleted, files.EpisodeKey{Episode: 1}) {
		t.Errorf("expected episode 1 deleted from the file, got %v", fm.deleted)
	}
}

// TestHandleSavedEpisodes_DeleteErrorIsTolerated: the automatic loop stays best-effort — a failure
// deleting from the file is logged, it doesn't abort the pass nor prevent freeing space.
func TestHandleSavedEpisodes_DeleteErrorIsTolerated(t *testing.T) {
	const hash = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	saved := []files.EpisodeStruct{{EpisodeNumber: 55, AnimeID: 300, EpisodeHash: hash}}
	fm := &failingFM{saved: saved, deleteErr: errors.New("write failed")}
	backend := fakeWithTorrents(hash)

	handleSavedEpisodes(fm, &files.Config{DeleteWatchedEpisodes: true, MaxEpisodesPerAnime: 12}, backend, testLibrarian(), handleEpisodesData{
		savedEpisodes:   saved,
		keysToDelete:    []files.EpisodeKey{epKey(300, 55)},
		checkedEpisodes: []files.EpisodeKey{epKey(300, 55)},
	})

	if _, ok := backend.Get(hash); ok {
		t.Error("the pass must go on and remove the torrent even when writing the file fails")
	}
	if !containsID(fm.deleted, epKey(300, 55)) {
		t.Errorf("the deletion must have been attempted, got %v", fm.deleted)
	}
}

// TestDeleteEpisodesByStatus_DeleteErrorIsTolerated: same for deletion by status.
func TestDeleteEpisodesByStatus_DeleteErrorIsTolerated(t *testing.T) {
	const hash = "ffffffffffffffffffffffffffffffffffffffff"
	deletableMedia := map[int]bool{100: true}

	saved := []files.EpisodeStruct{{EpisodeNumber: 42, AnimeID: 100, EpisodeHash: hash}}
	fm := &failingFM{saved: saved, deleteErr: errors.New("write failed")}
	backend := fakeWithTorrents(hash)

	deleteEpisodesByStatus(deletableMedia, fm, backend, testLibrarian(), saved)

	if _, ok := backend.Get(hash); ok {
		t.Error("the torrent must be removed even when writing the file fails")
	}
}

// --- RemoveTorrentWithEpisodes (Task 1.3 / 1.4) ---

// countingBackend wraps FakeBackend to count Remove calls per hash — FakeBackend.RemovedKeepData
// alone can't distinguish "called once" from "called N times with the same keepData value".
type countingBackend struct {
	*torrents.FakeBackend
	removeCalls map[string]int
}

func newCountingBackend(hashes ...string) *countingBackend {
	return &countingBackend{FakeBackend: fakeWithTorrents(hashes...), removeCalls: make(map[string]int)}
}

func (c *countingBackend) Remove(hash string, keepData bool) error {
	c.removeCalls[hash]++
	return c.FakeBackend.Remove(hash, keepData)
}

// TestRemoveTorrentWithEpisodes_BatchRemovesAllAndCallsBackendOnce checks that a batch with N
// episodes of the same hash is removed as a unit: the N records leave at once and backend.Remove is
// called exactly once (not once per episode).
func TestRemoveTorrentWithEpisodes_BatchRemovesAllAndCallsBackendOnce(t *testing.T) {
	const hash = "1111111111111111111111111111111111111111"
	saved := []files.EpisodeStruct{
		{EpisodeNumber: 1, EpisodeHash: hash},
		{EpisodeNumber: 2, EpisodeHash: hash},
		{EpisodeNumber: 3, EpisodeHash: hash},
	}
	fm := &failingFM{saved: saved}
	backend := newCountingBackend(hash)

	err := RemoveTorrentWithEpisodes(fm, backend, testLibrarian(), hash, RemoveTorrentOptions{})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	for _, ep := range []int{1, 2, 3} {
		if !containsID(fm.deleted, files.EpisodeKey{Episode: ep}) {
			t.Errorf("expected episode %d removed from the file, got %v", ep, fm.deleted)
		}
	}
	if _, ok := backend.Get(hash); ok {
		t.Error("expected the torrent removed from the client")
	}
	if got := backend.removeCalls[hash]; got != 1 {
		t.Fatalf("expected backend.Remove called exactly once for the batch, got %d", got)
	}
	if kd, ok := backend.RemovedKeepData[hash]; !ok || kd {
		t.Errorf("expected backend.Remove called with keepData=false for %s, got %v (ok=%v)", hash, kd, ok)
	}
}

// TestRemoveTorrentWithEpisodes_KeepDataSkipsLibraryRemoval checks that keepData=true doesn't call
// librarian.RemoveFromLibrary and that the value reaches the backend as keepData=true (via
// FakeBackend.RemovedKeepData).
func TestRemoveTorrentWithEpisodes_KeepDataSkipsLibraryRemoval(t *testing.T) {
	const hash = "2222222222222222222222222222222222222222"
	tmp := t.TempDir()
	libPath := filepath.Join(tmp, "library", "ep.mkv")

	saved := []files.EpisodeStruct{
		{EpisodeNumber: 1, EpisodeHash: hash, LibraryPaths: []string{libPath}},
	}
	fm := &failingFM{saved: saved}
	backend := fakeWithTorrents(hash)

	// spyLibrarian records whether RemoveFromLibrary was called.
	spy := &spyLibrarian{}

	err := RemoveTorrentWithEpisodes(fm, backend, spy, hash, RemoveTorrentOptions{KeepData: true})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if spy.called {
		t.Error("keepData=true must not call librarian.RemoveFromLibrary")
	}
	if kd, ok := backend.RemovedKeepData[hash]; !ok || !kd {
		t.Errorf("expected backend.Remove called with keepData=true, got %v (ok=%v)", kd, ok)
	}
}

// spyLibrarian is a minimal files.Librarian that only records whether RemoveFromLibrary was
// invoked, so tests can assert it was skipped without touching a real filesystem.
type spyLibrarian struct {
	called bool
}

func (s *spyLibrarian) Organize(files.OrganizeRequest) ([]string, error) { return nil, nil }
func (s *spyLibrarian) RemoveFromLibrary(string) error {
	s.called = true
	return nil
}
func (s *spyLibrarian) RemoveFolderIfNoVideo(string) error { return nil }
func (s *spyLibrarian) ProbePath(string) error             { return nil }

// TestRemoveTorrentWithEpisodes_OrphanTorrentCallsBackendOnly checks the orphan torrent case (no
// saved episode matches the hash): backend.Remove is called, nothing is blocked, no error.
func TestRemoveTorrentWithEpisodes_OrphanTorrentCallsBackendOnly(t *testing.T) {
	const hash = "3333333333333333333333333333333333333333"
	fm := &failingFM{saved: nil}
	backend := fakeWithTorrents(hash)

	err := RemoveTorrentWithEpisodes(fm, backend, testLibrarian(), hash, RemoveTorrentOptions{Block: true})
	if err != nil {
		t.Fatalf("expected success for an orphan torrent, got %v", err)
	}
	if _, ok := backend.Get(hash); ok {
		t.Error("expected the orphan torrent removed from the client")
	}
	if len(fm.blockedEpisodeKeys) != 0 {
		t.Errorf("an orphan torrent has no episode to block, got %v", fm.blockedEpisodeKeys)
	}
	if len(fm.deleted) != 0 {
		t.Errorf("an orphan torrent must not try to delete records, got %v", fm.deleted)
	}
}

// TestRemoveTorrentWithEpisodes_BlockBlocksAllIDsInGroup checks that Block=true blocks every id in
// the group before removing the records.
func TestRemoveTorrentWithEpisodes_BlockBlocksAllIDsInGroup(t *testing.T) {
	const hash = "4444444444444444444444444444444444444444"
	saved := []files.EpisodeStruct{
		{EpisodeNumber: 10, EpisodeHash: hash},
		{EpisodeNumber: 20, EpisodeHash: hash},
	}
	fm := &failingFM{saved: saved}
	backend := fakeWithTorrents(hash)

	err := RemoveTorrentWithEpisodes(fm, backend, testLibrarian(), hash, RemoveTorrentOptions{Block: true})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	for _, ep := range []int{10, 20} {
		if !containsID(fm.blockedEpisodeKeys, files.EpisodeKey{Episode: ep}) {
			t.Errorf("expected episode %d blocked, got %v", ep, fm.blockedEpisodeKeys)
		}
	}
}

// TestRemoveTorrentWithEpisodes_LoadErrorAbortsWithoutRemoving checks that a LoadSavedEpisodes
// failure returns an error and doesn't call backend.Remove.
func TestRemoveTorrentWithEpisodes_LoadErrorAbortsWithoutRemoving(t *testing.T) {
	const hash = "5555555555555555555555555555555555555555"
	fm := &failingFM{loadErr: errors.New("disk on fire")}
	backend := fakeWithTorrents(hash)

	err := RemoveTorrentWithEpisodes(fm, backend, testLibrarian(), hash, RemoveTorrentOptions{})
	if err == nil {
		t.Fatal("expected an error when LoadSavedEpisodes fails")
	}
	if _, ok := backend.Get(hash); !ok {
		t.Error("backend.Remove must not be called when LoadSavedEpisodes fails")
	}
}

// TestProcessAnimeEpisodes_NoMagnets_SkipsNewEpisodeWebhook: when no search finds a torrent,
// the episode must not fire "new episode detected, starting download" — that push used to go
// out on every loop pass (every 10 min) for an episode that never started downloading, and
// also made attemptDownloadWithRetries log "failed after all attempts" with zero attempts.
// Only the failure webhook (reason: no torrent found) should fire.
func TestProcessAnimeEpisodes_NoMagnets_SkipsNewEpisodeWebhook(t *testing.T) {
	events := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		events <- string(body)
	}))
	defer srv.Close()

	englishTitle := "No Torrent"
	anime := anilist.MediaList{
		Id:       900,
		Progress: 0,
		Status:   anilist.MediaListStatusCurrent,
		Media: anilist.Media{
			Id:     900,
			Status: anilist.MediaStatusReleasing,
			Title:  anilist.Title{English: &englishTitle},
			AiringSchedule: anilist.AiringSchedule{
				Nodes: []anilist.AiringNode{{ID: 9001, Episode: 1, TimeUntilAiring: -100}},
			},
		},
	}

	noResults := nyaaSearcher{
		searchAnime: func(anilist.Title, []string, []int) []nyaa.TorrentResult { return nil },
		searchSingleEpisode: func(anilist.AiringNode, anilist.Title, []string, anilist.MediaRelations, int) []nyaa.TorrentResult {
			return nil
		},
		searchMovie: func(anilist.Title, bool) []nyaa.TorrentResult { return nil },
	}

	configs := &files.Config{
		MaxEpisodesPerAnime: 12,
		EpisodeRetryLimit:   3,
		Notifications: files.NotificationsConfig{
			Webhooks: []files.WebhookPreset{{
				Name: "hook", URL: srv.URL, Method: "POST", Headers: map[string]string{},
				Body:   "{{title}}",
				Events: []string{"new_episode", "download_failed"},
			}},
		},
	}

	backend := torrents.NewFakeBackend()
	result := processAnimeEpisodes(configs, backend, anime, nil, nil, nil, map[files.EpisodeKey]bool{}, noResults, nil)

	if len(result.newEpisodes) > 0 {
		t.Errorf("no episode should be saved without a magnet, got %d", len(result.newEpisodes))
	}
	if len(backend.List()) > 0 {
		t.Errorf("backend.Add must not be called without a magnet, client has %d torrent(s)", len(backend.List()))
	}

	var got []string
	for range 2 {
		select {
		case body := <-events:
			got = append(got, body)
		case <-time.After(300 * time.Millisecond):
		}
	}
	if len(got) != 1 || got[0] != "Download failed" {
		t.Errorf("expected only the failure webhook, got %v", got)
	}
}

// TestFirstEpisodeToConsider covers the rule of "where the episode list starts": progress + 1 on
// the user's list, 1 on a standalone (which has progress 0), stepping back to the lowest episode
// already saved — without that step back, an episode saved below the progress would never be
// "checked" and the pruning would delete it ignoring watched_episodes_to_keep.
func TestFirstEpisodeToConsider(t *testing.T) {
	anime := func(progress int) anilist.MediaList {
		return anilist.MediaList{Progress: progress, Media: anilist.Media{Id: 21}}
	}
	saved := func(nums ...int) []files.EpisodeStruct {
		out := make([]files.EpisodeStruct, 0, len(nums))
		for _, n := range nums {
			out = append(out, files.EpisodeStruct{AnimeID: 21, EpisodeNumber: n})
		}
		return out
	}

	cases := []struct {
		name     string
		progress int
		saved    []files.EpisodeStruct
		want     int
	}{
		{"a standalone (progress 0) starts at 1", 0, nil, 1},
		{"the user's list starts after the watched one", 1122, nil, 1123},
		{"steps back to the lowest saved episode", 1122, saved(1120, 1125), 1120},
		{"a saved episode of ANOTHER anime does not count", 1122, []files.EpisodeStruct{{AnimeID: 99, EpisodeNumber: 3}}, 1123},
		{"a record without a number does not drag the list to zero", 5, saved(0), 6},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstEpisodeToConsider(anime(tc.progress), tc.saved); got != tc.want {
				t.Errorf("want %d, got %d", tc.want, got)
			}
		})
	}
}

// TestCheckEpisode_SkipCode: only the per-anime limit goes into the report. Every normal skip
// (excluded list, already watched, not yet released) returns "" — an up-to-date anime produces
// dozens of them per pass, and if they went into the report the real problems would be lost in the
// noise.
func TestCheckEpisode_SkipCode(t *testing.T) {
	title := "SkipCode Anime"
	base := anilist.MediaList{
		Id:       400,
		Progress: 2,
		Media: anilist.Media{
			Id:    400,
			Title: anilist.Title{English: &title},
		},
	}

	t.Run("per-anime limit reached", func(t *testing.T) {
		configs := &files.Config{MaxEpisodesPerAnime: 2}
		downloaded := 2
		ep := anilist.AiringNode{ID: 1, Episode: 5, TimeUntilAiring: -100}

		shouldDownload, shouldDelete, code := checkEpisode(configs, 2, ep, base, false, &downloaded, false, false, false)

		if shouldDownload || shouldDelete {
			t.Errorf("a reached limit neither downloads nor deletes, got (%v, %v)", shouldDownload, shouldDelete)
		}
		if code != IssueMaxEpisodesPerAnime {
			t.Errorf("expected %q, got %q", IssueMaxEpisodesPerAnime, code)
		}
	})

	t.Run("already watched", func(t *testing.T) {
		configs := &files.Config{MaxEpisodesPerAnime: 12}
		downloaded := 0
		ep := anilist.AiringNode{ID: 2, Episode: 1, TimeUntilAiring: -100}

		_, _, code := checkEpisode(configs, 12, ep, base, false, &downloaded, false, false, false)

		if code != "" {
			t.Errorf("a normal skip does not go into the report, got %q", code)
		}
	})

	t.Run("not released yet", func(t *testing.T) {
		configs := &files.Config{MaxEpisodesPerAnime: 12}
		downloaded := 0
		ep := anilist.AiringNode{ID: 3, Episode: 9, TimeUntilAiring: 3600}

		_, _, code := checkEpisode(configs, 12, ep, base, false, &downloaded, false, false, false)

		if code != "" {
			t.Errorf("a normal skip does not go into the report, got %q", code)
		}
	})

	t.Run("downloadable episode", func(t *testing.T) {
		configs := &files.Config{MaxEpisodesPerAnime: 12}
		downloaded := 0
		ep := anilist.AiringNode{ID: 4, Episode: 5, TimeUntilAiring: -100}

		shouldDownload, _, code := checkEpisode(configs, 12, ep, base, false, &downloaded, false, false, false)

		if !shouldDownload {
			t.Error("expected shouldDownload=true")
		}
		if code != "" {
			t.Errorf("a downloaded episode has no skip reason, got %q", code)
		}
	})
}

// TestSelectEpisodes_CountsLimitSkips: it's from selectEpisodes' FINAL result that the report takes
// downloaded/pending. With 10 pending episodes and a ceiling of 3, 7 are left.
func TestSelectEpisodes_CountsLimitSkips(t *testing.T) {
	title := "Limit Count Anime"
	nodes := make([]anilist.AiringNode, 10)
	for i := range nodes {
		nodes[i] = anilist.AiringNode{ID: 500 + i, Episode: i + 1, TimeUntilAiring: -100}
	}
	anime := anilist.MediaList{
		Id: 500,
		Media: anilist.Media{
			Id:             500,
			Title:          anilist.Title{English: &title},
			AiringSchedule: anilist.AiringSchedule{Nodes: nodes},
		},
	}
	configs := &files.Config{MaxEpisodesPerAnime: 3}

	sel := selectEpisodes(configs, 3, anime, nodes, map[files.EpisodeKey]bool{}, map[files.EpisodeKey]files.EpisodeStruct{}, map[string]bool{}, nil, nil)

	if len(sel.toDownload) != 3 {
		t.Fatalf("expected 3 to download, got %d", len(sel.toDownload))
	}
	if sel.downloaded != 3 {
		t.Errorf("expected downloaded=3, got %d", sel.downloaded)
	}
	if sel.limitSkipped != 7 {
		t.Errorf("expected limitSkipped=7, got %d", sel.limitSkipped)
	}
}

// --- F1: guard de exclusao de pack ---

// packRecords builds N pack records (episodes 1..n) of the same anime, all pointing to the same
// hash and declaring the range start..end.
func packRecords(animeID, n, start, end int, hash string) []files.EpisodeStruct {
	eps := make([]files.EpisodeStruct, 0, n)
	for i := 1; i <= n; i++ {
		eps = append(eps, files.EpisodeStruct{
			AnimeID: animeID, EpisodeNumber: i, EpisodeHash: hash,
			IsBatch: true, BatchStart: start, BatchEnd: end,
		})
	}
	return eps
}

func keysOf(eps []files.EpisodeStruct) []files.EpisodeKey {
	keys := make([]files.EpisodeKey, 0, len(eps))
	for _, ep := range eps {
		keys = append(keys, ep.Key())
	}
	return keys
}

// Defect B: a season pack (1..23) downloaded under cour 1, which only recorded 11 episodes.
// Watching cour 1 puts the 11 records in the delete set, but the 12 episodes of cour 2 are in the
// torrent with no record at all — deleting would take cour 2's content along.
func TestRemoveEpisodesAndLinks_KeepsPackWithUnrecordedContent(t *testing.T) {
	const hash = "1111111111111111111111111111111111111111"
	saved := packRecords(108465, 11, 1, 23, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	if err := removeEpisodesAndLinks(fm, backend, testLibrarian(), keysOf(saved), saved, false, false); err != nil {
		t.Fatalf("removeEpisodesAndLinks: %v", err)
	}
	if _, ok := backend.Get(hash); !ok {
		t.Error("a pack declaring 1..23 with only 11 records has unowned content: the torrent must survive")
	}
}

// Non-regression: a pack declaring 1..12 with all 12 records in the delete set has no unowned
// content — it goes whole, as it always did. That's what frees space for the next pack.
func TestRemoveEpisodesAndLinks_RemovesFullyCoveredPack(t *testing.T) {
	const hash = "2222222222222222222222222222222222222222"
	saved := packRecords(108465, 12, 1, 12, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	if err := removeEpisodesAndLinks(fm, backend, testLibrarian(), keysOf(saved), saved, false, false); err != nil {
		t.Fatalf("removeEpisodesAndLinks: %v", err)
	}
	if _, ok := backend.Get(hash); ok {
		t.Error("a pack with its whole range recorded and in the delete set must be removed")
	}
}

// packRange builds the records of episodes first..last of an entry with total episodes, all on the
// same hash and declaring the pack's range start..end.
func packRange(animeID, first, last, total, start, end int, hash string) []files.EpisodeStruct {
	eps := make([]files.EpisodeStruct, 0, last-first+1)
	for i := first; i <= last; i++ {
		eps = append(eps, files.EpisodeStruct{
			AnimeID: animeID, AnimeTotalEpisodes: total, EpisodeNumber: i, EpisodeHash: hash,
			IsBatch: true, BatchStart: start, BatchEnd: end,
		})
	}
	return eps
}

// The leak the record count created: a 01-11 pack picked with progress 5 is born with 6 records,
// and comparing 6 with the span 11 gave "unowned content" forever — the torrent never left the disk
// and the records were deleted anyway. What claims the pack is the ENTRY's episode total (11), not
// the number of records.
func TestRemoveEpisodesAndLinks_RemovesPackDownloadedMidWatch(t *testing.T) {
	const hash = "4444444444444444444444444444444444444444"
	saved := packRange(108465, 6, 11, 11, 1, 11, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	if err := removeEpisodesAndLinks(fm, backend, testLibrarian(), keysOf(saved), saved, false, false); err != nil {
		t.Fatalf("removeEpisodesAndLinks: %v", err)
	}
	if _, ok := backend.Get(hash); ok {
		t.Error("the pack's 11 episodes belong to the entry: the torrent must leave the disk")
	}
	if len(fm.deletedEpisodeKeys) != len(saved) {
		t.Errorf("expected the %d records deleted along, got %d", len(saved), len(fm.deletedEpisodeKeys))
	}
}

// The record is the only place where the pack's declared range exists. Deleting it with the torrent
// still on disk leaves the pack orphaned: no future cour can adopt it (findCoveringPack requires
// the range) and the claimed-content count never closes again.
func TestRemoveEpisodesAndLinks_KeepsRecordsOfHeldPack(t *testing.T) {
	const hash = "5555555555555555555555555555555555555555"
	saved := packRange(108465, 1, 11, 11, 1, 23, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	if err := removeEpisodesAndLinks(fm, backend, testLibrarian(), keysOf(saved), saved, false, false); err != nil {
		t.Fatalf("removeEpisodesAndLinks: %v", err)
	}
	if _, ok := backend.Get(hash); !ok {
		t.Fatal("a pack with an unowned cour 2 must survive")
	}
	if len(fm.deletedEpisodeKeys) != 0 {
		t.Errorf("the record carries the range of the pack that stayed: expected none deleted, got %d", len(fm.deletedEpisodeKeys))
	}
}

// K-ON! after method C: 13 TV + S00E19 stores 1-13, claimed == span, the torrent leaves.
// The 1-23 / total 11 pin above stays green (next cour).
func TestRemoveEpisodesAndLinks_RemovesKOnPackOnceOVAIsOutOfSpan(t *testing.T) {
	const hash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	// Incident: progress was already 2, so records 3-13 of media 5680, total 13, span 1-13.
	saved := packRange(5680, 3, 13, 13, 1, 13, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	if err := removeEpisodesAndLinks(fm, backend, testLibrarian(), keysOf(saved), saved, false, false); err != nil {
		t.Fatalf("removeEpisodesAndLinks: %v", err)
	}
	if _, ok := backend.Get(hash); ok {
		t.Error("K-ON! 1-13 with total 13 has no unclaimed cour: the torrent must leave")
	}
}

// The cycle closes: with cour 2 adopted, the two cours together claim the pack's 23 episodes and
// the torrent goes whole. Each one's range is in ITS OWN local numbering (1..23 and -10..12), and
// that's why declaredSpan measures per anime_id — joining the two would give a pack of 34.
func TestRemoveEpisodesAndLinks_RemovesPackOnceEveryCourClaimsIt(t *testing.T) {
	const hash = "6666666666666666666666666666666666666666"
	saved := append(packRange(108465, 1, 11, 11, 1, 23, hash), packRange(127720, 1, 12, 12, -10, 12, hash)...)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	if err := removeEpisodesAndLinks(fm, backend, testLibrarian(), keysOf(saved), saved, false, false); err != nil {
		t.Fatalf("removeEpisodesAndLinks: %v", err)
	}
	if _, ok := backend.Get(hash); ok {
		t.Error("with both cours in the delete set no unowned content is left: the torrent must go")
	}
}

// "Delete torrent" is manual and the unit of deletion there is the torrent. The guard that holds
// the pack in the automatic pass can't turn the user's click into a success with no effect.
func TestRemoveTorrentWithEpisodes_ForcesPastTheUnclaimedGuard(t *testing.T) {
	const hash = "7777777777777777777777777777777777777777"
	saved := packRange(108465, 1, 11, 11, 1, 23, hash)
	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	if err := RemoveTorrentWithEpisodes(fm, backend, testLibrarian(), hash, RemoveTorrentOptions{}); err != nil {
		t.Fatalf("RemoveTorrentWithEpisodes: %v", err)
	}
	if _, ok := backend.Get(hash); ok {
		t.Error("a manual torrent deletion must remove the torrent, unowned content or not")
	}
	if len(fm.deletedEpisodeKeys) != len(saved) {
		t.Errorf("expected the %d records deleted, got %d", len(saved), len(fm.deletedEpisodeKeys))
	}
}

// Defect A: the same pass adopts the pack under a new media id (cour 2) and prunes cour 1's
// records. The snapshot that reaches handleSavedEpisodes is PRE-pass, so the new records are only
// in newEpisodes — without including them in the guard, the just-adopted torrent is deleted.
//
// Without a declared range (BatchStart 0) on purpose: that way the test isolates defect A from
// defect B.
func TestHandleSavedEpisodes_NewEpisodesProtectSharedTorrent(t *testing.T) {
	const hash = "3333333333333333333333333333333333333333"
	saved := packRecords(108465, 11, 0, 0, hash)
	newEps := packRecords(127720, 12, 0, 0, hash)

	backend := fakeWithTorrents(hash)
	fm := &mockFileManagerForEpisodes{savedEpisodes: saved}

	handleSavedEpisodes(fm, &files.Config{DeleteWatchedEpisodes: true}, backend, testLibrarian(), handleEpisodesData{
		savedEpisodes:   saved,
		keysToDelete:    keysOf(saved),
		checkedEpisodes: append(keysOf(saved), keysOf(newEps)...),
		newEpisodes:     newEps,
	})

	if _, ok := backend.Get(hash); !ok {
		t.Error("the torrent adopted by the same pass's new records must survive the pruning of the old ones")
	}
}
