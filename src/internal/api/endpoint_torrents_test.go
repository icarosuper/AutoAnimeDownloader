package api

import (
	"AutoAnimeDownloader/src/internal/files"
	"AutoAnimeDownloader/src/internal/torrents"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const magnetA = "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567"
const hashA = "0123456789abcdef0123456789abcdef01234567"
const magnetB = "magnet:?xt=urn:btih:9123456789abcdef0123456789abcdef01234567"
const hashB = "9123456789abcdef0123456789abcdef01234567"

var errTestLoadEpisodes = errors.New("boom")

func decodeTorrentList(t *testing.T, w *httptest.ResponseRecorder) []map[string]interface{} {
	t.Helper()
	var response SuccessResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if !response.Success {
		t.Fatal("Expected success=true")
	}
	raw, ok := response.Data.([]interface{})
	if !ok {
		t.Fatalf("Expected data to be an array, got %T", response.Data)
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		out = append(out, item.(map[string]interface{}))
	}
	return out
}

func TestHandleTorrentsRejectsNonGet(t *testing.T) {
	server := &Server{Torrents: torrents.NewFakeBackend(), FileManager: &mockFileManager{}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()

	handleTorrents(server)(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// Sem save_path o SessionManager devolve nil em List(). A resposta tem que ser uma lista
// vazia com HTTP 200 — nunca 500 e nunca "data": null.
func TestHandleTorrentsEmptyBackendReturnsEmptyArray(t *testing.T) {
	server := &Server{Torrents: torrents.NewFakeBackend(), FileManager: &mockFileManager{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()

	handleTorrents(server)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	if items := decodeTorrentList(t, w); len(items) != 0 {
		t.Errorf("Expected 0 torrents, got %d", len(items))
	}
}

func TestHandleTorrentsJoinsEpisodeMetadata(t *testing.T) {
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	fm := &mockFileManager{episodes: []files.EpisodeStruct{
		{AnimeID: 42, AnimeName: "Frieren", EpisodeHash: hashA, EpisodeNumber: 7},
	}}
	server := &Server{Torrents: backend, FileManager: fm}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()
	handleTorrents(server)(w, req)

	items := decodeTorrentList(t, w)
	if len(items) != 1 {
		t.Fatalf("Expected 1 torrent, got %d", len(items))
	}
	if items[0]["anime_name"] != "Frieren" {
		t.Errorf("anime_name = %v, want Frieren", items[0]["anime_name"])
	}
	if items[0]["episode_number"] != float64(7) {
		t.Errorf("episode_number = %v, want 7", items[0]["episode_number"])
	}
	if items[0]["is_batch"] != false {
		t.Errorf("is_batch = %v, want false", items[0]["is_batch"])
	}
}

// Um torrent batch cobre N episódios mas é UM torrent: tem que aparecer uma vez só, sem
// número de episódio.
func TestHandleTorrentsBatchAppearsOnceWithoutEpisodeNumber(t *testing.T) {
	backend := torrents.NewFakeBackend()
	backend.Add(magnetA)
	fm := &mockFileManager{episodes: []files.EpisodeStruct{
		{AnimeID: 42, AnimeName: "Frieren", EpisodeHash: hashA, EpisodeNumber: 1, IsBatch: true},
		{AnimeID: 42, AnimeName: "Frieren", EpisodeHash: hashA, EpisodeNumber: 2, IsBatch: true},
		{AnimeID: 42, AnimeName: "Frieren", EpisodeHash: hashA, EpisodeNumber: 3, IsBatch: true},
	}}
	server := &Server{Torrents: backend, FileManager: fm}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()
	handleTorrents(server)(w, req)

	items := decodeTorrentList(t, w)
	if len(items) != 1 {
		t.Fatalf("Expected 1 torrent for a batch, got %d", len(items))
	}
	if items[0]["is_batch"] != true {
		t.Errorf("is_batch = %v, want true", items[0]["is_batch"])
	}
	if items[0]["episode_number"] != nil {
		t.Errorf("episode_number = %v, want null for a batch", items[0]["episode_number"])
	}
}

// bytes_total é 0 enquanto a metadata não chega: progress tem que ser 0, não NaN nem
// divisão por zero.
func TestHandleTorrentsProgressIsZeroWithoutMetadata(t *testing.T) {
	backend := torrents.NewFakeBackend()
	backend.Add(magnetA)
	server := &Server{Torrents: backend, FileManager: &mockFileManager{}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()
	handleTorrents(server)(w, req)

	items := decodeTorrentList(t, w)
	if items[0]["progress"] != float64(0) {
		t.Errorf("progress = %v, want 0", items[0]["progress"])
	}
	if items[0]["eta_seconds"] != nil {
		t.Errorf("eta_seconds = %v, want null", items[0]["eta_seconds"])
	}
}

// Uma falha ao ler downloaded_episodes não pode derrubar a tela de progresso: o join é
// opcional, os dados do torrent não.
func TestHandleTorrentsSurvivesEpisodeLoadFailure(t *testing.T) {
	backend := torrents.NewFakeBackend()
	backend.Add(magnetA)
	fm := &mockFileManager{loadEpisodesErr: errTestLoadEpisodes}
	server := &Server{Torrents: backend, FileManager: fm}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()
	handleTorrents(server)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	items := decodeTorrentList(t, w)
	if len(items) != 1 {
		t.Fatalf("Expected 1 torrent, got %d", len(items))
	}
	if items[0]["anime_name"] != nil && items[0]["anime_name"] != "" {
		t.Errorf("anime_name = %v, want empty when the join failed", items[0]["anime_name"])
	}
}

// Pausing a finished torrent takes it out of "seeding" but Completed (piece-derived) stays
// true, so the sort must key on Completed rather than the status slug. Without this pin, a
// paused-but-complete torrent whose name sorts alphabetically first would jump to the top of
// the list, ahead of the genuinely unfinished torrents the user opened the screen to watch.
func TestHandleTorrentsSortsUnfinishedFirstThenAlphabetical(t *testing.T) {
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add magnetA failed: %v", err)
	}
	if _, err := backend.Add(magnetB); err != nil {
		t.Fatalf("Add magnetB failed: %v", err)
	}
	// Name sorts alphabetically before magnetA/magnetB's names, but it must still land last:
	// it is paused (status "stopped") yet piece-complete, exactly the state Pause() leaves
	// behind on a finished torrent (see FINDING 1 / decisions.md #30).
	const hashC = "ffffffffffffffffffffffffffffffffffffffff"
	backend.AddPaused(hashC, "AAA-paused-but-complete", 10, 10, true)

	server := &Server{Torrents: backend, FileManager: &mockFileManager{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents", nil)
	w := httptest.NewRecorder()
	handleTorrents(server)(w, req)

	items := decodeTorrentList(t, w)
	if len(items) != 3 {
		t.Fatalf("Expected 3 torrents, got %d", len(items))
	}

	gotNames := make([]string, len(items))
	for i, it := range items {
		gotNames[i] = it["name"].(string)
	}
	wantNames := []string{magnetA, magnetB, "AAA-paused-but-complete"}
	for i, want := range wantNames {
		if gotNames[i] != want {
			t.Errorf("position %d: name = %q, want %q (order: %v)", i, gotNames[i], want, gotNames)
		}
	}

	if items[2]["completed"] != true {
		t.Errorf("completed = %v, want true for the paused-but-complete torrent", items[2]["completed"])
	}
	if items[2]["status"] != "stopped" {
		t.Errorf("status = %v, want stopped", items[2]["status"])
	}
	if items[0]["completed"] != false || items[1]["completed"] != false {
		t.Errorf("expected the two unfinished torrents to have completed=false, got %v and %v", items[0]["completed"], items[1]["completed"])
	}
}

// Test helper functions for Task 4

func newTorrentActionServer(t *testing.T) (*Server, *torrents.FakeBackend) {
	t.Helper()
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	return &Server{Torrents: backend, FileManager: &mockFileManager{}}, backend
}

func postTorrentAction(handler http.HandlerFunc, hash string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/"+hash+"/action", nil)
	req.SetPathValue("hash", hash)
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func TestHandleTorrentPause(t *testing.T) {
	server, backend := newTorrentActionServer(t)

	w := postTorrentAction(handleTorrentPause(server), hashA)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	info, _ := backend.Get(hashA)
	if info.Status != "stopped" {
		t.Errorf("Status = %q, want stopped", info.Status)
	}
}

func TestHandleTorrentResume(t *testing.T) {
	server, backend := newTorrentActionServer(t)
	_ = backend.Pause(hashA)

	w := postTorrentAction(handleTorrentResume(server), hashA)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	info, _ := backend.Get(hashA)
	if info.Status != "downloading" {
		t.Errorf("Status = %q, want downloading", info.Status)
	}
}

func TestHandleTorrentAnnounce(t *testing.T) {
	server, backend := newTorrentActionServer(t)

	w := postTorrentAction(handleTorrentAnnounce(server), hashA)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	if calls := backend.AnnounceCalls(); len(calls) != 1 || calls[0] != hashA {
		t.Errorf("AnnounceCalls() = %v, want [%s]", calls, hashA)
	}
}

func TestHandleTorrentPrioritize(t *testing.T) {
	server, backend := newTorrentActionServer(t)
	_ = backend.Pause(hashA)

	w := postTorrentAction(handleTorrentPrioritize(server), hashA)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	if calls := backend.PrioritizeCalls(); len(calls) != 1 || calls[0] != hashA {
		t.Errorf("PrioritizeCalls() = %v, want [%s]", calls, hashA)
	}
	info, _ := backend.Get(hashA)
	if info.Status != "downloading" {
		t.Errorf("Status = %q, want downloading", info.Status)
	}
}

func TestHandleTorrentPrioritizeCompletedTorrentIs500(t *testing.T) {
	// Prioritizing something that already finished is a caller mistake, not a missing
	// torrent — a 404 here would send the UI looking for a torrent that is right there.
	server, backend := newTorrentActionServer(t)
	backend.AddCompleted(hashA, "/fake/"+hashA)

	w := postTorrentAction(handleTorrentPrioritize(server), hashA)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("Expected %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

// --- POST /api/v1/torrents/prioritize (batch) ---

func postPrioritizeBatch(handler http.HandlerFunc, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/prioritize", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func TestHandleTorrentsPrioritizeAppliesTheOrderReceived(t *testing.T) {
	server, backend := newTorrentActionServer(t)
	if _, err := backend.Add(magnetB); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	w := postPrioritizeBatch(handleTorrentsPrioritize(server), `{"hashes":["`+hashB+`","`+hashA+`"]}`)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	calls := backend.PrioritizeCalls()
	if len(calls) != 2 || calls[0] != hashB || calls[1] != hashA {
		t.Errorf("PrioritizeCalls() = %v, want [%s %s]", calls, hashB, hashA)
	}
}

func TestHandleTorrentsPrioritizeIgnoresUnknownAndCompletedHashes(t *testing.T) {
	// Uma lista de 12 episodios nao pode falhar inteira porque um deles terminou de baixar
	// entre o render e o clique.
	server, backend := newTorrentActionServer(t)
	backend.AddCompleted(hashB, "/fake/"+hashB)
	unknown := "ffffffffffffffffffffffffffffffffffffffff"

	w := postPrioritizeBatch(handleTorrentsPrioritize(server), `{"hashes":["`+unknown+`","`+hashB+`","`+hashA+`"]}`)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	if calls := backend.PrioritizeCalls(); len(calls) != 1 || calls[0] != hashA {
		t.Errorf("PrioritizeCalls() = %v, want [%s]", calls, hashA)
	}
}

func TestHandleTorrentsPrioritizeRejectsAnEmptyList(t *testing.T) {
	server, _ := newTorrentActionServer(t)

	for _, body := range []string{`{"hashes":[]}`, `{}`, `not json`} {
		if w := postPrioritizeBatch(handleTorrentsPrioritize(server), body); w.Code != http.StatusBadRequest {
			t.Errorf("body %q: expected %d, got %d", body, http.StatusBadRequest, w.Code)
		}
	}
}

func TestHandleTorrentActionsUnknownHashReturn404(t *testing.T) {
	server, _ := newTorrentActionServer(t)

	handlers := map[string]http.HandlerFunc{
		"pause":      handleTorrentPause(server),
		"resume":     handleTorrentResume(server),
		"announce":   handleTorrentAnnounce(server),
		"prioritize": handleTorrentPrioritize(server),
	}
	for name, handler := range handlers {
		w := postTorrentAction(handler, "ffffffffffffffffffffffffffffffffffffffff")
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: expected %d, got %d", name, http.StatusNotFound, w.Code)
		}
	}
}

func TestHandleTorrentActionsRejectNonPost(t *testing.T) {
	server, _ := newTorrentActionServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents/"+hashA+"/pause", nil)
	req.SetPathValue("hash", hashA)
	w := httptest.NewRecorder()
	handleTorrentPause(server)(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleTorrentActionsRejectEmptyHash(t *testing.T) {
	server, _ := newTorrentActionServer(t)

	w := postTorrentAction(handleTorrentPause(server), "")

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// --- DELETE /api/v1/torrents/{hash} ---

// trackingLibrarian is a files.Librarian for tests that need to assert whether
// RemoveFromLibrary was (or was not) called — the stubLibrarian in
// endpoint_config_test.go doesn't record calls, which is exactly the point of the
// keep_data=true assertion below.
type trackingLibrarian struct {
	removedPaths []string
}

func (l *trackingLibrarian) Organize(files.OrganizeRequest) ([]string, error) { return nil, nil }
func (l *trackingLibrarian) RemoveFromLibrary(path string) error {
	l.removedPaths = append(l.removedPaths, path)
	return nil
}
func (l *trackingLibrarian) ProbePath(string) error { return nil }

func deleteTorrentRequest(hash, query string) *http.Request {
	url := "/api/v1/torrents/" + hash
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodDelete, url, nil)
	req.SetPathValue("hash", hash)
	return req
}

func TestHandleTorrentDeleteHappyPath(t *testing.T) {
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	fm := &mockFileManager{episodes: []files.EpisodeStruct{
		{EpisodeNumber: 1, AnimeID: 42, EpisodeHash: hashA, LibraryPaths: []string{"/lib/ep1.mkv"}},
	}}
	lib := &trackingLibrarian{}
	server := &Server{Torrents: backend, FileManager: fm, Librarian: lib}

	w := httptest.NewRecorder()
	handleTorrentDelete(server)(w, deleteTorrentRequest(hashA, ""))

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if _, ok := backend.Get(hashA); ok {
		t.Error("torrent should have left the session")
	}
	if len(fm.episodes) != 0 {
		t.Errorf("Expected episode records to be gone, got %v", fm.episodes)
	}
}

func TestHandleTorrentDeleteKeepDataSkipsLibraryAndPassesFlagToBackend(t *testing.T) {
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	fm := &mockFileManager{episodes: []files.EpisodeStruct{
		{EpisodeNumber: 1, AnimeID: 42, EpisodeHash: hashA, LibraryPaths: []string{"/lib/ep1.mkv"}},
	}}
	lib := &trackingLibrarian{}
	server := &Server{Torrents: backend, FileManager: fm, Librarian: lib}

	w := httptest.NewRecorder()
	handleTorrentDelete(server)(w, deleteTorrentRequest(hashA, "keep_data=true"))

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if kept, ok := backend.RemovedKeepData[hashA]; !ok || !kept {
		t.Errorf("RemovedKeepData[%q] = %v, %v; want true, true", hashA, kept, ok)
	}
	if len(lib.removedPaths) != 0 {
		t.Errorf("Expected no library hardlink removal with keep_data=true, got %v", lib.removedPaths)
	}
}

func TestHandleTorrentDeleteBlockBlocksEveryEpisodeInGroup(t *testing.T) {
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	fm := &mockFileManager{episodes: []files.EpisodeStruct{
		{EpisodeNumber: 1, AnimeID: 42, EpisodeHash: hashA, IsBatch: true},
		{EpisodeNumber: 2, AnimeID: 42, EpisodeHash: hashA, IsBatch: true},
		{EpisodeNumber: 3, AnimeID: 42, EpisodeHash: hashA, IsBatch: true},
	}}
	lib := &trackingLibrarian{}
	server := &Server{Torrents: backend, FileManager: fm, Librarian: lib}

	w := httptest.NewRecorder()
	handleTorrentDelete(server)(w, deleteTorrentRequest(hashA, "block=true"))

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	wantBlocked := map[files.EpisodeKey]bool{
		{AnimeID: 42, Episode: 1}: true,
		{AnimeID: 42, Episode: 2}: true,
		{AnimeID: 42, Episode: 3}: true,
	}
	if len(fm.blockedEpisodes) != len(wantBlocked) {
		t.Fatalf("Expected 3 episodes blocked, got %v", fm.blockedEpisodes)
	}
	for _, k := range fm.blockedEpisodes {
		if !wantBlocked[k] {
			t.Errorf("Unexpected episode blocked: %v", k)
		}
	}
}

func TestHandleTorrentDeleteOrphanTorrentRemovedWithoutBlocking(t *testing.T) {
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	fm := &mockFileManager{}
	server := &Server{Torrents: backend, FileManager: fm, Librarian: &trackingLibrarian{}}

	w := httptest.NewRecorder()
	handleTorrentDelete(server)(w, deleteTorrentRequest(hashA, "block=true"))

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d (body: %s)", http.StatusOK, w.Code, w.Body.String())
	}
	if _, ok := backend.Get(hashA); ok {
		t.Error("orphan torrent should have been removed")
	}
	if len(fm.blockedEpisodes) != 0 {
		t.Errorf("Expected nothing blocked for an orphan torrent, got %v", fm.blockedEpisodes)
	}
}

func TestHandleTorrentDeleteUnknownHashReturns404(t *testing.T) {
	server := &Server{Torrents: torrents.NewFakeBackend(), FileManager: &mockFileManager{}}

	w := httptest.NewRecorder()
	handleTorrentDelete(server)(w, deleteTorrentRequest("ffffffffffffffffffffffffffffffffffffffff", ""))

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleTorrentDeleteRejectsNonDelete(t *testing.T) {
	server, _ := newTorrentActionServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/torrents/"+hashA, nil)
	req.SetPathValue("hash", hashA)
	w := httptest.NewRecorder()
	handleTorrentDelete(server)(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleTorrentDeleteRejectsEmptyHash(t *testing.T) {
	server, _ := newTorrentActionServer(t)

	w := httptest.NewRecorder()
	handleTorrentDelete(server)(w, deleteTorrentRequest("", ""))

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleTorrentDeleteInvalidKeepDataReturns400(t *testing.T) {
	server, _ := newTorrentActionServer(t)

	w := httptest.NewRecorder()
	handleTorrentDelete(server)(w, deleteTorrentRequest(hashA, "keep_data=talvez"))

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// --- GET /torrents/{hash}/files ---

func filesRequest(hash string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents/"+hash+"/files", nil)
	req.SetPathValue("hash", hash)
	return req
}

func int64Ptr(v int64) *int64 { return &v }

func TestHandleTorrentFilesListsPathsWithEpisodeNumbers(t *testing.T) {
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	backend.SetFiles(hashA, []torrents.FileInfo{
		{Path: "[Judas] Frieren - S01E03.mkv", Size: 1490000000, BytesCompleted: int64Ptr(780000000)},
		{Path: "NCOP01.mkv", Size: 212000000, BytesCompleted: int64Ptr(212000000)},
	})
	server := &Server{Torrents: backend, FileManager: &mockFileManager{}}

	w := httptest.NewRecorder()
	handleTorrentFiles(server)(w, filesRequest(hashA))

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	items := decodeTorrentList(t, w)
	if len(items) != 2 {
		t.Fatalf("Expected 2 files, got %d", len(items))
	}
	// A ordem é a do metadata, não reordenada: é a ordem em que o release foi montado.
	if items[0]["path"] != "[Judas] Frieren - S01E03.mkv" {
		t.Errorf("path = %v", items[0]["path"])
	}
	if items[0]["episode"] != float64(3) {
		t.Errorf("episode = %v, want 3", items[0]["episode"])
	}
	if items[0]["bytes_completed"] != float64(780000000) {
		t.Errorf("bytes_completed = %v", items[0]["bytes_completed"])
	}
	if items[1]["episode"] != nil {
		t.Errorf("episode for NCOP01.mkv = %v, want null", items[1]["episode"])
	}
}

// Torrent parado: a rain libera as pieces, FileStats() falha e só sobra Files() — sem
// progresso por arquivo. O JSON tem que dizer "não sei" (null), não "nada baixado" (0).
func TestHandleTorrentFilesPausedTorrentHasNullBytesCompleted(t *testing.T) {
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	backend.SetFiles(hashA, []torrents.FileInfo{{Path: "ep01.mkv", Size: 100}})
	server := &Server{Torrents: backend, FileManager: &mockFileManager{}}

	w := httptest.NewRecorder()
	handleTorrentFiles(server)(w, filesRequest(hashA))

	items := decodeTorrentList(t, w)
	if len(items) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(items))
	}
	raw, ok := items[0]["bytes_completed"]
	if !ok {
		t.Fatal("bytes_completed missing from the payload")
	}
	if raw != nil {
		t.Errorf("bytes_completed = %v, want null", raw)
	}
}

// Metadata ainda não chegou: 200 com "data": [], nunca "data": null nem erro.
func TestHandleTorrentFilesWithoutMetadataReturnsEmptyArray(t *testing.T) {
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	server := &Server{Torrents: backend, FileManager: &mockFileManager{}}

	w := httptest.NewRecorder()
	handleTorrentFiles(server)(w, filesRequest(hashA))

	if w.Code != http.StatusOK {
		t.Fatalf("Expected %d, got %d", http.StatusOK, w.Code)
	}
	if items := decodeTorrentList(t, w); len(items) != 0 {
		t.Errorf("Expected 0 files, got %d", len(items))
	}
}

// A pasta do pack costuma carregar a faixa no nome ("… - 00 ~ 12 [1080p]/…"), que envenena a
// cascata de regexes: o caminho completo devolve nil em todo arquivo. O número tem que sair do
// NOME DO ARQUIVO. Caso real, pack do Erai-raws de Mushoku Tensei II.
func TestHandleTorrentFilesEpisodeComesFromTheFileNameNotTheFolder(t *testing.T) {
	backend := torrents.NewFakeBackend()
	if _, err := backend.Add(magnetA); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	const dir = "[Erai-raws] Mushoku Tensei II - Isekai Ittara Honki Dasu - 00 ~ 12 [1080p][Multiple Subtitle]"
	backend.SetFiles(hashA, []torrents.FileInfo{
		{Path: dir + "/[Erai-raws] Mushoku Tensei II - Isekai Ittara Honki Dasu - 07 [1080p][Multiple Subtitle].mkv", Size: 1},
	})
	server := &Server{Torrents: backend, FileManager: &mockFileManager{}}

	w := httptest.NewRecorder()
	handleTorrentFiles(server)(w, filesRequest(hashA))

	items := decodeTorrentList(t, w)
	if len(items) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(items))
	}
	if items[0]["episode"] != float64(7) {
		t.Errorf("episode = %v, want 7", items[0]["episode"])
	}
}

func TestHandleTorrentFilesUnknownHashReturns404(t *testing.T) {
	server := &Server{Torrents: torrents.NewFakeBackend(), FileManager: &mockFileManager{}}

	w := httptest.NewRecorder()
	handleTorrentFiles(server)(w, filesRequest(hashB))

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleTorrentFilesRejectsNonGet(t *testing.T) {
	server := &Server{Torrents: torrents.NewFakeBackend(), FileManager: &mockFileManager{}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/torrents/"+hashA+"/files", nil)
	req.SetPathValue("hash", hashA)

	w := httptest.NewRecorder()
	handleTorrentFiles(server)(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleTorrentFilesRejectsEmptyHash(t *testing.T) {
	server := &Server{Torrents: torrents.NewFakeBackend(), FileManager: &mockFileManager{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/torrents//files", nil)

	w := httptest.NewRecorder()
	handleTorrentFiles(server)(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// --- codec lido do cabeçalho do arquivo ---

// tinyMKV monta o menor Matroska que responde "qual o codec de vídeo": cabeçalho EBML, Segment,
// Tracks, um TrackEntry de vídeo. A montagem detalhada (trilhas fora de ordem, pulo de
// elemento gordo, arquivo truncado) é testada em files/mkv_test.go — aqui o que se prova é o
// LIGAMENTO: o handler junta DataDir + Path e só lê arquivo completo.
func tinyMKV(codecID string) []byte {
	elem := func(id []byte, payload []byte) []byte {
		out := append([]byte{}, id...)
		n := uint32(len(payload))
		out = append(out, 0x10|byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
		return append(out, payload...)
	}
	trackEntry := elem([]byte{0xAE}, append(
		elem([]byte{0x83}, []byte{0x01}), // TrackType = vídeo
		elem([]byte{0x86}, []byte(codecID))...,
	))
	tracks := elem([]byte{0x16, 0x54, 0xAE, 0x6B}, trackEntry)
	header := elem([]byte{0x1A, 0x45, 0xDF, 0xA3}, []byte{0x42, 0x82, 0x88, 'm', 'a', 't', 'r', 'o', 's', 'k', 'a'})
	return append(header, elem([]byte{0x18, 0x53, 0x80, 0x67}, tracks)...)
}

func TestHandleTorrentFilesReadsTheCodecFromTheFileHeader(t *testing.T) {
	dir := t.TempDir()
	content := tinyMKV("V_MPEGH/ISO/HEVC")
	if err := os.WriteFile(filepath.Join(dir, "ep07.mkv"), content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hashA, dir)
	size := int64(len(content))
	backend.SetFiles(hashA, []torrents.FileInfo{
		{Path: "ep07.mkv", Size: size, BytesCompleted: &size},
	})
	server := &Server{Torrents: backend, FileManager: &mockFileManager{}}

	w := httptest.NewRecorder()
	handleTorrentFiles(server)(w, filesRequest(hashA))

	items := decodeTorrentList(t, w)
	if len(items) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(items))
	}
	if items[0]["codec"] != "HEVC" {
		t.Errorf("codec = %v, want HEVC", items[0]["codec"])
	}
}

// O rain não baixa sequencial: um arquivo a 90% pode estar sem exatamente as primeiras peças,
// que é onde mora o cabeçalho. Ler antes do fim devolveria lixo ou nada — nem se tenta.
func TestHandleTorrentFilesSkipsTheCodecWhileTheFileIsIncomplete(t *testing.T) {
	dir := t.TempDir()
	content := tinyMKV("V_MPEGH/ISO/HEVC")
	if err := os.WriteFile(filepath.Join(dir, "ep07.mkv"), content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hashA, dir)
	partial := int64(len(content) - 1)
	backend.SetFiles(hashA, []torrents.FileInfo{
		{Path: "ep07.mkv", Size: int64(len(content)), BytesCompleted: &partial},
	})
	server := &Server{Torrents: backend, FileManager: &mockFileManager{}}

	w := httptest.NewRecorder()
	handleTorrentFiles(server)(w, filesRequest(hashA))

	items := decodeTorrentList(t, w)
	if _, present := items[0]["codec"]; present {
		t.Errorf("codec = %v, want the field omitted", items[0]["codec"])
	}
}

// Formato diferente de mkv sai sem codec: MP4 tem estrutura análoga mas exigiria um segundo
// parser, e em anime é praticamente tudo mkv.
func TestHandleTorrentFilesSkipsTheCodecForNonMKV(t *testing.T) {
	dir := t.TempDir()
	content := tinyMKV("V_MPEGH/ISO/HEVC")
	if err := os.WriteFile(filepath.Join(dir, "ep07.mp4"), content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	backend := torrents.NewFakeBackend()
	backend.AddCompleted(hashA, dir)
	size := int64(len(content))
	backend.SetFiles(hashA, []torrents.FileInfo{
		{Path: "ep07.mp4", Size: size, BytesCompleted: &size},
	})
	server := &Server{Torrents: backend, FileManager: &mockFileManager{}}

	w := httptest.NewRecorder()
	handleTorrentFiles(server)(w, filesRequest(hashA))

	items := decodeTorrentList(t, w)
	if _, present := items[0]["codec"]; present {
		t.Errorf("codec = %v, want the field omitted", items[0]["codec"])
	}
}
