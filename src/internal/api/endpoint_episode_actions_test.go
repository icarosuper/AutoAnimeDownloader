package api

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/nyaa"
	"AutoAnimeDownloader/src/internal/torrents"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testEpisodeHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// episodeActionServer wires a Server with the collaborators the episode-action handlers need:
// a mock FileManager, an in-memory torrent backend and a real Librarian (a no-op while the
// episode has no LibraryPaths).
func episodeActionServer(t *testing.T, fm *mockFileManager) (*Server, *torrents.FakeBackend) {
	t.Helper()
	backend := torrents.NewFakeBackend()
	backend.AddCompleted(testEpisodeHash, t.TempDir())
	return &Server{
		State:       daemon.NewState(),
		FileManager: fm,
		Torrents:    backend,
		Librarian:   files.NewLibrarian(files.NewOSFileSystem()),
	}, backend
}

// episodeRequest builds a request with the anime/episode path values already set.
func episodeRequest(method, target string, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	req.SetPathValue("id", "7")
	req.SetPathValue("episodeNumber", "5")
	return req
}

func savedEpisodeFixture() []files.EpisodeStruct {
	return []files.EpisodeStruct{{
		AnimeID:       7,
		AnimeName:     "My Anime",
		EpisodeHash:   testEpisodeHash,
		EpisodeName:   "My Anime - Episode 5",
		EpisodeNumber: 5,
	}}
}

// mockAniListRecorder mocks AniList and records whether it was called at all — used to prove a
// handler aborted before reaching ManualDownloadEpisode / ManualDownloadEpisodeWithMagnet.
func mockAniListRecorder(called *bool) func() {
	return anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		*called = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"Page":{"mediaList":[]}}}`)),
		}, nil
	})
}

// TestHandleDeleteEpisode_DeleteFailureReturns500 covers P1.3: when removing the file record
// fails, DELETE must not answer 200 pretending success.
func TestHandleDeleteEpisode_DeleteFailureReturns500(t *testing.T) {
	fm := &mockFileManager{
		episodes:          savedEpisodeFixture(),
		deleteEpisodesErr: errors.New("disk full"),
	}
	server, _ := episodeActionServer(t, fm)

	w := httptest.NewRecorder()
	handleDeleteEpisode(server)(w, episodeRequest(http.MethodDelete, "/api/v1/animes/7/episodes/5", ""))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500 when DeleteEpisodesFromFile fails, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleDeleteEpisode_Success guarantees the happy path stays 200 (no regression).
func TestHandleDeleteEpisode_Success(t *testing.T) {
	fm := &mockFileManager{episodes: savedEpisodeFixture()}
	server, backend := episodeActionServer(t, fm)

	w := httptest.NewRecorder()
	handleDeleteEpisode(server)(w, episodeRequest(http.MethodDelete, "/api/v1/animes/7/episodes/5", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, ok := backend.Get(testEpisodeHash); ok {
		t.Error("the torrent (seeded copy) must be removed on a manual delete")
	}
	if len(fm.episodes) != 0 {
		t.Errorf("want the record removed from the file, got %+v", fm.episodes)
	}
}

// TestHandleRedownloadEpisode_RemovalFailureAborts covers P1.3 in the worst case: if removal
// fails, the handler must answer 500 and NOT add a new torrent (which would have no tracking,
// since the old record survived with the old hash).
func TestHandleRedownloadEpisode_RemovalFailureAborts(t *testing.T) {
	anilistCalled := false
	defer mockAniListRecorder(&anilistCalled)()

	fm := &mockFileManager{
		episodes:          savedEpisodeFixture(),
		deleteEpisodesErr: errors.New("disk full"),
	}
	server, backend := episodeActionServer(t, fm)

	w := httptest.NewRecorder()
	handleRedownloadEpisode(server)(w, episodeRequest(http.MethodPost, "/api/v1/animes/7/episodes/5/redownload", ""))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500 when removal fails, got %d: %s", w.Code, w.Body.String())
	}
	if anilistCalled {
		t.Error("the handler must abort before trying to download again (AniList must not be consulted)")
	}
	// The only torrent present is the old one (removed best-effort or not); no new one entered.
	for _, tr := range backend.List() {
		if tr.Hash != testEpisodeHash {
			t.Errorf("no new torrent must be added, got %s", tr.Hash)
		}
	}
}

// TestHandleReplaceEpisodeWithMagnet_RemovalFailureAborts: the same guarantee for replacing an
// episode with the user's magnet.
func TestHandleReplaceEpisodeWithMagnet_RemovalFailureAborts(t *testing.T) {
	anilistCalled := false
	defer mockAniListRecorder(&anilistCalled)()

	fm := &mockFileManager{
		episodes:          savedEpisodeFixture(),
		deleteEpisodesErr: errors.New("disk full"),
	}
	server, backend := episodeActionServer(t, fm)

	magnet := `{"magnet":"magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`
	w := httptest.NewRecorder()
	handleReplaceEpisodeWithMagnet(server)(w, episodeRequest(http.MethodPost, "/api/v1/animes/7/episodes/5/replace", magnet))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500 when removal fails, got %d: %s", w.Code, w.Body.String())
	}
	if anilistCalled {
		t.Error("the handler must abort before adding the new magnet")
	}
	for _, tr := range backend.List() {
		if tr.Hash != testEpisodeHash {
			t.Errorf("no new torrent must be added, got %s", tr.Hash)
		}
	}
}

// TestHandleReplaceAnimeWithMagnet_RemovalFailureAborts: same guarantee for replacing the whole anime.
func TestHandleReplaceAnimeWithMagnet_RemovalFailureAborts(t *testing.T) {
	anilistCalled := false
	defer mockAniListRecorder(&anilistCalled)()

	fm := &mockFileManager{
		episodes:          savedEpisodeFixture(),
		deleteEpisodesErr: errors.New("disk full"),
	}
	server, backend := episodeActionServer(t, fm)

	magnet := `{"magnet":"magnet:?xt=urn:btih:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/animes/7/replace", strings.NewReader(magnet))
	req.SetPathValue("id", "7")

	w := httptest.NewRecorder()
	handleReplaceAnimeWithMagnet(server)(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500 when removal fails, got %d: %s", w.Code, w.Body.String())
	}
	if anilistCalled {
		t.Error("the handler must abort before adding the new magnet")
	}
	for _, tr := range backend.List() {
		if tr.Hash != testEpisodeHash {
			t.Errorf("no new torrent must be added, got %s", tr.Hash)
		}
	}
}

// --- handleDownloadEpisode must UPDATE an existing record, not discard the update ---

const replacementHash = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// realEpisodeStore is a FileManagerInterface whose episode persistence is the REAL
// files.FileManager over a temp dir, with only LoadConfigs stubbed. The api package's
// mockFileManager cannot be used here: its SaveEpisodesToFile and UpsertEpisodes are
// identical (both overwrite the slice), so it hides exactly the append-only-dedupe
// behaviour this test exists to pin down.
type realEpisodeStore struct {
	*files.FileManager
	configs *files.Config
}

func (r *realEpisodeStore) LoadConfigs() (*files.Config, error) { return r.configs, nil }

func newRealEpisodeStore(t *testing.T) *realEpisodeStore {
	t.Helper()
	dir := t.TempDir()
	return &realEpisodeStore{
		FileManager: files.NewManager(
			files.NewOSFileSystem(),
			filepath.Join(dir, "config.json"),
			filepath.Join(dir, "downloaded_episodes"),
			filepath.Join(dir, "blocked_episodes"),
			filepath.Join(dir, "anime_settings"),
			filepath.Join(dir, "standalone_animes"),
		),
		configs: &files.Config{
			AnilistUsernames:   []string{"testuser"},
			CompletedAnimePath: filepath.Join(dir, "completed"),
			CheckInterval:      10,
			EpisodeRetryLimit:  5,
		},
	}
}

// mockAnimeInfo makes GetAnimeInfo resolve anime 7 with a single airing node whose AniList
// airing-node ID is 42 and whose episode number is 5.
func mockAnimeInfo() func() {
	return anilist.MockAniListDo(func(_ *http.Request) (*http.Response, error) {
		body := `{"data":{"Page":{"mediaList":[{"id":700,"status":"CURRENT","progress":0,"media":{
			"id":7,"title":{"english":"My Anime","romaji":"My Anime"},"status":"RELEASING",
			"episodes":12,"airingSchedule":{"nodes":[{"id":42,"episode":5,"airingAt":0,"timeUntilAiring":0}]}}}]}}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
}

// mockNyaaResult returns one scrapeable release so ManualDownloadEpisode has a magnet to add.
func mockNyaaResult() func() {
	html := `<!doctype html><html><body><table class="torrent-list"><tbody><tr>
	  <td></td>
	  <td><a title="My.Anime.S01E05.1080p">My Anime S01E05 1080p</a></td>
	  <td><a></a><a href="magnet:?xt=urn:btih:` + replacementHash + `">magnet</a></td>
	  <td></td>
	  <td>` + time.Now().Format("2006-01-02 15:04") + `</td>
	  <td>100</td>
	</tr></tbody></table></body></html>`
	return nyaa.MockNyaaHttpGet(func(string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(html)),
			Header:     make(http.Header),
		}, nil
	})
}

// TestHandleDownloadEpisode_UpdatesExistingRecord pins the P1.1 failure mode reached through the
// API instead of the automatic loop. handleDownloadEpisode does NOT delete the old record first,
// so with append-only SaveEpisodesToFile a pre-existing record for this EpisodeID made the whole
// update vanish: the new hash was lost (JobOrganize can no longer join the torrent by hash) and
// the stale LibraryPaths survived (organizeTorrent then believes the episode is already
// organized and never creates the new hardlink).
func TestHandleDownloadEpisode_UpdatesExistingRecord(t *testing.T) {
	fm := newRealEpisodeStore(t)

	// A record already on disk for this episode: old hash, already organized somewhere.
	if err := fm.SaveEpisodesToFile([]files.EpisodeStruct{{
		AnimeID:       7,
		AnimeName:     "My Anime",
		EpisodeHash:   testEpisodeHash,
		EpisodeName:   "My Anime - Episode 5",
		EpisodeNumber: 5,
		LibraryPaths:  []string{"/library/My Anime/My Anime - E05.mkv"},
	}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	server, backend := episodeActionServer(t, nil)
	server.FileManager = fm
	backend.NextHash = replacementHash

	restoreAniList := mockAnimeInfo()
	defer restoreAniList()
	restoreNyaa := mockNyaaResult()
	defer restoreNyaa()

	rec := httptest.NewRecorder()
	handleDownloadEpisode(server)(rec, episodeRequest(http.MethodPost, "/api/v1/animes/7/episodes/5/download", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	saved, err := fm.LoadSavedEpisodes()
	if err != nil {
		t.Fatalf("LoadSavedEpisodes: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("want exactly 1 record, got %d: %+v", len(saved), saved)
	}
	ep := saved[0]

	if ep.EpisodeHash != replacementHash {
		t.Errorf("hash must be updated to the new torrent: want %s, got %s", replacementHash, ep.EpisodeHash)
	}
	if ep.LibraryPaths != nil {
		t.Errorf("LibraryPaths must be cleared (the new file needs to be organized again), got %v", ep.LibraryPaths)
	}
	if !ep.ManuallyManaged {
		t.Error("manual download must set ManuallyManaged: true")
	}
	if ep.EpisodeNumber != 5 {
		t.Errorf("EpisodeNumber must stay 5, got %d", ep.EpisodeNumber)
	}
}
