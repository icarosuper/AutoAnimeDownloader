package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"AutoAnimeDownloader/src/internal/anilist"
)

// captureSearchQuery returns the SearchMedia query body sent to AniList. The other queries
// (user list, customLists) respond empty — the guard is not the subject here.
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

// TestHandleAniListSearch_IncludeUnreleased: the parameter has to REACH the query. A test that
// only looked at status 200 would pass with the handler ignoring the parameter entirely.
func TestHandleAniListSearch_IncludeUnreleased(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		wantStatusNot  bool
		wantStatusCode int
	}{
		{"absent hides unreleased", "/api/v1/anilist/search?q=one", true, http.StatusOK},
		{"false hides unreleased", "/api/v1/anilist/search?q=one&include_unreleased=false", true, http.StatusOK},
		{"true shows everything", "/api/v1/anilist/search?q=one&include_unreleased=true", false, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent string
			defer captureSearchQuery(&sent)()

			rec := getSearch(t, tt.url)
			if rec.Code != tt.wantStatusCode {
				t.Fatalf("want %d, got %d: %s", tt.wantStatusCode, rec.Code, rec.Body.String())
			}
			if got := strings.Contains(sent, "status_not"); got != tt.wantStatusNot {
				t.Fatalf("status_not in query = %v, want %v. Query: %s", got, tt.wantStatusNot, sent)
			}
		})
	}
}

// TestHandleAniListSearch_InvalidBoolIs400: a non-boolean value is a client error, not a
// silent "assume false" — same contract as keep_data/block on /torrents.
func TestHandleAniListSearch_InvalidBoolIs400(t *testing.T) {
	var sent string
	defer captureSearchQuery(&sent)()

	rec := getSearch(t, "/api/v1/anilist/search?q=one&include_unreleased=maybe")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if sent != "" {
		t.Fatal("an invalid parameter must not spend a call from AniList's 30/min limit")
	}
}

// TestHandleAniListSearch_ShortTermSkipsAniList: under 3 characters nothing leaves the machine,
// and an invalid parameter alongside it is not 400 either — the empty list comes before any
// validation.
func TestHandleAniListSearch_ShortTermSkipsAniList(t *testing.T) {
	var sent string
	defer captureSearchQuery(&sent)()

	rec := getSearch(t, "/api/v1/anilist/search?q=on")
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if sent != "" {
		t.Fatalf("a short term must not hit AniList, sent: %s", sent)
	}
}
