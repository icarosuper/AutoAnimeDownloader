package api

import (
	"AutoAnimeDownloader/src/internal/daemon"
	"AutoAnimeDownloader/src/internal/files"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func newSettingsTestServer(t *testing.T) (*Server, *mockFileManager) {
	t.Helper()
	fm := &mockFileManager{}
	server := &Server{State: daemon.NewState(), FileManager: fm}
	return server, fm
}

func putSettings(t *testing.T, server *Server, id int, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/animes/"+strconv.Itoa(id)+"/settings", strings.NewReader(body))
	req.SetPathValue("id", strconv.Itoa(id))
	rec := httptest.NewRecorder()
	handleAnimeSettings(server)(rec, req)
	return rec
}

// Progress e ponteiro justamente para que um corpo SEM o campo nao seja lido como zero: um PUT
// vazio tem de preservar o progresso salvo, nao apaga-lo.
func TestPutAnimeSettings_AbsentFieldKeepsTheSavedProgress(t *testing.T) {
	server, fm := newSettingsTestServer(t)
	fm.animeSettings = map[int]files.AnimeSettings{7: {Progress: 48}}

	rec := putSettings(t, server, 7, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}
	if got := fm.animeSettings[7]; got.Progress != 48 {
		t.Errorf("esperava progresso preservado, obteve %+v", got)
	}

	rec = putSettings(t, server, 7, `{"progress":52}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, obteve %d: %s", rec.Code, rec.Body.String())
	}
	if got := fm.animeSettings[7]; got.Progress != 52 {
		t.Errorf("esperava progresso novo, obteve %+v", got)
	}
}

func TestPutAnimeSettings_RejectsNegativeProgress(t *testing.T) {
	server, _ := newSettingsTestServer(t)

	if rec := putSettings(t, server, 7, `{"progress":-1}`); rec.Code != http.StatusBadRequest {
		t.Errorf("esperava 400 para progresso negativo, obteve %d", rec.Code)
	}
}
