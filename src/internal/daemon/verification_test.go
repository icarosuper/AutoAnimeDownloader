package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
)

func TestSearchAnilist_FiltersByMediaStatus(t *testing.T) {
	anilistJSON := `{"data": {"Page": {"mediaList": [
		{"id": 1, "status": "CURRENT", "progress": 0, "customLists": {}, "media": {
			"id": 100, "format": "TV", "status": "RELEASING", "episodes": 12,
			"title": {"english": "Airing Anime", "romaji": "Airing Anime"},
			"synonyms": [], "relations": {"edges": []},
			"airingSchedule": {"nodes": []}
		}},
		{"id": 2, "status": "CURRENT", "progress": 0, "customLists": {}, "media": {
			"id": 200, "format": "TV", "status": "NOT_YET_RELEASED", "episodes": 12,
			"title": {"english": "Unreleased Anime", "romaji": "Unreleased Anime"},
			"synonyms": [], "relations": {"edges": []},
			"airingSchedule": {"nodes": []}
		}}
	]}}}`

	restore := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(anilistJSON)), Header: make(http.Header)}, nil
	})
	defer restore()

	config := &files.Config{
		AnilistUsernames:      []string{"user1"},
		CompletedAnimePath:    "/tmp/completed",
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
	}

	resp, err := searchAnilist(&mockFileManagerForEpisodes{}, config, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(resp.Data.Page.MediaList) != 1 {
		t.Fatalf("expected 1 anime after media-status filter, got %d", len(resp.Data.Page.MediaList))
	}
	if resp.Data.Page.MediaList[0].Media.Id != 100 {
		t.Errorf("expected surviving anime to be media id 100 (RELEASING), got %d", resp.Data.Page.MediaList[0].Media.Id)
	}
}

func TestSearchAnilist_EmptyMediaStatusesAllowsNothing(t *testing.T) {
	anilistJSON := `{"data": {"Page": {"mediaList": [
		{"id": 1, "status": "CURRENT", "progress": 0, "customLists": {}, "media": {
			"id": 100, "format": "TV", "status": "RELEASING", "episodes": 12,
			"title": {"english": "Airing Anime", "romaji": "Airing Anime"},
			"synonyms": [], "relations": {"edges": []},
			"airingSchedule": {"nodes": []}
		}}
	]}}}`

	restore := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(anilistJSON)), Header: make(http.Header)}, nil
	})
	defer restore()

	config := &files.Config{
		AnilistUsernames:      []string{"user1"},
		CompletedAnimePath:    "/tmp/completed",
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{},
	}

	resp, err := searchAnilist(&mockFileManagerForEpisodes{}, config, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(resp.Data.Page.MediaList) != 0 {
		t.Fatalf("expected 0 animes with empty DownloadMediaStatuses, got %d", len(resp.Data.Page.MediaList))
	}
}

// --- Status-deletion rule across accounts (decisions.md #43) ---
//
// Download is OR (one account wanting it is enough) and deletion is AND (every account that
// has the anime must want it). What these tests protect is the AND side, where a mistake
// deletes a file.

// mockStatusByAccount answers the GetMediaListStatus tiebreak query per user.
// A user missing from the map returns an empty list = "this account does not track the anime".
func mockStatusByAccount(t *testing.T, byUser map[string]string) func() {
	t.Helper()
	return anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		var payload struct {
			Variables struct {
				UserName string `json:"userName"`
			} `json:"variables"`
		}
		body, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unexpected body: %s", body)
		}
		status, tracked := byUser[payload.Variables.UserName]
		if !tracked {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"data":{"Page":{"mediaList":[]}}}`))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(
			fmt.Sprintf(`{"data":{"Page":{"mediaList":[{"status":%q}]}}}`, status)))}, nil
	})
}

func deleteRuleConfig() *files.Config {
	return &files.Config{
		AnilistUsernames: []string{"accountA", "accountB"},
		DeleteStatuses:   []string{"DROPPED", "COMPLETED"},
	}
}

func TestDeletableMediaIDs_AllAccountsAgree(t *testing.T) {
	// The list search already brought the anime as deletable in BOTH accounts: nobody needs
	// to be queried, and the anime is deleted.
	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		t.Error("must not query AniList when both lists already agree")
		return nil, fmt.Errorf("unexpected call")
	})()

	got := deletableMediaIDs(deleteRuleConfig(), map[string]map[int]bool{
		"accountA": {500: true},
		"accountB": {500: true},
	}, []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 500}})

	if !got[500] {
		t.Error("both accounts want to delete, the anime must be deletable")
	}
}

func TestDeletableMediaIDs_DifferentDeleteStatusesStillDeletes(t *testing.T) {
	// DROPPED in one account and COMPLETED in the other: different statuses, both deletion.
	defer mockStatusByAccount(t, map[string]string{"accountB": "COMPLETED"})()

	got := deletableMediaIDs(deleteRuleConfig(), map[string]map[int]bool{
		"accountA": {500: true}, // DROPPED, came from the list
		"accountB": {},          // COMPLETED arrives via the tiebreak
	}, []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 500}})

	if !got[500] {
		t.Error("different deletion statuses in the two accounts must still delete")
	}
}

func TestDeletableMediaIDs_NeutralStatusInOtherAccountVetoes(t *testing.T) {
	// Account B still intends to watch: deleting here would destroy episodes it wants.
	defer mockStatusByAccount(t, map[string]string{"accountB": "PLANNING"})()

	got := deletableMediaIDs(deleteRuleConfig(), map[string]map[int]bool{
		"accountA": {500: true},
		"accountB": {},
	}, []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 500}})

	if got[500] {
		t.Error("an account with the anime in a neutral status must veto the deletion")
	}
}

func TestDeletableMediaIDs_AccountWithoutTheAnimeDoesNotVeto(t *testing.T) {
	// Account B never had the anime on the list: it does not vote. Without this, the common
	// case (anime tracked by a single account) would never be deleted again.
	defer mockStatusByAccount(t, map[string]string{})()

	got := deletableMediaIDs(deleteRuleConfig(), map[string]map[int]bool{
		"accountA": {500: true},
		"accountB": {},
	}, []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 500}})

	if !got[500] {
		t.Error("an account that does not track the anime cannot veto the deletion")
	}
}

func TestDeletableMediaIDs_FailedAccountFetchVetoes(t *testing.T) {
	// Account B's fetch failed (it is not even in the map): without its opinion there is no
	// unanimity, and the next pass tries again. Deleting in the dark is irreversible.
	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		t.Error("an account missing from the map is a fetch failure, it must not become a query")
		return nil, fmt.Errorf("unexpected call")
	})()

	got := deletableMediaIDs(deleteRuleConfig(), map[string]map[int]bool{
		"accountA": {500: true},
	}, []files.EpisodeStruct{{EpisodeNumber: 1, AnimeID: 500}})

	if got[500] {
		t.Error("with one account's list unavailable nothing can be deleted")
	}
}

func TestDeletableMediaIDs_OnlyConsidersAnimesOnDisk(t *testing.T) {
	// With no episode on disk the answer changes nothing — and that is what keeps the
	// tiebreak queries rare instead of one per completed anime on the whole list.
	defer anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		t.Error("an anime with no episode on disk should not produce a query")
		return nil, fmt.Errorf("unexpected call")
	})()

	got := deletableMediaIDs(deleteRuleConfig(), map[string]map[int]bool{
		"accountA": {500: true},
		"accountB": {},
	}, nil)

	if len(got) != 0 {
		t.Errorf("want no candidates without episodes on disk, got %v", got)
	}
}

// reportPassAniList mocks AniList with one releasing anime whose episode 1 already aired, so a
// full pass has exactly one episode to look for.
func reportPassAniList(t *testing.T) {
	t.Helper()
	const body = `{"data": {"Page": {"mediaList": [
		{"id": 1, "status": "CURRENT", "progress": 0, "customLists": {}, "media": {
			"id": 900, "format": "TV", "status": "RELEASING", "episodes": 12,
			"title": {"english": "Report Anime", "romaji": "Report Anime"},
			"synonyms": [], "relations": {"edges": []},
			"airingSchedule": {"nodes": [{"id": 1, "episode": 1, "timeUntilAiring": -100, "airingAt": 1}]}
		}}
	]}}}`
	restore := anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	t.Cleanup(restore)
}

// emptyNyaa makes every Nyaa page come back with no rows, so the pass finds no torrent.
func emptyNyaa(t *testing.T) {
	t.Helper()
	restore := nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`<html><body><table class="torrent-list"><tbody></tbody></table></body></html>`)),
			Header:     make(http.Header),
		}, nil
	})
	t.Cleanup(restore)
}

func reportPassConfig(t *testing.T) *files.Config {
	t.Helper()
	base := t.TempDir()
	return &files.Config{
		AnilistUsernames:      []string{"tester"},
		CompletedAnimePath:    base + "/library",
		CheckInterval:         10,
		DownloadStatuses:      []string{"CURRENT"},
		DownloadMediaStatuses: []string{"RELEASING", "FINISHED"},
		MaxEpisodesPerAnime:   12,
		EpisodeRetryLimit:     1,
		MinSeeders:            1,
	}
}

// TestAnimeVerification_PublishesReport: the report is published AFTER the SetLastCheckError(nil)
// at the end of the pass. If the order flips, SetLastCheckError clears what was just published
// and the report never appears — which is the only way this feature can fail silently.
func TestAnimeVerification_PublishesReport(t *testing.T) {
	reportPassAniList(t)
	emptyNyaa(t)

	fm := &lifecycleFM{configs: reportPassConfig(t)}
	state := NewState()

	AnimeVerification(context.Background(), fm, state, nil, torrents.NewFakeBackend(), files.NewLibrarian(files.NewOSFileSystem()))

	report := state.GetLastCheckReport()
	if report.FinishedAt.IsZero() {
		t.Fatal("a complete pass must publish a report with finished_at set")
	}
	if len(report.Problems) == 0 {
		t.Fatalf("want at least one problem, got %+v", report)
	}
	if report.Problems[0].Code != IssueNoTorrentFound {
		t.Errorf("want %q, got %q", IssueNoTorrentFound, report.Problems[0].Code)
	}
}

// TestAnimeVerification_CancelledLeavesNoReport: an interrupted pass leaves no report — it
// was incomplete, and a partial report would say "only this anime had a problem" when the
// others were never even looked at.
func TestAnimeVerification_CancelledLeavesNoReport(t *testing.T) {
	reportPassAniList(t)
	emptyNyaa(t)

	fm := &lifecycleFM{configs: reportPassConfig(t)}
	state := NewState()
	// Seed the PREVIOUS pass's report: without this the test would pass on a freshly created
	// State even if the whole clear were deleted, because a new State is born with a zero
	// report. What has to be locked is that the interrupted pass does NOT leave the old
	// report on the screen.
	state.SetLastCheckReport(CheckReport{
		FinishedAt: time.Now(),
		Problems:   []Issue{{AnimeID: 1, AnimeName: "Previous", Code: IssueNoTorrentFound}},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	AnimeVerification(ctx, fm, state, nil, torrents.NewFakeBackend(), files.NewLibrarian(files.NewOSFileSystem()))

	report := state.GetLastCheckReport()
	if !report.FinishedAt.IsZero() || len(report.Problems) != 0 || len(report.Limits) != 0 {
		t.Errorf("a cancelled pass must not leave a report, got %+v", report)
	}
}

// blockingFM holds the pass inside LoadSavedEpisodes until it is released, so a second pass
// can be fired with the first proven to be in progress.
type blockingFM struct {
	lifecycleFM
	blocked atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (m *blockingFM) LoadSavedEpisodes() ([]files.EpisodeStruct, error) {
	saved, err := m.lifecycleFM.LoadSavedEpisodes()
	if m.blocked.CompareAndSwap(false, true) {
		close(m.entered)
		<-m.release
	}
	return saved, err
}

func (m *blockingFM) loads() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadedSavedCount
}

// Two simultaneous passes (the loop's + POST /check) used to read episodes.json at the start
// and only write at the end: the second saw the list BEFORE the first's downloads and
// downloaded everything again — that is how an anime already covered by a pack got loose
// torrents of the same episodes, overwriting the pack's records. A concurrent pass is dropped.
func TestAnimeVerification_SkipsWhenAnotherPassIsRunning(t *testing.T) {
	reportPassAniList(t)
	emptyNyaa(t)

	fm := &blockingFM{
		lifecycleFM: lifecycleFM{configs: reportPassConfig(t)},
		entered:     make(chan struct{}),
		release:     make(chan struct{}),
	}
	state := NewState()

	done := make(chan struct{})
	go func() {
		defer close(done)
		AnimeVerification(context.Background(), fm, state, nil, torrents.NewFakeBackend(), files.NewLibrarian(files.NewOSFileSystem()))
	}()

	select {
	case <-fm.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the first pass did not start")
	}
	before := fm.loads()

	AnimeVerification(context.Background(), fm, state, nil, torrents.NewFakeBackend(), files.NewLibrarian(files.NewOSFileSystem()))

	if got := fm.loads(); got != before {
		t.Errorf("the concurrent pass should have been dropped, but it read the saved episodes (%d -> %d)", before, got)
	}

	close(fm.release)
	<-done

	// And the next pass, with the first already finished, runs normally.
	AnimeVerification(context.Background(), fm, state, nil, torrents.NewFakeBackend(), files.NewLibrarian(files.NewOSFileSystem()))
	if got := fm.loads(); got <= before {
		t.Errorf("once the first pass is done, the next one must run (reads: %d -> %d)", before, got)
	}
}
