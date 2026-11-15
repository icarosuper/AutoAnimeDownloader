package daemon

import (
	"AutoAnimeDownloader/src/internal/anilist"
	"AutoAnimeDownloader/src/internal/files"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// An anime's sources are every account that has it on its list in ANY status (alice finished it,
// bob plans it) plus "standalone" from the standalone file. An id nobody tracks and that isn't
// standalone stays out of the map: unknown, so filtered webhooks skip it.
func TestNotificationSources(t *testing.T) {
	defer anilist.MockAniListDo(func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		body := `{"data":{"Page":{"mediaList":[{"id":1,"status":"COMPLETED","progress":12,"media":{"id":21}}]}}}`
		if strings.Contains(string(b), `"userName":"bob"`) {
			body = `{"data":{"Page":{"mediaList":[{"id":2,"status":"PLANNING","progress":0,"media":{"id":21}}]}}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})()

	fm := &mockFileManagerForEpisodes{standaloneAnimes: []int{21, 33}}
	cfg := &files.Config{AnilistUsernames: []string{"alice", "bob"}}

	got := NotificationSources(fm)(cfg, []int{21, 33, 44})

	if want := []string{"anilist:alice", "anilist:bob", "standalone"}; !slices.Equal(got[21], want) {
		t.Errorf("anime 21: got %v, want %v", got[21], want)
	}
	if want := []string{"standalone"}; !slices.Equal(got[33], want) {
		t.Errorf("anime 33: got %v, want %v", got[33], want)
	}
	if _, ok := got[44]; ok {
		t.Errorf("an anime nobody tracks must be unknown, got %v", got[44])
	}
}
