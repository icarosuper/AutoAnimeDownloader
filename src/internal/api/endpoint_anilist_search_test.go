package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
)

// captureSearchQuery devolve o corpo da query SearchMedia enviada a AniList. As demais queries
// (lista do usuario, customLists) respondem vazio — o guard nao e o assunto aqui.
func captureSearchQuery(sent *string) func() {
	return anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		payload := emptyListJSON
		if strings.Contains(string(body), "SearchMedia") {
			*sent = string(body)
			payload = `{"data":{"Page":{"media":[]}}}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payload)),
		}, nil
	})
}

func getSearch(t *testing.T, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	handleAniListSearch(standaloneServer(&mockFileManager{}))(rec, req)
	return rec
}

// TestHandleAniListSearch_IncludeUnreleased: o parametro precisa CHEGAR na query. Um teste que
// so olhasse o status 200 passaria com o handler ignorando o parametro inteiro.
func TestHandleAniListSearch_IncludeUnreleased(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		wantStatusNot  bool
		wantStatusCode int
	}{
		{"ausente esconde os nao lancados", "/api/v1/anilist/search?q=one", true, http.StatusOK},
		{"false esconde os nao lancados", "/api/v1/anilist/search?q=one&include_unreleased=false", true, http.StatusOK},
		{"true mostra tudo", "/api/v1/anilist/search?q=one&include_unreleased=true", false, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent string
			defer captureSearchQuery(&sent)()

			rec := getSearch(t, tt.url)
			if rec.Code != tt.wantStatusCode {
				t.Fatalf("quero %d, veio %d: %s", tt.wantStatusCode, rec.Code, rec.Body.String())
			}
			if got := strings.Contains(sent, "status_not"); got != tt.wantStatusNot {
				t.Fatalf("status_not na query = %v, quero %v. Query: %s", got, tt.wantStatusNot, sent)
			}
		})
	}
}

// TestHandleAniListSearch_InvalidBoolIs400: valor que nao e booleano e erro do cliente, nao um
// silencioso "assume false" — mesmo contrato de keep_data/block em /torrents.
func TestHandleAniListSearch_InvalidBoolIs400(t *testing.T) {
	var sent string
	defer captureSearchQuery(&sent)()

	rec := getSearch(t, "/api/v1/anilist/search?q=one&include_unreleased=talvez")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("quero 400, veio %d: %s", rec.Code, rec.Body.String())
	}
	if sent != "" {
		t.Fatal("um parametro invalido nao pode gastar uma chamada do limite de 30/min da AniList")
	}
}

// TestHandleAniListSearch_ShortTermSkipsAniList: abaixo de 3 caracteres nada sai da maquina, e um
// parametro invalido junto tampouco vira 400 — a lista vazia vem antes de qualquer validacao.
func TestHandleAniListSearch_ShortTermSkipsAniList(t *testing.T) {
	var sent string
	defer captureSearchQuery(&sent)()

	rec := getSearch(t, "/api/v1/anilist/search?q=on")
	if rec.Code != http.StatusOK {
		t.Fatalf("quero 200, veio %d", rec.Code)
	}
	if sent != "" {
		t.Fatalf("termo curto nao pode tocar a AniList, foi enviado: %s", sent)
	}
}
