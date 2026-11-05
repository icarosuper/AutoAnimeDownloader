package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// packFixture is a two-episode pack: both records point at the same torrent.
func packFixture() []files.EpisodeStruct {
	ep := func(n int) files.EpisodeStruct {
		return files.EpisodeStruct{
			AnimeID: 7, AnimeName: "My Anime", AnimeTotalEpisodes: 12,
			EpisodeHash: testEpisodeHash, EpisodeNumber: n,
			IsBatch: true, BatchStart: 5, BatchEnd: 6,
		}
	}
	return []files.EpisodeStruct{ep(5), ep(6)}
}

func batchRequest(target, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.SetPathValue("id", "7")
	return req
}

// TestHandleBatchDeleteEpisodes_FreesSharedPack is why the endpoint exists: deleting both episodes
// of a pack in one removal frees the torrent. N parallel single deletes could each see the other
// episode still on file, and the pack guard kept the torrent every time.
func TestHandleBatchDeleteEpisodes_FreesSharedPack(t *testing.T) {
	fm := &mockFileManager{episodes: packFixture()}
	server, backend := episodeActionServer(t, fm)

	w := httptest.NewRecorder()
	handleBatchDeleteEpisodes(server)(w, batchRequest("/api/v1/animes/7/episodes/batch-delete", `{"episodes":[6,5,6]}`))

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, ok := backend.Get(testEpisodeHash); ok {
		t.Error("the pack torrent must be removed once all its episodes are deleted together")
	}
	if len(fm.episodes) != 0 {
		t.Errorf("want both records removed, got %+v", fm.episodes)
	}
	if len(fm.blockedEpisodes) != 2 {
		t.Errorf("want both episodes blocked (duplicates dropped), got %+v", fm.blockedEpisodes)
	}
}

// TestHandleBatchDeleteEpisodes_UnknownEpisodeDeletesNothing: validation is all or nothing.
func TestHandleBatchDeleteEpisodes_UnknownEpisodeDeletesNothing(t *testing.T) {
	fm := &mockFileManager{episodes: packFixture()}
	server, backend := episodeActionServer(t, fm)

	w := httptest.NewRecorder()
	handleBatchDeleteEpisodes(server)(w, batchRequest("/api/v1/animes/7/episodes/batch-delete", `{"episodes":[5,9]}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", w.Code, w.Body.String())
	}
	if len(fm.episodes) != 2 || len(fm.blockedEpisodes) != 0 {
		t.Errorf("nothing may change when one episode is unknown: records=%d blocked=%d", len(fm.episodes), len(fm.blockedEpisodes))
	}
	if _, ok := backend.Get(testEpisodeHash); !ok {
		t.Error("the torrent must stay")
	}
}

func TestHandleBatchEpisodes_RejectsBadBody(t *testing.T) {
	for _, body := range []string{``, `{}`, `{"episodes":[]}`, `{"episodes":[0]}`, `{"episodes":[-1,2]}`} {
		for name, handler := range map[string]func(*Server) http.HandlerFunc{
			"delete": handleBatchDeleteEpisodes, "redownload": handleBatchRedownloadEpisodes,
		} {
			fm := &mockFileManager{episodes: packFixture()}
			server, _ := episodeActionServer(t, fm)
			w := httptest.NewRecorder()
			handler(server)(w, batchRequest("/api/v1/animes/7/episodes/batch-x", body))
			if w.Code != http.StatusBadRequest {
				t.Errorf("%s %q: want 400, got %d", name, body, w.Code)
			}
		}
	}
}

// TestHandleBatchRedownloadEpisodes_RemovalFailureAborts: same rule as the single redownload — a
// surviving record would shadow the new torrent, so nothing is downloaded.
func TestHandleBatchRedownloadEpisodes_RemovalFailureAborts(t *testing.T) {
	anilistCalled := false
	defer mockAniListRecorder(&anilistCalled)()

	fm := &mockFileManager{episodes: packFixture(), deleteEpisodesErr: errors.New("disk full")}
	server, _ := episodeActionServer(t, fm)

	w := httptest.NewRecorder()
	handleBatchRedownloadEpisodes(server)(w, batchRequest("/api/v1/animes/7/episodes/batch-redownload", `{"episodes":[5,6]}`))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500 when removal fails, got %d: %s", w.Code, w.Body.String())
	}
	if anilistCalled {
		t.Error("the handler must abort before trying to download again")
	}
}

// TestHandleBatchRedownloadEpisodes_FailureDoesNotStopTheRest: every episode is attempted and each
// failure is reported; the removed records stay removed and the episodes unblocked, for the next
// pass to pick up.
func TestHandleBatchRedownloadEpisodes_FailureDoesNotStopTheRest(t *testing.T) {
	anilistCalled := false
	defer mockAniListRecorder(&anilistCalled)()

	fm := &mockFileManager{
		episodes:        packFixture(),
		blockedEpisodes: []files.EpisodeKey{{AnimeID: 7, Episode: 6}},
	}
	server, _ := episodeActionServer(t, fm)

	w := httptest.NewRecorder()
	handleBatchRedownloadEpisodes(server)(w, batchRequest("/api/v1/animes/7/episodes/batch-redownload", `{"episodes":[5,6]}`))

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 with the failures listed, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Redownloaded []int                 `json:"redownloaded"`
			Failed       []batchEpisodeFailure `json:"failed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data.Failed) != 2 || resp.Data.Failed[0].Episode != 5 || resp.Data.Failed[1].Episode != 6 {
		t.Errorf("want both episodes attempted and reported, got %+v", resp.Data.Failed)
	}
	if len(fm.episodes) != 0 || len(fm.blockedEpisodes) != 0 {
		t.Errorf("records must stay removed and episodes unblocked: records=%d blocked=%+v", len(fm.episodes), fm.blockedEpisodes)
	}
}
